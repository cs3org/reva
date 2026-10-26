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
  `reva service list -v --wide` or `-o json`) targets **exactly that** instance;
- a **metadata selector** (`storageprovider[mount_id=eoshome-i01,mount_path=/eos/user]`)
  targets the live instances of the service whose registry metadata has all the
  pairs. A shared invocation goes to one of them. A storage provider advertises
  `mount_id` and `mount_path`, a data provider `mount_id`; and
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
rides the same selector + control fan-out as any invocation. `reva logs`
drives it:

```
reva logs userprovider              # recent lines from every userprovider, merged
reva logs 10.0.0.4:19003/userprovider   # one instance
reva logs 10.0.0.4:19003            # every service at that address
reva logs 10.0.0.4                  # every service on that machine
reva logs userprovider -f           # follow (InvokeStream) until Ctrl-C
reva logs userprovider -level warn -since 5m -grep timeout -n 500
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

`reva service drain <selector>` takes the matched instances **out of
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
visible in `reva service list` (the STATE column).

## Request activity (is it safe to restart?)

`reva service activity <selector>` reports, per instance, how many
requests it is **currently serving** (`in-flight`), how many it has served
(`total`), and how long it has been **idle**. This is the companion to `drain`:
drain stops new traffic, and activity tells you when the in-flight traffic has
finished, so you know when a node has quiesced and is safe to restart.

```
reva service drain 10.0.0.4:19003                        # out of rotation
reva service activity --wait --idle 5s 10.0.0.4:19003    # block until quiesced
# ... restart the process ...
reva service enable 10.0.0.4:19003
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
  user scope is guarded by `isAdminResource`. The machine scope declines admin
  requests in the same way.
- The decision is on the **gRPC method**, not on the request message: every
  method of the `reva.admin.v1beta1` and `reva.control.v1beta1` packages needs
  the admin scope, except `RequestAdmin` and `CheckAdmin`. A typed admin RPC can
  take a CS3 request message, which the user and lightweight scopes accept, so
  the message cannot decide. A new admin service needs no change to the scopes.

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
  audited and needs the machine-auth api key configured. Rules:
  - The target gets the scopes of its own login: the owner scope for a primary,
    secondary or service account, the lightweight scope for a lightweight
    account. Other account types are refused.
  - The token carries the impersonator in an `actor` scope, which authorizes
    nothing: the user id of the caller, or `socket:<uid>` on the local-root
    socket. A scope, not a JWT claim, because the auth cache and the token
    re-mints (for example the machine scope) keep only the user and the scopes.
    The request logs, and the audit events, of every call made with the token
    show `act=<actor>`.
  - `RequestAdmin` and `Impersonate` refuse a token with an actor, and
    `CheckAdmin` reports it as not admin. Else an admin who impersonates
    another admin could step up as that admin, and the audit would lose who did
    it.
  - A member of `admin_group` or a service account is refused unless the
    request sets `allow_privileged`. That is audited on its own line
    (`impersonate_privileged`).
  - `ttl_seconds` asks for a shorter lifetime, capped by the lifetime the
    server would give. The response has `expires_at`. A target that does not
    exist answers `NotFound`.

Invocations are classified `readonly | mutating | dangerous`; the CLI prompts
before a `dangerous` one. Every invocation and action emits an audit event via
[`pkg/admin`](../../../../pkg/admin) (`audit=true` on the context logger).

A client tells why it makes an admin call in the gRPC metadata header
`x-reva-admin-reason`. `admin.Audit` reads the header of the incoming call and
writes it as `reason` on the audit line. The text is from the client: the
server removes control and format characters and cuts it to 512 bytes. The
admin service sends the header on to the owning service through the control
channel, so an audit line that an invocation handler writes has the reason
too. A typed admin RPC that calls a service must do the same with
`admin.ForwardReason(ctx)`. `Impersonate` also takes a `reason` field. When
the header is not set, the field is the `reason`. When both are set and they
are different, the field is in `request_reason`.

### Local root

