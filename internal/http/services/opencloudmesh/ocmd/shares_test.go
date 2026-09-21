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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/studio-b12/gowebdav"
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
	createResp     *ocmincoming.CreateOCMIncomingShareResponse
	rejectAccepted bool
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
	code := rpc.Code_CODE_OK
	if m.rejectAccepted {
		code = rpc.Code_CODE_NOT_FOUND
	}
	return &invitepb.GetAcceptedUserResponse{
		Status: &rpc.Status{Code: code},
	}, nil
}

func initSharesHandler(t *testing.T, c *config) *sharesHandler {
	t.Helper()
	if c == nil {
		c = &config{}
	}
	c.ApplyDefaults()
	h := &sharesHandler{}
	if err := h.init(c); err != nil {
		t.Fatal(err)
	}
	return h
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
	h := initSharesHandler(t, &config{AllowLoopbackFederation: true})

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
		conf    config
		wantErr bool
	}{
		{
			name:    "verifies by default",
			conf:    config{AllowLoopbackFederation: true},
			wantErr: true,
		},
		{
			name: "skips when opted out",
			conf: config{
				AllowLoopbackFederation: true,
				OCMClientInsecure:       true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := initSharesHandler(t, &tt.conf)
			_, _, err := h.discoverOcmResourceTypes(context.Background(), srv.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("discoverOcmResourceTypes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSharesHandlerInitCreatesPublicOnlyClient(t *testing.T) {
	h := initSharesHandler(t, &config{})
	if h.ocmClient == nil {
		t.Fatal("init() did not store a discovery client")
	}
	const wantDefaultTimeout = 10 * time.Second
	if h.ocmClient.client.Timeout != wantDefaultTimeout {
		t.Errorf("timeout = %v, want %v", h.ocmClient.client.Timeout, wantDefaultTimeout)
	}
	tr := client.HTTPTransport(h.ocmClient.client.Transport)
	if tr == nil {
		t.Fatalf("transport: got %T, want *http.Transport or public-only wrapper", h.ocmClient.client.Transport)
	}
	if tr.Proxy != nil {
		t.Fatal("default inbound discovery client must be public-only and must not use a proxy")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("default inbound discovery client must verify TLS")
	}
	if h.webdavTransport == nil {
		t.Fatal("init() did not store a WebDAV round tripper")
	}
	wtr := client.HTTPTransport(h.webdavTransport)
	if wtr == nil {
		t.Fatalf("transport: got %T, want *http.Transport or public-only wrapper", h.webdavTransport)
	}
	if wtr.Proxy != nil {
		t.Fatal("default inbound WebDAV round tripper must be public-only and must not use a proxy")
	}
	if wtr.TLSClientConfig == nil {
		t.Fatal("WebDAV TLSClientConfig is nil")
	}
	if wtr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("default inbound WebDAV round tripper must verify TLS")
	}
}

const (
	sharesProxyChildKey    = "OCMD_SHARES_TEST_PROXY_CHILD"
	sharesProxyAddrKey     = "OCMD_SHARES_TEST_PROXY_ADDR"
	sharesProxyTarget      = "https://192.168.1.1"
	sharesProxyCONNECTHost = "192.168.1.1:443"
)

// TestSharesHandlerInitUseEnvProxyWiring checks inbound discovery proxy
// wiring by observing CONNECT on a local 127.0.0.1:0 listener. The
// target is HTTPS RFC1918 so Go will not bypass the proxy the way it
// does for localhost. Direct discovery is rejected before a socket is
// created; the proxy case tunnels CONNECT through the local listener.
// Each case runs in a child because ProxyFromEnvironment snapshots
// HTTPS_PROXY once per process.
func TestSharesHandlerInitUseEnvProxyWiring(t *testing.T) {
	if scenario := os.Getenv(sharesProxyChildKey); scenario != "" {
		runSharesProxyChild(t, scenario)
		return
	}

	t.Run("zero config dials HTTPS non-loopback direct", func(t *testing.T) {
		spy := startCONNECTListener(t)
		runSharesProxyChildProcess(t, "direct", spy.addr())
		if seen := spy.seenRequests(); len(seen) != 0 {
			t.Fatalf("zero config observed CONNECT: %q", seen)
		}
	})

	t.Run("true config sends CONNECT through HTTPS_PROXY", func(t *testing.T) {
		spy := startCONNECTListener(t)
		runSharesProxyChildProcess(t, "proxy", spy.addr())
		want := "CONNECT " + sharesProxyCONNECTHost
		for _, got := range spy.seenRequests() {
			if got == want {
				return
			}
		}
		t.Fatalf("true config CONNECT requests = %q, want %q", spy.seenRequests(), want)
	})
}

type connectListener struct {
	ln   net.Listener
	mu   sync.Mutex
	seen []string
}

func startCONNECTListener(t *testing.T) *connectListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &connectListener{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go s.accept()
	return s
}

func (s *connectListener) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *connectListener) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	s.mu.Lock()
	s.seen = append(s.seen, req.Method+" "+req.Host)
	s.mu.Unlock()
	_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n"))
}

func (s *connectListener) addr() string {
	return s.ln.Addr().String()
}

func (s *connectListener) seenRequests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.seen))
	copy(out, s.seen)
	return out
}

