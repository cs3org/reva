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

package scope

import (
	"context"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
)

// TestLightweightScopeHTTPPaths checks the request paths the HTTP auth
// interceptor passes to VerifyScope for a lightweight account token.
func TestLightweightScopeHTTPPaths(t *testing.T) {
	scopes, err := AddLightweightAccountScope(authpb.Role_ROLE_OWNER, nil)
	if err != nil {
		t.Fatalf("AddLightweightAccountScope: %v", err)
	}

	cases := []struct {
		path string
		want bool
	}{
		{"/sciencemesh/webapp-launch", true},
		{"/sciencemesh/open-in-app", true},
		{"/sciencemesh/list-providers", true},
		{"/sciencemesh/unknown", false},
		{"/ocs/v2.php/apps/files_sharing/api/v1/sharees", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, err := VerifyScope(context.Background(), scopes, tc.path)
			if err != nil {
				t.Fatalf("VerifyScope: %v", err)
			}
			if got != tc.want {
				t.Fatalf("VerifyScope(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
