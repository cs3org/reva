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

package shares

import (
	"testing"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
)

func TestGetStateFilter(t *testing.T) {
	tests := []struct {
		input    string
		expected collaboration.ShareState
	}{
		{"all", ocsStateUnknown},
		{"0", collaboration.ShareState_SHARE_STATE_ACCEPTED},
		{"1", collaboration.ShareState_SHARE_STATE_PENDING},
		{"2", collaboration.ShareState_SHARE_STATE_REJECTED},
		{"something_invalid", collaboration.ShareState_SHARE_STATE_ACCEPTED},
		{"", collaboration.ShareState_SHARE_STATE_ACCEPTED},
	}

	for _, tt := range tests {
		state := getStateFilter(tt.input)
		if state != tt.expected {
			t.Errorf("getStateFilter(\"%s\") returned %s instead of expected %s", tt.input, state, tt.expected)
		}
	}
}

func TestMapState(t *testing.T) {
	// case collaboration.ShareState_SHARE_STATE_PENDING:
	// 	mapped = ocsStatePending
	// case collaboration.ShareState_SHARE_STATE_ACCEPTED:
	// 	mapped = ocsStateAccepted
	// case collaboration.ShareState_SHARE_STATE_REJECTED:
	// 	mapped = ocsStateRejected
	// default:
	// 	mapped = ocsStateUnknown
	tests := []struct {
		input    collaboration.ShareState
		expected int
	}{
		{collaboration.ShareState_SHARE_STATE_PENDING, ocsStatePending},
		{collaboration.ShareState_SHARE_STATE_ACCEPTED, ocsStateAccepted},
		{collaboration.ShareState_SHARE_STATE_REJECTED, ocsStateRejected},
		{42, ocsStateUnknown},
	}

	for _, tt := range tests {
		state := mapState(tt.input)
		if state != tt.expected {
			t.Errorf("mapState(%d) returned %d instead of expected %d", tt.input, state, tt.expected)
		}
	}
}
