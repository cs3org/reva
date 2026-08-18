# Admin API

The `admin` gRPC service is reva's operator surface: an authenticated,
reva-specific API for looking at a running fleet and running bounded, audited
operations against it. It is defined inside reva (not in the CS3 APIs) because it
is tooling, and its Go stubs are exported from [`pkg/admin/adminpb`](../../../../pkg/admin/adminpb)
so external scripts integrate by a plain import.

It builds on three things: the **service registry** (which process runs what,
[`pkg/registry`](../../../../pkg/registry)), the **control channel** (a way to
reach a running service instance, [`pkg/control/controlpb`](../../../../pkg/control/controlpb) +
[`internal/grpc/control`](../../control)), and the **invocation framework** every
service can opt into ([`pkg/invoke`](../../../../pkg/invoke)). The Admin API is
the client that ties them together; the invocation framework is usable on its
own.

## Architecture

The Admin API is off by default. It turns on when a process loads the `admin`
service in a gRPC block with an `admin_group` set — typically the process that
also runs the gateway, so a single co-located admin can see the whole fleet
through the shared registry.

```
   reva CLI ──gRPC──► admin service (one gRPC block, admin-scoped)
                          │
      reads ─────────────┤ registry: services, nodes, health, metadata
                          │
      invokes ───────────┘ resolve selector ──► control channel(s)
                                                     │ (one gRPC port per process)
                                                     ▼
                                          route by node id ──► service instance
                                                     │
                                        shared defaults (config) + service methods
```

Two planes:

- **Introspection** reads the registry directly (server info, per-service
  health, the fleet's services with their nodes, a service's redacted config).
  No service has to cooperate; the runtime already advertises everything in
  registry metadata.
- **Invocations** run an operation *inside* a service instance. The admin does
  not call the service's business gRPC; it goes through the **control channel**,
  a separate small gRPC server every process stands up (see below), and the
  process routes the call to the addressed instance.

### The control channel

Every reva process that hosts anything invokable stands up **one** control gRPC
server (`reva.control.v1beta1`), on its own port, independent of whether the
Admin API service is loaded. It hosts `Invoke`, its streaming twin
`InvokeStream`, and `ListInvocations`. It is
*not* registered as a service of its own — instead every real service node
advertises the channel's address in its `control` registry metadata, and the
address is shared by all services in the process.

The control server routes by **node id** (`host:port/service`): the runtime
registers each loaded service as an *instance* under its node id, carrying the
service's redacted config and its optional `Invokable`. Routing by id (not by
service name) means a process can run several instances of the same service and
each is still addressed precisely.

A **serverless** service has no listen address, so its node id uses the
process's control address instead (`control-addr/service`) and its registry node
advertises an empty address. Everything else is identical — the `control`
metadata, the heartbeat, and the invocation routing — so the admin sees and
reaches it like any other instance.

### Selectors

The `Invoke` and `GetServiceConfig` RPCs take a **selector**, resolved
server-side against the registry — the admin counterpart of how the gateway
selects a peer:

- a plain **service name** (`userprovider`) fans out to **every** live instance,
  returning one result per instance;
- a **node id** (`10.0.0.4:19003/userprovider`, as shown by
  `reva admin services -o wide/json`) targets **exactly that** instance; and
- a **partial id** widens the scope: an address (`10.0.0.4:19003`) targets every
  instance bound to it, a bare host (`10.0.0.4`) every instance on that machine.
  A bare token that is both a service name and a host resolves as the service.

The admin resolves the selector to a set of `(node id, control address)`
endpoints, dials each process's control channel, and passes the node id as the
routing target. A unary `Invoke` is **synchronous**: every addressed instance
runs and reports before the call returns.

### Streaming invocations

Some operations produce a *stream* of results over time rather than a single one
(following logs is the built-in example). These ride `InvokeStream`, the
streaming twin of `Invoke`: same selector, same routing, but the admin
multiplexes each instance's stream into one, labelling every item with its node,
until the client disconnects. A developer declares one by giving an invocation a
streaming handler (`Set.Add(...).Stream().HandleStream(fn)`); the catalog flags
it `streaming` so the CLI knows to open a stream. Multi-instance follows
interleave as items arrive (like `kubectl logs -f`).

