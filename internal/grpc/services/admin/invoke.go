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
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// endpoint is one resolved invocation target: the node id to report, the
// control address to dial, and the target the control channel routes on. err
// set means the target could not be resolved.
type endpoint struct {
	node   string
	addr   string
	target string
	err    string
}

// ListInvocations returns the invocations a service exposes: the full specs
// from one live instance's control channel, falling back to the names in
// registry metadata if none is reachable.
func (s *svc) ListInvocations(ctx context.Context, req *adminpb.ListInvocationsRequest) (*adminpb.ListInvocationsResponse, error) {
	if req.Service == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: service is required")
	}
	reg, err := s.registryHandle()
	if err != nil {
		return nil, err
	}
	svcName, eps, err := resolveSelector(reg, req.Service)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "admin: %v", err)
	}
	for _, ep := range eps {
		if ep.addr == "" {
			continue
		}
		cli, err := controlClientAt(ep.addr)
		if err != nil {
			continue
		}
		if resp, err := cli.ListInvocations(ctx, &controlpb.ListInvocationsRequest{Target: ep.target}); err == nil {
			return &adminpb.ListInvocationsResponse{Invocations: specsToAdmin(resp.Invocations)}, nil
		}
	}
	specs, err := invocationsFromMetadata(reg, svcName)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "admin: %v", err)
	}
	return &adminpb.ListInvocationsResponse{Invocations: specs}, nil
}

// Invoke resolves the selector, dials each resolved instance's control channel
// and merges the per-instance results. A service name fans out to every
// instance; a node id targets one.
func (s *svc) Invoke(ctx context.Context, req *adminpb.InvokeRequest) (*adminpb.InvokeResponse, error) {
	if req.Service == "" || req.Invocation == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: service and invocation are required")
	}
	actor := actorName(ctx)

	reg, err := s.registryHandle()
	if err != nil {
		return nil, err
	}
	_, eps, err := resolveSelector(reg, req.Service)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "admin: resolving %q: %v", req.Service, err)
	}
	results := fanOutInvoke(ctx, eps, req.Invocation, req.Args)
	admin.Audit(ctx, admin.AuditEvent{Action: "invoke", Actor: actor, Target: req.Service,
		Fields: map[string]string{"invocation": req.Invocation, "selector": req.Service, "instances": strconv.Itoa(len(results))}})
	return &adminpb.InvokeResponse{Results: results}, nil
}

// perNodeTimeout bounds a single peer invocation so one slow or offline node
// never stalls a fleet-wide fan-out.
const perNodeTimeout = 10 * time.Second

// fanOutInvoke invokes every endpoint in parallel. An unreachable node is a
// per-node error rather than a failure of the whole call.
func fanOutInvoke(ctx context.Context, eps []endpoint, invocation string, args map[string]string) []*adminpb.NodeResult {
	results := make([]*adminpb.NodeResult, len(eps))
	var wg sync.WaitGroup
	for i, ep := range eps {
		wg.Add(1)
		go func(i int, ep endpoint) {
			defer wg.Done()
			results[i] = invokeOne(ctx, ep, invocation, args)
		}(i, ep)
	}
	wg.Wait()
	return results
}

// invokeOne runs a single peer invocation with a bounded timeout.
func invokeOne(ctx context.Context, ep endpoint, invocation string, args map[string]string) *adminpb.NodeResult {
	if ep.err != "" {
		return &adminpb.NodeResult{Node: ep.node, Error: ep.err}
	}
	cli, err := controlClientAt(ep.addr)
	if err != nil {
		return &adminpb.NodeResult{Node: ep.node, Error: err.Error()}
	}
	cctx, cancel := context.WithTimeout(ctx, perNodeTimeout)
	defer cancel()
	resp, err := cli.Invoke(cctx, &controlpb.InvokeRequest{Target: ep.target, Invocation: invocation, Args: args})
	if err != nil {
		return &adminpb.NodeResult{Node: ep.node, Error: err.Error()}
	}
	return &adminpb.NodeResult{Node: ep.node, ResultJson: resp.ResultJson, Error: resp.Error}
}

