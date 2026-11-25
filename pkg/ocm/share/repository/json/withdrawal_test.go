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
	"context"
	"errors"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"google.golang.org/protobuf/proto"
)

// The JSON driver records no withdrawal and changes nothing.
func TestJSONWithdrawalIsNotSupported(t *testing.T) {
	m, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()
	sender := &userpb.UserId{OpaqueId: "alice", Idp: "provider.example.org"}
	grantee := testGrantee()
	user := &userpb.User{Id: grantee.GetUserId()}
	rs, err := m.StoreReceivedShare(ctx, &ocm.ReceivedShare{
		Name: "doc", RemoteShareId: "remote-1", Creator: sender, Owner: sender, Grantee: grantee,
		SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
		RecipientType:      ocm.RecipientType_RECIPIENT_TYPE_USER,
		State:              ocm.ShareState_SHARE_STATE_PENDING,
		Protocols: []*ocm.Protocol{share.NewWebDAVProtocol("https://provider.example.org/dav/remote-1", "fixture",
			&ocm.SharePermissions{Permissions: permissions.NewViewerRole().CS3ResourcePermissions()}, nil, nil)},
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}}
	before, err := m.GetReceivedShare(ctx, user, ref)
	if err != nil || before.GetSharedResourceType() != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE || len(before.GetProtocols()) != 1 {
		t.Fatalf("invalid withdrawal fixture: %v", err)
	}
	r, err := m.WithdrawReceivedGrant(ctx, &share.GrantWithdrawal{
		Origin: sender.Idp, RemoteShareID: rs.RemoteShareId, ResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
		RecipientType: ocm.RecipientType_RECIPIENT_TYPE_USER,
	})
	var notSupported errtypes.NotSupported
	if r != nil || !errors.As(err, &notSupported) {
		t.Fatalf("withdraw: %v %v", r, err)
	}
	got, err := m.GetReceivedShare(ctx, user, ref)
	if err != nil || !proto.Equal(before, got) {
		t.Fatalf("withdrawal changed the share or its protocols: %v", err)
	}
}