The step-up above answers "who are you?" with a token, which is right for remote
access. For an operator working **on the box**, the Admin API can also be served
on a **Unix socket** where the transport itself is the authentication — like
`sudo`, `docker.sock`, or PostgreSQL `peer`. There is no token and no login: a
permitted local user runs a fleet command (`reva service list`, …) and it just works.

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
root). Every local-root request is audited with the OS user, the uid and the pid
of the peer. Because the OS user is usually `root`, a client can send the
`SUDO_USER` of its environment in the header `x-reva-sudo-user`, and the audit
line has it as `sudo_user`. The client sets this value, so do not trust it: it
only helps to find the person.

Local root is full admin. A member of `socket_group` can also impersonate any
user (`Impersonate`) without a Kerberos ticket or a password. Give the group
only to the persons who may do that.

It is **on by default** (Linux): the server binds a well-known path,
`/run/reva/admin.sock`, or `$XDG_RUNTIME_DIR/reva/admin.sock` when the former is
not writable (rootless). The CLI probes the same list, so the fleet commands
just work locally with no flag and no login. Because the default is `0600`, that
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
# Lifetime of a token minted by Impersonate. Unset, it is the token manager's
# own (a day for jwt), as for signing in: an impersonated session is used like
# any other, long transfers included.
# impersonation_ttl = "12h"

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

The in-memory log buffer that backs `reva logs` is sized in the `[log]`
section:

```toml
[log]
tail = 2000   # recent lines kept in memory for `reva logs` (0 disables it)
```

### Using the CLI

The fleet commands are at the top level of the `reva` CLI, marked "(admin)" in
its help. A command that needs admin rights gets them itself: it uses the local
socket if one answers, else an admin token from `reva elevate`, else it steps up
with the login token (`RequestAdmin`) and keeps that token in memory for the one
command. `--no-elevate` turns the automatic step-up off. `--reason` goes with
every admin call to the audit log, and `--yes` skips the confirmations.

```
reva --insecure --gateway <gateway> login --user <u>
reva elevate --admin-host <admin:port>      # optional: stores a short-TTL admin token for this login
reva whoami                                 # admin: socket | yes, 12m left | no
reva service list [-v] [--wide] [--state S] [service]
reva service config <service|node-id> [-o json] [--diff]
reva service drain  <selector> [-y]         # take instances out of rotation
reva service enable <selector>              # return them to rotation
reva service activity <selector> [--methods] [--wait [--idle D] [--wait-timeout D]]  # in-flight/idle
reva invocation list <service|node-id>
reva invocation get  <service|node-id> <invocation>
reva invoke [--stream] <selector> <invocation> [key=val ...]
reva logs <selector> [-f] [-n N] [--level L] [--since D] [--grep P] [-o json]
reva logs level <selector> [trace|debug|info|warn|error|reset]   # report/set runtime level
reva trace <traceid> | --user <username>    # one request/user across the fleet
reva stack <selector> [--grep P]            # goroutine dumps, e.g. of a hung process
reva job list | active | runs | status <id> | logs <id> | run <job> [k=v] | trigger <job> | cancel <id> | stop <job> | forget <job>
reva impersonate <user> [--reason R] [--ttl D] [--allow-privileged [-y]]
reva ls /home --as <user> --reason R        # any user command, as another user

# Local root: on the box, no login, no elevate, no flag: the CLI finds the socket.
# Only if it is absent (or denies you) does it need the network host.
reva service list
```

### Over HTTP

Clients that only speak the public HTTPS surface (the web UI, the `cernbox`
CLI) reach the user-facing part of the Admin API through the `admin` HTTP
service. It sits behind the normal HTTP auth middleware, so the caller
authenticates as any other request does, and resolves the Admin API through the
service registry.

```toml
[http.services.admin]
# prefix = "admin"   # the default
```

| Request | Answer |
|---|---|
| `GET /admin/status` | `{"admin": true\|false}`. Backed by `CheckAdmin`, which mints nothing and is not audited. |
| `POST /admin/impersonate` `{"user": "...", "reason": "...", "allow_privileged": false, "ttl_seconds": 0}` (all but user optional) | `{"token": "...", "expires_at": ...}`, a user token for the target. |

