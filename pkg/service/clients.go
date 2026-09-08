// Copyright 2018-2024 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

// Package service hosts the registry-backed peer resolver embedded by every
// reva service across all transports. It is neutral (imports only CS3 client
// types and the registry) to avoid an import cycle.
package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	applicationauth "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authprovider "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	authregistry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	group "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	labels "github.com/cs3org/go-cs3apis/cs3/labels/v1beta1"
	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storageregistry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"

	revtrace "github.com/cs3org/reva/v3/internal/grpc/interceptors/trace"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/logger"
	"github.com/cs3org/reva/v3/pkg/registry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// maxCallRecvMsgSize is the default maximum gRPC receive message size (in bytes).
const maxCallRecvMsgSize = 10240000

// A peer may be starting, restarting or waiting for its registration to
// propagate, so a lookup retries before it fails the call. A peer that stays
// unresolvable is different: this instance cannot route at all, so it exits
// instead of serving errors indefinitely. Both thresholds must be crossed, so
// neither a burst of requests during a peer restart nor a slow trickle of them
// over a long quiet period takes the process down.
const (
	resolveAttempts   = 3
	resolveRetryWait  = 250 * time.Millisecond
	unresolvableCalls = 5
	unresolvableFor   = time.Minute
)

// exit ends the process. It logs through zerolog (not raw stderr) so the line
// is structured and lands in the same sink as every other log. It writes
// synchronously so the record survives the immediate os.Exit. It is a variable
// so tests can observe it instead of dying.
var exit = func(reason string) {
	logger.New().Error().Msg(reason)
	os.Exit(1)
}

// Service names processes register under and the resolver looks up.
const (
	NameGateway         = "gateway"
	NameStorageProvider = "storageprovider"
	NameStorageRegistry = "storageregistry"
	NameAuthProvider    = "authprovider"
	NameAuthRegistry    = "authregistry"
	NameAppAuthProvider = "applicationauth"
	NameUserProvider    = "userprovider"
	NameGroupProvider   = "groupprovider"
	NameUserShare       = "usershareprovider"
	NamePublicShare     = "publicshareprovider"
	NameOCMShare        = "ocmshareprovider"
	NameOCMInvite       = "ocminvitemanager"
	NameOCMProvider     = "ocmproviderauthorizer"
	NameOCMIncoming     = "ocmincoming"
	NamePreferences     = "preferences"
	NamePermissions     = "permissions"
	NameAppRegistry     = "appregistry"
	NameAppProvider     = "appprovider"
	NameSpaces          = "spacesregistry"
	NameDataTx          = "datatx"
	NameLabels          = "labels"
	NameAdmin           = "admin"
	// NameControl labels the per-process control channel on its internal
	// server; it is discovered via node metadata, not registered as a service.
	NameControl = "control"
)

// Clients resolves a peer by kind and returns a typed CS3 client; resolution,
// selection and dialing happen below the call.
type Clients interface {
	Gateway(ctx context.Context) (gateway.GatewayAPIClient, error)
	StorageProvider(ctx context.Context) (storageprovider.ProviderAPIClient, error)
	StorageRegistry(ctx context.Context) (storageregistry.RegistryAPIClient, error)
	Spaces(ctx context.Context) (storageprovider.SpacesAPIClient, error)
	AuthProvider(ctx context.Context) (authprovider.ProviderAPIClient, error)
	AuthRegistry(ctx context.Context) (authregistry.RegistryAPIClient, error)
	AppAuthProvider(ctx context.Context) (applicationauth.ApplicationsAPIClient, error)
	UserProvider(ctx context.Context) (user.UserAPIClient, error)
	GroupProvider(ctx context.Context) (group.GroupAPIClient, error)
	UserShareProvider(ctx context.Context) (collaboration.CollaborationAPIClient, error)
	PublicShareProvider(ctx context.Context) (link.LinkAPIClient, error)
	OCMShareProvider(ctx context.Context) (ocm.OcmAPIClient, error)
	OCMInviteManager(ctx context.Context) (invitepb.InviteAPIClient, error)
	OCMProviderAuthorizer(ctx context.Context) (ocmprovider.ProviderAPIClient, error)
	OCMIncoming(ctx context.Context) (ocmincoming.OcmIncomingAPIClient, error)
	Preferences(ctx context.Context) (preferences.PreferencesAPIClient, error)
	Permissions(ctx context.Context) (permissions.PermissionsAPIClient, error)
	AppRegistry(ctx context.Context) (appregistry.RegistryAPIClient, error)
	AppProvider(ctx context.Context) (appprovider.ProviderAPIClient, error)
	DataTx(ctx context.Context) (datatx.TxAPIClient, error)
	Labels(ctx context.Context) (labels.LabelsAPIClient, error)

	// Degrade marks the node at address degraded after a failed dial/RPC.
	Degrade(service, address string)

	// HTTPEndpoint resolves one ready node matching the filters; HTTPEndpoints
	// returns all of them. Used for HTTP services whose URL is needed (data
	// gateway, data provider).
	HTTPEndpoint(ctx context.Context, opts ...EndpointOption) (Endpoint, error)
	HTTPEndpoints(ctx context.Context, opts ...EndpointOption) ([]Endpoint, error)
}

type clients struct {
	registry registry.Registry
	selector Selector

	mu    sync.Mutex
	conns map[string]*grpc.ClientConn

	failMu sync.Mutex
	fails  map[string]*failure
}

