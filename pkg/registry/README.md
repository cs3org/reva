# Service registry

For most of its life reva knew where its peers were because somebody had written
it down. Every service that needed the gateway carried a `gatewaysvc` address in
its configuration, a storage provider was told the URL of its data server, and
moving a service to another host meant a sweep through the configuration files of
everything that talked to it.

The registry replaces that arrangement. Each `revad` announces the services it
has loaded, and a caller that needs a peer asks for it by kind — "give me the
gateway" — rather than by address. The lookup, the choice between several
candidates and the dialing all happen underneath the call, so the calling code
never sees an address at all.

Two packages share the work. `pkg/registry` is the registry proper: it knows
what services exist, which nodes are serving them and whether those nodes are
still alive. `pkg/service` sits on top and is what most code actually touches:
it turns the name of a peer into a ready-to-use CS3 client or an HTTP endpoint.

## How it is put together

### The registry

At its core a registry is a map from a service name to the set of nodes serving
it, with a way to observe changes:

```go
type Registry interface {
	Add(Service) error
	GetService(string) (Service, error)
	ListServices() ([]Service, error)
	Remove(Service) error
	Watch() (<-chan Event, error)
}
```

The vocabulary underneath is deliberately small. A `Service` is a name and a set
of nodes, and a `Node` is an ID, a `host:port` address and a bag of metadata.
Everything else a caller might want to know — which transport the node speaks,
whether it is healthy, which storage it is bound to — travels in that metadata
rather than in the type system.

All of the interesting behaviour lives in `BaseRegistry`. It owns an in-process
cache keyed by service name and node ID, and it implements resolution, the watch
loop and the liveness state machine. A backend contributes only a `Driver`,
which is a much smaller obligation:

```go
type Driver interface {
	Add(service string, n Node) error
	Remove(service, nodeID string) error
	Watch() (<-chan Event, error)
	Close()
}
```

Writes go to the cache first and are then pushed through to the driver, which
means a process can always see its own registrations even when the shared store
is having a bad day. In the other direction, `BaseRegistry` consumes the events
the driver streams and applies remote additions and removals to the same cache,
re-establishing the stream if it closes. The practical consequence is worth
stating plainly: every lookup is answered out of local memory, so resolving a
peer never costs a network round trip.

Two drivers ship with reva, and both become available by importing
`pkg/registry/loader`.

The **`memory`** driver is the default and is, strictly speaking, a driver that
does nothing: there is no shared store behind it, so the registry ends up
containing exactly the services of the current process and nothing more. That is
precisely what a deployment running everything inside one `revad` wants, and it
costs nothing to operate.

The **`nats`** driver gives a fleet a common view of itself, backed by a NATS
JetStream key/value bucket. Each node is stored as a single key,
`<service>.<node id>` with the characters NATS dislikes mapped away, holding a
small JSON entry. When a process connects it watches the whole bucket, which
replays the keys already there — that replay is how a freshly started process
hydrates its cache — and then keeps streaming changes. An unreachable NATS
server is not treated as a fatal condition: writes are queued in memory and
flushed once the connection comes back. Entries carry a TTL as well, so a
process that dies without deregistering fades out of the shared view instead of
lingering forever.

### Knowing which nodes are alive

Registration alone would leave the registry full of nodes that used to exist, so
each node carries a `state` and a `last_seen` timestamp in its metadata, and a
sweep loop moves nodes between states according to how long they have been
quiet:

```
ready ──degraded_after──► degraded ──offline_after──► offline ──reap_after──► removed
  ▲                                                       │
  └──────────────── fresh heartbeat ──────────────────────┘
```

The transitions are reversible in the obvious way: a node that starts beating
again climbs straight back to `ready`. The sweep runs at a third of the shortest
configured threshold, and if no threshold is configured at all it does not run,
which is the sensible behaviour for a single-process deployment where liveness
tracking would only be bookkeeping. A node explicitly marked `draining` is left
where it is, since something has deliberately taken it out of rotation.
Selection, described further down, prefers `ready` nodes, will settle for
`degraded` ones if that is all there is, and never returns anything `offline` or
`draining`.

### Services register themselves

None of this requires code in the services. `cmd/revad/runtime` builds one
registry per reva instance from the `[shared.registry]` block, installs the
process-wide resolver *before* it constructs any service — so a service that
reaches for a peer early still finds a resolver waiting — and then, once the
listeners are bound and the addresses are therefore known, adds one node per
loaded service.

