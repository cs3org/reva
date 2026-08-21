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

// Package loginflow implements the NextCloud Login Flow V2 protocol for
// enrolling the NextCloud Desktop sync client without a password.
//
// The browser side (grant, deny, info) authenticates with a bearer token; the
// client side (init, poll) is anonymous. The app password is minted once, at
// the moment the client polls an approved flow, and is never stored.
package loginflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appauth"
	"github.com/cs3org/reva/v3/pkg/appauth/loginflow"
	storeregistry "github.com/cs3org/reva/v3/pkg/appauth/loginflow/registry"
	_ "github.com/cs3org/reva/v3/pkg/appauth/loginflow/sql"
	_ "github.com/cs3org/reva/v3/pkg/appauth/manager/json"
	appauthregistry "github.com/cs3org/reva/v3/pkg/appauth/manager/registry"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/token"
	tokenmgr "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func init() {
	global.Register("loginflow", New)
}

const maxDeviceNameLen = 64

type rateLimitConfig struct {
	InitPerIPPerMin    int `mapstructure:"init_per_ip_per_min"`
	PollPerIPPerMin    int `mapstructure:"poll_per_ip_per_min"`
	PollPerTokenPerMin int `mapstructure:"poll_per_token_per_min"`
	PollWaitMs         int `mapstructure:"poll_wait_ms"`
}

type config struct {
	Prefix         string                    `mapstructure:"prefix"`
	ServerBaseURL  string                    `mapstructure:"server_base_url"`
	WebUIURL       string                    `mapstructure:"webui_url"`
	FlowTTLSeconds int                       `mapstructure:"flow_ttl_seconds"`
	TokenSecret    string                    `mapstructure:"token_secret"`
	AppAuthDriver  string                    `mapstructure:"appauth_driver"`
	AppAuthDrivers map[string]map[string]any `mapstructure:"appauth_drivers"`
	StoreDriver    string                    `mapstructure:"store_driver"`
	StoreDrivers   map[string]map[string]any `mapstructure:"store_drivers"`
	RateLimit      rateLimitConfig           `mapstructure:"ratelimit"`
}

func (c *config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "index.php/login/v2"
	}
	if c.AppAuthDriver == "" {
		c.AppAuthDriver = "json"
	}
	if c.StoreDriver == "" {
		c.StoreDriver = "sql"
	}
	if c.FlowTTLSeconds == 0 {
		c.FlowTTLSeconds = 1200
	}
	if c.ServerBaseURL == "" {
		c.ServerBaseURL = "http://localhost:9998"
	}
	if c.WebUIURL == "" {
		c.WebUIURL = c.ServerBaseURL
	}
	if c.RateLimit.PollWaitMs == 0 {
		c.RateLimit.PollWaitMs = 5000
	}
}

type svc struct {
	c       *config
	router  *chi.Mux
	am      appauth.Manager
	store   loginflow.Manager
	tm      token.Manager
	limiter *limiter
}

// New creates a new loginflow HTTP service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	af, ok := appauthregistry.NewFuncs[c.AppAuthDriver]
	if !ok {
		return nil, fmt.Errorf("loginflow: appauth driver %q not registered", c.AppAuthDriver)
	}
	am, err := af(ctx, c.AppAuthDrivers[c.AppAuthDriver])
	if err != nil {
		return nil, fmt.Errorf("loginflow: initialising appauth driver: %w", err)
	}

	sf, ok := storeregistry.NewFuncs[c.StoreDriver]
	if !ok {
		return nil, fmt.Errorf("loginflow: store driver %q not registered", c.StoreDriver)
	}
	store, err := sf(ctx, c.StoreDrivers[c.StoreDriver])
	if err != nil {
		return nil, fmt.Errorf("loginflow: initialising store driver: %w", err)
	}

	tm, err := tokenmgr.New(map[string]any{"secret": c.TokenSecret})
	if err != nil {
		return nil, fmt.Errorf("loginflow: initialising token manager: %w", err)
	}

	s := &svc{
		c:       &c,
		am:      am,
		store:   store,
		tm:      tm,
		limiter: newLimiter(),
	}

	r := chi.NewRouter()
	r.Post("/", s.handleInit)
	r.Get("/flow/{lt}", s.handleBrowserFlow)
	r.Get("/flow/{lt}/info", s.handleInfo)
	r.Post("/flow/{lt}/grant", s.handleGrant)
	r.Post("/flow/{lt}/deny", s.handleDeny)
	r.Post("/poll", s.handlePoll)
	s.router = r

	appctx.GetLogger(ctx).Info().Str("service", "loginflow").Str("prefix", c.Prefix).Str("server_base_url", c.ServerBaseURL).Str("webui_url", c.WebUIURL).Str("appauth_driver", c.AppAuthDriver).Str("store_driver", c.StoreDriver).Int("flow_ttl_seconds", c.FlowTTLSeconds).Msg("loginflow service initialised")

	return s, nil
}

