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

package admin

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Impersonate mints an ordinary user-scoped token for a target user, so an
// admin can act as (or inspect the view of) that user. It reuses the machine
// auth manager to resolve the user and produce a user scope — never admin+user
// — so downstream services apply their normal user-scope checks. Every
// impersonation is audited (who, whom, why).
func (s *svc) Impersonate(ctx context.Context, req *adminpb.ImpersonateRequest) (*adminpb.ImpersonateResponse, error) {
	if req.User == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: user is required")
	}
	if s.machineAuth == nil {
		return nil, status.Error(codes.FailedPrecondition, "admin: impersonation is not configured (set machine_auth_apikey)")
	}

	u, scopes, err := s.machineAuth.Authenticate(ctx, req.User, s.machineAPIKey)
	if err != nil {
		admin.Audit(ctx, admin.AuditEvent{Action: "impersonate", Actor: actorName(ctx), Target: req.User, Err: err})
		return nil, status.Errorf(codes.Internal, "admin: impersonating %q: %v", req.User, err)
	}

	tkn, err := s.tokenManager.MintToken(ctx, u, scopes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "admin: minting user token: %v", err)
	}

	admin.Audit(ctx, admin.AuditEvent{Action: "impersonate", Actor: actorName(ctx), Target: req.User, Granted: true,
		Fields: map[string]string{"reason": req.Reason}})
	return &adminpb.ImpersonateResponse{Token: tkn}, nil
}