The address it advertises is the listener's `host:port`, except that a wildcard
bind host such as `::` or `0.0.0.0` is swapped for the hostname, since nobody
can dial a wildcard. The node ID is `<host:port>/<service>`, which stays
stable when the same process restarts on the same port. The metadata starts with
what the framework knows — `transport` (`grpc` or `http`), `host`, `pid`,
`state` and `last_seen`, plus `scheme` and `prefix` for HTTP services — and is
then extended with whatever the service itself wants to advertise, as described
under [Advertising extra metadata](#advertising-extra-metadata).

From then on a heartbeat goroutine re-adds the nodes every
`heartbeat_interval`, which is what keeps `last_seen` fresh, and a graceful
shutdown removes them so peers stop trying immediately rather than after a
timeout.

One detail that is easy to trip over: the name a service registers under is its
configuration key. `[grpc.services.gateway]` becomes `gateway`, and
`[http.services.dataprovider]` becomes `dataprovider`. There is no separate
naming scheme to keep in sync.

## Working with it

### Resolving a peer

Because the runtime installs a process-wide resolver, any piece of code can
resolve a peer without having been handed one. The package-level accessors in
`pkg/service` are the normal way in:

```go
import "github.com/cs3org/reva/v3/pkg/service"

client, err := service.Gateway(ctx)
if err != nil {
	return err
}
res, err := client.Stat(ctx, req)
```

There is one accessor per CS3 peer — `Gateway`, `StorageProvider`,
`StorageRegistry`, `Spaces`, `AuthProvider`, `AuthRegistry`, `AppAuthProvider`,
`UserProvider`, `GroupProvider`, `UserShareProvider`, `PublicShareProvider`,
`OCMShareProvider`, `OCMInviteManager`, `OCMProviderAuthorizer`, `OCMIncoming`,
`Preferences`, `Permissions`, `AppRegistry`, `AppProvider`, `DataTx` and
`Labels`. Each one looks its peer up under the registry name held in the
corresponding `Name*` constant in `pkg/service/clients.go`, picks a node, dials
it and hands back the typed CS3 client. Connections are cached per address and
shared, so calling an accessor on a hot path is cheap and there is no reason to
hold on to the client yourself.

There is one habit worth acquiring: **resolve at call time, not at construction
time.** A peer that lives in another process may well register after this one
has started, and a `New()` that fails because the gateway is not up yet turns a
condition that would have resolved itself in a second into a startup failure.
Resolving inside the method that needs the peer costs a map lookup and avoids
the whole class of problem.

Sometimes a caller already has an address, typically one that a registry RPC
handed back — a provider address from the storage registry, say. For those cases
the `*At` getters dial an explicit address and cache the connection the same
way: `StorageProviderAt`, `GatewayAt`, `AuthProviderAt`, `AppProviderAt`,
`UserProviderAt` and `StorageRegistryAt`.

### Resolving an HTTP endpoint

Some services are not consumed as CS3 clients but as URLs — the data gateway and
the data provider are the two that matter — so for those the resolver returns an
endpoint instead:

```go
ep, err := service.HTTPEndpoint(ctx, service.ByName("datagateway"))
if err != nil {
	return "", err
}
return ep.URL(), nil
```

`ByName` is mandatory, since there is nothing sensible to return without it, and
`ByMetadata(key, value)` narrows the candidates further and may be repeated.
That filtering is what lets a storage provider find the data provider that
serves its own storage rather than just any data provider in the fleet:

```go
ep, err := service.HTTPEndpoint(ctx,
	service.ByName("dataprovider"),
	service.ByMetadata("mount_id", s.mountID),
)
```

An `Endpoint` gives access to `Address`, `Host`, `Port`, `Scheme`, `Prefix`,
`Meta`, `Metadata` and the underlying `Node`, but most callers only want
`URL()`, which returns the advertised `public_url` when the service configured
one and falls back to `scheme://address/prefix` otherwise. When every match is
wanted rather than a single one, `HTTPEndpoints` returns the whole set.

### Advertising extra metadata

A service that needs to be found by something other than its name can say so by
implementing `MetadataProvider`. The runtime merges what it returns over the
framework-derived keys at registration time, so a service can add its own facts
and, if it really needs to, override the defaults:

```go
// RegistryMetadata advertises the mount affinity and the externally reachable
// URL so a storage provider can discover this data provider.
func (s *svc) RegistryMetadata() map[string]string {
	m := map[string]string{}
	if s.conf.MountID != "" {
		m[registry.MetaMountID] = s.conf.MountID
	}
	if s.conf.PublicURL != "" {
		m[registry.MetaPublicURL] = s.conf.PublicURL
	}
	return m
}
```

This is the other half of the `ByMetadata` filter above: the data provider
publishes its `mount_id`, and the storage provider bound to the same storage
selects on it. The well-known keys are constants in `pkg/registry` —
`MetaState`, `MetaLastSeen`, `MetaScheme`, `MetaPrefix`, `MetaPublicURL` and
`MetaMountID` — and using the constants rather than the string literals is worth
the import.

### Choosing between candidates

When several nodes serve the same name, the resolver picks one through a
`Selector`. `FirstSelector` is the default and is deterministic, which makes
debugging pleasant; `RoundRobinSelector` and `RandomSelector` spread the load
instead. All three apply the same eligibility rule before choosing: `ready`
nodes if there are any, `degraded` ones if that is all that is left, and never
anything `offline` or `draining`.

Callers can also push information back. `service.Degrade(name, address)` marks a
node degraded after a failed dial or RPC, so subsequent lookups step over it
until a heartbeat proves it healthy again. It is a hint rather than a guarantee
— it never errors and never blocks — but it shortens the window during which
everyone keeps dialing something that has already stopped answering.

### Testing against it

Two hooks make code that resolves peers testable without standing up the peers.

`service.SetGlobal(c Clients)` installs a resolver process-wide. The first
non-nil resolver wins, which is what makes the production path safe, but it also
means a test package should install its own exactly once — a `sync.Once` is the
usual arrangement — and then mutate it between tests. The convenient pattern is
a struct that embeds `service.Clients` and overrides only the accessors the test
cares about; several packages carry a small `resolver_test.go` doing precisely
that, and copying one of them is the fastest way to start.

`service.SetClientAt(address, client)` covers the other case, where the code
under test uses one of the `*At` getters. A fake peer has no listener to dial,
but the code still reaches for it by whatever address a registry lookup handed
back, so pinning a client to that address lets the call through. Passing `nil`
removes the pin.

For a test that exercises a whole reva instance, `runtime.WithRegistry(r)`
replaces the registry that would otherwise be built from configuration.

### Adding a backend

Writing a new backend means implementing `Driver`, registering a constructor
from the package's `init()` and adding a blank import to `pkg/registry/loader`
so the registration actually happens:

```go
func init() {
	registry.Register("mydriver", func(m map[string]any) (registry.Driver, error) {
		return New(m)
	})
}
```

The constructor receives the `[shared.registry.drivers.<name>]` block as a plain
map. Everything above the driver — the cache, resolution, the liveness state
machine, the watch loop and its reconnection — comes from `BaseRegistry`, so the
driver's whole job is to propagate writes to wherever they belong and to stream
back the changes it learns about. The `nats` driver is around three hundred
lines including its offline queueing, which is a fair estimate of the effort
involved.

## Configuration

All of it lives in the `[shared.registry]` block, which is set once per `revad`
process and applies to every service that process loads.

```toml
[shared.registry]
driver = "memory"          # "memory" (default) | "nats"

heartbeat_interval = "5s"  # how often this process refreshes its nodes
degraded_after     = "15s" # quiet for longer than this -> degraded
offline_after      = "30s" # quiet for longer than this -> offline
reap_after         = "5m"  # offline for longer than this -> removed
```

The values above are the defaults. A threshold left empty or set to zero
disables that transition, which is a reasonable thing to do for the ones you do
not want. The relationship that actually matters is between the heartbeat and
the thresholds: `heartbeat_interval` has to stay comfortably below
`degraded_after`, because a node is judged by how long it has been silent and a
heartbeat that only just beats the deadline will make healthy nodes flap into
`degraded` whenever a beat is late.

### memory

The default, and the whole of its configuration:

```toml
[shared.registry]
driver = "memory"
```

No external dependency and no shared view — a process sees only the services it
loaded itself. That is the right choice whenever a deployment runs its services
inside a single `revad`, and it is what the tests use unless they are
specifically exercising the shared path.

### nats

The shared view, backed by a JetStream KV bucket:

```toml
[shared.registry]
driver = "nats"

[shared.registry.drivers.nats]
address = "nats://nats.example.org:4222"  # required
token   = "s3cr3t"                        # optional
bucket  = "reva_registry"                 # default
ttl     = "30s"                           # default; per-entry TTL in the bucket
```

Only `address` has to be given; the bucket is created if it is not already
there. The `ttl` is the backstop for a process that dies without deregistering,
which means it has to be comfortably larger than `heartbeat_interval`: with the
defaults a node rewrites its entry every five seconds and the entry would expire
thirty seconds after the last write, so an entry only actually expires when the
process behind it has genuinely gone. A NATS server that is down when `revad`
starts does not prevent it from starting — the writes queue up and flush when
the connection succeeds.

Every process that should see the others must point at the same server and the
same bucket. Conversely, two independent deployments sharing a NATS server need
different `bucket` values, or they will discover each other's services and try
to use them.

### What this replaces

The point of all this is that peer addresses are no longer something you
configure. A service that used to be told where its peers lived now resolves
them, and what remains in configuration is the identity a service advertises
about itself — a data provider's `mount_id` and `public_url`, for instance, are
exactly what lets the storage provider in front of it find the right one.
