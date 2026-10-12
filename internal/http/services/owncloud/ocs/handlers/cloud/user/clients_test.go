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

package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	appauthpb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/v3/pkg/appauth"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// fakeGateway keeps the app passwords of one user.
type fakeGateway struct {
	gateway.GatewayAPIClient
	mu        sync.Mutex
	passwords []*appauthpb.AppPassword
	creators  map[string]string
	revoked   []string
	err       error
}

func (g *fakeGateway) ListAppPasswords(context.Context, *appauthpb.ListAppPasswordsRequest, ...grpc.CallOption) (*appauthpb.ListAppPasswordsResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err != nil {
		return nil, g.err
	}
	val, _ := json.Marshal(g.creators)
	return &appauthpb.ListAppPasswordsResponse{
		Status:       &rpc.Status{Code: rpc.Code_CODE_OK},
		AppPasswords: g.passwords,
		Opaque:       &typespb.Opaque{Map: map[string]*typespb.OpaqueEntry{appauth.OpaqueCreatedBy: {Decoder: "json", Value: val}}},
	}, nil
}

func (g *fakeGateway) InvalidateAppPassword(_ context.Context, req *appauthpb.InvalidateAppPasswordRequest, _ ...grpc.CallOption) (*appauthpb.InvalidateAppPasswordResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.revoked = append(g.revoked, req.Password)
	return &appauthpb.InvalidateAppPasswordResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

type resolver struct {
	service.Clients
	mu sync.Mutex
	gw *fakeGateway
}

func (r *resolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gw, nil
}

var testResolver = &resolver{}

func init() {
	service.SetGlobal(testResolver)
}

func clientsRouter(gw *fakeGateway) http.Handler {
	testResolver.mu.Lock()
	testResolver.gw = gw
	testResolver.mu.Unlock()
	h := &Handler{}
	mux := http.NewServeMux()
	v2 := response.VersionCtx("2")
	mux.Handle("GET /v2.php/cloud/user/clients", v2(http.HandlerFunc(h.ListClients)))
	mux.Handle("DELETE /v2.php/cloud/user/clients/{cid}", v2(http.HandlerFunc(h.DeleteClient)))
	return mux
}

func ocs(t *testing.T, h http.Handler, method, path, body string, data any) int {
	t.Helper()
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein-id"}, Username: "einstein"})
	r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if data != nil && w.Code == http.StatusOK {
		var res struct {
			OCS struct {
				Data json.RawMessage `json:"data"`
			} `json:"ocs"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("decode %s: %v", w.Body.String(), err)
		}
		if err := json.Unmarshal(res.OCS.Data, data); err != nil {
			t.Fatalf("decode data %s: %v", res.OCS.Data, err)
		}
	}
	return w.Code
}

const clientsPath = "/v2.php/cloud/user/clients?format=json"

func appPassword(t *testing.T, label, id, client string, perms ...string) *appauthpb.AppPassword {
	t.Helper()
	scopes, err := scope.AddAppAuthScope(&scope.AppAuthScope{PasswordID: id, Client: client, Permissions: perms}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &appauthpb.AppPassword{Password: "handle-" + id, Label: label, TokenScope: scopes, Ctime: &typespb.Timestamp{Seconds: 1}, Utime: &typespb.Timestamp{Seconds: 2}}
}

func TestListAndDeleteClients(t *testing.T) {
	gw := &fakeGateway{
		passwords: []*appauthpb.AppPassword{
			appPassword(t, "laptop|Nextcloud Sync Client v3.16.0|client-1", "pw-sync", scope.ClientSync, scope.PermAll),
			appPassword(t, "script", "pw-webdav", scope.ClientWebDAV, scope.PermFilesList),
		},
		creators: map[string]string{"pw-sync": "einstein-id", "pw-webdav": "admin-id"},
	}
	h := clientsRouter(gw)

	var clients []Client
	if code := ocs(t, h, http.MethodGet, clientsPath, "", &clients); code != http.StatusOK || len(clients) != 2 {
		t.Fatalf("got %d, %+v", code, clients)
	}
	sync, script := clients[0], clients[1]
	if sync.ID != "client-1" || sync.Name != "laptop" || sync.PasswordID != "pw-sync" || sync.Kind != "sync" || sync.CreatedBy != "einstein-id" {
		t.Errorf("sync client %+v", sync)
	}
	if script.ID != "pw-webdav" || script.Description != "script" || !slices.Equal(script.Permissions, []string{"files:list"}) || script.CreatedBy != "admin-id" {
		t.Errorf("script %+v", script)
	}

	// By the legacy client id, as the web UI does, and by password id.
	for _, id := range []string{"client-1", "pw-webdav", "unknown"} {
		if code := ocs(t, h, http.MethodDelete, "/v2.php/cloud/user/clients/"+id, "", nil); code != http.StatusNoContent {
			t.Errorf("delete %s: got %d", id, code)
		}
	}
	if !slices.Equal(gw.revoked, []string{"handle-pw-sync", "handle-pw-webdav"}) {
		t.Fatalf("revoked %v", gw.revoked)
	}
}

// An app password session is refused by the gateway, and told so.
func TestClientsRefusedToAppPasswords(t *testing.T) {
	refused := &fakeGateway{err: grpcstatus.Error(codes.PermissionDenied, "auth: core access token is invalid")}
	if got := ocs(t, clientsRouter(refused), http.MethodGet, clientsPath, "", nil); got != http.StatusForbidden {
		t.Errorf("got %d", got)
	}
}
