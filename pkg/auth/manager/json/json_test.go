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

	//"reflect"
	//"fmt"
	"testing"
	//user "github.com/cs3org/go-cs3apis/cs3/types"
	//"github.com/cs3org/reva/pkg/errtypes"
)

var ctx = context.Background()

func TestUserManager(t *testing.T) {
	// add tempdir
	tempdir, err := ioutil.TempDir("", "json_test")
	if err != nil {
		t.Fatalf("error while create temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	// parseConfig - negative test - 1
	input := map[string]interface{}{
		"users": true,
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
	userJSON = `[{"username":"einstein","secret":"albert"}]`

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

	// Authenticate - positive test
	_, err = manager.Authenticate(ctx, "einstein", "albert")
	if err != nil {
		t.Fatalf("error while authenticate with correct credentials")
	}

	// Authenticate - negative test
	_, err = manager.Authenticate(ctx, "einstein", "NotARealPassword")
	if err == nil {
		t.Fatalf("no error (but we expected one) while authenticate with bad credentials")
	}
}
