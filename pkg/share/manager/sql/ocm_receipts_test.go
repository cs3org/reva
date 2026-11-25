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

package sql

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/go-sql-driver/mysql"
	"github.com/mattn/go-sqlite3"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

func ownerContext(t *testing.T, id string) (context.Context, *userpb.User) {
	t.Helper()
	ctx := getUserContext(id)
	user, _ := appctx.ContextGetUser(ctx)
	user.Id.Idp = "local.example"
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
		if len(removal) != 1 {
			t.Fatalf("got %d receipts, want one", len(removal))
		}
		got := removal[0].Share
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

func TestDeleteShareByIDExactPrincipal(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, owner := ownerContext(t, "einstein")
		_, initiator := ownerContext(t, "delegate")
		for _, tc := range []struct {
			name   string
			caller *userpb.User
		}{{"owner", owner}, {"initiator", initiator}} {
			t.Run(tc.name, func(t *testing.T) {
				grant := getOcmShare(getOcmAccessMethods("viewer"), getUserOcmShareGrantee("marie"), owner.Id, getResourceId(), tc.name)
				grant.Creator = initiator.Id
				stored, err := storeOcmShareWithID(m, ctx, grant)
				if err != nil {
					t.Fatal(err)
				}
				_, differentCase := ownerContext(t, strings.ToUpper(tc.caller.Id.OpaqueId))
				if m.db.Dialector.Name() == "mysql" {
					var candidates int64
					principal := differentCase.Id.OpaqueId
					err := m.db.Model(&model.OcmShare{}).
						Where("id = ? AND (owner = ? OR initiator = ?)", stored.Id.OpaqueId, principal, principal).
						Count(&candidates).Error
					if err != nil || candidates != 1 {
						t.Fatalf("fixture collation matched %d entries, want 1: %v", candidates, err)
					}
				}
				ref := idRef(stored.Id)
				before, err := m.GetShare(ctx, tc.caller, ref)
				if err != nil {
					t.Fatal(err)
				}
				if receipt, err := m.UpdateShare(ctx, differentCase, ref); !errors.Is(err, share.ErrShareNotFound) || receipt != nil {
					t.Fatalf("byte-distinct principal updated a share: %v, %v", receipt, err)
				}
				if receipts, err := m.DeleteShare(ctx, differentCase, ref); !errors.Is(err, share.ErrShareNotFound) || receipts != nil {
					t.Fatalf("byte-distinct principal got %d receipts: %v", len(receipts), err)
				}
				if current, err := m.GetShare(ctx, tc.caller, ref); err != nil || !proto.Equal(current, before) {
					t.Fatalf("refused removal changed the stored row: %v", err)
				}
				receipts, err := m.DeleteShare(ctx, tc.caller, ref)
				if err != nil || len(receipts) != 1 || receipts[0].Share.GetId().GetOpaqueId() != stored.Id.OpaqueId {
					t.Fatalf("exact principal got %d receipts: %v", len(receipts), err)
				}
				if _, err := m.GetShare(ctx, tc.caller, ref); !errors.Is(err, share.ErrShareNotFound) {
					t.Errorf("acknowledged removal left the row active: %v", err)
				}
			})
		}
	})
}

