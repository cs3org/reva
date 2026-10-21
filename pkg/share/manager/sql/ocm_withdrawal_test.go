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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	"google.golang.org/genproto/protobuf/field_mask"
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

func identityRow(t *testing.T, m *mgr, origin, remote string) (model.OcmGrantIdentity, bool) {
	t.Helper()
	key, err := share.GrantKey(origin, remote)
	if err != nil {
		t.Fatal(err)
	}
	var ident model.OcmGrantIdentity
	err = m.db.Where("grant_key = ?", key).Take(&ident).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ident, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return ident, true
}

func countIdentities(t *testing.T, m *mgr) int64 {
	t.Helper()
	var n int64
	if err := m.db.Model(&model.OcmGrantIdentity{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func withdrawal(origin, remote string) *share.GrantWithdrawal {
	return &share.GrantWithdrawal{Origin: origin, RemoteShareID: remote, ResourceType: file, RecipientType: ocm.RecipientType_RECIPIENT_TYPE_USER}
}

func TestWithdrawalRetainsEveryProjectionOfOneGrantOnly(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx := context.Background()
		alice, mahdi := recipient("alice-local"), recipient("mahdi")
		a42 := mustStore(t, m, grantShare(alice, "a.example.org", "42", file))
		a42dup := mustStore(t, m, grantShare(alice, "A.example.org", "42", file)) // duplicate projection, same canonical origin
		a43 := mustStore(t, m, grantShare(mahdi, "a.example.org", "43", file))
		c42 := mustStore(t, m, grantShare(alice, "c.example.org", "42", file))
		if receivedRow(t, m, a42.Id.OpaqueId).GrantKey == nil ||
			*receivedRow(t, m, a42.Id.OpaqueId).GrantKey != *receivedRow(t, m, a42dup.Id.OpaqueId).GrantKey {
			t.Fatal("duplicate projections are not bound to one identity")
		}
		before := receivedRow(t, m, a42.Id.OpaqueId)

		// A scheme is an invalid origin, not one to strip.
		if r, err := m.WithdrawReceivedGrant(ctx, withdrawal("https://a.example.org", "42")); !errors.Is(err, share.ErrInvalidGrantIdentity) || r != nil {
			t.Fatalf("scheme origin: %v %v", r, err)
		}
		r, err := m.WithdrawReceivedGrant(ctx, withdrawal("A.Example.org", "42"))
		if err != nil || !r.Changed || len(r.Shares) != 2 {
			t.Fatalf("withdraw: %v %v", r, err)
		}
		for _, id := range []string{a42.Id.OpaqueId, a42dup.Id.OpaqueId} {
			row := receivedRow(t, m, id)
			if row.State != model.OcmShareStateInvalid || !row.Hidden || row.Mtime < before.Mtime {
				t.Fatalf("row %s not retained as withdrawn: %+v", id, row)
			}
			got, err := m.GetReceivedShare(ctx, alice, idRef(&ocm.ShareId{OpaqueId: id}))
			if err != nil || len(got.Protocols) != 1 || got.State != ocm.ShareState_SHARE_STATE_INVALID {
				t.Fatalf("retained metadata %v %v", got, err)
			}
		}
		for _, id := range []string{a43.Id.OpaqueId, c42.Id.OpaqueId} {
			if row := receivedRow(t, m, id); row.State != model.OcmShareStatePending || row.Hidden {
				t.Fatalf("other grant %s affected: %+v", id, row)
			}
		}
		if ident, _ := identityRow(t, m, "a.example.org", "42"); !ident.WithdrawnAt.Valid {
			t.Fatal("withdrawn_at not recorded")
		}

		if r, err := m.WithdrawReceivedGrant(ctx, withdrawal("a.example.org", "42")); err != nil || r.Changed {
			t.Fatalf("repeat: %v %v", r, err)
		}
		mismatch := withdrawal("a.example.org", "43")
		mismatch.ResourceType = ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER
		if r, err := m.WithdrawReceivedGrant(ctx, mismatch); err != nil || r.Changed {
			t.Fatalf("type mismatch acted: %v %v", r, err)
		}
		if row := receivedRow(t, m, a43.Id.OpaqueId); row.State != model.OcmShareStatePending {
			t.Fatal("type mismatch changed the share")
		}
		n := countIdentities(t, m)
		if r, err := m.WithdrawReceivedGrant(ctx, withdrawal("a.example.org", "99")); err != nil || r.Changed {
			t.Fatalf("unmatched: %v %v", r, err)
		}
		if countIdentities(t, m) != n {
			t.Fatal("unmatched withdrawal created an identity")
		}
		for _, bad := range []*share.GrantWithdrawal{withdrawal("", "42"), withdrawal("a.example.org", ""), withdrawal("a/b", "42"), withdrawal("u@a.example.org", "42")} {
			if _, err := m.WithdrawReceivedGrant(ctx, bad); err == nil {
				t.Fatalf("invalid identity accepted: %+v", bad)
			}
		}
	})
}

func TestWithdrawnGrantRefusesEveryWriterAcrossRestart(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		alice := recipient("alice-local")
		rs := mustStore(t, m, grantShare(alice, "a.example.org", "42", file))
		start, err := m.StartReceivedShareTransfer(ctx, alice, &ocm.ReceivedShare{Id: rs.Id}, stateMask())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.WithdrawReceivedGrant(ctx, withdrawal("a.example.org", "42")); err != nil {
			t.Fatal(err)
		}

		for _, mm := range []*mgr{m, openOCMManager(t, cfg)} { // the second one is a restart
			if u, err := mm.CompleteReceivedShareTransfer(ctx, alice, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil || u.StateChanged() {
				t.Fatalf("completion after withdrawal: %v %v", u, err)
			}
			accept := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}
			if _, err := mm.UpdateReceivedShare(ctx, alice, accept, stateMask()); !errors.Is(err, share.ErrShareWithdrawn) {
				t.Fatalf("reaccept: %v", err)
			}
			unhide := &ocm.ReceivedShare{Id: rs.Id, Hidden: false}
			if _, err := mm.UpdateReceivedShare(ctx, alice, unhide, &field_mask.FieldMask{Paths: []string{"hidden"}}); !errors.Is(err, share.ErrShareWithdrawn) {
				t.Fatalf("unhide: %v", err)
			}
			if _, err := mm.StartReceivedShareTransfer(ctx, alice, &ocm.ReceivedShare{Id: rs.Id}, stateMask()); !errors.Is(err, share.ErrShareWithdrawn) {
				t.Fatalf("start: %v", err)
			}
			var before int64
			mm.db.Model(&model.OcmReceivedShare{}).Count(&before)
			if _, err := mm.StoreReceivedShare(ctx, grantShare(alice, "A.example.org", "42", file)); !errors.Is(err, share.ErrShareWithdrawn) {
				t.Fatalf("stale create: %v", err)
			}
			var after int64
			mm.db.Model(&model.OcmReceivedShare{}).Count(&after)
			if after != before {
				t.Fatal("stale create added a row")
			}
			row := receivedRow(t, mm, rs.Id.OpaqueId)
			if row.State != model.OcmShareStateInvalid || !row.Hidden {
				t.Fatalf("withdrawn share resurrected: %+v", row)
			}
		}
		fresh := mustStore(t, m, grantShare(alice, "a.example.org", "44", file))
		accept := &ocm.ReceivedShare{Id: fresh.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}
		if u, err := m.UpdateReceivedShare(ctx, alice, accept, stateMask()); err != nil || !u.StateChanged() {
			t.Fatalf("fresh grant: %v %v", u, err)
		}
	})
}

