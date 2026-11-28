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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func recipient(id string) *userpb.User {
	return &userpb.User{Id: &userpb.UserId{OpaqueId: id, Idp: "local.example", Type: userpb.UserType_USER_TYPE_PRIMARY}}
}

// grantShare is a received share of remoteID sent by a user of origin.
func grantShare(to *userpb.User, origin, remoteID string, rt ocm.SharedResourceType) *ocm.ReceivedShare {
	now := &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Unix())}
	return &ocm.ReceivedShare{
		Name:          "doc",
		RemoteShareId: remoteID,
		Grantee: &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{
			UserId: &userpb.UserId{OpaqueId: to.Id.OpaqueId, Idp: to.Id.Idp, TenantId: to.Id.TenantId},
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
		expiredReq := grantShare(recipient("expired"), "peer.example:8443", "ExactID", ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER)
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

func TestSQLiteWithdrawalUsesIdentityIndexes(t *testing.T) {
	m := openOCMManager(t, ocmTestConfig(t, "sqlite"))
	if err := m.db.Exec(`
		WITH RECURSIVE unrelated(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM unrelated WHERE n < 10000)
		INSERT INTO ocm_received_shares
			(remote_share_id, name, item_type, share_with, owner, initiator, ctime, mtime, recipient_type, state)
		SELECT 'unrelated-' || n, 'doc', 'file', 'recipient', 'owner', 'sender@peer.example', 0, 0, 1, 1
		FROM unrelated
	`).Error; err != nil {
		t.Fatal(err)
	}
	var statTables int
	if err := m.db.Raw("SELECT count(*) FROM sqlite_schema WHERE name GLOB 'sqlite_stat[0-9]*'").Scan(&statTables).Error; err != nil || statTables != 0 {
		t.Fatalf("fresh schema has %d statistics tables: %v", statTables, err)
	}
	type query struct {
		statement string
		args      []any
	}
	var queries []query
	if err := m.db.Callback().Query().After("gorm:query").Register("test:withdrawal-plan", func(tx *gorm.DB) {
		if tx.Statement.Table == "ocm_received_shares" {
			queries = append(queries, query{tx.Statement.SQL.String(), append([]any(nil), tx.Statement.Vars...)})
		}
	}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	selector := withdrawal("peer.example", "absent")
	if receipt, err := m.WithdrawReceivedGrant(ctx, selector); err != nil || receipt == nil || receipt.Changed {
		t.Fatalf("withdrawal for absent grant = %v, %v", receipt, err)
	}
	zero := model.OcmPermissions(0)
	if changed, err := m.UpdateReceivedGrantPermissions(ctx, selector, &zero, nil); err != nil || changed {
		t.Fatalf("permission update for absent grant = %t, %v", changed, err)
	}

	var plans []string
	for _, q := range queries {
		var steps []struct{ Detail string }
		if err := m.db.Raw("EXPLAIN QUERY PLAN "+q.statement, q.args...).Scan(&steps).Error; err != nil {
			t.Fatal(err)
		}
		var detail []string
		for _, step := range steps {
			detail = append(detail, step.Detail)
		}
		plans = append(plans, strings.Join(detail, "\n"))
		t.Logf("%s\n%s", q.statement, plans[len(plans)-1])
	}
	want := []string{
		"USING INDEX i_ocmrecshare_grantkey (grant_key=?)",
		"USING INDEX i_ocmrecshare_remoteshareid (remote_share_id=?)",
		"USING INDEX i_ocmrecshare_grantkey (grant_key=?)",
	}
	if len(plans) != len(want) {
		t.Fatalf("expected two indexed withdrawal reads and one keyed permission read, got %v", plans)
	}
	for i, index := range want {
		if !strings.Contains(plans[i], index) || strings.Contains(plans[i], "SCAN ocm_received_shares") {
			t.Errorf("read %d plan = %q, want search %q", i+1, plans[i], index)
		}
	}
}

func TestHistoricalGrantWithdrawal(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx := context.Background()
		for _, remote := range []string{"0042", "Résumé"} {
			t.Run(remote, func(t *testing.T) {
				legacy := func(local, origin, id, initiator string, expired bool) *ocm.ReceivedShare {
					t.Helper()
					req := grantShare(recipient(local), origin, id, file)
					if expired {
						req.Expiration = &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())}
					}
					stored := mustStore(t, m, req)
					err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", stored.Id.OpaqueId).Updates(map[string]any{
						"grant_key": nil, "share_with_idp": "", "share_with_tenant": "", "initiator": initiator,
					}).Error
					if err != nil {
						t.Fatal(err)
					}
					return stored
				}
				first := legacy("alice", "peer.example", remote, "sender@PEER.Example", false)
				second := legacy("bob", "peer.example", remote, "sender@peer.example", false)
				survivors := []*ocm.ReceivedShare{
					legacy("carol", "peer.example", "42", "sender@peer.example", false),
					legacy("carol", "peer.example", "résumé", "sender@peer.example", false),
					legacy("carol", "peer.example", "Resume", "sender@peer.example", false),
					legacy("dora", "other.example", remote, "sender@other.example", false),
					legacy("erin", "peer.example", remote, "sender", false),
					legacy("finn", "peer.example", remote, "sender@peer.example", true),
				}
				selector := withdrawal("peer.example", remote)
				zero := model.OcmPermissions(0)
				if changed, err := m.UpdateReceivedGrantPermissions(ctx, selector, &zero, nil); err != nil || changed {
					t.Fatalf("legacy-only permission update = %t, %v", changed, err)
				}
				id := receivedRow(t, m, first.Id.OpaqueId).ID
				var keyed *ocm.ReceivedShare
				if remote == "Résumé" {
					before, err := readReceivedProjection(m.db, id)
					if err != nil {
						t.Fatal(err)
					}
					keyed = mustStore(t, m, grantShare(recipient("gina"), "peer.example", remote, file))
					if changed, err := m.UpdateReceivedGrantPermissions(ctx, selector, &zero, nil); err != nil || !changed {
						t.Fatalf("mixed permission update = %t, %v", changed, err)
					}
					after, err := readReceivedProjection(m.db, id)
					if err != nil || !before.equal(after) {
						t.Fatalf("mixed permission update changed a historical share: %v", err)
					}
				}
				for _, wrong := range []share.GrantWithdrawal{
					{Origin: selector.Origin, RemoteShareID: remote, ResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER},
					{Origin: selector.Origin, RemoteShareID: remote, ResourceType: file, RecipientType: ocm.RecipientType_RECIPIENT_TYPE_GROUP},
				} {
					if receipt, err := m.WithdrawReceivedGrant(ctx, &wrong); err != nil || receipt == nil || receipt.Changed {
						t.Fatalf("wrong selector: %v, %v", receipt, err)
					}
				}
				for _, want := range []bool{true, false} {
					if receipt, err := m.WithdrawReceivedGrant(ctx, selector); err != nil || receipt == nil || receipt.Changed != want {
						t.Fatalf("withdraw = %v, %v, want changed %t", receipt, err, want)
					}
				}
				assertProjectionCount(t, m, first.Id.OpaqueId, 0)
				assertProjectionCount(t, m, second.Id.OpaqueId, 0)
				if keyed != nil {
					assertProjectionCount(t, m, keyed.Id.OpaqueId, 0)
				}
				for _, kept := range survivors {
					assertProjectionCount(t, m, kept.Id.OpaqueId, 1)
				}
			})
		}
	})
}

