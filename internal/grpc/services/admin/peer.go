// Copyright 2018-2026 CERN
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

package admin

import (
	"sync"

	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// peerConns caches gRPC connections to peer control endpoints; the caller's
// admin token rides the outgoing context.
var peerConns = struct {
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}{conns: map[string]*grpc.ClientConn{}}

func peerConn(address string) (*grpc.ClientConn, error) {
	peerConns.mu.Lock()
	defer peerConns.mu.Unlock()
	if c, ok := peerConns.conns[address]; ok {
		return c, nil
	}
	c, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	peerConns.conns[address] = c
	return c, nil
}

// controlClientAt returns a Control client for a peer control endpoint address.
func controlClientAt(address string) (controlpb.ControlClient, error) {
	conn, err := peerConn(address)
	if err != nil {
		return nil, err
	}
	return controlpb.NewControlClient(conn), nil
}
