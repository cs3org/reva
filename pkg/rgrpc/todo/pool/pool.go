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
	appproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/appprovider/v1beta1"
	appregistryv1beta1pb "github.com/cs3org/go-cs3apis/cs3/appregistry/v1beta1"
	authproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/authprovider/v1beta1"
	authregistryv1beta1pb "github.com/cs3org/go-cs3apis/cs3/authregistry/v1beta1"
	gatewayv0alpahpb "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	ocmshareproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/ocmshareprovider/v1beta1"
	preferencesv1beta1pb "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	publicshareproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v1beta1"
	storageproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v1beta1"
	storageregistryv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v1beta1"
	userproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/userprovider/v1beta1"
	usershareproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v1beta1"

	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
)

// TODO(labkode): is concurrent access to the maps safe?
var storageProviders = map[string]storageproviderv1beta1pb.StorageProviderServiceClient{}
var authProviders = map[string]authproviderv1beta1pb.AuthProviderServiceClient{}
var authRegistries = map[string]authregistryv1beta1pb.AuthRegistryServiceClient{}
var userShareProviders = map[string]usershareproviderv1beta1pb.UserShareProviderServiceClient{}
var ocmShareProviders = map[string]ocmshareproviderv1beta1pb.OCMShareProviderServiceClient{}
var publicShareProviders = map[string]publicshareproviderv1beta1pb.PublicShareProviderServiceClient{}
var preferencesProviders = map[string]preferencesv1beta1pb.PreferencesServiceClient{}
var appRegistries = map[string]appregistryv1beta1pb.AppRegistryServiceClient{}
var appProviders = map[string]appproviderv1beta1pb.AppProviderServiceClient{}
var storageRegistries = map[string]storageregistryv1beta1pb.StorageRegistryServiceClient{}
var gatewayProviders = map[string]gatewayv0alpahpb.GatewayServiceClient{}
var userProviders = map[string]userproviderv1beta1pb.UserProviderServiceClient{}

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
func GetUserProviderServiceClient(endpoint string) (userproviderv1beta1pb.UserProviderServiceClient, error) {
	if val, ok := userProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	userProviders[endpoint] = userproviderv1beta1pb.NewUserProviderServiceClient(conn)
	return userProviders[endpoint], nil
}

// GetStorageProviderServiceClient returns a StorageProviderServiceClient.
func GetStorageProviderServiceClient(endpoint string) (storageproviderv1beta1pb.StorageProviderServiceClient, error) {
	if val, ok := storageProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	storageProviders[endpoint] = storageproviderv1beta1pb.NewStorageProviderServiceClient(conn)

	return storageProviders[endpoint], nil
}

// GetAuthRegistryServiceClient returns a new AuthRegistryServiceClient.
func GetAuthRegistryServiceClient(endpoint string) (authregistryv1beta1pb.AuthRegistryServiceClient, error) {
	if val, ok := authRegistries[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	authRegistries[endpoint] = authregistryv1beta1pb.NewAuthRegistryServiceClient(conn)

	return authRegistries[endpoint], nil
}

// GetAuthProviderServiceClient returns a new AuthProviderServiceClient.
func GetAuthProviderServiceClient(endpoint string) (authproviderv1beta1pb.AuthProviderServiceClient, error) {
	if val, ok := authProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	authProviders[endpoint] = authproviderv1beta1pb.NewAuthProviderServiceClient(conn)

	return authProviders[endpoint], nil
}

// GetUserShareProviderClient returns a new UserShareProviderClient.
func GetUserShareProviderClient(endpoint string) (usershareproviderv1beta1pb.UserShareProviderServiceClient, error) {
	if val, ok := userShareProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	userShareProviders[endpoint] = usershareproviderv1beta1pb.NewUserShareProviderServiceClient(conn)

	return userShareProviders[endpoint], nil
}

// GetOCMShareProviderClient returns a new OCMShareProviderClient.
func GetOCMShareProviderClient(endpoint string) (ocmshareproviderv1beta1pb.OCMShareProviderServiceClient, error) {
	if val, ok := ocmShareProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	ocmShareProviders[endpoint] = ocmshareproviderv1beta1pb.NewOCMShareProviderServiceClient(conn)

	return ocmShareProviders[endpoint], nil
}

// GetPublicShareProviderClient returns a new PublicShareProviderClient.
func GetPublicShareProviderClient(endpoint string) (publicshareproviderv1beta1pb.PublicShareProviderServiceClient, error) {
	if val, ok := publicShareProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	publicShareProviders[endpoint] = publicshareproviderv1beta1pb.NewPublicShareProviderServiceClient(conn)

	return publicShareProviders[endpoint], nil
}

// GetPreferencesClient returns a new PreferencesClient.
func GetPreferencesClient(endpoint string) (preferencesv1beta1pb.PreferencesServiceClient, error) {
	if val, ok := preferencesProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	preferencesProviders[endpoint] = preferencesv1beta1pb.NewPreferencesServiceClient(conn)

	return preferencesProviders[endpoint], nil
}

// GetAppRegistryClient returns a new AppRegistryClient.
func GetAppRegistryClient(endpoint string) (appregistryv1beta1pb.AppRegistryServiceClient, error) {
	if val, ok := appRegistries[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	appRegistries[endpoint] = appregistryv1beta1pb.NewAppRegistryServiceClient(conn)

	return appRegistries[endpoint], nil
}

// GetAppProviderClient returns a new AppRegistryClient.
func GetAppProviderClient(endpoint string) (appproviderv1beta1pb.AppProviderServiceClient, error) {
	if val, ok := appProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	appProviders[endpoint] = appproviderv1beta1pb.NewAppProviderServiceClient(conn)

	return appProviders[endpoint], nil
}

// GetStorageRegistryClient returns a new StorageRegistryClient.
func GetStorageRegistryClient(endpoint string) (storageregistryv1beta1pb.StorageRegistryServiceClient, error) {
	if val, ok := storageRegistries[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	storageRegistries[endpoint] = storageregistryv1beta1pb.NewStorageRegistryServiceClient(conn)

	return storageRegistries[endpoint], nil
}
