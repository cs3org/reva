// Copyright 2018-2025 CERN
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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIngressRoutesAreRateLimited(t *testing.T) {
	s, err := New(context.Background(), map[string]any{
		"gatewaysvc":              "localhost:19000",
		"token_managers":          map[string]any{"jwt": map[string]any{"secret": "test-secret"}},
		"unauth_rate_limit":       1,
		"unauth_rate_limit_burst": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// The limiter runs before the handlers, so a rejected request never
	// reaches them; every ingress route shares the client's bucket.
	post := func(path string) int {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.RemoteAddr = "203.0.113.7:1000"
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if got := post("/no-such-route"); got != http.StatusNotFound {
		t.Fatalf("first request: got %d, want 404", got)
	}
	for _, p := range []string{sharesPath, inviteAcceptedPath, notificationsPath, tokenPath} {
		if got := post(p); got != http.StatusTooManyRequests {
			t.Fatalf("%s: got %d, want 429", p, got)
		}
	}
}

func TestInvalidTrustedProxyFailsStartup(t *testing.T) {
	if _, err := New(context.Background(), map[string]any{
		"gatewaysvc":          "localhost:19000",
		"trusted_proxy_cidrs": []string{"10.0.0.0/33"},
	}); err == nil {
		t.Fatal("invalid trusted_proxy_cidrs accepted")
	}
}
