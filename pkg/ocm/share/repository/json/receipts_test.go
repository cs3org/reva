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

package json

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/utils"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
)

func receivedFixture(t *testing.T, m share.Repository) (*userpb.User, *ocm.ReceivedShare) {
	t.Helper()
	recipient := &userpb.User{Id: testGrantee().GetUserId()}
	rs, err := m.StoreReceivedShare(context.Background(), &ocm.ReceivedShare{
		RemoteShareId: "remote-1",
		Name:          "doc",
		Grantee:       testGrantee(),
		Owner:         &userpb.UserId{OpaqueId: "alice", Idp: "provider.example.org"},
		Creator:       &userpb.UserId{OpaqueId: "alice", Idp: "provider.example.org"},
		State:         ocm.ShareState_SHARE_STATE_PENDING,
		ShareType:     ocm.ShareType_SHARE_TYPE_USER,
	})
	if err != nil {
		t.Fatal(err)
	}
	return recipient, rs
}

func mask(paths ...string) *field_mask.FieldMask { return &field_mask.FieldMask{Paths: paths} }

func TestJSONDeleteAndUpdateShareReceipts(t *testing.T) {
	m, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()
	u := testUser()
	s := testShare("tok", legacyMethods())
	if _, err := m.StoreShare(ctx, s, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER); err != nil {
		t.Fatal(err)
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: s.Id}}
	// The resource type is persisted with the share.
	m, err := New(ctx, map[string]any{"file": m.(*mgr).c.File})
	if err != nil {
		t.Fatal(err)
	}

	exp := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_Expiration{
		Expiration: &typespb.Timestamp{Seconds: 1},
	}}
	if up, err := m.UpdateShare(ctx, u, ref, exp); err != nil || up.PermissionsChanged() {
		t.Fatalf("expiration only: %v %v", up, err)
	}
	editor := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
		AccessMethods: share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), nil, nil),
	}}
	if up, err := m.UpdateShare(ctx, u, ref, editor); err != nil || !up.WebDAVPermissionsChanged || up.WebappPermissionsChanged ||
		up.ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER {
		t.Fatalf("webdav change: %v %v", up, err)
	}
	if up, err := m.UpdateShare(ctx, u, ref, editor); err != nil || up.PermissionsChanged() {
		t.Fatalf("repeat: %v %v", up, err)
	}

	removal, err := m.DeleteShare(ctx, u, ref)
	if err != nil || len(removal) != 1 || removal[0].Share.GetToken() != "tok" || removal[0].Share.GetGrantee() == nil ||
		removal[0].ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER {
		t.Fatalf("delete: %v %v", removal, err)
	}
	if r, err := m.DeleteShare(ctx, u, ref); err == nil || r != nil {
		t.Fatalf("repeat delete: %v %v", r, err)
	}
}

func TestJSONTransferAttemptsPersistAcrossReload(t *testing.T) {
	m, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()
	user, rs := receivedFixture(t, m)

	start, err := m.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id}, mask("state"))
	if err != nil || start.Generation != 1 || start.Share.State != ocm.ShareState_SHARE_STATE_TRANSFERRING {
		t.Fatalf("start: %v %v", start, err)
	}
	if _, err := m.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id}, mask("state")); !errors.Is(err, share.ErrTransferInProgress) {
		t.Fatalf("second start: %v", err)
	}

	// A new manager on the same file sees the persisted attempt.
	reopened, err := New(ctx, map[string]any{"file": m.(*mgr).c.File})
	if err != nil {
		t.Fatal(err)
	}
	if u, err := reopened.CompleteReceivedShareTransfer(ctx, user, rs.Id, 2, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil || u.StateChanged() {
		t.Fatalf("wrong generation: %v %v", u, err)
	}
	u, err := reopened.CompleteReceivedShareTransfer(ctx, user, rs.Id, 1, ocm.ShareState_SHARE_STATE_ACCEPTED)
	if err != nil || !u.StateChanged() || u.After.State != ocm.ShareState_SHARE_STATE_ACCEPTED {
		t.Fatalf("complete: %v %v", u, err)
	}
	if u, err := reopened.CompleteReceivedShareTransfer(ctx, user, rs.Id, 1, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil || u.StateChanged() {
		t.Fatalf("duplicate: %v %v", u, err)
	}
	if _, err := reopened.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id}, mask("state")); !errors.Is(err, share.ErrShareAlreadyAccepted) {
		t.Fatalf("start after acceptance: %v", err)
	}

	reset := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_PENDING}
	if u, err := reopened.UpdateReceivedShare(ctx, user, reset, mask("state")); err != nil || !u.StateChanged() {
		t.Fatalf("reset: %v %v", u, err)
	}
	if u, err := reopened.UpdateReceivedShare(ctx, user, reset, mask("state")); err != nil || u.StateChanged() {
		t.Fatalf("repeat reset: %v %v", u, err)
	}
	second, err := reopened.StartReceivedShareTransfer(ctx, user, reset, mask("state"))
	if err != nil || second.Generation <= 1 {
		t.Fatalf("second attempt: %v %v", second, err)
	}
	if u, err := reopened.CompleteReceivedShareTransfer(ctx, user, rs.Id, 1, ocm.ShareState_SHARE_STATE_REJECTED); err != nil || u.StateChanged() {
		t.Fatalf("stale callback: %v %v", u, err)
	}
	stranger := &userpb.User{Id: &userpb.UserId{OpaqueId: "stranger"}}
	if _, err := reopened.UpdateReceivedShare(ctx, stranger, reset, mask("state")); err == nil {
		t.Fatal("foreign update accepted")
	}
}

