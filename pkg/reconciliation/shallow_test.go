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
	"encoding/json"
	"errors"
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/rjobs"
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
	return &ShallowJob{ShareStore: store, Gateway: gw, Grants: grantsFrom(grants)}
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
// share on a path above it gets no entry. Its entry would override the one
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
// share gets no entry. Writing it would hand back the access the deny takes
// away.
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

// TestShallowRemovesInheritedChildShare asserts that a share granting exactly
// what a share above it already grants the same recipient is removed: creating
// the share above would have deleted it, so keeping it around only makes the
// database disagree with the storage.
func TestShallowRemovesInheritedChildShare(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(60, "space-a", "inode-parent", "jdoe", false, ocsRead),
		shared(61, "space-a", "inode-child", "jdoe", false, ocsRead),
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
	if len(report.Removed) != 1 {
		t.Fatalf("removed = %+v, want the child share", report.Removed)
	}
	got := report.Removed[0]
	if got.ShareID != "61" || got.Reason != ReasonInherited || got.AncestorID != "60" {
		t.Errorf("removed = %+v, want share 61 inherited from 60", got)
	}
	if len(store.unshared) != 1 || store.unshared[0] != "61" {
		t.Errorf("unshared = %v, want [61]", store.unshared)
	}
}

// TestShallowRemovesWeakerChildShare asserts that a share granting less than a
// share above it is removed too. Its entry is never written, since writing it
// would take away access the ancestor grants, so the row is a share the
// hierarchy check would never have allowed to exist.
func TestShallowRemovesWeakerChildShare(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(62, "space-a", "inode-parent", "jdoe", false, ocsWrite),
		shared(63, "space-a", "inode-child", "jdoe", false, ocsRead),
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
	if len(report.Removed) != 1 || report.Removed[0].ShareID != "63" ||
		report.Removed[0].Reason != ReasonShadowedByAncestor {
		t.Fatalf("removed = %+v, want share 63 shadowed by an ancestor", report.Removed)
	}
	if len(store.unshared) != 1 || store.unshared[0] != "63" {
		t.Errorf("unshared = %v, want [63]", store.unshared)
	}
}

// TestShallowKeepsEscalatingChildShare asserts that a share granting more than
// the one above it is kept: it is the only thing giving its recipient that
// access, so removing it would silently downgrade them.
func TestShallowKeepsEscalatingChildShare(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(64, "space-a", "inode-parent", "jdoe", false, ocsRead),
		shared(65, "space-a", "inode-child", "jdoe", false, ocsWrite),
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
	if len(report.Removed) != 0 || len(store.unshared) != 0 {
		t.Fatalf("removed = %+v / unshared = %v, want neither", report.Removed, store.unshared)
	}
}

// TestShallowUnresolvedShareIsNotRemoved asserts that a share whose path cannot
// be resolved is never removed: it takes a named ancestor to make a share
// redundant, and a share nothing could be compared against is not one.
func TestShallowUnresolvedShareIsNotRemoved(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(66, "space-a", "inode-66", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{users: map[string]bool{"jdoe": true}} // no path resolves
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Skipped != 1 || len(report.Removed) != 0 || len(store.unshared) != 0 {
		t.Fatalf("report = %+v / unshared = %v, want 1 skipped and nothing removed", report, store.unshared)
	}
}

// TestShallowDryRunDoesNotRemove asserts that a dry run reports the removals it
// would make without touching the database.
func TestShallowDryRunDoesNotRemove(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(67, "space-a", "inode-parent", "jdoe", false, ocsRead),
		shared(68, "space-a", "inode-child", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{
			"eosuser/inode-parent": "/eos/user/a/alice/dir",
			"eosuser/inode-child":  "/eos/user/a/alice/dir/sub",
		},
	}
	grants := &fakeGrants{}

	job := shallowJob(store, gw, grants)
	job.DryRun = true
	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Removed) != 1 || report.Removed[0].ShareID != "68" {
		t.Fatalf("removed = %+v, want the child share reported", report.Removed)
	}
	if len(store.unshared) != 0 {
		t.Errorf("dry_run removed %v, want nothing", store.unshared)
	}
}

