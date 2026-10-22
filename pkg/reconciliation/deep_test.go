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
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/spaces"

	"github.com/cs3org/reva/v3/pkg/reconciliation/nsdump"
	"github.com/cs3org/reva/v3/pkg/storage/fs/eos/acl"
)

// testSpaceRoot is the space the synthetic namespace dump covers.
const testSpaceRoot = "/eos/project/c/cernbox"

// The entries of testdata/nsdump_cernbox.json. Folder paths carry a trailing
// separator, the way eos-ns-inspect prints them.
const (
	nsRoot        = testSpaceRoot + "/"
	nsShared      = testSpaceRoot + "/shared/"
	nsNotes       = testSpaceRoot + "/shared/notes.md"
	nsTodo        = testSpaceRoot + "/shared/todo.md"
	nsExternal    = testSpaceRoot + "/shared/external.md"
	nsInner       = testSpaceRoot + "/shared/inner/"
	nsPlan        = testSpaceRoot + "/shared/inner/plan.md"
	nsSharedDoc   = testSpaceRoot + "/shared-doc.md"
	nsWrongPerms  = testSpaceRoot + "/wrongperms/"
	nsTeam        = testSpaceRoot + "/team/"
	nsTeamReport  = testSpaceRoot + "/team/report.md"
	nsPrivate     = testSpaceRoot + "/private/"
	nsPrivateFile = testSpaceRoot + "/private/secret.md"
)

func aclEntry(t, qualifier, permissions string) *acl.Entry {
	return &acl.Entry{Type: t, Qualifier: qualifier, Permissions: permissions}
}

func aliceEntry() *acl.Entry    { return aclEntry(acl.TypeUser, "alice", "rx") }
func bobEntry() *acl.Entry      { return aclEntry(acl.TypeUser, "bob", "rwx") }
func carolEntry() *acl.Entry    { return aclEntry(acl.TypeUser, "carol", "rwx") }
func carolWrong() *acl.Entry    { return aclEntry(acl.TypeUser, "carol", "rx") }
func adminsEntry() *acl.Entry   { return aclEntry(acl.TypeGroup, "cernbox-admins", "rwx") }
func malloryEntry() *acl.Entry  { return aclEntry(acl.TypeUser, "mallory", "rwx") }
func daveEntry() *acl.Entry     { return aclEntry(acl.TypeUser, "dave", "rwx") }
func frankEntry() *acl.Entry    { return aclEntry(acl.TypeUser, "frank", "rx") }
func externalEntry() *acl.Entry { return aclEntry(acl.TypeUser, "cboxexternal", "rwx") }

// loadTestDump reads the synthetic namespace through the file dumper, which is
// the same parser the job uses on the output of eos-ns-inspect.
func loadTestDump(t *testing.T) *nsdump.NamespaceDump {
	t.Helper()

	dumper := &nsdump.EOSFileNSInspect{}
	if err := dumper.Setup(map[string]any{"file": "testdata/nsdump_cernbox.json"}); err != nil {
		t.Fatalf("setting up the file dumper: %v", err)
	}

	dump, err := dumper.Dump(testSpaceRoot, 0)
	if err != nil {
		t.Fatalf("reading the namespace dump: %v", err)
	}
	return dump
}

// testTree is the ideal state of the synthetic namespace: four shares, plus one
// entry that is allowed anywhere in the space without a share behind it.
func testTree() *ACLTree {
	tree := NewACLTree()
	tree.Insert(&ACLNode{
		Path:        testSpaceRoot,
		AllowedACLs: []*acl.Entry{externalEntry()},
	})
	tree.Insert(&ACLNode{
		Path:          testSpaceRoot + "/shared",
		MandatoryACLs: []*acl.Entry{aliceEntry()},
	})
	tree.Insert(&ACLNode{
		Path:          testSpaceRoot + "/shared/inner",
		MandatoryACLs: []*acl.Entry{bobEntry()},
	})
	tree.Insert(&ACLNode{
		Path:          testSpaceRoot + "/wrongperms",
		MandatoryACLs: []*acl.Entry{carolEntry()},
	})
	tree.Insert(&ACLNode{
		Path:          testSpaceRoot + "/team",
		MandatoryACLs: []*acl.Entry{adminsEntry()},
	})
	return tree
}