func TestGrantPermissionUpdateUsesActiveRows(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx := context.Background()
		selector := withdrawal("peer.example", "permission")
		zero := model.OcmPermissions(0)
		expiredReq := grantShare(recipient("expired"), selector.Origin, selector.RemoteShareID, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER)
		expiredReq.Expiration = &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())}
		expired := mustStore(t, m, expiredReq)
		expiredID := receivedRow(t, m, expired.Id.OpaqueId).ID
		before, err := readReceivedProjection(m.db, expiredID)
		if err != nil || before.protocols[0].Permissions == zero {
			t.Fatalf("expired fixture has no permissions to preserve: %v", err)
		}
		if changed, err := m.UpdateReceivedGrantPermissions(ctx, selector, &zero, nil); err != nil || changed {
			t.Fatalf("expired-only update = %t, %v", changed, err)
		}
		active := mustStore(t, m, grantShare(recipient("active"), selector.Origin, selector.RemoteShareID, file))
		for _, want := range []bool{true, false} {
			if changed, err := m.UpdateReceivedGrantPermissions(ctx, selector, &zero, nil); err != nil || changed != want {
				t.Fatalf("update = %t, %v, want %t", changed, err, want)
			}
		}
		after, err := readReceivedProjection(m.db, expiredID)
		if err != nil || !before.equal(after) {
			t.Fatalf("expired projection changed: %v", err)
		}
		current, err := readReceivedProjection(m.db, receivedRow(t, m, active.Id.OpaqueId).ID)
		if err != nil || current.protocols[0].Permissions != zero {
			t.Fatalf("active permissions were not cleared: %v", err)
		}
		mixed := mustStore(t, m, grantShare(recipient("container"), selector.Origin, selector.RemoteShareID, expiredReq.SharedResourceType))
		viewer := before.protocols[0].Permissions
		if changed, err := m.UpdateReceivedGrantPermissions(ctx, selector, &viewer, nil); err != nil || changed {
			t.Fatalf("mixed active resource types: %t, %v", changed, err)
		}
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", mixed.Id.OpaqueId).
			Update("expiration", time.Now().Add(-time.Hour)).Error; err != nil {
			t.Fatal(err)
		}

		second := mustStore(t, m, grantShare(recipient("second"), selector.Origin, selector.RemoteShareID, file))
		secondID := receivedRow(t, m, second.Id.OpaqueId).ID
		if changed, err := m.UpdateReceivedGrantPermissions(ctx, selector, &zero, nil); err != nil || !changed {
			t.Fatalf("prepare unchanged permission masks: %t, %v", changed, err)
		}
		fail := errors.New("reject the second permission write")
		writes := 0
		if err := m.db.Callback().Update().After("gorm:update").Register("test:reject-permission-write", func(tx *gorm.DB) {
			if tx.Statement.Table == "ocm_received_share_protocols" {
				writes++
				if writes == 2 {
					_ = tx.AddError(fail)
				}
			}
		}); err != nil {
			t.Fatal(err)
		}
		if changed, err := m.UpdateReceivedGrantPermissions(ctx, selector, &viewer, nil); changed || !errors.Is(err, fail) || writes != 2 {
			t.Fatalf("failed batch = %t, %v, writes=%d", changed, err, writes)
		}
		for _, id := range []uint{current.row.ID, secondID} {
			p, err := readReceivedProjection(m.db, id)
			if err != nil || p.protocols[0].Permissions != zero {
				t.Fatalf("failed batch changed permissions for %d: %v", id, err)
			}
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

func TestCorruptKeyPreventsMixedGrantWithdrawal(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		selector := withdrawal("peer.example", "42")
		legacy := mustStore(t, m, grantShare(recipient("legacy"), selector.Origin, selector.RemoteShareID, file))
		corrupt := mustStore(t, m, grantShare(recipient("corrupt"), selector.Origin, selector.RemoteShareID, file))
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", legacy.Id.OpaqueId).Update("grant_key", nil).Error; err != nil {
			t.Fatal(err)
		}
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", corrupt.Id.OpaqueId).Update("initiator", "alice@other.example").Error; err != nil {
			t.Fatal(err)
		}
		if receipt, err := m.WithdrawReceivedGrant(context.Background(), selector); receipt != nil || !errors.Is(err, share.ErrGrantIdentityMismatch) {
			t.Fatalf("withdraw corrupted grant = %v, %v", receipt, err)
		}
		assertProjectionCount(t, m, legacy.Id.OpaqueId, 1)
		assertProjectionCount(t, m, corrupt.Id.OpaqueId, 1)
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
		releaseDelete := sync.OnceFunc(func() { close(release) })
		finished := make(chan struct{})
		var receipt *share.GrantWithdrawalReceipt
		var deleteErr error
		go func() {
			defer close(finished)
			receipt, deleteErr = deleter.WithdrawReceivedGrant(ctx, selector)
		}()
		defer func() {
			releaseDelete()
			<-finished
		}()
		select {
		case <-locked:
		case <-finished:
			t.Fatalf("withdrawal exited before acquiring the projection lock: %v", deleteErr)
		case <-ctx.Done():
			t.Fatal("withdrawal did not acquire the projection lock")
		}

		pool, err := writer.db.DB()
		if err != nil {
			t.Fatal(err)
		}
		pool.SetMaxOpenConns(1)
		conflicts := 0
		onConflict := func() {
			conflicts++
			releaseDelete()
			<-finished
		}
		if cfg["db_engine"] == "sqlite" {
			if _, err := pool.ExecContext(ctx, "PRAGMA busy_timeout = 0"); err != nil {
				t.Fatal(err)
			}
			writer.db.Statement.ConnPool = &sqliteBeginProbe{DB: pool, onBusy: onConflict}
		} else {
			if _, err := pool.ExecContext(ctx, "SET SESSION innodb_lock_wait_timeout = 1"); err != nil {
				t.Fatal(err)
			}
			if err := writer.db.Callback().Query().After("gorm:query").Register("test:lock-wait", func(tx *gorm.DB) {
				var mysqlErr *mysql.MySQLError
				if errors.As(tx.Error, &mysqlErr) && mysqlErr.Number == 1205 {
					onConflict()
				}
			}); err != nil {
				t.Fatal(err)
			}
		}
		completed, err := writer.CompleteReceivedShareTransfer(ctx, alice, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED)
		if conflicts != 1 || completed != nil || !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("completion after %d lock conflicts = %v, %v", conflicts, completed, err)
		}
		if deleteErr != nil || receipt == nil || !receipt.Changed {
			t.Fatalf("withdrawal = %v, %v", receipt, deleteErr)
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
		legacy := mustStore(t, deleter, grantShare(recipient("legacy"), selector.Origin, selector.RemoteShareID, file))
		if err := deleter.db.Model(&model.OcmReceivedShare{}).Where("id = ?", legacy.Id.OpaqueId).Update("grant_key", nil).Error; err != nil {
			t.Fatal(err)
		}
		for _, apply := range []bool{false, true} {
			t.Run("withdraw-applied="+strconv.FormatBool(apply), func(t *testing.T) {
				attempt := openOCMManager(t, cfg)
				deleteFault := &commitFault{apply: apply}
				withCommitFault(t, attempt, deleteFault)
				deleteFault.armed.Store(true)
				receipt, err := attempt.WithdrawReceivedGrant(ctx, selector)
				want, count := share.CommitObservedOther, int64(1)
				if apply {
					want, count = share.CommitObservedRequested, 0
				}
				var unknown *share.CommitUnknownError
				if receipt != nil || !errors.As(err, &unknown) || unknown.Observed != want || deleteFault.commits.Load() != 1 {
					t.Fatalf("unacknowledged deletion = %v, %v", receipt, err)
				}
				assertProjectionCount(t, deleter, rs.Id.OpaqueId, count)
				assertProjectionCount(t, deleter, legacy.Id.OpaqueId, count)
				assertProjectionCount(t, deleter, expired.Id.OpaqueId, 1)
			})
		}
	})
}
