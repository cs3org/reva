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
	"strings"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"google.golang.org/protobuf/proto"
)

func TestNewGrantMustBeRepresentable(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		marie := recipient("marie")
		// The protocol URI of grantShare embeds the remote ID; keep it short.
		grant := func(origin, remote string) *ocm.ReceivedShare {
			rs := grantShare(marie, origin, remote, file)
			rs.Protocols[0].GetWebdavOptions().Uri = "https://a.example.org/dav/x"
			return rs
		}
		multibyte := strings.Repeat("a", share.MaxGrantFieldLength-2) + "é" // 255 bytes
		for _, remote := range []string{strings.Repeat("r", share.MaxGrantFieldLength), multibyte} {
			stored := mustStore(t, m, grant("a.example.org", remote))
			if receivedRow(t, m, stored.Id.OpaqueId).GrantKey == nil {
				t.Fatal("valid grant stored unbound")
			}
			if r, err := m.WithdrawReceivedGrant(context.Background(), withdrawal("a.example.org", remote)); err != nil || !r.Changed {
				t.Fatalf("%d bytes: %v %v", len(remote), r, err)
			}
		}

		var rows, protocols int64
		for _, rs := range []*ocm.ReceivedShare{
			grant("a.example.org", strings.Repeat("r", share.MaxGrantFieldLength+1)),
			grant("a.example.org", strings.Repeat("a", share.MaxGrantFieldLength-1)+"é"), // 256 bytes
			grant("a.example.org", ""),
			grant("", "no-origin"),
			grant("bad host/path", "bad-origin"),
			// The raw sender host is validated before any normalization.
			grant("https://a.example.org", "https-origin"),
			grant("http://a.example.org", "http-origin"),
			grant("a.example.org:x8443", "bad-port"),
			grant("[2001:db8::1", "bad-bracket"),
			grant("a.example.org ", "space"),
			grant("a.example.org\t", "tab"),
		} {
			if _, err := m.StoreReceivedShare(context.Background(), rs); !errors.Is(err, share.ErrInvalidGrantIdentity) {
				t.Fatalf("%d-byte remote id from %q: %v", len(rs.RemoteShareId), rs.Creator.Idp, err)
			}
		}
		for _, id := range []*userpb.UserId{{Idp: "local.example"}, {OpaqueId: "marie"}} {
			rs := grant("a.example.org", "incomplete-recipient")
			rs.Grantee.GetUserId().OpaqueId, rs.Grantee.GetUserId().Idp = id.OpaqueId, id.Idp
			if stored, err := m.StoreReceivedShare(context.Background(), rs); stored != nil || !errors.Is(err, share.ErrInvalidGrantIdentity) {
				t.Fatalf("incomplete recipient stored: %v", err)
			}
		}
		if err := m.db.Model(&model.OcmReceivedShare{}).Count(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if err := m.db.Model(&model.OcmReceivedShareProtocol{}).Count(&protocols).Error; err != nil {
			t.Fatal(err)
		}
		if rows != 0 || protocols != 0 {
			t.Fatalf("rejected grants left rows=%d protocols=%d", rows, protocols)
		}

		noCreator := grant("a.example.org", "no-creator")
		noCreator.Creator = nil
		emptyID := grant("a.example.org", "empty-id")
		emptyID.Creator.OpaqueId = ""
		for _, rs := range []*ocm.ReceivedShare{noCreator, emptyID} {
			if _, err := m.StoreReceivedShare(context.Background(), rs); !errors.Is(err, share.ErrInvalidGrantIdentity) {
				t.Fatalf("creator %v: %v", rs.Creator, err)
			}
		}
		if err := m.db.Model(&model.OcmReceivedShare{}).Count(&rows).Error; err != nil || rows != 0 {
			t.Fatalf("rejected creators left rows=%d %v", rows, err)
		}

		// The typed host is checked alone: joined with the opaque ID, an @ in
		// it would still parse as an address with a different host.
		for _, idp := range []string{"extra@a.example.org", "https://evil@a.example.org", "@a.example.org",
			"a.example.org:bad@b.example.org", "a.example.org@"} {
			if _, err := m.StoreReceivedShare(context.Background(), grant(idp, "at-"+idp)); !errors.Is(err, share.ErrInvalidGrantIdentity) {
				t.Fatalf("idp %q: %v", idp, err)
			}
		}
		if err := m.db.Model(&model.OcmReceivedShare{}).Count(&rows).Error; err != nil || rows != 0 {
			t.Fatalf("rejected idps left rows=%d %v", rows, err)
		}
		atID := grant("A.Example.ORG:8443", "at-id")
		atID.Creator.OpaqueId = "alice@evil.example"
		stored := mustStore(t, m, atID)
		if got := receivedRow(t, m, stored.Id.OpaqueId).Initiator; got != "alice@evil.example@a.example.org:8443" {
			t.Fatalf("initiator %q", got)
		}
		if r, err := m.WithdrawReceivedGrant(context.Background(), withdrawal("a.example.org:8443", "at-id")); err != nil || !r.Changed {
			t.Fatalf("withdraw by port origin: %v %v", r, err)
		}

		for _, tc := range []struct{ opaque, idp, initiator, origin string }{
			{"opaque@kept ", "A.Example.org", "opaque@kept @a.example.org", "a.example.org"},
			{"v4", "192.0.2.10:9443", "v4@192.0.2.10:9443", "192.0.2.10:9443"},
			{"v6", "[2001:DB8::1]", "v6@[2001:db8::1]", "[2001:db8::1]"},
		} {
			rs := grant(tc.idp, "kept-"+tc.opaque)
			rs.Creator.OpaqueId = tc.opaque
			stored := mustStore(t, m, rs)
			if got := receivedRow(t, m, stored.Id.OpaqueId).Initiator; got != tc.initiator {
				t.Fatalf("initiator %q, want %q", got, tc.initiator)
			}
			if r, err := m.WithdrawReceivedGrant(context.Background(), withdrawal(tc.origin, "kept-"+tc.opaque)); err != nil || !r.Changed {
				t.Fatalf("withdraw %q: %v %v", tc.origin, r, err)
			}
		}

		fresh := mustStore(t, m, grant("a.example.org", "fresh"))
		if receivedRow(t, m, fresh.Id.OpaqueId).GrantKey == nil {
			t.Fatal("fresh grant unbound")
		}
	})
}