func formatChange(c *Change) string {
	return fmt.Sprintf("%s %s %s:%s:%s", c.Action, c.Path, c.ACL.Type, c.ACL.Qualifier, c.ACL.Permissions)
}

func formatChanges(cs ChangeSet) string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = formatChange(c)
	}
	slices.Sort(out)
	return "\n\t" + strings.Join(out, "\n\t")
}

// sameChanges compares two change sets without regard to their order.
func sameChanges(got, want ChangeSet) bool {
	g, w := make([]string, len(got)), make([]string, len(want))
	for i, c := range got {
		g[i] = formatChange(c)
	}
	for i, c := range want {
		w[i] = formatChange(c)
	}
	slices.Sort(g)
	slices.Sort(w)
	return slices.Equal(g, w)
}

func checkChanges(t *testing.T, got, want ChangeSet) {
	t.Helper()
	if !sameChanges(got, want) {
		t.Errorf("changes = %s\n\nwant = %s", formatChanges(got), formatChanges(want))
	}
}

// The fixture must stay what the tests assume: the parser must give every entry
// its kind, and folder paths keep the trailing separator eos-ns-inspect prints.
func TestNamespaceDumpFixture(t *testing.T) {
	dump := loadTestDump(t)

	byPath := make(map[string]*nsdump.NameSpaceEntry, len(dump.Entries))
	for i := range dump.Entries {
		byPath[dump.Entries[i].Path] = &dump.Entries[i]
	}

	for _, p := range []string{nsRoot, nsShared, nsNotes, nsTodo, nsExternal, nsInner,
		nsPlan, nsSharedDoc, nsWrongPerms, nsTeam, nsTeamReport, nsPrivate, nsPrivateFile} {
		if _, ok := byPath[p]; !ok {
			t.Errorf("the dump has no entry for %q", p)
		}
	}
	if len(dump.Entries) != 13 {
		t.Errorf("the dump holds %d entries, want 13", len(dump.Entries))
	}

	if got := byPath[nsShared].EntryType(); got != nsdump.EntryTypeFolder {
		t.Errorf("%q is a %q, want a folder", nsShared, got)
	}
	if got := byPath[nsNotes].EntryType(); got != nsdump.EntryTypeFile {
		t.Errorf("%q is a %q, want a file", nsNotes, got)
	}
	if got := byPath[nsShared].XattrSysAcl; got != "u:alice:rx" {
		t.Errorf("%q holds acl %q, want u:alice:rx", nsShared, got)
	}
	if got := byPath[nsTodo].XattrSysAcl; got != "" {
		t.Errorf("%q holds acl %q, want none", nsTodo, got)
	}
}

// The whole comparison, over a namespace that holds one of every case.
func TestCompareAgainstNamespace(t *testing.T) {
	dump := loadTestDump(t)

	want := ChangeSet{
		// the file below the share carries no entry at all
		{Path: nsTodo, Action: ActionAdd, ACL: aliceEntry()},
		// the deeper share is not on the folder it was made on
		{Path: nsInner, Action: ActionAdd, ACL: bobEntry()},
		// a file whose name starts with the name of the shared folder is not
		// in the share, so its entry has nothing behind it
		{Path: nsSharedDoc, Action: ActionDelete, ACL: aliceEntry()},
		// the right user with the wrong permissions: the entry goes and the
		// right one takes its place
		{Path: nsWrongPerms, Action: ActionDelete, ACL: carolWrong()},
		{Path: nsWrongPerms, Action: ActionAdd, ACL: carolEntry()},
		// one entry too many below the group share
		{Path: nsTeamReport, Action: ActionDelete, ACL: malloryEntry()},
		// a subtree with no share at all
		{Path: nsPrivate, Action: ActionDelete, ACL: daveEntry()},
	}

	checkChanges(t, compare(testTree(), dump), want)
}

