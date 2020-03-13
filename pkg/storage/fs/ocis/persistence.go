package ocis

import (
	"context"
	"net/url"
	"os"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// TreePersistence is used to manage a tree hierarchy
type TreePersistence interface {
	GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error)
	GetMD(ctx context.Context, node *NodeInfo) (os.FileInfo, error)
	ListFolder(ctx context.Context, node *NodeInfo) ([]*NodeInfo, error)
	CreateDir(ctx context.Context, node *NodeInfo) (err error)
	CreateReference(ctx context.Context, path string, targetURI *url.URL) error
	Move(ctx context.Context, oldNode *NodeInfo, newNode *NodeInfo) (err error)
	Delete(ctx context.Context, node *NodeInfo) (err error)

	Propagate(ctx context.Context, node *NodeInfo) (err error)
}

// PathWrapper is used to encapsulate path transformations
type PathWrapper interface {
	Resolve(ctx context.Context, ref *provider.Reference) (node *NodeInfo, err error)
	WrapID(ctx context.Context, id *provider.ResourceId) (node *NodeInfo, err error)

	// Wrap returns a NodeInfo object:
	// - if the node exists with the node id, name and parent
	// - if only the parent exists, the node id is empty
	Wrap(ctx context.Context, fn string) (node *NodeInfo, err error)
	Unwrap(ctx context.Context, node *NodeInfo) (external string, err error)
	FillParentAndName(node *NodeInfo) (err error) // Tree persistence?
	ReadRootLink(root string) (node *NodeInfo, err error)
}
