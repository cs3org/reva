// Copyright 2018-2024 CERN
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

package sciencemesh

import (
	"bytes"
	"context"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/interceptors/auth"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/credential/strategy/bearer"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/token/strategy/bearer"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/tokenwriter/strategy/header"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	_ "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc"
)

func newLauncherRequest(t *testing.T, form url.Values) (*http.Request, *bytes.Buffer) {
	t.Helper()
	base, logs := newLaunchRequest(t, "")
	req := httptest.NewRequest(http.MethodPost, "/sciencemesh/webapp-launch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req.WithContext(base.Context()), logs
}

func launcherHandler(t *testing.T, appURI string) (*appsHandler, *observeClient) {
	t.Helper()
	obs := &observeClient{token: launchToken}
	share := receivedWebappShare(
		"https://dav.example/remote.php/dav/ocm/share",
		appURI,
		launchSecret,
		[]string{"must-exchange-token"},
	)
	gw := &fakeReceivedGateway{resp: okShareResponse(share)}
	return newTestHandler(t, gw, obs), obs
}

func assertLauncherHeaders(t *testing.T, rec *httptest.ResponseRecorder, wantFormAction string) string {
	t.Helper()
	csp := rec.Header().Get("Content-Security-Policy")
	m := regexp.MustCompile(`script-src 'nonce-([A-Za-z0-9_-]{16,})'`).FindStringSubmatch(csp)
	if m == nil {
		t.Fatalf("csp has no nonce: %q", csp)
	}
	for _, want := range []string{
		"default-src 'none'",
		"base-uri 'none'",
		"frame-ancestors 'none'",
		"form-action " + wantFormAction + ";",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("csp %q missing %q", csp, want)
		}
	}
	for k, v := range map[string]string{
		"Cache-Control":          "no-store",
		"Referrer-Policy":        "no-referrer",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := rec.Header().Get(k); got != v {
			t.Fatalf("%s = %q, want %q", k, got, v)
		}
	}
	return m[1]
}

// assertCSPDirectives parses the policy and checks the exact directive set,
// independent of serialization order.
func assertCSPDirectives(t *testing.T, csp, wantFormAction string) {
	t.Helper()
	got := map[string]string{}
	for _, part := range strings.Split(csp, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, " ")
		if _, dup := got[name]; dup {
			t.Fatalf("duplicate directive %q in %q", name, csp)
		}
		got[name] = strings.TrimSpace(value)
	}
	nonceRe := regexp.MustCompile(`^'nonce-[A-Za-z0-9_-]{16,}'$`)
	if !nonceRe.MatchString(got["script-src"]) {
		t.Fatalf("script-src %q is not a nonce in %q", got["script-src"], csp)
	}
	want := map[string]string{
		"default-src":     "'none'",
		"base-uri":        "'none'",
		"frame-ancestors": "'none'",
		"form-action":     wantFormAction,
		"script-src":      got["script-src"],
	}
	if len(got) != len(want) {
		t.Fatalf("directive set %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("directive %s = %q, want %q in %q", k, got[k], v, csp)
		}
	}
}

var (
	launchFormAction = regexp.MustCompile(`<form id="launch" method="post" action="([^"]*)">`)
	launchFormToken  = regexp.MustCompile(`name="access_token" value="([^"]*)"`)
)

// launchedForm returns the unescaped form action and access token of a
// launcher page.
func launchedForm(t *testing.T, rec *httptest.ResponseRecorder) (action, token string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	a := launchFormAction.FindStringSubmatch(body)
	tok := launchFormToken.FindStringSubmatch(body)
	if a == nil || tok == nil {
		t.Fatalf("no launch form: %s", body)
	}
	return html.UnescapeString(a[1]), html.UnescapeString(tok[1])
}