// failure is a service's current run of failed lookups.
type failure struct {
	first time.Time
	calls int
}

// NewClients builds a resolver over the registry, one per Reva instance.
func NewClients(r registry.Registry) Clients {
	return &clients{
		registry: r,
		selector: FirstSelector{},
		conns:    map[string]*grpc.ClientConn{},
		fails:    map[string]*failure{},
	}
}

func (c *clients) WithSelector(s Selector) *clients {
	c.selector = s
	return c
}

// resolve picks a gRPC node for name and returns a cached connection to it. A
// name is unique per transport, so an HTTP service can carry the same one.
func (c *clients) resolve(ctx context.Context, name string) (*grpc.ClientConn, string, error) {
	var node registry.Node
	err := c.lookup(ctx, name, func() error {
		svc, err := c.registry.GetService(name)
		if err != nil {
			return fmt.Errorf("service registry: resolving %q: %w", name, err)
		}
		nodes := filterByMetadata(svc.Nodes(), map[string]string{registry.MetaTransport: registry.TransportGRPC})
		picked, ok := c.selector.Pick(nodes)
		if !ok {
			return fmt.Errorf("service registry: no selectable grpc node for %q", name)
		}
		node = picked
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	addr := node.Address()
	conn, err := c.connFor(addr)
	if err != nil {
		return nil, "", err
	}
	return conn, addr, nil
}

// lookup runs try until it succeeds, retrying a peer that is not resolvable yet
// and escalating one that never becomes resolvable.
func (c *clients) lookup(ctx context.Context, name string, try func() error) error {
	var err error
	for attempt := 1; ; attempt++ {
		if err = try(); err == nil {
			c.resolved(name)
			return nil
		}
		if attempt >= resolveAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(resolveRetryWait):
		}
	}
	c.unresolved(ctx, name, err)
	return err
}

func (c *clients) resolved(name string) {
	c.failMu.Lock()
	delete(c.fails, name)
	c.failMu.Unlock()
}

// unresolved records a failed lookup and ends the process once name has been
// unresolvable for long enough: a reva instance that cannot reach a peer it
// needs is malfunctioning, and dying is more visible than logging forever.
func (c *clients) unresolved(ctx context.Context, name string, cause error) {
	now := time.Now()
	c.failMu.Lock()
	f, ok := c.fails[name]
	if !ok {
		f = &failure{first: now}
		c.fails[name] = f
	}
	f.calls++
	calls, since := f.calls, now.Sub(f.first)
	c.failMu.Unlock()

	log := appctx.GetLogger(ctx)
	if calls < unresolvableCalls || since < unresolvableFor {
		log.Error().Err(cause).Str("service", name).Int("failed_lookups", calls).
			Str("registry", c.registrySnapshot(name)).Msg("cannot resolve peer, retrying")
		return
	}
	reason := fmt.Sprintf("reva: %q has been unresolvable for %s over %d lookups, exiting: %v",
		name, since.Truncate(time.Second), calls, cause)
	log.Error().Err(cause).Str("service", name).Int("failed_lookups", calls).
		Dur("unresolvable_for", since).Str("registry", c.registrySnapshot(name)).
		Msg("peer is unresolvable, exiting")
	exit(reason)
}

// registrySnapshot summarizes the cache on the error path: the services held
// and the target's nodes with their transport and state, so a failed lookup
// logs whether the peer is absent, unselectable, or the cache never hydrated.
func (c *clients) registrySnapshot(name string) string {
	svcs, err := c.registry.ListServices()
	if err != nil {
		return fmt.Sprintf("cannot list services: %v", err)
	}
	names := make([]string, 0, len(svcs))
	var target registry.Service
	for _, s := range svcs {
		names = append(names, fmt.Sprintf("%s(%d)", s.Name(), len(s.Nodes())))
		if s.Name() == name {
			target = s
		}
	}
	sort.Strings(names)
	if target == nil {
		return fmt.Sprintf("%d services cached %v; %q absent", len(svcs), names, name)
	}
	nodes := make([]string, 0, len(target.Nodes()))
	for _, n := range target.Nodes() {
		md := n.Metadata()
		nodes = append(nodes, fmt.Sprintf("%s[%s/%s]", n.Address(), md[registry.MetaTransport], md[registry.MetaState]))
	}
	return fmt.Sprintf("%d services cached; %q nodes: %v", len(svcs), name, nodes)
}

// connFor returns a cached connection to address, dialing on first use.
func (c *clients) connFor(address string) (*grpc.ClientConn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if conn, ok := c.conns[address]; ok {
		return conn, nil
	}
	conn, err := dial(address)
	if err != nil {
		return nil, err
	}
	c.conns[address] = conn
	return conn, nil
}

// dial opens a gRPC connection to address with reva's standard options.
func dial(address string) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxCallRecvMsgSize)),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(revtrace.NewUnaryClientInterceptor()),
		grpc.WithChainStreamInterceptor(revtrace.NewStreamClientInterceptor()),
	)
}

// Degrade is a best-effort hint; it never errors.
func (c *clients) Degrade(service, address string) {
	svc, err := c.registry.GetService(service)
	if err != nil {
		return
	}
	for _, n := range svc.Nodes() {
		if n.Address() != address {
			continue
		}
		meta := n.Metadata()
		meta[registry.MetaState] = registry.StateDegraded
		_ = c.registry.Add(registry.NewService(service, []registry.Node{
			registry.NewNode(n.ID(), n.Address(), meta),
		}))
		return
	}
}
