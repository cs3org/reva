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

package json

import (
	"context"
	"os"
	"reflect"
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

var ctx = context.Background()

func TestUserManager(t *testing.T) {
	// add tempdir
	tempdir, err := os.MkdirTemp("", "json_test")
	if err != nil {
		t.Fatalf("error while create temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	// corrupt json object with user meta data
	userJSON := `[{`

	// get file handler for temporary file
	file, err := os.CreateTemp(tempdir, "json_test")
	if err != nil {
		t.Fatalf("error while open temp file: %v", err)
	}

	// write json object to tempdir
	_, err = file.WriteString(userJSON)
	if err != nil {
		t.Fatalf("error while writing temp file: %v", err)
	}

	// get manager
	input := map[string]interface{}{
		"groups": file.Name(),
	}
	_, err = New(input)
	if err == nil {
		t.Fatalf("no error (but we expected one) while get manager")
	}

	// clean up
	os.Remove(file.Name())

	// json object with user meta data
	userJSON = `[{"id":{"opaque_id":"sailing-lovers"},"group_name":"sailing-lovers","mail":"sailing-lovers@example.org","display_name":"Sailing Lovers","gid_number":1234,"members":[{"idp":"localhost","opaque_id":"einstein","type":1},{"idp":"localhost","opaque_id":"marie","type":1}]}]`

	// get file handler for temporary file
	file, err = os.CreateTemp(tempdir, "json_test")
	if err != nil {
		t.Fatalf("error while open temp file: %v", err)
	}
	defer os.Remove(file.Name())

	// write json object to tempdir
	_, err = file.WriteString(userJSON)
	if err != nil {
		t.Fatalf("error while writing temp file: %v", err)
	}

	// get manager - positive test
	input = map[string]interface{}{
		"groups": file.Name(),
	}
	manager, _ := New(input)

	// setup test data
	gid := &grouppb.GroupId{OpaqueId: "sailing-lovers"}
	uidEinstein := &userpb.UserId{Idp: "localhost", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_PRIMARY}
	uidMarie := &userpb.UserId{Idp: "localhost", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_PRIMARY}
	members := []*userpb.UserId{uidEinstein, uidMarie}
	group := &grouppb.Group{
		Id:          gid,
		GroupName:   "sailing-lovers",
		Mail:        "sailing-lovers@example.org",
		GidNumber:   1234,
		DisplayName: "Sailing Lovers",
		Members:     members,
	}
	groupWithoutMembers := &grouppb.Group{
		Id:          gid,
		GroupName:   "sailing-lovers",
		Mail:        "sailing-lovers@example.org",
		GidNumber:   1234,
		DisplayName: "Sailing Lovers",
	}
	groupFake := &grouppb.GroupId{OpaqueId: "fake-group"}

	// positive test GetGroup
	resGroup, _ := manager.GetGroup(ctx, gid, false)
	if !reflect.DeepEqual(resGroup, group) {
		t.Fatalf("group differs: expected=%v got=%v", group, resGroup)
	}

	// positive test GetGroup without members
	resGroupWithoutMembers, _ := manager.GetGroup(ctx, gid, true)
	if !reflect.DeepEqual(resGroupWithoutMembers, groupWithoutMembers) {
		t.Fatalf("group differs: expected=%v got=%v", groupWithoutMembers, resGroupWithoutMembers)
	}

	// negative test GetGroup
	expectedErr := errtypes.NotFound(groupFake.OpaqueId)
	_, err = manager.GetGroup(ctx, groupFake, false)
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("group not found error differ: expected='%v' got='%v'", expectedErr, err)
	}

	// positive test GetGroupByClaim by mail
	resGroupByEmail, _ := manager.GetGroupByClaim(ctx, "mail", "sailing-lovers@example.org", false)
	if !reflect.DeepEqual(resGroupByEmail, group) {
		t.Fatalf("group differs: expected=%v got=%v", group, resGroupByEmail)
	}

	// negative test GetGroupByClaim by mail
	expectedErr = errtypes.NotFound("abc@example.com")
	_, err = manager.GetGroupByClaim(ctx, "mail", "abc@example.com", false)
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("group not found error differs: expected='%v' got='%v'", expectedErr, err)
	}

	// test GetMembers
	resMembers, _ := manager.GetMembers(ctx, gid)
	if !reflect.DeepEqual(resMembers, members) {
		t.Fatalf("members differ: expected=%v got=%v", members, resMembers)
	}

	// positive test HasMember
	resMember, _ := manager.HasMember(ctx, gid, uidMarie)
	if resMember != true {
		t.Fatalf("result differs: expected=%v got=%v", true, resMember)
	}

	// negative test HasMember
	resMemberNegative, _ := manager.HasMember(ctx, gid, &userpb.UserId{Idp: "localhost", OpaqueId: "fake-user", Type: userpb.UserType_USER_TYPE_PRIMARY})
	if resMemberNegative != false {
		t.Fatalf("result differs: expected=%v got=%v", false, resMemberNegative)
	}

	// test FindGroups
	resFind, _ := manager.FindGroups(ctx, "sail", false)
	if len(resFind) != 1 {
		t.Fatalf("too many groups found: expected=%d got=%d", 1, len(resFind))
	}
	if !reflect.DeepEqual(resFind[0].GroupName, "sailing-lovers") {
		t.Fatalf("group differ: expected=%v got=%v", "sailing-lovers", resFind[0].GroupName)
	}
}
