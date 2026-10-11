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

package ocdav

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cs3org/reva/v3/pkg/rhttp/ratelimit"
)

func TestOnlyUnprotectedRoutesAreRateLimited(t *testing.T) {
	l, err := ratelimit.New(ratelimit.Config{RequestsPerMinute: 1, Burst: 1})
	if err != nil {
		t.Fatal(err)
	}
	s := &svc{c: &Config{}, limiter: l}
	t.Cleanup(func() { _ = s.Close() })

	h := s.limitUnprotected(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	get := func(p string) int {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.RemoteAddr = "203.0.113.7:1000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := get("/remote.php/dav/public-files/tok/a.txt"); got != http.StatusOK {
		t.Fatalf("first public request: got %d, want 200", got)
	}
	if got := get("/s/tok/download"); got != http.StatusTooManyRequests {
		t.Fatalf("second public request: got %d, want 429", got)
	}
	// Authenticated routes and OCM WebDAV, which authenticates by share
	// secret or token, are not limited.
	for _, p := range []string{"/remote.php/dav/files/einstein/a.txt", "/remote.php/dav/ocm/secret/a.txt"} {
		for i := 0; i < 3; i++ {
			if got := get(p); got != http.StatusOK {
				t.Fatalf("%s, request %d: got %d, want 200", p, i, got)
			}
		}
	}
}
