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

package sql

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
)

func ownerContext(t *testing.T, id string) (context.Context, *userpb.User) {
	t.Helper()
	ctx := getUserContext(id)
	user, _ := appctx.ContextGetUser(ctx)
	return ctx, user
}

func storeOutgoing(t *testing.T, m *mgr, ctx context.Context, owner *userpb.User, role, token string) *ocm.Share {
	t.Helper()
	sh := getOcmShare(getOcmAccessMethods(role), getUserOcmShareGrantee("marie"), owner.Id, getResourceId(), token)
	sh.Grantee.GetUserId().Idp = "remote.example.org"
	stored, err := storeOcmShareWithID(m, ctx, sh)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func storeReceived(t *testing.T, m *mgr, ctx context.Context, user *userpb.User, remoteID string) *ocm.ReceivedShare {
	t.Helper()
	rs := getOCMReceivedShare(user, getUserOcmShareGrantee(user.Id.OpaqueId), "file", "viewer", remoteID)
	rs.State = ocm.ShareState_SHARE_STATE_PENDING
	rs.Creator = &userpb.UserId{OpaqueId: "alice", Idp: "provider.example.org"}
	stored, err := m.StoreReceivedShare(ctx, rs)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func idRef(id *ocm.ShareId) *ocm.ShareReference {
	return &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: id}}
}

func stateMask() *field_mask.FieldMask { return &field_mask.FieldMask{Paths: []string{"state"}} }

func TestDeleteShareReceipt(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, owner := ownerContext(t, "einstein")
		stored := storeOutgoing(t, m, ctx, owner, "viewer", "tok-1")

		removal, err := m.DeleteShare(ctx, owner, idRef(stored.Id))
		if err != nil {
			t.Fatal(err)
		}
		got := removal.Share
		if got.GetId().GetOpaqueId() != stored.Id.OpaqueId || got.GetToken() != "tok-1" ||
			got.GetGrantee().GetUserId().GetOpaqueId() != "marie" || len(got.GetAccessMethods()) != 2 {
			t.Fatalf("receipt share %v", got)
		}
		if _, err := m.DeleteShare(ctx, owner, idRef(stored.Id)); !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("repeat delete: %v", err)
		}
		other := storeOutgoing(t, m, ctx, owner, "viewer", "tok-2")
		_, stranger := ownerContext(t, "stranger")
		if r, err := m.DeleteShare(ctx, stranger, idRef(other.Id)); err == nil || r != nil {
			t.Fatalf("foreign delete produced %v %v", r, err)
		}
	})
}

func TestDeleteShareByKeyReceipt(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, owner := ownerContext(t, "einstein")
		stored := storeOutgoing(t, m, ctx, owner, "viewer", "tok-k")
		removal, err := m.DeleteShare(ctx, owner, &ocm.ShareReference{Spec: &ocm.ShareReference_Key{Key: &ocm.ShareKey{
			Owner: owner.Id, ResourceId: stored.ResourceId, Grantee: stored.Grantee,
		}}})
		if err != nil || removal.Share.GetToken() != "tok-k" {
			t.Fatalf("%v %v", removal, err)
		}
	})
}