// assertLaunchRefused checks a formless error page. Error pages carry fixed
// copy only, so the redacted cause is matched in the request log.
func assertLaunchRefused(t *testing.T, rec *httptest.ResponseRecorder, logs string, status int, cause string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d; body %s", rec.Code, status, rec.Body.String())
	}
	assertLauncherHeaders(t, rec, "'none'")
	body := rec.Body.String()
	if strings.Contains(body, "<form") || strings.Contains(body, "access_token") {
		t.Fatalf("error page has a form: %s", body)
	}
	if !strings.Contains(logs, cause) {
		t.Fatalf("log missing %q: %s", cause, logs)
	}
}

func TestWebappLaunchErrorMappingsUseFixedCopy(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		copy   string
	}{
		{"not found", errtypes.NotFound("share gone " + launchSecret), http.StatusNotFound, "The shared application was not found."},
		{"permission denied", errtypes.PermissionDenied("denied " + launchSecret), http.StatusForbidden, "The sharing service is not trusted."},
		{"invalid credentials", errtypes.InvalidCredentials("bad " + launchToken), http.StatusUnauthorized, "You are not signed in."},
		{"internal", errtypes.InternalError("boom " + launchToken), http.StatusInternalServerError, "The launch failed."},
		{"generic", errors.New("raw " + launchSecret + " " + launchToken), http.StatusInternalServerError, "The launch failed."},
		{"timeout", context.DeadlineExceeded, http.StatusGatewayTimeout, "The launch did not complete in time."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &observeClient{token: launchToken, exchangeErr: tt.err}
			share := receivedWebappShare("https://dav.example/dav", "https://app.example/open", launchSecret,
				[]string{"must-exchange-token"})
			h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)
			req, logs := newLauncherRequest(t, url.Values{"file": {"/ocm/share-1/f.txt"}})
			rec := httptest.NewRecorder()
			h.WebappLaunch(rec, req)

			if rec.Code != tt.status {
				t.Fatalf("status %d, want %d; body %s", rec.Code, tt.status, rec.Body.String())
			}
			assertLauncherHeaders(t, rec, "'none'")
			body := rec.Body.String()
			if !strings.Contains(body, "<p>"+tt.copy+"</p>") {
				t.Fatalf("body missing fixed copy %q: %s", tt.copy, body)
			}
			if strings.Contains(body, "<form") || strings.Contains(body, "access_token") {
				t.Fatalf("error page has a form: %s", body)
			}
			assertNotLeaked(t, body, logs.String(), launchSecret, launchToken)
		})
	}
}

func TestWebappLaunchNonceFailureRendersErrorPage(t *testing.T) {
	old := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("entropy " + launchSecret) }
	t.Cleanup(func() { randRead = old })

	h, obs := launcherHandler(t, "https://app.example/open")
	req, logs := newLauncherRequest(t, url.Values{"file": {"/ocm/share-1/f.txt"}})
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content type %q", ct)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'none'") {
		t.Fatalf("csp %q", csp)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'none'") {
		t.Fatalf("csp lacks script-src 'none': %q", csp)
	}
	if strings.Contains(csp, "'nonce-") {
		t.Fatalf("csp carries a nonce: %q", csp)
	}
	for k, v := range map[string]string{
		"Cache-Control":          "no-store",
		"Referrer-Policy":        "no-referrer",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := rec.Header().Get(k); got != v {
			t.Fatalf("%s = %q, want %q", k, got, v)
		}
	}
	if !strings.Contains(rec.Body.String(), "<p>The launch failed.</p>") {
		t.Fatalf("body: %s", rec.Body.String())
	}
	if obs.exchangeCalls != 0 {
		t.Fatalf("exchange ran without a nonce: %d", obs.exchangeCalls)
	}
	assertNotLeaked(t, rec.Body.String(), logs.String(), launchSecret, launchToken)
}

