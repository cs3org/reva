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
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/token"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Impersonate mints an ordinary user token for a target user, so an admin can
// act as that user. The token has the scopes of the login of the user (never
// admin+user) and the caller in its actor scope. Every impersonation is audited.
func (s *svc) Impersonate(ctx context.Context, req *adminpb.ImpersonateRequest) (*adminpb.ImpersonateResponse, error) {
	if req.User == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: user is required")
	}
	if req.TtlSeconds < 0 {
		return nil, status.Error(codes.InvalidArgument, "admin: ttl_seconds must not be negative")
	}
	fields := map[string]string{}
	reason := admin.Clean(req.Reason)
	switch h := admin.Header(ctx, admin.ReasonHeader); {
	case h == "":
		fields["reason"] = reason
	case reason != "" && reason != h:
		// Audit writes the header as "reason".
		fields["request_reason"] = reason
	}
	ev := admin.AuditEvent{Action: "impersonate", Actor: actorName(ctx), Target: req.User, Fields: fields}

	// A token with an actor must not start a second impersonation: the audit
	// would lose the first.
	callerScopes, _ := appctx.ContextGetScopes(ctx)
	if scope.Actor(callerScopes) != "" {
		ev.Err = errtypes.PermissionDenied("an impersonated session cannot impersonate")
		admin.Audit(ctx, ev)
		return nil, status.Errorf(codes.PermissionDenied, "admin: %s", ev.Err.Error())
	}
	actor := impersonator(ctx)
	if actor == "" {
		return nil, status.Error(codes.Unauthenticated, "admin: cannot identify caller")
	}
	if s.machineAuth == nil {
		return nil, status.Error(codes.FailedPrecondition, "admin: impersonation is not configured (set machine_auth_apikey)")
	}

	u, _, err := s.machineAuth.Authenticate(ctx, req.User, s.machineAPIKey)
	if err != nil {
		ev.Err = err
		admin.Audit(ctx, ev)
		if _, ok := err.(errtypes.IsNotFound); ok {
			return nil, status.Errorf(codes.NotFound, "admin: user %q not found", req.User)
		}
		return nil, status.Errorf(codes.Internal, "admin: impersonating %q: %v", req.User, err)
	}

	scopes, err := loginScopes(u)
	if err != nil {
		ev.Err = err
		admin.Audit(ctx, ev)
		return nil, status.Errorf(codes.InvalidArgument, "admin: %v", err)
	}

	kind, err := s.privilege(ctx, u)
	if err != nil {
		ev.Err = err
		admin.Audit(ctx, ev)
		return nil, status.Errorf(codes.Unavailable, "admin: %v", err)
	}
	if kind != "" {
		pev := admin.AuditEvent{Action: "impersonate_privileged", Actor: ev.Actor, Target: req.User, Kind: kind,
			Granted: req.AllowPrivileged, Fields: ev.Fields}
		if !req.AllowPrivileged {
			pev.Err = errtypes.PermissionDenied(fmt.Sprintf("%q is a %s; set allow_privileged to impersonate it", req.User, kind))
			admin.Audit(ctx, pev)
			return nil, status.Errorf(codes.PermissionDenied, "admin: %s", pev.Err.Error())
		}
		admin.Audit(ctx, pev)
	}

	tkn, expires, err := s.mintImpersonation(ctx, u, scope.AddActorScope(actor, scopes), time.Duration(req.TtlSeconds)*time.Second)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "admin: minting user token: %v", err)
	}

	res := &adminpb.ImpersonateResponse{Token: tkn}
	if !expires.IsZero() {
		res.ExpiresAt = expires.Unix()
		ev.Fields["expires_at"] = expires.UTC().Format(time.RFC3339)
	}
	ev.Granted = true
	admin.Audit(ctx, ev)
	return res, nil
}