func TestGenericInvalidHiddenIsNotWithdrawal(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx := context.Background()
		alice := recipient("alice-local")
		rs := mustStore(t, m, grantShare(alice, "a.example.org", "42", file))
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", rs.Id.OpaqueId).
			Updates(map[string]any{"state": -1, "hidden": true}).Error; err != nil {
			t.Fatal(err)
		}
		if ident, _ := identityRow(t, m, "a.example.org", "42"); ident.WithdrawnAt.Valid {
			t.Fatal("generic invalid row has a withdrawal")
		}
		unhide := &ocm.ReceivedShare{Id: rs.Id, Hidden: false}
		if _, err := m.UpdateReceivedShare(ctx, alice, unhide, &field_mask.FieldMask{Paths: []string{"hidden"}}); err != nil {
			t.Fatalf("generic invalid row is treated as withdrawn: %v", err)
		}
		got, err := m.GetReceivedShare(ctx, alice, idRef(rs.Id))
		if err != nil || got.State != ocm.ShareState_SHARE_STATE_INVALID {
			t.Fatalf("generic invalid state %v %v", got, err)
		}
	})
}

// Every stored share is bound to its grant. A row without a binding, which
// only a manual edit can produce, is refused by every writer and is not
// repaired at startup.
func TestUnboundRowFailsClosed(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		marie := recipient("marie")
		for _, key := range []any{nil, ""} {
			rs := mustStore(t, m, grantShare(marie, "a.example.org", fmt.Sprintf("unbound-%v", key), file))
			if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", rs.Id.OpaqueId).Update("grant_key", key).Error; err != nil {
				t.Fatal(err)
			}
			before := receivedRow(t, m, rs.Id.OpaqueId)
			accept := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}
			if u, err := m.UpdateReceivedShare(ctx, marie, accept, stateMask()); !errors.Is(err, share.ErrGrantIdentityMismatch) || u != nil {
				t.Fatalf("update %v: %v %v", key, u, err)
			}
			if s, err := m.StartReceivedShareTransfer(ctx, marie, &ocm.ReceivedShare{Id: rs.Id}, stateMask()); !errors.Is(err, share.ErrGrantIdentityMismatch) || s != nil {
				t.Fatalf("start %v: %v %v", key, s, err)
			}
			if u, err := m.CompleteReceivedShareTransfer(ctx, marie, rs.Id, 0, ocm.ShareState_SHARE_STATE_ACCEPTED); !errors.Is(err, share.ErrGrantIdentityMismatch) || u != nil {
				t.Fatalf("complete %v: %v %v", key, u, err)
			}
			openOCMManager(t, cfg)
			if after := receivedRow(t, m, rs.Id.OpaqueId); after.State != before.State || after.Mtime != before.Mtime ||
				after.TransferGeneration != before.TransferGeneration ||
				(after.GrantKey == nil) != (before.GrantKey == nil) || (after.GrantKey != nil && *after.GrantKey != *before.GrantKey) {
				t.Fatalf("unbound row changed: %+v", after)
			}
		}
	})
}

