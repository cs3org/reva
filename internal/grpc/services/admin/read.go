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
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// registryHandle returns the process-wide registry, or Unavailable if the
// runtime has not installed one.
func (s *svc) registryHandle() (registry.Registry, error) {
	reg := service.GlobalRegistry()
	if reg == nil {
		return nil, status.Error(codes.Unavailable, "admin: service registry not available")
	}
	return reg, nil
}

// GetServerInfo reports this process's version, build, uptime, host and pid.
func (s *svc) GetServerInfo(ctx context.Context, _ *adminpb.GetServerInfoRequest) (*adminpb.GetServerInfoResponse, error) {
	host, _ := os.Hostname()
	version, build, goVer := serverVersion()
	return &adminpb.GetServerInfoResponse{Info: &adminpb.ServerInfo{
		Version:       version,
		Build:         build,
		GoVersion:     goVer,
		UptimeSeconds: int64(time.Since(s.startTime).Seconds()),
		Host:          host,
		Pid:           int32(os.Getpid()),
	}}, nil
}

// serverVersion reads version/build from the embedded build info (importing
// the revad main package would cycle).
func serverVersion() (version, build, goVer string) {
	goVer = runtime.Version()
	if bi, ok := debug.ReadBuildInfo(); ok {
		version = bi.Main.Version
		if bi.GoVersion != "" {
			goVer = bi.GoVersion
		}
		for _, kv := range bi.Settings {
			if kv.Key == "vcs.revision" {
				build = kv.Value
			}
		}
	}
	return version, build, goVer
}

// GetHealth rolls up each service's node states into a single health verdict.
func (s *svc) GetHealth(ctx context.Context, _ *adminpb.GetHealthRequest) (*adminpb.GetHealthResponse, error) {
	reg, err := s.registryHandle()
	if err != nil {
		return nil, err
	}
	svcs, err := reg.ListServices()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "admin: reading health: %v", err)
	}
	out := make([]*adminpb.ServiceHealth, 0, len(svcs))
	for _, sv := range svcs {
		nodes := sv.Nodes()
		var ready, draining int
		for _, n := range nodes {
			switch nodeState(n) {
			case registry.StateReady:
				ready++
			case registry.StateDraining:
				draining++
			}
		}
		out = append(out, &adminpb.ServiceHealth{
			Service: sv.Name(),
			State:   rollup(len(nodes), ready, draining),
			Ready:   int32(ready),
			Total:   int32(len(nodes)),
		})
	}
	return &adminpb.GetHealthResponse{Services: out}, nil
}

// ListServices returns the fleet map from the registry, optionally filtered to
// a single service name.
func (s *svc) ListServices(ctx context.Context, req *adminpb.ListServicesRequest) (*adminpb.ListServicesResponse, error) {
	reg, err := s.registryHandle()
	if err != nil {
		return nil, err
	}
	svcs, err := listServices(reg, req.Service)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "admin: listing services: %v", err)
	}
	out := make([]*adminpb.Service, 0, len(svcs))
	for _, sv := range svcs {
		nodes := make([]*adminpb.Node, 0, len(sv.Nodes()))
		for _, n := range sv.Nodes() {
			nodes = append(nodes, toNode(n))
		}
		out = append(out, &adminpb.Service{Name: sv.Name(), Nodes: nodes})
	}
	return &adminpb.ListServicesResponse{Services: out}, nil
}

// GetServiceConfig returns a service's effective, redacted config: a typed
// convenience that runs the shared config invocation on the selected
// instance(s), with the usual selector rules.
func (s *svc) GetServiceConfig(ctx context.Context, req *adminpb.GetServiceConfigRequest) (*adminpb.GetServiceConfigResponse, error) {
	if req.Service == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: service is required")
	}
	reg, err := s.registryHandle()
	if err != nil {
		return nil, err
	}
	_, eps, err := resolveSelector(reg, req.Service)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "admin: %v", err)
	}
	results := fanOutInvoke(ctx, eps, invoke.ConfigInvocation, nil)
	return &adminpb.GetServiceConfigResponse{Results: results}, nil
}

// listServices returns all services, or just the named one when filter != "".
func listServices(reg registry.Registry, filter string) ([]registry.Service, error) {
	if filter != "" {
		sv, err := reg.GetService(filter)
		if err != nil {
			return nil, err
		}
		return []registry.Service{sv}, nil
	}
	return reg.ListServices()
}

// nodeState reads a node's self-reported state, defaulting to ready.
func nodeState(n registry.Node) string {
	if st := n.Metadata()[registry.MetaState]; st != "" {
		return st
	}
	return registry.StateReady
}

// toNode maps a registry node to its adminpb representation.
func toNode(n registry.Node) *adminpb.Node {
	return &adminpb.Node{
		Id:       n.ID(),
		Address:  n.Address(),
		State:    nodeState(n),
		Metadata: n.Metadata(),
	}
}

// rollup reduces per-node counts to a single service health verdict.
func rollup(total, ready, draining int) string {
	switch {
	case total == 0:
		return registry.StateOffline
	case ready == total:
		return registry.StateReady
	case ready > 0:
		return registry.StateDegraded
	case draining > 0:
		return registry.StateDraining
	default:
		return registry.StateOffline
	}
}
