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

package sql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/mattn/go-sqlite3"
	"google.golang.org/genproto/protobuf/field_mask"
	"gorm.io/gorm"
)

func TestDirectedActionsRespectExactIdentities(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		alice := &userpb.UserId{OpaqueId: "00012", Idp: "Directory", TenantId: "Tenant", Type: userpb.UserType_USER_TYPE_PRIMARY}
		otherDirectory := &userpb.UserId{OpaqueId: "00012", Idp: "directory", TenantId: "Tenant", Type: userpb.UserType_USER_TYPE_PRIMARY}
		otherSpace := &userpb.UserId{OpaqueId: "00012", Idp: "Directory ", TenantId: "Tenant", Type: userpb.UserType_USER_TYPE_PRIMARY}
		otherOpaqueSpace := &userpb.UserId{OpaqueId: "00012 ", Idp: "Directory", TenantId: "Tenant", Type: userpb.UserType_USER_TYPE_PRIMARY}
		otherType := &userpb.UserId{OpaqueId: "00012", Idp: "Directory", TenantId: "Tenant", Type: userpb.UserType_USER_TYPE_FEDERATED}
		otherTenant := &userpb.UserId{OpaqueId: "00012", Idp: "Directory", TenantId: "tenant", Type: userpb.UserType_USER_TYPE_PRIMARY}
		bob := &userpb.UserId{OpaqueId: "bob", Idp: "remote.example.org:8443", Type: userpb.UserType_USER_TYPE_FEDERATED}
		otherHost := &userpb.UserId{OpaqueId: "bob", Idp: "remote.example.org", Type: userpb.UserType_USER_TYPE_FEDERATED}
		store := func(creator, remote *userpb.UserId, token, resource string) *ocm.Share {
			t.Helper()
			grantee := &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: remote}}
			s := getOcmShare(getOcmAccessMethods("viewer"), grantee, creator, &provider.ResourceId{StorageId: "storage", OpaqueId: resource}, token)
			s.RecipientType = ocm.RecipientType_RECIPIENT_TYPE_USER
			id, err := m.GenerateID(ctx)
			if err != nil {
				t.Fatal(err)
			}
			s.Id = id
			if _, err := m.StoreShare(ctx, s, file); err != nil {
				t.Fatal(err)
			}
			return s
		}
		aliceUser := &userpb.User{Id: alice}
		upperHostBob := &userpb.UserId{OpaqueId: "bob", Idp: "REMOTE.example.org:8443", Type: bob.Type}
		cutoff := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		clock := cutoff
		m.db = m.db.Session(&gorm.Session{NowFunc: func() time.Time { return clock }})
		prior := store(alice, upperHostBob, "prior", "z-shared")
		if _, err := m.DeleteShare(ctx, aliceUser, idRef(prior.Id)); err != nil {
			t.Fatal(err)
		}
		clock = cutoff.Add(time.Second)
		otherPrior := store(alice, bob, "other-prior", "a-other")
		if _, err := m.DeleteShare(ctx, aliceUser, idRef(otherPrior.Id)); err != nil {
			t.Fatal(err)
		}
		clock = cutoff
		selected := store(alice, upperHostBob, "selected", "z-shared")
		secondKey := store(alice, bob, "second-key", "a-other")
		duplicate := store(alice, upperHostBob, "duplicate", "z-shared")
		wrongDirectory := store(otherDirectory, upperHostBob, "other-directory", "z-shared")
		wrongSpace := store(otherSpace, bob, "other-space", "other-space")
		wrongOpaqueSpace := store(otherOpaqueSpace, bob, "other-opaque-space", "other-opaque-space")
		wrongType := store(otherType, bob, "other-type", "other-type")
		wrongHost := store(alice, otherHost, "other-host", "other-host")
		wrongTenant := store(otherTenant, bob, "other-tenant", "other-tenant")
		unboundOutgoing := store(alice, bob, "unbound", "unbound")
		if err := m.db.Model(&model.OcmShare{}).Where("id = ?", unboundOutgoing.Id.OpaqueId).
			Update("creator_user_type", userpb.UserType_USER_TYPE_INVALID).Error; err != nil {
			t.Fatal(err)
		}
		group := store(alice, bob, "group", "group")
		if err := m.db.Model(&model.OcmShare{}).Where("id = ?", group.Id.OpaqueId).Update("recipient_type", model.OcmShareTypeGroup).Error; err != nil {
			t.Fatal(err)
		}
		const callback = "test:abort-second-directed-delete"
		abort := errors.New("abort after second directed delete")
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
		rolledBack, err := m.RemoveOCMSharesForRecipient(ctx, alice, bob)
		if removeErr := m.db.Callback().Delete().Remove(callback); removeErr != nil {
			t.Fatal(removeErr)
		}
		if !errors.Is(err, abort) || rolledBack != nil || deletes != 2 {
			t.Fatalf("rollback returned %d receipts after %d writes: %v", len(rolledBack), deletes, err)
		}
		for _, grant := range []*ocm.Share{selected, secondKey, duplicate} {
			if _, err := m.GetShare(ctx, aliceUser, idRef(grant.Id)); err != nil {
				t.Fatalf("grant %s unavailable after rollback: %v", grant.Id.OpaqueId, err)
			}
		}
		removed, err := m.RemoveOCMSharesForRecipient(ctx, alice, bob)
		want := []struct {
			grant     *ocm.Share
			deletedAt time.Time
		}{
			{selected, cutoff.Add(time.Millisecond)},
			{secondKey, cutoff.Add(time.Second + time.Millisecond)},
			{duplicate, cutoff.Add(2 * time.Millisecond)},
		}
		if err != nil || len(removed) != len(want) {
			t.Fatalf("withdraw receipts %v: %v", removed, err)
		}
		for i, entry := range want {
			id := entry.grant.Id
			if receipt := removed[i]; receipt.ResourceType != file || receipt.Share.Id.OpaqueId != id.OpaqueId || receipt.Share.Token != entry.grant.Token {
				t.Errorf("receipt %d does not describe grant %s", i, id.OpaqueId)
			}
			var row model.OcmShare
			if err := m.db.Unscoped().Where("id = ?", id.OpaqueId).Take(&row).Error; err != nil ||
				!row.DeletedAt.Valid || !row.DeletedAt.Time.Equal(entry.deletedAt) {
				t.Errorf("grant %s deletion timestamp %v, want %v: %v", id.OpaqueId, row.DeletedAt, entry.deletedAt, err)
			}
			if _, err := m.GetShare(ctx, aliceUser, idRef(id)); !errors.Is(err, share.ErrShareNotFound) {
				t.Errorf("withdrawn grant %s lookup = %v", id.OpaqueId, err)
			}
		}
		if repeated, err := m.RemoveOCMSharesForRecipient(ctx, alice, bob); err != nil || len(repeated) != 0 {
			t.Fatalf("repeat withdrawal receipts %v: %v", repeated, err)
		}
		for _, survivor := range []*ocm.Share{wrongDirectory, wrongSpace, wrongOpaqueSpace, wrongType, wrongHost, wrongTenant, unboundOutgoing} {
			if _, err := m.GetShare(ctx, &userpb.User{Id: survivor.Creator}, idRef(survivor.Id)); err != nil {
				t.Fatalf("unrelated grant unavailable: %v", err)
			}
		}
		var remainingGroup int64
		if err := m.db.Model(&model.OcmShare{}).Where("id = ?", group.Id.OpaqueId).Count(&remainingGroup).Error; err != nil || remainingGroup != 1 {
			t.Fatalf("group grant was changed: count %d: %v", remainingGroup, err)
		}
		incoming := func(local *userpb.UserId, remote *userpb.UserId, id string) *ocm.ReceivedShare {
			t.Helper()
			s := grantShare(&userpb.User{Id: local}, remote.Idp, id, file)
			s.Creator = remote
			return mustStore(t, m, s)
		}
		chosen := incoming(alice, bob, "001.0")
		initial, err := m.GetReceivedShare(ctx, aliceUser, idRef(chosen.Id))
		if err != nil || initial.GetRemoteShareId() != "001.0" || initial.GetGrantee().GetUserId().GetOpaqueId() != "00012" {
			t.Fatalf("numeric-looking identities changed on read: %v %v", initial, err)
		}
		other := incoming(otherDirectory, bob, "incoming-two")
		tenantNeighbor := incoming(otherTenant, bob, "incoming-other-tenant")
		caseSender := &userpb.UserId{OpaqueId: "BOB", Idp: bob.Idp, Type: bob.Type}
		senderNeighbor := incoming(alice, caseSender, "incoming-other-sender")
		if _, err := m.StartReceivedShareTransfer(ctx, aliceUser, senderNeighbor, stateMask()); err != nil {
			t.Fatal(err)
		}
		portNeighbor := incoming(alice, otherHost, "incoming-other-port")
		unboundIncoming := incoming(alice, bob, "incoming-unbound")
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", unboundIncoming.Id.OpaqueId).
			Update("recipient_user_type", userpb.UserType_USER_TYPE_INVALID).Error; err != nil {
			t.Fatal(err)
		}
		transfer, err := m.StartReceivedShareTransfer(ctx, aliceUser, chosen, stateMask())
		if err != nil {
			t.Fatal(err)
		}
		completed, err := m.CompleteReceivedShareTransfer(ctx, aliceUser, chosen.Id, transfer.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED)
		if err != nil || !completed.StateChanged() {
			t.Fatalf("complete before directed decline: %v %v", completed, err)
		}
		declined, err := m.RejectReceivedOCMSharesFromSender(ctx, alice, bob)
		if err != nil || len(declined) != 1 || !declined[0].StateChanged() {
			t.Fatalf("decline receipts %v: %v", declined, err)
		}
		got, err := m.GetReceivedShare(ctx, &userpb.User{Id: alice}, idRef(chosen.Id))
		if err != nil || got.State != ocm.ShareState_SHARE_STATE_REJECTED || !got.Hidden {
			t.Fatalf("declined share: %v %v", got, err)
		}
		if _, err := m.GetReceivedShare(ctx, &userpb.User{Id: alice}, idRef(other.Id)); err == nil {
			t.Fatal("same opaque ID in another directory was visible")
		}
		got, err = m.GetReceivedShare(ctx, &userpb.User{Id: otherDirectory}, idRef(other.Id))
		if err != nil || got.State != ocm.ShareState_SHARE_STATE_PENDING {
			t.Fatalf("other projection changed: %v %v", got, err)
		}
		completion, err := m.CompleteReceivedShareTransfer(ctx, aliceUser, chosen.Id, transfer.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED)
		if err != nil || completion.StateChanged() {
			t.Fatalf("late transfer changed rejected projection: %v %v", completion, err)
		}
		for _, neighbor := range []struct {
			user  *userpb.UserId
			share *ocm.ReceivedShare
		}{{otherTenant, tenantNeighbor}, {alice, portNeighbor}, {alice, unboundIncoming}} {
			got, err := m.GetReceivedShare(ctx, &userpb.User{Id: neighbor.user}, idRef(neighbor.share.Id))
			if err != nil || got.GetState() != ocm.ShareState_SHARE_STATE_PENDING {
				t.Fatalf("unrelated projection changed: %v %v", got, err)
			}
		}
		got, err = m.GetReceivedShare(ctx, aliceUser, idRef(senderNeighbor.Id))
		if err != nil || got.GetState() != ocm.ShareState_SHARE_STATE_TRANSFERRING {
			t.Fatalf("copy belonging to a case-distinct sender changed: %v %v", got, err)
		}
		repeated, err := m.RejectReceivedOCMSharesFromSender(ctx, alice, bob)
		if err != nil || len(repeated) != 0 {
			t.Fatalf("repeat decline receipts %v: %v", repeated, err)
		}
		chosen.State = ocm.ShareState_SHARE_STATE_ACCEPTED
		if _, err := m.UpdateReceivedShare(ctx, &userpb.User{Id: alice}, chosen, &field_mask.FieldMask{Paths: []string{"state"}}); err != nil {
			t.Fatalf("explicit reaccept: %v", err)
		}
	})
}