// The cases of one entry, without the namespace around them.
func TestCalculateChangeSet(t *testing.T) {
	const p = "/eos/project/c/cernbox/shared"

	tests := []struct {
		name                        string
		mandatory, optional, actual []*acl.Entry
		want                        ChangeSet
	}{
		{
			name: "nothing wanted and nothing there",
		},
		{
			name:      "the wanted entry is there",
			mandatory: []*acl.Entry{aliceEntry()},
			actual:    []*acl.Entry{aliceEntry()},
		},
		{
			name:      "the wanted entry is missing",
			mandatory: []*acl.Entry{aliceEntry()},
			want:      ChangeSet{{Path: p, Action: ActionAdd, ACL: aliceEntry()}},
		},
		{
			name:      "one of two wanted entries is missing",
			mandatory: []*acl.Entry{aliceEntry(), bobEntry()},
			actual:    []*acl.Entry{aliceEntry()},
			want:      ChangeSet{{Path: p, Action: ActionAdd, ACL: bobEntry()}},
		},
		{
			name:   "an entry nothing asks for",
			actual: []*acl.Entry{daveEntry()},
			want:   ChangeSet{{Path: p, Action: ActionDelete, ACL: daveEntry()}},
		},
		{
			name:     "an allowed entry stays",
			optional: []*acl.Entry{externalEntry()},
			actual:   []*acl.Entry{externalEntry()},
		},
		{
			name:      "an allowed entry next to a wanted one",
			mandatory: []*acl.Entry{aliceEntry()},
			optional:  []*acl.Entry{externalEntry()},
			actual:    []*acl.Entry{aliceEntry(), externalEntry()},
		},
		{
			name:      "the right user with the wrong permissions",
			mandatory: []*acl.Entry{carolEntry()},
			actual:    []*acl.Entry{carolWrong()},
			want: ChangeSet{
				{Path: p, Action: ActionAdd, ACL: carolEntry()},
				{Path: p, Action: ActionDelete, ACL: carolWrong()},
			},
		},
		{
			name:      "a group entry is another entry than a user entry",
			mandatory: []*acl.Entry{adminsEntry()},
			actual:    []*acl.Entry{aclEntry(acl.TypeUser, "cernbox-admins", "rwx")},
			want: ChangeSet{
				{Path: p, Action: ActionAdd, ACL: adminsEntry()},
				{Path: p, Action: ActionDelete, ACL: aclEntry(acl.TypeUser, "cernbox-admins", "rwx")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateChangeSet(tt.mandatory, tt.optional, tt.actual, p)
			checkChanges(t, got, tt.want)
		})
	}
}

func TestParseACLs(t *testing.T) {
	tests := []struct {
		name    string
		sysattr string
		want    []*acl.Entry
	}{
		{
			name:    "one user entry",
			sysattr: "u:alice:rx",
			want:    []*acl.Entry{aliceEntry()},
		},
		{
			name:    "a user and a group",
			sysattr: "u:alice:rx,egroup:cernbox-admins:rwx",
			want:    []*acl.Entry{aliceEntry(), adminsEntry()},
		},
		{
			name:    "no acl at all",
			sysattr: "",
		},
		{
			name:    "an entry that does not parse",
			sysattr: "this is not an acl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseACLs(tt.sysattr); !sameEntries(got, tt.want) {
				t.Errorf("parseACLs(%q) = %v, want %v", tt.sysattr, formatEntries(got), formatEntries(tt.want))
			}
		})
	}
}

// testStorageID is the storage the shares of the test space point at.
const testStorageID = "eosproject-c"

