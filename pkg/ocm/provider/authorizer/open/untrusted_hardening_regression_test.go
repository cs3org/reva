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

package open

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
)

// TestUntrustedHardeningRegression proves open.New wires the shared
// public-only client. Per-host matrices stay in open_test.go.
func TestUntrustedHardeningRegression(t *testing.T) {
	got, err := New(context.Background(), map[string]any{"insecure": true})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := got.(*authorizer)
	if !ok {
		t.Fatalf("got %T, want *authorizer", got)
	}
	if a.publicOCMClient == nil {
		t.Fatal("open.New must construct a public-only OCM client")
	}

	t.Run("HTTP is refused", func(t *testing.T) {
		fetched := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fetched = true
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		resp, err := roundTripConstructor(t, a.publicOCMClient, srv.URL)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if fetched {
			t.Fatal("open.New client must not fetch HTTP")
		}
		if !errors.Is(err, ocmd.ErrNonHTTPS) {
			t.Fatalf("got %v, want ocmd.ErrNonHTTPS", err)
		}
	})

	t.Run("metadata host is refused", func(t *testing.T) {
		ctx, tr := contextWithHostDialTrace(context.Background())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://169.254.169.254/", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := a.publicOCMClient.HTTPTransport().RoundTrip(req)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		assertNoEstablishedHost(t, tr, "169.254.169.254")
		if !errors.Is(err, ocmd.ErrNonPublicAddr) {
			t.Fatalf("got %v, want ocmd.ErrNonPublicAddr", err)
		}
	})

	t.Run("public HTTPS discovery succeeds", func(t *testing.T) {
		var srv *httptest.Server
		srv = startPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/.well-known/ocm" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":    true,
				"apiVersion": "1.2.0",
				"endPoint":   srv.URL + "/ocm",
				"provider":   "public-host",
				"resourceTypes": []any{
					map[string]any{
						"name":      "file",
						"protocols": map[string]any{"webdav": "/remote.php/dav/ocm"},
					},
				},
			})
		}))

		info, err := a.GetInfoByDomain(context.Background(), srv.URL)
		if err != nil {
			t.Fatalf("GetInfoByDomain on a public HTTPS host: %v", err)
		}
		if info == nil || info.FullName != "public-host" {
			t.Fatalf("unexpected provider info: %+v", info)
		}
	})
}

func roundTripConstructor(t *testing.T, c *ocmd.OCMClient, rawURL string) (*http.Response, error) {
	t.Helper()
	if c == nil || c.HTTPTransport() == nil {
		t.Fatal("constructor-wired client has no transport")
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c.HTTPTransport().RoundTrip(req)
}
