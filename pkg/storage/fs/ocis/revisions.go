package ocis

import (
	"context"
	"io"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

func (fs *ocisfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	return nil, errtypes.NotSupported("operation not supported: ListRevisions")
}
func (fs *ocisfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("operation not supported: DownloadRevision")
}

func (fs *ocisfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	return errtypes.NotSupported("operation not supported: RestoreRevision")
}
