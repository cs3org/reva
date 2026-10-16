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
	"fmt"
	"strings"
	"time"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/token"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc/metadata"
)

// Identity lets an admin invocation act as a user.
//
// An invocation runs with the admin-only token of the operator, which no
// protected CS3 endpoint accepts: it can look users up, but it cannot stat a
// path, create a share or touch a grant. Those calls are made as the user they
// concern (the owner, the initiator), with a token minted here from the same
// JWT secret the rest of the deployment verifies.
type Identity struct {
	tokens token.Manager
	scope  map[string]*authpb.Scope
}

// NewIdentity returns an Identity minting owner-scoped tokens signed with
// jwtSecret, the [shared] secret of the deployment.
func NewIdentity(jwtSecret string) (*Identity, error) {
	if jwtSecret == "" {
		return nil, fmt.Errorf("admin: a jwt secret is required to act as a user")
	}
	tokens, err := jwt.New(map[string]any{"secret": jwtSecret})
	if err != nil {
		return nil, err
	}
	sc, err := scope.AddOwnerScope(nil)
	if err != nil {
		return nil, err
	}
	return &Identity{tokens: tokens, scope: sc}, nil
}

// Lookup resolves a username through the gateway. GetUserByClaim is
// unprotected, so it works under the admin token; the groups are fetched, as
// a handler may need them to decide whether the user administers a project.
func Lookup(ctx context.Context, username string) (*userpb.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("admin: no username given")
	}
	gw, err := service.Gateway(ctx)
	if err != nil {
		return nil, err
	}
	res, err := gw.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{Claim: "username", Value: username})
	if err != nil {
		return nil, fmt.Errorf("admin: looking up user %q: %w", username, err)
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("admin: user %q: %s", username, res.GetStatus().GetMessage())
	}
	return res.GetUser(), nil
}

// As returns ctx acting as u: the user and a freshly minted token are set on
// the context, and the token replaces the admin token in the outgoing
// metadata, so that a call made with the returned context is a call by u.
func (i *Identity) As(ctx context.Context, u *userpb.User) (context.Context, error) {
	if i == nil {
		return nil, fmt.Errorf("admin: acting as a user is not configured (no jwt secret)")
	}
	tkn, err := i.tokens.MintToken(ctx, u, i.scope)
	if err != nil {
		return nil, fmt.Errorf("admin: minting a token for %q: %w", u.GetUsername(), err)
	}
	ctx = appctx.ContextSetUser(ctx, u)
	ctx = appctx.ContextSetToken(ctx, tkn)
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.MD{}
	}
	md.Set(appctx.TokenHeader, tkn)
	return metadata.NewOutgoingContext(ctx, md), nil
}

// ActAs looks username up and returns a context acting as that user.
func (i *Identity) ActAs(ctx context.Context, username string) (context.Context, *userpb.User, error) {
	u, err := Lookup(ctx, username)
	if err != nil {
		return nil, nil, err
	}
	uctx, err := i.As(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	return uctx, u, nil
}

// Actor is the username of the operator running an invocation, for audit
// lines: the user carried by the admin token, or the synthetic local-root
// user on the Unix socket.
func Actor(ctx context.Context) string {
	if u, ok := appctx.ContextGetUser(ctx); ok && u.GetUsername() != "" {
		return u.GetUsername()
	}
	return "unknown"
}

// ParseTime parses the time arguments the admin invocations accept: RFC 3339,
// a date (interpreted at midnight, local time), or "none", which is reported
// as the zero time and ok.
func ParseTime(v string) (t time.Time, none bool, err error) {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "", "none", "never", "null":
		return time.Time{}, true, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, false, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
		return t, false, nil
	}
	return time.Time{}, false, fmt.Errorf("admin: %q is neither a date (2006-01-02), an RFC 3339 time nor none", v)
}

// FormatTime renders a time for an invocation result.
func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
