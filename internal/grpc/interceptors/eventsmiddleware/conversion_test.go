// Copyright 2018-2021 CERN
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

package eventsmiddleware

import (
	"testing"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/owncloud/reva/v2/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSpaceOwner and testExecutant are shared across tests.
var testSpaceOwner = &user.UserId{
	Idp:      "test-idp",
	OpaqueId: "space-owner-id",
	Type:     user.UserType_USER_TYPE_PRIMARY,
}

var testExecutant = &user.User{
	Id: &user.UserId{
		Idp:      "test-idp",
		OpaqueId: "executant-id",
		Type:     user.UserType_USER_TYPE_PRIMARY,
	},
}

var testRef = &provider.Reference{
	ResourceId: &provider.ResourceId{
		StorageId: "storage-1",
		SpaceId:   "space-1",
		OpaqueId:  "opaque-1",
	},
	Path: "./some/path",
}

func TestContainerCreated(t *testing.T) {
	req := &provider.CreateContainerRequest{Ref: testRef}
	resp := &provider.CreateContainerResponse{}

	ev := ContainerCreated(resp, req, testSpaceOwner, testExecutant)

	assert.Equal(t, testSpaceOwner, ev.SpaceOwner)
	assert.Equal(t, testRef, ev.Ref)
	assert.Equal(t, testExecutant.GetId(), ev.Executant)
}

func TestFileTouched(t *testing.T) {
	req := &provider.TouchFileRequest{Ref: testRef}
	resp := &provider.TouchFileResponse{}

	ev := FileTouched(resp, req, testSpaceOwner, testExecutant)

	assert.Equal(t, testSpaceOwner, ev.SpaceOwner)
	assert.Equal(t, testRef, ev.Ref)
	assert.Equal(t, testExecutant.GetId(), ev.Executant)
}

func TestFileLocked(t *testing.T) {
	req := &provider.SetLockRequest{Ref: testRef}
	resp := &provider.SetLockResponse{}

	ev := FileLocked(resp, req, testSpaceOwner, testExecutant)

	assert.Equal(t, testRef, ev.Ref)
	assert.Equal(t, testExecutant.GetId(), ev.Executant)
}

func TestFileUnlocked(t *testing.T) {
	req := &provider.UnlockRequest{Ref: testRef}
	resp := &provider.UnlockResponse{}

	ev := FileUnlocked(resp, req, testSpaceOwner, testExecutant)

	assert.Equal(t, testRef, ev.Ref)
	assert.Equal(t, testExecutant.GetId(), ev.Executant)
}

func TestItemRestored(t *testing.T) {
	oldRef := &provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: "storage-1",
			SpaceId:   "space-1",
			OpaqueId:  "trash-key-1",
		},
		Path: "./trash/path",
	}
	req := &provider.RestoreRecycleItemRequest{
		Ref:        oldRef,
		RestoreRef: testRef,
		Key:        "trash-key-1",
	}
	resp := &provider.RestoreRecycleItemResponse{}

	ev := ItemRestored(resp, req, testSpaceOwner, testExecutant)

	assert.Equal(t, testSpaceOwner, ev.SpaceOwner)
	// When RestoreRef is set, Ref should be RestoreRef; OldReference should be req.Ref.
	assert.Equal(t, testRef, ev.Ref)
	assert.Equal(t, oldRef, ev.OldReference)
	assert.Equal(t, testExecutant.GetId(), ev.Executant)
}

func TestFileVersionRestored(t *testing.T) {
	req := &provider.RestoreFileVersionRequest{
		Ref: testRef,
		Key: "v1",
	}
	resp := &provider.RestoreFileVersionResponse{}

	ev := FileVersionRestored(resp, req, testSpaceOwner, testExecutant)

	assert.Equal(t, testSpaceOwner, ev.SpaceOwner)
	assert.Equal(t, testRef, ev.Ref)
	assert.Equal(t, testExecutant.GetId(), ev.Executant)
	assert.Equal(t, "v1", ev.Key)
}

