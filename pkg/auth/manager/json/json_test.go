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

package json

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/stretchr/testify/assert"
)

var ctx = context.Background()

var input map[string]interface{}

type ExpectedError struct {
	message string
}

func TestGetManagerWithInvalidUser(t *testing.T) {
	tests := []struct {
		name          string
		user          interface{}
		expectedError string
	}{
		{
			"Boolean in user",
			false,
			"error decoding conf: 1 error(s) decoding:\n\n* " +
				"'users' expected type 'string', got unconvertible type 'bool', value: 'false'",
		},
		{
			"Nil in user",
			nil,
			"open /etc/revad/users.json: no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input = map[string]interface{}{
				"users": tt.user,
			}

			manager, err := New(input)

			assert.Empty(t, manager)
			assert.EqualError(t, err, tt.expectedError)
		})
	}
}

func TestGetManagerWithJSONObject(t *testing.T) {
	tests := []struct {
		name          string
		user          string
		expectManager bool
		expectedError *ExpectedError
	}{
		{
			"Invalid JSON object",
			"[{",
			false,
			&ExpectedError{"unexpected end of JSON input"},
		},
		{
			"JSON object with correct user metadata",
			`[{"username":"einstein","secret":"albert"}]`,
			true,
			nil,
		},
	}

	var tmpFile *os.File

	// add tempdir
	tmpDir, err := ioutil.TempDir("", "json_test")
	if err != nil {
		t.Fatalf("Error while creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, tt := range tests {
		// get file handler for temporary file
		tmpFile, err = ioutil.TempFile(tmpDir, "json_test")
		if err != nil {
			t.Fatalf("Error while opening temp file: %v", err)
		}

		// write json object to tempdir
		_, err = tmpFile.WriteString(tt.user)
		if err != nil {
			t.Fatalf("Error while writing temp file: %v", err)
		}

		t.Run(tt.name, func(t *testing.T) {
			// get manager
			input = map[string]interface{}{
				"users": tmpFile.Name(),
			}

			manager, err := New(input)

			if tt.expectManager {
				_, ok := manager.(auth.Manager)
				if !ok {
					t.Fatal("Expected response of type auth.Manager but found something else.")
				}
				assert.Equal(t, nil, err)
			} else if !tt.expectManager {
				assert.Empty(t, manager)
				assert.EqualError(t, err, tt.expectedError.message)
			}
		})
		// cleanup
		os.Remove(tmpFile.Name())
	}
}

func TestGetAuthenticatedManager(t *testing.T) {
	tests := []struct {
		name                string
		username            string
		secret              string
		expectAuthenticated bool
		expectedError       *ExpectedError
	}{
		{
			"Authenticate with incorrect user password",
			"einstein",
			"NotARealPassword",
			false,
			&ExpectedError{
				"error: invalid credentials: einstein",
			},
		},
		{
			"Authenticate with correct user auth credentials",
			"einstein",
			"albert",
			true,
			nil,
		},
	}

	// add tempdir
	tempdir, err := ioutil.TempDir("", "json_test")
	if err != nil {
		t.Fatalf("Error while creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	// get file handler for temporary file
	tempFile, err := ioutil.TempFile(tempdir, "json_test")
	if err != nil {
		t.Fatalf("Error while opening temp file: %v", err)
	}

	// write json object to tempdir
	_, err = tempFile.WriteString(`[{"username":"einstein","secret":"albert"}]`)
	if err != nil {
		t.Fatalf("Error while writing temp file: %v", err)
	}

	// get manager
	input := map[string]interface{}{
		"users": tempFile.Name(),
	}
	manager, _ := New(input)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authenticated, err := manager.Authenticate(ctx, tt.username, tt.secret)
			if !tt.expectAuthenticated {
				assert.Empty(t, authenticated)
				assert.EqualError(t, err, tt.expectedError.message)
			} else {
				assert.IsType(t, &user.User{}, authenticated)
				assert.Empty(t, err)
				assert.Equal(t, tt.username, authenticated.Username)
			}
			// cleanup
			os.Remove(tempFile.Name())
		})
	}
}