// TestShallowRemoveErrorIsNotReportedAsRemoved asserts that a share whose
// removal fails is counted failed and left out of the report, so the log never
// claims a row is gone that is still there.
func TestShallowRemoveErrorIsNotReportedAsRemoved(t *testing.T) {
	store := &fakeStore{
		shares: []storedShare{
			shared(69, "space-a", "inode-parent", "jdoe", false, ocsRead),
			shared(70, "space-a", "inode-child", "jdoe", false, ocsRead),
		},
		unshareErr: errors.New("database is down"),
	}
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
		t.Fatalf("Run must not fail on a per-share removal error: %v", err)
	}
	if report.Failed != 1 || len(report.Removed) != 0 {
		t.Fatalf("report = %+v, want 1 failed and nothing removed", report)
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
	// by share, not by position: the runs group per recipient and per space, and
	// the groups are independent, so their order is not part of the result.
	byShare := map[string]WrittenGrant{}
	for _, l := range live.Written {
		byShare[l.ShareID] = l
	}
	for _, d := range dry.Written {
		l, ok := byShare[d.ShareID]
		if !ok {
			t.Errorf("dry run reported share %s, live run did not", d.ShareID)
			continue
		}
		if d.Grantee != l.Grantee || d.Action != l.Action ||
			d.Observed != l.Observed || d.Expected != l.Expected {
			t.Errorf("share %s: dry = %+v, live = %+v", d.ShareID, d, l)
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

// twoSpaces builds a job over one share in each of two spaces, both missing
// their entry.
func twoSpaces() (*fakeStore, *fakeGrants, *ShallowJob) {
	store := &fakeStore{shares: []storedShare{
		shared(80, "space-a", "inode-a", "jdoe", false, ocsRead),
		shared(81, "space-b", "inode-b", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{
			"eosuser/inode-a": "/eos/project/a/one",
			"eosuser/inode-b": "/eos/project/b/one",
		},
	}
	grants := &fakeGrants{}
	return store, grants, shallowJob(store, gw, grants)
}

// TestShallowRunSpaceVisitsOnlyThatSpace asserts that a scoped run leaves the
// shares of every other space untouched.
func TestShallowRunSpaceVisitsOnlyThatSpace(t *testing.T) {
	_, grants, job := twoSpaces()

	report, err := job.RunSpace(context.Background(), "space-a")
	if err != nil {
		t.Fatalf("RunSpace: %v", err)
	}
	if report.SpaceID != "space-a" || report.Checked != 1 || len(report.Written) != 1 {
		t.Fatalf("report = %+v, want space-a / 1 checked / 1 written", report)
	}
	if len(grants.writes) != 1 || grants.writes[0].node != "eosuser/inode-a" {
		t.Errorf("writes = %+v, want only the share in space-a", grants.writes)
	}
}

// TestShallowRunSpaceNeedsASpace asserts that an empty space is refused rather
// than taken as a full run.
func TestShallowRunSpaceNeedsASpace(t *testing.T) {
	_, grants, job := twoSpaces()

	if _, err := job.RunSpace(context.Background(), ""); err == nil {
		t.Fatal("RunSpace must fail without a space")
	}
	if len(grants.writes) != 0 {
		t.Errorf("wrote %+v, want nothing", grants.writes)
	}
}

// TestShallowOnDemandRunsTheGivenSpace asserts that the space parameter scopes
// the run, and that its totals come back as the result.
func TestShallowOnDemandRunsTheGivenSpace(t *testing.T) {
	_, grants, job := twoSpaces()

	run, err := job.OnDemand(context.Background(), nil)
	if err != nil {
		t.Fatalf("OnDemand: %v", err)
	}
	result, err := run.Run(context.Background(), rjobs.Params{"space": "space-b"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result["space"] != "space-b" || result["checked"] != 1 || result["written"] != 1 {
		t.Fatalf("result = %+v, want space-b / 1 checked / 1 written", result)
	}
	if len(grants.writes) != 1 || grants.writes[0].node != "eosuser/inode-b" {
		t.Errorf("writes = %+v, want only the share in space-b", grants.writes)
	}
}

// TestShallowOnDemandWithoutASpaceRunsAll asserts that a run triggered without a
// space covers every space.
func TestShallowOnDemandWithoutASpaceRunsAll(t *testing.T) {
	_, grants, job := twoSpaces()

	run, err := job.OnDemand(context.Background(), nil)
	if err != nil {
		t.Fatalf("OnDemand: %v", err)
	}
	result, err := run.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result["space"] != "" || result["written"] != 2 {
		t.Fatalf("result = %+v, want no space / 2 written", result)
	}
	if len(grants.writes) != 2 {
		t.Errorf("writes = %+v, want both shares", grants.writes)
	}
}

// TestShallowOnDemandRejectsBadParams asserts that a run whose parameters cannot
// be read fails instead of falling back to a full run.
func TestShallowOnDemandRejectsBadParams(t *testing.T) {
	_, grants, job := twoSpaces()

	run, err := job.OnDemand(context.Background(), nil)
	if err != nil {
		t.Fatalf("OnDemand: %v", err)
	}
	if _, err := run.Run(context.Background(), rjobs.Params{"space": 42}); err == nil {
		t.Fatal("Run must fail on a space that is not a string")
	}
	if len(grants.writes) != 0 {
		t.Errorf("wrote %+v, want nothing", grants.writes)
	}
}

// TestShallowOnDemandRejectsUnknownParams asserts that a mistyped parameter
// fails the run rather than being ignored, which would turn a run meant for one
// space into a run over every space.
func TestShallowOnDemandRejectsUnknownParams(t *testing.T) {
	_, grants, job := twoSpaces()

	run, err := job.OnDemand(context.Background(), nil)
	if err != nil {
		t.Fatalf("OnDemand: %v", err)
	}
	if _, err := run.Run(context.Background(), rjobs.Params{"spaces": "space-a"}); err == nil {
		t.Fatal("Run must fail on a parameter it does not read")
	}
	if len(grants.writes) != 0 {
		t.Errorf("wrote %+v, want nothing", grants.writes)
	}
}

// TestShallowOnDemandTakesAdminParams asserts that the parameters survive the
// trip the admin API puts them through, where every value arrives as a string.
func TestShallowOnDemandTakesAdminParams(t *testing.T) {
	_, grants, job := twoSpaces()

	// what "reva admin jobs run reconciliation.shallow space=space-b" sends: a
	// string map, marshalled and unmarshalled on the way to the runner.
	raw, err := json.Marshal(map[string]string{"space": "space-b"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var params rjobs.Params
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	run, err := job.OnDemand(context.Background(), nil)
	if err != nil {
		t.Fatalf("OnDemand: %v", err)
	}
	result, err := run.Run(context.Background(), params)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result["space"] != "space-b" || len(grants.writes) != 1 {
		t.Fatalf("result = %+v / writes = %+v, want the run limited to space-b", result, grants.writes)
	}
}

// TestShallowSkipsSpaceWithANewShare asserts that a space a user shared in
// while the run was checking is left untouched. The run compared the shares as
// they were, so writing what it decided could contradict the new share.
func TestShallowSkipsSpaceWithANewShare(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(100, "space-a", "inode-100", "jdoe", false, ocsRead),
	}}
	// the share a user creates between the check and the writes.
	store.listHook = func(f *fakeStore, space string, done int) {
		if done == 1 {
			f.shares = append(f.shares, shared(101, "space-a", "inode-101", "asmith", false, ocsRead))
		}
	}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true, "asmith": true},
		paths: map[string]string{"eosuser/inode-100": "/eos/user/a/alice/dir"},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.SkippedSpaces) != 1 || report.SkippedSpaces[0] != "space-a" {
		t.Fatalf("skipped spaces = %v, want [space-a]", report.SkippedSpaces)
	}
	if len(report.Written) != 0 || len(grants.writes) != 0 {
		t.Errorf("report = %+v / writes = %+v, want nothing written", report, grants.writes)
	}
}

// TestShallowSkipsOnlyTheChangedSpace asserts that one space changing under the
// run does not hold up the others: spaces are disjoint, so each stands alone.
func TestShallowSkipsOnlyTheChangedSpace(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(102, "space-a", "inode-102", "jdoe", false, ocsRead),
		shared(103, "space-b", "inode-103", "jdoe", false, ocsRead),
	}}
	store.listHook = func(f *fakeStore, space string, done int) {
		if space == "space-a" {
			f.shares = append(f.shares, shared(104, "space-a", "inode-104", "asmith", false, ocsRead))
		}
	}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true, "asmith": true},
		paths: map[string]string{
			"eosuser/inode-102": "/eos/project/a/one",
			"eosuser/inode-103": "/eos/project/b/one",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.SkippedSpaces) != 1 || report.SkippedSpaces[0] != "space-a" {
		t.Fatalf("skipped spaces = %v, want [space-a]", report.SkippedSpaces)
	}
	if len(grants.writes) != 1 || grants.writes[0].node != "eosuser/inode-103" {
		t.Errorf("writes = %+v, want only the share in space-b", grants.writes)
	}
}

// TestShallowHoldsBackRemovalsOfAChangedSpace asserts that a skipped space keeps
// its redundant shares too. A share created under the run can be the very thing
// that makes one of them needed again.
func TestShallowHoldsBackRemovalsOfAChangedSpace(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(105, "space-a", "inode-parent", "jdoe", false, ocsRead),
		shared(106, "space-a", "inode-child", "jdoe", false, ocsRead),
	}}
	store.listHook = func(f *fakeStore, space string, done int) {
		if done == 1 {
			f.shares = append(f.shares, shared(107, "space-a", "inode-107", "asmith", false, ocsRead))
		}
	}
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
	if len(report.SkippedSpaces) != 1 || len(report.Removed) != 0 || len(store.unshared) != 0 {
		t.Fatalf("report = %+v / unshared = %v, want the space skipped and nothing removed", report, store.unshared)
	}
}

