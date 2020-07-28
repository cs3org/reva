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

	"github.com/stretchr/testify/assert"
)

var ctx = context.Background()

func TestUserManager(t *testing.T) {
	// add tempdir
	tempdir, err := ioutil.TempDir("", "json_test")
	if err != nil {
		t.Fatalf("Error while creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	// parseConfig - negative test - 1
	input := map[string]interface{}{
		"users": true,
	}
	_, err = New(input)
	if err == nil {
		t.Fatalf("Expected error while getting manager but found none.")
	}

	tests := []struct {
		name                string
		user                string
		username            string
		secret              string
		expectManager       bool
		expectAuthenticated bool
	}{
		{
			"Corrupt JSON object with user metadata",
			`[{`,
			"nil",
			"nil",
			false,
			false,
		},
		{
			"JSON object with incorrect user metadata",
			`[{"username":"einstein","secret":"albert"}]`,
			"einstein",
			"NotARealPassword",
			true,
			false,
		},
		{
			"JSON object with correct user metadata",
			`[{"username":"einstein","secret":"albert"}]`,
			"einstein",
			"albert",
			true,
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
			input = map[string]interface{}{
				"users": tempFile.Name(),
			}
			manager, err := New(input)
			if !tt.expectManager {
				assert.Equalf(t, nil, manager, "Expected no manager but found: %v", manager)
				assert.NotEqual(t, nil, err, "Expected error while getting manager but found none.")
			} else if manager != nil && tt.expectManager {
				authenticated, err := manager.Authenticate(ctx, tt.username, tt.secret)
				if !tt.expectAuthenticated {
					assert.NotEqual(t, nil, authenticated, "Expected manager but found none.")
					assert.NotEqual(t, nil, err, "Expected error while authenticate about bad credentials, but found none.")
				} else if tt.expectAuthenticated {
					assert.NotEqual(t, nil, authenticated, "Expected an authenticated manager but found none.")
					assert.Equalf(t, nil, err, "Error: %v", err)
					assert.Equal(t, tt.username, authenticated.Username)
				}
			}
			// cleanup
			os.Remove(tempFile.Name())
		})
	}
}
