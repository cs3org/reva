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

package spacesregistry

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/projects"
	projectsql "github.com/cs3org/reva/v3/pkg/projects/manager/sql"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
)

var (
	operator   = &userpb.User{Id: &userpb.UserId{OpaqueId: "operator"}, Username: "operator"}
	einstein   = &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}, Username: "einstein"}
	cernboxsvc = &userpb.User{Id: &userpb.UserId{OpaqueId: "cernboxsvc"}, Username: "cernboxsvc"}
	users      = []*userpb.User{operator, einstein, cernboxsvc}
)

type fakeGateway struct{ gateway.GatewayAPIClient }

func (fakeGateway) GetUserByClaim(_ context.Context, req *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	for _, u := range users {
		if req.GetClaim() == "username" && req.GetValue() == u.Username {
			return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: u}, nil
		}
	}
	return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND, Message: "not found"}}, nil
}

type resolver struct{ revaservice.Clients }

func (resolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) { return fakeGateway{}, nil }

func init() {
	revaservice.SetGlobal(resolver{Clients: revaservice.Global()})
}

func newService(t *testing.T) *service {
	t.Helper()
	cat, err := projectsql.New(context.Background(), map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "projects.db")})
	if err != nil {
		t.Fatal(err)
	}
	s := &service{c: &config{Driver: "sql"}, projects: cat}
	s.registerInvocations()
	return s
}

func adminContext() context.Context {
	return appctx.ContextSetUser(context.Background(), operator)
}

func mustInvoke(t *testing.T, s *service, name string, args map[string]any) invoke.Result {
	t.Helper()
	res, err := s.Invoke(adminContext(), name, args)
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return res
}

func names(t *testing.T, res invoke.Result) []string {
	t.Helper()
	ps, ok := res["projects"].([]invoke.Result)
	if !ok {
		t.Fatalf("no projects in %v", res)
	}
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p["name"].(string))
	}
	return out
}

func TestCreateListGet(t *testing.T) {
	s := newService(t)
	res := mustInvoke(t, s, "create", map[string]any{"name": "cernbox", "owner": "cernboxsvc", "storage_id": "eosproject-c", "snow_ticket": "RQF123"})
	if res["created"] != true || res["path"] != "/eos/project/c/cernbox" || res["status"] != "active" || res["admins"] != "cernbox-project-cernbox-admins" || res["space_id"] == "" || res["snow_ticket"] != "RQF123" {
		t.Fatalf("create: %v", res)
	}
	// the storage of a new project under the same initial is derived from its siblings
	res = mustInvoke(t, s, "create", map[string]any{"name": "cms-data", "owner": "einstein", "status": "creating"})
	if res["storage_id"] != "eosproject-c" || res["status"] != "creating" || res["path"] != "/eos/project/c/cms-data" {
		t.Fatalf("derived storage: %v", res)
	}
	for _, args := range []map[string]any{
		{"name": "dummy", "owner": "einstein"},                      // no sibling under d, no storage_id
		{"name": "cernbox", "owner": "einstein", "storage_id": "x"}, // duplicate name
		{"name": "x/y", "owner": "einstein", "storage_id": "x"},     // invalid name
		{"name": "zzz", "owner": "nobody", "storage_id": "x"},       // unknown owner
		{"name": "zzz", "owner": "einstein", "storage_id": "x", "status": "deleted"},
	} {
		if _, err := s.Invoke(adminContext(), "create", args); err == nil {
			t.Errorf("create %v must fail", args)
		}
	}

	if got := names(t, mustInvoke(t, s, "list", map[string]any{})); len(got) != 2 || got[0] != "cernbox" || got[1] != "cms-data" {
		t.Fatalf("list: %v", got)
	}
	if got := names(t, mustInvoke(t, s, "list", map[string]any{"status": "creating"})); len(got) != 1 || got[0] != "cms-data" {
		t.Fatalf("list by status: %v", got)
	}
	if got := names(t, mustInvoke(t, s, "list", map[string]any{"owner": "cernboxsvc"})); len(got) != 1 || got[0] != "cernbox" {
		t.Fatalf("list by owner: %v", got)
	}
	tomorrow := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	if got := names(t, mustInvoke(t, s, "list", map[string]any{"created_after": tomorrow})); len(got) != 0 {
		t.Fatalf("list created in the future: %v", got)
	}
	if got := names(t, mustInvoke(t, s, "list", map[string]any{"created_before": tomorrow})); len(got) != 2 {
		t.Fatalf("list created before tomorrow: %v", got)
	}
	if _, err := s.Invoke(adminContext(), "list", map[string]any{"status": "bogus"}); err == nil {
		t.Fatal("an unknown status must be rejected")
	}
	if got := mustInvoke(t, s, "get", map[string]any{"name": "cernbox"}); got["owner"] != "cernboxsvc" {
		t.Fatalf("get: %v", got)
	}
	if _, err := s.Invoke(adminContext(), "get", map[string]any{"name": "nope"}); err == nil {
		t.Fatal("an unknown project must be reported")
	}
}

