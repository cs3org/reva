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
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"gorm.io/gorm"
)

func recipient(id string) *userpb.User {
	return &userpb.User{Id: &userpb.UserId{OpaqueId: id, Type: userpb.UserType_USER_TYPE_PRIMARY}}
}

// grantShare is a received share of remoteID sent by a user of origin.
func grantShare(to *userpb.User, origin, remoteID string, rt ocm.SharedResourceType) *ocm.ReceivedShare {
	now := &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Unix())}
	return &ocm.ReceivedShare{
		Name:          "doc",
		RemoteShareId: remoteID,
		Grantee: &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{
			UserId: &userpb.UserId{OpaqueId: to.Id.OpaqueId},
		}},
		// The owner is a different, service-account identity on purpose: it
		// must never be used as the origin.
		Owner:              &userpb.UserId{OpaqueId: "service", Idp: "owner.example.org"},
		Creator:            &userpb.UserId{OpaqueId: "alice", Idp: origin},
		Ctime:              now,
		Mtime:              now,
		RecipientType:      ocm.RecipientType_RECIPIENT_TYPE_USER,
		SharedResourceType: rt,
		State:              ocm.ShareState_SHARE_STATE_PENDING,
		Protocols: []*ocm.Protocol{share.NewWebDAVProtocol("https://"+origin+"/dav/"+remoteID, "secret-"+remoteID,
			&ocm.SharePermissions{Permissions: permissions.NewViewerRole().CS3ResourcePermissions()}, nil, nil)},
	}
}

const file = ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE

func mustStore(t *testing.T, m *mgr, rs *ocm.ReceivedShare) *ocm.ReceivedShare {
	t.Helper()
	stored, err := m.StoreReceivedShare(context.Background(), rs)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func receivedRow(t *testing.T, m *mgr, id string) model.OcmReceivedShare {
	t.Helper()
	var row model.OcmReceivedShare
	if err := m.db.Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func withdrawal(origin, remote string) *share.GrantWithdrawal {
	return &share.GrantWithdrawal{Origin: origin, RemoteShareID: remote, ResourceType: file, RecipientType: ocm.RecipientType_RECIPIENT_TYPE_USER}
}

func assertProjectionCount(t *testing.T, m *mgr, id string, want int64) {
	t.Helper()
	for _, entry := range []struct {
		model  any
		column string
	}{
		{&model.OcmReceivedShare{}, "id"},
		{&model.OcmReceivedShareProtocol{}, "ocm_received_share_id"},
	} {
		var count int64
		err := m.db.Unscoped().Model(entry.model).Where(entry.column+" = ?", id).Count(&count).Error
		if err != nil || count != want {
			t.Fatalf("%T for projection %s: count %d, want %d, err %v", entry.model, id, count, want, err)
		}
	}
}

func TestActiveGrantDeletionRemovesRowsAndProtocols(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx := context.Background()
		if m.db.Migrator().HasTable("ocm_grant_identities") {
			t.Fatal("unexpected grant history table")
		}
		alice := recipient("Alice")
		req := grantShare(alice, "peer.example:8443", "ExactID", file)
		req.Grantee.GetUserId().Idp = "local.example"
		req.Grantee.GetUserId().TenantId = "tenant"
		alice.Id.Idp, alice.Id.TenantId = "local.example", "tenant"
		rs := mustStore(t, m, req)
		duplicateReq := grantShare(recipient("bob"), "PEER.example:8443", "ExactID", file)
		duplicateReq.Expiration = &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(time.Hour).Unix())}
		duplicate := mustStore(t, m, duplicateReq)
		expiredReq := grantShare(recipient("expired"), "peer.example:8443", "ExactID", file)
		expiredReq.Expiration = &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())}
		expired := mustStore(t, m, expiredReq)
		neighbors := []*ocm.ReceivedShare{
			expired,
			mustStore(t, m, grantShare(alice, "peer.example:8444", "ExactID", file)),
			mustStore(t, m, grantShare(alice, "peer.example:8443", "exactid", file)),
			mustStore(t, m, grantShare(alice, "peer.example:8443", "ExactID ", file)),
		}
		for _, wrong := range []*userpb.UserId{
			{OpaqueId: "alice", Idp: "local.example", TenantId: "tenant"},
			{OpaqueId: "Alice", Idp: "other.example", TenantId: "tenant"},
			{OpaqueId: "Alice", Idp: "local.example", TenantId: "other"},
		} {
			u := &userpb.User{Id: wrong}
			if got, err := m.UpdateReceivedShare(ctx, u, rs, stateMask()); !errors.Is(err, share.ErrShareNotFound) || got != nil {
				t.Fatalf("wrong local principal updated a share: %v, %v", got, err)
			}
		}
		if current, err := m.GetReceivedShare(ctx, alice, idRef(rs.Id)); err != nil ||
			current.GetGrantee().GetUserId().GetTenantId() != "tenant" {
			t.Fatalf("resolved recipient binding missing: %v, %v", current, err)
		}

		selector := withdrawal("PEER.example:8443", "ExactID")
		selector.ResourceType = ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER
		if receipt, err := m.WithdrawReceivedGrant(ctx, selector); err != nil || receipt.Changed {
			t.Fatalf("wrong resource type: %v, %v", receipt, err)
		}
		selector.ResourceType = file
		selector.RecipientType = ocm.RecipientType_RECIPIENT_TYPE_GROUP
		if receipt, err := m.WithdrawReceivedGrant(ctx, selector); err != nil || receipt.Changed {
			t.Fatalf("wrong recipient type: %v, %v", receipt, err)
		}
		selector.RecipientType = ocm.RecipientType_RECIPIENT_TYPE_USER
		for i := 0; i < 2; i++ {
			receipt, err := m.WithdrawReceivedGrant(ctx, selector)
			if err != nil || receipt == nil || receipt.Changed != (i == 0) {
				t.Fatalf("withdraw attempt %d = %v, %v", i, receipt, err)
			}
		}
		for _, gone := range []*ocm.ReceivedShare{rs, duplicate} {
			assertProjectionCount(t, m, gone.Id.OpaqueId, 0)
		}
		for _, kept := range neighbors {
			assertProjectionCount(t, m, kept.Id.OpaqueId, 1)
		}
	})
}