func TestDeleteShareByKeyReceipt(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		m := openOCMManager(t, cfg)
		writer := openOCMManager(t, cfg)
		userCtx, owner := ownerContext(t, "einstein")
		ctx, cancel := context.WithTimeout(userCtx, 10*time.Second)
		defer cancel()
		now := time.Now().UTC().Truncate(time.Millisecond)
		m.db = m.db.Session(&gorm.Session{NowFunc: func() time.Time { return now }})
		deleted := storeOutgoing(t, m, ctx, owner, "viewer", "tok-deleted")
		if _, err := m.DeleteShare(ctx, owner, idRef(deleted.Id)); err != nil {
			t.Fatal(err)
		}
		stored := storeOutgoing(t, m, ctx, owner, "viewer", "tok-k")
		duplicate := storeOutgoing(t, m, ctx, owner, "viewer", "tok-duplicate")
		otherCtx, otherOwner := ownerContext(t, "another-owner")
		survivor := storeOutgoing(t, m, otherCtx, otherOwner, "viewer", "tok-survivor")
		ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Key{Key: &ocm.ShareKey{
			Owner: owner.Id, ResourceId: stored.ResourceId, Grantee: stored.Grantee,
		}}}

		editor := share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), nil, nil)
		field := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{AccessMethods: editor}}
		update, err := m.UpdateShare(ctx, owner, ref, field)
		if err != nil || update == nil || update.After.GetId().GetOpaqueId() != stored.Id.OpaqueId || !update.WebDAVPermissionsChanged {
			t.Fatalf("update did not change the lowest-ID live share: %v %v", update, err)
		}
		got, err := m.GetShare(ctx, owner, idRef(duplicate.Id))
		if err != nil || !proto.Equal(webdavPermissions(got.GetAccessMethods()), webdavPermissions(duplicate.GetAccessMethods())) {
			t.Fatalf("update also changed the duplicate: %v", err)
		}

		const callback = "test:abort_second_ocm_delete"
		abort := errors.New("abort after deleting both rows")
		deletes := 0
		if err := m.db.Callback().Delete().After("gorm:delete").Register(callback, func(tx *gorm.DB) {
			if tx.Error == nil && tx.RowsAffected == 1 {
				deletes++
				if deletes == 2 {
					_ = tx.AddError(abort)
				}
			}
		}); err != nil {
			t.Fatal(err)
		}
		rolledBack, rollbackErr := m.DeleteShare(ctx, owner, ref)
		if err := m.db.Callback().Delete().Remove(callback); err != nil {
			t.Fatal(err)
		}
		if !errors.Is(rollbackErr, abort) || rolledBack != nil || deletes != 2 {
			t.Fatalf("rollback returned %d receipts after %d writes: %v", len(rolledBack), deletes, rollbackErr)
		}
		for _, want := range []*ocm.Share{stored, duplicate} {
			if got, err := m.GetShare(ctx, owner, idRef(want.Id)); err != nil || got.GetToken() != want.Token {
				t.Fatalf("rollback changed share %s: %v", want.GetId().GetOpaqueId(), err)
			}
		}
		deletePool, err := m.db.DB()
		if err != nil {
			t.Fatal(err)
		}
		gate := &transactionGate{DB: deletePool, mode: "commit", reached: make(chan struct{}), release: make(chan struct{})}
		m.db.Statement.ConnPool = gate
		writerPool, err := writer.db.DB()
		if err != nil {
			t.Fatal(err)
		}
		writerPool.SetMaxOpenConns(1)
		sqlite := cfg["db_engine"] == "sqlite"
		setTimeout := "SET SESSION innodb_lock_wait_timeout = 1"
		if sqlite {
			setTimeout = "PRAGMA busy_timeout = 0"
		}
		if _, err := writerPool.ExecContext(ctx, setTimeout); err != nil {
			t.Fatal(err)
		}
		var removals []share.ShareRemoval
		var deleteErr error
		done := make(chan struct{})
		release := sync.OnceFunc(func() { close(gate.release) })
		go func() {
			defer close(done)
			removals, deleteErr = m.DeleteShare(ctx, owner, ref)
		}()
		defer func() { release(); <-done }()
		select {
		case <-gate.reached:
		case <-done:
			t.Fatalf("delete exited before commit: %v", deleteErr)
		case <-ctx.Done():
			t.Fatal("delete did not reach commit")
		}
		up, lockErr := writer.UpdateShare(ctx, owner, ref, field)
		var sqliteErr sqlite3.Error
		var mysqlErr *mysql.MySQLError
		if up != nil || (sqlite && (!errors.As(lockErr, &sqliteErr) || sqliteErr.Code != sqlite3.ErrBusy)) ||
			(!sqlite && (!errors.As(lockErr, &mysqlErr) || mysqlErr.Number != 1205)) {
			t.Fatalf("competing update: %v, %v", up, lockErr)
		}
		t.Log("ordinary update encountered database contention during ordinary delete")
		release()
		<-done
		err = deleteErr
		if err != nil || len(removals) != 2 {
			t.Fatalf("got %d receipts: %v", len(removals), err)
		}
		for i, want := range []*ocm.Share{stored, duplicate} {
			got := removals[i].Share
			if got.GetId().GetOpaqueId() != want.Id.OpaqueId || got.GetToken() != want.Token ||
				len(got.GetAccessMethods()) != 2 || removals[i].ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE {
				t.Errorf("unexpected receipt for share %s", want.Id.OpaqueId)
			}
			if _, err := m.GetShare(ctx, owner, idRef(want.Id)); !errors.Is(err, share.ErrShareNotFound) {
				t.Errorf("share %s remains available: %v", want.Id.OpaqueId, err)
			}
		}
		if updated, err := writer.UpdateShare(ctx, owner, ref, field); updated != nil || !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("update after delete: %v %v", updated, err)
		}
		if removals, err := m.DeleteShare(ctx, owner, ref); !errors.Is(err, share.ErrShareNotFound) || removals != nil {
			t.Errorf("repeat returned %d receipts: %v", len(removals), err)
		}
		if got, err := m.GetShare(otherCtx, otherOwner, idRef(survivor.Id)); err != nil || got.GetToken() != survivor.Token {
			t.Errorf("unrelated share changed: %v %v", got, err)
		}
	})
}

