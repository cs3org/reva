package ocis

import (
	"context"
	"io"
	"os"
	"path/filepath"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

// Revision entries are stored inside the node folder and start with the same uuid as the current version.
// The `.REV.` indicates it is a revision and what follows is a timestamp, so multiple versions
// can be kept in the same location as the current file content. This prevents new fileuploads
// to trigger cross storage moves when revisions accidentally are stored on another partition,
// because the admin mounted a different partition there.
// We can add a background process to move old revisions to a slower storage
// and replace the revision file with a symbolic link in the future, if necessary.

func (fs *ocisfs) ListRevisions(ctx context.Context, ref *provider.Reference) (revisions []*provider.FileVersion, err error) {
	var node *Node
	if node, err = fs.pw.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}

	revisions = []*provider.FileVersion{}
	nodePath := filepath.Join(fs.pw.Root(), "nodes", node.ID)
	if items, err := filepath.Glob(nodePath + ".REV.*"); err == nil {
		for i := range items {
			if fi, err := os.Stat(items[i]); err == nil {
				rev := &provider.FileVersion{
					Key:   filepath.Base(items[i]),
					Size:  uint64(fi.Size()),
					Mtime: uint64(fi.ModTime().Unix()),
				}
				revisions = append(revisions, rev)
			}
		}
	}
	return
}
func (fs *ocisfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("operation not supported: DownloadRevision")
}

func (fs *ocisfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	return errtypes.NotSupported("operation not supported: RestoreRevision")
}