func TestDirectedDeclineRefusesLiveTransfer(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx := context.Background()
		alice := recipient("alice")
		bob := &userpb.UserId{OpaqueId: "bob", Idp: "remote.example.org", Type: userpb.UserType_USER_TYPE_FEDERATED}
		pending := grantShare(alice, bob.Idp, "pending", file)
		pending.Creator = bob
		pending = mustStore(t, m, pending)
		copying := grantShare(alice, bob.Idp, "copying", ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED)
		copying.Creator = bob
		copying.Protocols = append(copying.Protocols, share.NewEmbeddedProtocol(`{"@graph":[]}`))
		copying = mustStore(t, m, copying)
		transfer, err := m.StartReceivedShareTransfer(ctx, alice, copying, stateMask())
		if err != nil {
			t.Fatal(err)
		}

		updates, err := m.RejectReceivedOCMSharesFromSender(ctx, alice.Id, bob)
		if updates != nil || !errors.Is(err, share.ErrTransferInProgress) {
			t.Fatalf("directed decline returned %d receipts and %v", len(updates), err)
		}
		if got := receivedRow(t, m, pending.Id.OpaqueId); got.State != model.OcmShareStatePending || got.Hidden {
			t.Fatalf("pending grant changed: state %v, hidden %t", got.State, got.Hidden)
		}
		if got := receivedRow(t, m, copying.Id.OpaqueId); got.State != model.OcmShareStateTransferring ||
			got.Hidden || got.TransferGeneration != transfer.Generation {
			t.Fatalf("copy reservation changed: state %v, hidden %t, generation %d", got.State, got.Hidden, got.TransferGeneration)
		}
		if _, err := m.StartReceivedShareTransfer(ctx, alice, copying, stateMask()); !errors.Is(err, share.ErrTransferInProgress) {
			t.Fatalf("second transfer start: %v", err)
		}
		completion, err := m.CompleteReceivedShareTransfer(ctx, alice, copying.Id, transfer.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED)
		if err != nil || !completion.StateChanged() {
			t.Fatalf("complete original transfer: %v, %v", completion, err)
		}
		updates, err = m.RejectReceivedOCMSharesFromSender(ctx, alice.Id, bob)
		if err != nil || len(updates) != 2 ||
			updates[0].After.GetId().GetOpaqueId() != pending.Id.OpaqueId ||
			updates[1].After.GetId().GetOpaqueId() != copying.Id.OpaqueId {
			t.Fatalf("explicit decline after completion: %v, %v", updates, err)
		}
		for _, update := range updates {
			if !update.StateChanged() || !update.After.GetHidden() || update.After.GetState() != ocm.ShareState_SHARE_STATE_REJECTED {
				t.Fatalf("invalid decline receipt: %v", update)
			}
		}
	})
}