// TestShallowSkipsSpaceWhenTheCheckCannotRun asserts that a space whose shares
// cannot be listed again is left untouched. Without the second listing there is
// no telling whether the run is still working from what is in the database.
func TestShallowSkipsSpaceWhenTheCheckCannotRun(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(108, "space-a", "inode-108", "jdoe", false, ocsRead),
	}}
	store.listHook = func(f *fakeStore, space string, done int) {
		if done == 1 {
			f.listErr = errors.New("db down")
		}
	}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{"eosuser/inode-108": "/eos/user/a/alice/dir"},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not fail when one space cannot be listed again: %v", err)
	}
	if len(report.SkippedSpaces) != 1 || len(grants.writes) != 0 {
		t.Fatalf("report = %+v / writes = %+v, want the space skipped and nothing written", report, grants.writes)
	}
}

// TestShallowDoesNotRelistAnUnchangedSpace asserts that a space the run changes
// nothing in is not listed again: there is nothing to hold back, so a share
// created in it spoils nothing.
func TestShallowDoesNotRelistAnUnchangedSpace(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(109, "space-a", "inode-109", "jdoe", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true},
		paths: map[string]string{"eosuser/inode-109": "/eos/user/a/alice/dir"},
	}
	grants := &fakeGrants{grants: map[string][]*provider.Grant{
		"eosuser/inode-109": {grant(provider.GranteeType_GRANTEE_TYPE_USER, "jdoe", ocsRead)},
	}}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Written) != 0 || len(report.SkippedSpaces) != 0 {
		t.Fatalf("report = %+v, want nothing written and no space skipped", report)
	}
	if store.lists != 1 {
		t.Errorf("listed the shares %d times, want 1", store.lists)
	}
}

