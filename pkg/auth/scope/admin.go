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
	"strings"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/rs/zerolog"
)

// MethodResource identifies a gRPC call by its full method name. The auth
// interceptor passes it for a server-streaming call, whose request message is
// not yet available to type-switch on.
type MethodResource string

// adminScope authorizes only the Admin API's own request types, giving mutual
// isolation with every other scope: an admin token satisfies nothing else, and
// the always-true user scope declines admin requests via isAdminResource.
func adminScope(_ context.Context, _ *authpb.Scope, resource any, _ *zerolog.Logger) (bool, error) {
	return isAdminResource(resource), nil
}

// isAdminResource reports whether resource is an Admin API or control channel
// request requiring the admin scope. RequestAdminRequest is excluded: it is the
// step-up door, reachable with a user token.
func isAdminResource(resource any) bool {
	switch r := resource.(type) {
	case *adminpb.ImpersonateRequest,
		*adminpb.GetServerInfoRequest,
		*adminpb.GetHealthRequest,
		*adminpb.ListServicesRequest,
		*adminpb.GetServiceConfigRequest,
		*adminpb.ListInvocationsRequest,
		*adminpb.InvokeRequest,
		*adminpb.InspectJobsRequest,
		*adminpb.ListJobRunsRequest,
		*adminpb.GetJobRunRequest,
		*adminpb.EnqueueJobRequest,
		*adminpb.TriggerJobRequest,
		*adminpb.CancelJobRunRequest,
		*adminpb.CancelPeriodicJobRequest,
		*controlpb.ListInvocationsRequest,
		*controlpb.InvokeRequest:
		return true
	case MethodResource:
		// Streaming methods, identified by name: no request message is
		// available on the stream yet.
		return IsAdminMethod(string(r))
	}
	return false
}

// IsAdminMethod reports whether a full gRPC method belongs to the Admin API or
// the control channel. Callers that must exclude these operational RPCs from
// per-request accounting (e.g. request-activity tracking) use it too.
func IsAdminMethod(method string) bool {
	return strings.HasPrefix(method, "/reva.admin.v1beta1.AdminAPI/") ||
		strings.HasPrefix(method, "/reva.control.v1beta1.Control/")
}

// HasAdminScope reports whether a token's scopes include the admin scope. Such a
// token authorizes admin/control resources by capability alone (isAdminResource)
// and never by the caller's group membership — so its bearer, which may be a
// synthetic local-root identity unknown to any user provider, must not be
// subjected to a user-group lookup during token validation.
func HasAdminScope(scopes map[string]*authpb.Scope) bool {
	_, ok := scopes["admin"]
	return ok
}

// AddAdminScope adds the admin scope: a short-lived privilege satisfying Admin
// API requests and nothing else. The scope map carries "admin" and no "user"
// key, so the token cannot act on any user's data.
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
