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

// Package loginflow implements the NextCloud Login Flow V2 protocol for
// enrolling the NextCloud Desktop sync client without a password.
//
// POC LIMITATIONS:
//   - In-memory flow storage (restarts lose pending flows).
//   - Auto-approves every browser visit for AutoApproveUser; no real SSO.
//   - Uses the appauth JSON manager directly rather than via the gateway gRPC.
//     Both this service and the applicationauth gRPC service must point at the
//     same JSON file, and the UserId stored here must match the UserId your user
//     provider returns for AutoApproveUser (set AutoApproveUserIdp accordingly).
package loginflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appauth"
	_ "github.com/cs3org/reva/v3/pkg/appauth/manager/json"
	"github.com/cs3org/reva/v3/pkg/appauth/manager/registry"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/go-chi/chi/v5"
)

func init() {
	global.Register("loginflow", New)
}

type config struct {
	Prefix              string                    `mapstructure:"prefix"`
	AutoApproveUser     string                    `mapstructure:"auto_approve_user"`
	AutoApproveUserIdp  string                    `mapstructure:"auto_approve_user_idp"`
	ServerBaseURL       string                    `mapstructure:"server_base_url"`
	AppAuthDriver       string                    `mapstructure:"appauth_driver"`
	AppAuthDrivers      map[string]map[string]any `mapstructure:"appauth_drivers"`
	FlowTTLSeconds      int                       `mapstructure:"flow_ttl_seconds"`
}

func (c *config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "index.php/login/v2"
	}
	if c.AutoApproveUser == "" {
		c.AutoApproveUser = "jgeens"
	}
	if c.AppAuthDriver == "" {
		c.AppAuthDriver = "json"
	}
	if c.FlowTTLSeconds == 0 {
		c.FlowTTLSeconds = 1200
	}
	if c.ServerBaseURL == "" {
		c.ServerBaseURL = "http://localhost:9998"
	}
}

// flow holds the in-memory state for one pending enrolment.
type flow struct {
	loginHash string // hex(SHA256(logintoken))
	pollHash  string // hex(SHA256(polltoken))
	userAgent string
	expiresAt time.Time
	approved  bool
	username  string // set on approval
	appPass   string // plain-text app password, set on approval
}

type svc struct {
	c       *config
	router  *chi.Mux
	am      appauth.Manager
	mu      sync.Mutex
	byLogin map[string]*flow // keyed by loginHash
	byPoll  map[string]*flow // keyed by pollHash
}

// New creates a new loginflow HTTP service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	f, ok := registry.NewFuncs[c.AppAuthDriver]
	if !ok {
		return nil, fmt.Errorf("loginflow: appauth driver %q not registered", c.AppAuthDriver)
	}
	am, err := f(ctx, c.AppAuthDrivers[c.AppAuthDriver])
	if err != nil {
		return nil, fmt.Errorf("loginflow: initialising appauth driver: %w", err)
	}

	s := &svc{
		c:       &c,
		am:      am,
		byLogin: make(map[string]*flow),
		byPoll:  make(map[string]*flow),
	}

	appctx.GetLogger(ctx).Info().Str("service", "loginflow").Str("prefix", c.Prefix).Str("server_base_url", c.ServerBaseURL).Str("auto_approve_user", c.AutoApproveUser).Str("auto_approve_user_idp", c.AutoApproveUserIdp).Str("appauth_driver", c.AppAuthDriver).Int("flow_ttl_seconds", c.FlowTTLSeconds).Msg("loginflow service initialised")

	r := chi.NewRouter()
	r.Post("/", s.handleInit)
	r.Get("/flow/{lt}", s.handleBrowserFlow)
	r.Post("/poll", s.handlePoll)
	s.router = r

	return s, nil
}

func (s *svc) Prefix() string { return s.c.Prefix }
func (s *svc) Close() error   { return nil }

// Unprotected marks the entire service as skipping the auth middleware, since
// all three endpoints handle anonymous callers.
func (s *svc) Unprotected() []string { return []string{"/"} }

func (s *svc) Handler() http.Handler { return s.router }

// JSON wire types ---------------------------------------------------------

type initResponse struct {
	Poll  pollEndpoint `json:"poll"`
	Login string       `json:"login"`
}

type pollEndpoint struct {
	Token    string `json:"token"`
	Endpoint string `json:"endpoint"`
}

type pollResponse struct {
	Server      string `json:"server"`
	LoginName   string `json:"loginName"`
	AppPassword string `json:"appPassword"`
}

// Token helpers -----------------------------------------------------------

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Handlers ----------------------------------------------------------------

// handleInit implements POST /index.php/login/v2.
// The NextCloud client calls this first to start a flow.
func (s *svc) handleInit(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context()).With().Str("service", "loginflow").Str("handler", "init").Logger()
	log.Info().Str("method", r.Method).Str("path", r.URL.Path).Str("remote", r.RemoteAddr).Str("user_agent", r.Header.Get("User-Agent")).Msg("incoming request")

	lt, err := generateToken()
	if err != nil {
		log.Error().Err(err).Msg("could not generate login token")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	pt, err := generateToken()
	if err != nil {
		log.Error().Err(err).Msg("could not generate poll token")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	f := &flow{
		loginHash: tokenHash(lt),
		pollHash:  tokenHash(pt),
		userAgent: r.Header.Get("User-Agent"),
		expiresAt: time.Now().Add(time.Duration(s.c.FlowTTLSeconds) * time.Second),
	}

	s.mu.Lock()
	s.byLogin[f.loginHash] = f
	s.byPoll[f.pollHash] = f
	pending := len(s.byLogin)
	s.mu.Unlock()

	base := s.c.ServerBaseURL
	resp := initResponse{
		Poll: pollEndpoint{
			Token:    pt,
			Endpoint: base + "/index.php/login/v2/poll",
		},
		Login: base + "/index.php/login/v2/flow/" + lt,
	}

	log.Info().Str("login_hash", f.loginHash).Str("poll_hash", f.pollHash).Str("login_url", resp.Login).Time("expires_at", f.expiresAt).Int("pending_flows", pending).Msg("flow created")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("could not encode init response")
	}
}

