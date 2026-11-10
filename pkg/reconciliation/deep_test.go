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
	"iter"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"google.golang.org/grpc"
)

// testSpaceRoot is the space the synthetic namespace dump covers.
const testSpaceRoot = "/eos/project/c/cernbox"

// The nodes of testDump. Folder paths carry a trailing separator, the way
// eos-ns-inspect prints them.
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

const (
	userType  = provider.GranteeType_GRANTEE_TYPE_USER
	groupType = provider.GranteeType_GRANTEE_TYPE_GROUP
)

// testUIDs are the uid numbers the fake gateway gives the test users. The
// storage keeps users by uid, so the expected entries hold these.
var testUIDs = map[string]int64{
	"alice":        10001,
	"bob":          10002,
	"carol":        10003,
	"dave":         10004,
	"frank":        10005,
	"mallory":      10006,
	"cboxexternal": 10007,
}

func aclEntry(t provider.GranteeType, qualifier string, permissions sharehierarchy.PermLevel) *ACLEntry {
	return &ACLEntry{Type: t, Qualifier: qualifier, Permissions: permissions}
}

func userEntry(name string, permissions sharehierarchy.PermLevel) *ACLEntry {
	return aclEntry(userType, strconv.FormatInt(testUIDs[name], 10), permissions)
}

func aliceEntry() *ACLEntry    { return userEntry("alice", sharehierarchy.PermRead) }
func bobEntry() *ACLEntry      { return userEntry("bob", sharehierarchy.PermRW) }
func carolEntry() *ACLEntry    { return userEntry("carol", sharehierarchy.PermRW) }
func carolWrong() *ACLEntry    { return userEntry("carol", sharehierarchy.PermRead) }
func adminsEntry() *ACLEntry   { return aclEntry(groupType, "cernbox-admins", sharehierarchy.PermRW) }
func malloryEntry() *ACLEntry  { return userEntry("mallory", sharehierarchy.PermRW) }
func daveEntry() *ACLEntry     { return userEntry("dave", sharehierarchy.PermRW) }
func frankEntry() *ACLEntry    { return userEntry("frank", sharehierarchy.PermRead) }
func externalEntry() *ACLEntry { return userEntry("cboxexternal", sharehierarchy.PermRW) }

func folderNode(p string, acls ...*ACLEntry) NSDumpInfo {
	return NSDumpInfo{Path: p, Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER, ACLs: acls}
}

func fileNode(p string, acls ...*ACLEntry) NSDumpInfo {
	return NSDumpInfo{Path: p, Type: provider.ResourceType_RESOURCE_TYPE_FILE, ACLs: acls}
}

// testDump is the synthetic namespace: one node for each case the comparison
// has to handle.
func testDump() []NSDumpInfo {
	return []NSDumpInfo{
		folderNode(nsRoot),
		folderNode(nsShared, aliceEntry()),
		fileNode(nsNotes, aliceEntry()),
		fileNode(nsTodo),
		fileNode(nsExternal, aliceEntry(), externalEntry()),
		folderNode(nsInner, aliceEntry()),
		fileNode(nsPlan, aliceEntry(), bobEntry()),
		fileNode(nsSharedDoc, aliceEntry()),
		folderNode(nsWrongPerms, carolWrong()),
		folderNode(nsTeam, adminsEntry()),
		fileNode(nsTeamReport, adminsEntry(), malloryEntry()),
		folderNode(nsPrivate, daveEntry()),
		fileNode(nsPrivateFile),
	}
}