func TestDirectedDeclineRefusesCorruptBinding(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx := context.Background()
		alice := recipient("alice")
		bob := &userpb.UserId{OpaqueId: "bob", Idp: "remote.example.org"}
		for _, id := range []string{"valid", "corrupt"} {
			rs := grantShare(alice, bob.Idp, id, file)
			rs.Creator = bob
			mustStore(t, m, rs)
		}
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("remote_share_id = ?", "corrupt").
			Update("grant_key", strings.Repeat("0", 64)).Error; err != nil {
			t.Fatal(err)
		}
		updates, err := m.RejectReceivedOCMSharesFromSender(ctx, alice.Id, bob)
		if updates != nil || !errors.Is(err, share.ErrGrantIdentityMismatch) {
			t.Fatalf("directed decline of corrupt selection: %d receipts, %v", len(updates), err)
		}
		var pending int64
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("state = ? AND hidden = ?", model.OcmShareStatePending, false).
			Count(&pending).Error; err != nil || pending != 2 {
			t.Fatalf("pending projections after refusal: %d, %v", pending, err)
		}
	})
}

func TestDirectedBatchesAcrossBindBoundary(t *testing.T) {
	const limit = 500
	for _, direction := range []string{"outgoing", "incoming"} {
		t.Run(direction, func(t *testing.T) {
			forEachDialect(t, func(t *testing.T, m *mgr) {
				ctx := context.Background()
				sqlite := m.db.Dialector.Name() == "sqlite"
				if !sqlite && direction == "incoming" {
					t.Skip("SQLite enforces the reduced query limit")
				}
				cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				m.db = m.db.Session(&gorm.Session{NowFunc: func() time.Time { return cutoff }})
				alice := recipient("alice")
				bob := &userpb.UserId{OpaqueId: "bob", Idp: "remote.example.org", Type: userpb.UserType_USER_TYPE_FEDERATED}
				for i := range limit + 1 {
					token := fmt.Sprintf("grant-%d", i)
					if direction == "incoming" {
						s := grantShare(alice, bob.Idp, token, file)
						s.Creator = bob
						mustStore(t, m, s)
						continue
					}
					grantee := &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: bob}}
					resource := token
					switch i {
					case 0:
						resource = "same-resource"
					case limit:
						resource = "same-resource"
						if !sqlite {
							resource = "SAME-resource"
						}
					}
					s := getOcmShare(nil, grantee, alice.Id, &provider.ResourceId{StorageId: "storage", OpaqueId: resource}, token)
					s.RecipientType = ocm.RecipientType_RECIPIENT_TYPE_USER
					id, err := m.GenerateID(ctx)
					if err != nil {
						t.Fatal(err)
					}
					s.Id = id
					if _, err := m.StoreShare(ctx, s, file); err != nil {
						t.Fatal(err)
					}
				}
				fault := &commitFault{apply: true}
				withCommitFault(t, m, fault)
				if sqlite {
					fault.DB.SetMaxOpenConns(1)
					conn, err := fault.DB.Conn(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if err := conn.Raw(func(driverConn any) error {
						driverConn.(*sqlite3.SQLiteConn).SetLimit(sqlite3.SQLITE_LIMIT_VARIABLE_NUMBER, limit)
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					if err := conn.Close(); err != nil {
						t.Fatal(err)
					}
				}
				fault.armed.Store(true)
				var err error
				if direction == "incoming" {
					receipts, actionErr := m.RejectReceivedOCMSharesFromSender(ctx, alice.Id, bob)
					err = actionErr
					if receipts != nil {
						t.Fatalf("unknown commit returned %d incoming receipts", len(receipts))
					}
				} else {
					receipts, actionErr := m.RemoveOCMSharesForRecipient(ctx, alice.Id, bob)
					err = actionErr
					if receipts != nil {
						t.Fatalf("unknown commit returned %d outgoing receipts", len(receipts))
					}
				}
				var unknown *share.CommitUnknownError
				if !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedRequested || fault.reads.Load() != 2 {
					t.Fatalf("got %v after %d observation reads, want requested state after two bounded reads", err, fault.reads.Load())
				}
				if direction == "outgoing" {
					for i, token := range []string{"grant-0", fmt.Sprintf("grant-%d", limit)} {
						var row model.OcmShare
						if err := m.db.Unscoped().Where("token = ?", token).Take(&row).Error; err != nil {
							t.Fatal(err)
						}
						wantTime := cutoff.Add(time.Duration(i) * time.Millisecond)
						if !row.DeletedAt.Valid || !row.DeletedAt.Time.Equal(wantTime) {
							t.Fatalf("collated tuple %q deleted at %v, want %v", row.Inode, row.DeletedAt, wantTime)
						}
					}
				}
			})
		})
	}
}

func TestDirectedWithdrawalUnknownCommitObservesCapturedGrant(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		writer := openOCMManager(t, cfg)
		alice := &userpb.UserId{OpaqueId: "alice", Idp: "directory", Type: userpb.UserType_USER_TYPE_PRIMARY}
		bob := &userpb.UserId{OpaqueId: "bob", Idp: "remote.example.org", Type: userpb.UserType_USER_TYPE_FEDERATED}
		store := func(manager *mgr, token string) *ocm.Share {
			t.Helper()
			grantee := &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: bob}}
			s := getOcmShare(getOcmAccessMethods("viewer"), grantee, alice, getResourceId(), token)
			s.RecipientType = ocm.RecipientType_RECIPIENT_TYPE_USER
			id, err := manager.GenerateID(ctx)
			if err != nil {
				t.Fatal(err)
			}
			s.Id = id
			if _, err := manager.StoreShare(ctx, s, file); err != nil {
				t.Fatal(err)
			}
			return s
		}
		captured := []*ocm.Share{store(m, "captured"), store(m, "captured-duplicate")}
		var later *ocm.Share
		fault := &commitFault{apply: true, between: func() { later = store(writer, "later") }}
		withCommitFault(t, m, fault)
		fault.armed.Store(true)
		removed, err := m.RemoveOCMSharesForRecipient(ctx, alice, bob)
		var unknown *share.CommitUnknownError
		if removed != nil || !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedRequested {
			t.Fatalf("receipts %v, unknown commit %v", removed, err)
		}
		owner := &userpb.User{Id: alice}
		for _, grant := range captured {
			if _, err := writer.GetShare(ctx, owner, idRef(grant.Id)); !errors.Is(err, share.ErrShareNotFound) {
				t.Errorf("captured grant %s remains available: %v", grant.Id.OpaqueId, err)
			}
		}
		if later == nil {
			t.Fatal("later grant was not stored")
		}
		if got, err := writer.GetShare(ctx, owner, idRef(later.Id)); err != nil || got.GetToken() != later.Token {
			t.Fatalf("uncaptured grant unavailable: %v", err)
		}
	})
}