// handleBrowserFlow implements GET /index.php/login/v2/flow/{lt}.
// In the real implementation this would redirect to the web UI for SSO.
// For the POC it auto-approves immediately for AutoApproveUser.
func (s *svc) handleBrowserFlow(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context()).With().Str("service", "loginflow").Str("handler", "browserflow").Logger()
	lt := chi.URLParam(r, "lt")
	log.Info().Str("method", r.Method).Str("path", r.URL.Path).Str("remote", r.RemoteAddr).Bool("has_token", lt != "").Msg("incoming request")
	if lt == "" {
		log.Warn().Msg("missing login token in URL")
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	lh := tokenHash(lt)
	s.mu.Lock()
	f, ok := s.byLogin[lh]
	s.mu.Unlock()

	if !ok {
		log.Warn().Str("login_hash", lh).Msg("flow not found")
		http.Error(w, "flow not found or expired", http.StatusNotFound)
		return
	}
	if time.Now().After(f.expiresAt) {
		log.Warn().Str("login_hash", lh).Time("expires_at", f.expiresAt).Msg("flow expired")
		http.Error(w, "flow not found or expired", http.StatusNotFound)
		return
	}

	if f.approved {
		log.Info().Str("login_hash", lh).Msg("flow already approved, returning already-approved page")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, htmlPage("Already approved", "This flow was already approved. You may close this window."))
		return
	}

	// Build a user context for the configured auto-approve account.
	log.Info().Str("login_hash", lh).Str("auto_approve_user", s.c.AutoApproveUser).Str("auto_approve_user_idp", s.c.AutoApproveUserIdp).Msg("auto-approving flow")
	user := &userpb.User{
		Id: &userpb.UserId{
			OpaqueId: s.c.AutoApproveUser,
			Idp:      s.c.AutoApproveUserIdp,
		},
		Username: s.c.AutoApproveUser,
	}
	ctx := appctx.ContextSetUser(r.Context(), user)

	ownerScope, err := scope.AddOwnerScope(nil)
	if err != nil {
		log.Error().Err(err).Msg("could not build owner scope")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	label := fmt.Sprintf("Nextcloud Desktop (%s)", f.userAgent)
	appPass, err := s.am.GenerateAppPassword(ctx, ownerScope, label, nil)
	if err != nil {
		log.Error().Err(err).Str("label", label).Msg("could not generate app password")
		http.Error(w, "could not generate app password: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	f.approved = true
	f.username = s.c.AutoApproveUser
	f.appPass = appPass.Password
	s.mu.Unlock()

	log.Info().Str("login_hash", lh).Str("username", s.c.AutoApproveUser).Str("label", label).Msg("app password generated, flow approved")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, htmlPage("Access granted",
		fmt.Sprintf("Granted access for <strong>%s</strong> to client: %s<br>You may close this window.",
			s.c.AutoApproveUser, f.userAgent)))
}

// handlePoll implements POST /index.php/login/v2/poll.
// The NextCloud client calls this every ~3 s while waiting for the user to
// open the login URL in a browser.  Returns 404 until the flow is approved,
// then returns the credentials and deletes the flow.
func (s *svc) handlePoll(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context()).With().Str("service", "loginflow").Str("handler", "poll").Logger()
	log.Debug().Str("method", r.Method).Str("path", r.URL.Path).Str("remote", r.RemoteAddr).Msg("incoming request")

	if err := r.ParseForm(); err != nil {
		log.Warn().Err(err).Msg("could not parse poll form")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	pt := r.FormValue("token")
	if pt == "" {
		log.Warn().Msg("poll request missing token")
		http.NotFound(w, r)
		return
	}

	ph := tokenHash(pt)
	s.mu.Lock()
	f, ok := s.byPoll[ph]
	s.mu.Unlock()

	if !ok {
		log.Debug().Str("poll_hash", ph).Msg("poll: flow not found (already consumed or never existed)")
		http.NotFound(w, r)
		return
	}
	if time.Now().After(f.expiresAt) {
		log.Warn().Str("poll_hash", ph).Time("expires_at", f.expiresAt).Msg("poll: flow expired")
		http.NotFound(w, r)
		return
	}
	if !f.approved {
		log.Debug().Str("poll_hash", ph).Msg("poll: flow not yet approved, returning 404")
		http.NotFound(w, r)
		return
	}

	// Consume the flow: remove it so the credentials are returned exactly once.
	s.mu.Lock()
	delete(s.byLogin, f.loginHash)
	delete(s.byPoll, f.pollHash)
	username := f.username
	appPass := f.appPass
	s.mu.Unlock()

	resp := pollResponse{
		Server:      s.c.ServerBaseURL,
		LoginName:   username,
		AppPassword: appPass,
	}

	log.Info().Str("poll_hash", ph).Str("username", username).Str("server", resp.Server).Msg("poll: flow approved, returning credentials and consuming flow")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("could not encode poll response")
	}
}

func htmlPage(title, body string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>%s</title></head>
<body><h2>%s</h2><p>%s</p></body></html>`, title, title, body)
}
