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

package applicationauth

import (
	"context"
	"fmt"
	"strings"
	"time"

	appauthpb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appauth"
	"github.com/cs3org/reva/v3/pkg/appctx"
	authcache "github.com/cs3org/reva/v3/pkg/auth/cache"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/invoke"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
)

// registerInvocations exposes the app passwords of any user to administrators,
// through the Admin API.
func (s *service) registerInvocations() {
	s.Set = invoke.NewSet()
	s.Set.Add("create", "Create an app password for a user; it is returned once. Address one instance by node id: each instance reached creates one.").
		Mutating().
		Arg("user", "the username").
		Arg("client", "the kind of client: sync, cli, webdav or mcp").
		Arg("permissions", "comma separated permissions, or *").
		OptArg("roots", "comma separated root folders, for mcp").
		OptArg("expires_in", "seconds until it expires, the store default when absent").
		OptArg("label", "the label shown to the user").
		Handle(s.invokeCreate)
	s.Set.Add("list", "List the app passwords of a user.").
		Arg("user", "the username").
		Handle(s.invokeList)
	s.Set.Add("revoke", "Revoke an app password of a user.").
		Mutating().
		Arg("user", "the username").
		Arg("password_id", "the password id, as listed").
		Handle(s.invokeRevoke)
}

func (s *service) invokeCreate(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	actor := appctx.ContextMustGetUser(ctx)
	owner, err := s.user(ctx, a.String("user"))
	if err != nil {
		return nil, err
	}
	scopes, err := scope.AddAppAuthScope(&scope.AppAuthScope{
		Client:      a.String("client"),
		Permissions: splitList(a.String("permissions")),
		Roots:       splitList(a.String("roots")),
	}, nil)
	if err != nil {
		return nil, err
	}
	var expiration *typespb.Timestamp
	if a.Has("expires_in") {
		secs := a.Int("expires_in")
		if secs <= 0 {
			return nil, fmt.Errorf("expires_in must be a positive number of seconds")
		}
		expiration = &typespb.Timestamp{Seconds: uint64(time.Now().Unix() + int64(secs))}
	}
	label := a.String("label")
	if label == "" {
		label = "created by an administrator"
	}

	uctx := appauth.ContextWithAdministrator(appctx.ContextSetUser(ctx, owner), actor.GetId())
	pw, err := s.am.GenerateAppPassword(uctx, scopes, label, expiration)
	ap := appAuthScope(pw)
	audit(ctx, "create_app_password", actor, owner, ap, err)
	if err != nil {
		return nil, err
	}
	res := appPasswordResult(pw, actor.GetId().GetOpaqueId())
	res["password"] = pw.GetPassword()
	return res, nil
}

func (s *service) invokeList(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	owner, err := s.user(ctx, a.String("user"))
	if err != nil {
		return nil, err
	}
	uctx := appctx.ContextSetUser(ctx, owner)
	pws, err := s.am.ListAppPasswords(uctx)
	if err != nil {
		return nil, err
	}
	creators := map[string]string{}
	if cl, ok := s.am.(appauth.CreatorLister); ok {
		if creators, err = cl.ListAppPasswordCreators(uctx); err != nil {
			return nil, err
		}
	}
	out := make([]invoke.Result, 0, len(pws))
	for _, pw := range pws {
		out = append(out, appPasswordResult(pw, creators[appAuthScope(pw).PasswordID]))
	}
	return invoke.Result{"app_passwords": out}, nil
}

func (s *service) invokeRevoke(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	actor := appctx.ContextMustGetUser(ctx)
	owner, err := s.user(ctx, a.String("user"))
	if err != nil {
		return nil, err
	}
	id := a.String("password_id")
	err = s.am.InvalidateAppPassword(appctx.ContextSetUser(ctx, owner), id)
	audit(ctx, "revoke_app_password", actor, owner, &scope.AppAuthScope{PasswordID: id}, err)
	if err != nil {
		return nil, err
	}
	// So that it stops working at once on this node.
	authcache.Forget(func(i *authcache.Identity) bool {
		ap, ok := scope.GetAppAuthScope(i.Scopes)
		return ok && ap.PasswordID == id
	})
	return invoke.Result{"revoked": id}, nil
}

func (s *service) user(ctx context.Context, username string) (*userpb.User, error) {
	gw, err := revaservice.Gateway(ctx)
	if err != nil {
		return nil, err
	}
	res, err := gw.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{Claim: "username", Value: username, SkipFetchingUserGroups: true})
	if err != nil {
		return nil, err
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("user %q: %s", username, res.GetStatus().GetMessage())
	}
	return res.GetUser(), nil
}

func audit(ctx context.Context, action string, actor, owner *userpb.User, ap *scope.AppAuthScope, err error) {
	admin.Audit(ctx, admin.AuditEvent{
		Action:  action,
		Actor:   actor.GetUsername(),
		Target:  owner.GetUsername(),
		Granted: err == nil,
		Err:     err,
		Fields:  map[string]string{"password_id": ap.PasswordID, "client": ap.Client},
	})
}

func appAuthScope(pw *appauthpb.AppPassword) *scope.AppAuthScope {
	if ap, ok := scope.GetAppAuthScope(pw.GetTokenScope()); ok {
		return ap
	}
	return &scope.AppAuthScope{}
}

func appPasswordResult(pw *appauthpb.AppPassword, createdBy string) invoke.Result {
	ap := appAuthScope(pw)
	res := invoke.Result{
		"password_id":  ap.PasswordID,
		"label":        pw.GetLabel(),
		"client":       ap.Client,
		"permissions":  ap.Permissions,
		"roots":        ap.Roots,
		"created_at":   pw.GetCtime().GetSeconds(),
		"last_seen_at": pw.GetUtime().GetSeconds(),
		"created_by":   createdBy,
	}
	if exp := pw.GetExpiration().GetSeconds(); exp != 0 {
		res["expires_at"] = exp
	}
	return res
}

func splitList(s string) []string {
	var out []string
	for v := range strings.SplitSeq(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
