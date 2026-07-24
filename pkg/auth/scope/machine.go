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
	"errors"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"github.com/cs3org/reva/v3/pkg/utils"
	"google.golang.org/grpc/metadata"
)

// MachineScope marks a token minted through machine authentication. Machine
// auth is otherwise indistinguishable from a normal owner-scoped user token, so
// this marker lets privileged internal RPCs (for example gateway PublishEvent)
// verify that the caller is a reva daemon holding the machine secret. It grants
// no resource access on its own and has no verifier, so VerifyScope ignores it.
const MachineScope = "machine"

// AddMachineScope adds the machine-auth marker scope. It only records that the
// token was issued via machine auth; resource access still comes from the other
// scopes on the token.
func AddMachineScope(scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	ref := &provider.Reference{Path: "/"}
	val, err := utils.MarshalProtoV1ToJSON(ref)
	if err != nil {
		return nil, err
	}
	if scopes == nil {
		scopes = make(map[string]*authpb.Scope)
	}
	scopes[MachineScope] = &authpb.Scope{
		Resource: &types.OpaqueEntry{
			Decoder: "json",
			Value:   val,
		},
		Role: authpb.Role_ROLE_OWNER,
	}
	return scopes, nil
}

// ContextWithMachineScope re-signs the caller's token with the machine (reva
// daemon) scope added, keeping the acting user and any existing scopes
// unchanged. It lets a reva service call daemon-only RPCs as the real user in
// the context, without impersonating anyone. The new token is signed with the
// shared JWT secret, which only reva services hold, so an end user cannot mint
// one themselves. Returns a context carrying the re-signed token.
func ContextWithMachineScope(ctx context.Context) (context.Context, error) {
	u, ok := appctx.ContextGetUser(ctx)
	if !ok || u == nil {
		return nil, errors.New("scope: no user in context to elevate")
	}

	secret := sharedconf.GetJWTSecret("")
	if secret == "" {
		return nil, errors.New("scope: jwt secret is not configured")
	}
	tm, err := jwt.New(map[string]any{"secret": secret})
	if err != nil {
		return nil, err
	}

	existing, _ := appctx.ContextGetScopes(ctx)
	scopes, err := AddMachineScope(cloneScopes(existing))
	if err != nil {
		return nil, err
	}

	tkn, err := tm.MintToken(ctx, u, scopes)
	if err != nil {
		return nil, err
	}

	// Replace, don't append: the request context already carries the original
	// token in the outgoing metadata, and the server reads the first value. A
	// second token would leave the scope-less original in effect.
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.MD{}
	}
	md.Set(appctx.TokenHeader, tkn)

	ctx = appctx.ContextSetToken(ctx, tkn)
	ctx = metadata.NewOutgoingContext(ctx, md)
	return ctx, nil
}

func cloneScopes(in map[string]*authpb.Scope) map[string]*authpb.Scope {
	out := make(map[string]*authpb.Scope, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}
