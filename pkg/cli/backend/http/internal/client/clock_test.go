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

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// clockServer answers every request with the given status and Date.
func clockServer(t *testing.T, status int, date string) *Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %s, want HEAD: reading a clock must not ask for a body", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want none: the clock has to be readable "+
				"before a credential is", got)
		}
		if date == "" {
			// Suppress the one net/http supplies for a response without one.
			w.Header()["Date"] = nil
		} else {
			w.Header().Set("Date", date)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(ts.Close)

	// No credential source: a request that tries to authenticate fails.
	c, err := New(ts.URL, WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestServerTime(t *testing.T) {
	want := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	for _, status := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusNotFound} {
		got, err := clockServer(t, status, want.Format(http.TimeFormat)).ServerTime(context.Background())
		if err != nil {
			t.Fatalf("status %d: %v", status, err)
		}
		if !got.Equal(want) {
			t.Errorf("status %d: got %s, want %s", status, got, want)
		}
	}
}

func TestServerTimeWithoutADateHeader(t *testing.T) {
	_, err := clockServer(t, http.StatusOK, "").ServerTime(context.Background())
	if err == nil {
		t.Fatal("a response with no Date must be an error, not a zero time")
	}
	if !strings.Contains(err.Error(), "Date") {
		t.Errorf("error does not say what was missing: %v", err)
	}
}

func TestServerTimeWithAnUnparsableDate(t *testing.T) {
	_, err := clockServer(t, http.StatusOK, "the day before yesterday").ServerTime(context.Background())
	if err == nil {
		t.Fatal("an unparsable Date must be an error")
	}
}