func TestIdentityCollisionFailsClosed(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		key, _ := share.GrantKey("a.example.org", "42")
		// A record stored under the key of (a.example.org, 42) with other fields.
		if err := m.db.Create(&model.OcmGrantIdentity{GrantKey: key, Origin: "b.example.org", RemoteShareID: "42"}).Error; err != nil {
			t.Fatal(err)
		}
		var before int64
		m.db.Model(&model.OcmReceivedShare{}).Count(&before)
		if _, err := m.StoreReceivedShare(context.Background(), grantShare(recipient("alice-local"), "a.example.org", "42", file)); !errors.Is(err, share.ErrGrantIdentityMismatch) {
			t.Fatalf("collision: %v", err)
		}
		if _, err := m.WithdrawReceivedGrant(context.Background(), withdrawal("a.example.org", "42")); !errors.Is(err, share.ErrGrantIdentityMismatch) {
			t.Fatalf("collision withdrawal: %v", err)
		}
		var after int64
		m.db.Model(&model.OcmReceivedShare{}).Count(&after)
		if after != before {
			t.Fatal("collision created a row")
		}
	})
}

func TestFailedWithdrawalCommitsNothing(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx := context.Background()
		rs := mustStore(t, m, grantShare(recipient("alice-local"), "a.example.org", "42", file))
		var fail sync.Mutex
		failing := true
		if err := m.db.Callback().Update().After("gorm:update").Register("test:fail-received", func(db *gorm.DB) {
			fail.Lock()
			defer fail.Unlock()
			if failing && db.Statement.Table == "ocm_received_shares" {
				_ = db.AddError(errors.New("injected failure"))
			}
		}); err != nil {
			t.Fatal(err)
		}
		if r, err := m.WithdrawReceivedGrant(ctx, withdrawal("a.example.org", "42")); err == nil || r != nil {
			t.Fatalf("failed withdrawal returned %v %v", r, err)
		}
		if ident, _ := identityRow(t, m, "a.example.org", "42"); ident.WithdrawnAt.Valid {
			t.Fatal("withdrawn_at committed despite the failure")
		}
		if row := receivedRow(t, m, rs.Id.OpaqueId); row.State != model.OcmShareStatePending || row.Hidden {
			t.Fatal("rows changed despite the failure")
		}
		fail.Lock()
		failing = false
		fail.Unlock()
		if r, err := m.WithdrawReceivedGrant(ctx, withdrawal("a.example.org", "42")); err != nil || !r.Changed {
			t.Fatalf("withdrawal after recovery: %v %v", r, err)
		}
	})
}