func TestUpdateShareReceiptComparesStoredPermissions(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, owner := ownerContext(t, "einstein")
		stored := storeOutgoing(t, m, ctx, owner, "viewer", "tok")
		ref := idRef(stored.Id)
		viewer := permissions.NewViewerRole().CS3ResourcePermissions()
		editor := permissions.NewEditorRole().CS3ResourcePermissions()

		expiration := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_Expiration{
			Expiration: &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(72 * time.Hour).Unix())},
		}}
		webdavTo := func(role string) *ocm.UpdateOCMShareRequest_UpdateField {
			perms := viewer
			if role == "editor" {
				perms = editor
			}
			return &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
				AccessMethods: share.NewWebDavAccessMethod(perms, nil, nil),
			}}
		}
		webappTo := func(perms *provider.ResourcePermissions, app string) *ocm.UpdateOCMShareRequest_UpdateField {
			method := share.NewWebappAccessMethod(perms, nil)
			if app != "" {
				method.GetWebappOptions().AppNames = []string{app}
			}
			return &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
				AccessMethods: method,
			}}
		}
		storedWebapp := findWebappAccessMethod(stored.AccessMethods)

		cases := []struct {
			name           string
			field          *ocm.UpdateOCMShareRequest_UpdateField
			webdav, webapp bool
		}{
			{"expiration only", expiration, false, false},
			{"same webdav value", webdavTo("viewer"), false, false},
			{"webdav changed", webdavTo("editor"), true, false},
			{"webdav repeated", webdavTo("editor"), false, false},
			{"webapp app name only", webappTo(proto.Clone(storedWebapp.GetPermissions()).(*provider.ResourcePermissions), "renamed"), false, false},
			{"webapp flags changed", webappTo(webappCS3Permissions([]string{"share"}), ""), false, true},
			{"webapp flags repeated", webappTo(webappCS3Permissions([]string{"share"}), ""), false, false},
		}
		for _, tc := range cases {
			if tc.name == "expiration only" && m.db.Dialector.Name() == "mysql" {
				// MySQL rejects the inherited expiration update; a failed update has no receipt.
				if u, err := m.UpdateShare(ctx, owner, ref, tc.field); err == nil || u != nil {
					t.Fatalf("expiration on mysql: %v %v", u, err)
				}
				continue
			}
			u, err := m.UpdateShare(ctx, owner, ref, tc.field)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if u.WebDAVPermissionsChanged != tc.webdav || u.WebappPermissionsChanged != tc.webapp {
				t.Fatalf("%s: receipt webdav=%v webapp=%v", tc.name, u.WebDAVPermissionsChanged, u.WebappPermissionsChanged)
			}
		}
		// Permission updates preserve the stored Webapp descriptor.
		after, err := m.GetShare(ctx, owner, ref)
		if err != nil {
			t.Fatal(err)
		}
		wa := findWebappAccessMethod(after.AccessMethods)
		if !proto.Equal(wa.GetPermissions(), webappCS3Permissions([]string{"share"})) ||
			!slices.Equal(wa.GetAppNames(), storedWebapp.GetAppNames()) ||
			!slices.Equal(wa.GetRequirements(), storedWebapp.GetRequirements()) {
			t.Fatalf("webapp method %v", wa)
		}

		// A rejected update returns no receipt.
		bad := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
			AccessMethods: share.NewWebDavAccessMethod(editor, nil, []string{"must-exchange-token"}),
		}}
		if u, err := m.UpdateShare(ctx, owner, ref, bad); err == nil || u != nil {
			t.Fatalf("immutable change accepted: %v", u)
		}
		if _, err := m.UpdateShare(ctx, owner, idRef(&ocm.ShareId{OpaqueId: "999999"}), webdavTo("viewer")); !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("missing share: %v", err)
		}
	})
}

func receivedGeneration(t *testing.T, m *mgr, id *ocm.ShareId) uint64 {
	t.Helper()
	var row model.OcmReceivedShare
	if err := m.db.Where("id = ?", id.OpaqueId).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row.TransferGeneration
}

func TestUpdateReceivedShareReceipt(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, user := ownerContext(t, "marie")
		rs := storeReceived(t, m, ctx, user, "remote-1")

		accept := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}
		u, err := m.UpdateReceivedShare(ctx, user, accept, stateMask())
		if err != nil || !u.StateChanged() || u.Before.State != ocm.ShareState_SHARE_STATE_PENDING || u.After.State != ocm.ShareState_SHARE_STATE_ACCEPTED {
			t.Fatalf("accept: %v %v", u, err)
		}
		if u.After.GetRemoteShareId() != "remote-1" || u.After.GetCreator().GetIdp() != "provider.example.org" || len(u.After.GetProtocols()) == 0 {
			t.Fatalf("receipt lacks routing data: %v", u.After)
		}
		gen := receivedGeneration(t, m, rs.Id)
		if gen != 1 {
			t.Fatalf("state change generation %d", gen)
		}
		if u, err := m.UpdateReceivedShare(ctx, user, accept, stateMask()); err != nil || u.StateChanged() {
			t.Fatalf("repeat accept: %v %v", u, err)
		}
		hide := &ocm.ReceivedShare{Id: rs.Id, Hidden: true}
		if u, err := m.UpdateReceivedShare(ctx, user, hide, &field_mask.FieldMask{Paths: []string{"hidden"}}); err != nil || u.StateChanged() || !u.After.Hidden {
			t.Fatalf("hidden only: %v %v", u, err)
		}
		if receivedGeneration(t, m, rs.Id) != gen {
			t.Fatal("non-state update moved the generation")
		}
		_, stranger := ownerContext(t, "stranger")
		if _, err := m.UpdateReceivedShare(ctx, stranger, accept, stateMask()); !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("foreign update: %v", err)
		}
		if _, err := m.UpdateReceivedShare(ctx, user, accept, &field_mask.FieldMask{Paths: []string{"name"}}); err == nil {
			t.Fatal("unsupported mask accepted")
		}
	})
}

