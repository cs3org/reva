// Copyright 2018-2024 CERN
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

package localfs

import (
	"context"
	"os"
	"path"
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/storage/utils/grants"
)

const testIdp = "https://idp.example.test"

// newTestLocalfs returns a localfs over a temporary root with a shared
// directory owned by einstein, plus the owner context and its reference.
func newTestLocalfs(t *testing.T) (*localfs, context.Context, *provider.Reference) {
	t.Helper()

	config := &Config{Root: t.TempDir(), UserLayout: "{{.Username}}", ShareFolder: "/MyShares"}
	config.ApplyDefaults()
	db, err := initializeDB(config.Root, "test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if err := os.MkdirAll(path.Join(config.DataDirectory, "einstein", "sf"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(config.DataDirectory, "einstein", "sf", "shared.txt"), []byte("shared"), 0600); err != nil {
		t.Fatal(err)
	}

	owner := &userpb.User{
		Id:       &userpb.UserId{OpaqueId: "einstein", Idp: testIdp, Type: userpb.UserType_USER_TYPE_PRIMARY},
		Username: "einstein",
	}
	ctx := appctx.ContextSetUser(context.Background(), owner)
	return &localfs{conf: config, db: db}, ctx, &provider.Reference{Path: "/sf"}
}

// activeGrantees lists the grantees that carry permissions after a revoke. A
// revoked grant keeps its row with an empty role, so it must not show up here.
func activeGrantees(t *testing.T, fs *localfs, ctx context.Context, ref *provider.Reference) map[string]bool {
	t.Helper()

	list, err := fs.ListGrants(ctx, ref)
	if err != nil {
		t.Fatalf("ListGrants: %v", err)
	}
	active := map[string]bool{}
	for _, g := range list {
		if !g.GetPermissions().GetStat() {
			continue
		}
		key, err := aclGrantee(g.GetGrantee())
		if err != nil {
			t.Fatalf("aclGrantee: %v", err)
		}
		active[key] = true
	}
	return active
}

func userGrantee(id *userpb.UserId) *provider.Grantee {
	return &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id:   &provider.Grantee_UserId{UserId: id},
	}
}

// TestRemoveGrantRevokesUserAccess pins the grant lifecycle: AddGrant keys the
// ACL row as "u:marie:primary@idp" while RemoveGrant used to look for
// "u:marie@idp", so the revoke matched no row and a recipient kept the access
// the share carried after it was unshared or rejected.
func TestRemoveGrantRevokesUserAccess(t *testing.T) {
	fs, ctx, ref := newTestLocalfs(t)
	marie := &userpb.UserId{OpaqueId: "marie", Idp: testIdp, Type: userpb.UserType_USER_TYPE_PRIMARY}

	if err := fs.AddGrant(ctx, ref, &provider.Grant{Grantee: userGrantee(marie), Permissions: grants.GetGrantPermissionSet("viewer")}); err != nil {
		t.Fatalf("AddGrant: %v", err)
	}
	if !activeGrantees(t, fs, ctx, ref)["u:marie:primary@"+testIdp] {
		t.Fatal("grantee has no access after AddGrant")
	}

	if err := fs.RemoveGrant(ctx, ref, &provider.Grant{Grantee: userGrantee(marie)}); err != nil {
		t.Fatalf("RemoveGrant: %v", err)
	}
	if activeGrantees(t, fs, ctx, ref)["u:marie:primary@"+testIdp] {
		t.Fatal("grantee still has access after RemoveGrant")
	}
}

// TestRemoveGrantWithoutIdpRevokesAccess covers the shape the gateway sends: the
// share manager stores only the grantee username, so a revoke carries no idp
// while the grant was written with one. The revoke must still clear the grant.
func TestRemoveGrantWithoutIdpRevokesAccess(t *testing.T) {
	fs, ctx, ref := newTestLocalfs(t)
	marie := &userpb.UserId{OpaqueId: "marie", Idp: testIdp, Type: userpb.UserType_USER_TYPE_PRIMARY}
	// The share manager only knows the username, so the id it hands back has no idp.
	marieFromShare := &userpb.UserId{OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_PRIMARY}

	if err := fs.AddGrant(ctx, ref, &provider.Grant{Grantee: userGrantee(marie), Permissions: grants.GetGrantPermissionSet("viewer")}); err != nil {
		t.Fatalf("AddGrant: %v", err)
	}
	if err := fs.RemoveGrant(ctx, ref, &provider.Grant{Grantee: userGrantee(marieFromShare)}); err != nil {
		t.Fatalf("RemoveGrant without idp: %v", err)
	}
	if activeGrantees(t, fs, ctx, ref)["u:marie:primary@"+testIdp] {
		t.Fatal("grantee still has access after an idp-less RemoveGrant")
	}

	// A grant written without the idp must not leave an effective row behind when
	// it is revoked with the full id, and the other way round.
	if err := fs.AddGrant(ctx, ref, &provider.Grant{Grantee: userGrantee(marieFromShare), Permissions: grants.GetGrantPermissionSet("viewer")}); err != nil {
		t.Fatalf("AddGrant without idp: %v", err)
	}
	if err := fs.RemoveGrant(ctx, ref, &provider.Grant{Grantee: userGrantee(marie)}); err != nil {
		t.Fatalf("RemoveGrant with idp: %v", err)
	}
	if activeGrantees(t, fs, ctx, ref)["u:marie:primary@"+testIdp] || activeGrantees(t, fs, ctx, ref)["u:marie:primary@"] {
		t.Fatal("grantee still has access after re-granting without idp and revoking with it")
	}
}