// spaceShare builds a share of the test space with the fields the deep job
// reads: the space it belongs to, the resource it points at, the grantee and
// the permissions.
func spaceShare(id, spaceID, inode, shareWith string, isGroup bool, perms *provider.ResourcePermissions) storedShare {
	s := &collaboration.Share{
		Id: &collaboration.ShareId{OpaqueId: id},
		ResourceId: &provider.ResourceId{
			StorageId: testStorageID,
			SpaceId:   spaceID,
			OpaqueId:  inode,
		},
		Permissions: &collaboration.SharePermissions{Permissions: perms},
	}
	if isGroup {
		s.Grantee = &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: shareWith}},
		}
	} else {
		s.Grantee = &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: shareWith}},
		}
	}
	return storedShare{share: s}
}

// readOnly and readWrite are what OCSFromCS3Permission reads to decide between
// rx and rwx.
func readOnly() *provider.ResourcePermissions {
	return &provider.ResourcePermissions{InitiateFileDownload: true}
}

func readWrite() *provider.ResourcePermissions {
	return &provider.ResourcePermissions{InitiateFileDownload: true, InitiateFileUpload: true}
}

// recordingDumper keeps the root path a run asked for, which is the space path
// the job decoded out of the space id.
type recordingDumper struct {
	nsdump.NSDumpClient
	rootPath string
}

func (d *recordingDumper) Dump(rootPath string, maxDepth int) (*nsdump.NamespaceDump, error) {
	d.rootPath = rootPath
	return d.NSDumpClient.Dump(rootPath, maxDepth)
}

// failingDumper stands for a namespace that cannot be read.
type failingDumper struct {
	err error
}

func (d *failingDumper) Setup(config any) error { return nil }

func (d *failingDumper) Dump(rootPath string, maxDepth int) (*nsdump.NamespaceDump, error) {
	return nil, d.err
}

// fileDumper reads the synthetic namespace.
func fileDumper(t *testing.T) nsdump.NSDumpClient {
	t.Helper()
	d := &nsdump.EOSFileNSInspect{}
	if err := d.Setup(map[string]any{"file": "testdata/nsdump_cernbox.json"}); err != nil {
		t.Fatalf("setting up the file dumper: %v", err)
	}
	return d
}

// A whole run over the synthetic namespace, from the shares to the change set.
func TestRunAnalysis(t *testing.T) {
	spaceID := spaces.EncodeSpaceID(testSpaceRoot)
	otherSpaceID := spaces.EncodeSpaceID("/eos/project/o/other")

	store := &fakeStore{shares: []storedShare{
		spaceShare("1", spaceID, "1001", "alice", false, readOnly()),
		spaceShare("2", spaceID, "1002", "bob", false, readWrite()),
		spaceShare("3", spaceID, "1003", "carol", false, readWrite()),
		spaceShare("4", spaceID, "1004", "cernbox-admins", true, readWrite()),
		// a second share of the folder share 4 points at
		spaceShare("5", spaceID, "1004", "frank", false, readOnly()),
		// the resource is gone, so the path cannot be resolved and the share
		// cannot hold up an entry
		spaceShare("6", spaceID, "9999", "dave", false, readWrite()),
		// another space, which the listing filter keeps out
		spaceShare("7", otherSpaceID, "2001", "eve", false, readOnly()),
	}}

	gw := &fakeGateway{
		resources: map[string]bool{
			testStorageID + "/1001": true,
			testStorageID + "/1002": true,
			testStorageID + "/1003": true,
			testStorageID + "/1004": true,
		},
		paths: map[string]string{
			testStorageID + "/1001": testSpaceRoot + "/shared",
			testStorageID + "/1002": testSpaceRoot + "/shared/inner",
			testStorageID + "/1003": testSpaceRoot + "/wrongperms",
			testStorageID + "/1004": testSpaceRoot + "/team",
		},
	}

	dumper := &recordingDumper{NSDumpClient: fileDumper(t)}
	j := &DeepJob{shareMgr: store, gw: gw}

	got, err := j.runAnalysis(context.Background(), spaceID, dumper)
	if err != nil {
		t.Fatalf("runAnalysis: %v", err)
	}

	if dumper.rootPath != testSpaceRoot {
		t.Errorf("the namespace was read at %q, want %q", dumper.rootPath, testSpaceRoot)
	}

	want := ChangeSet{
		// the file below the share holds no entry
		{Path: nsTodo, Action: ActionAdd, ACL: aliceEntry()},
		// no share backs this entry, and nothing allows it either
		{Path: nsExternal, Action: ActionDelete, ACL: externalEntry()},
		// the deeper share is not on the folder it was made on
		{Path: nsInner, Action: ActionAdd, ACL: bobEntry()},
		// the name starts with the name of the shared folder, but it sits
		// next to it
		{Path: nsSharedDoc, Action: ActionDelete, ACL: aliceEntry()},
		// the right user with the wrong permissions
		{Path: nsWrongPerms, Action: ActionAdd, ACL: carolEntry()},
		{Path: nsWrongPerms, Action: ActionDelete, ACL: carolWrong()},
		// two shares on one folder: the group entry is there, the second one
		// is missing on the folder and on the file below it
		{Path: nsTeam, Action: ActionAdd, ACL: frankEntry()},
		{Path: nsTeamReport, Action: ActionAdd, ACL: frankEntry()},
		{Path: nsTeamReport, Action: ActionDelete, ACL: malloryEntry()},
		// the share of this subtree points at a resource that is gone, so the
		// entry stands on nothing
		{Path: nsPrivate, Action: ActionDelete, ACL: daveEntry()},
	}

	checkChanges(t, got, want)
}

