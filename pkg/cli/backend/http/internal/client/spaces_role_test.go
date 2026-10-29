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

package client

import "testing"

// The role must not depend on the order in which the server sends the grants.
func TestRoleOfCallerPicksTheCallersOwnGrant(t *testing.T) {
	me := &User{ID: "u-123", Username: "gdelmont"}

	perms := []Permission{
		{Role: "viewer", GrantedTo: &Identity{ID: "someone-else", Type: "user"}},
		{Role: "manager", GrantedTo: &Identity{ID: "gdelmont", Type: "user"}},
		{Role: "editor", GrantedTo: &Identity{ID: "a-third", Type: "user"}},
	}
	if got := roleOfCaller(perms, me); got != "manager" {
		t.Errorf("got %q, want manager: the caller's own grant is not the first one", got)
	}

	// A server may identify a user by name or by id.
	byID := []Permission{{Role: "manager", GrantedTo: &Identity{ID: "u-123"}}}
	if got := roleOfCaller(byID, me); got != "manager" {
		t.Errorf("got %q, want manager when identified by id", got)
	}
}

// A project root has this shape when the server names nobody.
func TestRoleOfCallerReadsALoneUnattributedGrant(t *testing.T) {
	perms := []Permission{{Role: "editor"}}
	if got := roleOfCaller(perms, &User{Username: "gdelmont"}); got != "editor" {
		t.Errorf("got %q, want editor", got)
	}
	if got := roleOfCaller(perms, nil); got != "editor" {
		t.Errorf("got %q with no caller, want editor", got)
	}
}

func TestRoleOfCallerSaysNothingRatherThanGuess(t *testing.T) {
	me := &User{Username: "gdelmont"}
	perms := []Permission{
		{Role: "viewer", GrantedTo: &Identity{ID: "someone-else"}},
		{Role: "editor", GrantedTo: &Identity{ID: "a-third"}},
	}
	if got := roleOfCaller(perms, me); got != "" {
		t.Errorf("got %q, want nothing: none of these grants are ours", got)
	}

	if got := roleOfCaller([]Permission{{Role: "viewer"}, {Role: "editor"}}, nil); got != "" {
		t.Errorf("got %q, want nothing when two grants name nobody", got)
	}
}

func TestRoleOfCallerIgnoresLinksAndRolelessGrants(t *testing.T) {
	me := &User{Username: "gdelmont"}
	perms := []Permission{
		{Link: &Link{Type: "view"}},
		{Role: "manager", GrantedTo: &Identity{ID: "gdelmont"}},
	}
	if got := roleOfCaller(perms, me); got != "manager" {
		t.Errorf("got %q, want manager", got)
	}
}