func TestDirectedDeclineUnknownCommitIgnoresEarlierRejection(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		alice := &userpb.UserId{OpaqueId: "alice", Idp: "directory", Type: userpb.UserType_USER_TYPE_PRIMARY}
		bob := &userpb.UserId{OpaqueId: "bob", Idp: "remote.example.org", Type: userpb.UserType_USER_TYPE_FEDERATED}
		store := func(id string) *ocm.ReceivedShare {
			s := grantShare(&userpb.User{Id: alice}, bob.Idp, id, file)
			s.Creator = bob
			return mustStore(t, m, s)
		}
		already := store("already-rejected")
		already.State = ocm.ShareState_SHARE_STATE_REJECTED
		if _, err := m.UpdateReceivedShare(ctx, &userpb.User{Id: alice}, already, &field_mask.FieldMask{Paths: []string{"state"}}); err != nil {
			t.Fatal(err)
		}
		store("pending")
		fault := &commitFault{}
		withCommitFault(t, m, fault)
		fault.armed.Store(true)
		receipts, err := m.RejectReceivedOCMSharesFromSender(ctx, alice, bob)
		var unknown *share.CommitUnknownError
		if receipts != nil || !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedPrevious {
			t.Fatalf("receipts %v, unknown commit %v", receipts, err)
		}
	})
}

