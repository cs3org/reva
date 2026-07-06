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
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"google.golang.org/grpc"
)

// ocmDiscoveryServer starts a local httptest.Server that responds to
// /.well-known/ocm with a minimal OcmDiscoveryData payload advertising
// the given protocol for the given resource type.
// The caller must call server.Close() when done.
func ocmDiscoveryServer(t *testing.T, proto, resType string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/ocm", func(w http.ResponseWriter, r *http.Request) {
		// srv.URL is not yet known when we register the handler, so we
		// build the endpoint dynamically from the request.
		endpoint := fmt.Sprintf("http://%s", r.Host)
		disco := wellknown.OcmDiscoveryData{
			Endpoint: endpoint,
			ResourceTypes: []wellknown.ResourceTypes{
				{
					Name: resType,
					Protocols: map[string]any{
						proto: "/remote.php/dav/ocm",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(disco)
	})
	srv := httptest.NewServer(mux)
	return srv
}

// --- gateway mock ---

type sharesMockGW struct {
	gateway.GatewayAPIClient
	createResp *ocmincoming.CreateOCMIncomingShareResponse
}

func (m *sharesMockGW) IsProviderAllowed(context.Context, *ocmprovider.IsProviderAllowedRequest, ...grpc.CallOption) (*ocmprovider.IsProviderAllowedResponse, error) {
	return &ocmprovider.IsProviderAllowedResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}, nil
}

func (m *sharesMockGW) GetUser(context.Context, *userpb.GetUserRequest, ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	return &userpb.GetUserResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		User: &userpb.User{
			Id: &userpb.UserId{OpaqueId: "local-recipient", Idp: "local.example.org"},
		},
	}, nil
}

func (m *sharesMockGW) CreateOCMIncomingShare(context.Context, *ocmincoming.CreateOCMIncomingShareRequest, ...grpc.CallOption) (*ocmincoming.CreateOCMIncomingShareResponse, error) {
	return m.createResp, nil
}

func (m *sharesMockGW) GetAcceptedUser(context.Context, *invitepb.GetAcceptedUserRequest, ...grpc.CallOption) (*invitepb.GetAcceptedUserResponse, error) {
	return &invitepb.GetAcceptedUserResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}, nil
}

// --- tests ---

func TestCreateShareReturnsServerErrorForNonOKCreateStatus(t *testing.T) {
	// Start a local OCM discovery server so discoverOcmResourceTypes succeeds.
	disco := ocmDiscoveryServer(t, "webdav", "file")
	defer disco.Close()

	// The sender's Idp must equal the host:port of our local discovery server
	// so that discoverOcmResourceTypes calls it instead of the real internet.
	senderAddr := disco.Listener.Addr().String() // e.g. "127.0.0.1:54321"

	h := &sharesHandler{
		gatewayClient: &sharesMockGW{
			createResp: &ocmincoming.CreateOCMIncomingShareResponse{
				Status: &rpc.Status{
					Code:    rpc.Code_CODE_INTERNAL,
					Message: "store failed",
				},
			},
		},
	}

	body, _ := json.Marshal(map[string]any{
		"shareWith":    "marie@local.example.org",
		"name":         "test.txt",
		"providerId":   "provider-id",
		"owner":        fmt.Sprintf("einstein@%s", senderAddr),
		"sender":       fmt.Sprintf("einstein@%s", senderAddr),
		"shareType":    "user",
		"resourceType": "file",
		"protocol": map[string]any{
			"webdav": map[string]any{
				"sharedSecret": "secret",
				"permissions":  []string{"read"},
				"uri":          fmt.Sprintf("http://%s/remote.php/dav/files/einstein/test.txt", senderAddr),
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/ocm/shares", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.15:12345"

	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("CreateShare() status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestMatchesAutoAccept(t *testing.T) {
	h := &sharesHandler{
		autoAcceptProviders: []*regexp.Regexp{
			regexp.MustCompile(`^trusted\.example\.org$`),
			regexp.MustCompile(`\.cern\.ch$`),
		},
	}

	cases := map[string]bool{
		"trusted.example.org":      true,
		"data.cern.ch":             true,
		"sub.data.cern.ch":         true,
		"untrusted.example.org":    false,
		"trusted.example.org.evil": false,
		"cern.ch.evil":             false,
	}
	for domain, want := range cases {
		if got := h.matchesAutoAccept(domain); got != want {
			t.Errorf("matchesAutoAccept(%q) = %v, want %v", domain, got, want)
		}
	}

	// no configured providers -> never matches
	empty := &sharesHandler{}
	if empty.matchesAutoAccept("trusted.example.org") {
		t.Errorf("matchesAutoAccept with no providers should return false")
	}
}

func TestParseOCMUser(t *testing.T) {
	tests := []struct {
		name       string
		addr       string
		wantOpaque string
		wantIdp    string
		wantErr    bool
	}{
		{
			name:       "spec-conformant bare id",
			addr:       "marie@cernbox2.docker",
			wantOpaque: "marie",
			wantIdp:    "cernbox2.docker",
		},
		{
			name:       "oCIS doubled recipient host collapses (shareWith)",
			addr:       "cbcbcbcb-2222@cernbox2.docker@cernbox2.docker",
			wantOpaque: "cbcbcbcb-2222",
			wantIdp:    "cernbox2.docker",
		},
		{
			name:       "oCIS doubled remote host collapses (sender/owner)",
			addr:       "4c510ada-1234@ocis1.docker@ocis1.docker",
			wantOpaque: "4c510ada-1234",
			wantIdp:    "ocis1.docker",
		},
		{
			name:       "OpenCloud doubled remote host with scheme collapses (sender/owner)",
			addr:       "b1f74ec4-5678@https://opencloud1.docker@opencloud1.docker",
			wantOpaque: "b1f74ec4-5678",
			wantIdp:    "opencloud1.docker",
		},
		{
			name:       "single scheme-qualified host is stripped to bare id",
			addr:       "b1f74ec4-5678@https://opencloud1.docker",
			wantOpaque: "b1f74ec4-5678",
			wantIdp:    "opencloud1.docker",
		},
		{
			name:    "address without provider is rejected",
			addr:    "no-at-sign",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOCMUser(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseOCMUser(%q) expected error, got nil", tt.addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOCMUser(%q) unexpected error: %v", tt.addr, err)
			}
			if got.OpaqueId != tt.wantOpaque {
				t.Errorf("parseOCMUser(%q) OpaqueId = %q, want %q", tt.addr, got.OpaqueId, tt.wantOpaque)
			}
			if got.Idp != tt.wantIdp {
				t.Errorf("parseOCMUser(%q) Idp = %q, want %q", tt.addr, got.Idp, tt.wantIdp)
			}
		})
	}
}