func TestRunAnalysisReportsAFailedListing(t *testing.T) {
	store := &fakeStore{listErr: errors.New("the share database is down")}
	j := &DeepJob{shareMgr: store, gw: &fakeGateway{}}

	_, err := j.runAnalysis(context.Background(), spaces.EncodeSpaceID(testSpaceRoot), fileDumper(t))
	if err == nil {
		t.Fatal("runAnalysis: no error, want the listing error")
	}
}

func TestRunAnalysisReportsAFailedDump(t *testing.T) {
	j := &DeepJob{shareMgr: &fakeStore{}, gw: &fakeGateway{}}

	dumper := &failingDumper{err: errors.New("eos-ns-inspect failed")}
	_, err := j.runAnalysis(context.Background(), spaces.EncodeSpaceID(testSpaceRoot), dumper)
	if err == nil {
		t.Fatal("runAnalysis: no error, want the dump error")
	}
}

func TestRunAnalysisRejectsABadSpaceID(t *testing.T) {
	j := &DeepJob{shareMgr: &fakeStore{}, gw: &fakeGateway{}}

	_, err := j.runAnalysis(context.Background(), "this is not a space id", fileDumper(t))
	if err == nil {
		t.Fatal("runAnalysis: no error, want the decoding error")
	}
}

// A share becomes one ACL entry: the grantee is the qualifier, and the
// permissions of the share decide the EOS permissions.
func TestShareToACL(t *testing.T) {
	spaceID := spaces.EncodeSpaceID(testSpaceRoot)

	tests := []struct {
		name  string
		share *collaboration.Share
		want  *acl.Entry
	}{
		{
			name:  "a user with read permissions",
			share: spaceShare("1", spaceID, "1001", "alice", false, readOnly()).share,
			want:  aclEntry(acl.TypeUser, "alice", "rx"),
		},
		{
			name:  "a user with write permissions",
			share: spaceShare("2", spaceID, "1001", "bob", false, readWrite()).share,
			want:  aclEntry(acl.TypeUser, "bob", "rwx"),
		},
		{
			name:  "a group with write permissions",
			share: spaceShare("3", spaceID, "1001", "cernbox-admins", true, readWrite()).share,
			want:  aclEntry(acl.TypeGroup, "cernbox-admins", "rwx"),
		},
		{
			name:  "a share that grants nothing denies",
			share: spaceShare("4", spaceID, "1001", "dave", false, &provider.ResourcePermissions{}).share,
			want:  aclEntry(acl.TypeUser, "dave", "!r!w!x"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shareToACL(tt.share); *got != *tt.want {
				t.Errorf("shareToACL() = %s:%s:%s, want %s:%s:%s",
					got.Type, got.Qualifier, got.Permissions,
					tt.want.Type, tt.want.Qualifier, tt.want.Permissions)
			}
		})
	}
}