func TestDirectedDeclineUnknownCommitObservesAppliedGeneration(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		reader := openOCMManager(t, cfg)
		alice := &userpb.UserId{OpaqueId: "alice", Idp: "directory", Type: userpb.UserType_USER_TYPE_PRIMARY}
		bob := &userpb.UserId{OpaqueId: "bob", Idp: "remote.example.org", Type: userpb.UserType_USER_TYPE_FEDERATED}
		s := grantShare(&userpb.User{Id: alice}, bob.Idp, "pending", file)
		s.Creator = bob
		pending := mustStore(t, m, s)
		const appliedGeneration = uint64(9)
		// Persist a distinct generation to exercise the observer's readback.
		if err := m.db.Callback().Update().Before("gorm:update").Register("test:applied-decline-generation", func(tx *gorm.DB) {
			fields, ok := tx.Statement.Dest.(map[string]any)
			if !ok || fields["transfer_generation"] == nil {
				_ = tx.AddError(errors.New("decline did not write a generation"))
				return
			}
			fields["transfer_generation"] = appliedGeneration
		}); err != nil {
			t.Fatal(err)
		}
		fault := &commitFault{apply: true}
		withCommitFault(t, m, fault)
		fault.armed.Store(true)
		receipts, err := m.RejectReceivedOCMSharesFromSender(ctx, alice, bob)
		row := receivedRow(t, reader, pending.Id.OpaqueId)
		if row.State != model.OcmShareStateRejected || !row.Hidden || row.TransferGeneration != appliedGeneration {
			t.Fatalf("stored decline state %v, hidden %t, generation %d", row.State, row.Hidden, row.TransferGeneration)
		}
		var unknown *share.CommitUnknownError
		if receipts != nil || !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedRequested {
			t.Fatalf("receipts %v, unknown commit %v", receipts, err)
		}
	})
}

