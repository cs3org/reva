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

// Package client resolves invocation targets through the service registry
// and runs invocations over the control channel. It is the shared
// machinery under the Admin API's Invoke and any other fleet-internal
// consumer of invocations (e.g. the stats prometheus collector).
package client

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Endpoint is one resolved invocation target: the node id to report, the
// control address to dial, and the target the control channel routes on.
// Err set means the target could not be resolved.
type Endpoint struct {
	Node   string
	Addr   string
	Target string
	Err    string
}

// NodeResult is the outcome of one peer invocation.
type NodeResult struct {
	Node       string
	ResultJSON string
	Error      string
}

// PerNodeTimeout bounds a single peer invocation so one slow or offline
// node never stalls a fleet-wide fan-out.
const PerNodeTimeout = 10 * time.Second

// Resolve maps a selector to its control endpoints: a node id
// "host:port/service" targets one instance, a service name every live one, a
// partial id ("host:port" or a bare host) every instance at that address or on
// that machine, and "*" every live instance in the fleet. It returns the
// resolved service name when the selector was one.
func Resolve(reg registry.Registry, selector string) (string, []Endpoint, error) {
	// "*": every live instance in the fleet.
	if selector == "*" {
		eps := endpointsMatching(reg, func(registry.Node) bool { return true })
		if len(eps) == 0 {
			return "", nil, fmt.Errorf("no live instances in the fleet")
		}
		return selector, eps, nil
	}

	// Node id "host:port/service": one exact instance.
	if i := strings.LastIndex(selector, "/"); i >= 0 {
		svcName := selector[i+1:]
		if svcName == "" {
			return "", nil, fmt.Errorf("invalid instance id %q", selector)
		}
		sv, err := reg.GetService(svcName)
		if err != nil {
			return "", nil, fmt.Errorf("instance %q: service %q not found", selector, svcName)
		}
		for _, n := range sv.Nodes() {
			if n.ID() == selector {
				return svcName, []Endpoint{EndpointFor(n)}, nil
			}
		}
		return "", nil, fmt.Errorf("instance %q not found", selector)
	}

	// Plain service name: every live instance.
	if sv, err := reg.GetService(selector); err == nil && len(sv.Nodes()) > 0 {
		var eps []Endpoint
		for _, n := range sv.Nodes() {
			// A drained node is out of service rotation but still alive and
			// control-reachable — keep it so it can be enabled again (and so
			// logs/stack/config still work against it). Only offline is skipped.
			if nodeState(n) == registry.StateOffline {
				continue
			}
			eps = append(eps, EndpointFor(n))
		}
		if len(eps) == 0 {
			return "", nil, fmt.Errorf("service %q has no live instances", selector)
		}
		return selector, eps, nil
	}

	// Partial id: "host:port" targets every instance at that address, a bare
	// host every instance on that machine.
	if eps := endpointsMatchingAddress(reg, selector); len(eps) > 0 {
		return selector, eps, nil
	}

	return "", nil, fmt.Errorf("%q matches no service, instance, address or host", selector)
}

// FanOut invokes every endpoint in parallel. An unreachable node is a
// per-node error rather than a failure of the whole call.
func FanOut(ctx context.Context, eps []Endpoint, invocation string, args map[string]string) []NodeResult {
	results := make([]NodeResult, len(eps))
	var wg sync.WaitGroup
	for i, ep := range eps {
		wg.Add(1)
		go func(i int, ep Endpoint) {
			defer wg.Done()
			results[i] = InvokeOne(ctx, ep, invocation, args)
		}(i, ep)
	}
	wg.Wait()
	return results
}

// InvokeOne runs a single peer invocation with a bounded timeout.
func InvokeOne(ctx context.Context, ep Endpoint, invocation string, args map[string]string) NodeResult {
	if ep.Err != "" {
		return NodeResult{Node: ep.Node, Error: ep.Err}
	}
	cli, err := ControlClientAt(ep.Addr)
	if err != nil {
		return NodeResult{Node: ep.Node, Error: err.Error()}
	}
	cctx, cancel := context.WithTimeout(ctx, PerNodeTimeout)
	defer cancel()
	resp, err := cli.Invoke(cctx, &controlpb.InvokeRequest{Target: ep.Target, Invocation: invocation, Args: args})
	if err != nil {
		return NodeResult{Node: ep.Node, Error: err.Error()}
	}
	return NodeResult{Node: ep.Node, ResultJSON: resp.ResultJson, Error: resp.Error}
}

// EndpointFor builds the endpoint dialing a node's control channel,
// routing by its id.
func EndpointFor(n registry.Node) Endpoint {
	if ctrl := n.Metadata()[registry.MetaControl]; ctrl != "" {
		return Endpoint{Node: n.ID(), Addr: ctrl, Target: n.ID()}
	}
	return Endpoint{Node: n.ID(), Err: "node advertises no control endpoint"}
}

// endpointsMatchingAddress resolves a partial node id: "host:port" matches the
// live instances bound to that address, a bare host those on that machine (by
// the id's host part or the node's host metadata).
func endpointsMatchingAddress(reg registry.Registry, selector string) []Endpoint {
	byAddress := strings.Contains(selector, ":")
	return endpointsMatching(reg, func(n registry.Node) bool {
		if byAddress {
			return strings.HasPrefix(n.ID(), selector+"/")
		}
		return onHost(n, selector)
	})
}

// endpointsMatching gathers the live instances accepted by match, sorted by
// node id.
func endpointsMatching(reg registry.Registry, match func(registry.Node) bool) []Endpoint {
	svcs, err := reg.ListServices()
	if err != nil {
		return nil
	}
	var eps []Endpoint
	for _, sv := range svcs {
		for _, n := range sv.Nodes() {
			// Drained nodes stay reachable for control (see Resolve); only
			// offline is skipped.
			if nodeState(n) == registry.StateOffline {
				continue
			}
			if !match(n) {
				continue
			}
			eps = append(eps, EndpointFor(n))
		}
	}
	sort.Slice(eps, func(i, j int) bool { return eps[i].Node < eps[j].Node })
	return eps
}

// onHost reports whether a node runs on the given host, by the host part of its
// id's address or by its host metadata.
func onHost(n registry.Node, host string) bool {
	id := n.ID()
	if i := strings.LastIndex(id, "/"); i >= 0 {
		if h, _, err := net.SplitHostPort(id[:i]); err == nil && h == host {
			return true
		}
	}
	return n.Metadata()["host"] == host
}

// nodeState reads a node's self-reported state, defaulting to ready.
func nodeState(n registry.Node) string {
	if st := n.Metadata()[registry.MetaState]; st != "" {
		return st
	}
	return registry.StateReady
}

// peerConns pools one client connection per control address.
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

// ControlClientAt returns a Control client for a peer control endpoint
// address, pooling the underlying connection.
func ControlClientAt(address string) (controlpb.ControlClient, error) {
	conn, err := peerConn(address)
	if err != nil {
		return nil, err
	}
	return controlpb.NewControlClient(conn), nil
}
