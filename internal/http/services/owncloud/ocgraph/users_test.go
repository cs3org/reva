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

package ocgraph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"google.golang.org/grpc"
)

// usersGateway is a fake gateway that models the real FindUsers backend: it
// honours the usertype filters it receives (AND-combined, one type per filter),
// so tests exercise the true end-to-end behaviour of the endpoint rather than
// its internal filter plumbing.
type usersGateway struct {
	gateway.GatewayAPIClient
	called bool
	users  []*userpb.User
}

func (g *usersGateway) FindUsers(_ context.Context, req *userpb.FindUsersRequest, _ ...grpc.CallOption) (*userpb.FindUsersResponse, error) {
	g.called = true
	out := make([]*userpb.User, 0, len(g.users))
	for _, u := range g.users {
		keep := true
		for _, f := range req.GetFilters() {
			if f.GetType() == userpb.Filter_TYPE_USERTYPE && u.GetId().GetType() != f.GetUsertype() {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, u)
		}
	}
	return &userpb.FindUsersResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Users: out}, nil
}

func mkUser(name string, t userpb.UserType) *userpb.User {
	return &userpb.User{
		Id:          &userpb.UserId{OpaqueId: name, Type: t},
		Username:    name,
		DisplayName: name,
	}
}

func listUsersURL(filter string) string {
	q := url.Values{}
	q.Set("$search", `"al"`)
	q.Set("$orderby", "displayName")
	if filter != "" {
		q.Set("$filter", filter)
	}
	return "/users?" + q.Encode()
}

func decodeUserIDs(t *testing.T, body []byte) []string {
	t.Helper()
	var resp struct {
		Value []libregraph.User `json:"value"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decoding response: %v; body: %s", err, body)
	}
	ids := make([]string, 0, len(resp.Value))
	for _, u := range resp.Value {
		ids = append(ids, u.GetId())
	}
	sort.Strings(ids)
	return ids
}

// TestListUsersRejectsMissingQueryParameters guards the nil-pointer crash: a
// request without $search or without $orderby must return 400, not panic, and
// must not reach the gateway.
func TestListUsersRejectsMissingQueryParameters(t *testing.T) {
	tests := []struct {
		name     string
		rawQuery string
		wantCode int
		wantCall bool
	}{
		{"missing search", `$orderby=displayName`, http.StatusBadRequest, false},
		{"search too short", `$search=a&$orderby=displayName`, http.StatusBadRequest, false},
		{"missing orderby", `$search=%22alice%22`, http.StatusBadRequest, false},
		{"valid search and orderby", `$search=%22alice%22&$orderby=displayName`, http.StatusOK, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := "ocgraph-users-" + t.Name()
			gw := &usersGateway{}
			pool.RegisterGatewayServiceClient(gw, endpoint)
			s := &svc{c: &config{GatewaySvc: endpoint}}

			req := httptest.NewRequest(http.MethodGet, "/users?"+tt.rawQuery, nil)
			rec := httptest.NewRecorder()

			s.listUsers(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if gw.called != tt.wantCall {
				t.Fatalf("gateway FindUsers called = %v, want %v", gw.called, tt.wantCall)
			}
		})
	}
}

// TestListUsersFilterByType checks the userType selection: the default is
// primary-only, a single type filters to that type, several types joined by `or`
// return their union, and `all` returns every type.
func TestListUsersFilterByType(t *testing.T) {
	users := []*userpb.User{
		mkUser("alice", userpb.UserType_USER_TYPE_PRIMARY),
		mkUser("bob", userpb.UserType_USER_TYPE_GUEST),
		mkUser("carol", userpb.UserType_USER_TYPE_LIGHTWEIGHT),
		mkUser("dave", userpb.UserType_USER_TYPE_SECONDARY),
	}

	tests := []struct {
		name    string
		filter  string
		wantIDs []string
	}{
		{"default is primary only", "", []string{"alice"}},
		{"single type", "userType eq 'guest'", []string{"bob"}},
		{"two types via or", "userType eq 'primary' or userType eq 'guest'", []string{"alice", "bob"}},
		{"three types via or", "userType eq 'primary' or userType eq 'guest' or userType eq 'lightweight'", []string{"alice", "bob", "carol"}},
		{"all types", "userType eq 'all'", []string{"alice", "bob", "carol", "dave"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := "ocgraph-users-type-" + t.Name()
			gw := &usersGateway{users: users}
			pool.RegisterGatewayServiceClient(gw, endpoint)
			s := &svc{c: &config{GatewaySvc: endpoint}}

			req := httptest.NewRequest(http.MethodGet, listUsersURL(tt.filter), nil)
			rec := httptest.NewRecorder()

			s.listUsers(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
			}
			got := decodeUserIDs(t, rec.Body.Bytes())
			if !slices.Equal(got, tt.wantIDs) {
				t.Fatalf("returned users = %v, want %v", got, tt.wantIDs)
			}
		})
	}
}

// TestListUsersFilterErrors rejects malformed or unsupported $filter expressions
// with 400 and without reaching the gateway.
func TestListUsersFilterErrors(t *testing.T) {
	tests := []struct {
		name   string
		filter string
	}{
		{"unknown usertype", "userType eq 'bogus'"},
		{"unsupported field", "displayName eq 'alice'"},
		{"unsupported operand", "userType ne 'guest'"},
		{"unsupported and", "userType eq 'primary' and userType eq 'guest'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := "ocgraph-users-err-" + t.Name()
			gw := &usersGateway{users: []*userpb.User{mkUser("alice", userpb.UserType_USER_TYPE_PRIMARY)}}
			pool.RegisterGatewayServiceClient(gw, endpoint)
			s := &svc{c: &config{GatewaySvc: endpoint}}

			req := httptest.NewRequest(http.MethodGet, listUsersURL(tt.filter), nil)
			rec := httptest.NewRecorder()

			s.listUsers(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
			}
			if gw.called {
				t.Fatalf("gateway FindUsers should not be called on a bad filter")
			}
		})
	}
}