func TestStatusOwnerDelete(t *testing.T) {
	s := newService(t)
	mustInvoke(t, s, "create", map[string]any{"name": "cernbox", "owner": "cernboxsvc", "storage_id": "eosproject-c"})

	if _, err := s.Invoke(adminContext(), "set_status", map[string]any{"name": "cernbox", "status": "gone"}); err == nil {
		t.Fatal("an unknown status must be rejected")
	}
	if res := mustInvoke(t, s, "set_status", map[string]any{"name": "cernbox", "status": "archiving"}); res["status"] != "archiving" || res["archived_at"] != nil {
		t.Fatalf("archiving: %v", res)
	}
	if _, err := s.Invoke(adminContext(), "set_owner", map[string]any{"name": "cernbox", "owner": "nobody"}); err == nil {
		t.Fatal("an unknown owner must be rejected")
	}
	if res := mustInvoke(t, s, "set_owner", map[string]any{"name": "cernbox", "owner": "einstein"}); res["owner"] != "einstein" {
		t.Fatalf("set_owner: %v", res)
	}
	// archiving hides the project from the default listing
	_, err := s.Invoke(adminContext(), "set_status", map[string]any{"name": "cernbox", "status": "archived"})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(t, mustInvoke(t, s, "list", map[string]any{})); len(got) != 0 {
		t.Fatalf("archived project listed: %v", got)
	}
	got := mustInvoke(t, s, "list", map[string]any{"include_archived": "true"})
	if ps := got["projects"].([]invoke.Result); len(ps) != 1 || ps[0]["archived_at"] == nil || ps[0]["status"] != "archived" {
		t.Fatalf("archived project: %v", ps)
	}

	mustInvoke(t, s, "create", map[string]any{"name": "atlas", "owner": "einstein", "storage_id": "eosproject-a"})
	if res := mustInvoke(t, s, "delete", map[string]any{"name": "atlas"}); res["hard"] != false {
		t.Fatalf("delete: %v", res)
	}
	if _, err := s.Invoke(adminContext(), "get", map[string]any{"name": "atlas"}); err == nil {
		t.Fatal("a deleted project must not be found")
	}
	if _, err := s.Invoke(adminContext(), "delete", map[string]any{"name": "atlas"}); err == nil {
		t.Fatal("deleting it again must be reported")
	}
	// the name stays taken until the row is hard-deleted
	if _, err := s.Invoke(adminContext(), "create", map[string]any{"name": "atlas", "owner": "einstein", "storage_id": "eosproject-a"}); err == nil {
		t.Fatal("a soft-deleted name must still be taken")
	}
	if res := mustInvoke(t, s, "delete", map[string]any{"name": "atlas", "hard": "true"}); res["hard"] != true {
		t.Fatalf("hard delete: %v", res)
	}
	mustInvoke(t, s, "create", map[string]any{"name": "atlas", "owner": "einstein", "storage_id": "eosproject-a"})
	if res := mustInvoke(t, s, "flush_cache", nil); res["flushed"] != true {
		t.Fatalf("flush: %v", res)
	}
}

func TestUnsupportedDriver(t *testing.T) {
	s := &service{c: &config{Driver: "memory"}, projects: stubCatalogue{}}
	s.registerInvocations()
	if _, err := s.Invoke(adminContext(), "list", map[string]any{}); err == nil {
		t.Fatal("a driver without the admin surface must be reported")
	}
}

type stubCatalogue struct{ projects.Catalogue }
