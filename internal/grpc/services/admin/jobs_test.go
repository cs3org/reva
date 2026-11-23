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
	"net"
	"testing"

	"github.com/cs3org/reva/v3/internal/grpc/control"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"google.golang.org/grpc"
)

// The first runner has no store, like the sssd warmer on cephweb.
func TestUsableRunnersSkipsRunnerWithoutStore(t *testing.T) {
	runner := func(info invoke.Result) invoke.Invokable {
		set := invoke.NewSet()
		set.Add("inspect", "").Handle(invoke.Typed(func(context.Context, struct{}) (invoke.Result, error) { return info, nil }))
		return set
	}
	leaderJob := []map[string]string{{"name": "reconciliation.orphans", "kind": "periodic", "scope": "leader"}}
	runners := map[string]invoke.Result{
		"a:1/jobs": {"store_wired": false, "jobs": []map[string]string{{"name": "sssdwarmer", "kind": "periodic", "scope": "all-nodes"}}},
		"b:1/jobs": {"store_wired": true, "jobs": []map[string]string{{"name": "takeout", "kind": "on-demand"}}},
		"c:1/jobs": {"store_wired": true, "jobs": leaderJob},
		"d:1/jobs": {"store_wired": false, "jobs": leaderJob},
	}
	for id, info := range runners {
		invoke.RegisterInstance(id, "jobs", nil, runner(info), nil)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	controlpb.RegisterControlServer(srv, control.NewServer())
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)

	endpoints := func() []client.Endpoint {
		var eps []client.Endpoint
		for _, id := range []string{"a:1/jobs", "b:1/jobs", "c:1/jobs", "d:1/jobs"} {
			eps = append(eps, client.Endpoint{Node: id, Addr: ln.Addr().String(), Target: id, Service: "jobs"})
		}
		return eps
	}
	nodes := func(eps []client.Endpoint) []string {
		var out []string
		for _, ep := range eps {
			out = append(out, ep.Node)
		}
		return out
	}
	ctx := context.Background()

	if got := nodes(usableRunners(ctx, endpoints(), hasStore)); len(got) != 2 || got[0] != "b:1/jobs" || got[1] != "c:1/jobs" {
		t.Errorf("runners with a store: got %v", got)
	}
	if got := nodes(usableRunners(ctx, endpoints(), runsLeaderJob("reconciliation.orphans"))); len(got) != 1 || got[0] != "c:1/jobs" {
		t.Errorf("runners of the leader job: got %v", got)
	}
	if got := usableRunners(ctx, endpoints(), runsLeaderJob("sssdwarmer")); len(got) != 0 {
		t.Errorf("an all-nodes job cannot be triggered, got %v", nodes(got))
	}
}