func TestEmbeddedPayloadRequiresExactRecipient(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		alice := &userpb.User{Id: &userpb.UserId{OpaqueId: "alice", Idp: "directory", Type: userpb.UserType_USER_TYPE_PRIMARY}}
		other := &userpb.User{Id: &userpb.UserId{OpaqueId: "alice", Idp: "other", Type: userpb.UserType_USER_TYPE_PRIMARY}}
		s := grantShare(alice, "remote.example.org", "embedded", ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED)
		s.Protocols = append(s.Protocols, share.NewEmbeddedProtocol(`{"@graph":[]}`))
		stored := mustStore(t, m, s)
		if payload, err := m.GetEmbeddedPayload(ctx, other, stored); err == nil || payload != "" {
			t.Fatalf("other recipient received payload: %q %v", payload, err)
		}
		payload, err := m.GetEmbeddedPayload(ctx, alice, stored)
		if err != nil {
			t.Fatalf("owner payload: %v", err)
		}
		var decoded map[string][]any
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil || len(decoded) != 1 || decoded["@graph"] == nil || len(decoded["@graph"]) != 0 {
			t.Fatalf("unexpected owner payload: %q %v", payload, err)
		}
	})
}

func TestOutgoingOptionalBindingPreservesOrdinaryAdmission(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		creator := &userpb.UserId{
			OpaqueId: "alice", Idp: strings.Repeat("d", 1020),
			TenantId: strings.Repeat("t", 1020), Type: userpb.UserType_USER_TYPE_PRIMARY,
		}
		const host = "remote.example.org"
		remote := &userpb.UserId{
			OpaqueId: strings.Repeat("𐍈", 255-1-len(host)), Idp: host, Type: userpb.UserType_USER_TYPE_FEDERATED,
		}
		store := func(c, r *userpb.UserId, token string) (*ocm.Share, error) {
			g := &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: r}}
			s := getOcmShare(getOcmAccessMethods("viewer"), g, c, &provider.ResourceId{StorageId: "storage", OpaqueId: token}, token)
			s.RecipientType = ocm.RecipientType_RECIPIENT_TYPE_USER
			id, err := m.GenerateID(ctx)
			if err != nil {
				t.Fatal(err)
			}
			s.Id = id
			return m.StoreShare(ctx, s, file)
		}
		stored, err := store(creator, remote, "boundary")
		if err != nil {
			t.Fatal(err)
		}
		got, err := m.GetShare(ctx, &userpb.User{Id: creator}, idRef(stored.Id))
		if err != nil || got.GetCreator().GetOpaqueId() != creator.OpaqueId || got.GetGrantee().GetUserId().GetOpaqueId() != remote.OpaqueId {
			t.Fatalf("identity did not round-trip: %v", err)
		}
		var bound model.OcmShare
		if err := m.db.First(&bound, "id = ?", stored.Id.OpaqueId).Error; err != nil {
			t.Fatal(err)
		}
		if string(bound.CreatorIdp) != creator.Idp || string(bound.CreatorTenant) != creator.TenantId ||
			string(bound.RemoteRecipient) != remote.OpaqueId+"@"+host {
			t.Fatal("upper-width binding changed")
		}
		for _, field := range []string{"idp", "tenant"} {
			tooLong := &userpb.UserId{OpaqueId: creator.OpaqueId, Idp: creator.Idp, TenantId: creator.TenantId, Type: creator.Type}
			if field == "idp" {
				tooLong.Idp += "x"
			} else {
				tooLong.TenantId = strings.Repeat("t", 1021)
			}
			unbound, err := store(tooLong, remote, field)
			if err != nil {
				t.Fatalf("ordinary store with overlong %s: %v", field, err)
			}
			if got, err := m.GetShare(ctx, &userpb.User{Id: tooLong}, idRef(unbound.Id)); err != nil || got.GetToken() != field {
				t.Fatalf("ordinary lookup with overlong %s: %v %v", field, got, err)
			}
			var row model.OcmShare
			if err := m.db.First(&row, "id = ?", unbound.Id.OpaqueId).Error; err != nil {
				t.Fatal(err)
			}
			if row.CreatorIdp != nil || row.CreatorTenant != nil || row.RemoteRecipient != nil || row.CreatorUserType != 0 {
				t.Fatalf("overlong %s left a partial binding", field)
			}
			receipts, err := m.RemoveOCMSharesForRecipient(ctx, tooLong, remote)
			if err != nil || len(receipts) != 0 {
				t.Fatalf("directed withdrawal with overlong %s = %v, %v", field, receipts, err)
			}
		}
		receipts, err := m.RemoveOCMSharesForRecipient(ctx, creator, remote)
		if err != nil || len(receipts) != 1 || receipts[0].Share.GetId().GetOpaqueId() != stored.Id.OpaqueId {
			t.Fatalf("directed withdrawal with exact representable binding = %v, %v", receipts, err)
		}
	})
}