func TestDeleteShareByKeyExactIdentity(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, caller := ownerContext(t, "delegate")
		_, owner := ownerContext(t, "einstein")
		base := getOcmShare(getOcmAccessMethods("viewer"), getUserOcmShareGrantee("marie"), owner.Id, getResourceId(), "exact")
		base.Creator = caller.Id
		base.Grantee.GetUserId().Idp = "Remote.Example.ORG"
		exact, err := storeOcmShareWithID(m, ctx, base)
		if err != nil {
			t.Fatal(err)
		}
		var survivors []*ocm.Share
		for _, variant := range []struct {
			name   string
			change func(*ocm.Share)
		}{
			{"owner", func(s *ocm.Share) { s.Owner.OpaqueId = "Einstein" }},
			{"storage", func(s *ocm.Share) { s.ResourceId.StorageId = strings.ToUpper(s.ResourceId.StorageId) }},
			{"resource", func(s *ocm.Share) { s.ResourceId.OpaqueId = strings.ToUpper(s.ResourceId.OpaqueId) }},
			{"grantee", func(s *ocm.Share) { s.Grantee.GetUserId().OpaqueId = "Marie" }},
			{"initiator", func(s *ocm.Share) { s.Creator.OpaqueId = "Delegate" }},
		} {
			other := proto.CloneOf(base)
			other.Token = variant.name
			variant.change(other)
			stored, err := storeOcmShareWithID(m, ctx, other)
			if err != nil {
				t.Fatal(err)
			}
			survivors = append(survivors, stored)
		}
		readback, err := m.GetShare(ctx, caller, idRef(exact.Id))
		if err != nil || readback.GetGrantee().GetUserId().GetIdp() != "Remote.Example.ORG" {
			t.Fatalf("readback did not preserve the stored grantee host: %v", err)
		}
		key := &ocm.ShareKey{Owner: readback.Owner, ResourceId: readback.ResourceId, Grantee: readback.Grantee}
		if m.db.Dialector.Name() == "mysql" {
			var candidates int64
			err := m.db.Model(&model.OcmShare{}).
				Where("owner = ? AND instance = ? AND inode = ? AND share_with = ? AND (initiator = ? OR owner = ?)",
					key.Owner.OpaqueId, key.ResourceId.StorageId, key.ResourceId.OpaqueId,
					formatUserID(key.Grantee.GetUserId()), caller.Id.OpaqueId, caller.Id.OpaqueId).Count(&candidates).Error
			if err != nil || candidates != int64(1+len(survivors)) {
				t.Fatalf("fixture collation matched %d entries, want %d: %v", candidates, 1+len(survivors), err)
			}
		}
		ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Key{Key: key}}
		wrong := proto.CloneOf(key)
		wrong.Owner.OpaqueId = "eInStEiN"
		wrongRef := &ocm.ShareReference{Spec: &ocm.ShareReference_Key{Key: wrong}}
		if receipt, err := m.UpdateShare(ctx, caller, wrongRef); !errors.Is(err, share.ErrShareNotFound) || receipt != nil {
			t.Fatalf("byte-distinct key updated a share: %v, %v", receipt, err)
		}
		updated, err := m.UpdateShare(ctx, caller, ref)
		if err != nil || updated.After.GetId().GetOpaqueId() != exact.Id.OpaqueId {
			t.Fatalf("update by readback key did not target share %s: %v", exact.Id.OpaqueId, err)
		}
		removed, err := m.DeleteShare(ctx, caller, ref)
		if err != nil || len(removed) != 1 || removed[0].Share.GetId().GetOpaqueId() != exact.Id.OpaqueId {
			t.Fatalf("got %d receipts, want only share %s: %v", len(removed), exact.Id.OpaqueId, err)
		}
		if _, err := m.GetShare(ctx, caller, idRef(exact.Id)); !errors.Is(err, share.ErrShareNotFound) {
			t.Errorf("exact match remains active: %v", err)
		}
		if removed, err := m.DeleteShare(ctx, caller, ref); !errors.Is(err, share.ErrShareNotFound) || removed != nil {
			t.Errorf("repeat returned %d receipts: %v", len(removed), err)
		}
		for _, want := range survivors {
			if got, err := m.GetShare(ctx, &userpb.User{Id: want.Owner}, idRef(want.Id)); err != nil || got.GetToken() != want.Token {
				t.Errorf("nonmatching %s row changed: %v", want.Token, err)
			}
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

		// SQL stores permission masks, so an absent permission pointer and an
		// empty message both represent zero permissions on an existing method.
		for _, newMethod := range []func(*provider.ResourcePermissions) *ocm.AccessMethod{
			func(p *provider.ResourcePermissions) *ocm.AccessMethod {
				return share.NewWebDavAccessMethod(p, nil, nil)
			},
			func(p *provider.ResourcePermissions) *ocm.AccessMethod { return share.NewWebappAccessMethod(p, nil) },
		} {
			for _, perms := range []*provider.ResourcePermissions{nil, {}} {
				u, err := m.UpdateShare(ctx, owner, ref, &ocm.UpdateOCMShareRequest_UpdateField{
					Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{AccessMethods: newMethod(perms)},
				})
				if err != nil || u == nil || u.PermissionsChanged() != (perms == nil) {
					t.Fatalf("zero permission update changed = %t, err = %v", u.PermissionsChanged(), err)
				}
			}
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

func TestGetEmbeddedPayloadRequiresShareID(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, user := ownerContext(t, "marie")
		payload, err := m.GetEmbeddedPayload(ctx, user, &ocm.ReceivedShare{})
		var badRequest errtypes.BadRequest
		if payload != "" || !errors.As(err, &badRequest) {
			t.Fatalf("missing share ID returned %q, %v", payload, err)
		}
	})
}

