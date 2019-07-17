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

package user

import (
	"context"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
)

type key int

const userKey key = iota

// ContextGetUser returns the user if set in the given context.
func ContextGetUser(ctx context.Context) (*authv0alphapb.User, bool) {
	u, ok := ctx.Value(userKey).(*authv0alphapb.User)
	return u, ok
}

// ContextMustGetUser panics if user is not in context.
func ContextMustGetUser(ctx context.Context) *authv0alphapb.User {
	u, ok := ContextGetUser(ctx)
	if !ok {
		panic("user not found in context")
	}
	return u
}

// ContextSetUser stores the user in the context.
func ContextSetUser(ctx context.Context, u *authv0alphapb.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// Manager is the interface to implement to manipulate users.
type Manager interface {
	GetUser(ctx context.Context, uid *typespb.UserId) (*authv0alphapb.User, error)
	GetUserGroups(ctx context.Context, uid *typespb.UserId) ([]string, error)
	IsInGroup(ctx context.Context, uid *typespb.UserId, group string) (bool, error)
	FindUsers(ctx context.Context, query string) ([]*authv0alphapb.User, error)
}
