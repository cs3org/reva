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

package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestHandlerLabel(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "", want: "/"},
		{path: "/", want: "/"},
		{path: "/remote.php/dav/files/einstein/foo", want: "/remote.php"},
		{path: "/remote.php/dav/files/marie/b/c?download=1", want: "/remote.php"},
		{path: "/ocs/v2.php/apps/files_sharing/api/v1/shares", want: "/ocs"},
		{path: "status.php", want: "/status.php"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := handlerLabel(tt.path); got != tt.want {
				t.Fatalf("handlerLabel(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestNewUsesBoundedHandlerLabel(t *testing.T) {
	duration.Reset()
	t.Cleanup(duration.Reset)

	middleware := New()
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	paths := []string{
		"/remote.php/dav/files/einstein/a",
		"/remote.php/dav/files/marie/b/c",
		"/remote.php/dav/files/richard/deep/path/file.txt",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, "https://example.org"+path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	if got := testutil.CollectAndCount(duration); got != 1 {
		t.Fatalf("duration collected %d metrics, want 1", got)
	}

	handlers := collectDurationHandlers(t)
	if got, want := len(handlers), 1; got != want {
		t.Fatalf("duration handler labels = %v, want one label", handlers)
	}
	if _, ok := handlers["/remote.php"]; !ok {
		t.Fatalf("duration handler labels = %v, want /remote.php", handlers)
	}
}

func collectDurationHandlers(t *testing.T) map[string]struct{} {
	t.Helper()

	metrics := make(chan prometheus.Metric)
	go func() {
		duration.Collect(metrics)
		close(metrics)
	}()

	handlers := map[string]struct{}{}
	for metric := range metrics {
		var dtoMetric dto.Metric
		if err := metric.Write(&dtoMetric); err != nil {
			t.Fatalf("failed to write metric: %v", err)
		}

		for _, label := range dtoMetric.GetLabel() {
			if label.GetName() == "handler" {
				handlers[label.GetValue()] = struct{}{}
			}
		}
	}

	return handlers
}