func TestItemMoved(t *testing.T) {
	spaceOwner := &user.UserId{OpaqueId: "owner-1"}
	executant := &user.User{Id: &user.UserId{OpaqueId: "user-1"}}
	newRef := &provider.Reference{
		ResourceId: &provider.ResourceId{StorageId: "storage-1", SpaceId: "space-1", OpaqueId: "node-1"},
		Path:       "./new-name.txt",
	}
	oldRef := &provider.Reference{
		ResourceId: &provider.ResourceId{StorageId: "storage-1", SpaceId: "space-1", OpaqueId: "node-1"},
		Path:       "./old-name.txt",
	}

	result := &storage.MoveResult{
		SpaceOwner:   spaceOwner,
		NewReference: newRef,
		OldReference: oldRef,
	}
	res := &provider.MoveResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}
	req := &provider.MoveRequest{Source: oldRef, Destination: newRef}

	ev := ItemMoved(res, req, result, executant)

	require.Equal(t, spaceOwner, ev.SpaceOwner)
	require.Equal(t, executant.GetId(), ev.Executant)
	require.Equal(t, newRef, ev.Ref)
	require.Equal(t, oldRef, ev.OldReference)
	require.NotNil(t, ev.Timestamp)
}
func TestItemTrashed(t *testing.T) {
	executant := &user.User{Id: &user.UserId{OpaqueId: "executant-id"}}
	spaceOwner := &user.UserId{OpaqueId: "space-owner-id"}

	reqRef := &provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: "storage-1",
			SpaceId:   "space-1",
			OpaqueId:  "request-opaque-id",
		},
	}
	req := &provider.DeleteRequest{Ref: reqRef}
	res := &provider.DeleteResponse{}

	tests := []struct {
		name              string
		result            *storage.DeleteResult
		wantStorageId     string
		wantSpaceId       string
		wantOpaqueId      string
		wantSpaceOwnerNil bool
	}{
		{
			name: "decomposedfs flow: typed result has SpaceId/OpaqueId, StorageId is filled from request",
			result: &storage.DeleteResult{
				SpaceOwner: spaceOwner,
				ResourceId: &provider.ResourceId{
					SpaceId:  "space-from-result",
					OpaqueId: "opaque-from-result",
				},
			},
			wantStorageId: "storage-1",
			wantSpaceId:   "space-from-result",
			wantOpaqueId:  "opaque-from-result",
		},
		{
			name:              "non-decomposedfs flow: empty result falls back to req.Ref.ResourceId",
			result:            &storage.DeleteResult{},
			wantStorageId:     "storage-1",
			wantSpaceId:       "space-1",
			wantOpaqueId:      "request-opaque-id",
			wantSpaceOwnerNil: true,
		},
		{
			name:              "nil result falls back to req.Ref.ResourceId",
			result:            nil,
			wantStorageId:     "storage-1",
			wantSpaceId:       "space-1",
			wantOpaqueId:      "request-opaque-id",
			wantSpaceOwnerNil: true,
		},
		{
			name: "typed result with StorageId already set is preserved",
			result: &storage.DeleteResult{
				SpaceOwner: spaceOwner,
				ResourceId: &provider.ResourceId{
					StorageId: "storage-from-result",
					SpaceId:   "space-from-result",
					OpaqueId:  "opaque-from-result",
				},
			},
			wantStorageId: "storage-from-result",
			wantSpaceId:   "space-from-result",
			wantOpaqueId:  "opaque-from-result",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := ItemTrashed(res, req, tc.result, executant)

			if ev.ID == nil {
				t.Fatalf("event ID is nil, want non-nil")
			}
			if got := ev.ID.GetStorageId(); got != tc.wantStorageId {
				t.Errorf("StorageId: got %q, want %q", got, tc.wantStorageId)
			}
			if got := ev.ID.GetSpaceId(); got != tc.wantSpaceId {
				t.Errorf("SpaceId: got %q, want %q", got, tc.wantSpaceId)
			}
			if got := ev.ID.GetOpaqueId(); got != tc.wantOpaqueId {
				t.Errorf("OpaqueId: got %q, want %q", got, tc.wantOpaqueId)
			}

			if tc.wantSpaceOwnerNil {
				if ev.SpaceOwner != nil {
					t.Errorf("SpaceOwner: got %v, want nil", ev.SpaceOwner)
				}
			} else if ev.SpaceOwner == nil || ev.SpaceOwner.GetOpaqueId() != spaceOwner.GetOpaqueId() {
				t.Errorf("SpaceOwner: got %v, want %v", ev.SpaceOwner, spaceOwner)
			}

			if ev.Executant.GetOpaqueId() != executant.Id.OpaqueId {
				t.Errorf("Executant: got %v, want %v", ev.Executant, executant.Id)
			}
			if ev.Ref != reqRef {
				t.Errorf("Ref: got %v, want %v", ev.Ref, reqRef)
			}
			if ev.Timestamp == nil {
				t.Errorf("Timestamp: got nil, want non-nil")
			}
		})
	}
}