func TestJSONTransferGenerationExhaustion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ocm.json")
	m, err := New(context.Background(), map[string]any{"file": path})
	if err != nil {
		t.Fatal(err)
	}
	user, rs := receivedFixture(t, m)
	mm := m.(*mgr)
	mm.Lock()
	if err := mm.load(); err != nil {
		t.Fatal(err)
	}
	mm.model.TransferGenerations[rs.Id.OpaqueId] = math.MaxInt64
	if err := mm.save(); err != nil {
		t.Fatal(err)
	}
	mm.Unlock()
	if _, err := m.StartReceivedShareTransfer(context.Background(), user, &ocm.ReceivedShare{Id: rs.Id}, mask("state")); !errors.Is(err, share.ErrTransferGenerationExhausted) {
		t.Fatalf("start: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

// A share without a supported resource type is never stored, and a stored
// one, e.g. in a hand-edited file, is not changed.
func TestJSONShareResourceTypeAdmissionAndExistingMutations(t *testing.T) {
	m, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := m.StoreShare(ctx, testShare("tok-none", legacyMethods()), ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID); !errors.Is(err, share.ErrInvalidResourceType) {
		t.Fatalf("store: %v", err)
	}
	if shares, err := m.ListShares(ctx, testUser(), nil); err != nil || len(shares) != 0 {
		t.Fatalf("stored: %v %v", shares, err)
	}

	s := testShare("tok", legacyMethods())
	raw, err := utils.MarshalProtoV1ToJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: s.Id}}
	editor := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
		AccessMethods: share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), nil, nil),
	}}
	for _, tc := range []struct{ name, typeEntry string }{
		{"prior format", ""},
		{"explicit invalid type", `,"resource_types":{"` + s.Id.OpaqueId + `":0}`},
		{"unsupported type", `,"resource_types":{"` + s.Id.OpaqueId + `":99}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ocm.json")
			file := `{"shares":{"` + s.Id.OpaqueId + `":` + string(raw) + `},"received_shares":{}` + tc.typeEntry + `}`
			if err := os.WriteFile(path, []byte(file), 0600); err != nil {
				t.Fatal(err)
			}
			repo, err := New(ctx, map[string]any{"file": path})
			if err != nil {
				t.Fatal(err)
			}
			up, updateErr := repo.UpdateShare(ctx, testUser(), ref, editor)
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if tc.typeEntry == "" {
				if updateErr != nil || up == nil || !up.WebDAVPermissionsChanged ||
					up.ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID {
					t.Errorf("update of prior-format share: %v %v", up, updateErr)
				}
				var persisted shareModel
				if err := json.Unmarshal(after, &persisted); err != nil {
					t.Fatal(err)
				}
				got := persisted.Shares[s.Id.OpaqueId]
				if got == nil || !proto.Equal(webdavPermissions(got), editor.GetAccessMethods().GetWebdavOptions().GetPermissions()) {
					t.Errorf("permissions not persisted: %v", got)
				}
				if _, present := persisted.ResourceTypes[s.Id.OpaqueId]; present {
					t.Error("update filled in a resource type")
				}
				removed, err := repo.DeleteShare(ctx, testUser(), ref)
				if err != nil || len(removed) != 1 || removed[0].ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID ||
					removed[0].Share.GetId().GetOpaqueId() != s.Id.OpaqueId {
					t.Errorf("withdrawal of prior-format share: %v %v", removed, err)
				}
				if got, err := repo.GetShare(ctx, testUser(), ref); err == nil || got != nil {
					t.Errorf("withdrawn share remains available: %v %v", got, err)
				}
				return
			}
			if !errors.Is(updateErr, share.ErrInvalidResourceType) || up != nil {
				t.Errorf("corrupt update: %v %v", up, updateErr)
			}
			if removed, err := repo.DeleteShare(ctx, testUser(), ref); !errors.Is(err, share.ErrInvalidResourceType) || removed != nil {
				t.Errorf("corrupt delete: %v %v", removed, err)
			}
			afterDelete, err := os.ReadFile(path)
			if err != nil || string(after) != file || string(afterDelete) != file {
				t.Errorf("corrupt file changed: %v", err)
			}
		})
	}
}
