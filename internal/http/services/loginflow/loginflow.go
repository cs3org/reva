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

// Package loginflow implements the anonymous side of the login flow for
// enrolling the Nextcloud Desktop sync client without a password. It lives under
// /index.php/login/v2 because that is the path sync clients expect.
//
// This service holds only the endpoints the sync client and the browser hit
// unauthenticated: init, poll, and the browser redirect. The authenticated user
// actions (info, grant, deny) live in the OCS service, behind the auth
// middleware, next to the connected-clients management API.
package loginflow

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func init() {
	global.Register("loginflow", New)
}

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
	// server_base_url has no default. It is the externally reachable URL handed
	// to the sync client in the init and poll responses, so only the deployment
	// knows it. New rejects an empty value.
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
	limiter *limiter
}

// New creates a new loginflow HTTP service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	if c.ServerBaseURL == "" {
		return nil, errors.New("loginflow: server_base_url is required")
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

	s := &svc{
		c:       &c,
		am:      am,
		store:   store,
		limiter: newLimiter(),
	}

	r := chi.NewRouter()
	r.Post("/", s.handleInit)
	r.Get("/flow/{lt}", s.handleBrowserFlow)
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

// Token helpers -----------------------------------------------------------

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
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

	ca := &loginflow.ClientAuthorization{
		LoginHash: loginflow.HashToken(lt),
		PollHash:  loginflow.HashToken(pt),
		ClientID:  uuid.New().String(),
		UserAgent: r.Header.Get("User-Agent"),
		ExpiresAt: time.Now().Add(time.Duration(s.c.FlowTTLSeconds) * time.Second),
	}
	if err := s.store.Create(r.Context(), ca); err != nil {
		log.Error().Err(err).Msg("could not create client authorization")
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

	log.Info().Str("client_id", ca.ClientID).Time("expires_at", ca.ExpiresAt).Msg("client authorization created")
	writeJSON(w, http.StatusOK, resp)
}

// handleBrowserFlow implements GET /index.php/login/v2/flow/{lt}. It redirects
// the anonymous browser to the web UI, which runs SSO and the confirmation page.
func (s *svc) handleBrowserFlow(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context()).With().Str("service", "loginflow").Str("handler", "browserflow").Logger()

	ca, code := s.lookupByLogin(r, chi.URLParam(r, "lt"))
	if code != 0 {
		http.Error(w, "client authorization not found or expired", code)
		return
	}

	target := strings.TrimRight(s.c.WebUIURL, "/") + "/login-flow/" + chi.URLParam(r, "lt")
	log.Info().Str("client_id", ca.ClientID).Msg("redirecting browser to web UI")
	http.Redirect(w, r, target, http.StatusFound)
}

// handlePoll implements POST /index.php/login/v2/poll. It waits briefly for an
// approval, then consumes the authorization and mints the app password exactly
// once.
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

	ph := loginflow.HashToken(pt)
	if !s.limiter.allow("poll-ip:"+clientIP(r), s.c.RateLimit.PollPerIPPerMin) ||
		!s.limiter.allow("poll-tok:"+base64.RawURLEncoding.EncodeToString(ph), s.c.RateLimit.PollPerTokenPerMin) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	ca := s.waitForApproval(r, ph)
	if ca == nil {
		http.NotFound(w, r)
		return
	}

	// Bind the poll to the original Init context: the same client process
	// must present the same User-Agent.
	if r.Header.Get("User-Agent") != ca.UserAgent {
		log.Warn().Str("client_id", ca.ClientID).Msg("poll user-agent mismatch")
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
// authorization's user. The label carries the client_id so the management API
// can map a row back to a client authorization.
func (s *svc) mintAppPassword(ctx context.Context, ca *loginflow.ClientAuthorization) (string, error) {
	user := &userpb.User{
		Id:       &userpb.UserId{OpaqueId: ca.UserID},
		Username: ca.Username,
	}
	ctx = appctx.ContextSetUser(ctx, user)

	ownerScope, err := scope.AddOwnerScope(nil)
	if err != nil {
		return "", err
	}

	// Label carries the parsed, human-readable parts the management API returns:
	// "<user-chosen name>|<client description>|<client_id>". Both the name and
	// the description are clean strings; the raw User-Agent is not stored.
	label := ca.DeviceName + "|" + loginflow.ClientDescription(ca.UserAgent) + "|" + ca.ClientID
	appPass, err := s.am.GenerateAppPassword(ctx, ownerScope, label, nil)
	if err != nil {
		return "", err
	}
	return appPass.Password, nil
}

// waitForApproval polls the store for up to poll_wait_ms while the authorization
// is PENDING. It returns the authorization once approved, or nil on timeout /
// not found / expired.
func (s *svc) waitForApproval(r *http.Request, pollHash []byte) *loginflow.ClientAuthorization {
	deadline := time.Now().Add(time.Duration(s.c.RateLimit.PollWaitMs) * time.Millisecond)
	for {
		ca, err := s.store.GetByPoll(r.Context(), pollHash)
		if err != nil || ca.Expired() {
			return nil
		}
		if ca.Approved() {
			return ca
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

// lookupByLogin resolves a client authorization from a login token and maps the
// "gone" vs "unknown" distinction the UI relies on: 404 unknown, 410 expired. A
// zero code means the authorization is live.
func (s *svc) lookupByLogin(r *http.Request, lt string) (*loginflow.ClientAuthorization, int) {
	if lt == "" {
		return nil, http.StatusNotFound
	}
	ca, err := s.store.GetByLogin(r.Context(), loginflow.HashToken(lt))
	if err != nil {
		return nil, statusForError(err)
	}
	if ca.Expired() {
		return nil, http.StatusGone
	}
	return ca, 0
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
