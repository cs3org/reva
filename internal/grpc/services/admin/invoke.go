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
	"strconv"
	"strings"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"github.com/cs3org/reva/v3/pkg/registry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
	svcName, eps, err := client.Resolve(reg, req.Service)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "admin: %v", err)
	}
	for _, ep := range eps {
		if ep.Addr == "" {
			continue
		}
		cli, err := client.ControlClientAt(ep.Addr)
		if err != nil {
			continue
		}
		if resp, err := cli.ListInvocations(ctx, &controlpb.ListInvocationsRequest{Target: ep.Target}); err == nil {
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
	_, eps, err := client.Resolve(reg, req.Service)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "admin: resolving %q: %v", req.Service, err)
	}
	results := fanOutInvoke(ctx, eps, req.Invocation, req.Args)
	admin.Audit(ctx, admin.AuditEvent{Action: "invoke", Actor: actor, Target: req.Service,
		Fields: map[string]string{"invocation": req.Invocation, "selector": req.Service, "instances": strconv.Itoa(len(results))}})
	return &adminpb.InvokeResponse{Results: results}, nil
}

// fanOutInvoke invokes every endpoint in parallel. An unreachable node is a
// per-node error rather than a failure of the whole call.
func fanOutInvoke(ctx context.Context, eps []client.Endpoint, invocation string, args map[string]string) []*adminpb.NodeResult {
	rs := client.FanOut(ctx, eps, invocation, args)
	results := make([]*adminpb.NodeResult, len(rs))
	for i, r := range rs {
		results[i] = &adminpb.NodeResult{Node: r.Node, ResultJson: r.ResultJSON, Error: r.Error}
	}
	return results
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
		out = append(out, &adminpb.InvocationSpec{
			Name:        s.Name,
			Description: s.Description,
			Args:        args,
			Kind:        s.Kind,
			Streaming:   s.Streaming,
		})
	}
	return out
}
