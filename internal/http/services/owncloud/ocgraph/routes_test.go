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

package ocgraph

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cs3org/reva/v3/pkg/rhttp/router"
)

// Declaring the routes must not panic: ServeMux rejects overlapping patterns
// at registration, which would take the server down at startup.
func TestRoutesRegister(t *testing.T) {
	r := router.New()
	(&svc{}).Routes(r.Service("ocgraph"))

	const drive = "/graph/v1beta1/drives/space!id/items/res!id"
	tests := map[string]struct {
		method string
		target string
	}{
		"me":                {http.MethodGet, "/graph/v1.0/me"},
		"space":             {http.MethodGet, "/graph/v1.0/drives/space!id"},
		"my drives":         {http.MethodGet, "/graph/v1beta1/me/drives"},
		"shared with me":    {http.MethodGet, "/graph/v1beta1/me/drive/sharedWithMe"},
		"role definitions":  {http.MethodGet, "/graph/v1beta1/roleManagement/permissions/roleDefinitions"},
		"root permissions":  {http.MethodGet, "/graph/v1beta1/drives/space!id/root/permissions"},
		"invite":            {http.MethodPost, drive + "/invite"},
		"permissions":       {http.MethodGet, drive + "/permissions"},
		"update permission": {http.MethodPatch, drive + "/permissions/share!id"},
		"set password":      {http.MethodPost, drive + "/permissions/share!id/setPassword"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if _, ok := r.Match(httptest.NewRequest(tt.method, tt.target, nil)); !ok {
				t.Errorf("%s %s resolves to no route", tt.method, tt.target)
			}
		})
	}
}
