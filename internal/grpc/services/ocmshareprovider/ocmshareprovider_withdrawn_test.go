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

package ocmshareprovider

import (
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"google.golang.org/genproto/protobuf/field_mask"
)

func (e *emitEnv) withdraw(t *testing.T, remoteID string, rt ocm.SharedResourceType) {
	t.Helper()
	r, err := e.repo.WithdrawReceivedGrant(e.recip, &share.GrantWithdrawal{
		Origin: e.peer.host(), RemoteShareID: remoteID, ResourceType: rt,
	})
	if err != nil || !r.Changed {
		t.Fatalf("withdraw: %v %v", r, err)
	}
}

func requireWithdrawnStatus(t *testing.T, s *rpc.Status) {
	t.Helper()
	if s.GetCode() != rpc.Code_CODE_FAILED_PRECONDITION || s.GetMessage() != share.WithdrawnShareMessage {
		t.Fatalf("status %v", s)
	}
}

func TestWithdrawnShareRefusesRecipientMutations(t *testing.T) {
	e := newEmitEnv(t)
	rs := e.received(t, "remote-w", false)
	e.withdraw(t, "remote-w", ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE)

	requireWithdrawnStatus(t, e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "").GetStatus())
	requireWithdrawnStatus(t, e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_REJECTED, "").GetStatus())
	res, err := e.svc.UpdateReceivedOCMShare(e.recip, &ocm.UpdateReceivedOCMShareRequest{
		Share:      &ocm.ReceivedShare{Id: rs.Id, Hidden: false},
		UpdateMask: &field_mask.FieldMask{Paths: []string{"hidden"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireWithdrawnStatus(t, res.GetStatus())
	requireNotices(t, e.peer)
}

func TestWithdrawalWinsOverEmbeddedTransfer(t *testing.T) {
	e := newEmitEnv(t)
	rs := e.received(t, "remote-x", true)
	if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest"); res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("start: %v", res.Status)
	}
	e.withdraw(t, "remote-x", ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED)
	e.xfer.callback(0)(nil) // the copy finishes after the withdrawal
	requireNotices(t, e.peer)

	// A new transfer of the withdrawn share cannot start.
	other := e.received(t, "remote-y", true)
	e.withdraw(t, "remote-y", ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED)
	requireWithdrawnStatus(t, e.setState(t, other.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest").GetStatus())
	if len(e.xfer.callbacks) != 1 {
		t.Fatal("a transfer of a withdrawn share was launched")
	}
}