func TestResolvedBlankRecipientIsExactlyBound(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		ordinary := &userpb.User{Id: &userpb.UserId{OpaqueId: "marie", Idp: "local.example"}}
		oldKeyed := mustStore(t, m, grantShare(ordinary, "peer.example", "old-keyed", file))
		historical := mustStore(t, m, grantShare(ordinary, "peer.example", "historical", file))
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", historical.Id.OpaqueId).Update("grant_key", nil).Error; err != nil {
			t.Fatal(err)
		}
		if err := m.db.Exec("ALTER TABLE ocm_received_shares DROP COLUMN recipient_user_type").Error; err != nil {
			t.Fatal(err)
		}
		m = openOCMManager(t, cfg)
		blank := &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}}
		primary := &userpb.User{Id: &userpb.UserId{OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_PRIMARY}}
		nonblankPrimary := &userpb.User{Id: &userpb.UserId{OpaqueId: "marie", Idp: "local.example", Type: primary.Id.Type}}
		for _, tc := range []struct {
			s  *ocm.ReceivedShare
			as *userpb.User
		}{{historical, primary}, {oldKeyed, nonblankPrimary}} {
			got, err := m.GetReceivedShare(ctx, tc.as, idRef(tc.s.Id))
			if err != nil || got.GetGrantee().GetUserId().GetType() != 0 || len(got.GetProtocols()) != 1 {
				t.Fatalf("upgraded historical share: %v, %v", got, err)
			}
		}

		req := grantShare(blank, "peer.example", "verified-zero", file)
		for _, proof := range []*userpb.UserId{nil, {}, primary.Id, ordinary.Id, {OpaqueId: "marie", TenantId: "other"}, {OpaqueId: "other"}} {
			got, err := m.StoreReceivedShareForResolvedRecipient(ctx, req, proof)
			if got != nil || !errors.Is(err, share.ErrInvalidGrantIdentity) {
				t.Fatalf("unmatched proof: %v, %v", got, err)
			}
		}
		if got, err := m.StoreReceivedShare(ctx, req); got != nil || !errors.Is(err, share.ErrInvalidGrantIdentity) {
			t.Fatalf("unproven recipient: %v, %v", got, err)
		}
		stored, err := m.StoreReceivedShareForResolvedRecipient(ctx, req, blank.Id)
		if err != nil {
			t.Fatal(err)
		}
		row := receivedRow(t, m, stored.Id.OpaqueId)
		if row.GrantKey == nil || row.ShareWithIdp != "" || row.ShareWithTenant != "" || row.RecipientUserType != 0 {
			t.Fatalf("type-zero binding: %+v", row)
		}
		ref := idRef(stored.Id)
		update := &ocm.ReceivedShare{Id: stored.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}
		if _, err := m.GetReceivedShare(ctx, primary, ref); !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("different type read: %v", err)
		}
		if _, err := m.UpdateReceivedShare(ctx, primary, update, stateMask()); !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("different type update: %v", err)
		}
		list, err := m.ListReceivedShares(ctx, primary, nil)
		if err != nil || len(list) != 1 || list[0].GetId().GetOpaqueId() != historical.GetId().GetOpaqueId() {
			t.Fatalf("different type list: %v, %v", list, err)
		}
		if u, err := m.UpdateReceivedShare(ctx, blank, update, stateMask()); err != nil || !u.StateChanged() {
			t.Fatalf("exact type-zero update: %v, %v", u, err)
		}
		got, err := m.GetReceivedShare(ctx, blank, ref)
		if err != nil || !proto.Equal(got.GetGrantee().GetUserId(), blank.Id) || got.GetState() != update.State {
			t.Fatalf("exact type-zero readback: %v, %v", got, err)
		}

		req = grantShare(primary, "peer.example", "verified-primary", file)
		req.Grantee.GetUserId().Type = primary.Id.Type
		stored, err = m.StoreReceivedShareForResolvedRecipient(ctx, req, primary.Id)
		if err != nil {
			t.Fatal(err)
		}
		got, err = m.GetReceivedShare(ctx, primary, idRef(stored.Id))
		if err != nil || !proto.Equal(got.GetGrantee().GetUserId(), primary.Id) {
			t.Fatalf("exact primary readback: %v, %v", got, err)
		}
	})
}

