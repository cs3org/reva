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
	"fmt"
)

type key int

const userKey key = iota

// User represents a user of the system.
type User struct {
	ID          *ID
	Subject     string                 `mapstructure:"sub"`
	Issuer      string                 `mapstructure:"iss"`
	Username    string                 `mapstructure:"username"`
	Groups      []string               `mapstructure:"groups"`
	Mail        string                 `mapstructure:"mail"`
	DisplayName string                 `mapstructure:"display_name"`
	Opaque      map[string]interface{} `mapstructure:"opaque"`
}

// ID uniquely identifies a user
// across all identity providers.
type ID struct {
	IDP      string
	OpaqueID string
}

// String will concatenate the OpaqueID and IDP with an @ if both are set
// If only one is set that is returned
// TODO but only id.IDP does not make sense ¯\_(ツ)_/¯
func (id ID) String() string {
	if id.OpaqueID != "" {
		if id.IDP != "" {
			return fmt.Sprintf("%s@%s", id.OpaqueID, id.IDP)
		}
		return id.OpaqueID
	}
	if id.IDP != "" {
		return id.IDP
	}
	return ""
}

// ContextGetUser returns the user if set in the given context.
func ContextGetUser(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userKey).(*User)
	return u, ok
}

// ContextMustGetUser panics if user is not in context.
func ContextMustGetUser(ctx context.Context) *User {
	u, ok := ContextGetUser(ctx)
	if !ok {
		panic("user not found in context")
	}
	return u
}

// ContextSetUser stores the user in the context.
func ContextSetUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// Manager is the interface to implement to manipulate users.
type Manager interface {
	GetUser(ctx context.Context, username string) (*User, error)
	GetUserGroups(ctx context.Context, username string) ([]string, error)
	IsInGroup(ctx context.Context, username, group string) (bool, error)
	FindUsers(ctx context.Context, query string) ([]*User, error)
}
