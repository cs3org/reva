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
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

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

const ingestWebDAVPropfindXML = `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/</d:href>
    <d:propstat>
      <d:prop>
        <d:displayname>legacy-share</d:displayname>
        <d:getcontentlength>4</d:getcontentlength>
        <d:getcontenttype>text/plain</d:getcontenttype>
        <d:getetag>"abc"</d:getetag>
        <d:getlastmodified>Mon, 02 Jan 2006 15:04:05 GMT</d:getlastmodified>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

func ingestWebDAVHandler(t *testing.T, contacted *bool) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*contacted = true
		if r.Method != "PROPFIND" {
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(207)
		_, _ = w.Write([]byte(ingestWebDAVPropfindXML))
	})
}

func firstPublicIPv4() (string, bool) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", false
	}
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP == nil {
			continue
		}
		ip := n.IP.To4()
		if ip == nil || !isPublicIP(ip) {
			continue
		}
		return ip.String(), true
	}
	return "", false
}

func startPublicTLSServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	ip, ok := firstPublicIPv4()
	if !ok {
		t.Skip("no public IPv4 address available for public-only httptest")
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Skipf("cannot listen on public IP %s: %v", ip, err)
	}
	srv := httptest.NewUnstartedServer(h)
	if srv.Listener != nil {
		_ = srv.Listener.Close()
	}
	srv.Listener = ln
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func TestIngestWebDAVStatPublicHTTPSHostSucceeds(t *testing.T) {
	contacted := false
	srv := startPublicTLSServer(t, ingestWebDAVHandler(t, &contacted))

	info, err := statIngestWebDAV(srv.URL, "secret", 5*time.Second, true)
	if err != nil {
		t.Fatalf("ingest stat to a public HTTPS WebDAV host: %v", err)
	}
	if info == nil {
		t.Fatal("expected WebDAV stat info for a public HTTPS host")
	}
	if !contacted {
		t.Fatal("expected ingest stat to reach the public HTTPS WebDAV host")
	}
}

func TestIngestWebDAVStatRefusesPrivateHost(t *testing.T) {
	t.Run("metadata host is refused at dial", func(t *testing.T) {
		_, err := statIngestWebDAV("https://169.254.169.254/remote.php/dav/ocm", "secret", 5*time.Second, true)
		if err == nil {
			t.Fatal("expected ingest stat to refuse the metadata host at dial")
		}
		if !strings.Contains(err.Error(), "non-public") {
			t.Fatalf("got %v, want the existing non-public dial error", err)
		}
	})

	t.Run("reachable private host is not contacted", func(t *testing.T) {
		contacted := false
		srv := httptest.NewTLSServer(ingestWebDAVHandler(t, &contacted))
		defer srv.Close()

		_, err := statIngestWebDAV(srv.URL, "secret", 5*time.Second, true)
		if contacted {
			t.Fatal("ingest stat contacted a private WebDAV host; the untrusted transport must refuse it at dial")
		}
		if err == nil {
			t.Fatal("expected ingest stat to a private host to fail")
		}
		if !strings.Contains(err.Error(), "non-public") {
			t.Fatalf("got %v, want the existing non-public dial error", err)
		}
	})
}

func TestIngestWebDAVStatRefusesHTTP(t *testing.T) {
	contacted := false
	srv := httptest.NewServer(ingestWebDAVHandler(t, &contacted))
	defer srv.Close()

	_, err := statIngestWebDAV(srv.URL, "secret", 5*time.Second, true)
	if contacted {
		t.Fatal("ingest stat contacted an http WebDAV host; the untrusted transport must refuse it")
	}
	if err == nil {
		t.Fatal("expected ingest stat to refuse a non-https URL")
	}
	if !errors.Is(err, errPublicOnlyNonHTTPS) {
		t.Fatalf("got %v, want errPublicOnlyNonHTTPS", err)
	}
}
