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

package demo

import (
	"context"
	"reflect"
	"testing"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/pkg/errtypes"
)

var ctx = context.Background()

func TestUserManager(t *testing.T) {
	// get manager
	manager, _ := New(nil)

	// setup test data
	userEinstein := &typespb.UserId{Idp: "localhost", OpaqueId: "einstein"}
	userFake := &typespb.UserId{Idp: "localhost", OpaqueId: "fakeUser"}
	groupsEinstein := []string{"sailing-lovers", "violin-haters", "physics-lovers"}

	// positive test GetUserGroups
	resGroups, _ := manager.GetUserGroups(ctx, userEinstein)
	if !reflect.DeepEqual(resGroups, groupsEinstein) {
		t.Fatalf("groups differ: expected=%v got=%v", resGroups, groupsEinstein)
	}

	// negative test GetUserGroups
	expectedErr := errtypes.NotFound(userFake.OpaqueId)
	_, err := manager.GetUserGroups(ctx, userFake)
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("user not found error differ: expected='%v' got='%v'", expectedErr, err)
	}

	// test FindUsers
	resUser, _ := manager.FindUsers(ctx, "einstein")
	if !reflect.DeepEqual(resUser, []*authv0alphapb.User{}) {
		t.Fatalf("user differ: expected=%v got=%v", resUser, []*authv0alphapb.User{})
	}

	// positive test IsInGroup
	resInGroup, _ := manager.IsInGroup(ctx, userEinstein, "physics-lovers")
	if !resInGroup {
		t.Fatalf("user not in group: expected=%v got=%v", true, false)
	}

	// negative test IsInGroup with wrong group
	resInGroup, _ = manager.IsInGroup(ctx, userEinstein, "notARealGroup")
	if resInGroup {
		t.Fatalf("user not in group: expected=%v got=%v", true, false)
	}

	// negative test IsInGroup with wrong user
	expectedErr = errtypes.NotFound(userFake.OpaqueId)
	resInGroup, err = manager.IsInGroup(ctx, userFake, "physics-lovers")
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("user not in group error differ: expected='%v' got='%v'", expectedErr, err)
	}
	if resInGroup {
		t.Fatalf("user not in group bool differ: expected='%v' got='%v'", false, resInGroup)
	}
}
