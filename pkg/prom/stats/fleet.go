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

package stats

import (
	"context"
	"fmt"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	svcregistry "github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/stats"
	"github.com/cs3org/reva/v3/pkg/token"
	"google.golang.org/grpc/metadata"
)

// fleet is the set of reva services registered in the deployment's service
// registry, each running as one or more instances (a service process on a
// specific node, identified by its node id "host:port/service"). The
// collector discovers the services advertising the stats invocation and
// invokes them over the control channel; this interface abstracts that
// discovery and invocation for testability.
type fleet interface {
	// AuthContext returns a context authorized to call the control
	// channel (the collector's own admin-scoped identity).
	AuthContext(ctx context.Context) (context.Context, error)
	// StatsTargets returns, per service advertising the stats invocation,
	// its live control endpoints.
	StatsTargets() map[string][]client.Endpoint
	// InvokeStats runs the stats invocation on one endpoint.
	InvokeStats(ctx context.Context, ep client.Endpoint) (*stats.Payload, error)
}

// registryFleet is the production fleet: the process-wide service
// registry plus the control channel.
type registryFleet struct {
	tokenManager token.Manager
}

// AuthContext mints a short-lived admin-scoped token for the collector's
// own identity — the control channel only accepts admin scope — and puts
// it on the outgoing metadata, exactly like an admin fan-out does. The
// token is signed with the deployment's shared JWT secret, so every
// process in the fleet validates it.
func (f registryFleet) AuthContext(ctx context.Context) (context.Context, error) {
	u := &userpb.User{
		Id:       &userpb.UserId{OpaqueId: "reva:stats-collector", Type: userpb.UserType_USER_TYPE_APPLICATION},
		Username: "stats-collector",
	}
	scopes, err := scope.AddAdminScope(nil)
	if err != nil {
		return nil, err
	}
	tkn, err := f.tokenManager.MintToken(ctx, u, scopes)
	if err != nil {
		return nil, err
	}
	return metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, tkn), nil
}

// StatsTargets scans registry metadata for services advertising the stats
// invocation — no dialing involved.
func (registryFleet) StatsTargets() map[string][]client.Endpoint {
	reg := service.GlobalRegistry()
	if reg == nil {
		return nil
	}
	svcs, err := reg.ListServices()
	if err != nil {
		return nil
	}
	targets := map[string][]client.Endpoint{}
	for _, sv := range svcs {
		if !advertisesStats(sv.Nodes()) {
			continue
		}
		if _, eps, err := client.Resolve(reg, sv.Name()); err == nil && len(eps) > 0 {
			targets[sv.Name()] = eps
		}
	}
	return targets
}

func advertisesStats(nodes []svcregistry.Node) bool {
	for _, n := range nodes {
		for name := range strings.SplitSeq(n.Metadata()[invoke.MetaInvocations], ",") {
			if strings.TrimSpace(name) == stats.Invocation {
				return true
			}
		}
	}
	return false
}

// invokeTimeout is longer than client.PerNodeTimeout: a stats invocation
// can aggregate large tables or call storage once per project, and the
// refresh runs in the background, so a scrape never waits for it.
const invokeTimeout = 5 * time.Minute

// InvokeStats runs the stats invocation on one endpoint and parses the
// payload.
func (registryFleet) InvokeStats(ctx context.Context, ep client.Endpoint) (*stats.Payload, error) {
	res := client.InvokeOne(ctx, ep, stats.Invocation, nil, invokeTimeout)
	if res.Error != "" {
		return nil, fmt.Errorf("%s: %s", res.Node, res.Error)
	}
	return stats.FromJSON([]byte(res.ResultJSON))
}