func TestTransferAttemptLifecycle(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, user := ownerContext(t, "marie")
		_, other := ownerContext(t, "Marie")
		rs := storeReceived(t, m, ctx, user, "remote-t")
		request := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}
		if _, err := m.GetEmbeddedPayload(ctx, other, request); !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("byte-distinct recipient read the share payload: %v", err)
		}
		if receipt, err := m.UpdateReceivedShare(ctx, other, request, stateMask()); !errors.Is(err, share.ErrShareNotFound) || receipt != nil {
			t.Fatalf("byte-distinct recipient updated the share: %v, %v", receipt, err)
		}
		if receipt, err := m.StartReceivedShareTransfer(ctx, other, request, stateMask()); !errors.Is(err, share.ErrShareNotFound) || receipt != nil {
			t.Fatalf("byte-distinct recipient started the transfer: %v, %v", receipt, err)
		}
		hide := &ocm.ReceivedShare{Id: rs.Id, Hidden: true}
		if _, err := m.UpdateReceivedShare(ctx, user, hide, &field_mask.FieldMask{Paths: []string{"hidden"}}); err != nil {
			t.Fatal(err)
		}

		start, err := m.StartReceivedShareTransfer(ctx, user, request, stateMask())
		if err != nil || start.Share.State != ocm.ShareState_SHARE_STATE_TRANSFERRING {
			t.Fatalf("start: %v %v", start, err)
		}
		if _, err := m.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id}, stateMask()); !errors.Is(err, share.ErrTransferInProgress) {
			t.Fatalf("second start: %v", err)
		}
		if receipt, err := m.CompleteReceivedShareTransfer(ctx, other, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED); !errors.Is(err, share.ErrShareNotFound) || receipt != nil {
			t.Fatalf("byte-distinct recipient completed the transfer: %v, %v", receipt, err)
		}
		if u, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation+1, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil || u == nil || !proto.Equal(u.Before, start.Share) || !proto.Equal(u.After, start.Share) {
			t.Fatalf("wrong generation completed: %v %v", u, err)
		}
		if got := receivedGeneration(t, m, rs.Id); got != start.Generation {
			t.Fatalf("wrong generation completion changed generation to %d, want %d", got, start.Generation)
		}
		const beforeMtime = 946684800
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", rs.Id.OpaqueId).
			Update("mtime", beforeMtime).Error; err != nil {
			t.Fatal(err)
		}
		u, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED)
		if err != nil || !u.StateChanged() || u.After.State != ocm.ShareState_SHARE_STATE_ACCEPTED {
			t.Fatalf("complete: %v %v", u, err)
		}
		if u.Before.GetMtime().GetSeconds() != beforeMtime || u.After.GetMtime().GetSeconds() == beforeMtime {
			t.Fatalf("completion timestamps: before %v, after %v", u.Before.GetMtime(), u.After.GetMtime())
		}
		stored, err := m.GetReceivedShare(ctx, user, idRef(rs.Id))
		if err != nil || !proto.Equal(stored.GetMtime(), u.After.GetMtime()) {
			t.Fatalf("completion did not return the stored mtime: %v %v", stored.GetMtime(), err)
		}
		if !u.After.Hidden || len(u.After.Protocols) != len(rs.Protocols) {
			t.Fatalf("completion changed hidden/protocols: %v", u.After)
		}
		if duplicate, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil || duplicate == nil || !proto.Equal(duplicate.Before, u.After) || !proto.Equal(duplicate.After, u.After) {
			t.Fatalf("duplicate completion: %v %v", duplicate, err)
		}
		if got := receivedGeneration(t, m, rs.Id); got != start.Generation {
			t.Fatalf("duplicate completion changed generation to %d, want %d", got, start.Generation)
		}
		if _, err := m.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id}, stateMask()); !errors.Is(err, share.ErrShareAlreadyAccepted) {
			t.Fatalf("start after acceptance: %v", err)
		}
		if got := receivedGeneration(t, m, rs.Id); got != start.Generation {
			t.Fatalf("refused start changed the generation to %d", got)
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
		if u, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_REJECTED); err != nil || u == nil || !proto.Equal(u.Before, second.Share) || !proto.Equal(u.After, second.Share) {
			t.Fatalf("stale callback completed a newer attempt: %v %v", u, err)
		}
		if got := receivedGeneration(t, m, rs.Id); got != second.Generation {
			t.Fatalf("stale callback changed generation to %d, want %d", got, second.Generation)
		}

		// An ordinary update that sets TRANSFERRING also invalidates the attempt.
		if _, err := m.UpdateReceivedShare(ctx, user, reset, stateMask()); err != nil {
			t.Fatal(err)
		}
		manual := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_TRANSFERRING}
		manualUpdate, err := m.UpdateReceivedShare(ctx, user, manual, stateMask())
		if err != nil {
			t.Fatal(err)
		}
		gen := receivedGeneration(t, m, rs.Id)
		if u, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, second.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil || u == nil || !proto.Equal(u.Before, manualUpdate.After) || !proto.Equal(u.After, manualUpdate.After) {
			t.Fatalf("manual TRANSFERRING revived an old attempt: %v %v", u, err)
		}
		if got := receivedGeneration(t, m, rs.Id); got != gen {
			t.Fatalf("manual TRANSFERRING completion changed generation to %d, want %d", got, gen)
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
		if err != nil || len(removal) != 1 || removal[0].ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER {
			t.Fatalf("delete: %v %v", removal, err)
		}

	})
}

