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
	"fmt"
	"io/ioutil"
	"os"
	"testing"

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
	}{
		{
			"Boolean in user",
			"t", // later use as boolean
			false,
		},
		{
			"Invalid JSON object",
			"[{",
			false,
		},
		{
			"JSON object with incorrect user metadata",
			`[{"abc": "albert", "def": "einstein"}]`,
			true,
		},
		{
			"JSON object with incorrect user metadata",
			`[{"username":"einstein","secret":"albert"}]`,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tmpFile *os.File
			if tt.user == "t" {
				input = map[string]interface{}{
					"users": true,
				}

			} else {
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
				if tt.user == "t" {
					assert.Equal(t, nil, manager)
					assert.EqualError(t, err, "error decoding conf: 1 error(s) decoding:\n\n* "+
						"'users' expected type 'string', got unconvertible type 'bool'")
				} else {
					assert.Equal(t, nil, manager)
					assert.EqualError(t, err, "unexpected end of JSON input")
				}
			}
			// cleanup
			if tt.user != "t" {
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
	}{
		{
			"JSON object with incorrect user metadata",
			`[{"username":"einstein","secret":"albert"}]`,
			"einstein",
			"NotARealPassword",
			false,
		},
		{
			"JSON object with correct user metadata",
			`[{"username":"einstein","secret":"albert"}]`,
			"einstein",
			"albert",
			true,
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
			authenticated, e := manager.Authenticate(ctx, tt.username, tt.secret)
			if !tt.expectAuthenticated {
				assert.NotEqual(t, nil, authenticated, "Expected manager but found none.")
				errMsg := fmt.Sprintf("error: invalid credentials: %s", tt.username)
				assert.EqualError(t, e, errMsg, e)
			} else if tt.expectAuthenticated {
				assert.NotEqual(t, nil, authenticated, "Expected an authenticated manager but found none.")
				assert.Equalf(t, nil, e, "%v", e)
				assert.Equal(t, tt.username, authenticated.Username)
			}
			// cleanup
			os.Remove(tempFile.Name())
		})
	}
}
