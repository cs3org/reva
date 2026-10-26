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
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
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

// ActAsFirst calls fn as each user in turn, until fn returns an OK status. A
// user that cannot be looked up, or that the call refuses with not found or
// permission denied, is skipped: the initiator of a share can have lost the
// right on the resource, and the owner can be a deleted account. ActAsFirst
// returns the user that fn was called as for the last time.
func ActAsFirst(ctx context.Context, users []string, fn func(uctx context.Context) (*rpc.Status, error)) (string, error) {
	var err error = errtypes.BadRequest("no user to act as")
	for _, u := range users {
		uctx, _, lerr := ActAs(ctx, u)
		if lerr != nil {
			err = lerr
			continue
		}
		st, cerr := fn(uctx)
		if cerr != nil {
			return u, cerr
		}
		switch st.GetCode() {
		case rpc.Code_CODE_OK:
			return u, nil
		case rpc.Code_CODE_NOT_FOUND, rpc.Code_CODE_PERMISSION_DENIED:
			err = fmt.Errorf("as %s: %w", u, status.NewErrtypeFromStatus(st))
		default:
			return u, fmt.Errorf("as %s: %w", u, status.NewErrtypeFromStatus(st))
		}
	}
	return "", err
}

// Users keeps one context per user for the length of one invocation, because
// a list often acts as the same few users many times. Ctx is the context of
// the invocation.
type Users struct {
	Ctx  context.Context
	mu   sync.Mutex
	ctxs map[string]context.Context
}

// As returns Ctx acting as username, or nil if that user cannot be looked up.
func (c *Users) As(username string) context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	if uctx, ok := c.ctxs[username]; ok {
		return uctx
	}
	if c.ctxs == nil {
		c.ctxs = map[string]context.Context{}
	}
	uctx, _, err := ActAs(c.Ctx, username)
	if err != nil {
		uctx = nil
	}
	c.ctxs[username] = uctx
	return uctx
}

// ParseExpiry parses the expiration argument of an invocation: a date at
// midnight local time, an RFC 3339 time, or none, which is returned as a zero
// timestamp.
func ParseExpiry(v string) (*typespb.Timestamp, error) {
	if v == "none" {
		return &typespb.Timestamp{}, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		if t, err = time.ParseInLocation(time.DateOnly, v, time.Local); err != nil {
			return nil, errtypes.BadRequest(fmt.Sprintf("expires_at %q is not a date (2006-01-02), an RFC 3339 time or none", v))
		}
	}
	return &typespb.Timestamp{Seconds: uint64(t.Unix())}, nil
}