func TestShareResourceTypeAdmissionAndExistingMutations(t *testing.T) {
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
			if itemType == "" {
				ref := idRef(corrupt.Id)
				up, err := m.UpdateShare(ctx, owner, ref, editor)
				if err != nil || up == nil || !up.WebDAVPermissionsChanged ||
					up.ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID {
					t.Errorf("update of prior-format row: %v %v", up, err)
				} else {
					got, err := m.GetShare(ctx, owner, ref)
					if err != nil || !proto.Equal(webdavPermissions(got.GetAccessMethods()), editor.GetAccessMethods().GetWebdavOptions().GetPermissions()) {
						t.Errorf("permissions not persisted: %v %v", got, err)
					}
					var persisted model.OcmShare
					if err := m.db.First(&persisted, before.Id).Error; err != nil || persisted.ItemType != "" {
						t.Errorf("stored type changed: %q %v", persisted.ItemType, err)
					}
				}
				removed, err := m.DeleteShare(ctx, owner, ref)
				if err != nil || len(removed) != 1 || removed[0].ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID ||
					removed[0].Share.GetId().GetOpaqueId() != corrupt.Id.OpaqueId {
					t.Errorf("withdrawal of prior-format row: %v %v", removed, err)
				}
				if _, err := m.GetShare(ctx, owner, ref); !errors.Is(err, share.ErrShareNotFound) {
					t.Errorf("withdrawn share remains available: %v", err)
				}
				continue
			}
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

// Two concurrent acceptances on separate connections: one commits and returns
// the only changed receipt, the other changes nothing.
func TestConcurrentAcceptanceHasOneWinner(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx, user := ownerContext(t, "marie")
		rs := storeReceived(t, m, ctx, user, "remote-w")
		accept := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}

		type result struct {
			u   *share.ReceivedShareUpdate
			err error
		}
		results := make(chan result, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			go func() {
				<-start
				u, err := m.UpdateReceivedShare(ctx, user, accept, stateMask())
				results <- result{u, err}
			}()
		}
		close(start)
		winners := 0
		for i := 0; i < 2; i++ {
			r := <-results
			if r.err == nil && r.u.StateChanged() {
				winners++
			}
		}
		got, err := m.GetReceivedShare(ctx, user, idRef(rs.Id))
		if err != nil {
			t.Fatal(err)
		}
		if winners != 1 || got.State != ocm.ShareState_SHARE_STATE_ACCEPTED {
			t.Fatalf("winners %d, state %v", winners, got.State)
		}
		if gen := receivedGeneration(t, m, rs.Id); gen != 1 {
			t.Fatalf("generation %d, want one committed change", gen)
		}
	})
}
