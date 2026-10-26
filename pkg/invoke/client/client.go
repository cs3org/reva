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
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// Endpoint is one resolved invocation target: the node id to report, the
// control address to dial, and the target the control channel routes on.
// Err set means the target could not be resolved. Service, Draining and Shared
// come from the node's registry entry and route the shared invocations.
type Endpoint struct {
	Node     string
	Addr     string
	Target   string
	Err      string
	Service  string
	Draining bool
	Shared   []string
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

// readyTimeout bounds the wait for a connection before a shared invocation
// moves on to the next instance.
const readyTimeout = 2 * time.Second

// Resolve maps a selector to its control endpoints. An instance is one
// service process running on a specific node, identified by its node id
// "host:port/service"; the fleet is every instance of every service in the
// registry. A node id targets one instance, a service name every live one, a
// partial id ("host:port" or a bare host) every instance at that address or on
// that machine, "service[key=value,...]" every live instance of the service
// whose registry metadata has all the pairs, and "*" every live instance in
// the fleet. It returns the resolved service name when the selector was one.
func Resolve(reg registry.Registry, selector string) (string, []Endpoint, error) {
	// "*": every live instance in the fleet.
	if selector == "*" {
		eps := endpointsMatching(reg, func(registry.Node) bool { return true })
		if len(eps) == 0 {
			return "", nil, fmt.Errorf("no live instances in the fleet")
		}
		return selector, eps, nil
	}

	// Before the node id, because a value such as a mount path can contain "/".
	if svcName, pairs, ok := strings.Cut(selector, "["); ok {
		pairs, ok = strings.CutSuffix(pairs, "]")
		if !ok || svcName == "" {
			return "", nil, fmt.Errorf("invalid selector %q: want service[key=value,...]", selector)
		}
		meta := map[string]string{}
		for pair := range strings.SplitSeq(pairs, ",") {
			k, v, ok := strings.Cut(pair, "=")
			if k = strings.TrimSpace(k); !ok || k == "" {
				return "", nil, fmt.Errorf("invalid selector %q: %q is not key=value", selector, pair)
			}
			meta[k] = strings.TrimSpace(v)
		}
		eps, err := ResolveMetadata(reg, svcName, meta)
		if err != nil {
			return "", nil, err
		}
		return svcName, eps, nil
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

// ResolveMetadata returns the live instances of a service whose registry
// metadata has every key and value in meta. Shared invocations then go to one
// of them, other invocations to all of them.
func ResolveMetadata(reg registry.Registry, svcName string, meta map[string]string) ([]Endpoint, error) {
	sv, err := reg.GetService(svcName)
	if err != nil {
		return nil, fmt.Errorf("service %q not found", svcName)
	}
	var eps []Endpoint
	for _, n := range service.FilterByMetadata(sv.Nodes(), meta) {
		if nodeState(n) != registry.StateOffline {
			eps = append(eps, EndpointFor(n))
		}
	}
	if len(eps) == 0 {
		return nil, fmt.Errorf("service %q has no live instance with metadata %v", svcName, meta)
	}
	return eps, nil
}

// FanOut invokes every routed group in parallel, one result per group. An
// unreachable node is a per-node error rather than a failure of the whole call.
func FanOut(ctx context.Context, eps []Endpoint, invocation string, args map[string]string) []NodeResult {
	groups := Route(eps, invocation)
	results := make([]NodeResult, len(groups))
	var wg sync.WaitGroup
	for i, g := range groups {
		wg.Add(1)
		go func(i int, g []Endpoint) {
			defer wg.Done()
			results[i] = InvokeOne(ctx, Pick(ctx, g), invocation, args)
		}(i, g)
	}
	wg.Wait()
	return results
}

// Route groups the endpoints into the calls an invocation makes, one call per
// group. A shared invocation needs one instance per service, so the instances
// of a service share a group. Any other invocation gets a group per instance.
// A node that does not advertise the invocation as shared gets its own group,
// as before shared invocations existed.
func Route(eps []Endpoint, invocation string) [][]Endpoint {
	var groups [][]Endpoint
	byService := map[string]int{}
	for _, ep := range eps {
		if !slices.Contains(ep.Shared, invocation) {
			groups = append(groups, []Endpoint{ep})
			continue
		}
		if i, ok := byService[ep.Service]; ok {
			groups[i] = append(groups[i], ep)
			continue
		}
		byService[ep.Service] = len(groups)
		groups = append(groups, []Endpoint{ep})
	}
	return groups
}

// Pick selects the endpoint of a group that gets the call: the first one that
// can take it now, with instances in rotation before draining ones. It moves
// on only before the call is sent, because a sent call can have run already.
// If no endpoint can take the call, the first one gets it and reports the
// failure.
func Pick(ctx context.Context, eps []Endpoint) Endpoint {
	if len(eps) == 1 {
		return eps[0]
	}
	for _, draining := range []bool{false, true} {
		for _, ep := range eps {
			if ep.Draining == draining && ep.Err == "" && connReady(ctx, ep.Addr) {
				return ep
			}
		}
	}
	return eps[0]
}

// connReady reports whether the connection to a control endpoint becomes ready
// within readyTimeout. A failed connection stays failed for its backoff time,
// so there is no reason to wait for it.
func connReady(ctx context.Context, addr string) bool {
	conn, err := peerConn(addr)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	conn.Connect()
	for {
		st := conn.GetState()
		switch st {
		case connectivity.Ready:
			return true
		case connectivity.TransientFailure, connectivity.Shutdown:
			return false
		}
		if !conn.WaitForStateChange(ctx, st) {
			return false
		}
	}
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
	id := n.ID()
	ep := Endpoint{
		Node:     id,
		Service:  id[strings.LastIndex(id, "/")+1:],
		Draining: nodeState(n) == registry.StateDraining,
	}
	if csv := n.Metadata()[invoke.MetaSharedInvocations]; csv != "" {
		ep.Shared = strings.Split(csv, ",")
	}
	if ctrl := n.Metadata()[registry.MetaControl]; ctrl != "" {
		ep.Addr, ep.Target = ctrl, id
		return ep
	}
	ep.Err = "node advertises no control endpoint"
	return ep
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
	return n.Metadata()[registry.MetaHost] == host
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
	c, err := service.Dial(address)
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
