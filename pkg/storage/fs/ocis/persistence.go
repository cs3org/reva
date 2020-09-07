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
	GetMD(ctx context.Context, node *Node) (os.FileInfo, error)
	ListFolder(ctx context.Context, node *Node) ([]*Node, error)
	CreateDir(ctx context.Context, node *Node) (err error)
	CreateReference(ctx context.Context, path string, targetURI *url.URL) error
	Move(ctx context.Context, oldNode *Node, newNode *Node) (err error)
	Delete(ctx context.Context, node *Node) (err error)

	Propagate(ctx context.Context, node *Node) (err error)
}

// PathWrapper is used to encapsulate path transformations
type PathWrapper interface {
	NodeFromResource(ctx context.Context, ref *provider.Reference) (node *Node, err error)
	NodeFromID(ctx context.Context, id *provider.ResourceId) (node *Node, err error)

	// Wrap returns a Node object:
	// - if the node exists with the node id, name and parent
	// - if only the parent exists, the node id is empty
	NodeFromPath(ctx context.Context, fn string) (node *Node, err error)
	Path(ctx context.Context, node *Node) (path string, err error)

	RootNode(ctx context.Context) (node *Node, err error)
	// Root returns the internal root of the storage
	Root() string
}