func (s *svc) Prefix() string        { return s.c.Prefix }
func (s *svc) Close() error          { return nil }
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

type infoResponse struct {
	Client     string `json:"client"`
	User       string `json:"user"`
	CreatedAt  string `json:"created_at"`
	ServerTime string `json:"server_time"`
	Status     string `json:"status"`
}

type grantRequest struct {
	Name string `json:"name"`
}

type statusResponse struct {
	Status string `json:"status"`
}

// Token helpers -----------------------------------------------------------

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func tokenHash(tok string) []byte {
	h := sha256.Sum256([]byte(tok))
	return h[:]
}

// Handlers ----------------------------------------------------------------

// handleInit implements POST /index.php/login/v2.
func (s *svc) handleInit(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context()).With().Str("service", "loginflow").Str("handler", "init").Logger()

	ip := clientIP(r)
	if !s.limiter.allow("init:"+ip, s.c.RateLimit.InitPerIPPerMin) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

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

	f := &loginflow.Flow{
		LoginHash: tokenHash(lt),
		PollHash:  tokenHash(pt),
		ClientID:  uuid.New().String(),
		UserAgent: r.Header.Get("User-Agent"),
		ExpiresAt: time.Now().Add(time.Duration(s.c.FlowTTLSeconds) * time.Second),
	}
	if err := s.store.CreateFlow(r.Context(), f); err != nil {
		log.Error().Err(err).Msg("could not create flow")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := initResponse{
		Poll: pollEndpoint{
			Token:    pt,
			Endpoint: s.c.ServerBaseURL + "/index.php/login/v2/poll",
		},
		Login: s.c.ServerBaseURL + "/index.php/login/v2/flow/" + lt,
	}

	log.Info().Str("client_id", f.ClientID).Time("expires_at", f.ExpiresAt).Msg("flow created")
	writeJSON(w, http.StatusOK, resp)
}

// handleBrowserFlow implements GET /index.php/login/v2/flow/{lt}. It redirects
// the anonymous browser to the web UI, which runs SSO and the confirmation page.
func (s *svc) handleBrowserFlow(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context()).With().Str("service", "loginflow").Str("handler", "browserflow").Logger()

	f, code := s.lookupByLogin(r, chi.URLParam(r, "lt"))
	if code != 0 {
		http.Error(w, "flow not found or expired", code)
		return
	}

	target := strings.TrimRight(s.c.WebUIURL, "/") + "/login-flow/" + chi.URLParam(r, "lt")
	log.Info().Str("client_id", f.ClientID).Msg("redirecting browser to web UI")
	http.Redirect(w, r, target, http.StatusFound)
}

// handleInfo implements GET /index.php/login/v2/flow/{lt}/info. The web UI calls
// it to render the confirmation page. It requires any authenticated user but
// runs no authorization check: a valid token is enough, its identity is not
// restricted.
func (s *svc) handleInfo(w http.ResponseWriter, r *http.Request) {
	user, _, err := s.authUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	f, code := s.lookupByLogin(r, chi.URLParam(r, "lt"))
	if code != 0 {
		http.Error(w, "flow not found or expired", code)
		return
	}

	status := "pending"
	if f.Approved() {
		status = "approved"
	}
	writeJSON(w, http.StatusOK, infoResponse{
		Client:     parseUserAgent(f.UserAgent),
		User:       user.GetUsername(),
		CreatedAt:  f.CreatedAt.UTC().Format(time.RFC3339),
		ServerTime: time.Now().UTC().Format(time.RFC3339),
		Status:     status,
	})
}

