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

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeAdmin struct {
	adminpb.UnimplementedAdminAPIServer
	adminToken string
	isAdmin    bool

	mu                                    sync.Mutex
	requestAdmin, checkAdmin, impersonate int
	impersonated                          *adminpb.ImpersonateRequest
	last                                  map[string]metadata.MD
}

func (f *fakeAdmin) record(ctx context.Context, method string) {
	md, _ := metadata.FromIncomingContext(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.last == nil {
		f.last = map[string]metadata.MD{}
	}
	f.last[method] = md
}

func (f *fakeAdmin) header(method, key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.last[method].Get(key), ",")
}

func (f *fakeAdmin) GetServerInfo(ctx context.Context, _ *adminpb.GetServerInfoRequest) (*adminpb.GetServerInfoResponse, error) {
	f.record(ctx, "GetServerInfo")
	return &adminpb.GetServerInfoResponse{}, nil
}

func (f *fakeAdmin) ListServices(ctx context.Context, _ *adminpb.ListServicesRequest) (*adminpb.ListServicesResponse, error) {
	f.record(ctx, "ListServices")
	return &adminpb.ListServicesResponse{}, nil
}

func (f *fakeAdmin) RequestAdmin(ctx context.Context, _ *adminpb.RequestAdminRequest) (*adminpb.RequestAdminResponse, error) {
	f.record(ctx, "RequestAdmin")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requestAdmin++
	// A wrong expires_at: the client must read the exp of the token.
	return &adminpb.RequestAdminResponse{Token: f.adminToken, ExpiresAt: time.Now().Add(time.Hour).Unix()}, nil
}

func (f *fakeAdmin) CheckAdmin(ctx context.Context, _ *adminpb.CheckAdminRequest) (*adminpb.CheckAdminResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkAdmin++
	return &adminpb.CheckAdminResponse{Admin: f.isAdmin}, nil
}

func (f *fakeAdmin) Impersonate(ctx context.Context, req *adminpb.ImpersonateRequest) (*adminpb.ImpersonateResponse, error) {
	f.record(ctx, "Impersonate")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.impersonate++
	f.impersonated = req
	return &adminpb.ImpersonateResponse{Token: "impersonated-" + req.User}, nil
}

func serve(t *testing.T, lis net.Listener, f adminpb.AdminAPIServer) {
	s := grpc.NewServer()
	adminpb.RegisterAdminAPIServer(s, f)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
}

func serveTCP(t *testing.T, f adminpb.AdminAPIServer) string {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serve(t, lis, f)
	return lis.Addr().String()
}

func token(t *testing.T, user string, exp time.Time) string {
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": exp.Unix(), "user": map[string]string{"username": user},
	}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// setup isolates the config dir and the socket paths of the test.
func setup(t *testing.T) string {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("SUDO_USER", "")
	old := socketPaths
	socketPaths = func() []string { return nil }
	t.Cleanup(func() { socketPaths = old })
	return dir
}

func adminTokenFile(dir string) string { return filepath.Join(dir, ".config", "reva", "admin-token") }