// TestShallowDryRunSkipsAChangedSpace asserts that a dry run holds back a
// changed space as well, so its report says what a live run would do.
func TestShallowDryRunSkipsAChangedSpace(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(110, "space-a", "inode-110", "jdoe", false, ocsRead),
	}}
	store.listHook = func(f *fakeStore, space string, done int) {
		if done == 1 {
			f.shares = append(f.shares, shared(111, "space-a", "inode-111", "asmith", false, ocsRead))
		}
	}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true, "asmith": true},
		paths: map[string]string{"eosuser/inode-110": "/eos/user/a/alice/dir"},
	}
	grants := &fakeGrants{}

	job := shallowJob(store, gw, grants)
	job.DryRun = true
	report, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.SkippedSpaces) != 1 || len(report.Written) != 0 {
		t.Fatalf("report = %+v, want the space skipped and nothing reported written", report)
	}
}

// TestShallowRunSpaceChecksItsOwnSpace asserts that a run scoped to one space
// checks that space too, and reports it skipped rather than writing against a
// view that moved.
func TestShallowRunSpaceChecksItsOwnSpace(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(112, "space-a", "inode-112", "jdoe", false, ocsRead),
	}}
	store.listHook = func(f *fakeStore, space string, done int) {
		if done == 1 {
			f.shares = append(f.shares, shared(113, "space-a", "inode-113", "asmith", false, ocsRead))
		}
	}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true, "asmith": true},
		paths: map[string]string{"eosuser/inode-112": "/eos/user/a/alice/dir"},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).RunSpace(context.Background(), "space-a")
	if err != nil {
		t.Fatalf("RunSpace: %v", err)
	}
	if len(report.SkippedSpaces) != 1 || len(grants.writes) != 0 {
		t.Fatalf("report = %+v / writes = %+v, want the space skipped", report, grants.writes)
	}
}

