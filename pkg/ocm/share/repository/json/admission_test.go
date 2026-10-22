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
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"google.golang.org/protobuf/proto"
)

func TestJSONStoreReceivedShareChecksRawCreator(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ocm-shares.json")
	m, err := New(ctx, map[string]any{"file": path})
	if err != nil {
		t.Fatal(err)
	}
	received := func(opaque, idp string) *ocm.ReceivedShare {
		return &ocm.ReceivedShare{
			RemoteShareId: "remote-1",
			Name:          "doc",
			Grantee:       testGrantee(),
			Owner:         &userpb.UserId{OpaqueId: "alice", Idp: "provider.example.org"},
			Creator:       &userpb.UserId{OpaqueId: opaque, Idp: idp},
			State:         ocm.ShareState_SHARE_STATE_PENDING,
		}
	}
	valid := received("alice@evil.example", "Provider.Example.ORG:8443")
	want := proto.Clone(valid.Creator)
	if _, err := m.StoreReceivedShare(ctx, valid); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	noCreator := received("alice", "provider.example.org")
	noCreator.Creator = nil
	for _, rs := range []*ocm.ReceivedShare{
		noCreator,
		received("", "provider.example.org"),
		received("alice", ""),
		received("alice", "evil.example@provider.example.org"),
		received("alice", "https://provider.example.org"),
		received("alice", "provider.example.org:x8443"),
		received("alice", "provider.example.org "),
	} {
		got, err := m.StoreReceivedShare(ctx, rs)
		if got != nil || !errors.Is(err, share.ErrInvalidGrantIdentity) {
			t.Fatalf("creator %v: %v %v", rs.Creator, got, err)
		}
		if rs.Id != nil || rs.Ctime != nil {
			t.Fatalf("rejected share %v was changed", rs.Creator)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected shares changed the file: %v", err)
	}

	shares, err := m.ListReceivedShares(ctx, &userpb.User{Id: testGrantee().GetUserId()}, nil)
	if err != nil || len(shares) != 1 || !proto.Equal(shares[0].Creator, want) {
		t.Fatalf("stored creators: %v %v", shares, err)
	}
}