func TestWebappLaunchSuccessHeadersAndForm(t *testing.T) {
	h, obs := launcherHandler(t, "https://app.example:8443/hub/open?folder=1")
	form := url.Values{
		"file":                         {"/ocm/share-1/dir/my file.txt"},
		"expired_session_redirect_uri": {"https://receiver.example/sessions/expired"},
	}
	req, _ := newLauncherRequest(t, form)
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content type %q", ct)
	}
	nonce := assertLauncherHeaders(t, rec, "https://app.example:8443")
	body := rec.Body.String()
	if !strings.Contains(body, `<script nonce="`+nonce+`">`) {
		t.Fatalf("script nonce mismatch: %s", body)
	}
	if !strings.Contains(body, `action="https://app.example:8443/hub/open/dir/my%20file.txt?folder=1"`) {
		t.Fatalf("form action: %s", body)
	}
	if !strings.Contains(body, `name="access_token" value="`+launchToken+`"`) {
		t.Fatalf("token field: %s", body)
	}
	if !strings.Contains(body, `name="expired_session_redirect_uri" value="https://receiver.example/sessions/expired"`) {
		t.Fatalf("redirect field: %s", body)
	}
	if !strings.Contains(body, "<noscript>") {
		t.Fatalf("no noscript fallback: %s", body)
	}
	if strings.Contains(body, launchSecret) {
		t.Fatal("body leaked shared secret")
	}
	if obs.exchangeCalls != 1 {
		t.Fatalf("exchange calls %d", obs.exchangeCalls)
	}

	// A second response gets a fresh nonce.
	req2, _ := newLauncherRequest(t, form)
	rec2 := httptest.NewRecorder()
	h.WebappLaunch(rec2, req2)
	if nonce2 := assertLauncherHeaders(t, rec2, "https://app.example:8443"); nonce2 == nonce {
		t.Fatal("nonce was reused")
	}
}

func TestWebappLaunchExpiredSessionRedirectOrigin(t *testing.T) {
	tests := []struct {
		name     string
		redirect string
		forward  bool
	}{
		{"own origin", "https://receiver.example/x?a=1", true},
		{"own origin with ampersand", "https://receiver.example/x?a=1&b=2", true},
		{"own origin bare", "https://receiver.example", true},
		{"other host", "https://evil.example/x", false},
		{"http scheme", "http://receiver.example/x", false},
		{"port differs", "https://receiver.example:8443/x", false},
		{"explicit default port", "https://receiver.example:443/x", true},
		{"host case fold", "https://Receiver.Example/x", true},
		{"http default port", "https://receiver.example:80/x", false},
		{"suffix host", "https://receiver.example.evil.example/x", false},
		{"userinfo host trick", "https://receiver.example@evil.example/x", false},
		{"prefix host", "https://evilreceiver.example/x", false},
		{"relative", "/sessions/expired", false},
		{"scheme relative", "//receiver.example/x", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := launcherHandler(t, "https://app.example/hub/open")
			form := url.Values{"file": {"/ocm/share-1/f.txt"}}
			if tt.redirect != "" {
				form.Set("expired_session_redirect_uri", tt.redirect)
			}
			req, _ := newLauncherRequest(t, form)
			// A hostile Host header must not change the accepted origin.
			req.Host = "evil.example"
			req.Header.Set("X-Forwarded-Host", "evil.example")
			rec := httptest.NewRecorder()
			h.WebappLaunch(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d", rec.Code)
			}
			body := rec.Body.String()
			if !tt.forward {
				if strings.Contains(body, "expired_session_redirect_uri") {
					t.Fatalf("redirect forwarded: %s", body)
				}
				return
			}
			// html/template escapes '&' in attribute values; nothing else in these values changes.
			want := `name="expired_session_redirect_uri" value="` + strings.ReplaceAll(tt.redirect, "&", "&amp;") + `"`
			if !strings.Contains(body, want) {
				t.Fatalf("want %s in: %s", want, body)
			}
		})
	}
}

