// Copyright 2018-2021 CERN
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

	ref := &provider.Reference{
		Spec: &provider.Reference_Path{
			Path: "/",
		},
	}
	val, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := m.MintToken(ctx, u, map[string]*auth.Scope{
		"user": &auth.Scope{
			Resource: &types.OpaqueEntry{
				Decoder: "json",
				Value:   val,
			},
			Role: auth.Role_ROLE_OWNER,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	decodedUser, err := m.DismantleToken(ctx, encoded, nil)
	if err != nil {
		t.Fatal(err)
	}

	if u.Username != decodedUser.Username {
		t.Fatalf("mail claims differ: expected=%s got=%s", u.Username, decodedUser.Username)
	}
}
