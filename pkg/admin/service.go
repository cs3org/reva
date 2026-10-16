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
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc/metadata"
)

const serviceTokenLifetime = 5 * time.Minute

type serviceToken struct {
	token   string
	expires time.Time
}

var (
	serviceTokensMu sync.Mutex
	serviceTokens   = map[string]serviceToken{}
	now             = time.Now
)

// AsService returns ctx acting as the internal service name on the Admin API:
// its outgoing token is replaced by an admin-scoped one for a service identity.
// The name is for the audit log only: any holder of the shared JWT secret can
// claim any name.
func AsService(ctx context.Context, name string) (context.Context, error) {
	tkn, err := serviceTokenFor(ctx, name)
	if err != nil {
		return nil, err
	}

	// Replace, do not append: the server reads the first token, and ctx can
	// already carry the caller's user token.
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.MD{}
	}
	md.Set(appctx.TokenHeader, tkn)

	ctx = appctx.ContextSetToken(ctx, tkn)
	return metadata.NewOutgoingContext(ctx, md), nil
}

func serviceTokenFor(ctx context.Context, name string) (string, error) {
	serviceTokensMu.Lock()
	defer serviceTokensMu.Unlock()

	t := now()
	if c, ok := serviceTokens[name]; ok && c.expires.Sub(t) > serviceTokenLifetime/2 {
		return c.token, nil
	}

	secret := sharedconf.GetJWTSecret("")
	if secret == "" {
		return "", errors.New("admin: jwt secret is not configured")
	}
	tm, err := jwt.New(map[string]any{"secret": secret, "expires": int64(serviceTokenLifetime / time.Second)})
	if err != nil {
		return "", err
	}
	scopes, err := scope.AddAdminScope(nil)
	if err != nil {
		return "", err
	}
	username := "service:" + name
	u := &userpb.User{
		Id:       &userpb.UserId{Idp: "reva", OpaqueId: username, Type: userpb.UserType_USER_TYPE_SERVICE},
		Username: username,
	}
	tkn, err := tm.MintToken(ctx, u, scopes)
	if err != nil {
		return "", fmt.Errorf("admin: minting the token of service %s: %w", name, err)
	}
	serviceTokens[name] = serviceToken{token: tkn, expires: t.Add(serviceTokenLifetime)}
	appctx.GetLogger(ctx).Debug().Str("service", name).Msg("admin: minted a service admin token")
	return tkn, nil
}
