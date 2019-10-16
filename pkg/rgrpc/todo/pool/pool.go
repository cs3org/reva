// Copyright 2018-2019 CERN
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
	appproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/appprovider/v0alpha"
	appregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/appregistry/v0alpha"
	authproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/authprovider/v0alpha"
	authregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/authregistry/v0alpha"
	gatewayv0alpahpb "github.com/cs3org/go-cs3apis/cs3/gateway/v0alpha"
	ocmshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/ocmshareprovider/v0alpha"
	preferencesv0alphapb "github.com/cs3org/go-cs3apis/cs3/preferences/v0alpha"
	publicshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storageregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"
	userproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/userprovider/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"

	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
)

// TODO(labkode): is concurrent access to the maps safe?
var storageProviders = map[string]storageproviderv0alphapb.StorageProviderServiceClient{}
var authProviders = map[string]authproviderv0alphapb.AuthProviderServiceClient{}
var authRegistries = map[string]authregistryv0alphapb.AuthRegistryServiceClient{}
var userShareProviders = map[string]usershareproviderv0alphapb.UserShareProviderServiceClient{}
var ocmShareProviders = map[string]ocmshareproviderv0alphapb.OCMShareProviderServiceClient{}
var publicShareProviders = map[string]publicshareproviderv0alphapb.PublicShareProviderServiceClient{}
var preferencesProviders = map[string]preferencesv0alphapb.PreferencesServiceClient{}
var appRegistries = map[string]appregistryv0alphapb.AppRegistryServiceClient{}
var appProviders = map[string]appproviderv0alphapb.AppProviderServiceClient{}
var storageRegistries = map[string]storageregistryv0alphapb.StorageRegistryServiceClient{}
var gatewayProviders = map[string]gatewayv0alpahpb.GatewayServiceClient{}
var userProviders = map[string]userproviderv0alphapb.UserProviderServiceClient{}

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
func GetGatewayServiceClient(endpoint string) (gatewayv0alpahpb.GatewayServiceClient, error) {
	if val, ok := gatewayProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	gatewayProviders[endpoint] = gatewayv0alpahpb.NewGatewayServiceClient(conn)

	return gatewayProviders[endpoint], nil
}

// GetUserProviderServiceClient returns a UserProviderServiceClient.
func GetUserProviderServiceClient(endpoint string) (userproviderv0alphapb.UserProviderServiceClient, error) {
	if val, ok := userProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	userProviders[endpoint] = userproviderv0alphapb.NewUserProviderServiceClient(conn)
	return userProviders[endpoint], nil
}

// GetStorageProviderServiceClient returns a StorageProviderServiceClient.
func GetStorageProviderServiceClient(endpoint string) (storageproviderv0alphapb.StorageProviderServiceClient, error) {
	if val, ok := storageProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	storageProviders[endpoint] = storageproviderv0alphapb.NewStorageProviderServiceClient(conn)

	return storageProviders[endpoint], nil
}

// GetAuthRegistryServiceClient returns a new AuthRegistryServiceClient.
func GetAuthRegistryServiceClient(endpoint string) (authregistryv0alphapb.AuthRegistryServiceClient, error) {
	if val, ok := authRegistries[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	authRegistries[endpoint] = authregistryv0alphapb.NewAuthRegistryServiceClient(conn)

	return authRegistries[endpoint], nil
}

// GetAuthProviderServiceClient returns a new AuthProviderServiceClient.
func GetAuthProviderServiceClient(endpoint string) (authproviderv0alphapb.AuthProviderServiceClient, error) {
	if val, ok := authProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	authProviders[endpoint] = authproviderv0alphapb.NewAuthProviderServiceClient(conn)

	return authProviders[endpoint], nil
}

// GetUserShareProviderClient returns a new UserShareProviderClient.
func GetUserShareProviderClient(endpoint string) (usershareproviderv0alphapb.UserShareProviderServiceClient, error) {
	if val, ok := userShareProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	userShareProviders[endpoint] = usershareproviderv0alphapb.NewUserShareProviderServiceClient(conn)

	return userShareProviders[endpoint], nil
}

// GetOCMShareProviderClient returns a new OCMShareProviderClient.
func GetOCMShareProviderClient(endpoint string) (ocmshareproviderv0alphapb.OCMShareProviderServiceClient, error) {
	if val, ok := ocmShareProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	ocmShareProviders[endpoint] = ocmshareproviderv0alphapb.NewOCMShareProviderServiceClient(conn)

	return ocmShareProviders[endpoint], nil
}

// GetPublicShareProviderClient returns a new PublicShareProviderClient.
func GetPublicShareProviderClient(endpoint string) (publicshareproviderv0alphapb.PublicShareProviderServiceClient, error) {
	if val, ok := publicShareProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	publicShareProviders[endpoint] = publicshareproviderv0alphapb.NewPublicShareProviderServiceClient(conn)

	return publicShareProviders[endpoint], nil
}

// GetPreferencesClient returns a new PreferencesClient.
func GetPreferencesClient(endpoint string) (preferencesv0alphapb.PreferencesServiceClient, error) {
	if val, ok := preferencesProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	preferencesProviders[endpoint] = preferencesv0alphapb.NewPreferencesServiceClient(conn)

	return preferencesProviders[endpoint], nil
}

// GetAppRegistryClient returns a new AppRegistryClient.
func GetAppRegistryClient(endpoint string) (appregistryv0alphapb.AppRegistryServiceClient, error) {
	if val, ok := appRegistries[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	appRegistries[endpoint] = appregistryv0alphapb.NewAppRegistryServiceClient(conn)

	return appRegistries[endpoint], nil
}

// GetAppProviderClient returns a new AppRegistryClient.
func GetAppProviderClient(endpoint string) (appproviderv0alphapb.AppProviderServiceClient, error) {
	if val, ok := appProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	appProviders[endpoint] = appproviderv0alphapb.NewAppProviderServiceClient(conn)

	return appProviders[endpoint], nil
}

// GetStorageRegistryClient returns a new StorageRegistryClient.
func GetStorageRegistryClient(endpoint string) (storageregistryv0alphapb.StorageRegistryServiceClient, error) {
	if val, ok := storageRegistries[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	storageRegistries[endpoint] = storageregistryv0alphapb.NewStorageRegistryServiceClient(conn)

	return storageRegistries[endpoint], nil
}
