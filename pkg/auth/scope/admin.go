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

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/rs/zerolog"
)

// adminScope authorizes only the Admin API's own request types. This gives
// mutual isolation with every other scope, with no new code path: an admin
// token can never satisfy a storage/share request (those verifiers do not
// recognize admin messages), and a user/share token can never satisfy an admin
// request (the always-true user scope is guarded by isAdminResource, and the
// other verifiers type-switch on their own messages and fall through).
func adminScope(_ context.Context, _ *authpb.Scope, resource any, _ *zerolog.Logger) (bool, error) {
	return isAdminResource(resource), nil
}

// isAdminResource reports whether resource is an Admin API (adminpb) request, or
// a control channel (controlpb) request, that requires the admin scope.
// RequestAdminRequest is deliberately excluded: it is the step-up door,
// reachable with an ordinary user-scoped token, so the user scope (not the admin
// scope) must satisfy it.
func isAdminResource(resource any) bool {
	switch resource.(type) {
	case *adminpb.ImpersonateRequest,
		*adminpb.GetServerInfoRequest,
		*adminpb.GetHealthRequest,
		*adminpb.ListServicesRequest,
		*adminpb.GetServiceConfigRequest,
		*adminpb.ListInvocationsRequest,
		*adminpb.InvokeRequest,
		*controlpb.ListInvocationsRequest,
		*controlpb.InvokeRequest:
		return true
	}
	return false
}

// AddAdminScope adds the admin scope: a short-lived, admin-only privilege that
// satisfies Admin API requests and nothing else. It is minted only after the
// group check in RequestAdmin, exactly like AddOwnerScope mints the owner
// token. The scope map carries key "admin" and no "user" key, so the token
// cannot act on any user's data — that stays behind explicit impersonation.
func AddAdminScope(scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	if scopes == nil {
		scopes = make(map[string]*authpb.Scope)
	}
	scopes["admin"] = &authpb.Scope{
		Resource: &types.OpaqueEntry{
			Decoder: "json",
			Value:   []byte(`{"admin":true}`),
		},
		Role: authpb.Role_ROLE_OWNER,
	}
	return scopes, nil
}
