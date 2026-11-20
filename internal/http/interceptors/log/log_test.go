// Copyright 2018-2024 CERN
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

package log

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
)

// processed runs one request through the logging middleware, with inner as
// the rest of the chain, and returns the "processed http request" line.
func processed(t *testing.T, inner http.HandlerFunc) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	l := zerolog.New(&buf)
	req := httptest.NewRequest(http.MethodGet, "/remote.php/dav/spaces/x", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Firefox/144.0")
	req = req.WithContext(appctx.WithLogger(req.Context(), &l))
	New()(inner).ServeHTTP(httptest.NewRecorder(), req)

	for line := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatal(err)
		}
		if m["message"] == "processed http request" {
			return m
		}
	}
	t.Fatalf("no processed http request line in %q", buf.String())
	return nil
}

// The middleware runs outside the auth one, so it only learns the user
// through SetUser; with it, a request line says who made it and with what.
func TestProcessedRequestCarriesUserAndAgent(t *testing.T) {
	m := processed(t, func(w http.ResponseWriter, r *http.Request) {
		SetUser(r.Context(), "alice")
	})
	if m["user"] != "alice" {
		t.Errorf("user = %v, want alice", m["user"])
	}
	if m["user_agent"] != "Mozilla/5.0 (X11; Linux x86_64) Firefox/144.0" {
		t.Errorf("user_agent = %v", m["user_agent"])
	}
}

func TestProcessedRequestWithoutUser(t *testing.T) {
	m := processed(t, func(w http.ResponseWriter, r *http.Request) {})
	if _, ok := m["user"]; ok {
		t.Errorf("anonymous request logged with user %v", m["user"])
	}
	if m["user_agent"] == nil {
		t.Error("user_agent missing")
	}
}

// SetUser outside the middleware (no recorder in the context) is a no-op.
func TestSetUserWithoutMiddleware(t *testing.T) {
	SetUser(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "alice")
}