// The size of the generated space the benchmarks run on: a namespace of a
// hundred thousand entries, with a thousand shares in it.
const (
	benchEntries = 100000
	benchShares  = 1000
)

// memoryDumper hands out a namespace that is already in memory, so a benchmark
// measures the run and not the reading of a file.
type memoryDumper struct {
	dump *nsdump.NamespaceDump
}

func (d *memoryDumper) Setup(config any) error { return nil }

func (d *memoryDumper) Dump(rootPath string, maxDepth int) (*nsdump.NamespaceDump, error) {
	return d.dump, nil
}

// benchFixture is a generated space: its namespace, the shares on it, and what
// the gateway answers for the resources those shares point at.
type benchFixture struct {
	dump      *nsdump.NamespaceDump
	shares    []storedShare
	resources map[string]bool
	paths     map[string]string
}

// buildBenchFixture generates a space of `entries` namespace entries over 1110
// folders, three levels deep, with a share on each of the first `shares`
// folders. Most entries hold the ACL they should: a namespace that is mostly in
// order is the normal case for a run. A folder in every 25 carries the wrong
// permissions, a file in every 100 carries an entry too many, and a file in
// every 100 carries none at all.
func buildBenchFixture(entries, shares int) *benchFixture {
	spaceID := spaces.EncodeSpaceID(testSpaceRoot)
	f := &benchFixture{
		dump:      &nsdump.NamespaceDump{},
		resources: make(map[string]bool, shares),
		paths:     make(map[string]string, shares),
	}

	// the folders, parents before children
	var folders []string
	for i := range 10 {
		l1 := fmt.Sprintf("%s/g%d", testSpaceRoot, i)
		folders = append(folders, l1)
		for j := range 10 {
			l2 := fmt.Sprintf("%s/s%d", l1, j)
			folders = append(folders, l2)
			for k := range 10 {
				folders = append(folders, fmt.Sprintf("%s/t%d", l2, k))
			}
		}
	}

	// the entries every folder should hold: the ones of its shared ancestors,
	// plus its own share
	inherited := make(map[string][]string, len(folders))
	for i, folder := range folders {
		own := slices.Clone(inherited[path.Dir(folder)])

		if i < shares {
			inode := strconv.Itoa(100000 + i)
			f.resources[testStorageID+"/"+inode] = true
			f.paths[testStorageID+"/"+inode] = folder

			var e string
			switch {
			case i%5 == 0:
				f.shares = append(f.shares, spaceShare(strconv.Itoa(i), spaceID, inode, fmt.Sprintf("group%d", i), true, readWrite()))
				e = fmt.Sprintf("egroup:group%d:rwx", i)
			case i%2 == 0:
				f.shares = append(f.shares, spaceShare(strconv.Itoa(i), spaceID, inode, fmt.Sprintf("user%d", i), false, readWrite()))
				e = fmt.Sprintf("u:user%d:rwx", i)
			default:
				f.shares = append(f.shares, spaceShare(strconv.Itoa(i), spaceID, inode, fmt.Sprintf("user%d", i), false, readOnly()))
				e = fmt.Sprintf("u:user%d:rx", i)
			}
			own = append(own, e)
		}
		inherited[folder] = own

		// a folder in every 25 holds the wrong permissions
		actual := slices.Clone(own)
		if i%25 == 24 && len(actual) > 0 {
			last := actual[len(actual)-1]
			if strings.HasSuffix(last, ":rwx") {
				last = strings.TrimSuffix(last, ":rwx") + ":rx"
			} else {
				last = strings.TrimSuffix(last, ":rx") + ":rwx"
			}
			actual[len(actual)-1] = last
		}

		f.dump.Entries = append(f.dump.Entries, nsdump.NameSpaceEntry{
			CID:         strconv.Itoa(100000 + i),
			Name:        path.Base(folder),
			Path:        folder + "/",
			XattrSysAcl: strings.Join(actual, acl.ShortTextForm),
		})
	}

	// the files, spread over the folders
	for i := range entries - len(folders) {
		folder := folders[i%len(folders)]
		actual := slices.Clone(inherited[folder])

		switch {
		case i%100 == 0:
			actual = append(actual, "u:intruder:rwx")
		case i%100 == 50:
			actual = nil
		}

		f.dump.Entries = append(f.dump.Entries, nsdump.NameSpaceEntry{
			FID:         strconv.Itoa(900000 + i),
			Name:        fmt.Sprintf("file%d.txt", i),
			Path:        fmt.Sprintf("%s/file%d.txt", folder, i),
			XattrSysAcl: strings.Join(actual, acl.ShortTextForm),
		})
	}

	return f
}