`impersonate` runs `RequestAdmin` and `Impersonate` back to back on the
server, so the admin token never reaches the client: the client holds at most
a user token. Both steps are audited as usual, with `reason` when one is given. Errors
are `{"message": "..."}` with 401 (caller not identified), 403 (not an admin, a
refused target, or an impersonated caller), 404 (no such user), 501
(impersonation not configured) or 503 (no Admin API).

## Jobs

`reva job` drives the background jobs runner ([`pkg/rjobs`](../../../../pkg/rjobs)),
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

## Typed services

The Admin API has two layers:

- **Invocations** (`Invoke`, `InvokeStream`): any operation that a service
  exposes in its `invoke.Set`, reached by a selector. The arguments are strings
  and the result is JSON. They need no proto change, so a service, a driver or a
  plugin can add one.
- **Typed services**, in their own proto files of the `reva.admin.v1beta1`
  package: `AdminAPI` (`admin.proto`), `SpacesAdmin` (`spaces.proto`),
  `SharesAdmin` (`shares.proto`) and `StorageAdmin` (`storage.proto`), with the
  shared messages in `types.proto`. A typed RPC is a facade over the invocations
  of the owning service, like the jobs RPCs: the admin service resolves the
  instances, runs the invocation over the control channel and decodes the result
  into its message. The helpers are in [`facade.go`](facade.go): `invokeOne` (one
  picked instance), `invokeAll` (every instance), `relayStream` (an
  `InvokeStream` relayed into a typed server stream) and `decode` (protojson into
  an `adminpb` message).