func TestWebappLaunchHostileAppURLIsEscaped(t *testing.T) {
	h, _ := launcherHandler(t, `https://app.example/open?x="><script>alert(1)</script>&y='z'`)
	req, _ := newLauncherRequest(t, url.Values{"file": {"/ocm/share-1/f.txt"}})
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "<script>alert(1)") || strings.Contains(body, `"><script`) {
		t.Fatalf("hostile url was not escaped: %s", body)
	}
	if n := strings.Count(body, "<script"); n != 1 {
		t.Fatalf("script tags = %d: %s", n, body)
	}
	assertLauncherHeaders(t, rec, "https://app.example")
}

func TestWebappLaunchHostileAccessTokenIsEscaped(t *testing.T) {
	obs := &observeClient{token: `tok"><script>alert(1)</script>`}
	share := receivedWebappShare("https://dav.example/dav", "https://app.example/open", launchSecret,
		[]string{"must-exchange-token"})
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)
	req, _ := newLauncherRequest(t, url.Values{"file": {"/ocm/share-1/f.txt"}})
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, "<script>alert(1)") {
		t.Fatalf("token was not escaped: %s", body)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, body)
	}
	assertLauncherHeaders(t, rec, "https://app.example")
	if !strings.Contains(body, `<form id="launch" method="post" action="https://app.example/open/f.txt"`) {
		t.Fatalf("launcher form missing or wrong action: %s", body)
	}
	if !strings.Contains(body, `name="access_token"`) {
		t.Fatalf("token field missing: %s", body)
	}
}

func TestWebappLaunchValidationFailureRendersRedactedErrorPage(t *testing.T) {
	obs := &observeClient{token: launchToken}
	share := receivedWebappShare("https://dav.example/dav", "http://app.example/open", launchSecret,
		[]string{"must-exchange-token"})
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)

	req, _ := newLauncherRequest(t, url.Values{"file": {"/ocm/share-1/f.txt"}})
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	assertLauncherHeaders(t, rec, "'none'")
	body := rec.Body.String()
	if strings.Contains(body, "<form") || strings.Contains(body, "access_token") {
		t.Fatalf("error page has a form: %s", body)
	}
	assertNotLeaked(t, body, "", launchSecret, launchToken)
	if obs.exchangeCalls != 0 {
		t.Fatalf("exchange ran for an invalid launch: %d", obs.exchangeCalls)
	}
}

func TestWebappLaunchRequestErrorsUseSecureHeaders(t *testing.T) {
	h, _ := launcherHandler(t, "https://app.example/open")
	for name, form := range map[string]url.Values{
		"missing file": {},
		"bad file":     {"file": {"/ocm/"}},
		"traversal":    {"file": {"/ocm/share-1/../x"}},
	} {
		t.Run(name, func(t *testing.T) {
			req, _ := newLauncherRequest(t, form)
			rec := httptest.NewRecorder()
			h.WebappLaunch(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			assertLauncherHeaders(t, rec, "'none'")
		})
	}
}

func TestWebappLaunchExchangeFailureDoesNotLeak(t *testing.T) {
	obs := &observeClient{token: launchToken, exchangeErr: context.DeadlineExceeded}
	share := receivedWebappShare("https://dav.example/dav", "https://app.example/open", launchSecret,
		[]string{"must-exchange-token"})
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)
	req, _ := newLauncherRequest(t, url.Values{"file": {"/ocm/share-1/f.txt"}})
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("unexpected success: %s", rec.Body.String())
	}
	assertLauncherHeaders(t, rec, "'none'")
	assertNotLeaked(t, rec.Body.String(), "", launchSecret, launchToken)
}

func TestWebappLaunchLogsDoNotLeakTokens(t *testing.T) {
	h, _ := launcherHandler(t, "https://app.example/open")
	for _, f := range []url.Values{
		{"file": {"/ocm/share-1/f.txt"}},
		{"file": {"/ocm/share-1/../x"}},
	} {
		req, logs := newLauncherRequest(t, f)
		h.WebappLaunch(httptest.NewRecorder(), req)
		assertLogsClean(t, logs.String(), launchSecret, launchToken)
	}

	failing := &observeClient{token: launchToken, exchangeErr: errors.New("exchange failed " + launchToken)}
	share := receivedWebappShare("https://dav.example/dav", "https://app.example/open", launchSecret,
		[]string{"must-exchange-token"})
	h2 := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, failing)
	req, logs := newLauncherRequest(t, url.Values{"file": {"/ocm/share-1/f.txt"}})
	rec := httptest.NewRecorder()
	h2.WebappLaunch(rec, req)
	assertNotLeaked(t, rec.Body.String(), logs.String(), launchSecret, launchToken)
}

