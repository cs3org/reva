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
	"strings"
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
	createResp  *ocmincoming.CreateOCMIncomingShareResponse
	createCalls int
	created     *ocmincoming.CreateOCMIncomingShareRequest
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

func (m *sharesMockGW) CreateOCMIncomingShare(_ context.Context, req *ocmincoming.CreateOCMIncomingShareRequest, _ ...grpc.CallOption) (*ocmincoming.CreateOCMIncomingShareResponse, error) {
	m.createCalls++
	m.created = req
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

	stampGateway(&sharesMockGW{
		createResp: &ocmincoming.CreateOCMIncomingShareResponse{
			Status: &rpc.Status{
				Code:    rpc.Code_CODE_INTERNAL,
				Message: "store failed",
			},
		},
	})
	h := &sharesHandler{}

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

// tlsOcmDiscoveryServer serves discovery over TLS with a self-signed cert.
func tlsOcmDiscoveryServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/ocm", func(w http.ResponseWriter, r *http.Request) {
		disco := wellknown.OcmDiscoveryData{
			Endpoint: "https://" + r.Host,
			ResourceTypes: []wellknown.ResourceTypes{
				{Name: "file", Protocols: map[string]any{"webdav": "/remote.php/dav/ocm"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(disco)
	})
	return httptest.NewTLSServer(mux)
}

// discovery must verify certs by default, and skip only when asked.
func TestDiscoverVerifiesTLSUnlessInsecure(t *testing.T) {
	srv := tlsOcmDiscoveryServer(t) // self-signed, https://127.0.0.1:port
	defer srv.Close()

	tests := []struct {
		name    string
		handler *sharesHandler
		wantErr bool
	}{
		{name: "verifies by default", handler: &sharesHandler{}, wantErr: true},
		{name: "skips when opted out", handler: &sharesHandler{ocmClientInsecure: true}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := tt.handler.discoverOcmResourceTypes(context.Background(), srv.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("discoverOcmResourceTypes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func webappReceiveHandler(targets []string) *sharesHandler {
	copied := append([]string{}, targets...)
	return &sharesHandler{webappReceiveTargets: &copied}
}

func postShare(t *testing.T, h *sharesHandler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/ocm/shares", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.15:12345"
	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)
	return rr
}

func shareBody(sender, resourceType string, protocol map[string]any) map[string]any {
	return map[string]any{
		"shareWith":    "marie@local.example.org",
		"name":         "test.txt",
		"providerId":   "provider-id",
		"owner":        "einstein@" + sender,
		"sender":       "einstein@" + sender,
		"shareType":    "user",
		"resourceType": resourceType,
		"protocol":     protocol,
	}
}

func webappOffer(uri string, requirements, targets []string) map[string]any {
	offer := map[string]any{
		"uri":          uri,
		"sharedSecret": "secret",
		"permissions":  []string{"read"},
		"requirements": requirements,
		"targets":      targets,
	}
	return map[string]any{"webapp": offer}
}

func TestCreateShareWebappValidation(t *testing.T) {
	disco := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(wellknown.OcmDiscoveryData{
			Endpoint: "http://" + r.Host + "/ocm",
			ResourceTypes: []wellknown.ResourceTypes{{
				Name: "file",
				Protocols: map[string]any{
					"webapp": "/apps/",
					"webdav": "/remote.php/dav/ocm",
				},
			}},
		})
	}))
	defer disco.Close()
	sender := disco.Listener.Addr().String()

	okCreate := &ocmincoming.CreateOCMIncomingShareResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}

	t.Run("compatible target is stored", func(t *testing.T) {
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(sender, "file", webappOffer(
			"https://app.example/hub",
			[]string{"must-exchange-token"},
			[]string{"blank"},
		)))
		if rr.Code != http.StatusCreated {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		if gw.createCalls != 1 {
			t.Fatalf("create calls %d", gw.createCalls)
		}
	})

	t.Run("relative uri resolves against advertised root", func(t *testing.T) {
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(sender, "file", webappOffer(
			"/hub",
			[]string{"must-exchange-token"},
			[]string{"blank"},
		)))
		if rr.Code != http.StatusCreated {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		if gw.createCalls != 1 || gw.created == nil {
			t.Fatalf("create calls %d", gw.createCalls)
		}
		opts := gw.created.Protocols[0].GetWebappOptions()
		wantURI := "http://" + sender + "/hub"
		if opts == nil || opts.Uri != wantURI {
			t.Fatalf("resolved uri %#v, want %s", opts, wantURI)
		}
	})

	cases := []struct {
		name     string
		protocol map[string]any
		want     string
	}{
		{
			name:     "empty targets",
			protocol: webappOffer("https://app.example/hub", []string{"must-exchange-token"}, []string{}),
			want:     "missing targets",
		},
		{
			name:     "no intersection",
			protocol: webappOffer("https://app.example/hub", []string{"must-exchange-token"}, []string{"iframe"}),
			want:     "no compatible target",
		},
		{
			name:     "missing uri",
			protocol: webappOffer(" ", []string{"must-exchange-token"}, []string{"blank"}),
			want:     "missing uri",
		},
		{
			name: "missing secret",
			protocol: map[string]any{"webapp": map[string]any{
				"uri": "https://app.example/hub", "sharedSecret": "",
				"permissions": []string{"read"}, "requirements": []string{"must-exchange-token"},
				"targets": []string{"blank"},
			}},
			want: "sharedSecret",
		},
		{
			name:     "malformed requirement",
			protocol: webappOffer("https://app.example/hub", []string{" must-exchange-token"}, []string{"blank"}),
			want:     "malformed requirement",
		},
		{
			name:     "absent must-exchange-token",
			protocol: webappOffer("https://app.example/hub", []string{"must-use-mfa"}, []string{"blank"}),
			want:     "must-exchange-token",
		},
		{
			name:     "malformed uri",
			protocol: webappOffer("http://http://evil.example/hub", []string{"must-exchange-token"}, []string{"blank"}),
			want:     "malformed",
		},
		{
			name:     "unknown mandatory requirement",
			protocol: webappOffer("https://app.example/hub", []string{"must-exchange-token", "must-sign"}, []string{"blank"}),
			want:     "unsupported requirement",
		},
		{
			name:     "must-use-mfa",
			protocol: webappOffer("https://app.example/hub", []string{"must-exchange-token", "must-use-mfa"}, []string{"blank"}),
			want:     "must-use-mfa",
		},
		{
			name: "mixed webdav and bad webapp",
			protocol: map[string]any{
				"webdav": map[string]any{
					"sharedSecret": "secret", "permissions": []string{"read"},
					"uri": "https://dav.example/file",
				},
				"webapp": map[string]any{
					"uri": "https://app.example/hub", "sharedSecret": "secret",
					"permissions": []string{"read"}, "requirements": []string{"must-exchange-token"},
					"targets": []string{"iframe"},
				},
			},
			want: "no compatible target",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			gw := &sharesMockGW{createResp: okCreate}
			stampGateway(gw)
			rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(sender, "file", tt.protocol))
			if rr.Code == http.StatusCreated {
				t.Fatalf("stored invalid share: %s", rr.Body.String())
			}
			if gw.createCalls != 0 {
				t.Fatalf("create calls %d body %s", gw.createCalls, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tt.want) {
				t.Fatalf("body %s", rr.Body.String())
			}
		})
	}

	t.Run("relative uri without receive base is not stored", func(t *testing.T) {
		noRoot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(wellknown.OcmDiscoveryData{
				Endpoint: "http://" + r.Host + "/ocm",
				ResourceTypes: []wellknown.ResourceTypes{{
					Name:      "file",
					Protocols: map[string]any{"webapp": map[string]any{"targets": []string{"blank"}}},
				}},
			})
		}))
		defer noRoot.Close()
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(
			noRoot.Listener.Addr().String(),
			"file",
			webappOffer("/hub", []string{"must-exchange-token"}, []string{"blank"}),
		))
		if rr.Code == http.StatusCreated || gw.createCalls != 0 {
			t.Fatalf("status %d calls %d body %s", rr.Code, gw.createCalls, rr.Body.String())
		}
	})

	t.Run("nil webapp does not panic", func(t *testing.T) {
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		rr := postShare(t, webappReceiveHandler([]string{"blank"}), shareBody(sender, "file", map[string]any{
			"webapp": nil,
		}))
		if gw.createCalls != 0 {
			t.Fatalf("create calls %d body %s", gw.createCalls, rr.Body.String())
		}
	})

	t.Run("duplicate webapp does not persist", func(t *testing.T) {
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		h := webappReceiveHandler([]string{"blank"})
		first := &Webapp{
			URI: "https://app.example/hub", SharedSecret: "secret",
			Permissions: []string{"read"}, Requirements: []string{"must-exchange-token"},
			Targets: []string{"blank"},
		}
		second := &Webapp{
			URI: "https://app.example/other", SharedSecret: "secret",
			Permissions: []string{"read"}, Requirements: []string{"must-exchange-token"},
			Targets: []string{"blank"},
		}
		// CreateShare persists only after getAndResolveProtocols returns protocols.
		// Duplicate JSON object keys collapse before that boundary, so this
		// passes two decoded webapp offers into the same builder.
		protos, legacy, err := h.getAndResolveProtocols(context.Background(), Protocols{first, second}, "file", sender)
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("err %v", err)
		}
		if protos != nil || legacy {
			t.Fatalf("protocols %#v legacy %v", protos, legacy)
		}
	})

	t.Run("webdav only still persists", func(t *testing.T) {
		gw := &sharesMockGW{createResp: okCreate}
		stampGateway(gw)
		rr := postShare(t, webappReceiveHandler(nil), shareBody(sender, "file", map[string]any{
			"webdav": map[string]any{
				"sharedSecret": "secret",
				"permissions":  []string{"read"},
				"uri":          "https://dav.example/file.txt",
			},
		}))
		if rr.Code != http.StatusCreated {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		if gw.createCalls != 1 {
			t.Fatalf("create calls %d", gw.createCalls)
		}
		if gw.created == nil || gw.created.Protocols[0].GetWebdavOptions() == nil {
			t.Fatal("expected webdav protocol to be stored")
		}
		if gw.created.Protocols[0].GetWebappOptions() != nil {
			t.Fatal("webdav share stored a webapp protocol")
		}
	})
}
