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
	"net/http"
	"net/http/httptest"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"google.golang.org/grpc"
)

type usersGateway struct {
	gateway.GatewayAPIClient
	called bool
}

func (g *usersGateway) FindUsers(_ context.Context, _ *userpb.FindUsersRequest, _ ...grpc.CallOption) (*userpb.FindUsersResponse, error) {
	g.called = true
	return &userpb.FindUsersResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
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