func sharesProxyChildEnv(scenario, proxyAddr string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "http_proxy", "https_proxy", "no_proxy", "cgi_no_proxy":
			continue
		}
		if key == sharesProxyChildKey || key == sharesProxyAddrKey {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		sharesProxyChildKey+"="+scenario,
		sharesProxyAddrKey+"="+proxyAddr,
	)
}

func runSharesProxyChildProcess(t *testing.T, scenario, proxyAddr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run=^TestSharesHandlerInitUseEnvProxyWiring$",
		"-test.v=true",
	)
	cmd.Env = sharesProxyChildEnv(scenario, proxyAddr)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("proxy child %s timed out: %v\n%s", scenario, err, out)
	}
	if err != nil {
		t.Fatalf("proxy child %s failed: %v\n%s", scenario, err, out)
	}
}

func runSharesProxyChild(t *testing.T, scenario string) {
	t.Helper()
	proxyAddr := os.Getenv(sharesProxyAddrKey)
	if proxyAddr == "" {
		t.Fatal("child missing " + sharesProxyAddrKey)
	}
	t.Setenv("HTTPS_PROXY", "http://"+proxyAddr)

	conf := &config{}
	switch scenario {
	case "direct":
		// Zero config keeps Proxy nil, so HTTPS_PROXY must not CONNECT.
	case "proxy":
		// AllowLoopbackFederation is required so Control accepts the
		// 127.0.0.1 proxy hop; otherwise CONNECT never reaches the listener.
		conf = &config{
			OCMClientUseEnvProxy:    true,
			AllowLoopbackFederation: true,
		}
	default:
		t.Fatalf("unknown child scenario %q", scenario)
	}

	h := initSharesHandler(t, conf)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := h.discoverOcmResourceTypes(ctx, sharesProxyTarget)
	if scenario == "direct" && !errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("zero config error = %v, want ErrPolicyViolation", err)
	}
	if scenario == "proxy" && errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("true config denied the proxy hop: %v", err)
	}
}

func TestDiscoverOcmResourceTypesLoopbackPolicy(t *testing.T) {
	disco := ocmDiscoveryServer(t, "webdav", "file")
	defer disco.Close()

	tests := []struct {
		name    string
		conf    config
		wantErr bool
	}{
		{name: "fails by default", wantErr: true},
		{
			name: "succeeds with allow_loopback_federation",
			conf: config{AllowLoopbackFederation: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := initSharesHandler(t, &tt.conf)
			rts, endpoint, err := h.discoverOcmResourceTypes(context.Background(), disco.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("discoverOcmResourceTypes() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, client.ErrPolicyViolation) {
					t.Errorf("discoverOcmResourceTypes() error = %v, want ErrPolicyViolation", err)
				}
				return
			}
			if err != nil {
				return
			}
			if len(rts) == 0 {
				t.Fatal("expected advertised resource types")
			}
			if endpoint == "" {
				t.Fatal("expected discovery endpoint")
			}
		})
	}
}