func TestWebappLaunchRouteIsProtectedAndPostOnly(t *testing.T) {
	svc, err := New(context.Background(), scienceMeshServiceConfig("receiver.example", true))
	if err != nil {
		t.Fatal(err)
	}
	r := router.New()
	svc.Routes(r)
	if slices.Contains(r.Unprotected(), mount+"/webapp-launch") {
		t.Fatalf("/webapp-launch must be protected: %v", r.Unprotected())
	}

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, mount+"/webapp-launch", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status %d, want 405", method, rec.Code)
		}
	}
}

func TestOpenInAppRouteIsRemoved(t *testing.T) {
	svc, err := New(context.Background(), scienceMeshServiceConfig("receiver.example", true))
	if err != nil {
		t.Fatal(err)
	}
	r := router.New()
	svc.Routes(r)
	req := httptest.NewRequest(http.MethodPost, mount+"/open-in-app", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /open-in-app status %d, want 404", rec.Code)
	}
}

// authGateway records Authenticate calls; a rejected login is all it returns.
type authGateway struct {
	*fakeReceivedGateway
	authCalls int
}

func (g *authGateway) Authenticate(
	_ context.Context, _ *gateway.AuthenticateRequest, _ ...grpc.CallOption,
) (*gateway.AuthenticateResponse, error) {
	g.authCalls++
	return &gateway.AuthenticateResponse{
		Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_UNAUTHENTICATED},
	}, nil
}

