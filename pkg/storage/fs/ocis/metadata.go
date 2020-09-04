package ocis

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

func (fs *ocisfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	return errtypes.NotSupported("operation not supported: SetArbitraryMetadata")
}

func (fs *ocisfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	return errtypes.NotSupported("operation not supported: UnsetArbitraryMetadata")
}
