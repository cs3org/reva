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
	"sync"

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
	"github.com/cs3org/reva/v3/pkg/registry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// maxCallRecvMsgSize is the default maximum gRPC receive message size (in bytes).
const maxCallRecvMsgSize = 10240000

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
}

// NewClients builds a resolver over the registry, one per Reva instance.
func NewClients(r registry.Registry) Clients {
	return &clients{
		registry: r,
		selector: FirstSelector{},
		conns:    map[string]*grpc.ClientConn{},
	}
}

func (c *clients) WithSelector(s Selector) *clients {
	c.selector = s
	return c
}

// resolve picks a node for name and returns a cached connection to it.
func (c *clients) resolve(name string) (*grpc.ClientConn, string, error) {
	svc, err := c.registry.GetService(name)
	if err != nil {
		return nil, "", fmt.Errorf("service registry: resolving %q: %w", name, err)
	}
	node, ok := c.selector.Pick(svc.Nodes())
	if !ok {
		return nil, "", fmt.Errorf("service registry: no selectable node for %q", name)
	}
	addr := node.Address()
	conn, err := c.connFor(addr)
	if err != nil {
		return nil, "", err
	}
	return conn, addr, nil
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