// reva resolves the route before the auth middleware runs, and the middleware
// only lets through routes declared unprotected.
func TestWebappLaunchUnauthenticatedFailsClosed(t *testing.T) {
	svc, err := New(context.Background(), scienceMeshServiceConfig("receiver.example", true))
	if err != nil {
		t.Fatal(err)
	}
	r := router.New()
	svc.Routes(r)
	mw, err := auth.New(map[string]any{
		"token_strategy_chain": []string{"bearer"},
		"credential_chain":     []string{"bearer"},
		"token_manager":        "jwt",
		"token_managers": map[string]map[string]any{
			"jwt": {"secret": "unauthenticated-test-secret"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Authenticate must never mint anything here, so it is the only place a
	// token could come from; the launcher itself needs the gateway to read the
	// share, which must not be reached either.
	for name, tc := range map[string]struct {
		header   string
		form     url.Values
		wantAuth int
	}{
		"no credentials":      {form: url.Values{"file": {"/ocm/share-1/f.txt"}}},
		"rejected bearer":     {header: "Bearer bad-header-token", form: url.Values{"file": {"/ocm/share-1/f.txt"}}, wantAuth: 1},
		"rejected form token": {form: url.Values{"file": {"/ocm/share-1/f.txt"}, "access_token": {"bad-form-token"}}, wantAuth: 1},
	} {
		t.Run(name, func(t *testing.T) {
			share := receivedWebappShare("https://dav.example/dav", "https://app.example/open", launchSecret,
				[]string{"must-exchange-token"})
			fake := &fakeReceivedGateway{resp: okShareResponse(share)}
			gw := &authGateway{fakeReceivedGateway: fake}
			installLaunchGateway(t, gw)

			base, logs := newLaunchRequest(t, "")
			req := httptest.NewRequest(http.MethodPost, mount+"/webapp-launch", strings.NewReader(tc.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			req = req.WithContext(base.Context())
			rt, ok := r.Match(req)
			if !ok {
				t.Fatal("no route declared for /webapp-launch")
			}
			req = req.WithContext(router.WithRoute(req.Context(), rt))

			rec := httptest.NewRecorder()
			mw(r).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status %d, want 401; body %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if strings.Contains(body, "<form") || strings.Contains(body, "access_token") || strings.Contains(body, launchToken) {
				t.Fatalf("unauthenticated response rendered launcher content: %s", body)
			}
			if gw.authCalls != tc.wantAuth {
				t.Fatalf("authenticate calls %d, want %d", gw.authCalls, tc.wantAuth)
			}
			if fake.calls != 0 {
				t.Fatalf("launcher reached the gateway %d times", fake.calls)
			}
			assertNotLeaked(t, body, logs.String(), launchSecret, launchToken)
		})
	}
}

func TestWebappLaunchFormActionIsExactOrigin(t *testing.T) {
	tests := []struct {
		name           string
		appURI         string
		wantFormAction string
		wantAction     string
	}{
		{"ascii host with port", "https://app.example:8443/open",
			"https://app.example:8443", "https://app.example:8443/open/f.txt"},
		{"mixed case host", "https://App.Example/open",
			"https://app.example", "https://App.Example/open/f.txt"},
		{"ipv4 host", "https://192.0.2.10/open",
			"https://192.0.2.10", "https://192.0.2.10/open/f.txt"},
		{"internationalized host", "https://b\u00fccher.example/open?x=1",
			"https://xn--bcher-kva.example", "https://xn--bcher-kva.example/open/f.txt?x=1"},
		{"internationalized host with port", "https://B\u00dcCHER.example:8443/open",
			"https://xn--bcher-kva.example:8443", "https://xn--bcher-kva.example:8443/open/f.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, obs := launcherHandler(t, tt.appURI)
			req, _ := newLauncherRequest(t, url.Values{"file": {"/ocm/share-1/f.txt"}})
			rec := httptest.NewRecorder()
			h.WebappLaunch(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			assertLauncherHeaders(t, rec, tt.wantFormAction)
			assertCSPDirectives(t, rec.Header().Get("Content-Security-Policy"), tt.wantFormAction)
			want := `<form id="launch" method="post" action="` + tt.wantAction + `">`
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("form action, want %s in: %s", want, rec.Body.String())
			}
			if obs.exchangeCalls != 1 {
				t.Fatalf("exchange calls %d", obs.exchangeCalls)
			}
		})
	}
}

func TestWebappLaunchRefusesHostsCSPCannotName(t *testing.T) {
	for name, appURI := range map[string]string{
		"semicolon":    "https://app;script-src.example/open",
		"comma":        "https://app,evil.example/open",
		"quote":        "https://app'evil.example/open",
		"asterisk":     "https://app*.example/open",
		"underscore":   "https://app_x.example/open",
		"ipv6 literal": "https://[2001:db8::1]/open",
		"trailing dot": "https://app.example./open",
		"bad hyphen":   "https://-app.example/open",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := requireHTTPSAppURI(appURI); err != nil {
				t.Fatalf("shared validation rejects %q: %v", appURI, err)
			}
			h, obs := launcherHandler(t, appURI)
			req, logs := newLauncherRequest(t, url.Values{"file": {"/ocm/share-1/f.txt"}})
			rec := httptest.NewRecorder()
			h.WebappLaunch(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			assertLauncherHeaders(t, rec, "'none'")
			body := rec.Body.String()
			if !strings.Contains(body, "<p>The shared application cannot be launched.</p>") {
				t.Fatalf("body: %s", body)
			}
			if strings.Contains(body, "<form") || strings.Contains(body, "access_token") {
				t.Fatalf("error page has a form: %s", body)
			}
			if obs.discoverCalls != 0 || obs.exchangeCalls != 0 {
				t.Fatalf("refused host reached discovery %d / exchange %d", obs.discoverCalls, obs.exchangeCalls)
			}
			assertNotLeaked(t, body, logs.String(), launchSecret, launchToken)
		})
	}
}
