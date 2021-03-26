// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package decomposedfs

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/pkg/errors"
)

// Revision entries are stored inside the node folder and start with the same uuid as the current version.
// The `.REV.` indicates it is a revision and what follows is a timestamp, so multiple versions
// can be kept in the same location as the current file content. This prevents new fileuploads
// to trigger cross storage moves when revisions accidentally are stored on another partition,
// because the admin mounted a different partition there.
// We can add a background process to move old revisions to a slower storage
// and replace the revision file with a symbolic link in the future, if necessary.

// ListRevisions lists the revisions of the given resource
func (fs *Decomposedfs) ListRevisions(ctx context.Context, ref *provider.Reference) (revisions []*provider.FileVersion, err error) {
	var n *node.Node
	if n, err = fs.lu.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !n.Exists {
		err = errtypes.NotFound(filepath.Join(n.ParentID, n.Name))
		return
	}

	ok, err := fs.p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
		return rp.ListFileVersions
	})
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !ok:
		return nil, errtypes.PermissionDenied(filepath.Join(n.ParentID, n.Name))
	}

	revisions = []*provider.FileVersion{}
	np := n.InternalPath()
	if items, err := filepath.Glob(np + ".REV.*"); err == nil {
		for i := range items {
			if fi, err := os.Stat(items[i]); err == nil {
				mtime := fi.ModTime()
				rev := &provider.FileVersion{
					Key:   filepath.Base(items[i]),
					Mtime: uint64(mtime.Unix()),
				}
				blobSize, err := node.ReadBlobSizeAttr(items[i])
				if err != nil {
					return nil, errors.Wrapf(err, "error reading blobsize xattr")
				}
				rev.Size = uint64(blobSize)
				etag, err := node.CalculateEtag(np, mtime)
				if err != nil {
					return nil, errors.Wrapf(err, "error calculating etag")
				}
				rev.Etag = etag
				revisions = append(revisions, rev)
			}
		}
	}
	return
}

// DownloadRevision returns a reader for the specified revision
func (fs *Decomposedfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	log := appctx.GetLogger(ctx)

	// verify revision key format
	kp := strings.SplitN(revisionKey, ".REV.", 2)
	if len(kp) != 2 {
		log.Error().Str("revisionKey", revisionKey).Msg("malformed revisionKey")
		return nil, errtypes.NotFound(revisionKey)
	}
	log.Debug().Str("revisionKey", revisionKey).Msg("DownloadRevision")

	// check if the node is available and has not been deleted
	n, err := node.ReadNode(ctx, fs.lu, kp[0])
	if err != nil {
		return nil, err
	}
	if !n.Exists {
		err = errtypes.NotFound(filepath.Join(n.ParentID, n.Name))
		return nil, err
	}

	ok, err := fs.p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
		// TODO add explicit permission in the CS3 api?
		return rp.ListFileVersions && rp.RestoreFileVersion && rp.InitiateFileDownload
	})
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !ok:
		return nil, errtypes.PermissionDenied(filepath.Join(n.ParentID, n.Name))
	}

	contentPath := fs.lu.InternalPath(revisionKey)

	r, err := os.Open(contentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(contentPath)
		}
		return nil, errors.Wrap(err, "Decomposedfs: error opening revision "+revisionKey)
	}
	return r, nil
}

// RestoreRevision restores the specified revision of the resource
func (fs *Decomposedfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (err error) {
	log := appctx.GetLogger(ctx)

	// verify revision key format
	kp := strings.SplitN(revisionKey, ".REV.", 2)
	if len(kp) != 2 {
		log.Error().Str("revisionKey", revisionKey).Msg("malformed revisionKey")
		return errtypes.NotFound(revisionKey)
	}

	// check if the node is available and has not been deleted
	n, err := node.ReadNode(ctx, fs.lu, kp[0])
	if err != nil {
		return err
	}
	if !n.Exists {
		err = errtypes.NotFound(filepath.Join(n.ParentID, n.Name))
		return err
	}

	ok, err := fs.p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
		return rp.RestoreFileVersion
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(filepath.Join(n.ParentID, n.Name))
	}

	// move current version to new revision
	nodePath := fs.lu.InternalPath(kp[0])
	var fi os.FileInfo
	if fi, err = os.Stat(nodePath); err == nil {
		// versions are stored alongside the actual file, so a rename can be efficient and does not cross storage / partition boundaries
		versionsPath := fs.lu.InternalPath(kp[0] + ".REV." + fi.ModTime().UTC().Format(time.RFC3339Nano))

		err = os.Rename(nodePath, versionsPath)
		if err != nil {
			return
		}

		// copy old revision to current location

		revisionPath := fs.lu.InternalPath(revisionKey)
		var revision, destination *os.File
		revision, err = os.Open(revisionPath)
		if err != nil {
			return
		}
		defer revision.Close()

		destination, err = os.OpenFile(nodePath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
		if err != nil {
			return
		}
		defer destination.Close()
		_, err = io.Copy(destination, revision)
		if err != nil {
			return
		}

		return fs.copyMD(revisionPath, nodePath)
	}

	log.Error().Err(err).Interface("ref", ref).Str("originalnode", kp[0]).Str("revisionKey", revisionKey).Msg("original node does not exist")
	return
}
