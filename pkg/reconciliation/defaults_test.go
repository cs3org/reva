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

package reconciliation

import (
	"testing"

	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
)

// entryKey renders a DefaultEntry as a stable string for comparison.
func entryKey(d DefaultEntry) string {
	return string(d.Enforcement) + " " + d.Entry.Type + ":" + d.Entry.Qualifier + ":" + d.Entry.Permissions
}

func keys(ds []DefaultEntry) map[string]bool {
	m := make(map[string]bool, len(ds))
	for _, d := range ds {
		m[entryKey(d)] = true
	}
	return m
}

// personalRule is the single rule governing personal spaces: the owner "must"
// entry alongside the global "may" entries.
func personalRule() PathPrefixRule {
	return PathPrefixRule{
		Prefix: "/eos/user",
		DefaultACLs: []DefaultACLRule{
			{Type: acl.TypeUser, Qualifier: "{owner}", Permissions: "rwx", Enforcement: EnforcementMust},
			{Type: acl.TypeGroup, Qualifier: "cbackeosro", Permissions: "rx", Enforcement: EnforcementMay},
			{Type: acl.TypeGroup, Qualifier: "cboxexternal", Permissions: "rx", Enforcement: EnforcementMay},
		},
	}
}

// projectRule is the single rule governing project spaces: the reader/writer/
// admin "must" egroups alongside the global "may" entries.
func projectRule() PathPrefixRule {
	return PathPrefixRule{
		Prefix: "/eos/project",
		DefaultACLs: []DefaultACLRule{
			{Type: acl.TypeGroup, Qualifier: "cernbox-project-{project}-admins", Permissions: "rwx+d", Enforcement: EnforcementMust},
			{Type: acl.TypeGroup, Qualifier: "cernbox-project-{project}-writers", Permissions: "rwx+d", Enforcement: EnforcementMust},
			{Type: acl.TypeGroup, Qualifier: "cernbox-project-{project}-readers", Permissions: "rx", Enforcement: EnforcementMust},
			{Type: acl.TypeGroup, Qualifier: "cbackeosro", Permissions: "rx", Enforcement: EnforcementMay},
			{Type: acl.TypeGroup, Qualifier: "cboxexternal", Permissions: "rx", Enforcement: EnforcementMay},
		},
	}
}

func TestDefaultACLsPersonal(t *testing.T) {
	// Both rules are configured; only the personal one governs a personal space.
	c := &Config{PathPrefixes: []PathPrefixRule{personalRule(), projectRule()}}

	space := &Space{ID: "s1", Type: SpaceTypePersonal, Root: "/eos/user/j/jdoe", Owner: "jdoe"}
	got, err := c.DefaultACLs(space)
	if err != nil {
		t.Fatalf("DefaultACLs: %v", err)
	}

	want := map[string]bool{
		"must u:jdoe:rwx":            true,
		"may egroup:cbackeosro:rx":   true,
		"may egroup:cboxexternal:rx": true,
	}
	if g := keys(got); !equalSet(g, want) {
		t.Errorf("entries = %v, want %v", g, want)
	}
}

func TestDefaultACLsProject(t *testing.T) {
	c := &Config{PathPrefixes: []PathPrefixRule{personalRule(), projectRule()}}

	space := &Space{ID: "s2", Type: SpaceTypeProject, Root: "/eos/project/c/cernbox", Project: "cernbox"}
	got, err := c.DefaultACLs(space)
	if err != nil {
		t.Fatalf("DefaultACLs: %v", err)
	}

	want := map[string]bool{
		"must egroup:cernbox-project-cernbox-admins:rwx+d":  true,
		"must egroup:cernbox-project-cernbox-writers:rwx+d": true,
		"must egroup:cernbox-project-cernbox-readers:rx":    true,
		"may egroup:cbackeosro:rx":                          true,
		"may egroup:cboxexternal:rx":                        true,
	}
	if g := keys(got); !equalSet(g, want) {
		t.Errorf("entries = %v, want %v", g, want)
	}
}

func TestDefaultACLsNoMatch(t *testing.T) {
	c := &Config{PathPrefixes: []PathPrefixRule{personalRule()}}
	// A project space with no project rule configured: no defaults apply.
	space := &Space{ID: "s3", Type: SpaceTypeProject, Root: "/eos/project/c/cernbox", Project: "cernbox"}
	got, err := c.DefaultACLs(space)
	if err != nil {
		t.Fatalf("DefaultACLs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("entries = %v, want none", keys(got))
	}
}

func TestDefaultACLsErrors(t *testing.T) {
	ownerRule := PathPrefixRule{
		Prefix: "/eos/user",
		DefaultACLs: []DefaultACLRule{
			{Type: acl.TypeUser, Qualifier: "{owner}", Permissions: "rwx", Enforcement: EnforcementMust},
		},
	}

	t.Run("invalid space type", func(t *testing.T) {
		c := &Config{PathPrefixes: []PathPrefixRule{ownerRule}}
		if _, err := c.DefaultACLs(&Space{ID: "x", Type: "bogus", Root: "/eos/user/j/jdoe"}); err == nil {
			t.Error("expected error for invalid space type")
		}
	})

	t.Run("nil space", func(t *testing.T) {
		c := &Config{}
		if _, err := c.DefaultACLs(nil); err == nil {
			t.Error("expected error for nil space")
		}
	})

	t.Run("unresolved owner template", func(t *testing.T) {
		c := &Config{PathPrefixes: []PathPrefixRule{ownerRule}}
		// Personal space matching the rule but without an owner set.
		if _, err := c.DefaultACLs(&Space{ID: "x", Type: SpaceTypePersonal, Root: "/eos/user/j/jdoe"}); err == nil {
			t.Error("expected error for missing owner")
		}
	})
}

func TestPathHasPrefix(t *testing.T) {
	tests := []struct {
		path, prefix string
		want         bool
	}{
		{"/eos/user/j/jdoe", "/eos/user", true},
		{"/eos/user", "/eos/user", true},
		{"/eos/user/", "/eos/user", true},
		{"/eos/username/x", "/eos/user", false},
		{"/eos/project/c/cernbox", "/eos/user", false},
		{"/eos/anything", "", true},
		{"/eos/user/j/jdoe", "/eos/user/", true},
	}
	for _, tt := range tests {
		if got := pathHasPrefix(tt.path, tt.prefix); got != tt.want {
			t.Errorf("pathHasPrefix(%q, %q) = %v, want %v", tt.path, tt.prefix, got, tt.want)
		}
	}
}

func equalSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
