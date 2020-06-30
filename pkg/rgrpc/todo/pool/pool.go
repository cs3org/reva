// Copyright 2018-2020 CERN
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
	authprovider "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	authregistry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmcore "github.com/cs3org/go-cs3apis/cs3/ocm/core/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storageregistry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"

	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
)

var (
	publicShareProviders = sync.Map{}
	preferencesProviders = sync.Map{}
	userShareProviders   = sync.Map{}
	storageRegistries    = sync.Map{}
	gatewayProviders     = sync.Map{}
	storageProviders     = sync.Map{}
	authRegistries       = sync.Map{}
	authProviders        = sync.Map{}
	appRegistries        = sync.Map{}
	userProviders        = sync.Map{}
	appProviders         = sync.Map{}

	ocmProviderAuthorizers = sync.Map{}
	ocmShareProviders      = sync.Map{}
	ocmInviteManagers      = sync.Map{}
	ocmCores               = sync.Map{}
)

// NewConn creates a new connection to a grpc server with open census tracing support.
// TODO(labkode): make grpc tls configurable.
func NewConn(endpoint string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(endpoint, grpc.WithInsecure(), grpc.WithStatsHandler(&ocgrpc.ClientHandler{}))
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// GetGatewayServiceClient returns a GatewayServiceClient.
func GetGatewayServiceClient(endpoint string) (gateway.GatewayAPIClient, error) {
	if c, ok := gatewayProviders.Load(endpoint); ok {
		return c.(gateway.GatewayAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := gateway.NewGatewayAPIClient(conn)
	gatewayProviders.Store(endpoint, v)
	return v, nil
}

// GetUserProviderServiceClient returns a UserProviderServiceClient.
func GetUserProviderServiceClient(endpoint string) (user.UserAPIClient, error) {
	if c, ok := userProviders.Load(endpoint); ok {
		return c.(user.UserAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := user.NewUserAPIClient(conn)
	userProviders.Store(endpoint, v)
	return v, nil
}

// GetStorageProviderServiceClient returns a StorageProviderServiceClient.
func GetStorageProviderServiceClient(endpoint string) (storageprovider.ProviderAPIClient, error) {
	if c, ok := storageProviders.Load(endpoint); ok {
		return c.(storageprovider.ProviderAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := storageprovider.NewProviderAPIClient(conn)
	storageProviders.Store(endpoint, v)
	return v, nil
}

// GetAuthRegistryServiceClient returns a new AuthRegistryServiceClient.
func GetAuthRegistryServiceClient(endpoint string) (authregistry.RegistryAPIClient, error) {
	if c, ok := authRegistries.Load(endpoint); ok {
		return c.(authregistry.RegistryAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := authregistry.NewRegistryAPIClient(conn)
	authRegistries.Store(endpoint, v)
	return v, nil
}

// GetAuthProviderServiceClient returns a new AuthProviderServiceClient.
func GetAuthProviderServiceClient(endpoint string) (authprovider.ProviderAPIClient, error) {
	if c, ok := authProviders.Load(endpoint); ok {
		return c.(authprovider.ProviderAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := authprovider.NewProviderAPIClient(conn)
	authProviders.Store(endpoint, v)
	return v, nil
}

// GetUserShareProviderClient returns a new UserShareProviderClient.
func GetUserShareProviderClient(endpoint string) (collaboration.CollaborationAPIClient, error) {
	if c, ok := userShareProviders.Load(endpoint); ok {
		return c.(collaboration.CollaborationAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := collaboration.NewCollaborationAPIClient(conn)
	userShareProviders.Store(endpoint, v)
	return v, nil
}

// GetOCMShareProviderClient returns a new OCMShareProviderClient.
func GetOCMShareProviderClient(endpoint string) (ocm.OcmAPIClient, error) {
	if c, ok := ocmShareProviders.Load(endpoint); ok {
		return c.(ocm.OcmAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := ocm.NewOcmAPIClient(conn)
	ocmShareProviders.Store(endpoint, v)
	return v, nil
}

// GetOCMInviteManagerClient returns a new OCMInviteManagerClient.
func GetOCMInviteManagerClient(endpoint string) (invitepb.InviteAPIClient, error) {
	if c, ok := ocmInviteManagers.Load(endpoint); ok {
		return c.(invitepb.InviteAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := invitepb.NewInviteAPIClient(conn)
	ocmInviteManagers.Store(endpoint, v)
	return v, nil
}

// GetPublicShareProviderClient returns a new PublicShareProviderClient.
func GetPublicShareProviderClient(endpoint string) (link.LinkAPIClient, error) {
	if c, ok := publicShareProviders.Load(endpoint); ok {
		return c.(link.LinkAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := link.NewLinkAPIClient(conn)
	publicShareProviders.Store(endpoint, v)
	return v, nil
}

// GetPreferencesClient returns a new PreferencesClient.
func GetPreferencesClient(endpoint string) (preferences.PreferencesAPIClient, error) {
	if c, ok := preferencesProviders.Load(endpoint); ok {
		return c.(preferences.PreferencesAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := preferences.NewPreferencesAPIClient(conn)
	preferencesProviders.Store(endpoint, v)
	return v, nil
}

// GetAppRegistryClient returns a new AppRegistryClient.
func GetAppRegistryClient(endpoint string) (appregistry.RegistryAPIClient, error) {
	if c, ok := appRegistries.Load(endpoint); ok {
		return c.(appregistry.RegistryAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := appregistry.NewRegistryAPIClient(conn)
	appRegistries.Store(endpoint, v)
	return v, nil
}

// GetAppProviderClient returns a new AppRegistryClient.
func GetAppProviderClient(endpoint string) (appprovider.ProviderAPIClient, error) {
	if c, ok := appProviders.Load(endpoint); ok {
		return c.(appprovider.ProviderAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := appprovider.NewProviderAPIClient(conn)
	appRegistries.Store(endpoint, v)
	return v, nil
}

// GetStorageRegistryClient returns a new StorageRegistryClient.
func GetStorageRegistryClient(endpoint string) (storageregistry.RegistryAPIClient, error) {
	if c, ok := storageRegistries.Load(endpoint); ok {
		return c.(storageregistry.RegistryAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := storageregistry.NewRegistryAPIClient(conn)
	appRegistries.Store(endpoint, v)
	return v, nil
}

// GetOCMProviderAuthorizerClient returns a new OCMProviderAuthorizerClient.
func GetOCMProviderAuthorizerClient(endpoint string) (ocmprovider.ProviderAPIClient, error) {
	if c, ok := ocmProviderAuthorizers.Load(endpoint); ok {
		return c.(ocmprovider.ProviderAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := ocmprovider.NewProviderAPIClient(conn)
	ocmProviderAuthorizers.Store(endpoint, v)
	return v, nil
}

// GetOCMCoreClient returns a new OCMCoreClient.
func GetOCMCoreClient(endpoint string) (ocmcore.OcmCoreAPIClient, error) {
	if c, ok := ocmCores.Load(endpoint); ok {
		return c.(ocmcore.OcmCoreAPIClient), nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	v := ocmcore.NewOcmCoreAPIClient(conn)
	ocmProviderAuthorizers.Store(endpoint, v)
	return v, nil
}
