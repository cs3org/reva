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

package templates

import (
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

type testUnit struct {
	expected string
	template string
	user     *userpb.User
}

var tests = []*testUnit{
	&testUnit{
		expected: "alabasta",
		user: &userpb.User{
			Username: "alabasta",
		},
		template: "{{.Username}}",
	},
	&testUnit{
		expected: "a/alabasta",
		user: &userpb.User{
			Username: "alabasta",
		},
		template: "{{substr 0 1 .Username}}/{{.Username}}",
	},
	&testUnit{
		expected: "idp@opaque",
		user: &userpb.User{
			Id: &userpb.UserId{
				Idp:      "idp",
				OpaqueId: "opaque",
			},
		},
		template: "{{.Id.Idp}}@{{.Id.OpaqueId}}",
	},
	&testUnit{ // test path clean
		expected: "/alabasta",
		user: &userpb.User{
			Username: "alabasta",
		},
		template: "///{{.Username}}",
	},
	&testUnit{
		expected: "michael",
		user: &userpb.User{
			Username: "MICHAEL",
		},
		template: "{{lower .Username}}",
	},
	&testUnit{
		expected: "somewhere.com/michael@somewhere.com",
		user: &userpb.User{
			Username: "michael@somewhere.com",
		},
		template: "{{.Email.Domain}}/{{.Username}}",
	},
	&testUnit{
		expected: "somewhere.com/michael",
		user: &userpb.User{
			Username: "michael@somewhere.com",
		},
		template: "{{.Email.Domain}}/{{.Email.Local}}",
	},
	&testUnit{
		expected: "_unknown/michael",
		user: &userpb.User{
			Username: "michael",
		},
		template: "{{.Email.Domain}}/{{.Username}}",
	},
}

func TestLayout(t *testing.T) {
	for _, u := range tests {
		got := WithUser(u.user, u.template)
		if u.expected != got {
			t.Fatal("expected: " + u.expected + " got: " + got)
		}
	}
}

func TestLayoutPanic(t *testing.T) {
	assertPanic(t, testBadLayout)
}

func TestUserPanic(t *testing.T) {
	assertPanic(t, testBadUser)
}

// should panic
func testBadLayout() {
	layout := "{{ bad layout syntax"
	user := &userpb.User{}
	WithUser(user, layout)
}

// should panic
func testBadUser() {
	layout := "{{ .DoesNotExist }}"
	user := &userpb.User{}
	WithUser(user, layout)
}

func assertPanic(t *testing.T, f func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("the code did not panic")
		}
	}()
	f()
}