// testTree is the ideal state of the synthetic namespace: four shares, plus one
// entry that is allowed anywhere in the space without a share behind it.
func testTree() *ACLTree {
	tree := NewACLTree()
	tree.Insert(&ACLNode{
		Path:        testSpaceRoot,
		AllowedACLs: []*ACLEntry{externalEntry()},
	})
	tree.Insert(&ACLNode{
		Path:          testSpaceRoot + "/shared",
		MandatoryACLs: []*ACLEntry{aliceEntry()},
	})
	tree.Insert(&ACLNode{
		Path:          testSpaceRoot + "/shared/inner",
		MandatoryACLs: []*ACLEntry{bobEntry()},
	})
	tree.Insert(&ACLNode{
		Path:          testSpaceRoot + "/wrongperms",
		MandatoryACLs: []*ACLEntry{carolEntry()},
	})
	tree.Insert(&ACLNode{
		Path:          testSpaceRoot + "/team",
		MandatoryACLs: []*ACLEntry{adminsEntry()},
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

// The whole comparison, over a namespace that holds one of every case.
func TestCompareAgainstNamespace(t *testing.T) {
	want := ChangeSet{
		// the file below the share carries no entry at all
		{Path: nsTodo, Action: ActionAdd, ACL: aliceEntry()},
		// the deeper share is not on the folder it was made on
		{Path: nsInner, Action: ActionAdd, ACL: bobEntry()},
		// a file whose name starts with the name of the shared folder is not
		// in the share, so its entry has nothing behind it
		{Path: nsSharedDoc, Action: ActionDelete, ACL: aliceEntry()},
		// the right user with the wrong permissions: one update replaces the
		// entry. A removal of the old entry would remove the update too.
		{Path: nsWrongPerms, Action: ActionUpdate, ACL: carolEntry()},
		// one entry too many below the group share
		{Path: nsTeamReport, Action: ActionDelete, ACL: malloryEntry()},
		// a subtree with no share at all
		{Path: nsPrivate, Action: ActionDelete, ACL: daveEntry()},
	}

	got, err := compare(testTree(), testStorage().DumpNamespace(context.Background(), testSpaceRoot))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	checkChanges(t, got, want)
}

// A dump that fails part way gives no change set, because the entries after
// the failure are not compared.
func TestCompareRejectsABrokenDump(t *testing.T) {
	entries := testDump()
	d := &failingDumper{before: entries[:len(entries)/2], err: errors.New("the dump stops here")}

	got, err := compare(testTree(), d.DumpNamespace(context.Background(), testSpaceRoot))
	if err == nil {
		t.Fatal("compare: no error, want the dump error")
	}
	if got != nil {
		t.Errorf("compare gave %d changes, want none", len(got))
	}
}

// The cases of one entry, without the namespace around them.
func TestCalculateChangeSet(t *testing.T) {
	const p = "/eos/project/c/cernbox/shared"

	tests := []struct {
		name                        string
		mandatory, optional, actual []*ACLEntry
		want                        ChangeSet
	}{
		{
			name: "nothing wanted and nothing there",
		},
		{
			name:      "the wanted entry is there",
			mandatory: []*ACLEntry{aliceEntry()},
			actual:    []*ACLEntry{aliceEntry()},
		},
		{
			name:      "the wanted entry is missing",
			mandatory: []*ACLEntry{aliceEntry()},
			want:      ChangeSet{{Path: p, Action: ActionAdd, ACL: aliceEntry()}},
		},
		{
			name:      "one of two wanted entries is missing",
			mandatory: []*ACLEntry{aliceEntry(), bobEntry()},
			actual:    []*ACLEntry{aliceEntry()},
			want:      ChangeSet{{Path: p, Action: ActionAdd, ACL: bobEntry()}},
		},
		{
			name:   "an entry nothing asks for",
			actual: []*ACLEntry{daveEntry()},
			want:   ChangeSet{{Path: p, Action: ActionDelete, ACL: daveEntry()}},
		},
		{
			name:     "an allowed entry stays",
			optional: []*ACLEntry{externalEntry()},
			actual:   []*ACLEntry{externalEntry()},
		},
		{
			name:      "an allowed entry next to a wanted one",
			mandatory: []*ACLEntry{aliceEntry()},
			optional:  []*ACLEntry{externalEntry()},
			actual:    []*ACLEntry{aliceEntry(), externalEntry()},
		},
		{
			name:      "the right user with the wrong permissions",
			mandatory: []*ACLEntry{carolEntry()},
			actual:    []*ACLEntry{carolWrong()},
			want:      ChangeSet{{Path: p, Action: ActionUpdate, ACL: carolEntry()}},
		},
		{
			name:      "an update next to an entry nothing asks for",
			mandatory: []*ACLEntry{carolEntry()},
			actual:    []*ACLEntry{carolWrong(), daveEntry()},
			want: ChangeSet{
				{Path: p, Action: ActionUpdate, ACL: carolEntry()},
				{Path: p, Action: ActionDelete, ACL: daveEntry()},
			},
		},
		{
			name:      "a group entry is another entry than a user entry",
			mandatory: []*ACLEntry{adminsEntry()},
			actual:    []*ACLEntry{aclEntry(userType, "cernbox-admins", sharehierarchy.PermRW)},
			want: ChangeSet{
				{Path: p, Action: ActionAdd, ACL: adminsEntry()},
				{Path: p, Action: ActionDelete, ACL: aclEntry(userType, "cernbox-admins", sharehierarchy.PermRW)},
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

// A grantee with the wrong permissions must keep its access once the change
// set is on the storage. The storage replaces the entry of a grantee on an
// add, and removes it by grantee, whatever its permissions.
func TestApplyingAWrongPermissionKeepsTheGrantee(t *testing.T) {
	storage := &memoryStorage{dump: []NSDumpInfo{folderNode(nsWrongPerms, carolWrong(), daveEntry())}}
	tree := NewACLTree()
	tree.Insert(&ACLNode{Path: nsWrongPerms, MandatoryACLs: []*ACLEntry{carolEntry()}})
	ctx := context.Background()

	cs, err := compare(tree, storage.DumpNamespace(ctx, testSpaceRoot))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	j := &DeepJob{}
	if err := j.applyChangeSet(ctx, storage, cs); err != nil {
		t.Fatalf("applyChangeSet: %v", err)
	}

	if got, want := storage.dump[0].ACLs, []*ACLEntry{carolEntry()}; !sameEntries(got, want) {
		t.Errorf("the node holds %v, want %v", formatEntries(got), formatEntries(want))
	}
}

// A storage can keep a copy of the ACL of a file on another node, as EOS does
// on the version folder. The copy must carry the entries of the file, so a
// missing one is added and a present one stays.
func TestCompareVersionFolder(t *testing.T) {
	const versions = testSpaceRoot + "/shared/.sys.v#.notes.md/"

	tests := []struct {
		name string
		acls []*ACLEntry
		want ChangeSet
	}{
		{
			name: "the copy misses the share of its file",
			want: ChangeSet{{Path: versions, Action: ActionAdd, ACL: aliceEntry()}},
		},
		{
			name: "the copy carries the share of its file",
			acls: []*ACLEntry{aliceEntry()},
		},
		{
			name: "the copy carries an entry its file does not need",
			acls: []*ACLEntry{aliceEntry(), daveEntry()},
			want: ChangeSet{{Path: versions, Action: ActionDelete, ACL: daveEntry()}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := folderNode(versions, tt.acls...)
			node.ACLsOf = nsNotes
			storage := &memoryStorage{dump: []NSDumpInfo{node}}

			got, err := compare(testTree(), storage.DumpNamespace(context.Background(), testSpaceRoot))
			if err != nil {
				t.Fatalf("compare: %v", err)
			}
			checkChanges(t, got, tt.want)
		})
	}
}

func TestParseAllowedACL(t *testing.T) {
	tests := []struct {
		in      string
		want    *ACLEntry
		wantErr bool
	}{
		{in: "user:cboxexternal:RW", want: aclEntry(userType, "cboxexternal", sharehierarchy.PermRW)},
		{in: "group:cernbox-admins:R", want: aclEntry(groupType, "cernbox-admins", sharehierarchy.PermRead)},
		{in: "user:dave:D", want: aclEntry(userType, "dave", sharehierarchy.PermDeny)},
		{in: "u:cboxexternal:RW", wantErr: true},
		{in: "user:cboxexternal:rwx", wantErr: true},
		{in: "user::RW", wantErr: true},
		{in: "user:cboxexternal", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseAllowedACL(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseAllowedACL(%q) error = %v, want an error: %v", tt.in, err, tt.wantErr)
			}
			if tt.want != nil && *got != *tt.want {
				t.Errorf("ParseAllowedACL(%q) = %+v, want %+v", tt.in, *got, *tt.want)
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

// readOnly and readWrite are what PermLevelFromCS3 reads to decide between
// read and read-write.
func readOnly() *provider.ResourcePermissions {
	return &provider.ResourcePermissions{InitiateFileDownload: true}
}

func readWrite() *provider.ResourcePermissions {
	return &provider.ResourcePermissions{InitiateFileDownload: true, InitiateFileUpload: true}
}

// recordingDumper keeps the root path a run asked for, which is the space path
// the job decoded out of the space id.
type recordingDumper struct {
	ReconcileMethods
	rootPath string
}

func (d *recordingDumper) DumpNamespace(ctx context.Context, rootPath string) iter.Seq2[NSDumpInfo, error] {
	d.rootPath = rootPath
	return d.ReconcileMethods.DumpNamespace(ctx, rootPath)
}

// failingDumper stands for a namespace that cannot be read, or that fails after
// the entries in before. A run that fails here never writes, so the writes are
// left unimplemented.
type failingDumper struct {
	ReconcileMethods
	before []NSDumpInfo
	err    error
}

func (d *failingDumper) DumpNamespace(ctx context.Context, rootPath string) iter.Seq2[NSDumpInfo, error] {
	return func(yield func(NSDumpInfo, error) bool) {
		for _, e := range d.before {
			if !yield(e, nil) {
				return
			}
		}
		yield(NSDumpInfo{}, d.err)
	}
}

// testStorage holds the synthetic namespace.
func testStorage() ReconcileMethods {
	return &memoryStorage{dump: testDump()}
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
		uids: testUIDs,
	}

	dumper := &recordingDumper{ReconcileMethods: testStorage()}
	j := &DeepJob{ShareStore: store}

	got, _, err := j.runAnalysis(context.Background(), gw, uidCache{}, RunParameters{SpaceID: spaceID}, dumper)
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
		{Path: nsWrongPerms, Action: ActionUpdate, ACL: carolEntry()},
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
	j := &DeepJob{ShareStore: store}

	_, _, err := j.runAnalysis(context.Background(), &fakeGateway{}, uidCache{}, RunParameters{SpaceID: spaces.EncodeSpaceID(testSpaceRoot)}, testStorage())
	if err == nil {
		t.Fatal("runAnalysis: no error, want the listing error")
	}
}

// A share whose user cannot be resolved to a uid has no entry to compare with.
// Comparing without it would delete the entry the user really has, so the
// space fails instead.
func TestRunAnalysisFailsOnAnUnresolvableUser(t *testing.T) {
	spaceID := spaces.EncodeSpaceID(testSpaceRoot)
	store := &fakeStore{shares: []storedShare{
		spaceShare("1", spaceID, "1001", "nobody", false, readOnly()),
	}}
	gw := &fakeGateway{
		resources: map[string]bool{testStorageID + "/1001": true},
		paths:     map[string]string{testStorageID + "/1001": testSpaceRoot + "/shared"},
		uids:      testUIDs,
	}
	j := &DeepJob{ShareStore: store}

	if _, _, err := j.runAnalysis(context.Background(), gw, uidCache{}, RunParameters{SpaceID: spaceID}, testStorage()); err == nil {
		t.Fatal("runAnalysis: no error, want the error of the user lookup")
	}
}

func TestRunAnalysisReportsAFailedDump(t *testing.T) {
	j := &DeepJob{ShareStore: &fakeStore{}}

	dumper := &failingDumper{err: errors.New("eos-ns-inspect failed")}
	_, _, err := j.runAnalysis(context.Background(), &fakeGateway{}, uidCache{}, RunParameters{SpaceID: spaces.EncodeSpaceID(testSpaceRoot)}, dumper)
	if err == nil {
		t.Fatal("runAnalysis: no error, want the dump error")
	}
}

func TestRunAnalysisRejectsABadSpaceID(t *testing.T) {
	j := &DeepJob{ShareStore: &fakeStore{}}

	_, _, err := j.runAnalysis(context.Background(), &fakeGateway{}, uidCache{}, RunParameters{SpaceID: "this is not a space id"}, testStorage())
	if err == nil {
		t.Fatal("runAnalysis: no error, want the decoding error")
	}
}

// A share becomes one ACL entry: a user by uid, a group by name, and the
// permissions of the share decide the level.
func TestShareToACL(t *testing.T) {
	spaceID := spaces.EncodeSpaceID(testSpaceRoot)
	lightweight := spaceShare("5", spaceID, "1001", "guest@example.org", false, readOnly()).share
	lightweight.Grantee.GetUserId().Type = userpb.UserType_USER_TYPE_LIGHTWEIGHT

	tests := []struct {
		name  string
		share *collaboration.Share
		want  *ACLEntry
	}{
		{
			name:  "a user with read permissions",
			share: spaceShare("1", spaceID, "1001", "alice", false, readOnly()).share,
			want:  aliceEntry(),
		},
		{
			name:  "a user with write permissions",
			share: spaceShare("2", spaceID, "1001", "bob", false, readWrite()).share,
			want:  bobEntry(),
		},
		{
			name:  "a group with write permissions",
			share: spaceShare("3", spaceID, "1001", "cernbox-admins", true, readWrite()).share,
			want:  adminsEntry(),
		},
		{
			name:  "a share that grants nothing denies",
			share: spaceShare("4", spaceID, "1001", "dave", false, &provider.ResourcePermissions{}).share,
			want:  userEntry("dave", sharehierarchy.PermDeny),
		},
		{
			// the access of a lightweight account is not in the ACL
			name:  "a lightweight account has no entry",
			share: lightweight,
		},
	}

	gw := &fakeGateway{uids: testUIDs}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shareToACL(context.Background(), gw, uidCache{}, tt.share)
			if err != nil {
				t.Fatalf("shareToACL: %v", err)
			}
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("shareToACL() = %+v, want no entry", *got)
			case tt.want != nil && (got == nil || *got != *tt.want):
				t.Errorf("shareToACL() = %v, want %+v", got, *tt.want)
			}
		})
	}
}

// A run asks for the uid of a user once, however many shares the user has.
func TestUIDCacheAsksOnce(t *testing.T) {
	gw := &fakeGateway{uids: testUIDs}
	uids := uidCache{}
	for range 3 {
		uid, err := uids.uid(context.Background(), gw, &userpb.UserId{OpaqueId: "alice"})
		if err != nil {
			t.Fatalf("uid: %v", err)
		}
		if want := strconv.FormatInt(testUIDs["alice"], 10); uid != want {
			t.Errorf("uid = %q, want %q", uid, want)
		}
	}
	if gw.uidLookups != 1 {
		t.Errorf("the gateway was asked %d times, want 1", gw.uidLookups)
	}
}

// GetUser answers from uids, and NOT_FOUND for a user that is not in it.
func (f *fakeGateway) GetUser(ctx context.Context, in *userpb.GetUserRequest, _ ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	f.uidLookups++
	uid, ok := f.uids[in.GetUserId().GetOpaqueId()]
	if !ok {
		return &userpb.GetUserResponse{Status: status(false)}, nil
	}
	return &userpb.GetUserResponse{Status: status(true), User: &userpb.User{Id: in.GetUserId(), UidNumber: uid}}, nil
}

// The size of the generated space the benchmarks run on: a namespace of a
// hundred thousand entries, with a thousand shares in it.
const (
	benchEntries = 100000
	benchShares  = 1000
)

// memoryStorage hands out a namespace that is already in memory, so a benchmark
// measures the run and not the reading of a file. Its writes follow the rules
// of EOS: an add replaces the entry of the grantee, and a removal is by
// grantee, whatever the permissions.
type memoryStorage struct {
	dump []NSDumpInfo
}

func (d *memoryStorage) DumpNamespace(ctx context.Context, rootPath string) iter.Seq2[NSDumpInfo, error] {
	return func(yield func(NSDumpInfo, error) bool) {
		for _, e := range d.dump {
			if !yield(e, nil) {
				return
			}
		}
	}
}

func (d *memoryStorage) AddNodeACL(ctx context.Context, p string, e ACLEntry) error {
	if err := d.RemoveNodeACL(ctx, p, e); err != nil {
		return err
	}
	i := slices.IndexFunc(d.dump, func(n NSDumpInfo) bool { return n.Path == p })
	d.dump[i].ACLs = append(d.dump[i].ACLs, &e)
	return nil
}

func (d *memoryStorage) RemoveNodeACL(ctx context.Context, p string, e ACLEntry) error {
	i := slices.IndexFunc(d.dump, func(n NSDumpInfo) bool { return n.Path == p })
	if i < 0 {
		return fmt.Errorf("no node at %s", p)
	}
	d.dump[i].ACLs = slices.DeleteFunc(slices.Clone(d.dump[i].ACLs), func(c *ACLEntry) bool {
		return c.Type == e.Type && c.Qualifier == e.Qualifier
	})
	return nil
}

// benchFixture is a generated space: its namespace, the shares on it, and what
// the gateway answers for the resources those shares point at.
type benchFixture struct {
	dump   []NSDumpInfo
	shares []storedShare
	// shareACLs holds the entry of each share, in the order of shares.
	shareACLs []*ACLEntry
	resources map[string]bool
	paths     map[string]string
	uids      map[string]int64
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
		resources: make(map[string]bool, shares),
		paths:     make(map[string]string, shares),
		uids:      make(map[string]int64, shares),
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
	inherited := make(map[string][]*ACLEntry, len(folders))
	for i, folder := range folders {
		own := slices.Clone(inherited[path.Dir(folder)])

		if i < shares {
			inode := strconv.Itoa(100000 + i)
			f.resources[testStorageID+"/"+inode] = true
			f.paths[testStorageID+"/"+inode] = folder

			user := fmt.Sprintf("user%d", i)
			uid := strconv.Itoa(20000 + i)
			f.uids[user] = int64(20000 + i)

			var e *ACLEntry
			switch {
			case i%5 == 0:
				f.shares = append(f.shares, spaceShare(strconv.Itoa(i), spaceID, inode, fmt.Sprintf("group%d", i), true, readWrite()))
				e = aclEntry(groupType, fmt.Sprintf("group%d", i), sharehierarchy.PermRW)
			case i%2 == 0:
				f.shares = append(f.shares, spaceShare(strconv.Itoa(i), spaceID, inode, user, false, readWrite()))
				e = aclEntry(userType, uid, sharehierarchy.PermRW)
			default:
				f.shares = append(f.shares, spaceShare(strconv.Itoa(i), spaceID, inode, user, false, readOnly()))
				e = aclEntry(userType, uid, sharehierarchy.PermRead)
			}
			f.shareACLs = append(f.shareACLs, e)
			own = append(own, e)
		}
		inherited[folder] = own

		// a folder in every 25 holds the wrong permissions
		actual := slices.Clone(own)
		if i%25 == 24 && len(actual) > 0 {
			// a copy: the entry is shared with the folders below
			wrong := *actual[len(actual)-1]
			if wrong.Permissions == sharehierarchy.PermRW {
				wrong.Permissions = sharehierarchy.PermRead
			} else {
				wrong.Permissions = sharehierarchy.PermRW
			}
			actual[len(actual)-1] = &wrong
		}

		node := folderNode(folder+"/", actual...)
		node.ID = strconv.Itoa(100000 + i)
		f.dump = append(f.dump, node)
	}

	// the files, spread over the folders
	for i := range entries - len(folders) {
		folder := folders[i%len(folders)]
		actual := slices.Clone(inherited[folder])

		switch {
		case i%100 == 0:
			actual = append(actual, aclEntry(userType, "99999", sharehierarchy.PermRW))
		case i%100 == 50:
			actual = nil
		}

		node := fileNode(fmt.Sprintf("%s/file%d.txt", folder, i), actual...)
		node.ID = strconv.Itoa(900000 + i)
		f.dump = append(f.dump, node)
	}

	return f
}

// benchTree builds the ideal state out of the shares of a fixture, the way
// runAnalysis does.
func benchTree(f *benchFixture) *ACLTree {
	paths := make([]*ShareWithPath, 0, len(f.shares))
	for i, s := range f.shares {
		rid := s.share.GetResourceId()
		paths = append(paths, &ShareWithPath{
			Share: s.share,
			Path:  f.paths[rid.GetStorageId()+"/"+rid.GetOpaqueId()],
			ACL:   f.shareACLs[i],
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

	if got := len(f.dump); got != benchEntries {
		t.Errorf("the namespace holds %d entries, want %d", got, benchEntries)
	}
	if got := len(f.shares); got != benchShares {
		t.Errorf("the space holds %d shares, want %d", got, benchShares)
	}

	changes, err := compare(benchTree(f), (&memoryStorage{dump: f.dump}).DumpNamespace(context.Background(), testSpaceRoot))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("the namespace is in order everywhere, so the run has no work at all")
	}
	if share := float64(len(changes)) / float64(len(f.dump)); share > 0.1 {
		t.Errorf("%d of %d entries change (%.0f%%), want under 10%%",
			len(changes), len(f.dump), share*100)
	} else {
		t.Logf("%d of %d entries change (%.1f%%)", len(changes), len(f.dump), share*100)
	}
}

// The comparison on its own: one lookup and one ACL diff per namespace entry.
func BenchmarkCompare(b *testing.B) {
	f := buildBenchFixture(benchEntries, benchShares)
	tree := benchTree(f)
	dumper := &memoryStorage{dump: f.dump}
	ctx := context.Background()

	b.Run(fmt.Sprintf("%d entries/%d shares", len(f.dump), len(f.shares)), func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := compare(tree, dumper.DumpNamespace(ctx, testSpaceRoot)); err != nil {
				b.Fatalf("compare: %v", err)
			}
		}
	})
}

// A whole run: the listing, the path of every share, the tree, and the
// comparison over the namespace.
func BenchmarkRunAnalysis(b *testing.B) {
	f := buildBenchFixture(benchEntries, benchShares)

	j := &DeepJob{ShareStore: &fakeStore{shares: f.shares}}
	gw := &fakeGateway{resources: f.resources, paths: f.paths, uids: f.uids}
	dumper := &memoryStorage{dump: f.dump}
	spaceID := spaces.EncodeSpaceID(testSpaceRoot)
	ctx := context.Background()

	b.Run(fmt.Sprintf("%d entries/%d shares", len(f.dump), len(f.shares)), func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			// a new cache per iteration, as a run starts with an empty one
			if _, _, err := j.runAnalysis(ctx, gw, uidCache{}, RunParameters{SpaceID: spaceID}, dumper); err != nil {
				b.Fatalf("runAnalysis: %v", err)
			}
		}
	})
}
