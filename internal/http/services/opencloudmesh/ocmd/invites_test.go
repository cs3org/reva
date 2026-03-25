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
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"google.golang.org/grpc"
)

type invitesMockGW struct {
	gateway.GatewayAPIClient
	acceptResp *invitepb.AcceptInviteResponse
}

func (m *invitesMockGW) IsProviderAllowed(context.Context, *ocmprovider.IsProviderAllowedRequest, ...grpc.CallOption) (*ocmprovider.IsProviderAllowedResponse, error) {
	return &ocmprovider.IsProviderAllowedResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}, nil
}

func (m *invitesMockGW) AcceptInvite(context.Context, *invitepb.AcceptInviteRequest, ...grpc.CallOption) (*invitepb.AcceptInviteResponse, error) {
	return m.acceptResp, nil
}

func newInviteRequest() *http.Request {
	body := []byte(`{
		"userID":"remote-user",
		"email":"marie@example.org",
		"name":"Marie Curie",
		"recipientProvider":"remote.example.org",
		"token":"invite-token"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/ocm/invite-accepted", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.20:12345"
	return req
}

func TestAcceptInviteSuccessSetsJSONHeaders(t *testing.T) {
	h := &invitesHandler{
		gatewayClient: &invitesMockGW{
			acceptResp: &invitepb.AcceptInviteResponse{
				Status:      &rpc.Status{Code: rpc.Code_CODE_OK},
				UserId:      &userpb.UserId{OpaqueId: "local-user"},
				Email:       "marie@example.org",
				DisplayName: "Marie Curie",
			},
		},
	}

	rr := httptest.NewRecorder()
	h.AcceptInvite(rr, newInviteRequest())

	if rr.Code != http.StatusOK {
		t.Fatalf("AcceptInvite() status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("AcceptInvite() Content-Type = %q, want application/json", got)
	}

	var resp RemoteUser
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("AcceptInvite() response body is not valid JSON: %v", err)
	}
	if resp.UserID != "local-user" {
		t.Fatalf("AcceptInvite() userID = %q, want local-user", resp.UserID)
	}
}

func TestAcceptInviteInvalidArgumentUsesBackendMessage(t *testing.T) {
	h := &invitesHandler{
		gatewayClient: &invitesMockGW{
			acceptResp: &invitepb.AcceptInviteResponse{
				Status: &rpc.Status{
					Code:    rpc.Code_CODE_INVALID_ARGUMENT,
					Message: "token invalid or not found",
				},
			},
		},
	}

	rr := httptest.NewRecorder()
	h.AcceptInvite(rr, newInviteRequest())

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("AcceptInvite() status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("token invalid or not found")) {
		t.Fatalf("AcceptInvite() body = %q, want backend invalid-argument message", rr.Body.String())
	}
}
