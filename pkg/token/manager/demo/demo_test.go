// Copyright 2018-2023 CERN
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

package demo

import (
	"context"
	"encoding/json"
	"testing"

	auth "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

var ctx = context.Background()

func TestEncodeDecode(t *testing.T) {
	m, _ := New(nil)
	u := &user.User{
		Username: "marie",
	}

	ref := &provider.Reference{Path: "/"}
	val, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	scope := map[string]*auth.Scope{
		"user": {
			Resource: &types.OpaqueEntry{
				Decoder: "json",
				Value:   val,
			},
			Role: auth.Role_ROLE_OWNER,
		},
	}

	encoded, err := m.MintToken(ctx, u, scope)
	if err != nil {
		t.Fatal(err)
	}

	decodedUser, decodedScope, err := m.DismantleToken(ctx, encoded)
	if err != nil {
		t.Fatal(err)
	}

	if u.Username != decodedUser.Username {
		t.Fatalf("mail claims differ: expected=%s got=%s", u.Username, decodedUser.Username)
	}

	if s, ok := decodedScope["user"]; !ok || s.Role != auth.Role_ROLE_OWNER {
		t.Fatalf("scope claims differ: expected=%s got=%s", scope, decodedScope)
	}
}