func TestReceivedInitializationPreservesLegacyMySQLRemoteID(t *testing.T) {
	target, ok := mysqlTestTarget(t)
	if !ok {
		t.Skip("disposable MySQL required")
	}
	cfg := openMySQLDatabase(t, target)
	m := openOCMManager(t, cfg)
	alice := recipient("alice")
	stored := mustStore(t, m, grantShare(alice, "remote.example.org", "initial", file))
	// Seed a row at the inherited VARCHAR character limit before reopening.
	if err := m.db.Exec("ALTER TABLE ocm_received_shares MODIFY remote_share_id VARCHAR(255) CHARACTER SET utf8mb4 NOT NULL").Error; err != nil {
		t.Fatal(err)
	}
	remoteID := strings.Repeat("界", 255)
	if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", stored.Id.OpaqueId).
		Updates(map[string]any{"remote_share_id": remoteID, "grant_key": nil}).Error; err != nil {
		t.Fatal(err)
	}
	m = openOCMManager(t, cfg)
	reopened, err := m.GetReceivedShare(context.Background(), alice, idRef(stored.Id))
	if err != nil || reopened.GetRemoteShareId() != remoteID {
		t.Fatalf("legacy remote ID changed on initialization: got %d bytes, error %v", len(reopened.GetRemoteShareId()), err)
	}
}

