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
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
)

// TestAdminScopeIsolation asserts the mutual isolation the security model
// depends on: an admin token satisfies admin requests and nothing else, and a
// user token satisfies user requests and never an admin request.
func TestAdminScopeIsolation(t *testing.T) {
	ctx := context.Background()

	adminReq := &adminpb.ListServicesRequest{}
	userReq := &provider.StatRequest{}

	adminToken, err := AddAdminScope(nil)
	if err != nil {
		t.Fatalf("AddAdminScope: %v", err)
	}
	userToken, err := AddOwnerScope(nil)
	if err != nil {
		t.Fatalf("AddOwnerScope: %v", err)
	}

	cases := []struct {
		name  string
		scope map[string]*authpb.Scope
		req   any
		want  bool
	}{
		{"admin token, admin request", adminToken, adminReq, true},
		{"admin token, user request", adminToken, userReq, false},
		{"user token, user request", userToken, userReq, true},
		{"user token, admin request", userToken, adminReq, false},
		// RequestAdmin is the step-up door: a user token must satisfy it, an
		// admin token must not.
		{"user token, RequestAdmin", userToken, &adminpb.RequestAdminRequest{}, true},
		{"admin token, RequestAdmin", adminToken, &adminpb.RequestAdminRequest{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := VerifyScope(ctx, tc.scope, tc.req)
			if err != nil {
				t.Fatalf("VerifyScope: %v", err)
			}
			if got != tc.want {
				t.Fatalf("VerifyScope = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIsAdminResource covers the type guard shared by adminScope and userScope.
func TestIsAdminResource(t *testing.T) {
	if !isAdminResource(&adminpb.InvokeRequest{}) {
		t.Fatal("expected adminpb request to be recognized as an admin resource")
	}
	if !isAdminResource(&controlpb.InvokeRequest{}) {
		t.Fatal("expected control channel request to be recognized as an admin resource")
	}
	if isAdminResource(&provider.StatRequest{}) {
		t.Fatal("expected a storage request to not be an admin resource")
	}
	if isAdminResource(&adminpb.RequestAdminRequest{}) {
		t.Fatal("RequestAdmin must not require the admin scope (it is the step-up door)")
	}
}