func TestAdminPrefersTheSocket(t *testing.T) {
	dir := setup(t)
	network := &fakeAdmin{adminToken: token(t, "alice", time.Now().Add(time.Hour))}
	host := serveTCP(t, network)

	sock := filepath.Join(dir, "admin.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	local := &fakeAdmin{}
	serve(t, lis, local)
	socketPaths = func() []string { return []string{filepath.Join(dir, "missing.sock"), sock} }

	login := token(t, "alice", time.Now().Add(time.Hour))
	rt := &Runtime{adminHost: host, insecure: true, tokenValue: login}
	defer rt.Close()
	c, ctx, err := rt.Admin(WithToken(context.Background(), login))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Socket {
		t.Fatal("the socket is not used")
	}
	if _, err := c.ListServices(ctx, &adminpb.ListServicesRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := local.header("ListServices", appctx.TokenHeader); got != "" {
		t.Errorf("a token went to the socket: %q", got)
	}
	if network.requestAdmin != 0 {
		t.Errorf("step-up with a working socket: %d", network.requestAdmin)
	}
	if got := rt.AdminState(context.Background()); got != "socket" {
		t.Errorf("admin state = %q", got)
	}
}

func TestStepUpStaysInMemory(t *testing.T) {
	dir := setup(t)
	f := &fakeAdmin{adminToken: token(t, "alice", time.Now().Add(15*time.Minute))}
	rt := &Runtime{adminHost: serveTCP(t, f), insecure: true, tokenValue: token(t, "alice", time.Now().Add(time.Hour))}
	defer rt.Close()

	if got := rt.AdminState(context.Background()); got != "no" {
		t.Errorf("admin state before the step-up = %q", got)
	}
	for range 2 {
		c, ctx, err := rt.Admin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.ListServices(ctx, &adminpb.ListServicesRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if f.requestAdmin != 1 {
		t.Errorf("step-ups = %d, want 1", f.requestAdmin)
	}
	if got := f.header("ListServices", appctx.TokenHeader); got != f.adminToken {
		t.Errorf("token of the admin call = %q, want the admin token", got)
	}
	if _, err := os.Stat(adminTokenFile(dir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the automatic step-up wrote the admin token to disk: %v", err)
	}
	if got := rt.AdminState(context.Background()); got != "yes, 15m left" {
		t.Errorf("admin state = %q", got)
	}
}

func TestOnlyElevatePersists(t *testing.T) {
	dir := setup(t)
	f := &fakeAdmin{adminToken: token(t, "alice", time.Now().Add(15*time.Minute))}
	host := serveTCP(t, f)
	login := token(t, "alice", time.Now().Add(time.Hour))

	rt := &Runtime{adminHost: host, insecure: true, tokenValue: login}
	exp, err := rt.Elevate(context.Background())
	rt.Close()
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(exp) > 16*time.Minute {
		t.Errorf("expiry %s comes from expires_at, not from the token", exp)
	}
	if _, err := os.Stat(adminTokenFile(dir)); err != nil {
		t.Fatalf("elevate did not store the admin token: %v", err)
	}

	// The same login uses the stored token.
	rt = &Runtime{adminHost: host, insecure: true, tokenValue: login}
	if _, _, err := rt.Admin(context.Background()); err != nil {
		t.Fatal(err)
	}
	rt.Close()
	if f.requestAdmin != 1 {
		t.Errorf("step-ups = %d, want only the one of elevate", f.requestAdmin)
	}

	// Another login does not.
	rt = &Runtime{adminHost: host, insecure: true, tokenValue: token(t, "bob", time.Now().Add(2*time.Hour))}
	if _, _, err := rt.Admin(context.Background()); err != nil {
		t.Fatal(err)
	}
	rt.Close()
	if f.requestAdmin != 2 {
		t.Errorf("step-ups = %d, want a new one for another login", f.requestAdmin)
	}
}

func TestExpiredStoredAdminTokenIsNotUsed(t *testing.T) {
	setup(t)
	f := &fakeAdmin{adminToken: token(t, "alice", time.Now().Add(-time.Minute))}
	host := serveTCP(t, f)
	login := token(t, "alice", time.Now().Add(time.Hour))
	rt := &Runtime{adminHost: host, insecure: true, tokenValue: login}
	if _, err := rt.Elevate(context.Background()); err != nil {
		t.Fatal(err)
	}
	rt.Close()
	rt = &Runtime{adminHost: host, insecure: true, tokenValue: login}
	defer rt.Close()
	if got := rt.AdminState(context.Background()); got != "no" {
		t.Errorf("admin state with an expired token = %q", got)
	}
	if _, _, err := rt.Admin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.requestAdmin != 2 {
		t.Errorf("step-ups = %d, want a new one", f.requestAdmin)
	}
}

func TestNoElevate(t *testing.T) {
	setup(t)
	f := &fakeAdmin{adminToken: token(t, "alice", time.Now().Add(time.Hour))}
	_, routeAdd, _ := tree()
	rt := &Runtime{adminHost: serveTCP(t, f), insecure: true, tokenValue: token(t, "alice", time.Now().Add(time.Hour)), noElevate: true}
	defer rt.Close()
	if err := rt.SelectPlane(routeAdd); err != nil {
		t.Fatal(err)
	}
	_, _, err := rt.Admin(context.Background())
	if err == nil || !strings.Contains(err.Error(), "`reva route add` needs admin rights") {
		t.Fatalf("err = %v, want one that names the command", err)
	}
	if f.requestAdmin != 0 {
		t.Errorf("step-up with --no-elevate")
	}
}

func TestAdminHeaders(t *testing.T) {
	setup(t)
	t.Setenv("SUDO_USER", "gdelmont")
	f := &fakeAdmin{adminToken: token(t, "alice", time.Now().Add(time.Hour))}
	rt := &Runtime{adminHost: serveTCP(t, f), insecure: true, tokenValue: token(t, "alice", time.Now().Add(time.Hour)), reason: "INC0042"}
	defer rt.Close()
	c, ctx, err := rt.Admin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListServices(ctx, &adminpb.ListServicesRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"RequestAdmin", "ListServices"} {
		if got := f.header(m, admin.ReasonHeader); got != "INC0042" {
			t.Errorf("%s: reason = %q", m, got)
		}
		if got := f.header(m, admin.SudoUserHeader); got != "gdelmont" {
			t.Errorf("%s: sudo user = %q", m, got)
		}
	}
}

func TestAs(t *testing.T) {
	setup(t)
	f := &fakeAdmin{adminToken: token(t, "alice", time.Now().Add(time.Hour)), isAdmin: true}
	login := token(t, "alice", time.Now().Add(time.Hour))
	rt := &Runtime{adminHost: serveTCP(t, f), insecure: true, tokenValue: login, as: "bob", reason: "INC0042"}
	defer rt.Close()
	for range 2 {
		got, err := rt.Token(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got != "impersonated-bob" {
			t.Fatalf("user-plane token = %q", got)
		}
	}
	if f.impersonate != 1 || f.impersonated.User != "bob" || f.impersonated.Reason != "INC0042" {
		t.Errorf("impersonations = %d, request = %v", f.impersonate, f.impersonated)
	}
	if got := f.header("Impersonate", admin.ReasonHeader); got != "INC0042" {
		t.Errorf("reason header of Impersonate = %q", got)
	}
	if got := f.header("Impersonate", appctx.TokenHeader); got != f.adminToken {
		t.Errorf("Impersonate without the admin token: %q", got)
	}
	if got, _ := rt.loginToken(context.Background()); got != login {
		t.Errorf("the login token changed")
	}
	if rt.CanAdmin(context.Background()) || f.checkAdmin != 0 {
		t.Errorf("the owner fallback is on under --as")
	}
	rt.as = ""
	if !rt.CanAdmin(context.Background()) {
		t.Errorf("an admin cannot be admin")
	}
}

func TestCanAdminAsksTheServer(t *testing.T) {
	setup(t)
	f := &fakeAdmin{}
	rt := &Runtime{adminHost: serveTCP(t, f), insecure: true, tokenValue: token(t, "alice", time.Now().Add(time.Hour))}
	defer rt.Close()
	if rt.CanAdmin(context.Background()) {
		t.Error("a non-admin can be admin")
	}
	if f.checkAdmin != 1 || f.requestAdmin != 0 {
		t.Errorf("CheckAdmin calls = %d, step-ups = %d", f.checkAdmin, f.requestAdmin)
	}
}

func TestImpersonateHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case r.URL.Path != "/admin/impersonate":
			http.NotFound(w, r)
		case r.Header.Get("x-access-token") != "login":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"no token"}`))
		case body["user"] == "nobody":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"no such user"}`))
		case body["reason"] != "INC0042":
			w.WriteHeader(http.StatusBadRequest)
		default:
			_, _ = w.Write([]byte(`{"token":"impersonated-` + body["user"] + `"}`))
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	if got, err := ImpersonateHTTP(ctx, srv.Client(), srv.URL, "login", "bob", "INC0042"); err != nil || got != "impersonated-bob" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := ImpersonateHTTP(ctx, srv.Client(), srv.URL, "login", "nobody", "INC0042"); err == nil || !strings.Contains(err.Error(), "no such user") {
		t.Errorf("unknown user: %v", err)
	}
	if _, err := ImpersonateHTTP(ctx, srv.Client(), srv.URL+"/other", "login", "bob", "INC0042"); err == nil || !strings.Contains(err.Error(), "does not expose") {
		t.Errorf("not exposed: %v", err)
	}
}

func TestConfirmWithoutTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	_, _ = w.WriteString("y\n")
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })

	if err := (&Runtime{}).Confirm("remove"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("a script confirmed without --yes: %v", err)
	}
	if err := (&Runtime{yes: true}).Confirm("remove"); err != nil {
		t.Errorf("--yes: %v", err)
	}
	if err := (&Runtime{as: "bob"}).ConfirmMutation("reva rm /x"); err == nil {
		t.Errorf("a mutation under --as ran without a confirmation")
	}
	if err := (&Runtime{}).ConfirmMutation("reva rm /x"); err != nil {
		t.Errorf("a mutation without --as asked: %v", err)
	}
}