// checkGrantInvariant: a withdrawn grant has only INVALID hidden rows; an
// active grant has none that were turned INVALID by withdrawal.
func checkGrantInvariant(t *testing.T, m *mgr, origin, remote string) (withdrawnAt bool) {
	t.Helper()
	ident, ok := identityRow(t, m, origin, remote)
	if !ok {
		return false
	}
	var rows []model.OcmReceivedShare
	if err := m.db.Where("grant_key = ?", ident.GrantKey).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if ident.WithdrawnAt.Valid && (r.State != model.OcmShareStateInvalid || !r.Hidden) {
			t.Fatalf("withdrawn grant has an active row: %+v", r)
		}
		if !ident.WithdrawnAt.Valid && r.State == model.OcmShareStateInvalid {
			t.Fatalf("active grant has an invalid row: %+v", r)
		}
	}
	if ident.WithdrawnAt.Valid && len(rows) == 0 {
		t.Fatal("withdrawal recorded without any retained share")
	}
	return ident.WithdrawnAt.Valid
}

func TestWithdrawalRacesOnSeparateConnections(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m1, m2 := openOCMManager(t, cfg), openOCMManager(t, cfg)
		alice := recipient("alice-local")
		for i := 0; i < 12; i++ {
			remote := fmt.Sprintf("race-%d", i)
			rs := mustStore(t, m1, grantShare(alice, "a.example.org", remote, file))
			start, err := m1.StartReceivedShareTransfer(ctx, alice, &ocm.ReceivedShare{Id: rs.Id}, stateMask())
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			var completion *share.ReceivedShareUpdate
			var withdrawalReceipt *share.GrantWithdrawalReceipt
			var cerr, werr error
			wg.Add(2)
			go func() {
				defer wg.Done()
				completion, cerr = m1.CompleteReceivedShareTransfer(ctx, alice, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED)
			}()
			go func() {
				defer wg.Done()
				withdrawalReceipt, werr = m2.WithdrawReceivedGrant(ctx, withdrawal("a.example.org", remote))
			}()
			wg.Wait()
			if cerr != nil || werr != nil {
				t.Fatalf("completion %v withdrawal %v", cerr, werr)
			}
			if !withdrawalReceipt.Changed || !checkGrantInvariant(t, m1, "a.example.org", remote) {
				t.Fatal("withdrawal of an existing grant did not win")
			}
			if completion.StateChanged() && completion.After.State != ocm.ShareState_SHARE_STATE_ACCEPTED {
				t.Fatalf("completion receipt %v", completion.After.State)
			}
		}

		// Withdrawal against accept, unhide and start on another connection.
		for i, op := range []func(id *ocm.ShareId) error{
			func(id *ocm.ShareId) error {
				_, err := m1.UpdateReceivedShare(ctx, alice, &ocm.ReceivedShare{Id: id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}, stateMask())
				return err
			},
			func(id *ocm.ShareId) error {
				_, err := m1.UpdateReceivedShare(ctx, alice, &ocm.ReceivedShare{Id: id, Hidden: false}, &field_mask.FieldMask{Paths: []string{"hidden"}})
				return err
			},
			func(id *ocm.ShareId) error {
				_, err := m1.StartReceivedShareTransfer(ctx, alice, &ocm.ReceivedShare{Id: id}, stateMask())
				return err
			},
		} {
			remote := fmt.Sprintf("writer-%d", i)
			rs := mustStore(t, m1, grantShare(alice, "a.example.org", remote, file))
			var wg sync.WaitGroup
			var opErr, werr error
			wg.Add(2)
			go func() { defer wg.Done(); opErr = op(rs.Id) }()
			go func() {
				defer wg.Done()
				_, werr = m2.WithdrawReceivedGrant(ctx, withdrawal("a.example.org", remote))
			}()
			wg.Wait()
			if werr != nil || (opErr != nil && !errors.Is(opErr, share.ErrShareWithdrawn)) {
				t.Fatalf("op %d: %v withdrawal %v", i, opErr, werr)
			}
			if !checkGrantInvariant(t, m1, "a.example.org", remote) {
				t.Fatalf("op %d: grant not withdrawn", i)
			}
		}

		// First create and stale duplicate creates against withdrawal.
		for i := 0; i < 8; i++ {
			remote := fmt.Sprintf("create-%d", i)
			if i%2 == 1 {
				mustStore(t, m1, grantShare(alice, "a.example.org", remote, file))
			}
			var wg sync.WaitGroup
			var cerr, werr error
			wg.Add(2)
			go func() {
				defer wg.Done()
				_, cerr = m1.StoreReceivedShare(ctx, grantShare(alice, "a.example.org", remote, file))
			}()
			go func() {
				defer wg.Done()
				_, werr = m2.WithdrawReceivedGrant(ctx, withdrawal("a.example.org", remote))
			}()
			wg.Wait()
			if werr != nil || (cerr != nil && !errors.Is(cerr, share.ErrShareWithdrawn)) {
				t.Fatalf("create %v withdrawal %v", cerr, werr)
			}
			checkGrantInvariant(t, m1, "a.example.org", remote)
		}

		// Absent-key insert contention: one identity, every row bound to it.
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(mm *mgr) {
				defer wg.Done()
				_, err := mm.StoreReceivedShare(ctx, grantShare(alice, "a.example.org", "contended", file))
				errs <- err
			}([]*mgr{m1, m2}[i%2])
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		ident, ok := identityRow(t, m1, "a.example.org", "contended")
		var bound int64
		m1.db.Model(&model.OcmReceivedShare{}).Where("grant_key = ?", ident.GrantKey).Count(&bound)
		var identities int64
		m1.db.Model(&model.OcmGrantIdentity{}).Where("remote_share_id = ?", "contended").Count(&identities)
		if !ok || bound != 8 || identities != 1 {
			t.Fatalf("contention: identities %d bound %d", identities, bound)
		}
	})
}

