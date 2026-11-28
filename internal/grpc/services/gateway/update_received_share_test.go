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
)

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
	})
	require.NoError(t, err)
	require.Equal(t, rpc.Code_CODE_OK, res.Status.Code, res.Status.Message)
	h.assertCannotRead(t, userAGrantee(), parentPath)
}

// TestUpdateReceivedShareKeepsStateWhenGrantFails makes sure a rejection whose
// grant cannot be revoked leaves the share accepted instead of rejected but still
// reachable.
func TestUpdateReceivedShareKeepsStateWhenGrantFails(t *testing.T) {
	h := newShareHierarchyGatewayHarness(t)
	share := h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
	h.storage.failGrants = true

	res, err := h.svc.UpdateReceivedShare(context.Background(), &collaboration.UpdateReceivedShareRequest{
		Share: &collaboration.ReceivedShare{
			Share: &collaboration.Share{Id: share.Id},
			State: collaboration.ShareState_SHARE_STATE_REJECTED,
		},
	})
	require.NoError(t, err)
	require.NotEqual(t, rpc.Code_CODE_OK, res.Status.Code)
	require.Equal(t, collaboration.ShareState_SHARE_STATE_PENDING, h.shares.stateOf(share.Id.OpaqueId))
	h.assertCanRead(t, userAGrantee(), parentPath)
}