// TestShallowLooksUpEachRecipientOnce asserts that a run resolves a recipient
// once, however many shares that recipient holds.
func TestShallowLooksUpEachRecipientOnce(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(90, "space-a", "inode-90", "jdoe", false, ocsRead),
		shared(91, "space-a", "inode-91", "jdoe", false, ocsRead),
		shared(92, "space-a", "inode-92", "jdoe", false, ocsRead),
		shared(93, "space-a", "inode-93", "asmith", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{"jdoe": true, "asmith": true},
		paths: map[string]string{
			"eosuser/inode-90": "/eos/user/a/alice/one",
			"eosuser/inode-91": "/eos/user/a/alice/two",
			"eosuser/inode-92": "/eos/user/a/alice/three",
			"eosuser/inode-93": "/eos/user/a/alice/four",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Checked != 4 || len(report.Written) != 4 {
		t.Fatalf("report = %+v, want 4 checked / 4 written", report)
	}
	if gw.userLookups != 2 {
		t.Errorf("looked up %d users, want 2", gw.userLookups)
	}
}

// TestShallowRetriesAnUnresolvedRecipient asserts that a name which does not
// resolve is not remembered, so every share of that name is judged on its own
// lookup.
func TestShallowRetriesAnUnresolvedRecipient(t *testing.T) {
	store := &fakeStore{shares: []storedShare{
		shared(94, "space-a", "inode-94", "ghost", false, ocsRead),
		shared(95, "space-a", "inode-95", "ghost", false, ocsRead),
	}}
	gw := &fakeGateway{
		users: map[string]bool{},
		paths: map[string]string{
			"eosuser/inode-94": "/eos/user/a/alice/one",
			"eosuser/inode-95": "/eos/user/a/alice/two",
		},
	}
	grants := &fakeGrants{}

	report, err := shallowJob(store, gw, grants).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Skipped != 2 || gw.userLookups != 2 {
		t.Fatalf("report = %+v / %d lookups, want 2 skipped and 2 lookups", report, gw.userLookups)
	}
}
