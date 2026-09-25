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

package sciencemesh

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

type fakeDiscoverer struct {
	calls        []string
	byURL        map[string]*wellknown.OcmDiscoveryData
	fallback     ocmDiscoverer
	fallbackErrs map[string]error
}

func (f *fakeDiscoverer) Discover(ctx context.Context, endpoint string) (*wellknown.OcmDiscoveryData, error) {
	f.calls = append(f.calls, endpoint)
	if disco, ok := f.byURL[endpoint]; ok {
		return disco, nil
	}
	if f.fallback != nil {
		disco, err := f.fallback.Discover(ctx, endpoint)
		if err != nil {
			if f.fallbackErrs == nil {
				f.fallbackErrs = map[string]error{}
			}
			f.fallbackErrs[endpoint] = err
		}
		return disco, err
	}
	return nil, errors.New("unexpected discovery endpoint: " + endpoint)
}

func requireFallbackPolicyViolation(t *testing.T, fake *fakeDiscoverer, endpoint string) {
	t.Helper()
	err, ok := fake.fallbackErrs[endpoint]
	if !ok {
		t.Fatalf("no recorded discovery error for %q", endpoint)
	}
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("discovery error for %q = %v, want ErrPolicyViolation", endpoint, err)
	}
}

func serveDirectory(t *testing.T, hits *atomic.Int32, federation string, servers []ocmd.DirectoryServiceServer) *httptest.Server {
	t.Helper()
	if servers == nil {
		servers = []ocmd.DirectoryServiceServer{}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ocmd.DirectoryService{
			Federation: federation,
			Servers:    servers,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func countingTarget(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Error("blocked target received a request")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func trustedClient() *ocmd.OCMClient {
	return ocmd.NewClient(5*time.Second, true)
}

func publicOnlyClient() *ocmd.OCMClient {
	return ocmd.NewPublicOnlyClient(5*time.Second, true)
}

func publicOnlyClientWithCIDRs(t *testing.T, timeout time.Duration, cidrs ...string) *ocmd.OCMClient {
	t.Helper()
	parsed, err := client.ParseFederationCIDRs(cidrs)
	if err != nil {
		t.Fatalf("ParseFederationCIDRs: %v", err)
	}
	return ocmd.NewPublicOnlyClientWithConfig(client.TransportConfig{
		Timeout:                timeout,
		Insecure:               true,
		AllowedFederationCIDRs: parsed,
	})
}

func TestFetchInfoSkipsLoopbackListedProviderBeforeRequest(t *testing.T) {
	t.Parallel()

	blocked, hits := countingTarget(t)
	dir := serveDirectory(t, nil, "loopback-fed", []ocmd.DirectoryServiceServer{
		{DisplayName: "loopback", URL: blocked.URL},
	})

	fake := &fakeDiscoverer{
		fallback: publicOnlyClient(),
	}
	h := &wayfHandler{
		ocmClient:       trustedClient(),
		untrustedClient: fake,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h.fetchInfo(ctx, []string{dir.URL})

	if hits.Load() != 0 {
		t.Fatalf("loopback listed provider received %d requests, want 0", hits.Load())
	}
	requireFallbackPolicyViolation(t, fake, blocked.URL)
	if len(h.directoryServices) != 0 {
		t.Fatalf("directoryServices = %#v, want loopback entry skipped", h.directoryServices)
	}
}

func TestFetchInfoSkipsRFC1918ListedProvider(t *testing.T) {
	t.Parallel()

	rfc1918URL := "http://192.168.1.50"
	dir := serveDirectory(t, nil, "rfc1918-fed", []ocmd.DirectoryServiceServer{
		{DisplayName: "private", URL: rfc1918URL},
	})

	fake := &fakeDiscoverer{
		fallback: publicOnlyClient(),
	}
	h := &wayfHandler{
		ocmClient:       trustedClient(),
		untrustedClient: fake,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h.fetchInfo(ctx, []string{dir.URL})

	requireFallbackPolicyViolation(t, fake, rfc1918URL)
	if len(h.directoryServices) != 0 {
		t.Fatalf("directoryServices = %#v, want RFC1918 entry skipped", h.directoryServices)
	}
}

func TestFetchInfoKeepsOtherEntriesWhenOneIsBlocked(t *testing.T) {
	t.Parallel()

	blocked, hits := countingTarget(t)
	goodURL := "https://good.example"
	dir := serveDirectory(t, nil, "mixed-fed", []ocmd.DirectoryServiceServer{
		{DisplayName: "blocked", URL: blocked.URL},
		{DisplayName: "ok", URL: goodURL},
	})

	fake := &fakeDiscoverer{
		byURL: map[string]*wellknown.OcmDiscoveryData{
			goodURL: {InviteAcceptDialog: "/accept"},
		},
		fallback: publicOnlyClient(),
	}
	h := &wayfHandler{
		ocmClient:       trustedClient(),
		untrustedClient: fake,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h.fetchInfo(ctx, []string{dir.URL})

	if hits.Load() != 0 {
		t.Fatalf("blocked listed provider received %d requests, want 0", hits.Load())
	}
	if len(h.directoryServices) != 1 {
		t.Fatalf("directoryServices len = %d, want 1", len(h.directoryServices))
	}
	servers := h.directoryServices[0].Servers
	if len(servers) != 1 {
		t.Fatalf("servers = %#v, want only the unblocked entry", servers)
	}
	if servers[0].DisplayName != "ok" || servers[0].URL != goodURL {
		t.Fatalf("kept server = %#v, want ok at %s", servers[0], goodURL)
	}
	if servers[0].InviteAcceptDialog != "https://good.example/accept" {
		t.Fatalf("InviteAcceptDialog = %q, want absolute good.example URL", servers[0].InviteAcceptDialog)
	}
}

func TestFetchInfoUsesTrustedClientForConfiguredDirectory(t *testing.T) {
	t.Parallel()

	var dirHits atomic.Int32
	goodURL := "https://listed.example"
	dir := serveDirectory(t, &dirHits, "trusted-fed", []ocmd.DirectoryServiceServer{
		{DisplayName: "listed", URL: goodURL},
	})

	fake := &fakeDiscoverer{
		byURL: map[string]*wellknown.OcmDiscoveryData{
			goodURL: {InviteAcceptDialog: "https://listed.example/accept"},
		},
	}
	h := &wayfHandler{
		ocmClient:       trustedClient(),
		untrustedClient: fake,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h.fetchInfo(ctx, []string{dir.URL})

	if dirHits.Load() == 0 {
		t.Fatal("configured directory URL was not fetched by the trusted client")
	}
	for _, call := range fake.calls {
		if call == dir.URL {
			t.Fatalf("untrusted Discover was called with the directory URL %q", dir.URL)
		}
	}
	if len(fake.calls) != 1 || fake.calls[0] != goodURL {
		t.Fatalf("untrusted Discover calls = %v, want [%s]", fake.calls, goodURL)
	}
	if len(h.directoryServices) != 1 || len(h.directoryServices[0].Servers) != 1 {
		t.Fatalf("directoryServices = %#v, want the listed provider kept", h.directoryServices)
	}
}

func TestDiscoverProviderUsesUntrustedClient(t *testing.T) {
	t.Parallel()

	fake := &fakeDiscoverer{
		byURL: map[string]*wellknown.OcmDiscoveryData{
			"https://peer.example": {InviteAcceptDialog: "https://peer.example/accept"},
		},
	}
	h := &wayfHandler{untrustedClient: fake}

	req := httptest.NewRequest(http.MethodPost, "/discover", strings.NewReader(`{"domain":"peer.example"}`))
	rec := httptest.NewRecorder()
	h.DiscoverProvider(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(fake.calls) != 1 || fake.calls[0] != "https://peer.example" {
		t.Fatalf("untrusted Discover calls = %v, want [https://peer.example]", fake.calls)
	}

	var got DiscoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.InviteAcceptDialog != "https://peer.example/accept" {
		t.Fatalf("InviteAcceptDialog = %q, want https://peer.example/accept", got.InviteAcceptDialog)
	}
}

func TestDiscoverProviderRejectsLoopbackBeforeRequest(t *testing.T) {
	t.Parallel()

	blocked, hits := countingTarget(t)
	fake := &fakeDiscoverer{
		fallback: publicOnlyClient(),
	}
	h := &wayfHandler{untrustedClient: fake}

	req := httptest.NewRequest(http.MethodPost, "/discover", strings.NewReader(`{"domain":"`+blocked.URL+`"}`))
	rec := httptest.NewRecorder()
	h.DiscoverProvider(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s, want 404", rec.Code, rec.Body.String())
	}
	if hits.Load() != 0 {
		t.Fatalf("loopback discover target received %d requests, want 0", hits.Load())
	}
	requireFallbackPolicyViolation(t, fake, blocked.URL)
}

func TestInitRejectsInvalidFederationCIDRsNoDirectoryRequest(t *testing.T) {
	t.Parallel()

	// A real directory server with a hit counter proves init never fetches
	// it when the CIDR config is invalid: the helper runs before any startup
	// network I/O.
	var dirHits atomic.Int32
	dir := serveDirectory(t, &dirHits, "fed", []ocmd.DirectoryServiceServer{
		{DisplayName: "listed", URL: "https://listed.example"},
	})

	c := &config{
		DirectoryServiceURLs:   dir.URL,
		OCMClientTimeout:       2,
		OCMClientInsecure:      true,
		AllowedFederationCIDRs: []string{"10.0.0.0/8", "nope"},
	}
	h := new(wayfHandler)
	err := h.init(c)
	if err == nil {
		t.Fatal("init: expected error for invalid allowed_federation_cidrs")
	}
	if !strings.Contains(err.Error(), invalidFederationCIDRText) {
		t.Fatalf("init error = %v, want %q in message", err, invalidFederationCIDRText)
	}
	if dirHits.Load() != 0 {
		t.Fatalf("directory server received %d requests, want 0 (config failure must abort before fetch)", dirHits.Load())
	}
}

func TestInitRejectsInvalidFederationCIDRsEmptyDirectory(t *testing.T) {
	t.Parallel()

	// The critical early-return guard: invalid CIDRs abort initialization even
	// when the directory list is empty. The helper runs before the
	// empty-directory return at wayf.go, so a bad config cannot disappear.
	c := &config{
		DirectoryServiceURLs:   "",
		OCMClientTimeout:       2,
		OCMClientInsecure:      true,
		AllowedFederationCIDRs: []string{"8.8.8.8/32"},
	}
	h := new(wayfHandler)
	err := h.init(c)
	if err == nil {
		t.Fatal("init: expected error for invalid allowed_federation_cidrs with empty directory list")
	}
	if !strings.Contains(err.Error(), invalidFederationCIDRText) {
		t.Fatalf("init error = %v, want %q in message", err, invalidFederationCIDRText)
	}
}

func TestInitRejectsScalarFederationCIDRs(t *testing.T) {
	t.Parallel()

	var c config
	err := cfg.Decode(map[string]any{
		"gatewaysvc":               "grpc:0",
		"mesh_directory_url":       "https://dir.example",
		"provider_domain":          "example.org",
		"allowed_federation_cidrs": "10.0.0.0/8",
	}, &c)
	if err == nil {
		t.Fatal("decode: expected error for scalar allowed_federation_cidrs")
	}
	if strings.Contains(err.Error(), invalidFederationCIDRText) {
		t.Fatalf("scalar decode error = %v, must not be the CIDR parse error", err)
	}
}

func TestInitRejectsMixedValueFederationCIDRs(t *testing.T) {
	t.Parallel()

	var c config
	err := cfg.Decode(map[string]any{
		"gatewaysvc":               "grpc:0",
		"mesh_directory_url":       "https://dir.example",
		"provider_domain":          "example.org",
		"allowed_federation_cidrs": []any{"10.0.0.0/8", 42},
	}, &c)
	if err == nil {
		t.Fatal("decode: expected error for mixed-value allowed_federation_cidrs")
	}
	if strings.Contains(err.Error(), invalidFederationCIDRText) {
		t.Fatalf("mixed-value decode error = %v, must not be the CIDR parse error", err)
	}
}

func TestInitAcceptsValidFederationCIDRs(t *testing.T) {
	t.Parallel()

	c := &config{
		DirectoryServiceURLs:   "",
		OCMClientTimeout:       2,
		OCMClientInsecure:      true,
		AllowedFederationCIDRs: []string{"10.1.2.0/24", "fd12:3456:789a::/48"},
	}
	h := new(wayfHandler)
	if err := h.init(c); err != nil {
		t.Fatalf("init: %v", err)
	}
	// The helper output flows into NewPublicOnlyClientWithConfig, so the
	// untrusted client is a real *ocmd.OCMClient (not a stub). The trusted
	// directory client is also a real *ocmd.OCMClient.
	if _, ok := h.untrustedClient.(*ocmd.OCMClient); !ok {
		t.Fatalf("untrustedClient: got %T, want *ocmd.OCMClient", h.untrustedClient)
	}
	if h.ocmClient == nil {
		t.Fatal("trusted ocmClient must be initialized")
	}
}

func TestInitConfiguredPolicyDeniesUnlistedRangeForListedProvider(t *testing.T) {
	t.Parallel()

	// Configure an explicit 10.1.2.0/24 exception. The operator-configured
	// directory is fetched by the trusted client (loopback allowed); the
	// listed provider at an unrelated RFC1918 address is discovered by the
	// public-only client and denied by the configured policy. This preserves
	// the trusted-directory split while applying the configured public policy
	// to listed providers.
	var dirHits atomic.Int32
	dir := serveDirectory(t, &dirHits, "fed", []ocmd.DirectoryServiceServer{
		{DisplayName: "private", URL: "http://192.168.1.50"},
	})

	c := &config{
		DirectoryServiceURLs:   dir.URL,
		OCMClientTimeout:       2,
		OCMClientInsecure:      true,
		AllowedFederationCIDRs: []string{"10.1.2.0/24"},
	}
	h := new(wayfHandler)
	if err := h.init(c); err != nil {
		t.Fatalf("init: %v", err)
	}
	if dirHits.Load() == 0 {
		t.Fatal("configured directory URL was not fetched by the trusted client")
	}
	if len(h.directoryServices) != 0 {
		t.Fatalf("directoryServices = %#v, want the unlisted-range listed provider skipped", h.directoryServices)
	}
	// Direct Discover proves the configured policy rejected the unlisted
	// range. fetchInfo skips discovery errors, so an empty provider list
	// alone cannot tell policy rejection from a network failure.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := h.untrustedClient.Discover(ctx, "http://192.168.1.50")
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("discovery error for %q = %v, want ErrPolicyViolation", "http://192.168.1.50", err)
	}
}

func TestDiscoverProviderSharesConfiguredPublicPolicy(t *testing.T) {
	t.Parallel()

	// Request-supplied /discover domains go through the configured public
	// policy. An out-of-range RFC1918 domain is denied by policy; an in-range
	// domain passes the guard and then fails to connect with a non-policy
	// error. This exercises the configured policy on the request-supplied path
	// without duplicating H1's normalization rules.
	denied := &fakeDiscoverer{fallback: publicOnlyClientWithCIDRs(t, time.Second, "10.1.2.0/24")}
	hDenied := &wayfHandler{untrustedClient: denied}
	req := httptest.NewRequest(http.MethodPost, "/discover", strings.NewReader(`{"domain":"192.168.1.50"}`))
	rec := httptest.NewRecorder()
	hDenied.DiscoverProvider(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s, want 404", rec.Code, rec.Body.String())
	}
	requireFallbackPolicyViolation(t, denied, "https://192.168.1.50")

	// A fully deterministic admission test cannot open a real successful
	// connection here. Configured CIDRs exclude loopback: the parser rejects
	// 127.0.0.0/8 (federation_cidrs.go:99-108), and loopback is admitted only
	// through AllowLoopback, never via configured CIDR (client.go:204-213).
	// Private RFC1918 listener addresses are not portable, and the no-seam
	// rule forbids injecting a custom transport. Admission is therefore proven
	// by the Control-decision contrast: admitted -> real dial error, denied ->
	// ErrPolicyViolation with no real dial. Discover normalizes that dial
	// error to a non-policy InternalError, so the raw dial is checked on a
	// direct public-only GET with the same transport config.
	admitted := &fakeDiscoverer{fallback: publicOnlyClientWithCIDRs(t, 1*time.Second, "10.1.2.0/24")}
	hAdmitted := &wayfHandler{untrustedClient: admitted}
	req2 := httptest.NewRequest(http.MethodPost, "/discover", strings.NewReader(`{"domain":"10.1.2.3"}`))
	rec2 := httptest.NewRecorder()
	hAdmitted.DiscoverProvider(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s, want 404", rec2.Code, rec2.Body.String())
	}
	err, ok := admitted.fallbackErrs["https://10.1.2.3"]
	if !ok {
		t.Fatal("no recorded discovery error for in-range 10.1.2.3")
	}
	if errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("in-range 10.1.2.3 denied by policy = %v, want a non-policy discovery error", err)
	}

	parsed, parseErr := client.ParseFederationCIDRs([]string{"10.1.2.0/24"})
	if parseErr != nil {
		t.Fatalf("ParseFederationCIDRs: %v", parseErr)
	}
	rawClient := client.NewPublicOnlyHTTPClient(client.TransportConfig{
		Timeout:                1 * time.Second,
		Insecure:               true,
		AllowedFederationCIDRs: parsed,
	})
	resp, rawErr := rawClient.Get("https://10.1.2.3")
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if rawErr == nil {
		t.Fatal("in-range 10.1.2.3 direct GET succeeded, want a dial error")
	}
	if errors.Is(rawErr, client.ErrPolicyViolation) {
		t.Fatalf("in-range 10.1.2.3 direct GET denied by policy = %v, want a non-policy dial error", rawErr)
	}
	var netErr net.Error
	if !errors.As(rawErr, &netErr) {
		t.Fatalf("in-range 10.1.2.3 direct GET error = %v, want a dial/network error", rawErr)
	}
}
