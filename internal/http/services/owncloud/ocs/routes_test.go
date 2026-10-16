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

package ocs

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cs3org/reva/v3/pkg/rhttp/router"
)

// Declaring the routes must not panic. ServeMux rejects two patterns that
// overlap without one being more specific, and it does so at registration, so
// a bad pair takes the whole server down at startup rather than failing a
// request.
func TestRoutesRegister(t *testing.T) {
	r := router.New()
	(&svc{}).Routes(r.Service("ocs"))

	if len(r.Routes()) == 0 {
		t.Fatal("no routes declared")
	}
}

// The share actions resolve the way they did before, in particular the
// pending subtree, which overlaps /shares/{shareid}/notify.
func TestShareRoutesResolve(t *testing.T) {
	r := router.New()
	(&svc{}).Routes(r.Service("ocs"))

	const base = "/ocs/v1.php/apps/files_sharing/api/v1/shares"
	tests := map[string]struct {
		method string
		target string
	}{
		"list":            {http.MethodGet, base},
		"create":          {http.MethodPost, base},
		"get":             {http.MethodGet, base + "/42"},
		"update":          {http.MethodPut, base + "/42"},
		"remove":          {http.MethodDelete, base + "/42"},
		"notify":          {http.MethodPost, base + "/42/notify"},
		"accept pending":  {http.MethodPost, base + "/pending/42"},
		"reject pending":  {http.MethodDelete, base + "/pending/42"},
		"federated list":  {http.MethodGet, base + "/remote_shares"},
		"federated get":   {http.MethodGet, base + "/remote_shares/42"},
		"ambiguous share": {http.MethodPost, base + "/pending/notify"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if _, ok := r.Match(httptest.NewRequest(tt.method, tt.target, nil)); !ok {
				t.Errorf("%s %s resolves to no route", tt.method, tt.target)
			}
		})
	}
}

// The mounted pending subtree routes by method inside itself, where Match only
// sees the mount.
func TestPendingSharesRoutes(t *testing.T) {
	m, ok := (&svc{}).pendingShares().(*http.ServeMux)
	if !ok {
		t.Fatal("pendingShares is not a ServeMux")
	}

	const base = "/ocs/v2.php/apps/files_sharing/api/v1/shares/pending/42"
	tests := map[string]struct {
		method string
		found  bool
	}{
		"accept": {http.MethodPost, true},
		"reject": {http.MethodDelete, true},
		"get":    {http.MethodGet, false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, pattern := m.Handler(httptest.NewRequest(tt.method, base, nil))
			if (pattern != "") != tt.found {
				t.Errorf("got pattern %q, expected found=%v", pattern, tt.found)
			}
		})
	}
}
