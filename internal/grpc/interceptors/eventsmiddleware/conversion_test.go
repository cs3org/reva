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
