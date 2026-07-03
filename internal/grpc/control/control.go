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

// Package control implements the per-process control channel
// (reva.control.v1beta1): a gRPC service that runs invocations locally on the
// addressed target. It is independent of the Admin API.
package control

import (
	"context"
	"encoding/json"

	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// server implements the Control gRPC service, routing each request to the
// service instance its target node id addresses.
type server struct {
	controlpb.UnimplementedControlServer
}

// NewServer returns the Control gRPC server implementation.
func NewServer() controlpb.ControlServer { return &server{} }

// ListInvocations returns the InvocationSpecs the addressed target exposes.
func (s *server) ListInvocations(_ context.Context, req *controlpb.ListInvocationsRequest) (*controlpb.ListInvocationsResponse, error) {
	specs, ok := invoke.Invocations(req.Target)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "control: %q is not invokable on this node", req.Target)
	}
	out := make([]*controlpb.InvocationSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, toPBSpec(spec))
	}
	return &controlpb.ListInvocationsResponse{Invocations: out}, nil
}

// Invoke runs a named invocation on the addressed target. An invocation error
// surfaces as a soft error in the response; only an unknown target is a gRPC
// error.
func (s *server) Invoke(ctx context.Context, req *controlpb.InvokeRequest) (*controlpb.InvokeResponse, error) {
	if _, ok := invoke.Invocations(req.Target); !ok {
		return nil, status.Errorf(codes.NotFound, "control: %q is not invokable on this node", req.Target)
	}
	res, err := invoke.Invoke(ctx, req.Target, req.Invocation, ArgsToAny(req.Args))
	if err != nil {
		return &controlpb.InvokeResponse{Error: err.Error()}, nil
	}
	b, err := json.Marshal(res)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "control: marshaling result: %v", err)
	}
	return &controlpb.InvokeResponse{ResultJson: string(b)}, nil
}

func toPBSpec(spec invoke.InvocationSpec) *controlpb.InvocationSpec {
	args := make([]*controlpb.ArgSpec, 0, len(spec.Args))
	for _, a := range spec.Args {
		args = append(args, &controlpb.ArgSpec{Name: a.Name, Description: a.Description, Required: a.Required})
	}
	return &controlpb.InvocationSpec{
		Name:        spec.Name,
		Description: spec.Description,
		Args:        args,
		Kind:        string(spec.Kind),
	}
}

// ArgsToAny widens the wire's string args to the map[string]any an Invokable
// expects.
func ArgsToAny(args map[string]string) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = v
	}
	return out
}

// Service adapts the control channel to rgrpc.Service so the runtime can host
// it on a dedicated per-process server.
type Service struct{}

// New returns the control channel as an rgrpc.Service.
func New() rgrpc.Service { return &Service{} }

func (Service) Register(ss *grpc.Server)       { controlpb.RegisterControlServer(ss, NewServer()) }
func (Service) Close() error                   { return nil }
func (Service) UnprotectedEndpoints() []string { return nil }
