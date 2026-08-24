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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cs3org/reva/v3/pkg/appauth/loginflow"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/go-chi/chi/v5"
)

// These handlers implement the authenticated, user-facing side of the login
// flow. The anonymous side (init, poll, browser redirect) lives in the
// standalone loginflow service; these run inside OCS so the auth middleware
// establishes the user. They require an authenticated user but apply no
// authorization check. The responses are plain JSON, not the OCS envelope,
// because the web UI codes against that shape and branches on the HTTP status
// (404 unknown, 410 expired, 409 lost race).
//
// No CSRF Origin check is needed: the auth middleware authenticates these via
// the Authorization/token headers, which browsers never auto-attach on
// cross-origin requests, so a foreign page cannot forge an authenticated call
// (bearer-only auth).

const maxDeviceNameLen = 64

type loginFlowInfo struct {
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

// LoginFlowInfo handles GET /cloud/user/login-flow/{lt}.
func (h *Handler) LoginFlowInfo(w http.ResponseWriter, r *http.Request) {
	user := appctx.ContextMustGetUser(r.Context())

	ca, code := h.lookupAuthorization(r)
	if code != 0 {
		http.Error(w, "client authorization not found or expired", code)
		return
	}

	status := "pending"
	if ca.Approved() {
		status = "approved"
	}
	writeLoginFlowJSON(w, http.StatusOK, loginFlowInfo{
		Client:     loginflow.ClientDescription(ca.UserAgent),
		User:       user.GetUsername(),
		CreatedAt:  ca.CreatedAt.UTC().Format(time.RFC3339),
		ServerTime: time.Now().UTC().Format(time.RFC3339),
		Status:     status,
	})
}

// LoginFlowGrant handles POST /cloud/user/login-flow/{lt}/grant. It records the
// granting user and the device name; the app password is minted later, when the
// sync client polls.
func (h *Handler) LoginFlowGrant(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	user := appctx.ContextMustGetUser(r.Context())

	name, err := parseDeviceName(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ca, code := h.lookupAuthorization(r)
	if code != 0 {
		http.Error(w, "client authorization not found or expired", code)
		return
	}

	// name stays the user's chosen device name (may be empty). The human
	// description of the client is derived from the User-Agent at mint time.
	if err := h.loginFlowStore.Approve(r.Context(), ca.LoginHash, user.GetId().GetOpaqueId(), user.GetUsername(), name); err != nil {
		http.Error(w, "could not approve client authorization", loginFlowStatus(err))
		return
	}

	log.Info().Str("client_id", ca.ClientID).Str("username", user.GetUsername()).Msg("client authorization approved")
	writeLoginFlowJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

// LoginFlowDeny handles POST /cloud/user/login-flow/{lt}/deny.
func (h *Handler) LoginFlowDeny(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	ca, code := h.lookupAuthorization(r)
	if code != 0 {
		http.Error(w, "client authorization not found or expired", code)
		return
	}

	if err := h.loginFlowStore.Deny(r.Context(), ca.LoginHash); err != nil {
		http.Error(w, "could not deny client authorization", loginFlowStatus(err))
		return
	}

	log.Info().Str("client_id", ca.ClientID).Msg("client authorization denied")
	writeLoginFlowJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

// lookupAuthorization resolves the client authorization from the {lt} path
// parameter and maps the "gone" vs "unknown" distinction the web UI relies on:
// 404 unknown, 410 expired. A zero code means the authorization is live.
func (h *Handler) lookupAuthorization(r *http.Request) (*loginflow.ClientAuthorization, int) {
	lt := chi.URLParam(r, "lt")
	if lt == "" {
		return nil, http.StatusNotFound
	}
	ca, err := h.loginFlowStore.GetByLogin(r.Context(), loginflow.HashToken(lt))
	if err != nil {
		return nil, loginFlowStatus(err)
	}
	if ca.Expired() {
		return nil, http.StatusGone
	}
	return ca, 0
}

// parseDeviceName reads the optional {"name": "..."} body and validates it: at
// most 64 runes, no control characters.
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
	// "|" separates name, description and client_id inside the app password
	// label, so it cannot appear in a user-chosen name.
	if strings.ContainsRune(name, '|') {
		return "", fmt.Errorf("name contains a reserved character")
	}
	return name, nil
}

func loginFlowStatus(err error) int {
	switch err.(type) {
	case errtypes.NotFound:
		return http.StatusNotFound
	case errtypes.Conflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func writeLoginFlowJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
