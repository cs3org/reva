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

package service

import (
	"context"
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
	"google.golang.org/grpc"
)

var (
	globalMu      sync.RWMutex
	globalClients Clients
)

// SetGlobal installs the process-wide peer resolver. The first non-nil resolver
// wins; later calls are ignored. The runtime sets it once at startup so any
// component can resolve peers without being explicitly wired.
func SetGlobal(c Clients) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalClients == nil {
		globalClients = c
	}
}

// Global returns the process-wide resolver, or nil if none was set.
func Global() Clients {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalClients
}

// resolved returns the client produced by get from the global resolver, or
// errNoResolver if none is installed. It backs the package-level accessors
// below so callers can write service.Gateway(ctx) instead of reaching for the
// resolver themselves.
func resolved[T any](get func(Clients) (T, error)) (T, error) {
	c := Global()
	if c == nil {
		var zero T
		return zero, errNoResolver
	}
	return get(c)
}

// Gateway resolves the gateway through the global resolver.
func Gateway(ctx context.Context) (gateway.GatewayAPIClient, error) {
	return resolved(func(c Clients) (gateway.GatewayAPIClient, error) { return c.Gateway(ctx) })
}

// StorageProvider resolves the storage provider through the global resolver.
func StorageProvider(ctx context.Context) (storageprovider.ProviderAPIClient, error) {
	return resolved(func(c Clients) (storageprovider.ProviderAPIClient, error) { return c.StorageProvider(ctx) })
}

// StorageRegistry resolves the storage registry through the global resolver.
func StorageRegistry(ctx context.Context) (storageregistry.RegistryAPIClient, error) {
	return resolved(func(c Clients) (storageregistry.RegistryAPIClient, error) { return c.StorageRegistry(ctx) })
}

// Spaces resolves the spaces registry through the global resolver.
func Spaces(ctx context.Context) (storageprovider.SpacesAPIClient, error) {
	return resolved(func(c Clients) (storageprovider.SpacesAPIClient, error) { return c.Spaces(ctx) })
}

// AuthProvider resolves the auth provider through the global resolver.
func AuthProvider(ctx context.Context) (authprovider.ProviderAPIClient, error) {
	return resolved(func(c Clients) (authprovider.ProviderAPIClient, error) { return c.AuthProvider(ctx) })
}

// AuthRegistry resolves the auth registry through the global resolver.
func AuthRegistry(ctx context.Context) (authregistry.RegistryAPIClient, error) {
	return resolved(func(c Clients) (authregistry.RegistryAPIClient, error) { return c.AuthRegistry(ctx) })
}

// AppAuthProvider resolves the application auth provider through the global resolver.
func AppAuthProvider(ctx context.Context) (applicationauth.ApplicationsAPIClient, error) {
	return resolved(func(c Clients) (applicationauth.ApplicationsAPIClient, error) { return c.AppAuthProvider(ctx) })
}

// UserProvider resolves the user provider through the global resolver.
func UserProvider(ctx context.Context) (user.UserAPIClient, error) {
	return resolved(func(c Clients) (user.UserAPIClient, error) { return c.UserProvider(ctx) })
}

// GroupProvider resolves the group provider through the global resolver.
func GroupProvider(ctx context.Context) (group.GroupAPIClient, error) {
	return resolved(func(c Clients) (group.GroupAPIClient, error) { return c.GroupProvider(ctx) })
}

// UserShareProvider resolves the user share provider through the global resolver.
func UserShareProvider(ctx context.Context) (collaboration.CollaborationAPIClient, error) {
	return resolved(func(c Clients) (collaboration.CollaborationAPIClient, error) { return c.UserShareProvider(ctx) })
}

// PublicShareProvider resolves the public share provider through the global resolver.
func PublicShareProvider(ctx context.Context) (link.LinkAPIClient, error) {
	return resolved(func(c Clients) (link.LinkAPIClient, error) { return c.PublicShareProvider(ctx) })
}

// OCMShareProvider resolves the OCM share provider through the global resolver.
func OCMShareProvider(ctx context.Context) (ocm.OcmAPIClient, error) {
	return resolved(func(c Clients) (ocm.OcmAPIClient, error) { return c.OCMShareProvider(ctx) })
}

// OCMInviteManager resolves the OCM invite manager through the global resolver.
func OCMInviteManager(ctx context.Context) (invitepb.InviteAPIClient, error) {
	return resolved(func(c Clients) (invitepb.InviteAPIClient, error) { return c.OCMInviteManager(ctx) })
}

// OCMProviderAuthorizer resolves the OCM provider authorizer through the global resolver.
func OCMProviderAuthorizer(ctx context.Context) (ocmprovider.ProviderAPIClient, error) {
	return resolved(func(c Clients) (ocmprovider.ProviderAPIClient, error) { return c.OCMProviderAuthorizer(ctx) })
}

