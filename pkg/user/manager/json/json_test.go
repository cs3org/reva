// Copyright 2018-2020 CERN
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
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

var ctx = context.Background()

func TestUserManager(t *testing.T) {
	// add tempdir
	tempdir, err := ioutil.TempDir("", "json_test")
	if err != nil {
		t.Fatalf("error while create temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	// parseConfig - negative test
	input := map[string]interface{}{
		"users": true,
	}
	_, err = New(input)
	if err == nil {
		t.Fatalf("no error (but we expected one) while get manager")
	}

	// read file - negative test
	input = map[string]interface{}{
		"thisFailsSoHard": "TestingTesting",
	}
	_, err = New(input)
	if err == nil {
		t.Fatalf("no error (but we expected one) while get manager")
	}

	// corrupt json object with user meta data
	userJSON := `[{`

	// get file handler for temporary file
	file, err := ioutil.TempFile(tempdir, "json_test")
	if err != nil {
		t.Fatalf("error while open temp file: %v", err)
	}

	// write json object to tempdir
	_, err = file.WriteString(userJSON)
	if err != nil {
		t.Fatalf("error while writing temp file: %v", err)
	}

	// get manager
	input = map[string]interface{}{
		"users": file.Name(),
	}
	_, err = New(input)
	if err == nil {
		t.Fatalf("no error (but we expected one) while get manager")
	}

	// clean up
	os.Remove(file.Name())

	// json object with user meta data
	userJSON = `[{"id":{"idp":"localhost","opaque_id":"einstein"},"username":"einstein","mail":"einstein@example.org","display_name":"Albert Einstein","groups":["sailing-lovers","violin-haters","physics-lovers"]}]`

	// get file handler for temporary file
	file, err = ioutil.TempFile(tempdir, "json_test")
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
		"users": file.Name(),
	}
	manager, _ := New(input)

	// setup test data
	userEinstein := &userpb.UserId{Idp: "localhost", OpaqueId: "einstein"}
	userFake := &userpb.UserId{Idp: "localhost", OpaqueId: "fakeUser"}
	groupsEinstein := []string{"sailing-lovers", "violin-haters", "physics-lovers"}

	// positive test GetUserGroups
	resGroups, _ := manager.GetUserGroups(ctx, userEinstein)
	if !reflect.DeepEqual(resGroups, groupsEinstein) {
		t.Fatalf("groups differ: expected=%v got=%v", resGroups, groupsEinstein)
	}

	// negative test GetUserGroups
	expectedErr := errtypes.NotFound(userFake.OpaqueId)
	_, err = manager.GetUserGroups(ctx, userFake)
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("user not found error differ: expected='%v' got='%v'", expectedErr, err)
	}

	// test FindUsers
	resUser, _ := manager.FindUsers(ctx, "stein")
	if len(resUser) != 1 {
		t.Fatalf("too many users found: expected=%d got=%d", 1, len(resUser))
	}
	if !reflect.DeepEqual(resUser[0].Username, "einstein") {
		t.Fatalf("user differ: expected=%v got=%v", "einstein", resUser[0].Username)
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
