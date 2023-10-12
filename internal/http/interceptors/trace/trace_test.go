// Copyright 2018-2023 CERN
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

package trace

import (
	"context"
	"net/http"
	"testing"

	"github.com/cs3org/reva/pkg/trace"
)

type testPair struct {
	e string
	r *http.Request
}

func TestGetTrace(t *testing.T) {
	pairs := []*testPair{
		{
			r: newRequest(context.Background(), map[string]string{"X-Trace-ID": "def"}),
			e: "def",
		},
		{
			r: newRequest(context.Background(), map[string]string{"X-Request-ID": "abc"}),
			e: "abc",
		},
		{
			r: newRequest(trace.Set(context.Background(), "fgh"), nil),
			e: "fgh",
		},
	}

	for _, p := range pairs {
		got, _ := getTraceID(p.r)
		t.Logf("headers: %+v context: %+v got: %+v\n", p.r, p.r.Context(), got)
		if got != p.e {
			t.Fatal("expected: "+p.e, "got: "+got)
			return
		}
	}
}

func newRequest(ctx context.Context, headers map[string]string) *http.Request {
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestGenerateTrace(t *testing.T) {
	got, _ := getTraceID(newRequest(context.Background(), nil))
	if len(got) != 36 {
		t.Fatal("expected random generated UUID 36 chars trace ID but got:" + got)
		return
	}
}