// handleGrant implements POST /index.php/login/v2/flow/{lt}/grant. It records
// the granting user; the app password is minted later, at poll time.
func (s *svc) handleGrant(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context()).With().Str("service", "loginflow").Str("handler", "grant").Logger()

	if !s.originAllowed(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}

	user, _, err := s.authUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	name, err := parseDeviceName(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f, code := s.lookupByLogin(r, chi.URLParam(r, "lt"))
	if code != 0 {
		http.Error(w, "flow not found or expired", code)
		return
	}

	if name == "" {
		name = parseUserAgent(f.UserAgent)
	}

	err = s.store.Approve(r.Context(), f.LoginHash, user.GetId().GetOpaqueId(), user.GetUsername(), name)
	if err != nil {
		http.Error(w, "could not approve flow", statusForError(err))
		return
	}

	log.Info().Str("client_id", f.ClientID).Str("username", user.GetUsername()).Msg("flow approved")
	writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

// handleDeny implements POST /index.php/login/v2/flow/{lt}/deny.
func (s *svc) handleDeny(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context()).With().Str("service", "loginflow").Str("handler", "deny").Logger()

	if !s.originAllowed(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}

	if _, _, err := s.authUser(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	f, code := s.lookupByLogin(r, chi.URLParam(r, "lt"))
	if code != 0 {
		http.Error(w, "flow not found or expired", code)
		return
	}

	if err := s.store.Deny(r.Context(), f.LoginHash); err != nil {
		http.Error(w, "could not deny flow", statusForError(err))
		return
	}

	log.Info().Str("client_id", f.ClientID).Msg("flow denied")
	writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

// handlePoll implements POST /index.php/login/v2/poll. It waits briefly for an
// approval, then consumes the flow and mints the app password exactly once.
func (s *svc) handlePoll(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context()).With().Str("service", "loginflow").Str("handler", "poll").Logger()

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	pt := r.FormValue("token")
	if pt == "" {
		http.NotFound(w, r)
		return
	}

	ph := tokenHash(pt)
	if !s.limiter.allow("poll-ip:"+clientIP(r), s.c.RateLimit.PollPerIPPerMin) ||
		!s.limiter.allow("poll-tok:"+base64.RawURLEncoding.EncodeToString(ph), s.c.RateLimit.PollPerTokenPerMin) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	f := s.waitForApproval(r, ph)
	if f == nil {
		http.NotFound(w, r)
		return
	}

	// Bind the poll to the original Init context (§2.8): the same client process
	// must present the same User-Agent.
	if r.Header.Get("User-Agent") != f.UserAgent {
		log.Warn().Str("client_id", f.ClientID).Msg("poll user-agent mismatch")
		http.NotFound(w, r)
		return
	}

	consumed, err := s.store.Consume(r.Context(), ph)
	if err != nil {
		// Lost the consume race or already consumed: the client just sees 404.
		http.NotFound(w, r)
		return
	}

	appPass, err := s.mintAppPassword(r.Context(), consumed)
	if err != nil {
		log.Error().Err(err).Str("client_id", consumed.ClientID).Msg("could not mint app password after consume")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	log.Info().Str("client_id", consumed.ClientID).Str("username", consumed.Username).Msg("poll consumed, app password minted")
	writeJSON(w, http.StatusOK, pollResponse{
		Server:      s.c.ServerBaseURL,
		LoginName:   consumed.Username,
		AppPassword: appPass,
	})
}

// Helpers -----------------------------------------------------------------

// mintAppPassword generates an owner-scoped, non-expiring app password for the
// flow's user. The label carries the client_id so the management API can map a
// row back to a flow (§2.5).
func (s *svc) mintAppPassword(ctx context.Context, f *loginflow.Flow) (string, error) {
	user := &userpb.User{
		Id:       &userpb.UserId{OpaqueId: f.UserID},
		Username: f.Username,
	}
	ctx = appctx.ContextSetUser(ctx, user)

	ownerScope, err := scope.AddOwnerScope(nil)
	if err != nil {
		return "", err
	}

	label := f.DeviceName + "|" + f.ClientID
	appPass, err := s.am.GenerateAppPassword(ctx, ownerScope, label, nil)
	if err != nil {
		return "", err
	}
	return appPass.Password, nil
}

// waitForApproval polls the store for up to poll_wait_ms while the flow is
// PENDING. It returns the flow once approved, or nil on timeout / not found /
// expired.
func (s *svc) waitForApproval(r *http.Request, pollHash []byte) *loginflow.Flow {
	deadline := time.Now().Add(time.Duration(s.c.RateLimit.PollWaitMs) * time.Millisecond)
	for {
		f, err := s.store.GetByPoll(r.Context(), pollHash)
		if err != nil || f.Expired() {
			return nil
		}
		if f.Approved() {
			return f
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-r.Context().Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// lookupByLogin resolves a flow from a login token and maps the "gone" vs
// "unknown" distinction the UI relies on (§3): 404 unknown, 410 expired. A zero
// code means the flow is live.
func (s *svc) lookupByLogin(r *http.Request, lt string) (*loginflow.Flow, int) {
	if lt == "" {
		return nil, http.StatusNotFound
	}
	f, err := s.store.GetByLogin(r.Context(), tokenHash(lt))
	if err != nil {
		return nil, statusForError(err)
	}
	if f.Expired() {
		return nil, http.StatusGone
	}
	return f, 0
}

// authUser validates the request token and returns its user. It reads the token
// from the carriers Reva uses for HTTP: the x-access-token header, an
// Authorization: Bearer header, or an access_token query parameter. It requires
// a valid token but performs no authorization check. It never reads cookies, so
// a cross-origin page cannot forge an authenticated request (§2.4).
func (s *svc) authUser(r *http.Request) (*userpb.User, string, error) {
	tok := tokenFromRequest(r)
	if tok == "" {
		return nil, "", errtypes.InvalidCredentials("missing token")
	}
	user, _, err := s.tm.DismantleToken(r.Context(), tok)
	if err != nil {
		return nil, "", err
	}
	return user, tok, nil
}

// tokenFromRequest extracts a Reva access token from the standard HTTP carriers.
func tokenFromRequest(r *http.Request) string {
	if h := r.Header.Get(appctx.TokenHeader); h != "" {
		return h
	}
	if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && tok != "" {
		return tok
	}
	return r.URL.Query().Get("access_token")
}

// originAllowed enforces the Origin check on state-changing browser POSTs (§2.4).
func (s *svc) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	return strings.TrimRight(origin, "/") == strings.TrimRight(s.c.WebUIURL, "/")
}

// parseDeviceName reads the optional {"name": "..."} body and validates it: at
// most 64 runes, no control characters (§ UI #1).
func parseDeviceName(r *http.Request) (string, error) {
	if r.Body == nil {
		return "", nil
	}
	var req grantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// An empty body is allowed; a malformed one is not.
		if err.Error() == "EOF" {
			return "", nil
		}
		return "", fmt.Errorf("invalid request body")
	}
	name := strings.TrimSpace(req.Name)
	if utf8.RuneCountInString(name) > maxDeviceNameLen {
		return "", fmt.Errorf("name too long")
	}
	for _, c := range name {
		if unicode.IsControl(c) {
			return "", fmt.Errorf("name contains control characters")
		}
	}
	return name, nil
}

func statusForError(err error) int {
	switch err.(type) {
	case errtypes.NotFound:
		return http.StatusNotFound
	case errtypes.Conflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func clientIP(r *http.Request) string {
	ip, err := utils.GetClientIP(r, false)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
