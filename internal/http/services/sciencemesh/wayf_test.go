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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
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