// impersonator names the caller for the actor scope: the user id, or
// "socket:<uid>" on the local-root socket, whose user is synthetic.
func impersonator(ctx context.Context) string {
	if cred, ok := peerCredFrom(ctx); ok {
		return "socket:" + strconv.FormatUint(uint64(cred.uid), 10)
	}
	if u, ok := appctx.ContextGetUser(ctx); ok {
		return u.GetId().GetOpaqueId()
	}
	return ""
}

// loginScopes returns the scopes that the login of u gives. Machine auth gives
// the owner scope to every user, which is too much for a lightweight account.
func loginScopes(u *userpb.User) (map[string]*authpb.Scope, error) {
	switch t := u.GetId().GetType(); t {
	case userpb.UserType_USER_TYPE_PRIMARY, userpb.UserType_USER_TYPE_SECONDARY, userpb.UserType_USER_TYPE_SERVICE:
		return scope.AddOwnerScope(nil)
	case userpb.UserType_USER_TYPE_LIGHTWEIGHT:
		return scope.AddLightweightAccountScope(authpb.Role_ROLE_OWNER, nil)
	default:
		return nil, fmt.Errorf("cannot impersonate a user of type %s", t)
	}
}

// privilege returns why u is a privileged target, or "" if it is not. A failed
// group lookup is an error, so that a privileged target is never let through.
func (s *svc) privilege(ctx context.Context, u *userpb.User) (string, error) {
	if u.GetId().GetType() == userpb.UserType_USER_TYPE_SERVICE {
		return "service account", nil
	}
	groups := u.Groups
	if len(groups) == 0 {
		gw, err := service.Gateway(ctx)
		if err != nil {
			return "", fmt.Errorf("checking the groups of %q: %w", u.Username, err)
		}
		res, err := gw.GetUserGroups(ctx, &userpb.GetUserGroupsRequest{UserId: u.Id})
		if err == nil && res.GetStatus().GetCode() != rpcv1beta1.Code_CODE_OK {
			err = errors.New(res.GetStatus().GetMessage())
		}
		if err != nil {
			return "", fmt.Errorf("checking the groups of %q: %w", u.Username, err)
		}
		groups = res.Groups
	}
	if slices.Contains(groups, s.adminGroup) {
		return "member of " + s.adminGroup, nil
	}
	return "", nil
}

// mintImpersonation mints a token that lives for impersonation_ttl, or for ttl
// if that is shorter. The first token gives the cap, because the "expires" of
// the token manager config wins over impersonation_ttl.
func (s *svc) mintImpersonation(ctx context.Context, u *userpb.User, scopes map[string]*authpb.Scope, ttl time.Duration) (string, time.Time, error) {
	expiry := func(tm token.Manager, tkn string) time.Time {
		if v, ok := tm.(token.ValidatedExpiry); ok {
			if exp, err := v.ValidatedExpiresAt(ctx, tkn); err == nil {
				return exp
			}
		}
		return time.Time{}
	}

	tkn, err := s.userTokenManager.MintToken(ctx, u, scopes)
	if err != nil {
		return "", time.Time{}, err
	}
	exp := expiry(s.userTokenManager, tkn)
	if ttl <= 0 || exp.IsZero() || !time.Now().Add(ttl).Before(exp) {
		return tkn, exp, nil
	}

	tmConf := map[string]any{}
	maps.Copy(tmConf, s.conf.TokenManagers[s.conf.TokenManager])
	tmConf["expires"] = int64(ttl.Seconds())
	// This manager only mints, so it needs no cache.
	tmConf["cache_ttl"] = -1
	c := *s.conf
	c.TokenManagers = map[string]map[string]any{c.TokenManager: tmConf}
	tm, err := newTokenManager(c, 0)
	if err != nil {
		return "", time.Time{}, err
	}
	if tkn, err = tm.MintToken(ctx, u, scopes); err != nil {
		return "", time.Time{}, err
	}
	return tkn, expiry(tm, tkn), nil
}
