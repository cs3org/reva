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
	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	authprovider "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	authregistry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/invite/v1beta1"
	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storageregistry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"

	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
)

// TODO(labkode): is concurrent access to the maps safe?
var storageProviders = map[string]storageprovider.ProviderAPIClient{}
var authProviders = map[string]authprovider.ProviderAPIClient{}
var authRegistries = map[string]authregistry.RegistryAPIClient{}
var userShareProviders = map[string]collaboration.CollaborationAPIClient{}
var ocmShareProviders = map[string]ocm.OcmAPIClient{}
var ocmInviteManagers = map[string]invitepb.InviteAPIClient{}
var publicShareProviders = map[string]link.LinkAPIClient{}
var preferencesProviders = map[string]preferences.PreferencesAPIClient{}
var appRegistries = map[string]appregistry.RegistryAPIClient{}
var appProviders = map[string]appprovider.ProviderAPIClient{}
var storageRegistries = map[string]storageregistry.RegistryAPIClient{}
var gatewayProviders = map[string]gateway.GatewayAPIClient{}
var userProviders = map[string]user.UserAPIClient{}

// NewConn creates a new connection to a grpc server
// with open census tracing support.
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
	if val, ok := gatewayProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	gatewayProviders[endpoint] = gateway.NewGatewayAPIClient(conn)

	return gatewayProviders[endpoint], nil
}

// GetUserProviderServiceClient returns a UserProviderServiceClient.
func GetUserProviderServiceClient(endpoint string) (user.UserAPIClient, error) {
	if val, ok := userProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	userProviders[endpoint] = user.NewUserAPIClient(conn)
	return userProviders[endpoint], nil
}

// GetStorageProviderServiceClient returns a StorageProviderServiceClient.
func GetStorageProviderServiceClient(endpoint string) (storageprovider.ProviderAPIClient, error) {
	if val, ok := storageProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	storageProviders[endpoint] = storageprovider.NewProviderAPIClient(conn)

	return storageProviders[endpoint], nil
}

// GetAuthRegistryServiceClient returns a new AuthRegistryServiceClient.
func GetAuthRegistryServiceClient(endpoint string) (authregistry.RegistryAPIClient, error) {
	if val, ok := authRegistries[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	authRegistries[endpoint] = authregistry.NewRegistryAPIClient(conn)

	return authRegistries[endpoint], nil
}

// GetAuthProviderServiceClient returns a new AuthProviderServiceClient.
func GetAuthProviderServiceClient(endpoint string) (authprovider.ProviderAPIClient, error) {
	if val, ok := authProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	authProviders[endpoint] = authprovider.NewProviderAPIClient(conn)

	return authProviders[endpoint], nil
}

// GetUserShareProviderClient returns a new UserShareProviderClient.
func GetUserShareProviderClient(endpoint string) (collaboration.CollaborationAPIClient, error) {
	if val, ok := userShareProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	userShareProviders[endpoint] = collaboration.NewCollaborationAPIClient(conn)

	return userShareProviders[endpoint], nil
}

// GetOCMShareProviderClient returns a new OCMShareProviderClient.
func GetOCMShareProviderClient(endpoint string) (ocm.OcmAPIClient, error) {
	if val, ok := ocmShareProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	ocmShareProviders[endpoint] = ocm.NewOcmAPIClient(conn)

	return ocmShareProviders[endpoint], nil
}

// GetOCMInviteManagerClient returns a new OCMInviteManagerClient.
func GetOCMInviteManagerClient(endpoint string) (invitepb.InviteAPIClient, error) {
	if val, ok := ocmInviteManagers[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	ocmInviteManagers[endpoint] = invitepb.NewInviteAPIClient(conn)

	return ocmInviteManagers[endpoint], nil
}

// GetPublicShareProviderClient returns a new PublicShareProviderClient.
func GetPublicShareProviderClient(endpoint string) (link.LinkAPIClient, error) {
	if val, ok := publicShareProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	publicShareProviders[endpoint] = link.NewLinkAPIClient(conn)

	return publicShareProviders[endpoint], nil
}

// GetPreferencesClient returns a new PreferencesClient.
func GetPreferencesClient(endpoint string) (preferences.PreferencesAPIClient, error) {
	if val, ok := preferencesProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	preferencesProviders[endpoint] = preferences.NewPreferencesAPIClient(conn)

	return preferencesProviders[endpoint], nil
}

// GetAppRegistryClient returns a new AppRegistryClient.
func GetAppRegistryClient(endpoint string) (appregistry.RegistryAPIClient, error) {
	if val, ok := appRegistries[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	appRegistries[endpoint] = appregistry.NewRegistryAPIClient(conn)

	return appRegistries[endpoint], nil
}

// GetAppProviderClient returns a new AppRegistryClient.
func GetAppProviderClient(endpoint string) (appprovider.ProviderAPIClient, error) {
	if val, ok := appProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	appProviders[endpoint] = appprovider.NewProviderAPIClient(conn)

	return appProviders[endpoint], nil
}

// GetStorageRegistryClient returns a new StorageRegistryClient.
func GetStorageRegistryClient(endpoint string) (storageregistry.RegistryAPIClient, error) {
	if val, ok := storageRegistries[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	storageRegistries[endpoint] = storageregistry.NewRegistryAPIClient(conn)

	return storageRegistries[endpoint], nil
}
