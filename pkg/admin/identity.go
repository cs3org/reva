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
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
)

// actAsTokenLifetime is short because the token is used only for the calls of
// one invocation, on the server, and is never given to a client.
const actAsTokenLifetime = 5 * time.Minute

// Lookup finds a user by username through the gateway. The groups are
// included, because a storage can use them to decide, for example whether
// the user administers a project.
func Lookup(ctx context.Context, username string) (*userpb.User, error) {
	if username == "" {
		return nil, errors.New("admin: no username given")
	}
	gw, err := service.Gateway(ctx)
	if err != nil {
		return nil, err
	}
	res, err := gw.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{Claim: "username", Value: username})
	if err != nil {
		return nil, fmt.Errorf("admin: looking up user %q: %w", username, err)
	}
	switch res.GetStatus().GetCode() {
	case rpc.Code_CODE_OK:
		return res.GetUser(), nil
	case rpc.Code_CODE_NOT_FOUND:
		return nil, errtypes.NotFound("user " + username)
	default:
		return nil, fmt.Errorf("admin: looking up user %q: %s: %s", username, res.GetStatus().GetCode(), res.GetStatus().GetMessage())
	}
}

// As returns ctx acting as u: a call made with it is a call by u. An admin
// invocation needs it because no CS3 endpoint accepts the admin token. The
// token has the owner scope and is signed with the shared JWT secret.
func As(ctx context.Context, u *userpb.User) (context.Context, error) {
	secret := sharedconf.GetJWTSecret("")
	if secret == "" {
		return nil, errors.New("admin: jwt secret is not configured")
	}
	tm, err := jwt.New(map[string]any{"secret": secret, "expires": int64(actAsTokenLifetime / time.Second)})
	if err != nil {
		return nil, err
	}
	scopes, err := scope.AddOwnerScope(nil)
	if err != nil {
		return nil, err
	}
	tkn, err := tm.MintToken(ctx, u, scopes)
	if err != nil {
		return nil, fmt.Errorf("admin: minting a token for %q: %w", u.GetUsername(), err)
	}
	return withToken(appctx.ContextSetUser(ctx, u), tkn), nil
}

// ActAs looks username up and returns ctx acting as that user.
func ActAs(ctx context.Context, username string) (context.Context, *userpb.User, error) {
	u, err := Lookup(ctx, username)
	if err != nil {
		return nil, nil, err
	}
	uctx, err := As(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	return uctx, u, nil
}