func TestTransferAttemptLifecycle(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, user := ownerContext(t, "marie")
		rs := storeReceived(t, m, ctx, user, "remote-t")
		hide := &ocm.ReceivedShare{Id: rs.Id, Hidden: true}
		if _, err := m.UpdateReceivedShare(ctx, user, hide, &field_mask.FieldMask{Paths: []string{"hidden"}}); err != nil {
			t.Fatal(err)
		}

		start, err := m.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}, stateMask())
		if err != nil || start.Share.State != ocm.ShareState_SHARE_STATE_TRANSFERRING {
			t.Fatalf("start: %v %v", start, err)
		}
		if _, err := m.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id}, stateMask()); !errors.Is(err, share.ErrTransferInProgress) {
			t.Fatalf("second start: %v", err)
		}
		if u, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation+1, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil || u.StateChanged() {
			t.Fatalf("wrong generation completed: %v %v", u, err)
		}
		u, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED)
		if err != nil || !u.StateChanged() || u.After.State != ocm.ShareState_SHARE_STATE_ACCEPTED {
			t.Fatalf("complete: %v %v", u, err)
		}
		if !u.After.Hidden || len(u.After.Protocols) != len(rs.Protocols) {
			t.Fatalf("completion changed hidden/protocols: %v", u.After)
		}
		if u, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil || u.StateChanged() {
			t.Fatalf("duplicate completion: %v %v", u, err)
		}

		// Reset, start a new attempt, then the old callback arrives.
		reset := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_PENDING}
		if _, err := m.UpdateReceivedShare(ctx, user, reset, stateMask()); err != nil {
			t.Fatal(err)
		}
		second, err := m.StartReceivedShareTransfer(ctx, user, reset, stateMask())
		if err != nil || second.Generation <= start.Generation {
			t.Fatalf("second attempt: %v %v", second, err)
		}
		if u, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_REJECTED); err != nil || u.StateChanged() {
			t.Fatalf("stale callback completed a newer attempt: %v %v", u, err)
		}

		// An ordinary update that sets TRANSFERRING also invalidates the attempt.
		if _, err := m.UpdateReceivedShare(ctx, user, reset, stateMask()); err != nil {
			t.Fatal(err)
		}
		manual := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_TRANSFERRING}
		if _, err := m.UpdateReceivedShare(ctx, user, manual, stateMask()); err != nil {
			t.Fatal(err)
		}
		if u, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, second.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil || u.StateChanged() {
			t.Fatalf("manual TRANSFERRING revived an old attempt: %v %v", u, err)
		}
	})
}

func TestTransferGenerationExhaustionIsRejected(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, user := ownerContext(t, "marie")
		rs := storeReceived(t, m, ctx, user, "remote-x")
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", rs.Id.OpaqueId).
			Update("transfer_generation", uint64(maxTransferGeneration)).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := m.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id}, stateMask()); !errors.Is(err, share.ErrTransferGenerationExhausted) {
			t.Fatalf("start: %v", err)
		}
		accept := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}
		if _, err := m.UpdateReceivedShare(ctx, user, accept, stateMask()); !errors.Is(err, share.ErrTransferGenerationExhausted) {
			t.Fatalf("update: %v", err)
		}
		got, err := m.GetReceivedShare(ctx, user, idRef(rs.Id))
		if err != nil || got.State != ocm.ShareState_SHARE_STATE_PENDING {
			t.Fatalf("state after exhaustion: %v %v", got, err)
		}
	})
}

