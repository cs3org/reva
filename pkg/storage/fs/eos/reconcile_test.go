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

package eos

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/reconciliation"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/storage/fs/eos/nsdump"
)

func user(uid string, level sharehierarchy.PermLevel) *reconciliation.ACLEntry {
	return &reconciliation.ACLEntry{Type: provider.GranteeType_GRANTEE_TYPE_USER, Qualifier: uid, Permissions: level}
}

func group(name string, level sharehierarchy.PermLevel) *reconciliation.ACLEntry {
	return &reconciliation.ACLEntry{Type: provider.GranteeType_GRANTEE_TYPE_GROUP, Qualifier: name, Permissions: level}
}

func formatACLs(acls []*reconciliation.ACLEntry) string {
	out := ""
	for _, e := range acls {
		out += fmt.Sprintf("[%s %s %s]", e.Type, e.Qualifier, e.Permissions)
	}
	return out
}

func sameACLs(got, want []*reconciliation.ACLEntry) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if *got[i] != *want[i] {
			return false
		}
	}
	return true
}

// The fixture is what eos-ns-inspect prints for a small project space. Every
// node in it is one the job compares, and each sys.acl comes out by level.
func TestNSDumpInfoOfTheFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/nsdump_cernbox.json")
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	var entries []nsdump.NameSpaceEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("decoding the fixture: %v", err)
	}

	byPath := map[string]reconciliation.NSDumpInfo{}
	for _, e := range entries {
		info, ok := toNSDumpInfo(e)
		if !ok {
			t.Errorf("%q was left out of the dump", e.Path)
			continue
		}
		byPath[info.Path] = info
	}
	if len(byPath) != 13 {
		t.Errorf("the dump holds %d nodes, want 13", len(byPath))
	}

	const root = "/eos/project/c/cernbox"
	tests := []struct {
		path     string
		wantType provider.ResourceType
		want     []*reconciliation.ACLEntry
	}{
		{root + "/shared/", provider.ResourceType_RESOURCE_TYPE_CONTAINER, []*reconciliation.ACLEntry{user("alice", sharehierarchy.PermRead)}},
		{root + "/shared/todo.md", provider.ResourceType_RESOURCE_TYPE_FILE, nil},
		{root + "/shared/external.md", provider.ResourceType_RESOURCE_TYPE_FILE, []*reconciliation.ACLEntry{
			user("alice", sharehierarchy.PermRead), user("cboxexternal", sharehierarchy.PermRW),
		}},
		{root + "/team/", provider.ResourceType_RESOURCE_TYPE_CONTAINER, []*reconciliation.ACLEntry{group("cernbox-admins", sharehierarchy.PermRW)}},
	}
	for _, tt := range tests {
		info, ok := byPath[tt.path]
		if !ok {
			t.Errorf("the dump has no node %q", tt.path)
			continue
		}
		if info.Type != tt.wantType {
			t.Errorf("%q is a %s, want a %s", tt.path, info.Type, tt.wantType)
		}
		if !sameACLs(info.ACLs, tt.want) {
			t.Errorf("%q holds %s, want %s", tt.path, formatACLs(info.ACLs), formatACLs(tt.want))
		}
	}
}

// The cases of one node: the permission strings EOS really holds, the entries
// the job must not see, and the nodes the job must not touch.
func TestNSDumpInfo(t *testing.T) {
	const dir = "/eos/user/j/jdoe"

	tests := []struct {
		name       string
		entry      nsdump.NameSpaceEntry
		wantKept   bool
		wantACLs   []*reconciliation.ACLEntry
		wantACLsOf string
	}{
		{
			name:     "the permissions AddGrant writes",
			entry:    nsdump.NameSpaceEntry{CID: "1", Path: dir + "/a/", XattrSysAcl: "u:173503:rwx+d,u:178352:rx!d"},
			wantKept: true,
			wantACLs: []*reconciliation.ACLEntry{user("173503", sharehierarchy.PermRW), user("178352", sharehierarchy.PermRead)},
		},
		{
			name:     "a denial",
			entry:    nsdump.NameSpaceEntry{CID: "1", Path: dir + "/a/", XattrSysAcl: "u:173503:!r!w!x!m!u!d"},
			wantKept: true,
			wantACLs: []*reconciliation.ACLEntry{user("173503", sharehierarchy.PermDeny)},
		},
		{
			name:     "a permission no share writes still has a level",
			entry:    nsdump.NameSpaceEntry{CID: "1", Path: dir + "/a/", XattrSysAcl: "u:58679:rxt"},
			wantKept: true,
			wantACLs: []*reconciliation.ACLEntry{user("58679", sharehierarchy.PermRead)},
		},
		{
			name:     "an empty entry in the list",
			entry:    nsdump.NameSpaceEntry{CID: "1", Path: dir + "/a/", XattrSysAcl: ",u:187895:rx"},
			wantKept: true,
			wantACLs: []*reconciliation.ACLEntry{user("187895", sharehierarchy.PermRead)},
		},
		{
			// a unix group is not a grantee a share can have
			name:     "an entry of another type is left out",
			entry:    nsdump.NameSpaceEntry{CID: "1", Path: dir + "/a/", XattrSysAcl: "g:2763:rx,u:173503:rwx"},
			wantKept: true,
			wantACLs: []*reconciliation.ACLEntry{user("173503", sharehierarchy.PermRW)},
		},
		{
			name:  "an acl that does not parse leaves the node alone",
			entry: nsdump.NameSpaceEntry{CID: "1", Path: dir + "/a/", XattrSysAcl: "this is not an acl"},
		},
		{
			name:       "a version folder carries the acl of its file",
			entry:      nsdump.NameSpaceEntry{CID: "1", Path: dir + "/.sys.v#.notes.md/", XattrSysAcl: "u:173503:rwx"},
			wantKept:   true,
			wantACLs:   []*reconciliation.ACLEntry{user("173503", sharehierarchy.PermRW)},
			wantACLsOf: dir + "/notes.md",
		},
		{
			name:  "a version in a version folder",
			entry: nsdump.NameSpaceEntry{FID: "2", Path: dir + "/.sys.v#.notes.md/1737542468.05d85a7b"},
		},
		{
			name:  "an atomic upload",
			entry: nsdump.NameSpaceEntry{FID: "2", Path: dir + "/.sys.a#.notes.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := toNSDumpInfo(tt.entry)
			if ok != tt.wantKept {
				t.Fatalf("kept = %v, want %v", ok, tt.wantKept)
			}
			if !ok {
				return
			}
			if !sameACLs(info.ACLs, tt.wantACLs) {
				t.Errorf("acls = %s, want %s", formatACLs(info.ACLs), formatACLs(tt.wantACLs))
			}
			if info.ACLsOf != tt.wantACLsOf {
				t.Errorf("acls of = %q, want %q", info.ACLsOf, tt.wantACLsOf)
			}
		})
	}
}