func TestReceivedCommitObservationChecksRecipientType(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		ctx := context.Background()
		m := openOCMManager(t, cfg)
		observer := other(t, cfg)
		recipient := &userpb.User{Id: &userpb.UserId{OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_PRIMARY}}
		fault := &commitFault{apply: true, between: func() {
			id := rowOf(t, observer, "type-observation")
			if id == 0 {
				t.Fatal("committed row missing")
			}
			setRow(t, observer, id, map[string]any{"recipient_user_type": 0})
		}}
		withCommitFault(t, m, fault)
		fault.armed.Store(true)
		req := grantShare(recipient, "peer.example", "type-observation", file)
		req.Grantee.GetUserId().Type = recipient.Id.Type
		got, err := m.StoreReceivedShareForResolvedRecipient(ctx, req, recipient.Id)
		var unknown *share.CommitUnknownError
		if got != nil || !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedOther {
			t.Fatalf("observed different type after lost acknowledgment: %v, %v", got, err)
		}
	})
}

// A bound projection matches only its exact recipient, an empty tenant
// included. A row without a grant key still matches on the opaque ID.
func TestBoundRecipientIsExact(t *testing.T) {
	forEachDialect(t, func(t *testing.T, m *mgr) {
		ctx := context.Background()
		exact := &userpb.User{Id: &userpb.UserId{OpaqueId: "Alice", Idp: "local.example"}}
		req := grantShare(exact, "peer.example", "bound", file)
		req.Grantee.GetUserId().Idp = "local.example"
		bound := mustStore(t, m, req)
		legacy := mustStore(t, m, grantShare(exact, "peer.example", "legacy", file))
		if err := m.db.Model(&model.OcmReceivedShare{}).Where("id = ?", legacy.Id.OpaqueId).Update("grant_key", nil).Error; err != nil {
			t.Fatal(err)
		}
		accept := func(rs *ocm.ReceivedShare) *ocm.ReceivedShare {
			return &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}
		}

		for _, other := range []*userpb.UserId{
			{OpaqueId: "Alice", Idp: "local.example", TenantId: "tenant"},
			{OpaqueId: "Alice", Idp: "other.example"},
			{OpaqueId: "Alice"},
		} {
			u := &userpb.User{Id: other}
			if got, err := m.GetReceivedShare(ctx, u, idRef(bound.Id)); !errors.Is(err, share.ErrShareNotFound) {
				t.Fatalf("%v read the bound share: %v %v", other, got, err)
			}
			if got, err := m.UpdateReceivedShare(ctx, u, accept(bound), stateMask()); !errors.Is(err, share.ErrShareNotFound) {
				t.Fatalf("%v updated the bound share: %v %v", other, got, err)
			}
			list, err := m.ListReceivedShares(ctx, u, nil)
			if err != nil || len(list) != 1 || list[0].Id.OpaqueId != legacy.Id.OpaqueId {
				t.Fatalf("%v listed %v %v", other, list, err)
			}
		}

		got, err := m.GetReceivedShare(ctx, exact, idRef(bound.Id))
		if err != nil || got.GetGrantee().GetUserId().GetIdp() != "local.example" || got.GetGrantee().GetUserId().GetTenantId() != "" {
			t.Fatalf("exact recipient read: %v %v", got, err)
		}
		if u, err := m.UpdateReceivedShare(ctx, exact, accept(bound), stateMask()); err != nil || !u.StateChanged() {
			t.Fatalf("exact recipient update: %v %v", u, err)
		}
		if list, err := m.ListReceivedShares(ctx, exact, nil); err != nil || len(list) != 2 {
			t.Fatalf("exact recipient listed %v %v", list, err)
		}
		if u, err := m.UpdateReceivedShare(ctx, &userpb.User{Id: &userpb.UserId{OpaqueId: "Alice", Idp: "other.example"}}, accept(legacy), stateMask()); err != nil || !u.StateChanged() {
			t.Fatalf("legacy update: %v %v", u, err)
		}
	})
}
