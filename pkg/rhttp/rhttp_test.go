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

package rhttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/rs/zerolog"
)

func TestURLHasPrefix(t *testing.T) {
	tests := map[string]struct {
		url      string
		prefix   string
		expected bool
	}{
		"root": {
			url:      "/",
			prefix:   "/",
			expected: true,
		},
		"suburl_root": {
			url:      "/api/v0",
			prefix:   "/",
			expected: true,
		},
		"suburl_root_slash_end": {
			url:      "/api/v0/",
			prefix:   "/",
			expected: true,
		},
		"suburl_root_no_slash": {
			url:      "/api/v0",
			prefix:   "",
			expected: true,
		},
		"no_common_prefix": {
			url:      "/api/v0/project",
			prefix:   "/api/v0/p",
			expected: false,
		},
		"long_url_prefix": {
			url:      "/api/v0/project/test",
			prefix:   "/api/v0",
			expected: true,
		},
		"prefix_end_slash": {
			url:      "/api/v0/project/test",
			prefix:   "/api/v0/",
			expected: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			res := urlHasPrefix(test.url, test.prefix)
			if res != test.expected {
				t.Fatalf("%s got an unexpected result: %+v instead of %+v", t.Name(), res, test.expected)
			}
		})
	}
}

type testService struct{}

func (testService) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
}
func (testService) Prefix() string        { return "data" }
func (testService) Close() error          { return nil }
func (testService) Unprotected() []string { return nil }

func TestRejectDotSegments(t *testing.T) {
	tests := map[string]struct {
		target   string
		rejected bool
	}{
		"dotdot":          {target: "/data/tus/../simple/x", rejected: true},
		"dot":             {target: "/data/tus/./x", rejected: true},
		"trailing_dotdot": {target: "/data/tus/..", rejected: true},
		"leading_dotdot":  {target: "/../data/x", rejected: true},
		"encoded_dotdot":  {target: "/data/tus/%2e%2e/simple/x", rejected: true},
		"encoded_dot":     {target: "/data/tus/%2E/x", rejected: true},
		"half_encoded":    {target: "/data/tus/.%2e/simple/x", rejected: true},
		"encoded_slashes": {target: "/data/tus%2F..%2Fsimple/x", rejected: true},
		"dotdot_prefix":   {target: "/data/..foo"},
		"inner_dots":      {target: "/data/a..b"},
		"hidden":          {target: "/data/.hidden"},
		"three_dots":      {target: "/data/..."},
		"trailing_slash":  {target: "/data/tus/x/"},
		"root":            {target: "/"},
	}

	for name, test := range tests {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodOptions} {
			t.Run(name+"_"+method, func(t *testing.T) {
				var reached bool
				var buf bytes.Buffer
				s, _ := New(
					WithLogger(zerolog.New(&buf)),
					WithServices(map[string]global.Service{"dataprovider": testService{}}),
					WithMiddlewares([]global.Middleware{func(h http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							reached = true
							h.ServeHTTP(w, r)
						})
					}}),
				)
				h, _ := s.getHandler()
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest(method, test.target, nil))

				logged := strings.Contains(buf.String(), "dot segments")
				if rejected := w.Code == http.StatusBadRequest && !reached; rejected != test.rejected || logged != test.rejected {
					t.Fatalf("%s got status %d, middlewares reached %t, rejection logged %t", t.Name(), w.Code, reached, logged)
				}
			})
		}
	}
}