// Concurrent identical changes produce at most one changed receipt. On
// SQLite a losing writer may fail instead; it never reports a change.
func TestConcurrentSameChangeHasOneReceipt(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, user := ownerContext(t, "marie")
		rs := storeReceived(t, m, ctx, user, "remote-c")
		accept := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}

		var wg sync.WaitGroup
		var mu sync.Mutex
		changed, starts, deletes := 0, 0, 0
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if u, err := m.UpdateReceivedShare(ctx, user, accept, stateMask()); err == nil && u.StateChanged() {
					mu.Lock()
					changed++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		got, err := m.GetReceivedShare(ctx, user, idRef(rs.Id))
		if err != nil {
			t.Fatal(err)
		}
		// A changed receipt exists exactly when the transition was committed.
		committed := got.State == ocm.ShareState_SHARE_STATE_ACCEPTED
		if (committed && changed != 1) || (!committed && changed != 0) {
			t.Fatalf("changed receipts %d, committed %v", changed, committed)
		}

		reset := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_PENDING}
		if _, err := m.UpdateReceivedShare(ctx, user, reset, stateMask()); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := m.StartReceivedShareTransfer(ctx, user, reset, stateMask()); err == nil {
					mu.Lock()
					starts++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if starts > 1 {
			t.Fatalf("concurrent starts %d", starts)
		}

		octx, owner := ownerContext(t, "einstein")
		out := storeOutgoing(t, m, octx, owner, "viewer", "tok-c")
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if r, err := m.DeleteShare(octx, owner, idRef(out.Id)); err == nil && r != nil {
					mu.Lock()
					deletes++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if deletes > 1 {
			t.Fatalf("delete receipts %d", deletes)
		}
	})
}

func TestShareReceiptsCarryStoredResourceType(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, owner := ownerContext(t, "einstein")
		sh := getOcmShare(getOcmAccessMethods("viewer"), getUserOcmShareGrantee("marie"), owner.Id, getResourceId(), "tok-folder")
		id, err := m.GenerateID(ctx)
		if err != nil {
			t.Fatal(err)
		}
		sh.Id = id
		if _, err := m.StoreShare(ctx, sh, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER); err != nil {
			t.Fatal(err)
		}
		editor := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
			AccessMethods: share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), nil, nil),
		}}
		up, err := m.UpdateShare(ctx, owner, idRef(id), editor)
		if err != nil || up.ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER {
			t.Fatalf("update: %v %v", up, err)
		}
		removal, err := m.DeleteShare(ctx, owner, &ocm.ShareReference{Spec: &ocm.ShareReference_Key{Key: &ocm.ShareKey{
			Owner: owner.Id, ResourceId: sh.ResourceId, Grantee: sh.Grantee,
		}}})
		if err != nil || removal.ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER {
			t.Fatalf("delete: %v %v", removal, err)
		}

	})
}

// A share without a supported resource type is never stored, and a corrupt
// row is refused before any change; its type is not guessed.
func TestShareWithoutSupportedResourceTypeIsRefused(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, owner := ownerContext(t, "einstein")
		sh := getOcmShare(getOcmAccessMethods("viewer"), getUserOcmShareGrantee("marie"), owner.Id, getResourceId(), "tok-none")
		id, err := m.GenerateID(ctx)
		if err != nil {
			t.Fatal(err)
		}
		sh.Id = id
		if _, err := m.StoreShare(ctx, sh, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID); !errors.Is(err, share.ErrInvalidResourceType) {
			t.Fatalf("store: %v", err)
		}
		var rows int64
		if err := m.db.Model(&model.OcmShare{}).Count(&rows).Error; err != nil || rows != 0 {
			t.Fatalf("rows %d %v", rows, err)
		}

		for _, itemType := range []string{"", "reference"} {
			corrupt := storeOutgoing(t, m, ctx, owner, "viewer", "tok-"+itemType)
			if err := m.db.Model(&model.OcmShare{}).Where("id = ?", corrupt.Id.OpaqueId).Update("item_type", itemType).Error; err != nil {
				t.Fatal(err)
			}
			var before model.OcmShare
			if err := m.db.Where("id = ?", corrupt.Id.OpaqueId).First(&before).Error; err != nil {
				t.Fatal(err)
			}
			stored, err := m.GetShare(ctx, owner, idRef(corrupt.Id))
			if err != nil {
				t.Fatal(err)
			}
			editor := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
				AccessMethods: share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), nil, nil),
			}}
			if up, err := m.UpdateShare(ctx, owner, idRef(corrupt.Id), editor); !errors.Is(err, share.ErrInvalidResourceType) || up != nil {
				t.Fatalf("%q update: %v %v", itemType, up, err)
			}
			if r, err := m.DeleteShare(ctx, owner, idRef(corrupt.Id)); !errors.Is(err, share.ErrInvalidResourceType) || r != nil {
				t.Fatalf("%q delete: %v %v", itemType, r, err)
			}
			var after model.OcmShare
			if err := m.db.Where("id = ?", corrupt.Id.OpaqueId).First(&after).Error; err != nil || after.Mtime != before.Mtime {
				t.Fatalf("%q row changed: %v %v", itemType, after, err)
			}
			got, err := m.GetShare(ctx, owner, idRef(corrupt.Id))
			if err != nil || !proto.Equal(got, stored) {
				t.Fatalf("%q methods changed: %v %v", itemType, got, err)
			}
		}
	})
}