func TestDiscoverOcmResourceTypesMalformedReturnsDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	h := initSharesHandler(t, &config{AllowLoopbackFederation: true})
	_, _, err := h.discoverOcmResourceTypes(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected discovery decode error")
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Errorf("error type = %T, want errtypes.InternalError", err)
	}
	if !strings.Contains(err.Error(), "Invalid payload on OCM discovery") {
		t.Errorf("error = %v, want Invalid payload on OCM discovery", err)
	}
}

func TestCreateShareUnauthorizedSenderFailsBeforeDiscovery(t *testing.T) {
	var hits int
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/ocm", func(w http.ResponseWriter, r *http.Request) {
		hits++
		endpoint := fmt.Sprintf("http://%s", r.Host)
		disco := wellknown.OcmDiscoveryData{
			Endpoint: endpoint,
			ResourceTypes: []wellknown.ResourceTypes{
				{
					Name: "file",
					Protocols: map[string]any{
						"webdav": "/remote.php/dav/ocm",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(disco)
	})
	disco := httptest.NewServer(mux)
	defer disco.Close()

	senderAddr := disco.Listener.Addr().String()
	stampGateway(&sharesMockGW{rejectAccepted: true})
	h := initSharesHandler(t, &config{AllowLoopbackFederation: true})

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

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("CreateShare() status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if hits != 0 {
		t.Fatalf("discovery hits = %d, want 0 (acceptance gate must run first)", hits)
	}
}

func TestSharesHandlerInitRespectsExplicitTimeout(t *testing.T) {
	h := initSharesHandler(t, &config{OCMClientTimeout: 3})
	if h.ocmClient.client.Timeout != 3*time.Second {
		t.Errorf("timeout = %v, want %v", h.ocmClient.client.Timeout, 3*time.Second)
	}
}

func TestConfigApplyDefaultsOCMClientTimeout(t *testing.T) {
	var c config
	c.ApplyDefaults()
	if c.OCMClientTimeout != 10 {
		t.Errorf("default timeout = %d, want 10", c.OCMClientTimeout)
	}
	c.OCMClientTimeout = 7
	c.ApplyDefaults()
	if c.OCMClientTimeout != 7 {
		t.Errorf("explicit timeout = %d, want 7 (default must not overwrite)", c.OCMClientTimeout)
	}
}

// dialOCMTransport dials the address through the real guarded transport; the
// guard admits allowed private ranges, so the dial proceeds to the network
// layer, which is expected to fail with a non-policy net.Error
// (timeout/refused/unreachable) for this unlikely destination.
func dialOCMTransport(t *testing.T, h *sharesHandler, address string) error {
	t.Helper()
	tr, ok := h.ocmClient.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport: got %T, want *http.Transport", h.ocmClient.client.Transport)
	}
	if tr.DialContext == nil {
		t.Fatal("inbound discovery transport must install a guarded DialContext")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	conn, err := tr.DialContext(ctx, "tcp", address)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}

// TestOcmServiceAllowedFederationCIDRs drives the native config map through
// service New so decode and init failures both propagate before the router is
// served. gatewaysvc is set so the required-field validator passes and the
// CIDR parse path is reached for the malformed/mixed cases.
func TestOcmServiceAllowedFederationCIDRs(t *testing.T) {
	tests := []struct {
		name      string
		cidrs     any
		setKey    bool
		wantErr   bool
		wantParse bool
	}{
		{name: "absent key succeeds", setKey: false, wantErr: false},
		{name: "empty list succeeds", cidrs: []any{}, setKey: true, wantErr: false},
		{name: "valid ipv4 succeeds", cidrs: []any{"10.197.228.0/24"}, setKey: true, wantErr: false},
		{name: "valid ula succeeds", cidrs: []any{"fd42:8c6d:7a10:23::/64"}, setKey: true, wantErr: false},
		{name: "valid ipv4 and ula succeeds", cidrs: []any{"10.197.228.0/24", "fd42:8c6d:7a10:23::/64"}, setKey: true, wantErr: false},
		{name: "scalar instead of list fails decode", cidrs: "10.0.0.0/8", setKey: true, wantErr: true},
		{name: "wrong element type fails decode", cidrs: []any{"10.0.0.0/8", 5}, setKey: true, wantErr: true},
		{name: "malformed cidr fails init", cidrs: []any{"not-a-cidr"}, setKey: true, wantErr: true, wantParse: true},
		{name: "public cidr fails init", cidrs: []any{"8.8.8.0/24"}, setKey: true, wantErr: true, wantParse: true},
		{name: "mixed valid invalid fails init atomically", cidrs: []any{"10.0.0.0/8", "8.8.8.0/24"}, setKey: true, wantErr: true, wantParse: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// gatewaysvc satisfies the required-field validator; token_managers
			// lets the token handler init succeed so success cases pass full New
			// and CIDR parse failures still surface from sharesHandler.init, which
			// runs first in routerInit.
			m := map[string]any{
				"gatewaysvc": "localhost:9100",
				"token_managers": map[string]any{
					"jwt": map[string]any{"secret": "ocm-test-jwt-secret"},
				},
			}
			if tt.setKey {
				m["allowed_federation_cidrs"] = tt.cidrs
			}
			svc, err := New(context.Background(), m)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("New() error = nil, want error")
				}
				if svc != nil {
					t.Fatalf("New() returned a service on error")
				}
				if tt.wantParse && !strings.Contains(err.Error(), "invalid federation CIDR") {
					t.Errorf("error = %v, want it to wrap the parser failure", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("New() error = %v, want nil", err)
			}
			if svc == nil {
				t.Fatal("New() returned nil service")
			}
		})
	}
}

// TestSharesHandlerInitDefaultDeniesPrivateDestinations confirms the stored
// inbound discovery client refuses RFC 1918 and ULA destinations by default.
func TestSharesHandlerInitDefaultDeniesPrivateDestinations(t *testing.T) {
	h := initSharesHandler(t, &config{})
	for _, addr := range []string{"10.0.0.5:9", "192.168.1.1:9", "172.16.5.4:9", "[fd00::1]:9"} {
		err := dialOCMTransport(t, h, addr)
		if !errors.Is(err, client.ErrPolicyViolation) {
			t.Errorf("default dial %q = %v, want ErrPolicyViolation", addr, err)
		}
	}
}

// TestSharesHandlerInitAllowedFederationCIDRsReachTransport confirms the parsed
// exception reaches the real guarded transport: configured private ranges pass
// the guard, while unrelated private ranges stay denied.
func TestSharesHandlerInitAllowedFederationCIDRsReachTransport(t *testing.T) {
	h := initSharesHandler(t, &config{
		AllowedFederationCIDRs: []string{"10.50.0.0/16", "fd42:8c6d:7a10:23::/64"},
	})
	// Configured private ranges are admitted past the guard, then the dial
	// proceeds to the network layer and must fail with a non-policy net.Error
	// (timeout/refused/unreachable). A nil error (successful handshake) must
	// FAIL: it would mean the guard admitted AND a real service answered on
	// this unlikely destination. A non-policy net.Error PASSes (guard admitted,
	// network failed - expected).
	for _, addr := range []string{"10.50.1.1:9", "[fd42:8c6d:7a10:23::1]:9"} {
		err := dialOCMTransport(t, h, addr)
		if err == nil {
			t.Fatalf("configured CIDR dial %q = nil; want a non-policy net.Error (guard admitted, network failed)", addr)
		}
		if errors.Is(err, client.ErrPolicyViolation) {
			t.Fatalf("configured CIDR dial %q = %v; want it to pass the guard (non-policy net.Error)", addr, err)
		}
	}
	// Unrelated private ranges stay denied even with a configured exception.
	for _, addr := range []string{"10.9.9.9:9", "192.168.1.1:9", "172.16.5.4:9", "[fd00::1]:9"} {
		err := dialOCMTransport(t, h, addr)
		if !errors.Is(err, client.ErrPolicyViolation) {
			t.Errorf("unrelated private dial %q = %v, want ErrPolicyViolation", addr, err)
		}
	}
}

func webDAVPropfindXML(isDir bool) string {
	resourceType := "<d:resourcetype/>"
	if isDir {
		resourceType = `<d:resourcetype><d:collection/></d:resourcetype>`
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/</d:href>
    <d:propstat>
      <d:prop>
        <d:displayname>item</d:displayname>
        ` + resourceType + `
        <d:getcontentlength>1</d:getcontentlength>
        <d:getcontenttype>text/plain</d:getcontenttype>
        <d:getetag>"e"</d:getetag>
        <d:getlastmodified>Mon, 01 Jan 2024 00:00:00 GMT</d:getlastmodified>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`
}

func writeWebDAVPropfind(w http.ResponseWriter, isDir bool) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(webDAVPropfindXML(isDir)))
}

func legacyCreateShareBody(senderAddr string) []byte {
	body, _ := json.Marshal(map[string]any{
		"shareWith":    "marie@local.example.org",
		"name":         "test.txt",
		"providerId":   "provider-id",
		"owner":        fmt.Sprintf("einstein@%s", senderAddr),
		"sender":       fmt.Sprintf("einstein@%s", senderAddr),
		"shareType":    "user",
		"resourceType": "file",
		"protocol": map[string]any{
			"name": "ocm10format",
			"options": map[string]any{
				"sharedSecret": "secret",
			},
		},
	})
	return body
}

func discoveryAndDAVServer(t *testing.T, discoHits, davHits *int, isDir bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/ocm", func(w http.ResponseWriter, r *http.Request) {
		if discoHits != nil {
			*discoHits++
		}
		disco := wellknown.OcmDiscoveryData{
			Endpoint: fmt.Sprintf("http://%s", r.Host),
			ResourceTypes: []wellknown.ResourceTypes{
				{Name: "file", Protocols: map[string]any{"webdav": "/remote.php/dav/ocm"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(disco)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			http.NotFound(w, r)
			return
		}
		if davHits != nil {
			*davHits++
		}
		writeWebDAVPropfind(w, isDir)
	})
	return httptest.NewServer(mux)
}

func TestLegacyWebDAVStatPublicOnlyPolicy(t *testing.T) {
	t.Run("rejects loopback by default before the server receives a request", func(t *testing.T) {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			writeWebDAVPropfind(w, false)
		}))
		defer srv.Close()

		h := initSharesHandler(t, &config{})
		c := gowebdav.NewClient(srv.URL, "", "")
		c.SetTransport(h.webdavTransport)
		_, err := c.Stat("")
		if err == nil {
			t.Fatal("expected Stat to refuse loopback")
		}
		if !errors.Is(err, client.ErrPolicyViolation) {
			t.Errorf("Stat() error = %v, want ErrPolicyViolation", err)
		}
		if hits != 0 {
			t.Fatalf("server hits = %d, want 0", hits)
		}
	})

	t.Run("succeeds with allow_loopback_federation", func(t *testing.T) {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			writeWebDAVPropfind(w, false)
		}))
		defer srv.Close()

		h := initSharesHandler(t, &config{AllowLoopbackFederation: true})
		c := gowebdav.NewClient(srv.URL, "", "")
		c.SetTransport(h.webdavTransport)
		info, err := c.Stat("")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if info.IsDir() {
			t.Fatal("expected a file resource")
		}
		if hits == 0 {
			t.Fatal("expected the WebDAV server to receive Stat")
		}
	})

	t.Run("allow_loopback_federation does not permit RFC1918", func(t *testing.T) {
		h := initSharesHandler(t, &config{AllowLoopbackFederation: true})
		c := gowebdav.NewClient("http://192.168.1.1:9/", "", "")
		c.SetTransport(h.webdavTransport)
		_, err := c.Stat("")
		if err == nil {
			t.Fatal("expected Stat to refuse RFC1918")
		}
		if !errors.Is(err, client.ErrPolicyViolation) {
			t.Errorf("Stat() error = %v, want ErrPolicyViolation", err)
		}
	})
}

func TestCreateShareLegacyWebDAVStatUsesStoredTransport(t *testing.T) {
	t.Run("default probe is blocked before the dav server receives a request", func(t *testing.T) {
		var discoHits, davHits int
		srv := discoveryAndDAVServer(t, &discoHits, &davHits, true)
		defer srv.Close()

		h := initSharesHandler(t, &config{})
		h.ocmClient = NewPublicOnlyClientWithConfig(client.TransportConfig{
			Timeout:       10 * time.Second,
			AllowLoopback: true,
		})
		stampGateway(&sharesMockGW{
			createResp: &ocmincoming.CreateOCMIncomingShareResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
			},
		})

		req := httptest.NewRequest(http.MethodPost, "/ocm/shares", bytes.NewReader(legacyCreateShareBody(srv.Listener.Addr().String())))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.0.2.15:12345"
		rr := httptest.NewRecorder()
		h.CreateShare(rr, req)

		if discoHits == 0 {
			t.Fatal("discovery must run so the Stat probe is reached")
		}
		if davHits != 0 {
			t.Fatalf("dav hits = %d, want 0 (probe must refuse loopback)", davHits)
		}
	})

	t.Run("allow_loopback_federation stats the remote resource", func(t *testing.T) {
		var davHits int
		srv := discoveryAndDAVServer(t, nil, &davHits, true)
		defer srv.Close()

		h := initSharesHandler(t, &config{AllowLoopbackFederation: true})
		stampGateway(&sharesMockGW{
			createResp: &ocmincoming.CreateOCMIncomingShareResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
			},
		})

		req := httptest.NewRequest(http.MethodPost, "/ocm/shares", bytes.NewReader(legacyCreateShareBody(srv.Listener.Addr().String())))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.0.2.15:12345"
		rr := httptest.NewRecorder()
		h.CreateShare(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("CreateShare() status = %d, want %d", rr.Code, http.StatusCreated)
		}
		if davHits == 0 {
			t.Fatal("expected the legacy WebDAV Stat probe to reach the server")
		}
	})
}

// TestSharesHandlerOCMAndWebDAVShareParsedPolicy checks that inbound discovery
// and the legacy WebDAV probe admit and deny the same private ranges.
func TestSharesHandlerOCMAndWebDAVShareParsedPolicy(t *testing.T) {
	h := initSharesHandler(t, &config{
		AllowedFederationCIDRs: []string{"10.50.0.0/16", "fd42:8c6d:7a10:23::/64"},
	})
	admitted := []string{"10.50.1.1:9", "[fd42:8c6d:7a10:23::1]:9"}
	denied := []string{"10.9.9.9:9", "192.168.1.1:9", "[fd00::1]:9"}
	for _, addr := range admitted {
		err := dialOCMTransport(t, h, addr)
		if err == nil || errors.Is(err, client.ErrPolicyViolation) {
			t.Fatalf("discovery dial %q = %v; want a non-policy network error", addr, err)
		}
		err = dialSharesRoundTripper(t, h.webdavTransport, addr)
		if err == nil || errors.Is(err, client.ErrPolicyViolation) {
			t.Fatalf("webdav dial %q = %v; want a non-policy network error", addr, err)
		}
	}
	for _, addr := range denied {
		if err := dialOCMTransport(t, h, addr); !errors.Is(err, client.ErrPolicyViolation) {
			t.Errorf("discovery dial %q = %v, want ErrPolicyViolation", addr, err)
		}
		if err := dialSharesRoundTripper(t, h.webdavTransport, addr); !errors.Is(err, client.ErrPolicyViolation) {
			t.Errorf("webdav dial %q = %v, want ErrPolicyViolation", addr, err)
		}
	}
}

func dialSharesRoundTripper(t *testing.T, rt http.RoundTripper, address string) error {
	t.Helper()
	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("transport: got %T, want *http.Transport", rt)
	}
	if tr.DialContext == nil {
		t.Fatal("transport must install a guarded DialContext")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	conn, err := tr.DialContext(ctx, "tcp", address)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}