// OCMIncoming resolves the OCM incoming service through the global resolver.
func OCMIncoming(ctx context.Context) (ocmincoming.OcmIncomingAPIClient, error) {
	return resolved(func(c Clients) (ocmincoming.OcmIncomingAPIClient, error) { return c.OCMIncoming(ctx) })
}

// Preferences resolves the preferences service through the global resolver.
func Preferences(ctx context.Context) (preferences.PreferencesAPIClient, error) {
	return resolved(func(c Clients) (preferences.PreferencesAPIClient, error) { return c.Preferences(ctx) })
}

// Permissions resolves the permissions service through the global resolver.
func Permissions(ctx context.Context) (permissions.PermissionsAPIClient, error) {
	return resolved(func(c Clients) (permissions.PermissionsAPIClient, error) { return c.Permissions(ctx) })
}

// AppRegistry resolves the app registry through the global resolver.
func AppRegistry(ctx context.Context) (appregistry.RegistryAPIClient, error) {
	return resolved(func(c Clients) (appregistry.RegistryAPIClient, error) { return c.AppRegistry(ctx) })
}

// AppProvider resolves the app provider through the global resolver.
func AppProvider(ctx context.Context) (appprovider.ProviderAPIClient, error) {
	return resolved(func(c Clients) (appprovider.ProviderAPIClient, error) { return c.AppProvider(ctx) })
}

// DataTx resolves the data transfer service through the global resolver.
func DataTx(ctx context.Context) (datatx.TxAPIClient, error) {
	return resolved(func(c Clients) (datatx.TxAPIClient, error) { return c.DataTx(ctx) })
}

// Labels resolves the labels service through the global resolver.
func Labels(ctx context.Context) (labels.LabelsAPIClient, error) {
	return resolved(func(c Clients) (labels.LabelsAPIClient, error) { return c.Labels(ctx) })
}

// HTTPEndpoint resolves one ready HTTP endpoint matching the filters through the
// global resolver.
func HTTPEndpoint(ctx context.Context, opts ...EndpointOption) (Endpoint, error) {
	return resolved(func(c Clients) (Endpoint, error) { return c.HTTPEndpoint(ctx, opts...) })
}

// HTTPEndpoints resolves every ready HTTP endpoint matching the filters through
// the global resolver.
func HTTPEndpoints(ctx context.Context, opts ...EndpointOption) ([]Endpoint, error) {
	return resolved(func(c Clients) ([]Endpoint, error) { return c.HTTPEndpoints(ctx, opts...) })
}

// Degrade marks the node at address degraded on the global resolver.
func Degrade(service, address string) {
	if c := Global(); c != nil {
		c.Degrade(service, address)
	}
}

var errNoResolver = errorString("service: no global resolver installed")

type errorString string

func (e errorString) Error() string { return string(e) }

// byAddr caches connections dialed to explicit addresses (for peers the caller
// already located, e.g. a provider address handed back by a registry RPC).
var byAddr = struct {
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}{conns: map[string]*grpc.ClientConn{}}

func connAt(address string) (*grpc.ClientConn, error) {
	byAddr.mu.Lock()
	defer byAddr.mu.Unlock()
	if conn, ok := byAddr.conns[address]; ok {
		return conn, nil
	}
	conn, err := dial(address)
	if err != nil {
		return nil, err
	}
	byAddr.conns[address] = conn
	return conn, nil
}

// StorageProviderAt returns a storage provider client for an explicit address.
func StorageProviderAt(address string) (storageprovider.ProviderAPIClient, error) {
	conn, err := connAt(address)
	if err != nil {
		return nil, err
	}
	return storageprovider.NewProviderAPIClient(conn), nil
}

// StorageRegistryAt returns a storage registry client for an explicit address.
func StorageRegistryAt(address string) (storageregistry.RegistryAPIClient, error) {
	conn, err := connAt(address)
	if err != nil {
		return nil, err
	}
	return storageregistry.NewRegistryAPIClient(conn), nil
}

// AuthProviderAt returns an auth provider client for an explicit address.
func AuthProviderAt(address string) (authprovider.ProviderAPIClient, error) {
	conn, err := connAt(address)
	if err != nil {
		return nil, err
	}
	return authprovider.NewProviderAPIClient(conn), nil
}

// AppProviderAt returns an app provider client for an explicit address.
func AppProviderAt(address string) (appprovider.ProviderAPIClient, error) {
	conn, err := connAt(address)
	if err != nil {
		return nil, err
	}
	return appprovider.NewProviderAPIClient(conn), nil
}

// GatewayAt returns a gateway client for an explicit address.
func GatewayAt(address string) (gateway.GatewayAPIClient, error) {
	conn, err := connAt(address)
	if err != nil {
		return nil, err
	}
	return gateway.NewGatewayAPIClient(conn), nil
}

// UserProviderAt returns a user provider client for an explicit address.
func UserProviderAt(address string) (user.UserAPIClient, error) {
	conn, err := connAt(address)
	if err != nil {
		return nil, err
	}
	return user.NewUserAPIClient(conn), nil
}
