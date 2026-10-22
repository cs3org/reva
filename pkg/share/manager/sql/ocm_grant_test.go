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
	"strings"
	"testing"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
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
			if r, err := m.WithdrawReceivedGrant(context.Background(), withdrawal("a.example.org", remote)); err != nil || !r.Changed {
				t.Fatalf("%d bytes: %v %v", len(remote), r, err)
			}
			if receivedRow(t, m, stored.Id.OpaqueId).GrantKey == nil {
				t.Fatal("valid grant stored unbound")
			}
		}

		before := countIdentities(t, m)
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
		if err := m.db.Model(&model.OcmReceivedShare{}).Count(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if err := m.db.Model(&model.OcmReceivedShareProtocol{}).Count(&protocols).Error; err != nil {
			t.Fatal(err)
		}
		if rows != 2 || protocols != 2 || countIdentities(t, m) != before {
			t.Fatalf("rejected grants left rows=%d protocols=%d identities=%d", rows, protocols, countIdentities(t, m))
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
		if err := m.db.Model(&model.OcmReceivedShare{}).Count(&rows).Error; err != nil || rows != 2 || countIdentities(t, m) != before {
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
		if err := m.db.Model(&model.OcmReceivedShare{}).Count(&rows).Error; err != nil || rows != 2 || countIdentities(t, m) != before {
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
