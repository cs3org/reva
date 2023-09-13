// Copyright 2018-2023 CERN
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

package pool

import (
	"sync"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	applicationauth "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authprovider "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	authregistry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	group "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmcore "github.com/cs3org/go-cs3apis/cs3/ocm/core/v1beta1"
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
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type provider struct {
	m    sync.Mutex
	conn map[string]interface{}
}

func newProvider() provider {
	return provider{
		sync.Mutex{},
		make(map[string]interface{}),
	}
}

// TODO(labkode): is concurrent access to the maps safe?
// var storageProviders = map[string]storageprovider.ProviderAPIClient{}.
var (
	storageProviders       = newProvider()
	authProviders          = newProvider()
	appAuthProviders       = newProvider()
	authRegistries         = newProvider()
	userShareProviders     = newProvider()
	ocmShareProviders      = newProvider()
	ocmInviteManagers      = newProvider()
	ocmProviderAuthorizers = newProvider()
	ocmCores               = newProvider()
	publicShareProviders   = newProvider()
	preferencesProviders   = newProvider()
	permissionsProviders   = newProvider()
	appRegistries          = newProvider()
	appProviders           = newProvider()
	storageRegistries      = newProvider()
	gatewayProviders       = newProvider()
	userProviders          = newProvider()
	groupProviders         = newProvider()
	dataTxs                = newProvider()
)

// NewConn creates a new connection to a grpc server
// with open census tracing support.
// TODO(labkode): make grpc tls configurable.
func NewConn(options Options) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(
		options.Endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(options.MaxCallRecvMsgSize),
		),
		grpc.WithChainUnaryInterceptor(
			otelgrpc.UnaryClientInterceptor(),
		),
		grpc.WithChainStreamInterceptor(
			otelgrpc.StreamClientInterceptor(),
		),
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// GetGatewayServiceClient returns a GatewayServiceClient.
func GetGatewayServiceClient(opts ...Option) (gateway.GatewayAPIClient, error) {
	gatewayProviders.m.Lock()
	defer gatewayProviders.m.Unlock()

	options := newOptions(opts...)
	if val, ok := gatewayProviders.conn[options.Endpoint]; ok {
		return val.(gateway.GatewayAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := gateway.NewGatewayAPIClient(conn)
	gatewayProviders.conn[options.Endpoint] = v

	return v, nil
}

// GetUserProviderServiceClient returns a UserProviderServiceClient.
func GetUserProviderServiceClient(opts ...Option) (user.UserAPIClient, error) {
	userProviders.m.Lock()
	defer userProviders.m.Unlock()

	options := newOptions(opts...)
	if val, ok := userProviders.conn[options.Endpoint]; ok {
		return val.(user.UserAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := user.NewUserAPIClient(conn)
	userProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetGroupProviderServiceClient returns a GroupProviderServiceClient.
func GetGroupProviderServiceClient(opts ...Option) (group.GroupAPIClient, error) {
	groupProviders.m.Lock()
	defer groupProviders.m.Unlock()

	options := newOptions(opts...)
	if val, ok := groupProviders.conn[options.Endpoint]; ok {
		return val.(group.GroupAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := group.NewGroupAPIClient(conn)
	groupProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetStorageProviderServiceClient returns a StorageProviderServiceClient.
func GetStorageProviderServiceClient(opts ...Option) (storageprovider.ProviderAPIClient, error) {
	storageProviders.m.Lock()
	defer storageProviders.m.Unlock()

	options := newOptions(opts...)
	if c, ok := storageProviders.conn[options.Endpoint]; ok {
		return c.(storageprovider.ProviderAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := storageprovider.NewProviderAPIClient(conn)
	storageProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetAuthRegistryServiceClient returns a new AuthRegistryServiceClient.
func GetAuthRegistryServiceClient(opts ...Option) (authregistry.RegistryAPIClient, error) {
	authRegistries.m.Lock()
	defer authRegistries.m.Unlock()

	// if there is already a connection to this node, use it.
	options := newOptions(opts...)
	if c, ok := authRegistries.conn[options.Endpoint]; ok {
		return c.(authregistry.RegistryAPIClient), nil
	}

	// if not, create a new connection
	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	// and memoize it
	v := authregistry.NewRegistryAPIClient(conn)
	authRegistries.conn[options.Endpoint] = v
	return v, nil
}

// GetAuthProviderServiceClient returns a new AuthProviderServiceClient.
func GetAuthProviderServiceClient(opts ...Option) (authprovider.ProviderAPIClient, error) {
	authProviders.m.Lock()
	defer authProviders.m.Unlock()

	options := newOptions(opts...)
	if c, ok := authProviders.conn[options.Endpoint]; ok {
		return c.(authprovider.ProviderAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := authprovider.NewProviderAPIClient(conn)
	authProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetAppAuthProviderServiceClient returns a new AppAuthProviderServiceClient.
func GetAppAuthProviderServiceClient(opts ...Option) (applicationauth.ApplicationsAPIClient, error) {
	appAuthProviders.m.Lock()
	defer appAuthProviders.m.Unlock()

	options := newOptions(opts...)
	if c, ok := appAuthProviders.conn[options.Endpoint]; ok {
		return c.(applicationauth.ApplicationsAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := applicationauth.NewApplicationsAPIClient(conn)
	appAuthProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetUserShareProviderClient returns a new UserShareProviderClient.
func GetUserShareProviderClient(opts ...Option) (collaboration.CollaborationAPIClient, error) {
	userShareProviders.m.Lock()
	defer userShareProviders.m.Unlock()

	options := newOptions(opts...)
	if c, ok := userShareProviders.conn[options.Endpoint]; ok {
		return c.(collaboration.CollaborationAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := collaboration.NewCollaborationAPIClient(conn)
	userShareProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetOCMShareProviderClient returns a new OCMShareProviderClient.
func GetOCMShareProviderClient(opts ...Option) (ocm.OcmAPIClient, error) {
	ocmShareProviders.m.Lock()
	defer ocmShareProviders.m.Unlock()

	options := newOptions(opts...)
	if c, ok := ocmShareProviders.conn[options.Endpoint]; ok {
		return c.(ocm.OcmAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := ocm.NewOcmAPIClient(conn)
	ocmShareProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetOCMInviteManagerClient returns a new OCMInviteManagerClient.
func GetOCMInviteManagerClient(opts ...Option) (invitepb.InviteAPIClient, error) {
	ocmInviteManagers.m.Lock()
	defer ocmInviteManagers.m.Unlock()

	options := newOptions(opts...)
	if c, ok := ocmInviteManagers.conn[options.Endpoint]; ok {
		return c.(invitepb.InviteAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := invitepb.NewInviteAPIClient(conn)
	ocmInviteManagers.conn[options.Endpoint] = v
	return v, nil
}

// GetPublicShareProviderClient returns a new PublicShareProviderClient.
func GetPublicShareProviderClient(opts ...Option) (link.LinkAPIClient, error) {
	publicShareProviders.m.Lock()
	defer publicShareProviders.m.Unlock()

	options := newOptions(opts...)
	if c, ok := publicShareProviders.conn[options.Endpoint]; ok {
		return c.(link.LinkAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := link.NewLinkAPIClient(conn)
	publicShareProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetPreferencesClient returns a new PreferencesClient.
func GetPreferencesClient(opts ...Option) (preferences.PreferencesAPIClient, error) {
	preferencesProviders.m.Lock()
	defer preferencesProviders.m.Unlock()

	options := newOptions(opts...)
	if c, ok := preferencesProviders.conn[options.Endpoint]; ok {
		return c.(preferences.PreferencesAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := preferences.NewPreferencesAPIClient(conn)
	preferencesProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetPermissionsClient returns a new PermissionsClient.
func GetPermissionsClient(opts ...Option) (permissions.PermissionsAPIClient, error) {
	permissionsProviders.m.Lock()
	defer permissionsProviders.m.Unlock()

	options := newOptions(opts...)
	if c, ok := permissionsProviders.conn[options.Endpoint]; ok {
		return c.(permissions.PermissionsAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := permissions.NewPermissionsAPIClient(conn)
	permissionsProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetAppRegistryClient returns a new AppRegistryClient.
func GetAppRegistryClient(opts ...Option) (appregistry.RegistryAPIClient, error) {
	appRegistries.m.Lock()
	defer appRegistries.m.Unlock()

	options := newOptions(opts...)
	if c, ok := appRegistries.conn[options.Endpoint]; ok {
		return c.(appregistry.RegistryAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := appregistry.NewRegistryAPIClient(conn)
	appRegistries.conn[options.Endpoint] = v
	return v, nil
}

// GetAppProviderClient returns a new AppRegistryClient.
func GetAppProviderClient(opts ...Option) (appprovider.ProviderAPIClient, error) {
	appProviders.m.Lock()
	defer appProviders.m.Unlock()

	options := newOptions(opts...)
	if c, ok := appProviders.conn[options.Endpoint]; ok {
		return c.(appprovider.ProviderAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := appprovider.NewProviderAPIClient(conn)
	appProviders.conn[options.Endpoint] = v
	return v, nil
}

// GetStorageRegistryClient returns a new StorageRegistryClient.
func GetStorageRegistryClient(opts ...Option) (storageregistry.RegistryAPIClient, error) {
	storageRegistries.m.Lock()
	defer storageRegistries.m.Unlock()

	options := newOptions(opts...)
	if c, ok := storageRegistries.conn[options.Endpoint]; ok {
		return c.(storageregistry.RegistryAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := storageregistry.NewRegistryAPIClient(conn)
	storageRegistries.conn[options.Endpoint] = v
	return v, nil
}

// GetOCMProviderAuthorizerClient returns a new OCMProviderAuthorizerClient.
func GetOCMProviderAuthorizerClient(opts ...Option) (ocmprovider.ProviderAPIClient, error) {
	ocmProviderAuthorizers.m.Lock()
	defer ocmProviderAuthorizers.m.Unlock()

	options := newOptions(opts...)
	if c, ok := ocmProviderAuthorizers.conn[options.Endpoint]; ok {
		return c.(ocmprovider.ProviderAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := ocmprovider.NewProviderAPIClient(conn)
	ocmProviderAuthorizers.conn[options.Endpoint] = v
	return v, nil
}

// GetOCMCoreClient returns a new OCMCoreClient.
func GetOCMCoreClient(opts ...Option) (ocmcore.OcmCoreAPIClient, error) {
	ocmCores.m.Lock()
	defer ocmCores.m.Unlock()

	options := newOptions(opts...)
	if c, ok := ocmCores.conn[options.Endpoint]; ok {
		return c.(ocmcore.OcmCoreAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := ocmcore.NewOcmCoreAPIClient(conn)
	ocmCores.conn[options.Endpoint] = v
	return v, nil
}

// GetDataTxClient returns a new DataTxClient.
func GetDataTxClient(opts ...Option) (datatx.TxAPIClient, error) {
	dataTxs.m.Lock()
	defer dataTxs.m.Unlock()

	options := newOptions(opts...)
	if c, ok := dataTxs.conn[options.Endpoint]; ok {
		return c.(datatx.TxAPIClient), nil
	}

	conn, err := NewConn(options)
	if err != nil {
		return nil, err
	}

	v := datatx.NewTxAPIClient(conn)
	dataTxs.conn[options.Endpoint] = v
	return v, nil
}

// getEndpointByName resolve service names to ip addresses present on the registry.
//	func getEndpointByName(name string) (string, error) {
//		if services, err := utils.GlobalRegistry.GetService(name); err == nil {
//			if len(services) > 0 {
//				for i := range services {
//					for j := range services[i].Nodes() {
//						// return the first one. This MUST be improved upon with selectors.
//						return services[i].Nodes()[j].Address(), nil
//					}
//				}
//			}
//		}
//
//		return "", fmt.Errorf("could not get service by name: %v", name)
//	}