// resolveSelector maps a selector to its control endpoints: a node id
// "host:port/service" targets one instance, a service name every live one, and
// a partial id ("host:port" or a bare host) every instance at that address or
// on that machine.
func resolveSelector(reg registry.Registry, selector string) (string, []endpoint, error) {
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
				return svcName, []endpoint{controlEndpointFor(n)}, nil
			}
		}
		return "", nil, fmt.Errorf("instance %q not found", selector)
	}

	// Plain service name: every live instance.
	if sv, err := reg.GetService(selector); err == nil && len(sv.Nodes()) > 0 {
		var eps []endpoint
		for _, n := range sv.Nodes() {
			if st := nodeState(n); st == registry.StateOffline || st == registry.StateDraining {
				continue
			}
			eps = append(eps, controlEndpointFor(n))
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

// endpointsMatchingAddress resolves a partial node id: "host:port" matches the
// live instances bound to that address, a bare host those on that machine (by
// the id's host part or the node's host metadata).
func endpointsMatchingAddress(reg registry.Registry, selector string) []endpoint {
	svcs, err := reg.ListServices()
	if err != nil {
		return nil
	}
	byAddress := strings.Contains(selector, ":")
	var eps []endpoint
	for _, sv := range svcs {
		for _, n := range sv.Nodes() {
			if st := nodeState(n); st == registry.StateOffline || st == registry.StateDraining {
				continue
			}
			if byAddress {
				if !strings.HasPrefix(n.ID(), selector+"/") {
					continue
				}
			} else if !onHost(n, selector) {
				continue
			}
			eps = append(eps, controlEndpointFor(n))
		}
	}
	sort.Slice(eps, func(i, j int) bool { return eps[i].node < eps[j].node })
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

// controlEndpointFor builds the endpoint dialing a node's control channel,
// routing by its id.
func controlEndpointFor(n registry.Node) endpoint {
	if ctrl := n.Metadata()[registry.MetaControl]; ctrl != "" {
		return endpoint{node: n.ID(), addr: ctrl, target: n.ID()}
	}
	return endpoint{node: n.ID(), err: "node advertises no control endpoint"}
}

// invocationsFromMetadata reads the invocation names a service advertises in
// registry metadata, without dialing.
func invocationsFromMetadata(reg registry.Registry, svcName string) ([]*adminpb.InvocationSpec, error) {
	sv, err := reg.GetService(svcName)
	if err != nil {
		return nil, err
	}
	for _, n := range sv.Nodes() {
		if csv := n.Metadata()[invoke.MetaInvocations]; csv != "" {
			var specs []*adminpb.InvocationSpec
			for name := range strings.SplitSeq(csv, ",") {
				if name = strings.TrimSpace(name); name != "" {
					specs = append(specs, &adminpb.InvocationSpec{Name: name})
				}
			}
			return specs, nil
		}
	}
	return nil, fmt.Errorf("service %q advertises no invocations", svcName)
}

func actorName(ctx context.Context) string {
	if u, ok := appctx.ContextGetUser(ctx); ok && u != nil {
		return u.Username
	}
	return ""
}

// specsToAdmin maps the control channel's InvocationSpecs to the admin wire
// type, keeping the two protos decoupled.
func specsToAdmin(in []*controlpb.InvocationSpec) []*adminpb.InvocationSpec {
	out := make([]*adminpb.InvocationSpec, 0, len(in))
	for _, s := range in {
		args := make([]*adminpb.ArgSpec, 0, len(s.Args))
		for _, a := range s.Args {
			args = append(args, &adminpb.ArgSpec{Name: a.Name, Description: a.Description, Required: a.Required})
		}
		out = append(out, &adminpb.InvocationSpec{Name: s.Name, Description: s.Description, Args: args, Kind: s.Kind})
	}
	return out
}