// TestRemoveGrantRevokesGroupAccess covers group grants: AddGrant stores the
// group as "egroup::scientists@idp" while RemoveGrant used a single colon, so
// the revoke matched no row and every member kept access.
func TestRemoveGrantRevokesGroupAccess(t *testing.T) {
	fs, ctx, ref := newTestLocalfs(t)
	groupGrantee := func(idp string) *provider.Grantee {
		return &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: "scientists", Idp: idp}},
		}
	}

	if err := fs.AddGrant(ctx, ref, &provider.Grant{Grantee: groupGrantee(testIdp), Permissions: grants.GetGrantPermissionSet("viewer")}); err != nil {
		t.Fatalf("AddGrant: %v", err)
	}
	// The share manager hands the group back without an idp.
	if err := fs.AddGrant(ctx, ref, &provider.Grant{Grantee: groupGrantee(""), Permissions: grants.GetGrantPermissionSet("viewer")}); err != nil {
		t.Fatalf("AddGrant without idp: %v", err)
	}
	if !activeGrantees(t, fs, ctx, ref)["egroup::scientists@"+testIdp] && !activeGrantees(t, fs, ctx, ref)["egroup::scientists@"] {
		t.Fatal("group has no access after AddGrant")
	}

	if err := fs.RemoveGrant(ctx, ref, &provider.Grant{Grantee: groupGrantee(testIdp)}); err != nil {
		t.Fatalf("RemoveGrant: %v", err)
	}
	active := activeGrantees(t, fs, ctx, ref)
	if active["egroup::scientists@"+testIdp] || active["egroup::scientists@"] {
		t.Fatal("group still has access after RemoveGrant")
	}
}

// TestRemoveGrantRevokesNonASCIIGrantee covers user names outside ASCII: the
// prefix length must be compared in characters, because SQLite counts code
// points while len() counts bytes, and a byte count would skip the row.
func TestRemoveGrantRevokesNonASCIIGrantee(t *testing.T) {
	fs, ctx, ref := newTestLocalfs(t)
	jose := &userpb.UserId{OpaqueId: "josé", Idp: testIdp, Type: userpb.UserType_USER_TYPE_PRIMARY}
	joseFromShare := &userpb.UserId{OpaqueId: "josé", Type: userpb.UserType_USER_TYPE_PRIMARY}

	if err := fs.AddGrant(ctx, ref, &provider.Grant{Grantee: userGrantee(jose), Permissions: grants.GetGrantPermissionSet("viewer")}); err != nil {
		t.Fatalf("AddGrant: %v", err)
	}
	if err := fs.RemoveGrant(ctx, ref, &provider.Grant{Grantee: userGrantee(joseFromShare)}); err != nil {
		t.Fatalf("RemoveGrant without idp: %v", err)
	}
	active := activeGrantees(t, fs, ctx, ref)
	if active["u:josé:primary@"+testIdp] || active["u:josé:primary@"] {
		t.Fatal("grantee still has access after RemoveGrant")
	}
}

// TestRemoveGrantWithIdpContainingAt covers an idp whose URL carries an "@":
// the identity must be split on the first "@" of the last key field, not on the
// last "@" of the whole key.
func TestRemoveGrantWithIdpContainingAt(t *testing.T) {
	fs, ctx, ref := newTestLocalfs(t)
	atIdp := "https://idp.example.test/@realm"
	marie := &userpb.UserId{OpaqueId: "marie", Idp: atIdp, Type: userpb.UserType_USER_TYPE_PRIMARY}
	marieFromShare := &userpb.UserId{OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_PRIMARY}

	if err := fs.AddGrant(ctx, ref, &provider.Grant{Grantee: userGrantee(marie), Permissions: grants.GetGrantPermissionSet("viewer")}); err != nil {
		t.Fatalf("AddGrant: %v", err)
	}
	if err := fs.RemoveGrant(ctx, ref, &provider.Grant{Grantee: userGrantee(marieFromShare)}); err != nil {
		t.Fatalf("RemoveGrant without idp: %v", err)
	}
	active := activeGrantees(t, fs, ctx, ref)
	if active["u:marie:primary@"+atIdp] || active["u:marie:primary@"] {
		t.Fatal("grantee still has access after RemoveGrant")
	}
}
