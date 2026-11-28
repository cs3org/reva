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

package gateway

import (
	"context"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// stateMask is the update mask a client sends when it accepts or rejects a share.
func stateMask() *fieldmaskpb.FieldMask {
	return &fieldmaskpb.FieldMask{Paths: []string{"state"}}
}

// TestUpdateReceivedShareManagesStorageGrant pins the grant lifecycle of a
// received share: rejecting it must revoke the grant the share carries, and
// accepting it again must restore it.
func TestUpdateReceivedShareManagesStorageGrant(t *testing.T) {
	h := newShareHierarchyGatewayHarness(t)
	share := h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)

	updateState := func(state collaboration.ShareState) {
		t.Helper()
		res, err := h.svc.UpdateReceivedShare(context.Background(), &collaboration.UpdateReceivedShareRequest{
			Share: &collaboration.ReceivedShare{
				Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: share.Id.OpaqueId}},
				State: state,
			},
			UpdateMask: stateMask(),
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, rpc.Code_CODE_OK, res.Status.Code, res.Status.Message)
		require.Equal(t, state, h.shares.stateOf(share.Id.OpaqueId))
	}

	// The grant is committed when the share is created, so the recipient can read
	// even before accepting.
	h.assertCanRead(t, userAGrantee(), parentPath)

	updateState(collaboration.ShareState_SHARE_STATE_ACCEPTED)
	h.assertCanRead(t, userAGrantee(), parentPath)

	updateState(collaboration.ShareState_SHARE_STATE_REJECTED)
	h.assertCannotRead(t, userAGrantee(), parentPath)

	updateState(collaboration.ShareState_SHARE_STATE_ACCEPTED)
	h.assertCanRead(t, userAGrantee(), parentPath)

	updateState(collaboration.ShareState_SHARE_STATE_REJECTED)
	h.assertCannotRead(t, userAGrantee(), parentPath)
}

// TestUpdateReceivedShareRejectsPendingShare checks the grant of a share that the
// recipient never accepted: it was applied at creation, so rejecting the share
// must revoke it too.
func TestUpdateReceivedShareRejectsPendingShare(t *testing.T) {
	h := newShareHierarchyGatewayHarness(t)
	share := h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
	require.Equal(t, collaboration.ShareState_SHARE_STATE_PENDING, h.shares.stateOf(share.Id.OpaqueId))
	h.assertCanRead(t, userAGrantee(), parentPath)

	res, err := h.svc.UpdateReceivedShare(context.Background(), &collaboration.UpdateReceivedShareRequest{
		Share: &collaboration.ReceivedShare{
			Share: &collaboration.Share{Id: share.Id},
			State: collaboration.ShareState_SHARE_STATE_REJECTED,
		},
		UpdateMask: stateMask(),
	})
	require.NoError(t, err)
	require.Equal(t, rpc.Code_CODE_OK, res.Status.Code, res.Status.Message)
	h.assertCannotRead(t, userAGrantee(), parentPath)
}

// TestUpdateReceivedShareFollowsStoredState makes sure the grant follows the state
// the provider stores, not the state the request asks for: a request whose update
// mask leaves the state alone must not change the grant either.
func TestUpdateReceivedShareFollowsStoredState(t *testing.T) {
	h := newShareHierarchyGatewayHarness(t)
	share := h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)

	res, err := h.svc.UpdateReceivedShare(context.Background(), &collaboration.UpdateReceivedShareRequest{
		Share: &collaboration.ReceivedShare{
			Share: &collaboration.Share{Id: share.Id},
			State: collaboration.ShareState_SHARE_STATE_REJECTED,
		},
		UpdateMask: stateMask(),
	})
	require.NoError(t, err)
	require.Equal(t, rpc.Code_CODE_OK, res.Status.Code, res.Status.Message)
	h.assertCannotRead(t, userAGrantee(), parentPath)

	// The mask names another field, so the share stays rejected.
	res, err = h.svc.UpdateReceivedShare(context.Background(), &collaboration.UpdateReceivedShareRequest{
		Share: &collaboration.ReceivedShare{
			Share: &collaboration.Share{Id: share.Id},
			State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"hidden"}},
	})
	require.NoError(t, err)
	require.Equal(t, rpc.Code_CODE_OK, res.Status.Code, res.Status.Message)
	require.Equal(t, collaboration.ShareState_SHARE_STATE_REJECTED, h.shares.stateOf(share.Id.OpaqueId))
	h.assertCannotRead(t, userAGrantee(), parentPath)
}

// TestUpdateReceivedShareKeepsChildGrants covers the hierarchy: the storage fake
// applies a grant on a container to everything below it, like EOS, so the grants of
// the child shares have to be re-applied around a state change of the parent share.
func TestUpdateReceivedShareKeepsChildGrants(t *testing.T) {
	h := newShareHierarchyGatewayHarness(t)
	parent := h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
	h.createShare(t, childPath, userAGrantee(), permissions.NewEditorRole().CS3ResourcePermissions(), false)
	h.assertCanWrite(t, userAGrantee(), childPath)

	updateState := func(state collaboration.ShareState) {
		t.Helper()
		res, err := h.svc.UpdateReceivedShare(context.Background(), &collaboration.UpdateReceivedShareRequest{
			Share: &collaboration.ReceivedShare{
				Share: &collaboration.Share{Id: parent.Id},
				State: state,
			},
			UpdateMask: stateMask(),
		})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, res.Status.Code, res.Status.Message)
	}

	// Revoking the grant of the parent must not take the grant of the child with it.
	updateState(collaboration.ShareState_SHARE_STATE_REJECTED)
	h.assertCannotRead(t, userAGrantee(), parentPath)
	h.assertCanWrite(t, userAGrantee(), childPath)

	// Accepting the parent applies its grant on the whole subtree, so the child
	// grant has to be re-applied afterwards to keep its higher permissions.
	updateState(collaboration.ShareState_SHARE_STATE_ACCEPTED)
	h.assertCanRead(t, userAGrantee(), parentPath)
	h.assertCanWrite(t, userAGrantee(), childPath)
}

// TestUpdateReceivedShareKeepsStateWhenGrantFails makes sure a state change whose
// grant cannot be changed leaves the share in the state the grant still matches.
func TestUpdateReceivedShareKeepsStateWhenGrantFails(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state collaboration.ShareState
	}{
		{name: "reject", state: collaboration.ShareState_SHARE_STATE_REJECTED},
		{name: "accept", state: collaboration.ShareState_SHARE_STATE_ACCEPTED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newShareHierarchyGatewayHarness(t)
			share := h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
			previous := h.shares.stateOf(share.Id.OpaqueId)
			h.storage.failGrants = true

			res, err := h.svc.UpdateReceivedShare(context.Background(), &collaboration.UpdateReceivedShareRequest{
				Share: &collaboration.ReceivedShare{
					Share: &collaboration.Share{Id: share.Id},
					State: tc.state,
				},
				UpdateMask: stateMask(),
			})
			require.NoError(t, err)
			require.NotEqual(t, rpc.Code_CODE_OK, res.Status.Code)
			require.Equal(t, previous, h.shares.stateOf(share.Id.OpaqueId))
			h.assertCanRead(t, userAGrantee(), parentPath)
		})
	}
}
