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

package log

import "testing"

func TestRedactLoginToken(t *testing.T) {
	cases := []struct {
		uri  string
		want string
	}{
		{
			"/index.php/login/v2/flow/abc123",
			"/index.php/login/v2/flow/<redacted>",
		},
		{
			"/ocs/v2.php/cloud/user/login-flow/abc123",
			"/ocs/v2.php/cloud/user/login-flow/<redacted>",
		},
		{
			"/ocs/v2.php/cloud/user/login-flow/abc123/grant",
			"/ocs/v2.php/cloud/user/login-flow/<redacted>/grant",
		},
		{
			"/index.php/login/v2/flow/abc123?foo=bar",
			"/index.php/login/v2/flow/<redacted>?foo=bar",
		},
		// No token to hide.
		{"/index.php/login/v2/flow/", "/index.php/login/v2/flow/"},
		{"/index.php/login/v2/poll", "/index.php/login/v2/poll"},
		{"/ocs/v2.php/cloud/user/clients", "/ocs/v2.php/cloud/user/clients"},
		{"/remote.php/dav/files/jdoe/x.txt", "/remote.php/dav/files/jdoe/x.txt"},
	}
	for _, c := range cases {
		if got := redactLoginToken(c.uri); got != c.want {
			t.Errorf("redactLoginToken(%q) = %q, want %q", c.uri, got, c.want)
		}
	}
}
