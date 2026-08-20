// Copyright 2018-2026 CERN
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

package scope

import "testing"

func TestCheckSharePath(t *testing.T) {
	tests := []struct {
		path    string
		allowed bool
	}{
		// both WebDAV mount URLs must behave the same
		{"/webdav/home/file.txt", true},
		{"/remote.php/webdav/home/file.txt", true},
		{"/remote.php/dav/files/einstein/file.txt", true},
		{"/ocs/v1.php/apps/files_sharing/api/v1/shares", true},
		// the stripped dav endpoint is deliberately not whitelisted
		{"/dav/files/einstein/file.txt", false},
		{"/webapp", false},
		{"/", false},
	}
	for _, tt := range tests {
		if got := checkSharePath(tt.path); got != tt.allowed {
			t.Errorf("checkSharePath(%q) = %v, want %v", tt.path, got, tt.allowed)
		}
	}
}

func TestCheckLightweightPath(t *testing.T) {
	tests := []struct {
		path    string
		allowed bool
	}{
		// both WebDAV mount URLs must behave the same
		{"/webdav/home/file.txt", true},
		{"/remote.php/webdav/home/file.txt", true},
		{"/remote.php/dav/files/einstein/file.txt", true},
		{"/remote.php/dav/spaces/space-id/file.txt", true},
		// the stripped dav endpoint is deliberately not whitelisted
		{"/dav/files/einstein/file.txt", false},
		{"/webapp", false},
		{"/", false},
	}
	for _, tt := range tests {
		if got := checkLightweightPath(tt.path); got != tt.allowed {
			t.Errorf("checkLightweightPath(%q) = %v, want %v", tt.path, got, tt.allowed)
		}
	}
}
