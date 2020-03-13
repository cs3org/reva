package ocis

import (
	"context"
	"net/url"
	"os"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type TreePersistence interface {
	GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error)
	GetMD(ctx context.Context, internal string) (os.FileInfo, error)
	ListFolder(ctx context.Context, internal string) ([]os.FileInfo, error)
	CreateDir(ctx context.Context, internal string, newName string) (err error)
	CreateReference(ctx context.Context, path string, targetURI *url.URL) error
	Move(ctx context.Context, oldInternal string, newInternal string) (err error)
	Delete(ctx context.Context, internal string) (err error)

	Propagate(ctx context.Context, internal string) (err error)
}

type PathWrapper interface {
	Resolve(ctx context.Context, ref *provider.Reference) (internal string, err error)
	WrapID(ctx context.Context, id *provider.ResourceId) (internal string, err error)
	Wrap(ctx context.Context, fn string) (internal string, err error)
	Unwrap(ctx context.Context, np string) (external string, err error)
	ReadParentName(ctx context.Context, internal string) (parentNodeID string, name string, err error) // Tree persistence?
}
