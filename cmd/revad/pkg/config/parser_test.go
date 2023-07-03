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

package config

import (
	"testing"

	"gotest.tools/assert"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		key   string
		token string
		next  string
	}{
		{
			key:   ".grpc.services.authprovider[1].address",
			token: "grpc",
			next:  ".services.authprovider[1].address",
		},
		{
			key:   "[1].address",
			token: "1",
			next:  ".address",
		},
		{
			key:   "[100].address",
			token: "100",
			next:  ".address",
		},
		{
			key: "",
		},
	}

	for _, tt := range tests {
		token, next := split(tt.key)
		assert.Equal(t, token, tt.token)
		assert.Equal(t, next, tt.next)
	}
}

func TestParseNext(t *testing.T) {
	tests := []struct {
		key  string
		cmd  Command
		next string
		err  error
	}{
		{
			key:  ".grpc.services.authprovider[1].address",
			cmd:  FieldByKey{Key: "grpc"},
			next: ".services.authprovider[1].address",
		},
		{
			key:  ".authprovider[1].address",
			cmd:  FieldByKey{Key: "authprovider"},
			next: "[1].address",
		},
		{
			key:  "[1].authprovider.address",
			cmd:  FieldByIndex{Index: 1},
			next: ".authprovider.address",
		},
		{
			key:  ".authprovider",
			cmd:  FieldByKey{Key: "authprovider"},
			next: "",
		},
	}

	for _, tt := range tests {
		cmd, next, err := parseNext(tt.key)
		assert.Equal(t, err, tt.err)
		if tt.err == nil {
			assert.Equal(t, cmd, tt.cmd)
			assert.Equal(t, next, tt.next)
		}
	}
}
