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

package ocmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/token"
	tokenregistry "github.com/cs3org/reva/v3/pkg/token/manager/registry"
)

type tokenHandler struct {
	tokenmgr token.Manager
}

func (h *tokenHandler) init(c *config) error {
	tokenmgr, err := getTokenManager(c.TokenManager, c.TokenManagers)
	if err != nil {
		return err
	}
	h.tokenmgr = tokenmgr
	return nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type tokenErrorResponse struct {
	Error string `json:"error"`
}

// ExchangeToken handles POST /ocm/token (OCM code-flow token exchange).
func (h *tokenHandler) ExchangeToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	if err := r.ParseForm(); err != nil {
		writeTokenError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")

	switch grantType {
	case "authorization_code", "ocm_share":
		// Keep the legacy OCM grant name alongside the OAuth2-standard one while
		// partner stacks converge on the same token exchange contract.
	default:
		writeTokenError(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}
	if code == "" {
		writeTokenError(w, http.StatusBadRequest, "invalid_grant")
		return
	}

	gw, err := service.Gateway(ctx)
	if err != nil {
		log.Error().Err(err).Msg("token exchange: error getting gateway client")
		writeTokenError(w, http.StatusInternalServerError, "server_error")
		return
	}

	// client_id identifies the receiving server, but the exchanged code remains
	// the lookup key for the accepted share. Do not reinterpret client_id as a
	// share identifier.
	authRes, err := gw.Authenticate(ctx, &gateway.AuthenticateRequest{
		Type:         "ocmsharecode",
		ClientId:     clientID,
		ClientSecret: code,
	})

	switch {
	case err != nil:
		log.Error().Err(err).Msg("token exchange: gateway authenticate error")
		writeTokenError(w, http.StatusInternalServerError, "server_error")
		return
	case authRes.Status.Code == rpc.Code_CODE_NOT_FOUND ||
		authRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED:
		writeTokenError(w, http.StatusBadRequest, "invalid_grant")
		return
	case authRes.Status.Code == rpc.Code_CODE_UNAUTHENTICATED:
		log.Error().Interface("status", authRes.Status).Msg("token exchange: unexpected unauthenticated from authprovider")
		writeTokenError(w, http.StatusInternalServerError, "server_error")
		return
	case authRes.Status.Code != rpc.Code_CODE_OK:
		log.Error().Interface("status", authRes.Status).Msg("token exchange: unexpected status")
		writeTokenError(w, http.StatusInternalServerError, "server_error")
		return
	}

	// Derive expires_in from the validated expiry on the minted JWT
	expmgr, ok := h.tokenmgr.(token.ValidatedExpiry)
	if !ok {
		log.Error().Msg("token exchange: token manager does not implement ValidatedExpiry")
		writeTokenError(w, http.StatusInternalServerError, "server_error")
		return
	}
	expiresAt, err := expmgr.ValidatedExpiresAt(ctx, authRes.Token)
	if err != nil {
		log.Error().Err(err).Msg("token exchange: failed to derive expiry")
		writeTokenError(w, http.StatusInternalServerError, "server_error")
		return
	}
	expiresIn := max(0, expiresAt.Unix()-time.Now().Unix())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tokenResponse{
		AccessToken: authRes.Token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
	})
}

func writeTokenError(w http.ResponseWriter, status int, errCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(tokenErrorResponse{Error: errCode})
}

func getTokenManager(manager string, m map[string]map[string]any) (token.Manager, error) {
	if f, ok := tokenregistry.NewFuncs[manager]; ok {
		return f(m[manager])
	}
	return nil, fmt.Errorf("driver %s not found for token manager", manager)
}
