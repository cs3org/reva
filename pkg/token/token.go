// Copyright 2018-2019 CERN
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

package token

import (
	"context"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
)

type key int

const tokenKey key = iota

// Claims is the map of attributes to encode into a token
type Claims map[string]interface{}

// Manager is the interface to implement to sign and verify tokens
type Manager interface {
	MintToken(ctx context.Context, u *authv0alphapb.User) (string, error)
	DismantleToken(ctx context.Context, token string) (*authv0alphapb.User, error)
}

// ContextGetToken returns the token if set in the given context.
func ContextGetToken(ctx context.Context) (string, bool) {
	u, ok := ctx.Value(tokenKey).(string)
	return u, ok
}

// ContextMustGetToken panics if token is not in context.
func ContextMustGetToken(ctx context.Context) string {
	u, ok := ContextGetToken(ctx)
	if !ok {
		panic("token not found in context")
	}
	return u
}

// ContextSetToken stores the token in the context.
func ContextSetToken(ctx context.Context, t string) context.Context {
	return context.WithValue(ctx, tokenKey, t)
}