## Reading logs

Every reva process keeps its most recent log lines in a bounded in-memory ring
([`pkg/logtail`](../../../../pkg/logtail)) that the logger tees into — the real
sink (stdout, a file) is unchanged. That buffer is exposed as a built-in `logs`
invocation on every service (like `config`), so it needs no per-service code and
rides the same selector + control fan-out as any invocation. `reva admin logs`
drives it:

```
reva admin logs userprovider              # recent lines from every userprovider, merged
reva admin logs 10.0.0.4:19003/userprovider   # one instance
reva admin logs 10.0.0.4:19003            # every service at that address
reva admin logs 10.0.0.4                  # every service on that machine
reva admin logs userprovider -f           # follow (InvokeStream) until Ctrl-C
reva admin logs userprovider -level warn -since 5m -grep timeout -n 500
```

Because the buffer is process-wide, lines are attributed to a service by their
`service` log field. It is stamped in two places: construction-time loggers get
it from the service loader, and **per-request** loggers get it from the appctx
interceptor — the gRPC server records which proto services each reva service
registers, so a request's full method resolves to its owning service (the HTTP
router does the same by routed prefix). Process-level lines that belong to no
service (startup, registry) match no service filter; the raw stream, them
included, is reachable with `admin invoke <node-id> logs all=true`. The window
is bounded (`[log] tail`, default 2000 lines); there is no deeper history.

## Draining instances

`reva admin services drain <selector>` takes the matched instances **out of
rotation**: the process starts advertising them as `draining` in the registry,
and the shared [service selector](../../../../pkg/service/selector.go) already
excludes draining nodes from every `Pick`, so no new traffic is routed to them.
Established in-flight calls finish on their own — the selector has no per-request
handle to sever — so a drain is graceful by construction. `enable` returns them
to `ready`. Both ride the same selector fan-out, so `drain 10.0.0.4:19003`
drains a whole process and `drain userprovider` drains every instance of a
service.

This is a built-in `rotation` invocation flipping an in-memory per-node flag the
heartbeat reads (like `logs level`): it is **not persisted** — a restart or
config reload re-registers the node `ready`. A drained node stays alive and
control-reachable, so `logs`, `stack`, `config` and `enable` still work against
it; only offline nodes are hidden from the admin fan-out. The current state is
visible in `admin services` (the STATE column).

## Request activity (is it safe to restart?)

`reva admin services activity <selector>` reports, per instance, how many
requests it is **currently serving** (`in-flight`), how many it has served
(`total`), and how long it has been **idle**. This is the companion to `drain`:
drain stops new traffic, and activity tells you when the in-flight traffic has
finished, so you know when a node has quiesced and is safe to restart.

```
admin services drain 10.0.0.4:19003            # out of rotation
admin services -wait -idle 5s activity 10.0.0.4:19003   # block until quiesced
# ... restart the process ...
admin services enable 10.0.0.4:19003
```

`-wait` polls until **every** matched instance has zero in-flight requests and
has been idle for at least `-idle` (default 5s), or `-timeout` (default 2m)
elapses — in which case it lists the still-active instances and exits non-zero,
so it drops into a shell procedure. (As with the other subcommands, flags come
before the `activity` token: `services -wait … activity <selector>`.)

`-methods` adds the per-RPC-method breakdown for gRPC services (busiest first),
so you can see *what* an instance is serving, not just how much:

```
10.0.0.4:19003/userprovider: in-flight=2 total=1841 idle=0s
    GetUser          in-flight=1 total=1203 idle=0s
    GetUserByClaim   in-flight=1 total= 512 idle=0s
    GetUserGroups    in-flight=0 total= 126 idle=3s
```

