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
	authprovider "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storageregistry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
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

// Gateway resolves the gateway through the global resolver.
func Gateway(ctx context.Context) (gateway.GatewayAPIClient, error) {
	c := Global()
	if c == nil {
		return nil, errNoResolver
	}
	return c.Gateway(ctx)
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
