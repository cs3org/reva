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

package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
)

// The issuer must equal the server URL or NewProvider rejects the response.
func discoveryServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                srv.URL,
			"authorization_endpoint":                srv.URL + "/auth",
			"token_endpoint":                        srv.URL + "/token",
			"jwks_uri":                              srv.URL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func resolveConcurrently(t *testing.T, am *mgr, n int, issuerFor func(i int) string) {
	t.Helper()
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, n)

	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := am.getOIDCProviderForIssuer(context.Background(), issuerFor(i)); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("getOIDCProviderForIssuer: %v", err)
	}
}

// Unsynchronized this is "fatal error: concurrent map writes". Run with -race.
func TestGetOIDCProviderForIssuerConcurrent(t *testing.T) {
	tests := []struct {
		name    string
		issuers int
		primed  bool
	}{
		{
			name:    "one uncached issuer",
			issuers: 1,
		},
		{
			// a read racing a write is equally fatal, so the read path needs
			// the lock too
			name:    "uncached issuer racing a cached one",
			issuers: 2,
			primed:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvs := make([]*httptest.Server, tt.issuers)
			hits := make([]*atomic.Int64, tt.issuers)
			for i := range srvs {
				srvs[i], hits[i] = discoveryServer(t)
			}

			am := &mgr{providers: make(map[string]*oidc.Provider)}
			if tt.primed {
				if _, err := am.getOIDCProviderForIssuer(context.Background(), srvs[0].URL); err != nil {
					t.Fatalf("priming the cache: %v", err)
				}
			}

			resolveConcurrently(t, am, 32, func(i int) string {
				return srvs[i%tt.issuers].URL
			})

			for i, h := range hits {
				if got := h.Load(); got != 1 {
					t.Errorf("issuer %d: discovery requests = %d, want 1", i, got)
				}
			}
		})
	}
}

func TestGetOIDCProviderForIssuerCaches(t *testing.T) {
	srv, hits := discoveryServer(t)
	am := &mgr{providers: make(map[string]*oidc.Provider)}

	first, err := am.getOIDCProviderForIssuer(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	second, err := am.getOIDCProviderForIssuer(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("expected the cached provider to be reused")
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("discovery requests = %d, want 1", got)
	}
}
