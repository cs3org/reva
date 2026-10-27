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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/ocm/httpsig"
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

func newInviteRequest(recipientProvider string) *http.Request {
	body := []byte(fmt.Sprintf(`{
		"userID":"remote-user",
		"email":"marie@example.org",
		"name":"Marie Curie",
		"recipientProvider":%q,
		"token":"invite-token"
	}`, recipientProvider))
	req := httptest.NewRequest(http.MethodPost, "/ocm/invite-accepted", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.20:12345"
	return req
}

func TestAcceptInviteSuccessSetsJSONHeaders(t *testing.T) {
	stampGateway(&invitesMockGW{
		acceptResp: &invitepb.AcceptInviteResponse{
			Status:      &rpc.Status{Code: rpc.Code_CODE_OK},
			UserId:      &userpb.UserId{OpaqueId: "local-user"},
			Email:       "marie@example.org",
			DisplayName: "Marie Curie",
		},
	})
	disco := ocmDiscoveryServer(t, "webdav", "file")
	defer disco.Close()
	h := &invitesHandler{ocmClient: NewPublicOnlyClientWithConfig(client.TransportConfig{AllowLoopback: true})}

	rr := httptest.NewRecorder()
	h.AcceptInvite(rr, newInviteRequest(disco.Listener.Addr().String()))

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
	stampGateway(&invitesMockGW{
		acceptResp: &invitepb.AcceptInviteResponse{
			Status: &rpc.Status{
				Code:    rpc.Code_CODE_INVALID_ARGUMENT,
				Message: "token invalid or not found",
			},
		},
	})
	disco := ocmDiscoveryServer(t, "webdav", "file")
	defer disco.Close()
	h := &invitesHandler{ocmClient: NewPublicOnlyClientWithConfig(client.TransportConfig{AllowLoopback: true})}

	rr := httptest.NewRecorder()
	h.AcceptInvite(rr, newInviteRequest(disco.Listener.Addr().String()))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("AcceptInvite() status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("token invalid or not found")) {
		t.Fatalf("AcceptInvite() body = %q, want backend invalid-argument message", rr.Body.String())
	}
}

func newSigningKey(t *testing.T) *httpsig.Key {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ocm.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := httpsig.LoadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// signingServer is a remote OCM server that advertises http-sig and publishes the public part of key.
func signingServer(t *testing.T, key *httpsig.Key) *httptest.Server {
	t.Helper()
	jwks, err := key.JWKS()
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/ocm", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(wellknown.OcmDiscoveryData{
			Endpoint: "http://" + r.Host,
			ResourceTypes: []wellknown.ResourceTypes{{
				Name:      "file",
				Protocols: map[string]any{"webdav": "/remote.php/dav/ocm"},
			}},
			Capabilities: []string{"http-sig"},
			JwksURI:      "http://" + r.Host + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jwks)
	})
	return httptest.NewServer(mux)
}

func TestAcceptInviteFromSigningServer(t *testing.T) {
	stampGateway(&invitesMockGW{
		acceptResp: &invitepb.AcceptInviteResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_OK},
			UserId: &userpb.UserId{OpaqueId: "local-user"},
		},
	})
	key := newSigningKey(t)
	remote := signingServer(t, key)
	defer remote.Close()
	h := &invitesHandler{ocmClient: NewPublicOnlyClientWithConfig(client.TransportConfig{AllowLoopback: true})}

	t.Run("signed", func(t *testing.T) {
		req := newInviteRequest(remote.Listener.Addr().String())
		body, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
		if err := key.Sign(req, body); err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		h.AcceptInvite(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("AcceptInvite() status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body)
		}
	})

	t.Run("unsigned", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.AcceptInvite(rr, newInviteRequest(remote.Listener.Addr().String()))
		if rr.Code != http.StatusForbidden {
			t.Fatalf("AcceptInvite() status = %d, want %d", rr.Code, http.StatusForbidden)
		}
	})
}
