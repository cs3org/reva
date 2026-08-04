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
	"context"
	"errors"
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"google.golang.org/grpc"
)

// The OCS permission values the shallow job sees in the database.
const (
	ocsRead  = uint8(permissions.PermissionRead)
	ocsWrite = uint8(permissions.PermissionRead | permissions.PermissionWrite |
		permissions.PermissionCreate | permissions.PermissionDelete)
	ocsDeny = uint8(permissions.PermissionInvalid)
)

// grantWrite is one grant the job pushed to the storage.
type grantWrite struct {
	node   string
	action ActionKind
	grant  *provider.Grant
}

// fakeGrants is an in-memory GrantStore holding the grants of every node and
// recording what the job writes to them.
type fakeGrants struct {
	// grants maps "<storage>/<inode>" to the grants set on it.
	grants   map[string][]*provider.Grant
	writes   []grantWrite
	listErr  error
	writeErr error
}

func (f *fakeGrants) node(ref *provider.Reference) string {
	id := ref.GetResourceId()
	return id.GetStorageId() + "/" + id.GetOpaqueId()
}

func (f *fakeGrants) ListGrants(ctx context.Context, in *provider.ListGrantsRequest, _ ...grpc.CallOption) (*provider.ListGrantsResponse, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &provider.ListGrantsResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Grants: f.grants[f.node(in.GetRef())],
	}, nil
}

func (f *fakeGrants) AddGrant(ctx context.Context, in *provider.AddGrantRequest, _ ...grpc.CallOption) (*provider.AddGrantResponse, error) {
	if f.writeErr != nil {
		return nil, f.writeErr
	}
	f.record(ActionAdd, f.node(in.GetRef()), in.GetGrant())
	return &provider.AddGrantResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

func (f *fakeGrants) UpdateGrant(ctx context.Context, in *provider.UpdateGrantRequest, _ ...grpc.CallOption) (*provider.UpdateGrantResponse, error) {
	if f.writeErr != nil {
		return nil, f.writeErr
	}
	f.record(ActionUpdate, f.node(in.GetRef()), in.GetGrant())
	return &provider.UpdateGrantResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

func (f *fakeGrants) record(action ActionKind, node string, g *provider.Grant) {
	f.writes = append(f.writes, grantWrite{node: node, action: action, grant: g})
}

func grantsFrom(f *fakeGrants) func(context.Context, string) (GrantStore, error) {
	return func(context.Context, string) (GrantStore, error) { return f, nil }
}

// grant builds a grant as ListGrants would return it.
func grant(granteeType provider.GranteeType, id string, perms uint8) *provider.Grant {
	g := &provider.Grant{
		Grantee:     &provider.Grantee{Type: granteeType},
		Permissions: permissions.OcsPermissions(perms).AsCS3Permissions(),
	}
	if granteeType == provider.GranteeType_GRANTEE_TYPE_GROUP {
		g.Grantee.Id = &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: id}}
	} else {
		g.Grantee.Id = &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: id}}
	}
	return g
}

// shared builds a share with the fields the shallow job reads.
func shared(id uint, space, inode, shareWith string, isGroup bool, perms uint8) storedShare {
	s := share(id, "eosuser", inode, shareWith, isGroup, false)
	s.share.ResourceId.SpaceId = space
	s.share.Permissions = &collaboration.SharePermissions{
		Permissions: permissions.OcsPermissions(perms).AsCS3Permissions(),
	}
	return s
}

// shallowJob wires a job over the three fakes.
func shallowJob(store *fakeStore, gw *fakeGateway, grants *fakeGrants) *ShallowJob {
	return &ShallowJob{Shares: store, Gateway: gw, Grants: grantsFrom(grants)}
}

