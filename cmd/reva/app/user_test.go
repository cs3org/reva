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

package revacmd

import (
	"encoding/json"
	"strings"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/clitest"
)

func userBackend() *clitest.Backend {
	b := clitest.New("alice")
	b.AddUser(&userpb.User{
		Id:          &userpb.UserId{Idp: "fake", OpaqueId: "bob", Type: userpb.UserType_USER_TYPE_PRIMARY},
		Username:    "bob",
		DisplayName: "Bob Builder",
		Mail:        "bob@example.org",
		UidNumber:   1001,
		GidNumber:   2000,
		Groups:      []string{"zeta-team", "alpha-team"},
	})
	b.AddUser(&userpb.User{
		Id:          &userpb.UserId{Idp: "fake", OpaqueId: "bobsvc", Type: userpb.UserType_USER_TYPE_SERVICE},
		Username:    "bobsvc",
		DisplayName: "Service of Bob",
		UidNumber:   1002,
	})
	return b
}

func TestUserGet(t *testing.T) {
	b := userBackend()
	for _, args := range [][]string{{"bob"}, {"--claim", "mail", "bob@example.org"}, {"--claim", "uid", "1001"}} {
		out := mustRun(t, b, append([]string{"user", "get"}, args...)...)
		for _, want := range []string{"bob@example.org", "Bob Builder", "1001", "2000", "primary"} {
			if !strings.Contains(out, want) {
				t.Errorf("user get %v = %q, want %s", args, out, want)
			}
		}
	}
	var u struct {
		Username string   `json:"username"`
		Groups   []string `json:"groups"`
	}
	if err := json.Unmarshal([]byte(mustRun(t, b, "user", "get", "bob", "-o", "json")), &u); err != nil || u.Username != "bob" || len(u.Groups) != 2 {
		t.Errorf("user get -o json = %+v, %v: want bob and his groups", u, err)
	}
	if _, err := runFake(t, b, "user", "get", "nobody"); err == nil {
		t.Error("user get of an unknown user: no error")
	}
}

func TestUserFind(t *testing.T) {
	b := userBackend()
	out := mustRun(t, b, "user", "find", "BOB")
	if !strings.Contains(out, "bobsvc") || !strings.Contains(out, "service") || strings.Contains(out, "alice") {
		t.Errorf("user find = %q, want bob and bobsvc of every type, not alice", out)
	}
	var users []struct {
		Username string   `json:"username"`
		Groups   []string `json:"groups"`
	}
	if err := json.Unmarshal([]byte(mustRun(t, b, "user", "find", "example.org", "-o", "json")), &users); err != nil || len(users) != 1 || users[0].Username != "bob" || users[0].Groups != nil {
		t.Errorf("user find -o json = %+v, %v: want bob without groups", users, err)
	}
	if code := run([]string{"user", "find", "bo"}); code != 2 {
		t.Errorf("a short query: exit code %d, want 2", code)
	}
}

func TestUserGroups(t *testing.T) {
	b := userBackend()
	if out := mustRun(t, b, "user", "groups", "bob"); out != "alpha-team\nzeta-team\n" {
		t.Errorf("user groups = %q, want the groups sorted, one per line", out)
	}
	var groups []string
	if err := json.Unmarshal([]byte(mustRun(t, b, "user", "groups", "--claim", "uid", "1001", "-o", "json")), &groups); err != nil || len(groups) != 2 {
		t.Errorf("user groups -o json = %v, %v", groups, err)
	}
}

func TestUserCompletion(t *testing.T) {
	b := userBackend()
	out := mustRun(t, b, "__complete", "user", "groups", "bob")
	if !strings.Contains(out, "bob\tBob Builder") || !strings.Contains(out, "bobsvc\t") {
		t.Errorf("completion = %q, want bob and bobsvc", out)
	}
	if out := mustRun(t, b, "__complete", "user", "get", "--claim", "mail", "bob"); !strings.Contains(out, "bob@example.org") {
		t.Errorf("completion of a mail = %q", out)
	}
	if out := mustRun(t, b, "__complete", "user", "get", "bo"); strings.Contains(out, "bob") {
		t.Errorf("completion of a short prefix = %q, want nothing", out)
	}
}