The method set is bounded, so this is cardinality-safe. The breakdown lives in
the same per-instance counter (a lazily-built per-method map alongside the
lock-free aggregate, so `-wait`'s quiescence check never pays for it). HTTP
requests count toward the aggregate only.

The count is kept always-on in a small counter (`pkg/activity`) created **per
service per server** in the runtime and shared by reference between the request
choke points that feed it — the same ones that stamp the `service` log field,
the gRPC appctx interceptor and the HTTP router — and the `activity` invocation
that reports it. There is no process-global state: because a server is one
listener, the counter is `1:1` with a node id, so the numbers are **per
instance** — unlike the shared `logs`/`stack` process buffers, two co-located
same-name instances (on different listeners) count separately. Admin API and
control-channel RPCs are excluded, so the activity query itself and long-lived
admin streams (e.g. `logs -f`) never register as traffic. It counts
registry-routed *and* directly addressed requests alike — a request reaching a
drained node through a pinned connection still shows up (correctly: it *is*
being served). Counters reset on restart (a fresh process starts idle).

## Security and scope

The security model is sudo-style step-up, built on an `admin` auth scope that is
mutually isolated from user scopes (see
[`pkg/auth/scope/admin.go`](../../../../pkg/auth/scope/admin.go)):

- An **admin-scoped token** satisfies Admin API and control requests and
  *nothing else* — the storage/share verifiers do not recognize admin messages.
- A **user-scoped token** can never satisfy an admin request — the always-true
  user scope is guarded by `isAdminResource`.

Getting an admin token is a deliberate step-up:

- `RequestAdmin` is the one method reachable with an ordinary **user** token (it
  is the elevation door). It checks the caller's membership in `admin_group` and,
  only on success, mints a **short-TTL, admin-only** token (scope `{admin}`, no
  `user` key). Grant and deny are both audited.
- Every other method (including `Invoke`, and the control channel itself)
  requires that admin token. The control server runs the standard auth
  interceptor, so it is exactly as protected as the Admin API — an
  unauthenticated call to the control port is rejected. An admin token is
  authorized purely by its scope, so validating it never triggers a user-group
  lookup — its bearer may be a synthetic local-root identity (see below) that no
  user provider knows, and on a `skip_user_groups_in_token` fleet that lookup
  would otherwise reject a valid admin token on every fan-out hop.
- `Impersonate` mints an ordinary **user-scoped** token for a target user (never
  admin+user), so downstream services apply their normal user checks. It is
  audited and needs the machine-auth api key configured.

Invocations are classified `readonly | mutating | dangerous`; the CLI prompts
before a `dangerous` one. Every invocation and action emits an audit event via
[`pkg/admin`](../../../../pkg/admin) (`audit=true` on the context logger).

### Local root

The step-up above answers "who are you?" with a token, which is right for remote
access. For an operator working **on the box**, the Admin API can also be served
on a **Unix socket** where the transport itself is the authentication — like
`sudo`, `docker.sock`, or PostgreSQL `peer`. There is no token and no login: a
permitted local user runs `reva admin …` and it just works.

On each connection the socket reads the peer's OS credentials via `SO_PEERCRED`
(the kernel reports the connecting process's uid/gid, so it cannot be forged). If
the uid is permitted, the interceptor mints an admin token internally and injects
it, so the request runs exactly like a remotely-elevated one — fan-out to other
processes' control channels included. Two gates apply:

- **the socket file's permissions** — who can even connect; and
- **an optional `socket_group`** — with it set, the peer's uid must belong to
  that Unix group (checked against the OS group database, supplementary groups
  included); without it, anyone who can open the socket is admin.

reva sets the socket ownership/mode to match: `0660` owned by `socket_group` (so
reva must be root or a member of it), else `0600` (only the process owner and
root). Every local-root request is audited with the OS user and uid.

It is **on by default** (Linux): the server binds a well-known path —
`/run/reva/admin.sock`, or `$XDG_RUNTIME_DIR/reva/admin.sock` when the former is
not writable (rootless) — and the CLI probes the same list, so `reva admin …`
just works locally with no flag and no login. Because the default is `0600`, that
means only the reva process owner and root until a `socket_group` opens it.
Binding the default is best-effort (a missing runtime dir simply disables it);
set `socket = "off"` to turn it off, or an explicit path to override.

## Configuration

The Admin API is enabled by loading the `admin` service with an `admin_group`.
The control channel is configured once per process in the `[grpc]` section.

```toml
[grpc]
# The per-process control channel's bind address. Empty => a random port on all
# interfaces (the auth interceptor, not the bind host, is what protects it).
control_address = "127.0.0.1:19700"

[grpc.services.admin]
address    = "127.0.0.1:19010"
admin_group = "sailing-lovers"   # required: members may step up. Unset => no Admin API.
admin_ttl  = "15m"               # lifetime of a minted admin token (default 15m)

# Optional: enables Impersonate.
machine_auth_apikey = "..."

# Local root over a Unix socket (Linux, ON BY DEFAULT). Omit to use the default
# path; "off" disables it; an explicit path overrides the default.
socket       = "off"             # e.g. "/run/reva/admin.sock"
socket_group = "reva-admin"      # optional: restrict to this group (else 0600, owner-only)
```

A process that hosts an invokable service but not the `admin` service still
stands up its control channel (so a remote admin can reach it); it just reads the
same `[grpc] control_address` key.

The in-memory log buffer that backs `reva admin logs` is sized in the `[log]`
section:

```toml
[log]
tail = 2000   # recent lines kept in memory for `admin logs` (0 disables it)
```

### Using the CLI

```
reva -insecure -host <gateway> login -username <u> -password <p> basic
reva admin elevate -admin-host <admin:port>     # step up, stores a short-TTL admin token
reva admin services [-v] [-o wide|json] [service]
reva admin services drain  <selector> [-y]   # take instances out of rotation
reva admin services enable <selector>        # return them to rotation
reva admin services [-wait [-idle D] [-timeout D]] activity <selector>  # in-flight/idle
reva admin config   <service|node-id> [-o toml|json] [-diff]
reva admin invocations <service|node-id>
reva admin invoke   [-stream] <selector> <invocation> [key=val ...]
reva admin logs     <selector> [-f] [-n N] [-level L] [-since D] [-grep P] [-o text|json]
reva admin logs level <selector> [trace|debug|info|warn|error]   # report/set runtime level
reva admin trace    <traceid> | -user <username>   # one request/user across the fleet
reva admin stack    <selector> [-grep P]           # goroutine dumps, e.g. of a hung process
reva admin jobs     list | active | runs | status <id> | run <job> [k=v] | trigger <job> | cancel <id> | stop <job>
reva admin impersonate <user>

# Local root: on the box, no login/elevate/flag — the CLI finds the socket. Only
# if it is absent (or denies you) does it need the network host + elevate above.
reva admin services
```

## Jobs

`reva admin jobs` drives the background jobs runner ([`pkg/rjobs`](../../../../pkg/rjobs)),
and shows the general shape for adding **typed Admin API methods to interact with
a service**: the surface is typed RPCs on the `AdminAPI`, and each method's body
reuses the **invoke** channel to reach the service — fanning out or targeting one
runner by where the state lives. The admin keeps **no store or business
dependency**; it reaches the runner the same way it reaches any service.

Two sources of truth, two access patterns:

- **The driver's live internal state** — registered jobs and what each is doing
  right now (running/idle, scope, worker pool), held in per-process memory and
  largely absent from any database. `InspectJobs` **fans out** to every runner
  and merges. `jobs list` is the job-centric view, `jobs active` the run-centric
  one:

  ```
  jobs list                 # NAME, KIND, SCHEDULE, SCOPE, STATUS (running-where)
  jobs active               # per node: the runs executing now + worker pool
  ```

- **The durable run ledger** — the persisted history of runs, in the shared store
  (nats queue + SQL status). `ListJobRuns`/`GetJobRun` **target one** runner (any;
  the store is shared). The mutations `EnqueueJob`/`TriggerJob`/`CancelJobRun`/
  `CancelPeriodicJob` also target one — **ingress ≠ execution**: an enqueued run
  is later claimed by whatever worker is free (possibly a different runner), and
  cancels broadcast cluster-wide. They are audited.

  ```
  jobs runs [-job X] [-owner Y] [-state s,...] [-internal] [-n N]
  jobs status <run-id>
  jobs run <job> [k=v ...] [-owner U]   # enqueue on-demand → run id
  jobs trigger <job>                    # fire a periodic job now
  jobs cancel <run-id>   |   jobs stop <job>
  ```

Under the hood the runner exposes an `invoke.Set` (`inspect`/`runs`/`status`/
`enqueue`/`trigger`/`cancel`/`stop`) that the admin calls over the control
channel — the admin↔runner transport, never a user-facing surface. Caveats:
`all-nodes` periodic jobs never touch the store so they have no `runs` history
but *do* show in `list`/`active`; and `failed` is not terminal — the framework
retries.

## Adding an invocation to a service (for developers)

A service exposes admin operations by building an [`invoke.Set`](../../../../pkg/invoke/set.go)
— you declare each method once and the framework does the name→handler routing,
builds the catalog, and validates required arguments. There is no `Invoke`
switch to maintain. Every service also gets the built-in invocations (`config`,
`logs`, `loglevel`, `stack`, `version`) for free, so what you add is *on top of*
those. (Built-ins live in
`pkg/invoke`, one self-registering file each — see `config.go`.)

Two steps: embed the `*invoke.Set` (that makes the service `Invokable`, which the
runtime discovers), and declare your methods at construction.

```go
type service struct {
    *invoke.Set              // makes the service Invokable
    groupmgr group.Manager
}

func New(ctx context.Context, m map[string]any) (rgrpc.Service, error) {
    // ...build groupManager...
    svc := &service{groupmgr: groupManager}
    svc.registerInvocations()
    return svc, nil
}

// All a developer writes to add an operator method: name it, declare its args,
// hand it a handler.
func (s *service) registerInvocations() {
    s.Set = invoke.NewSet()
    s.Set.Add("is_member", "Report whether a user is a member of a group").
        Arg("group", "the group id").
        Arg("user", "the user id").
        OptArg("idp", "the identity provider of the group and user").
        Handle(s.invokeIsMember)
}

func (s *service) invokeIsMember(ctx context.Context, a invoke.Args) (invoke.Result, error) {
    ok, err := s.groupmgr.HasMember(ctx,
        &grouppb.GroupId{OpaqueId: a.String("group"), Idp: a.String("idp")},
        &userpb.UserId{OpaqueId: a.String("user"), Idp: a.String("idp")},
    )
    if err != nil {
        return nil, err
    }
    return invoke.Result{"member": ok}, nil
}
```

That is the whole thing. `is_member` now shows up in
`reva admin invocations groupprovider`, its required `group`/`user` args are
checked before the handler runs, and:

```
reva admin invoke groupprovider is_member group=sailing-lovers user=einstein idp=cernbox
  # → 127.0.0.1:19004/groupprovider: {"member":true}   (one line per instance)
```

Notes:

- **Kinds**: `.Mutating()` or `.Dangerous()` after `Add(...)` change the kind
  (default readonly); `dangerous` makes the CLI confirm.
- **Args**: handlers receive `invoke.Args` with typed accessors — `a.String(k)`,
  `a.Bool(k)`, `a.Int(k)`, `a.Has(k)`. Arguments arrive as strings over the wire.
- **Result**: return an `invoke.Result` (a `map[string]any`); the framework
  redacts nothing for you, so do not put secrets in it. Return an error to
  surface a per-instance failure.
- **Streaming**: for an operation that emits results over time, use
  `.Stream().HandleStream(fn)` with a handler
  `func(ctx, invoke.Args, emit invoke.StreamEmit) error` that calls `emit` per
  result until `ctx` is done or `emit` reports the client gone. It is reached via
  `InvokeStream`; the built-in `logs` invocation is an example.
