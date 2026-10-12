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

package auth

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/credential/loader"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/token/loader"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/tokenwriter/loader"
	"github.com/cs3org/reva/v3/pkg/appctx"
	authcache "github.com/cs3org/reva/v3/pkg/auth/cache"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/service"
	jwt "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	_ "github.com/cs3org/reva/v3/pkg/token/manager/loader"
	"google.golang.org/grpc"
)

const testJWTSecret = "auth-middleware-test-secret"

// fakeGateway authenticates the app passwords it knows and records every
// authentication request.
type fakeGateway struct {
	gateway.GatewayAPIClient
	mu        sync.Mutex
	passwords map[string]scope.AppAuthScope
	requests  []*gateway.AuthenticateRequest
}

func (g *fakeGateway) Authenticate(ctx context.Context, req *gateway.AuthenticateRequest, _ ...grpc.CallOption) (*gateway.AuthenticateResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, req)
	ap, ok := g.passwords[req.ClientSecret]
	if req.Type != "appauth" || !ok {
		return &gateway.AuthenticateResponse{Status: &rpc.Status{Code: rpc.Code_CODE_UNAUTHENTICATED}}, nil
	}
	scopes, err := scope.AddAppAuthScope(&ap, nil)
	if err != nil {
		return nil, err
	}
	mgr, err := jwt.New(map[string]any{"secret": testJWTSecret})
	if err != nil {
		return nil, err
	}
	u := &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}, Username: "einstein"}
	tkn, err := mgr.MintToken(ctx, u, scopes)
	if err != nil {
		return nil, err
	}
	return &gateway.AuthenticateResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: u, Token: tkn}, nil
}

func (g *fakeGateway) authentications() []*gateway.AuthenticateRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]*gateway.AuthenticateRequest(nil), g.requests...)
}

func (g *fakeGateway) revoke(secret string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.passwords, secret)
}

type testResolver struct {
	service.Clients
	mu sync.Mutex
	gw gateway.GatewayAPIClient
}

func (r *testResolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gw, nil
}

var (
	resolver     = &testResolver{}
	resolverOnce sync.Once
)

// App passwords of every kind, 43 characters without a dot like the real ones.
var (
	syncPassword   = "syncsyncsyncsyncsyncsyncsyncsyncsyncsyncsyn"
	cliPassword    = "clicliclicliclicliclicliclicliclicliclicliX"
	webdavPassword = "webdavwebdavwebdavwebdavwebdavwebdavwebdavw"
)

func newAppAuthGateway() *fakeGateway {
	return &fakeGateway{passwords: map[string]scope.AppAuthScope{
		syncPassword:   {PasswordID: "p-sync", Client: scope.ClientSync, Permissions: []string{scope.PermAll}},
		cliPassword:    {PasswordID: "p-cli", Client: scope.ClientCLI, Permissions: scope.ReadWritePermissions()},
		webdavPassword: {PasswordID: "p-webdav", Client: scope.ClientWebDAV, Permissions: scope.ReadOnlyPermissions()},
	}}
}

// newAppAuthMiddleware returns the middleware around a handler that
// answers 200, and the gateway it authenticates against.
func newAppAuthMiddleware(t *testing.T, conf map[string]any) (http.Handler, *fakeGateway) {
	t.Helper()
	gw := newAppAuthGateway()
	resolverOnce.Do(func() { service.SetGlobal(resolver) })
	resolver.mu.Lock()
	resolver.gw = gw
	resolver.mu.Unlock()

	m := map[string]any{
		"token_managers":   map[string]any{"jwt": map[string]any{"secret": testJWTSecret}},
		"credential_chain": []string{"appauth", "basic", "bearer"},
	}
	maps.Copy(m, conf)
	mw, err := New(m)
	if err != nil {
		t.Fatalf("new middleware: %v", err)
	}
	return mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := appctx.ContextGetUser(r.Context()); !ok {
			t.Error("the handler must see the user")
		}
		w.WriteHeader(http.StatusOK)
	})), gw
}

type request struct {
	path, ua      string
	basic, bearer string
}