func TestFailedWithdrawalCommitsNothing(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		rs := mustStore(t, m, grantShare(recipient("alice"), "peer.example", "42", file))
		if err := m.db.Callback().Delete().After("gorm:delete").Register("test:fail-projection-delete", func(tx *gorm.DB) {
			if tx.Statement.Table == "ocm_received_shares" {
				_ = tx.AddError(errors.New("injected parent delete failure"))
			}
		}); err != nil {
			t.Fatal(err)
		}
		if receipt, err := m.WithdrawReceivedGrant(context.Background(), withdrawal("peer.example", "42")); err == nil || receipt != nil {
			t.Fatalf("failed withdrawal = %v, %v", receipt, err)
		}
		assertProjectionCount(t, m, rs.Id.OpaqueId, 1)
	})
}

func TestActiveGrantDeletionSerializesLifecycle(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		writer, deleter := openOCMManager(t, cfg), openOCMManager(t, cfg)
		alice := recipient("alice")
		rs := mustStore(t, writer, grantShare(alice, "peer.example", "42", file))
		selector := withdrawal("peer.example", "42")
		zero := model.OcmPermissions(0)
		if changed, err := writer.UpdateReceivedGrantPermissions(ctx, selector, &zero, nil); err != nil || !changed {
			t.Fatalf("permission update = %t, %v", changed, err)
		}
		if changed, err := writer.UpdateReceivedGrantPermissions(ctx, selector, &zero, nil); err != nil || changed {
			t.Fatalf("repeated permission update = %t, %v", changed, err)
		}
		start, err := writer.StartReceivedShareTransfer(ctx, alice, rs, stateMask())
		if err != nil {
			t.Fatal(err)
		}
		locked, release := make(chan struct{}), make(chan struct{})
		if err := deleter.db.Callback().Delete().After("gorm:delete").Register("test:hold-delete", func(tx *gorm.DB) {
			if tx.Statement.Table == "ocm_received_share_protocols" {
				close(locked)
				select {
				case <-release:
				case <-ctx.Done():
					_ = tx.AddError(ctx.Err())
				}
			}
		}); err != nil {
			t.Fatal(err)
		}
		deleted := make(chan error, 1)
		go func() {
			_, err := deleter.WithdrawReceivedGrant(ctx, selector)
			deleted <- err
		}()
		select {
		case <-locked:
		case <-ctx.Done():
			t.Fatal("delete did not acquire the projection lock")
		}
		completed := make(chan error, 1)
		go func() {
			_, err := writer.CompleteReceivedShareTransfer(ctx, alice, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED)
			completed <- err
		}()
		select {
		case err := <-completed:
			close(release)
			t.Fatalf("completion escaped the delete lock: %v", err)
		case <-time.After(30 * time.Millisecond):
			close(release)
		}
		if err := <-deleted; err != nil {
			t.Fatal(err)
		}
		if err := <-completed; !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("delayed completion = %v", err)
		}
		reopened := openOCMManager(t, cfg)
		if _, err := reopened.UpdateReceivedShare(ctx, alice, rs, stateMask()); !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("stale lifecycle update = %v", err)
		}
		if changed, err := reopened.UpdateReceivedGrantPermissions(ctx, selector, &zero, nil); err != nil || changed {
			t.Fatalf("stale permission update = %t, %v", changed, err)
		}
		assertProjectionCount(t, reopened, rs.Id.OpaqueId, 0)
		fresh := mustStore(t, reopened, grantShare(alice, "peer.example", "43", file))
		if fresh.Id.OpaqueId == rs.Id.OpaqueId {
			t.Fatal("reused deleted projection ID")
		}
	})
}

func TestActiveGrantCommitUncertaintyReturnsNoReceipt(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		rs := mustStore(t, m, grantShare(recipient("alice"), "peer.example", "42", file))
		fault := &commitFault{apply: true}
		withCommitFault(t, m, fault)
		fault.armed.Store(true)
		selector := withdrawal("peer.example", "42")
		zero := model.OcmPermissions(0)
		changed, err := m.UpdateReceivedGrantPermissions(ctx, selector, &zero, nil)
		var unknown *share.CommitUnknownError
		if changed || !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedRequested {
			t.Fatalf("unacknowledged permission update = %t, %v", changed, err)
		}

		deleter := openOCMManager(t, cfg)
		expiredReq := grantShare(recipient("expired"), "peer.example", "42", file)
		expiredReq.Expiration = &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())}
		expired := mustStore(t, deleter, expiredReq)
		deleteFault := &commitFault{apply: true}
		withCommitFault(t, deleter, deleteFault)
		deleteFault.armed.Store(true)
		receipt, err := deleter.WithdrawReceivedGrant(ctx, selector)
		if receipt != nil || !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedRequested {
			t.Fatalf("unacknowledged deletion = %v, %v", receipt, err)
		}
		reopened := openOCMManager(t, cfg)
		assertProjectionCount(t, reopened, rs.Id.OpaqueId, 0)
		assertProjectionCount(t, reopened, expired.Id.OpaqueId, 1)
	})
}