func TestDirectedWithdrawalRejectsInvalidSelectionAtomically(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		alice := &userpb.UserId{OpaqueId: "alice", Idp: "directory", Type: userpb.UserType_USER_TYPE_PRIMARY}
		bob := &userpb.UserId{OpaqueId: "bob", Idp: "remote.example.org", Type: userpb.UserType_USER_TYPE_FEDERATED}
		ids := make([]*ocm.ShareId, 0, 2)
		for _, token := range []string{"first", "second"} {
			g := &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: bob}}
			s := getOcmShare(getOcmAccessMethods("viewer"), g, alice, &provider.ResourceId{StorageId: "storage", OpaqueId: token}, token)
			s.RecipientType = ocm.RecipientType_RECIPIENT_TYPE_USER
			id, err := m.GenerateID(ctx)
			if err != nil {
				t.Fatal(err)
			}
			s.Id = id
			if _, err := m.StoreShare(ctx, s, file); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		if err := m.db.Model(&model.OcmShare{}).Where("id = ?", ids[1].OpaqueId).Update("item_type", "invalid").Error; err != nil {
			t.Fatal(err)
		}
		if receipts, err := m.RemoveOCMSharesForRecipient(ctx, alice, bob); err == nil || receipts != nil {
			t.Fatalf("invalid selection returned receipts %v, error %v", receipts, err)
		}
		var remaining int64
		if err := m.db.Model(&model.OcmShare{}).Where("id IN ?", []string{ids[0].OpaqueId, ids[1].OpaqueId}).Count(&remaining).Error; err != nil || remaining != 2 {
			t.Fatalf("selection was partly withdrawn: %d rows: %v", remaining, err)
		}
	})
}

// Ordinary reads and updates use every part of the stored recipient, its user
// type included. A row without a stored type keeps matching any type.
func TestReceivedShareRequiresExactRecipient(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx := context.Background()
		exact := &userpb.UserId{OpaqueId: "00012", Idp: "Directory", TenantId: "Tenant", Type: userpb.UserType_USER_TYPE_PRIMARY}
		store := func(remoteID string) *ocm.ReceivedShare {
			t.Helper()
			s := grantShare(&userpb.User{Id: exact}, "remote.example.org", remoteID, file)
			s.Grantee = &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: exact}}
			return mustStore(t, m, s)
		}
		typed := store("typed")
		untyped := store("untyped")
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", untyped.Id.OpaqueId).Update("recipient_user_type", 0).Error; err != nil {
			t.Fatal(err)
		}
		accept := func(rs *ocm.ReceivedShare) *ocm.ReceivedShare {
			return &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}
		}

		otherType := &userpb.UserId{OpaqueId: "00012", Idp: "Directory", TenantId: "Tenant", Type: userpb.UserType_USER_TYPE_FEDERATED}
		for _, other := range []*userpb.UserId{
			{OpaqueId: "00012 ", Idp: "Directory", TenantId: "Tenant", Type: exact.Type},
			{OpaqueId: "00012", Idp: "directory", TenantId: "Tenant", Type: exact.Type},
			{OpaqueId: "00012", Idp: "Directory", TenantId: "tenant", Type: exact.Type},
			otherType,
		} {
			u := &userpb.User{Id: other}
			if got, err := m.GetReceivedShare(ctx, u, idRef(typed.Id)); !errors.Is(err, share.ErrShareNotFound) {
				t.Fatalf("%v read the share: %v %v", other, got, err)
			}
			if got, err := m.UpdateReceivedShare(ctx, u, accept(typed), stateMask()); !errors.Is(err, share.ErrShareNotFound) {
				t.Fatalf("%v updated the share: %v %v", other, got, err)
			}
			list, err := m.ListReceivedShares(ctx, u, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, rs := range list {
				if rs.Id.OpaqueId == typed.Id.OpaqueId {
					t.Fatalf("%v listed the share", other)
				}
			}
		}

		owner := &userpb.User{Id: exact}
		got, err := m.GetReceivedShare(ctx, owner, idRef(typed.Id))
		if err != nil || got.GetGrantee().GetUserId().GetType() != userpb.UserType_USER_TYPE_PRIMARY {
			t.Fatalf("typed readback: %v %v", got, err)
		}
		if u, err := m.UpdateReceivedShare(ctx, owner, accept(typed), stateMask()); err != nil || !u.StateChanged() {
			t.Fatalf("owner update: %v %v", u, err)
		}
		if u, err := m.UpdateReceivedShare(ctx, &userpb.User{Id: otherType}, accept(untyped), stateMask()); err != nil || !u.StateChanged() {
			t.Fatalf("untyped row update: %v %v", u, err)
		}
	})
}
