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
	"reflect"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

var ctx = context.Background()

func TestUserManager(t *testing.T) {
	// get manager
	manager, _ := New(nil)

	// setup test data
	uidEinstein := &userpb.UserId{Idp: "http://localhost:9998", OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51"}
	userEinstein := &userpb.User{
		Id:          uidEinstein,
		Username:    "einstein",
		Groups:      []string{"sailing-lovers", "violin-haters", "physics-lovers"},
		Mail:        "einstein@example.org",
		DisplayName: "Albert Einstein",
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"uid": &types.OpaqueEntry{Decoder: "plain", Value: []byte("123")},
				"gid": &types.OpaqueEntry{Decoder: "plain", Value: []byte("987")},
			},
		},
	}
	uidFake := &userpb.UserId{Idp: "nonesense", OpaqueId: "fakeUser"}
	groupsEinstein := []string{"sailing-lovers", "violin-haters", "physics-lovers"}

	// positive test GetUserGroups
	resGroups, _ := manager.GetUserGroups(ctx, uidEinstein)
	if !reflect.DeepEqual(resGroups, groupsEinstein) {
		t.Fatalf("groups differ: expected=%v got=%v", groupsEinstein, resGroups)
	}

	// negative test GetUserGroups
	expectedErr := errtypes.NotFound(uidFake.OpaqueId)
	_, err := manager.GetUserGroups(ctx, uidFake)
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("user not found error differs: expected='%v' got='%v'", expectedErr, err)
	}

	// positive test GetUserByClaim by uid
	resUserByUID, _ := manager.GetUserByClaim(ctx, "uid", "123")
	if !reflect.DeepEqual(resUserByUID, userEinstein) {
		t.Fatalf("user differs: expected=%v got=%v", userEinstein, resUserByUID)
	}

	// negative test GetUserByClaim by uid
	expectedErr = errtypes.NotFound("789")
	_, err = manager.GetUserByClaim(ctx, "uid", "789")
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("user not found error differs: expected='%v' got='%v'", expectedErr, err)
	}

	// positive test GetUserByClaim by mail
	resUserByEmail, _ := manager.GetUserByClaim(ctx, "mail", "einstein@example.org")
	if !reflect.DeepEqual(resUserByEmail, userEinstein) {
		t.Fatalf("user differs: expected=%v got=%v", userEinstein, resUserByEmail)
	}

	// test FindUsers
	resUser, _ := manager.FindUsers(ctx, "einstein")
	if !reflect.DeepEqual(resUser, []*userpb.User{userEinstein}) {
		t.Fatalf("user differs: expected=%v got=%v", []*userpb.User{userEinstein}, resUser)
	}

	// negative test FindUsers
	resUsers, _ := manager.FindUsers(ctx, "notARealUser")
	if len(resUsers) > 0 {
		t.Fatalf("user not in group: expected=%v got=%v", []*userpb.User{}, resUsers)
	}
}
