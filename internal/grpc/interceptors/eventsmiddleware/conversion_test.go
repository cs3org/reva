package eventsmiddleware

import (
	"testing"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/owncloud/reva/v2/pkg/utils"
	"github.com/stretchr/testify/require"
)

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

	res := &provider.MoveResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}
	res.Opaque = utils.AppendJSONToOpaque(res.Opaque, "newref", newRef)
	res.Opaque = utils.AppendJSONToOpaque(res.Opaque, "oldref", oldRef)
	req := &provider.MoveRequest{Source: oldRef, Destination: newRef}

	ev := ItemMoved(res, req, spaceOwner, executant)

	require.Equal(t, spaceOwner, ev.SpaceOwner)
	require.Equal(t, executant.GetId(), ev.Executant)
	require.Equal(t, newRef.GetResourceId().GetSpaceId(), ev.Ref.GetResourceId().GetSpaceId())
	require.Equal(t, newRef.GetResourceId().GetOpaqueId(), ev.Ref.GetResourceId().GetOpaqueId())
	require.Equal(t, newRef.Path, ev.Ref.Path)
	require.Equal(t, oldRef.Path, ev.OldReference.Path)
	require.NotNil(t, ev.Timestamp)
}