// Receipts compare the stored model.OcmPermissions, so a request that
// normalizes to the stored value is not a change, and view, read, write and
// share stay distinct.
func TestUpdateShareReceiptUnderTypedPermissions(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, owner := ownerContext(t, "einstein")
		stored := storeOutgoing(t, m, ctx, owner, "viewer", "tok-typed")
		ref := idRef(stored.Id)
		storedBits := func(typ model.OcmProtocol) model.OcmPermissions {
			t.Helper()
			var row model.OcmShareProtocol
			if err := m.db.Where("ocm_share_id = ? AND type = ?", stored.Id.OpaqueId, typ).First(&row).Error; err != nil {
				t.Fatal(err)
			}
			return row.Permissions
		}
		webapp := func(names []string, extra func(*provider.ResourcePermissions)) *ocm.UpdateOCMShareRequest_UpdateField {
			perms := webappCS3Permissions(names)
			if extra != nil {
				extra(perms)
			}
			return &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
				AccessMethods: share.NewWebappAccessMethod(perms, nil),
			}}
		}
		webdav := func(perms *provider.ResourcePermissions) *ocm.UpdateOCMShareRequest_UpdateField {
			return &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
				AccessMethods: share.NewWebDavAccessMethod(perms, nil, nil),
			}}
		}
		versions := func(p *provider.ResourcePermissions) { p.ListFileVersions = true }
		viewerPlus := permissions.NewViewerRole().CS3ResourcePermissions()
		viewerPlus.ListGrants = true

		cases := []struct {
			name    string
			field   *ocm.UpdateOCMShareRequest_UpdateField
			typ     model.OcmProtocol
			changed bool
			bits    model.OcmPermissions
		}{
			{"webapp read with an unstored extra", webapp([]string{"read"}, versions), model.WebappProtocol, false, model.OcmPermissionRead},
			{"webapp view", webapp([]string{"view"}, nil), model.WebappProtocol, true, model.OcmPermissionView},
			{"webapp view repeated", webapp([]string{"view"}, versions), model.WebappProtocol, false, model.OcmPermissionView},
			{"webapp view and write", webapp([]string{"view", "write"}, nil), model.WebappProtocol, true, model.OcmPermissionView | model.OcmPermissionWrite},
			{"webapp read, write and share", webapp([]string{"read", "write", "share"}, nil), model.WebappProtocol, true, model.OcmPermissionRead | model.OcmPermissionWrite | model.OcmPermissionShare},
			{"webapp read, write and share repeated", webapp([]string{"read", "write", "share"}, versions), model.WebappProtocol, false, model.OcmPermissionRead | model.OcmPermissionWrite | model.OcmPermissionShare},
			{"webdav viewer with an unstored extra", webdav(viewerPlus), model.WebDAVProtocol, false, model.OcmPermissionRead},
			{"webdav editor", webdav(permissions.NewEditorRole().CS3ResourcePermissions()), model.WebDAVProtocol, true,
				model.OcmPermissionRead | model.OcmPermissionWrite | model.OcmPermissionCreate | model.OcmPermissionDelete},
		}
		for _, tc := range cases {
			u, err := m.UpdateShare(ctx, owner, ref, tc.field)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			got := u.WebappPermissionsChanged
			if tc.typ == model.WebDAVProtocol {
				got = u.WebDAVPermissionsChanged
			}
			if got != tc.changed || storedBits(tc.typ) != tc.bits {
				t.Fatalf("%s: changed=%v bits=%d, want %v %d", tc.name, got, storedBits(tc.typ), tc.changed, tc.bits)
			}
		}
	})
}
