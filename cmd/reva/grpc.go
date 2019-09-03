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

package main

import (
	"context"
	"fmt"
	"log"

	appproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/appprovider/v0alpha"
	appregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/appregistry/v0alpha"
	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	preferencesv0alphapb "github.com/cs3org/go-cs3apis/cs3/preferences/v0alpha"
	publicshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storageregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/pkg/token"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const defaultHeader = "x-access-token"

func getAuthContext() context.Context {
	ctx := context.Background()
	// read token from file
	t, err := readToken()
	if err != nil {
		log.Println(err)
		return ctx
	}
	ctx = token.ContextSetToken(ctx, t)
	ctx = metadata.AppendToOutgoingContext(ctx, defaultHeader, t)
	return ctx
}

func getAppProviderClient(host string) (appproviderv0alphapb.AppProviderServiceClient, error) {
	conn, err := getConnToHost(host)
	if err != nil {
		return nil, err
	}
	return appproviderv0alphapb.NewAppProviderServiceClient(conn), nil
}
func getStorageBrokerClient() (storageregistryv0alphapb.StorageRegistryServiceClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return storageregistryv0alphapb.NewStorageRegistryServiceClient(conn), nil
}

func getAppRegistryClient() (appregistryv0alphapb.AppRegistryServiceClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return appregistryv0alphapb.NewAppRegistryServiceClient(conn), nil
}

func getUserShareProviderClient() (usershareproviderv0alphapb.UserShareProviderServiceClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return usershareproviderv0alphapb.NewUserShareProviderServiceClient(conn), nil
}

func getPublicShareProviderClient() (publicshareproviderv0alphapb.PublicShareProviderServiceClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return publicshareproviderv0alphapb.NewPublicShareProviderServiceClient(conn), nil
}

func getStorageProviderClient() (storageproviderv0alphapb.StorageProviderServiceClient, error) {
	return pool.GetStorageProviderServiceClient(conf.Host)
}

func getAuthClient() (authv0alphapb.AuthServiceClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return authv0alphapb.NewAuthServiceClient(conn), nil
}

func getPreferencesClient() (preferencesv0alphapb.PreferencesServiceClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return preferencesv0alphapb.NewPreferencesServiceClient(conn), nil
}

func getConn() (*grpc.ClientConn, error) {
	return grpc.Dial(conf.Host, grpc.WithInsecure())
}

func getConnToHost(host string) (*grpc.ClientConn, error) {
	return grpc.Dial(host, grpc.WithInsecure())
}

func formatError(status *rpcpb.Status) error {
	return fmt.Errorf("error: code=%+v msg=%q support_trace=%q", status.Code, status.Message, status.Trace)
}
