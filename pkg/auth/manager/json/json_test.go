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
	"testing"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/stretchr/testify/assert"
)

var ctx = context.Background()

func TestGetManager(t *testing.T) {
	var input map[string]interface{}
	tests := []struct {
		name          string
		user          string
		expectManager bool
		hasError      string
	}{
		{
			"Boolean in user",
			"t", // later converted to boolean value
			false,
			"error decoding conf: 1 error(s) decoding:\n\n* " +
				"'users' expected type 'string', got unconvertible type 'bool'",
		},
		{
			"Nil in user",
			"nil", // later converted to nil value
			false,
			"open /etc/revad/users.json: no such file or directory",
		},
		{
			"Invalid JSON object",
			"[{",
			false,
			"unexpected end of JSON input",
		},
		{
			"JSON object with incorrect user metadata",
			`[{"abc": "albert", "def": "einstein"}]`,
			true,
			"nil",
		},
		{
			"JSON object with incorrect user metadata",
			`[{"username":"einstein","secret":"albert"}]`,
			true,
			"nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tmpFile *os.File

			switch {
			case tt.user == "t":
				input = map[string]interface{}{
					"users": true,
				}
			case tt.user == "nil":
				input = map[string]interface{}{
					"users": nil,
				}
			default:
				// add tempdir
				tmpDir, err := ioutil.TempDir("", "json_test")
				if err != nil {
					t.Fatalf("Error while creating temp dir: %v", err)
				}
				defer os.RemoveAll(tmpDir)

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
				// get manager
				input = map[string]interface{}{
					"users": tmpFile.Name(),
				}
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
				assert.EqualError(t, err, tt.hasError)
			}
			// cleanup
			if tt.user != "t" && tt.user != "nil" {
				os.Remove(tmpFile.Name())
			}
		})
	}
}

func TestGetAuthenticatedManager(t *testing.T) {
	// add tempdir
	tempdir, err := ioutil.TempDir("", "json_test")
	if err != nil {
		t.Fatalf("Error while creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	tests := []struct {
		name                string
		user                string
		username            string
		secret              string
		expectAuthenticated bool
		hasError            string
	}{
		{
			"JSON object with incorrect user metadata",
			`[{"username":"einstein","secret":"albert"}]`,
			"einstein",
			"NotARealPassword",
			false,
			"error: invalid credentials: einstein",
		},
		{
			"JSON object with correct user metadata",
			`[{"username":"einstein","secret":"albert"}]`,
			"einstein",
			"albert",
			true,
			"nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// get file handler for temporary file
			tempFile, err := ioutil.TempFile(tempdir, "json_test")
			if err != nil {
				t.Fatalf("Error while opening temp file: %v", err)
			}

			// write json object to tempdir
			_, err = tempFile.WriteString(tt.user)
			if err != nil {
				t.Fatalf("Error while writing temp file: %v", err)
			}

			// get manager
			input := map[string]interface{}{
				"users": tempFile.Name(),
			}
			manager, _ := New(input)
			authenticated, er := manager.Authenticate(ctx, tt.username, tt.secret)
			if !tt.expectAuthenticated {
				assert.Empty(t, authenticated)
				assert.NotEmpty(t, er, "Expected manager but found none.")
				assert.EqualError(t, er, tt.hasError)
			} else if tt.expectAuthenticated {
				assert.IsType(t, &user.User{}, authenticated)
				assert.NotEmpty(t, authenticated, "Expected an authenticated manager but found none.")
				assert.Empty(t, er)
				assert.Equal(t, tt.username, authenticated.Username)
			}
			// cleanup
			os.Remove(tempFile.Name())
		})
	}
}
