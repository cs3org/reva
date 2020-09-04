package ocis

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

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