func TestShallowAddsMissingGrant(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(1, "space-a", "inode-1", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{"eosuser/inode-1": "/eos/user/j/jdoe/shared"},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 1 || len(report.Written) != 1 {
		t.Fatalf("report = %+v, want 1 checked / 1 written", report)
	}
	if got := report.Written[0]; got.Action != ActionAdd || got.Path != "/eos/user/j/jdoe/shared" || got.Expected != "R" {
		t.Errorf("written = %+v, want an add of R on the shared path", got)
	}
	if len(grants.writes) != 1 {
		t.Fatalf("writes = %+v, want 1", grants.writes)
	}
	w := grants.writes[0]
	if w.action != ActionAdd || w.node != "eosuser/inode-1" || w.grant.GetGrantee().GetUserId().GetOpaqueId() != "jdoe" {
		t.Errorf("write = %+v, want an add for jdoe on eosuser/inode-1", w)
	}
	if level := sharehierarchy.PermLevelFromCS3(w.grant.GetPermissions()); level != sharehierarchy.PermRead {
		t.Errorf("wrote %s, want R", level)
	}
}

func TestShallowCorrectsWrongPermissions(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(2, "space-a", "inode-2", "jdoe", false, ocsWrite),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{"eosuser/inode-2": "/eos/user/j/jdoe/shared"},
	}
	grants := &fakeGrants{grants: map[string][]*provider.Grant{
		"eosuser/inode-2": {grant(provider.GranteeType_GRANTEE_TYPE_USER, "jdoe", ocsRead)},
	}}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Written) != 1 {
		t.Fatalf("report = %+v, want 1 written", report)
	}
	if got := report.Written[0]; got.Action != ActionUpdate || got.Observed != "R" || got.Expected != "RW" {
		t.Errorf("written = %+v, want an update from R to RW", got)
	}
	if len(grants.writes) != 1 || grants.writes[0].action != ActionUpdate {
		t.Fatalf("writes = %+v, want 1 update", grants.writes)
	}
	if level := sharehierarchy.PermLevelFromCS3(grants.writes[0].grant.GetPermissions()); level != sharehierarchy.PermRW {
		t.Errorf("wrote %s, want RW", level)
	}
}

func TestShallowLeavesCorrectGrantAlone(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(3, "space-a", "inode-3", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{"eosuser/inode-3": "/eos/user/j/jdoe/shared"},
	}
	grants := &fakeGrants{grants: map[string][]*provider.Grant{
		"eosuser/inode-3": {grant(provider.GranteeType_GRANTEE_TYPE_USER, "jdoe", ocsRead)},
	}}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 1 || len(report.Written) != 0 {
		t.Fatalf("report = %+v, want 1 checked / nothing written", report)
	}
	if len(grants.writes) != 0 {
		t.Errorf("writes = %+v, want none", grants.writes)
	}
}

func TestShallowGroupRecipient(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(4, "space-p", "inode-4", "cernbox-admins", true, ocsWrite),
	}}
	gw := &fakeGateway{
		groups: map[string]bool{"cernbox-admins": true},
		paths:  map[string]string{"eosuser/inode-4": "/eos/project/c/cernbox/data"},
	}
	grants := &fakeGrants{}

	if _, err := shallowJob(store, gw, grants).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(grants.writes) != 1 {
		t.Fatalf("writes = %+v, want 1", grants.writes)
	}
	g := grants.writes[0].grant.GetGrantee()
	if g.GetType() != provider.GranteeType_GRANTEE_TYPE_GROUP || g.GetGroupId().GetOpaqueId() != "cernbox-admins" {
		t.Errorf("grantee = %+v, want the group cernbox-admins", g)
	}
}

// TestShallowLightweightRecipientCarriesUserType asserts that the grantee is
// written with the user type the gateway reports. The storage driver keys the
// lightweight xattr off that type, so a grantee built from the stored name
// alone would put an external account into the native ACLs.
func TestShallowLightweightRecipientCarriesUserType(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(5, "space-a", "inode-5", "guest@example.org", false, ocsRead),
	}}
	gw := &fakeGateway{
		users:     map[string]bool{"guest@example.org": true},
		userTypes: map[string]userpb.UserType{"guest@example.org": userpb.UserType_USER_TYPE_LIGHTWEIGHT},
		paths:     map[string]string{"eosuser/inode-5": "/eos/user/j/jdoe/shared"},
	}
	grants := &fakeGrants{}

	if _, err := shallowJob(store, gw, grants).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(grants.writes) != 1 {
		t.Fatalf("writes = %+v, want 1", grants.writes)
	}
	id := grants.writes[0].grant.GetGrantee().GetUserId()
	if id.GetType() != userpb.UserType_USER_TYPE_LIGHTWEIGHT {
		t.Errorf("grantee type = %s, want lightweight", id.GetType())
	}
}

