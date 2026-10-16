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

## Shares and public links

The `usershareprovider` and the `publicshareprovider` expose the operations an
operator needs on the shares and public links of **any** user, as invocations
on top of the built-in ones. Their catalog (`reva admin invocations
usershareprovider`, `... publicshareprovider`):

```
list          owner= initiator= sharee= id= resource= space= path= [as=] [include_orphans=] [include_expired=] [resolve_paths=] [limit=] [offset=]
get           id=  (links: id= or token=)                        [resolve_path=]
create        path= initiator= sharee= permissions= [type=user|group] [expires_at=] [ignore_existing=] [force=]
              (links: path= initiator= permissions= [password=] [expires_at=] [name=] [description=] [internal=] [force=])
remove        id=  [db_only=]                                     (links: id= or token=)
set_expiry    id= expires_at=<date|RFC 3339|none> [sync=]         (links: [force=] to clear an editor link's expiry)
set_password  id= password=                                       (links only; empty removes it)
sync_expiry   id=                                                 (shares only: drop the storage grant of an expired share)
transfer      id= initiator=                                      (project shares only; the new initiator must administer the project)
move          id= path= [owner=]                                  (dangerous: repoints the record, grants are not touched)
mark_orphan   id=
```

Two things make these different from the built-ins:

- **They act as users.** An invocation runs with the operator's admin-only
  token, which no protected CS3 endpoint accepts. So a handler that has to stat
  a path, create a share or remove a grant does it *as the share's initiator*
  (or the owner), with a token minted on the spot by
  [`pkg/admin.Identity`](../../../../pkg/admin/identity.go) from the
  deployment's JWT secret (`jwt_secret` in the service config, falling back to
  `[shared]`). `create` and `remove` therefore go through the gateway and end
  up with exactly the storage grants, hierarchy checks and permission checks a
  share made from the web interface gets. The admin token itself never travels
  downstream: `Identity.As` replaces it in the outgoing metadata.
- **Mutations target one instance.** The share database is shared, so a
  mutation selected by service name would run once per instance. Address a
  node id (`host:port/usershareprovider`); `list` and `get` are safe to fan
  out.

Every mutation emits an audit event (`action=usershareprovider.<name>` or
`publicshareprovider.<name>`, actor = the operator, target = the user acted
as) in the provider's log, next to the admin service's own `invoke` audit line.
The result of a link never includes its password hash.

```
reva admin invoke usershareprovider list owner=einstein resolve_paths=true
reva admin invoke host:9144/usershareprovider create path=/eos/user/e/einstein/doc initiator=einstein sharee=marie permissions=editor
reva admin invoke host:9144/usershareprovider set_expiry id=4711 expires_at=2026-12-31
reva admin invoke host:9144/publicshareprovider create path=/eos/project/c/cernbox/data initiator=admin1 permissions=viewer expires_at=2026-12-31 name=handout
reva admin invoke publicshareprovider list expiring_from=2026-10-20 expiring_to=2026-10-27
```

Only the `sql` driver implements the manager surface these need; with any
other driver the invocations are listed but answer that the driver does not
support them.

## Projects

The `spacesregistry` exposes the projects catalogue to operators; the CS3
surface only ever lists the caller's own projects. Catalog (`reva admin
invocations spacesregistry`):

```
list          [status=] [owner=] [created_after=] [created_before=] [include_archived=]
get           name=
set_status    name= status=creating|active|archiving|archived
set_owner     name= owner=
create        name= owner= [path=] [storage_id=] [space_id=] [status=] [readers= writers= admins= resource_admins=] [description=] [service_account=] [snow_ticket=] [comments=] [capabilities=]   (dangerous)
delete        name= [hard=]                                                                                                                                                                    (dangerous)
flush_cache
```

`create` records a project the storage already serves (provisioning the space
on EOS is a separate workflow): the path, the space id and the access groups
are derived from the name, and the storage id is copied from the projects
already under the same initial unless given. `delete` is soft by default,
which keeps the space id taken; `hard=true` removes the row.

Every gateway process caches the project list for `cache_ttl` seconds, and a
mutation only invalidates the cache of the instance it ran on. Address one node
for the mutation, then fan `flush_cache` out by service name:

```
reva admin invoke host:9142/spacesregistry set_status name=cernbox status=archiving
reva admin invoke spacesregistry flush_cache
```

Only the `sql` driver implements the catalogue surface these need.

## Quotas

The `storageprovider` reads and sets the quotas of the spaces it serves, through
the optional `storage.QuotaAdmin` interface of its driver (the EOS driver
implements it). A provider only knows its own mount, so the caller addresses the
instance serving the path; each instance advertises `mount_id` and `mount_path`
in its registry metadata (`reva admin services -v storageprovider`) and answers
the `mount` invocation. Catalog:

```
mount
quota_get   path= [style=gid99|uid] [user=]
quota_set   path= max_bytes= max_inodes= [style=gid99|uid] [user=]
```

`style=gid99` (the default) is the project quota set on the path itself;
`style=uid` is the quota of `user` on the path, or on the storage's quota node
when the path is the mount. Bytes and inodes are what EOS stores: a tool that
shows logical volumes applies the replica factor itself. A path served by
another instance fails loudly rather than silently touching the wrong storage.

```
reva admin invoke host:14503/storageprovider quota_get path=/eos/project/c/cernbox
reva admin invoke host:14503/storageprovider quota_set path=/eos/project/c/cernbox max_bytes=4000000000000 max_inodes=5000000
```

## Routing

The `storageregistry` lists its providers and resolves paths on any driver, and
administers the routing table of the `dynamic` driver, which every instance
reads once at start-up into an in-memory tree:

```
providers
resolve        path=
routes
route_add      path= mount_id=      (one instance, then route_reload the others)
route_remove   path=                (one instance, then route_reload the others)
route_reload
```

`route_add` and `route_remove` change the table and reload the instance they
ran on; `route_reload` is idempotent and safe to fan out by service name, so
the usual sequence is one node for the change, the service for the reload:

```
reva admin invoke host:9142/storageregistry route_add path=/eos/user/z mount_id=eoshome-z
reva admin invoke storageregistry route_reload
```

A route to a mount that is not in the registry's `rules` is refused, as it
would be skipped at load time anyway.

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
