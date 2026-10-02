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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// testResolver is a process-wide service.Clients whose Admin client is
// swappable, so each test can install its own fake.
type testResolver struct {
	service.Clients
	mu    sync.Mutex
	admin adminpb.AdminAPIClient
}

func (r *testResolver) Admin(context.Context) (adminpb.AdminAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.admin, nil
}

var (
	globalTestResolver     = &testResolver{}
	globalTestResolverOnce sync.Once
)

func stampAdmin(c adminpb.AdminAPIClient) {
	globalTestResolverOnce.Do(func() { service.SetGlobal(globalTestResolver) })
	globalTestResolver.mu.Lock()
	globalTestResolver.admin = c
	globalTestResolver.mu.Unlock()
}

// fakeAdmin records the token each call carried, the way the gRPC server would
// read it: the first value of the token header.
type fakeAdmin struct {
	adminpb.AdminAPIClient
	isAdmin        bool
	requestErr     error
	impersonateErr error

	impersonateToken string
	impersonated     *adminpb.ImpersonateRequest
}

func firstToken(ctx context.Context) string {
	md, _ := metadata.FromOutgoingContext(ctx)
	if v := md.Get(appctx.TokenHeader); len(v) > 0 {
		return v[0]
	}
	return ""
}

func (f *fakeAdmin) CheckAdmin(context.Context, *adminpb.CheckAdminRequest, ...grpc.CallOption) (*adminpb.CheckAdminResponse, error) {
	return &adminpb.CheckAdminResponse{Admin: f.isAdmin}, nil
}

func (f *fakeAdmin) RequestAdmin(context.Context, *adminpb.RequestAdminRequest, ...grpc.CallOption) (*adminpb.RequestAdminResponse, error) {
	if f.requestErr != nil {
		return nil, f.requestErr
	}
	return &adminpb.RequestAdminResponse{Token: "admin-token", ExpiresAt: 100}, nil
}

func (f *fakeAdmin) Impersonate(ctx context.Context, req *adminpb.ImpersonateRequest, _ ...grpc.CallOption) (*adminpb.ImpersonateResponse, error) {
	f.impersonateToken = firstToken(ctx)
	f.impersonated = req
	if f.impersonateErr != nil {
		return nil, f.impersonateErr
	}
	return &adminpb.ImpersonateResponse{Token: "user-token-for-" + req.User}, nil
}

func newTestService(t *testing.T) http.Handler {
	t.Helper()
	s, err := New(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.Handler()
}

// serve runs a request as the HTTP auth middleware would leave it: the
// caller's own token already set for outgoing calls.
func serve(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := appctx.ContextSetToken(r.Context(), "caller-token")
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, "caller-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(ctx))
	return w
}

func TestStatus(t *testing.T) {
	h := newTestService(t)
	for _, want := range []bool{true, false} {
		stampAdmin(&fakeAdmin{isAdmin: want})
		w := serve(t, h, http.MethodGet, "/status", "")
		if w.Code != http.StatusOK {
			t.Fatalf("status: code %d, body %s", w.Code, w.Body)
		}
		var got statusResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if got.Admin != want {
			t.Errorf("admin = %v, want %v", got.Admin, want)
		}
	}
}

// TestImpersonateUsesAdminToken asserts that Impersonate carries the freshly
// minted admin token, not the caller's user token the middleware set first.
func TestImpersonateUsesAdminToken(t *testing.T) {
	h := newTestService(t)
	fake := &fakeAdmin{}
	stampAdmin(fake)

	w := serve(t, h, http.MethodPost, "/impersonate", `{"user":"marie","reason":"INC42"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d, body %s", w.Code, w.Body)
	}
	if fake.impersonateToken != "admin-token" {
		t.Errorf("Impersonate carried token %q, want the admin token", fake.impersonateToken)
	}
	if fake.impersonated.GetUser() != "marie" || fake.impersonated.GetReason() != "INC42" {
		t.Errorf("Impersonate got %v", fake.impersonated)
	}
	var got impersonateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got.Token != "user-token-for-marie" {
		t.Errorf("response = %+v", got)
	}
	if strings.Contains(w.Body.String(), "admin-token") {
		t.Error("the admin token must not leave the server")
	}
}

// TestImpersonateWithoutReason asserts the reason is optional.
func TestImpersonateWithoutReason(t *testing.T) {
	h := newTestService(t)
	fake := &fakeAdmin{}
	stampAdmin(fake)

	w := serve(t, h, http.MethodPost, "/impersonate", `{"user":"marie"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d, body %s", w.Code, w.Body)
	}
	if fake.impersonated.GetUser() != "marie" || fake.impersonated.GetReason() != "" {
		t.Errorf("Impersonate got %v", fake.impersonated)
	}
}

func TestImpersonateErrors(t *testing.T) {
	h := newTestService(t)
	cases := []struct {
		name string
		body string
		fake *fakeAdmin
		want int
	}{
		{"no user", `{"reason":"r"}`, &fakeAdmin{}, http.StatusBadRequest},
		{"bad json", `{`, &fakeAdmin{}, http.StatusBadRequest},
		{"not admin", `{"user":"marie","reason":"r"}`,
			&fakeAdmin{requestErr: status.Error(codes.PermissionDenied, "no")}, http.StatusForbidden},
		{"unauthenticated", `{"user":"marie","reason":"r"}`,
			&fakeAdmin{requestErr: status.Error(codes.Unauthenticated, "no")}, http.StatusUnauthorized},
		{"no such user", `{"user":"nobody","reason":"r"}`,
			&fakeAdmin{impersonateErr: status.Error(codes.NotFound, "no")}, http.StatusNotFound},
		{"not configured", `{"user":"marie","reason":"r"}`,
			&fakeAdmin{impersonateErr: status.Error(codes.FailedPrecondition, "no")}, http.StatusNotImplemented},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stampAdmin(tc.fake)
			w := serve(t, h, http.MethodPost, "/impersonate", tc.body)
			if w.Code != tc.want {
				t.Fatalf("code %d, want %d (body %s)", w.Code, tc.want, w.Body)
			}
			if !strings.Contains(w.Body.String(), `"message"`) {
				t.Errorf("error body carries no message: %s", w.Body)
			}
		})
	}
}
