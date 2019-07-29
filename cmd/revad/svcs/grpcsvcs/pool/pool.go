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
	appregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/appregistry/v0alpha"
	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	preferencesv0alphapb "github.com/cs3org/go-cs3apis/cs3/preferences/v0alpha"
	publicshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storageregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"

	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
)

// TODO(labkode): protect with mutexes.
var storageProviders = map[string]storageproviderv0alphapb.StorageProviderServiceClient{}
var authProviders = map[string]authv0alphapb.AuthServiceClient{}
var userShareProviders = map[string]usershareproviderv0alphapb.UserShareProviderServiceClient{}
var publicShareProviders = map[string]publicshareproviderv0alphapb.PublicShareProviderServiceClient{}
var preferencesProviders = map[string]preferencesv0alphapb.PreferencesServiceClient{}
var appRegistries = map[string]appregistryv0alphapb.AppRegistryServiceClient{}
var storageRegistries = map[string]storageregistryv0alphapb.StorageRegistryServiceClient{}

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

// GetAuthServiceClient returns a new AuthServiceClient.
func GetAuthServiceClient(endpoint string) (authv0alphapb.AuthServiceClient, error) {
	if val, ok := authProviders[endpoint]; ok {
		return val, nil
	}

	conn, err := NewConn(endpoint)
	if err != nil {
		return nil, err
	}

	authProviders[endpoint] = authv0alphapb.NewAuthServiceClient(conn)

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
