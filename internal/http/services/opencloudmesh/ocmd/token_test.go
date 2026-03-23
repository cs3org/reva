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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	jwt "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc"
)

type tokenMockGW struct {
	gateway.GatewayAPIClient
	status *rpc.Status
	token  string
	user   *userpb.User
	err    error
}

func (m *tokenMockGW) Authenticate(_ context.Context, _ *gateway.AuthenticateRequest, _ ...grpc.CallOption) (*gateway.AuthenticateResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &gateway.AuthenticateResponse{
		Status: m.status,
		Token:  m.token,
		User:   m.user,
	}, nil
}

func setupTokenHandler(t *testing.T, statusCode rpc.Code, gwErr error) *tokenHandler {
	t.Helper()
	tokenmgr, err := jwt.New(map[string]any{"secret": "test-secret-for-token-handler", "expires": int64(3600)})
	if err != nil {
		t.Fatal(err)
	}

	u := &userpb.User{
		Id: &userpb.UserId{OpaqueId: "remote-user", Idp: "remote.example.com", Type: userpb.UserType_USER_TYPE_FEDERATED},
	}
	share := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share-abc"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
	}
	scopes, _ := scope.AddCodeFlowOCMShareScope(share, authpb.Role_ROLE_VIEWER, nil)
	mintedToken, _ := tokenmgr.MintToken(context.Background(), u, scopes)

	return &tokenHandler{
		gw: &tokenMockGW{
			status: &rpc.Status{Code: statusCode},
			token:  mintedToken,
			user:   u,
			err:    gwErr,
		},
		tokenmgr: tokenmgr,
	}
}

func postTokenForm(h *tokenHandler, grantType, code, clientID string) *httptest.ResponseRecorder {
	form := url.Values{}
	form.Set("grant_type", grantType)
	form.Set("code", code)
	form.Set("client_id", clientID)
	req := httptest.NewRequest(http.MethodPost, "/ocm/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ExchangeToken(rr, req)
	return rr
}

func TestExchangeTokenValid(t *testing.T) {
	h := setupTokenHandler(t, rpc.Code_CODE_OK, nil)
	rr := postTokenForm(h, "authorization_code", "code123", "nextcloud1.docker")

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	var resp tokenResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("token_type: got %q, want Bearer", resp.TokenType)
	}
	if resp.AccessToken == "" {
		t.Error("access_token should not be empty")
	}
	if resp.ExpiresIn <= 0 {
		t.Errorf("expires_in should be positive, got %d", resp.ExpiresIn)
	}
}

func TestExchangeTokenOcmShareGrant(t *testing.T) {
	h := setupTokenHandler(t, rpc.Code_CODE_OK, nil)
	rr := postTokenForm(h, "ocm_share", "code123", "nextcloud1.docker")

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestExchangeTokenUnsupportedGrant(t *testing.T) {
	h := setupTokenHandler(t, rpc.Code_CODE_OK, nil)
	rr := postTokenForm(h, "client_credentials", "code123", "nextcloud1.docker")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var errResp tokenErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Fatal(err)
	}
	if errResp.Error != "unsupported_grant_type" {
		t.Errorf("error: got %q, want unsupported_grant_type", errResp.Error)
	}
}

func TestExchangeTokenEmptyGrant(t *testing.T) {
	h := setupTokenHandler(t, rpc.Code_CODE_OK, nil)
	rr := postTokenForm(h, "", "code123", "nextcloud1.docker")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestExchangeTokenNotFound(t *testing.T) {
	h := setupTokenHandler(t, rpc.Code_CODE_NOT_FOUND, nil)
	rr := postTokenForm(h, "authorization_code", "bad-code", "nextcloud1.docker")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var errResp tokenErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Fatal(err)
	}
	if errResp.Error != "invalid_grant" {
		t.Errorf("error: got %q, want invalid_grant", errResp.Error)
	}
}

func TestExchangeTokenPermissionDenied(t *testing.T) {
	h := setupTokenHandler(t, rpc.Code_CODE_PERMISSION_DENIED, nil)
	rr := postTokenForm(h, "authorization_code", "bad-code", "nextcloud1.docker")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var errResp tokenErrorResponse
	_ = json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp.Error != "invalid_grant" {
		t.Errorf("error: got %q, want invalid_grant", errResp.Error)
	}
}

func TestExchangeTokenGatewayError(t *testing.T) {
	h := setupTokenHandler(t, rpc.Code_CODE_OK, errors.New("connection refused"))
	rr := postTokenForm(h, "authorization_code", "code123", "nextcloud1.docker")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	var errResp tokenErrorResponse
	_ = json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp.Error != "server_error" {
		t.Errorf("error: got %q, want server_error", errResp.Error)
	}
}
