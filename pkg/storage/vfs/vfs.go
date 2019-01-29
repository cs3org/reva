package vfs

import (
	"context"
	"io"
	"path"
	"strings"

	"github.com/cernbox/reva/pkg/storage"
	"github.com/pkg/errors"
)

type vfs struct {
	fsTable storage.FSTable
}

func New(fsTable storage.FSTable) storage.FS {
	fs := &vfs{fsTable: fsTable}
	return fs
}

func validate(fn string) error {
	if !strings.HasPrefix(fn, "/") {
		return invalidFilenameError(fn)
	}
	return nil
}

// findFS finds the mount for the given filename.
// it assumes the filename starts with slash and the path is
// already cleaned. The cases to handle are the following:
// - /
// - /docs
// - /docs/
// - /docs/one
func (fs *vfs) findFS(fn string) (storage.FS, string, error) {
	if err := validate(fn); err != nil {
		return nil, "", errors.Wrap(err, "vfs: invalid fn")
	}

	mount, err := fs.fsTable.GetMount(fn)
	if err != nil {
		// if mount is not found and fn is /, return list of mounts
		return nil, "", errors.Wrap(err, "vfs: error finding mount")
	}

	thefs := mount.GetFS()
	fsfn := path.Join("/", strings.TrimPrefix(fn, mount.GetDir()))
	return thefs, fsfn, nil
}

func (vfs *vfs) CreateDir(ctx context.Context, fn string) error {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}

	if err := fs.CreateDir(ctx, fsfn); err != nil {
		return errors.Wrap(err, "vfs: error creating dir")
	}
	return nil
}

func (vfs *vfs) Delete(ctx context.Context, fn string) error {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}

	if err := fs.Delete(ctx, fsfn); err != nil {
		return errors.Wrap(err, "vfs: error deleting file")
	}
	return nil
}

func (vfs *vfs) Download(ctx context.Context, fn string) (io.ReadCloser, error) {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: fs not found")
	}

	rc, err := fs.Download(ctx, fsfn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: error downloading file")
	}
	return rc, nil
}

func (vfs *vfs) GetPathByID(ctx context.Context, id string) (string, error) {
	return "", errors.New("todo")
}

func (vfs *vfs) SetACL(ctx context.Context, fn string, a *storage.ACL) error {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}

	if err = fs.SetACL(ctx, fsfn, a); err != nil {
		return errors.Wrap(err, "vfs: error setting acl")
	}
	return nil
}

func (vfs *vfs) UnsetACL(ctx context.Context, fn string, a *storage.ACL) error {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}
	if err = fs.UnsetACL(ctx, fsfn, a); err != nil {
		return errors.Wrap(err, "vfs: error unsetting acl")
	}
	return nil
}

func (vfs *vfs) UpdateACL(ctx context.Context, fn string, a *storage.ACL) error {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}
	if err = fs.UnsetACL(ctx, fsfn, a); err != nil {
		return errors.Wrap(err, "vfs: error updating acl")
	}
	return nil
}

func (vfs *vfs) GetACL(ctx context.Context, fn string, aclType storage.ACLType, target string) (*storage.ACL, error) {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: fs not found")
	}
	acl, err := fs.GetACL(ctx, fsfn, aclType, target)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: error getting acl")
	}
	return acl, nil
}

func (vfs *vfs) ListACLs(ctx context.Context, fn string) ([]*storage.ACL, error) {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: fs not found")
	}
	acls, err := fs.ListACLs(ctx, fsfn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: error listing acls")
	}
	return acls, nil
}

func (vfs *vfs) GetMD(ctx context.Context, fn string) (*storage.MD, error) {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: fs not found")
	}

	md, err := fs.GetMD(ctx, fsfn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: error getting md")
	}
	return md, nil
}

func (vfs *vfs) ListFolder(ctx context.Context, fn string) ([]*storage.MD, error) {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: fs not found")
	}

	mds, err := fs.ListFolder(ctx, fsfn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: error listing folder")
	}
	return mds, nil
}

func (vfs *vfs) GetQuota(ctx context.Context, fn string) (int, int, error) {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return 0, 0, errors.Wrap(err, "vfs: fs not found")
	}
	a, b, err := fs.GetQuota(ctx, fsfn)
	if err != nil {
		return 0, 0, errors.Wrap(err, "vfs: error getting quota")
	}
	return a, b, nil

}

func (vfs *vfs) Move(ctx context.Context, oldFn, newFn string) error {
	oldFS, oldFn, err := vfs.findFS(oldFn)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}

	newFS, newFn, err := vfs.findFS(newFn)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}

	if oldFS != newFS {
		return errors.New("cross storage move not supported")
	}

	if err = oldFS.Move(ctx, oldFn, newFn); err != nil {
		return errors.Wrap(err, "vfs: error moving file")
	}
	return nil
}

func (vfs *vfs) Upload(ctx context.Context, fn string, r io.ReadCloser) error {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}

	if err = fs.Upload(ctx, fsfn, r); err != nil {
		return errors.Wrap(err, "vfs: error uploading file")
	}
	return nil
}

func (vfs *vfs) ListRevisions(ctx context.Context, fn string) ([]*storage.Revision, error) {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: fs not found")
	}

	revs, err := fs.ListRevisions(ctx, fsfn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: error listing revs")
	}
	return revs, nil
}

func (vfs *vfs) DownloadRevision(ctx context.Context, fn, revisionKey string) (io.ReadCloser, error) {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: fs not found")
	}

	rc, err := fs.Download(ctx, fsfn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: error downloading rev")
	}
	return rc, nil
}

func (vfs *vfs) RestoreRevision(ctx context.Context, fn, revisionKey string) error {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}

	if err = fs.RestoreRevision(ctx, fsfn, revisionKey); err != nil {
		return errors.Wrap(err, "vfs: error restoring rev")
	}
	return nil
}

func (vfs *vfs) EmptyRecycle(ctx context.Context, fn string) error {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}

	if err = fs.EmptyRecycle(ctx, fsfn); err != nil {
		return errors.Wrap(err, "vfs: error emptying recycle")
	}
	return nil
}

func (vfs *vfs) ListRecycle(ctx context.Context, fn string) ([]*storage.RecycleItem, error) {
	fs, fsfn, err := vfs.findFS(fn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: fs not found")
	}

	items, err := fs.ListRecycle(ctx, fsfn)
	if err != nil {
		return nil, errors.Wrap(err, "vfs: error listing recycle")
	}
	return items, nil
}

func (vfs *vfs) RestoreRecycleItem(ctx context.Context, fsfn, key string) error {
	fs, fsfn, err := vfs.findFS(key)
	if err != nil {
		return errors.Wrap(err, "vfs: fs not found")
	}

	if err = fs.RestoreRecycleItem(ctx, fsfn, key); err != nil {
		return errors.Wrap(err, "vfs: error restoring recycle item")
	}
	return nil
}

type invalidFilenameError string
type mountNotFoundError string

func (e invalidFilenameError) Error() string { return string(e) }
func (e invalidFilenameError) IsNotFound()   {}
func (e mountNotFoundError) Error() string   { return string(e) }
func (e mountNotFoundError) IsNotFound()     {}
