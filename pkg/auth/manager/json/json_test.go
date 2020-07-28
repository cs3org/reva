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

	tests := []struct {
		name          string
		user          string
		clientID      string
		clientSecret  string
		expectManager bool
	}{
		{
			"Corrupt JSON object with user metadata",
			`[{`,
			"nil",
			"nil",
			false,
		},
		{
			"JSON object with user metadata",
			`[{"username":"einstein","secret":"albert"}]`,
			"einstein",
			"NotARealPassword",
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
			if manager == nil && !tt.expectManager {
				if err == nil {
					t.Fatalf("Expected error while getting manager but found none.")
				}
			} else if manager != nil && tt.expectManager {
				_, err = manager.Authenticate(ctx, tt.clientID, tt.clientSecret)
				if err == nil {
					t.Fatalf("Expected error while authenticate about bad credentials, but found none.")
				}
			}
			// cleanup
			os.Remove(tempFile.Name())
		})
	}
}