// benchTree builds the ideal state out of the shares of a fixture, the way
// runAnalysis does.
func benchTree(f *benchFixture) *ACLTree {
	paths := make([]*ShareWithPath, 0, len(f.shares))
	for _, s := range f.shares {
		rid := s.share.GetResourceId()
		paths = append(paths, &ShareWithPath{
			Share: s.share,
			Path:  f.paths[rid.GetStorageId()+"/"+rid.GetOpaqueId()],
		})
	}
	slices.SortFunc(paths, func(a, b *ShareWithPath) int { return cmp.Compare(a.Path, b.Path) })

	tree := NewACLTree()
	for _, s := range paths {
		tree.Insert(s.toTreeNode())
	}
	return tree
}

// The generated namespace must be mostly in order, the way a real space is.
// A generator that lines up no entry at all would make every entry a change,
// and the benchmarks would measure the wrong work.
func TestBenchFixtureIsMostlyInOrder(t *testing.T) {
	f := buildBenchFixture(benchEntries, benchShares)

	if got := len(f.dump.Entries); got != benchEntries {
		t.Errorf("the namespace holds %d entries, want %d", got, benchEntries)
	}
	if got := len(f.shares); got != benchShares {
		t.Errorf("the space holds %d shares, want %d", got, benchShares)
	}

	changes := compare(benchTree(f), f.dump)
	if len(changes) == 0 {
		t.Fatal("the namespace is in order everywhere, so the run has no work at all")
	}
	if share := float64(len(changes)) / float64(len(f.dump.Entries)); share > 0.1 {
		t.Errorf("%d of %d entries change (%.0f%%), want under 10%%",
			len(changes), len(f.dump.Entries), share*100)
	} else {
		t.Logf("%d of %d entries change (%.1f%%)", len(changes), len(f.dump.Entries), share*100)
	}
}

// The comparison on its own: one lookup and one ACL diff per namespace entry.
func BenchmarkCompare(b *testing.B) {
	f := buildBenchFixture(benchEntries, benchShares)
	tree := benchTree(f)

	b.Run(fmt.Sprintf("%d entries/%d shares", len(f.dump.Entries), len(f.shares)), func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			compare(tree, f.dump)
		}
	})
}

// A whole run: the listing, the path of every share, the tree, and the
// comparison over the namespace.
func BenchmarkRunAnalysis(b *testing.B) {
	f := buildBenchFixture(benchEntries, benchShares)

	j := &DeepJob{
		shareMgr: &fakeStore{shares: f.shares},
		gw:       &fakeGateway{resources: f.resources, paths: f.paths},
	}
	dumper := &memoryDumper{dump: f.dump}
	spaceID := spaces.EncodeSpaceID(testSpaceRoot)
	ctx := context.Background()

	b.Run(fmt.Sprintf("%d entries/%d shares", len(f.dump.Entries), len(f.shares)), func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := j.runAnalysis(ctx, spaceID, dumper); err != nil {
				b.Fatalf("runAnalysis: %v", err)
			}
		}
	})
}
