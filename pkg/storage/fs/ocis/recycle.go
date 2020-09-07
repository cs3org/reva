package ocis

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

// Recycle items are stored inside the node folder and start with the uuid of the deleted node.
// The `.T.` indicates it is a trash item and what follows is the timestamp of the deletion.
// The deleted file is kept in the same location/dir as the original node. This prevents deletes
// from triggering cross storage moves when the trash is accidentally stored on another partition,
// because the admin mounted a different partition there.
// TODO For an efficient listing of deleted nodes the ocis storages trash folder should have
// contain a directory with symlinks to trash files for every userid/"root"

func (fs *ocisfs) PurgeRecycleItem(ctx context.Context, key string) error {
	return errtypes.NotSupported("operation not supported: PurgeRecycleItem")
}

func (fs *ocisfs) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("operation not supported: EmptyRecycle")
}

func (fs *ocisfs) ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("operation not supported: ListRecycle")
}

func (fs *ocisfs) RestoreRecycleItem(ctx context.Context, key string) error {
	return errtypes.NotSupported("operation not supported: RestoreRecycleItem")
}
