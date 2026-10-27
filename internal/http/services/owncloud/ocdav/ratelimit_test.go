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

package ocdav

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cs3org/reva/v3/pkg/rhttp/ratelimit"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
)

func TestOnlyUnprotectedRoutesAreRateLimited(t *testing.T) {
	l, err := ratelimit.New(ratelimit.Config{RequestsPerMinute: 1, Burst: 1})
	if err != nil {
		t.Fatal(err)
	}
	s := &svc{c: &Config{}, limiter: l}
	t.Cleanup(func() { _ = s.Close() })

	r := router.New()
	s.Routes(r.Service("ocdav"))

	do := func(method, p string) int {
		req := httptest.NewRequest(method, p, nil)
		req.RemoteAddr = "203.0.113.7:1000"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	// status.php answers from its own configuration, so this spends the
	// client's only token without reaching any storage.
	if got := do(http.MethodGet, "/status.php"); got != http.StatusOK {
		t.Fatalf("first unauthenticated request: got %d, want 200", got)
	}
	// The bucket is shared by the unauthenticated entry points, so the next
	// one is refused before its handler runs.
	if got := do(http.MethodGet, "/ocm-provider"); got != http.StatusTooManyRequests {
		t.Fatalf("second unauthenticated request: got %d, want 429", got)
	}

	// The authenticated subtrees, and OCM's which authenticates by share
	// secret or token, are not limited: an unserved method reaches their
	// dispatch and is refused as one, never as a rate limit.
	for _, p := range []string{"/remote.php/dav/files/einstein/a.txt", "/remote.php/dav/ocm/secret/a.txt"} {
		for i := range 3 {
			if got := do("FROBNICATE", p); got != http.StatusMethodNotAllowed {
				t.Fatalf("%s, request %d: got %d, want 405", p, i, got)
			}
		}
	}
}