// TestShallowDenyShareIsWritten asserts that a share with no permissions is
// written as an entry granting nothing, not treated as an absent share.
func TestShallowDenyShareIsWritten(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(6, "space-a", "inode-6", "jdoe", false, ocsDeny),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{"eosuser/inode-6": "/eos/user/j/jdoe/secret"},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Written) != 1 || report.Written[0].Expected != "D" {
		t.Fatalf("report = %+v, want one deny written", report)
	}
	if len(grants.writes) != 1 {
		t.Fatalf("writes = %+v, want 1", grants.writes)
	}
	if level := sharehierarchy.PermLevelFromCS3(grants.writes[0].grant.GetPermissions()); level != sharehierarchy.PermDeny {
		t.Errorf("wrote %s, want D", level)
	}
}

// TestShallowAncestorShareCoversChild asserts that no entry is written for a
// share whose parent share already grants the same access to the same
// recipient: the storage inherits it, and writing it again would litter the
// namespace with redundant entries.
func TestShallowAncestorShareCoversChild(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(10, "space-a", "inode-parent", "jdoe", false, ocsRead),
		shared(11, "space-a", "inode-child", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{
			"eosuser/inode-parent": "/eos/user/a/alice/dir",
			"eosuser/inode-child":  "/eos/user/a/alice/dir/sub",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 2 || report.Covered != 1 || len(report.Written) != 1 {
		t.Fatalf("report = %+v, want 2 checked / 1 covered / 1 written", report)
	}
	if len(grants.writes) != 1 || grants.writes[0].node != "eosuser/inode-parent" {
		t.Errorf("writes = %+v, want only the parent", grants.writes)
	}
}

// TestShallowEscalatingChildIsWritten asserts that a child share granting more
// than its parent keeps its own entry: dropping it would silently downgrade the
// recipient to what the parent inherits.
func TestShallowEscalatingChildIsWritten(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(12, "space-a", "inode-parent", "jdoe", false, ocsRead),
		shared(13, "space-a", "inode-child", "jdoe", false, ocsWrite),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{
			"eosuser/inode-parent": "/eos/user/a/alice/dir",
			"eosuser/inode-child":  "/eos/user/a/alice/dir/sub",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Covered != 0 || len(report.Written) != 2 {
		t.Fatalf("report = %+v, want nothing covered and 2 written", report)
	}
}

// TestShallowWeakerChildIsNotWritten asserts that a share granting less than a
// share on a path above it is left alone. Its entry would override the one
// inherited from the parent and take away access the parent share grants, so
// enforcing it would break the parent share.
func TestShallowWeakerChildIsNotWritten(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(18, "space-a", "inode-parent", "jdoe", false, ocsWrite),
		shared(19, "space-a", "inode-child", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{
			"eosuser/inode-parent": "/eos/user/a/alice/dir",
			"eosuser/inode-child":  "/eos/user/a/alice/dir/sub",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Conflicting != 1 || len(report.Written) != 1 {
		t.Fatalf("report = %+v, want 1 conflicting / 1 written", report)
	}
	if len(grants.writes) != 1 || grants.writes[0].node != "eosuser/inode-parent" {
		t.Errorf("writes = %+v, want only the parent", grants.writes)
	}
}

// TestShallowChildUnderDenyIsNotWritten asserts that a share below a denying
// share is left alone. Writing its entry would hand back the access the deny
// takes away.
func TestShallowChildUnderDenyIsNotWritten(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(40, "space-a", "inode-parent", "jdoe", false, ocsDeny),
		shared(41, "space-a", "inode-child", "jdoe", false, ocsWrite),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{
			"eosuser/inode-parent": "/eos/user/a/alice/dir",
			"eosuser/inode-child":  "/eos/user/a/alice/dir/sub",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Conflicting != 1 || len(report.Written) != 1 {
		t.Fatalf("report = %+v, want 1 conflicting / 1 written", report)
	}
	if len(grants.writes) != 1 || grants.writes[0].node != "eosuser/inode-parent" {
		t.Errorf("writes = %+v, want only the deny on the parent", grants.writes)
	}
}

// TestShallowSkippedAncestorDoesNotShadow asserts that a share which gets no
// entry of its own does not shadow the shares below it: what they inherit comes
// from the nearest share that does get one. Were the share above them taken at
// face value, the deepest one here would look like an escalation and be written
// for nothing.
func TestShallowSkippedAncestorDoesNotShadow(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(50, "space-a", "inode-top", "jdoe", false, ocsWrite),
		shared(51, "space-a", "inode-mid", "jdoe", false, ocsRead),
		shared(52, "space-a", "inode-deep", "jdoe", false, ocsWrite),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{
			"eosuser/inode-top":  "/eos/user/a/alice/dir",
			"eosuser/inode-mid":  "/eos/user/a/alice/dir/sub",
			"eosuser/inode-deep": "/eos/user/a/alice/dir/sub/deep",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// the middle share grants less than the one above it, the deepest grants
	// exactly what it already inherits from the top one.
	if report.Conflicting != 1 || report.Covered != 1 || len(report.Written) != 1 {
		t.Fatalf("report = %+v, want 1 conflicting / 1 covered / 1 written", report)
	}
	if len(grants.writes) != 1 || grants.writes[0].node != "eosuser/inode-top" {
		t.Errorf("writes = %+v, want only the top share", grants.writes)
	}
}

// TestShallowAncestryHoldsAcrossSubtrees asserts that shares in one subtree do
// not disturb the ancestry of another, whatever order the database returns them
// in.
func TestShallowAncestryHoldsAcrossSubtrees(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(60, "space-a", "inode-child", "jdoe", false, ocsRead),
		shared(61, "space-a", "inode-elsewhere", "jdoe", false, ocsRead),
		shared(62, "space-a", "inode-parent", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{
			"eosuser/inode-child":     "/eos/user/a/alice/one/sub",
			"eosuser/inode-elsewhere": "/eos/user/x/xavier/other",
			"eosuser/inode-parent":    "/eos/user/a/alice/one",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Covered != 1 || len(report.Written) != 2 {
		t.Fatalf("report = %+v, want 1 covered (the child) / 2 written", report)
	}
	written := map[string]bool{}
	for _, w := range grants.writes {
		written[w.node] = true
	}
	if !written["eosuser/inode-parent"] || !written["eosuser/inode-elsewhere"] {
		t.Errorf("writes = %+v, want the parent and the unrelated share", grants.writes)
	}
}

// TestShallowAncestorCoversOnlySameRecipient asserts that a share does not
// shadow another recipient's share below it. Their ACL entries are independent.
func TestShallowAncestorCoversOnlySameRecipient(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(14, "space-a", "inode-parent", "jdoe", false, ocsRead),
		shared(15, "space-a", "inode-child", "asmith", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true, "asmith": true},
		paths: map[string]string{
			"eosuser/inode-parent": "/eos/user/a/alice/dir",
			"eosuser/inode-child":  "/eos/user/a/alice/dir/sub",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Covered != 0 || len(report.Written) != 2 {
		t.Fatalf("report = %+v, want nothing covered and 2 written", report)
	}
}

// TestShallowAncestryDoesNotCrossSpaces asserts that a share only shadows a
// share in the same space. Spaces are disjoint, so a path that looks like a
// parent in another space is unrelated.
func TestShallowAncestryDoesNotCrossSpaces(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(16, "space-a", "inode-parent", "jdoe", false, ocsRead),
		shared(17, "space-b", "inode-child", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{
			"eosuser/inode-parent": "/eos/user/a/alice/dir",
			"eosuser/inode-child":  "/eos/user/a/alice/dir/sub",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Covered != 0 || len(report.Written) != 2 {
		t.Fatalf("report = %+v, want nothing covered and 2 written", report)
	}
}

// TestShallowDryRunMatchesLiveRun asserts that dry_run reports exactly the
// grants a live run writes, so a dry run can be trusted as a preview.
func TestShallowDryRunMatchesLiveRun(t *testing.T) {
	shares := []storedShare{
		shared(20, "space-a", "inode-20", "jdoe", false, ocsRead),    // correct already
		shared(21, "space-a", "inode-21", "jdoe", false, ocsRead),    // entry missing
		shared(22, "space-a", "inode-22", "asmith", false, ocsWrite), // entry too weak
	}
	newGateway := func() *fakeGateway {
		return &fakeGateway{
			users: map[string]bool{"jdoe": true, "asmith": true},
			paths: map[string]string{
				"eosuser/inode-20": "/eos/user/a/alice/one",
				"eosuser/inode-21": "/eos/user/a/alice/two",
				"eosuser/inode-22": "/eos/user/a/alice/three",
			},
		}
	}
	newGrants := func() *fakeGrants {
		return &fakeGrants{grants: map[string][]*provider.Grant{
			"eosuser/inode-20": {grant(provider.GranteeType_GRANTEE_TYPE_USER, "jdoe", ocsRead)},
			"eosuser/inode-22": {grant(provider.GranteeType_GRANTEE_TYPE_USER, "asmith", ocsRead)},
		}}
	}

	dryGrants := newGrants()
	dryJob := shallowJob(&fakeStore{shares: shares}, newGateway(), dryGrants)
	dryJob.DryRun = true
	dry, err := dryJob.Run(context.Background())
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	liveGrants := newGrants()
	live, err := shallowJob(&fakeStore{shares: shares}, newGateway(), liveGrants).Run(context.Background())
	if err != nil {
		t.Fatalf("live run: %v", err)
	}

	if len(dry.Written) != len(live.Written) {
		t.Fatalf("dry run reported %d grants, live run %d", len(dry.Written), len(live.Written))
	}
	for i := range dry.Written {
		d, l := dry.Written[i], live.Written[i]
		if d.ShareID != l.ShareID || d.Grantee != l.Grantee || d.Action != l.Action ||
			d.Observed != l.Observed || d.Expected != l.Expected {
			t.Errorf("written[%d]: dry = %+v, live = %+v", i, d, l)
		}
	}
	if len(dryGrants.writes) != 0 {
		t.Errorf("dry_run wrote %+v, want nothing", dryGrants.writes)
	}
	if len(liveGrants.writes) != 2 {
		t.Errorf("live run wrote %+v, want 2", liveGrants.writes)
	}
}

func TestShallowPathLookupFailureSkips(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(30, "space-a", "inode-30", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{users: map[string]bool{"jdoe": true}} // no path resolves
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not fail on a per-share lookup error: %v", err)
	}
	if report.Skipped != 1 || report.Checked != 0 {
		t.Fatalf("report = %+v, want 1 skipped / 0 checked", report)
	}
	if len(grants.writes) != 0 {
		t.Errorf("wrote %+v without a resolved path, want nothing", grants.writes)
	}
}

// TestShallowMissingRecipientSkips asserts that a share whose recipient is gone
// is left alone. Deciding it is dead is the orphan job's call, and an entry
// cannot be written for an identity that does not resolve.
func TestShallowMissingRecipientSkips(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(31, "space-a", "inode-31", "ghost", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{},
		paths: map[string]string{"eosuser/inode-31": "/eos/user/a/alice/dir"},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Skipped != 1 || len(report.Written) != 0 {
		t.Fatalf("report = %+v, want 1 skipped / 0 written", report)
	}
	if len(grants.writes) != 0 {
		t.Errorf("wrote %+v for a missing recipient, want nothing", grants.writes)
	}
}

func TestShallowGrantsUnreadableSkips(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(32, "space-a", "inode-32", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{"eosuser/inode-32": "/eos/user/a/alice/dir"},
	}
	grants := &fakeGrants{listErr: errors.New("storage down")}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not fail when one node cannot be read: %v", err)
	}
	if report.Skipped != 1 || len(report.Written) != 0 {
		t.Fatalf("report = %+v, want 1 skipped / 0 written", report)
	}
}

func TestShallowListErrorFails(t *testing.T) {
	job := shallowJob(&fakeStore{listErr: errors.New("db down")}, &fakeGateway{}, &fakeGrants{})

	if _, err := job.Run(context.Background()); err == nil {
		t.Fatal("Run must fail when shares cannot be listed")
	}
}

// TestShallowWriteErrorIsNotReportedAsWritten asserts that a failed write is
// counted as a failure and kept out of the report, so neither the log nor the
// report claims a change that did not happen.
func TestShallowWriteErrorIsNotReportedAsWritten(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(33, "space-a", "inode-33", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{"eosuser/inode-33": "/eos/user/a/alice/dir"},
	}
	grants := &fakeGrants{writeErr: errors.New("storage refused")}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not fail on a per-share write error: %v", err)
	}
	if report.Failed != 1 || len(report.Written) != 0 {
		t.Fatalf("report = %+v, want 1 failed / 0 written", report)
	}
}