func serve(h http.Handler, req request) int {
	r := httptest.NewRequest(http.MethodGet, "https://cernbox.example.org"+req.path, nil)
	r.Header.Set("User-Agent", req.ua)
	if req.basic != "" {
		r.SetBasicAuth("einstein", req.basic)
	}
	if req.bearer != "" {
		r.Header.Set("Authorization", "Bearer "+req.bearer)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

const (
	mirall = "Mozilla/5.0 (Linux) mirall/3.16.0 (Nextcloud)"
	webdav = "/remote.php/webdav/thesis"
)

func TestAppAuthBearerRouting(t *testing.T) {
	h, gw := newAppAuthMiddleware(t, nil)

	if code := serve(h, request{path: webdav, ua: "rclone/v1.66", bearer: webdavPassword}); code != http.StatusOK {
		t.Fatalf("app password as bearer: got %d", code)
	}
	reqs := gw.authentications()
	if len(reqs) != 1 || reqs[0].Type != "appauth" || reqs[0].ClientId != "" || reqs[0].ClientSecret != webdavPassword {
		t.Fatalf("app password as bearer: authenticated as %+v", reqs)
	}

	// A JWT, such as an OIDC access token, goes to the bearer strategy.
	if code := serve(h, request{path: webdav, ua: "rclone/v1.66", bearer: "aaa.bbb.ccc"}); code != http.StatusUnauthorized {
		t.Fatalf("JWT as bearer: got %d", code)
	}
	if reqs := gw.authentications(); len(reqs) != 2 || reqs[1].Type != "bearer" {
		t.Fatalf("JWT as bearer: authenticated as %+v", reqs[1:])
	}

	// With basic auth, an app password goes to appauth with the username, and a
	// regular password to the basic strategy.
	if code := serve(h, request{path: webdav, ua: mirall, basic: syncPassword}); code != http.StatusOK {
		t.Fatalf("app password with basic auth: got %d", code)
	}
	if reqs := gw.authentications(); len(reqs) != 3 || reqs[2].Type != "appauth" || reqs[2].ClientId != "einstein" {
		t.Fatalf("app password with basic auth: authenticated as %+v", reqs[2:])
	}
	if code := serve(h, request{path: webdav, ua: mirall, basic: "relativity"}); code != http.StatusUnauthorized {
		t.Fatalf("regular password: got %d", code)
	}
	if reqs := gw.authentications(); len(reqs) != 4 || reqs[3].Type != "basic" {
		t.Fatalf("regular password: authenticated as %+v", reqs[3:])
	}
}

func TestAppAuthDefaultChain(t *testing.T) {
	h, gw := newAppAuthMiddleware(t, map[string]any{"credential_chain": nil})
	if code := serve(h, request{path: webdav, ua: "rclone/v1.66", bearer: webdavPassword}); code != http.StatusOK {
		t.Fatalf("the default chain must accept an app password as bearer, got %d", code)
	}
	if reqs := gw.authentications(); len(reqs) != 1 || reqs[0].Type != "appauth" {
		t.Fatalf("authenticated as %+v", reqs)
	}
}

func TestAppAuthClientRules(t *testing.T) {
	h, _ := newAppAuthMiddleware(t, nil)
	cases := []struct {
		name string
		req  request
		want int
	}{
		{"sync, basic", request{path: webdav, ua: mirall, basic: syncPassword}, http.StatusOK},
		{"sync, bearer", request{path: webdav, ua: mirall, bearer: syncPassword}, http.StatusUnauthorized},
		{"sync, unexpected client", request{path: webdav, ua: "curl/8.0", basic: syncPassword}, http.StatusUnauthorized},
		{"cli, bearer", request{path: webdav, ua: "cernbox-cli/1.2.0", bearer: cliPassword}, http.StatusOK},
		{"cli, basic", request{path: webdav, ua: "cernbox-cli/1.2.0", basic: cliPassword}, http.StatusUnauthorized},
		{"cli, unexpected client", request{path: webdav, ua: "curl/8.0", bearer: cliPassword}, http.StatusUnauthorized},
		{"webdav, basic", request{path: webdav, ua: "rclone/v1.66", basic: webdavPassword}, http.StatusOK},
		{"webdav, bearer", request{path: webdav, ua: "curl/8.0", bearer: webdavPassword}, http.StatusOK},
	}
	for _, c := range cases {
		// Twice: the second answer comes from the cache and must not differ.
		for range 2 {
			if got := serve(h, c.req); got != c.want {
				t.Errorf("%s: got %d, want %d", c.name, got, c.want)
			}
		}
	}
}

func TestAppAuthClientOverrides(t *testing.T) {
	h, _ := newAppAuthMiddleware(t, map[string]any{"appauth_clients": map[string]any{
		"cli":  map[string]any{"strict_user_agent": false},
		"sync": map[string]any{"user_agents": []string{"CERNBox-iOS/"}},
	}})
	cases := []struct {
		name string
		req  request
		want int
	}{
		{"cli, soft", request{path: webdav, ua: "curl/8.0", bearer: cliPassword}, http.StatusOK},
		{"sync, listed client", request{path: webdav, ua: "CERNBox-iOS/1.4", basic: syncPassword}, http.StatusOK},
		{"sync, replaced client", request{path: webdav, ua: mirall, basic: syncPassword}, http.StatusUnauthorized},
	}
	for _, c := range cases {
		if got := serve(h, c.req); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}

	if _, err := New(map[string]any{"appauth_clients": map[string]any{"printer": map[string]any{}}}); err == nil {
		t.Error("an unknown client kind must be refused")
	}
}

// An app password is answered from the cache for a short time only, so that
// a revoked one stops working soon.
func TestAppAuthCacheTTL(t *testing.T) {
	h, gw := newAppAuthMiddleware(t, map[string]any{"appauth_cache_ttl_seconds": 1})
	req := request{path: webdav, ua: "curl/8.0", bearer: webdavPassword}
	for range 3 {
		if code := serve(h, req); code != http.StatusOK {
			t.Fatalf("got %d", code)
		}
	}
	if n := len(gw.authentications()); n != 1 {
		t.Fatalf("expected one authentication, got %d", n)
	}
	time.Sleep(1100 * time.Millisecond)
	gw.revoke(webdavPassword)
	if code := serve(h, req); code != http.StatusUnauthorized {
		t.Fatalf("a revoked app password must be refused once its entry expires, got %d", code)
	}
}

func TestAppAuthRevocationForgetsCache(t *testing.T) {
	h, gw := newAppAuthMiddleware(t, nil)
	req := request{path: webdav, ua: "curl/8.0", bearer: webdavPassword}
	if code := serve(h, req); code != http.StatusOK {
		t.Fatalf("got %d", code)
	}
	gw.revoke(webdavPassword)
	if code := serve(h, req); code != http.StatusOK {
		t.Fatalf("still cached: got %d", code)
	}
	authcache.Forget(func(id *authcache.Identity) bool {
		ap, ok := scope.GetAppAuthScope(id.Scopes)
		return ok && ap.PasswordID == "p-webdav"
	})
	if code := serve(h, req); code != http.StatusUnauthorized {
		t.Fatalf("a forgotten app password must be authenticated again, got %d", code)
	}
}

func TestAppAuthFailuresThrottled(t *testing.T) {
	h, gw := newAppAuthMiddleware(t, map[string]any{"credential_strategies": map[string]any{
		"appauth": map[string]any{"failures_per_ip_per_min": 2},
	}})
	valid := request{path: webdav, ua: "curl/8.0", bearer: webdavPassword}
	if code := serve(h, valid); code != http.StatusOK {
		t.Fatalf("got %d", code)
	}

	// Wrong, but shaped like app passwords.
	wrong1, wrong2, wrong3 := strings.Repeat("A", 43), strings.Repeat("B", 43), strings.Repeat("C", 43)
	wrong := func(s string) request { return request{path: webdav, ua: "curl/8.0", bearer: s} }
	for _, s := range []string{wrong1, wrong2} {
		if code := serve(h, wrong(s)); code != http.StatusUnauthorized {
			t.Fatalf("%s: got %d", s, code)
		}
	}
	// A wrong app password already answered from the cache costs nothing.
	if code := serve(h, wrong(wrong1)); code != http.StatusUnauthorized {
		t.Fatalf("cached wrong1: got %d", code)
	}
	n := len(gw.authentications())
	if code := serve(h, wrong(wrong3)); code != http.StatusTooManyRequests {
		t.Fatalf("wrong3: got %d", code)
	}
	if len(gw.authentications()) != n {
		t.Fatal("a throttled client must not reach the gateway")
	}
	// The app passwords it already used keep working.
	if code := serve(h, valid); code != http.StatusOK {
		t.Fatalf("cached valid password: got %d", code)
	}
}
