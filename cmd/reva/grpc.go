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

	"google.golang.org/grpc/metadata"

	"github.com/pkg/errors"

	appproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/appprovider/v0alpha"
	appregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/appregistry/v0alpha"
	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	preferencesv0alphapb "github.com/cs3org/go-cs3apis/cs3/preferences/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storageregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"

	"google.golang.org/grpc"
)

func getAuthContext() context.Context {
	ctx := context.Background()
	// read token from file
	t, err := readToken()
	if err != nil {
		log.Println(err)
		return ctx
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "x-access-token", t)
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

func getStorageProviderClient(host string) (storageproviderv0alphapb.StorageProviderServiceClient, error) {
	conn, err := getConnToHost(host)
	if err != nil {
		return nil, err
	}
	return storageproviderv0alphapb.NewStorageProviderServiceClient(conn), nil
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
	switch status.Code {
	case rpcpb.Code_CODE_NOT_FOUND:
		return errors.New("error: not found")

	default:
		return errors.New(fmt.Sprintf("apierror: code=%v msg=%s", status.Code, status.Message))
	}
}