type helperRequest struct {
	Config map[string]any `json:"config"`
	Op     string         `json:"op"`
	Remote string         `json:"remote"`
}

const helperEnv = "REVA_OCM_HELPER_REQUEST"

// TestOCMHelperProcess is not a test: it runs one operation in a separate
// process for the multi-process races.
func TestOCMHelperProcess(t *testing.T) {
	raw := os.Getenv(helperEnv)
	if raw == "" {
		t.Skip("helper process only")
	}
	var req helperRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatal(err)
	}
	if port, ok := req.Config["db_port"].(float64); ok {
		req.Config["db_port"] = int(port)
	}
	m := openOCMManager(t, req.Config)
	var err error
	switch req.Op {
	case "create":
		_, err = m.StoreReceivedShare(context.Background(), grantShare(recipient("alice-local"), "a.example.org", req.Remote, file))
	case "withdraw":
		_, err = m.WithdrawReceivedGrant(context.Background(), withdrawal("a.example.org", req.Remote))
	}
	if err != nil && !errors.Is(err, share.ErrShareWithdrawn) {
		t.Fatal(err)
	}
}

func TestWithdrawalRacesAcrossProcesses(t *testing.T) {
	if os.Getenv(helperEnv) != "" {
		t.Skip()
	}
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		m := openOCMManager(t, cfg)
		for round := 0; round < 3; round++ {
			remote := fmt.Sprintf("proc-%d", round)
			var procs []*exec.Cmd
			for i := 0; i < 5; i++ {
				op := "create"
				if i == 2 {
					op = "withdraw"
				}
				raw, _ := json.Marshal(helperRequest{Config: cfg, Op: op, Remote: remote})
				cmd := exec.Command(os.Args[0], "-test.run=^TestOCMHelperProcess$", "-test.count=1")
				cmd.Env = append(os.Environ(), helperEnv+"="+string(raw))
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				procs = append(procs, cmd)
			}
			for _, p := range procs {
				if err := p.Wait(); err != nil {
					t.Fatalf("helper process: %v", err)
				}
			}
			checkGrantInvariant(t, m, "a.example.org", remote)
			var identities int64
			m.db.Model(&model.OcmGrantIdentity{}).Where("remote_share_id = ?", remote).Count(&identities)
			if identities != 1 {
				t.Fatalf("identities %d", identities)
			}
		}
	})
}