An operation gets a typed RPC when it is generic and a user cannot do it on the
user plane: a fleet-wide view (any owner, orphans, every space), an operation on
someone else's resource (the owner fallback of the CLI), a field that CS3 does
not carry, an operation that no owner can do (a transfer, a database-only
removal), operator-only state (routes, space quotas, the space catalogue), or
an orchestration over many instances (a reload or a cache flush on each one).
Everything else stays an invocation: deployment-specific operations (for
example CERN's), and the built-ins.

Rules for a typed RPC:

- A resource message wraps the CS3 message of the same resource, and adds only
  what CS3 cannot carry, with an optional `map<string, string> extra` for the
  attributes of a driver or a deployment. It never carries a secret.
- A list is a server stream.
- The invocation returns the protojson of the message: `invoke.ProtoResult`.
- An invocation error has a gRPC code: return `invoke.Error{Code, Message}` or a
  `status.Error`. The control channel sends the code (`code` in the control and
  admin results), and the typed RPC answers with it. NotFound must mean that the
  resource does not exist: the CLI reads it so. A service that is not live is
  Unavailable.
- Register the service in `register` ([`services.go`](services.go)), which both
  the gRPC listener and the local socket use.

The protos import CS3 messages. `make protobuf` needs a checkout of
[cs3apis](https://github.com/cs3org/cs3apis) in `CS3APIS_DIR` (default
`../cs3apis`).

## Shares

The `usershareprovider` exposes the operations an operator needs on the shares
of any user. All of them read or change the share database, so they are
shared: one instance of the service runs each call.

```
list         owner= initiator= sharee= id= space= path= [as=] [include_orphans=] [include_expired=] [resolve_paths=]   (stream)
get          id=
update       id= [role=] [expires_at=<date|RFC 3339|none>] [fallback=]
remove       id= [db_only=] [fallback=]
sync_expiry  id=          remove the storage grant of an expired share now, keep the record
transfer     id= initiator=   the new initiator must be allowed to add a grant on the resource
move         id= path= [as=] [apply_grant=]   (dangerous) point the share at a new path; its owner becomes the owner of the share
mark_orphan  id=
```

`list` needs at least one of `owner`, `initiator`, `sharee`, `id`, `space` or
`path`: the share table is too large to read without a filter. Each result,
streamed or not, is the JSON form of the `AdminShare` message of the typed
Admin API: `share` (the CS3 share, in protojson), `orphan`, `path` and
`extra` (`initial_path`, `orphaned_at`, `expired_at`).

An invocation runs with the admin token of the operator, and no CS3 endpoint
accepts that token. So a call that needs the storage (a stat, a grant, the
gateway's `UpdateShare` and `RemoveShare`) is made as a user, with
[`admin.ActAs`](../../../../pkg/admin/identity.go): a short-lived token with
the owner scope, signed with the shared JWT secret, that never leaves the
server. `update` and `remove` act as the share's initiator, then as its owner
if the initiator is refused or does not exist anymore, so that the gateway
applies its usual checks and changes the storage grant. A path is resolved as
the user in `as` (or `owner`, `initiator` for `list`).

Every mutation logs an audit event `action=usershareprovider.<name>`, with
the operator as actor, `target=share:<id>`, the user acted as, and
`fallback=true` when the Admin API calls it for a user command that fell back
to admin rights.

Only the `sql` driver has what these invocations need. With another driver
they are listed, and answer that the driver does not support them.

```
reva admin invoke usershareprovider list owner=einstein resolve_paths=true
reva admin invoke usershareprovider update id=4711 expires_at=2026-12-31
reva admin invoke usershareprovider remove id=4711 db_only=true
```

## Public links

The `publicshareprovider` has the same operations on the public links of any
user, all shared too. A link is given by `id=` or by `token=`.

```
expiring     from= to= [after_expiration=] [after_id=] [limit=]   the links that expire in a window, by page
list         id= token= owner= initiator= space= [include_orphans=] [include_expired=] [resolve_paths=]   (stream)
get          id= | token=
update       id= | token= [role=viewer|uploader|editor] [expires_at=<date|RFC 3339|none>] [password=] [fallback=]
remove       id= | token= [db_only=] [fallback=]
transfer     id= | token= initiator=   the new initiator must be allowed to add a grant on the resource
move         id= | token= path= [as=]   (dangerous) point the link at a new path; its owner becomes the owner of the link
mark_orphan  id= | token=
```

Each result of `list`, `get` and the mutations is the JSON form of the
`AdminPublicShare` message: `public_share` (the CS3 link, token included),
`orphan`, `path`, `password_protected` and `extra` (`initial_path`,
`orphaned_at`, and `url` when `public_link_base_url` is set). The password and
its hash are never in a result. An empty `password=` removes the password.

`update` and `remove` act as the initiator of the link, then as its owner,
through the gateway, like the share invocations. `remove` of an orphan, or with
`db_only`, deletes the record only. With `pub_rw_link_max_expiration` set (the
option of the OCS and Graph services, in seconds after the end of today), a
change of the role or of the expiration of an editor link must give it an
expiration within that limit. The audit events are
`action=publicshareprovider.<name>`, with `target=link:<id>`.

```toml
[grpc.services.publicshareprovider]
driver = "sql"
pub_rw_link_max_expiration = 2592000              # 0 or unset: no rule
public_link_base_url = "https://example.org/s/"   # unset: no url in the results
```

### SharesAdmin

`SharesAdmin` ([`shares.proto`](../../../../pkg/admin/proto/shares.proto)) is
the typed facade over the share and link invocations. Each RPC runs one
invocation, except:

- `ListShares` and `ListPublicLinks` resolve a `SpaceRef` first: by id, or by
  name through the `spacesregistry` `get` invocation. A path is refused, because
  only a user can resolve it. The `ONLY` selections of `orphans` and `expired`
  are applied to what the provider streams.
- `MoveShares` lists the shares and the links of `owner` and/or `space` with
  their paths, keeps those under `from_path` (by the current path, or by the
  path at creation when the resource cannot be found), and runs `move` on each
  one. A failure does not stop the others; the stream ends with the status of
  the first failure. `dry_run` only lists. Orphans are not moved.

The mutations write an audit line in the admin service too (for example
`action=update_share target=share:4711 fallback=true`). A link is named by its
id in the audit, never by its token, and a password change is only
`password=set` or `password=removed`.

## Projects

The `spacesregistry` exposes the catalogue of project spaces to operators. The
CS3 calls show a project only to its members; these show every project.

```
list         [type=project] [status=] [owner=] [member=] [created_after=] [created_before=] [include_archived=]   (stream)
get          id= | name=
update       id= | name=  [description=] [readme=<path>] [thumbnail=<path>] [fallback=]
set_status   id= | name=  status=creating|active|archiving|archived
set_owner    id= | name=  owner=
create       name= owner= [path=] [storage_id=] [space_id=] [readers=] [writers=] [admins=] [description=] [status=]   (dangerous)
delete       id= | name=  [hard=]   (dangerous)
check        [status=] [owner=]   (stream)
flush_cache
```

A project is selected by its space id (`<storage id>$<space id>` or the space
id alone) or by its name. A name selects the project that is not archived, or
else the one archived last. Each result is the JSON form of the `AdminSpace`
message of the typed Admin API: `space` (the CS3 storage space, in protojson),
`status` and `extra`, with the columns that CS3 cannot carry. `member` matches
the owner and the role groups against the groups that the gateway gives for
the user, and sets the permissions of that user in `space.rootInfo`.

`create` records a space that the storage already serves. A value that is not
given comes from a template of the `sql` driver, with the fields `.Name` and
`.Initial` (the first letter of the name) and the sprig functions. Without a
template, the value is required:

```toml
[grpc.services.spacesregistry.drivers.sql]
path_template = "/eos/project/{{ .Initial }}/{{ .Name }}"
storage_id_template = "newproject-{{ .Initial }}"
readers_group_template = "cernbox-project-{{ .Name }}-readers"
writers_group_template = "cernbox-project-{{ .Name }}-writers"
admins_group_template = "cernbox-project-{{ .Name }}-admins"
```

`update` is the server side of the owner fallback: it makes the change as the
owner of the project, through the same check as a user call, which lets only
the owner and the admins group change a project. `check` reports, for each
project with a problem, findings of these kinds: `owner_missing`,
`owner_expiring`, `owner_blocked`, `path_missing` (the path does not stat as the
owner), `group_unset`, `group_missing` and `no_members`, or `*_error` when a
call failed.

Each instance caches the project list, and a mutation drops only the cache of
the instance that ran it. `flush_cache` is not shared, so it runs on every
instance:

```
reva admin invoke spacesregistry set_status name=cernbox status=archiving
reva admin invoke spacesregistry flush_cache
```

Every mutation logs an audit event `action=spacesregistry.<name>` with
`target=project:<name or id>`. Only the `sql` driver has what these
invocations need.

The typed `SpacesAdmin` service ([`spaces.go`](spaces.go)) calls these
invocations. After a change of the catalogue, it runs `flush_cache` on every
instance, and logs `action=spaces.<invocation>` with `target=space:<ref>`. If
the flush fails on an instance, the change stays done: the audit line has
`flush_cache=<error>`, and that instance shows the old list until its cache
expires. A `SpaceRef` by path selects the space with the deepest root that holds
the path. The catalogue has no index on the path, so this reads the whole list.
A space that is not in the catalogue, a personal space for example, is NotFound.

## Routing

The `dynamic` storage registry reads its routing table (the `routing` SQL
table) into memory. Each instance (usually one per gateway) has its own copy.
The `storageregistry` invocations change the table and load it again without a
restart:

```
routes                              list the table
resolve        path=                the mounts that serve a path, on each instance
route_add      path= mount_id=      add or change a route (table only)
route_remove   path=                remove a route (table only)
route_reload                        load the table again into memory
```

`route_add` and `route_remove` change only the table, so they run on one
instance (they are shared). They have no effect until you reload. `route_reload`
is not shared: called by service name, it runs on every instance. The flow is
always a change, then a reload:

```
reva admin invoke storageregistry route_add path=/eos/user/z mount_id=eoshome-i05
reva admin invoke storageregistry route_reload
reva admin invoke storageregistry resolve path=/eos/user/z/zed   # one line per instance
```

If a reload fails on an instance, that instance keeps its old routes. Run
`route_reload` again. `route_add` refuses a mount that is not in the `rules` of
the registry, because the reload would skip the route. The `static` driver has
no routing table and answers "not supported".

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
`reva invocation list groupprovider`, its required `group`/`user` args are
checked before the handler runs, and:

```
reva invoke groupprovider is_member group=sailing-lovers user=einstein idp=cernbox
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
