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
Admin API service is loaded. It hosts only `Invoke` and `ListInvocations`. It is
*not* registered as a service of its own — instead every real service node
advertises the channel's address in its `control` registry metadata, and the
address is shared by all services in the process.

The control server routes by **node id** (`host:port/service`): the runtime
registers each loaded service as an *instance* under its node id, carrying the
service's redacted config and its optional `Invokable`. Routing by id (not by
service name) means a process can run several instances of the same service and
each is still addressed precisely.

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
routing target. Invocations are **synchronous**: every addressed instance runs
and reports before the call returns.

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
  unauthenticated call to the control port is rejected.
- `Impersonate` mints an ordinary **user-scoped** token for a target user (never
  admin+user), so downstream services apply their normal user checks. It is
  audited and needs the machine-auth api key configured.

Invocations are classified `readonly | mutating | dangerous`; the CLI prompts
before a `dangerous` one. Every invocation and action emits an audit event via
[`pkg/admin`](../../../../pkg/admin) (`audit=true` on the context logger).

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
```

A process that hosts an invokable service but not the `admin` service still
stands up its control channel (so a remote admin can reach it); it just reads the
same `[grpc] control_address` key.

### Using the CLI

```
reva -insecure -host <gateway> login -username <u> -password <p> basic
reva admin elevate -admin-host <admin:port>     # step up, stores a short-TTL admin token
reva admin services [-v] [-o wide|json] [service]
reva admin config   <service|node-id> [-o toml|json]
reva admin invocations <service|node-id>
reva admin invoke   <service|node-id> <invocation> [key=val ...]
reva admin impersonate <user>
```

## Adding an invocation to a service (for developers)

A service exposes admin operations by building an [`invoke.Set`](../../../../pkg/invoke/set.go)
— you declare each method once and the framework does the name→handler routing,
builds the catalog, and validates required arguments. There is no `Invoke`
switch to maintain. Every service also gets the built-in invocations (`config`,
`logs`) for free, so what you add is *on top of* those. (Built-ins live in
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
