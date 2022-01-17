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
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// Recycle items are stored inside the node folder and start with the uuid of the deleted node.
// The `.T.` indicates it is a trash item and what follows is the timestamp of the deletion.
// The deleted file is kept in the same location/dir as the original node. This prevents deletes
// from triggering cross storage moves when the trash is accidentally stored on another partition,
// because the admin mounted a different partition there.
// For an efficient listing of deleted nodes the ocis storage driver maintains a 'trash' folder
// with symlinks to trash files for every storagespace.

// ListRecycle returns the list of available recycle items
// ref -> the space (= resourceid), key -> deleted node id, relativePath = relative to key
func (fs *Decomposedfs) ListRecycle(ctx context.Context, ref *provider.Reference, key, relativePath string) ([]*provider.RecycleItem, error) {
	log := appctx.GetLogger(ctx)

	items := make([]*provider.RecycleItem, 0)

	if ref == nil || ref.ResourceId == nil || ref.ResourceId.OpaqueId == "" {
		return items, errtypes.BadRequest("spaceid required")
	}

	// check permissions
	trashnode, err := fs.lu.NodeFromSpaceID(ctx, ref.ResourceId)
	if err != nil {
		return nil, err
	}
	ok, err := fs.p.HasPermission(ctx, trashnode, func(rp *provider.ResourcePermissions) bool {
		return rp.ListRecycle
	})
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !ok:
		return nil, errtypes.PermissionDenied(key)
	}

	spaceID := ref.ResourceId.OpaqueId
	if key == "" && relativePath == "/" {
		return fs.listTrashRoot(ctx, spaceID)
	}

	trashRoot := fs.getRecycleRoot(ctx, spaceID)
	f, err := os.Open(filepath.Join(trashRoot, key, relativePath))
	if err != nil {
		if os.IsNotExist(err) {
			return items, nil
		}
		return nil, errors.Wrapf(err, "tree: error listing %s", trashRoot)
	}
	defer f.Close()

	parentNode, err := os.Readlink(filepath.Join(trashRoot, key))
	if err != nil {
		log.Error().Err(err).Str("trashRoot", trashRoot).Msg("error reading trash link, skipping")
		return nil, err
	}

	if md, err := f.Stat(); err != nil {
		return nil, err
	} else if !md.IsDir() {
		// this is the case when we want to directly list a file in the trashbin
		item, err := fs.createTrashItem(ctx, parentNode, filepath.Dir(relativePath), filepath.Join(trashRoot, key, relativePath))
		if err != nil {
			return items, err
		}
		items = append(items, item)
		return items, err
	}

	names, err := f.Readdirnames(0)
	if err != nil {
		return nil, err
	}
	for i := range names {
		if item, err := fs.createTrashItem(ctx, parentNode, relativePath, filepath.Join(trashRoot, key, relativePath, names[i])); err == nil {
			items = append(items, item)
		}
	}
	return items, nil
}

func (fs *Decomposedfs) createTrashItem(ctx context.Context, parentNode, intermediatePath, itemPath string) (*provider.RecycleItem, error) {
	log := appctx.GetLogger(ctx)
	trashnode, err := os.Readlink(itemPath)
	if err != nil {
		log.Error().Err(err).Msg("error reading trash link, skipping")
		return nil, err
	}
	parts := strings.SplitN(filepath.Base(parentNode), node.TrashIDDelimiter, 2)
	if len(parts) != 2 {
		log.Error().Str("trashnode", trashnode).Interface("parts", parts).Msg("malformed trash link, skipping")
		return nil, errors.New("malformed trash link")
	}

	nodePath := fs.lu.InternalPath(filepath.Base(trashnode))
	md, err := os.Stat(nodePath)
	if err != nil {
		log.Error().Err(err).Str("trashnode", trashnode).Msg("could not stat trash item, skipping")
		return nil, err
	}

	item := &provider.RecycleItem{
		Type: getResourceType(md.IsDir()),
		Size: uint64(md.Size()),
		Key:  path.Join(parts[0], intermediatePath, filepath.Base(itemPath)),
	}
	if deletionTime, err := time.Parse(time.RFC3339Nano, parts[1]); err == nil {
		item.DeletionTime = &types.Timestamp{
			Seconds: uint64(deletionTime.Unix()),
			// TODO nanos
		}
	} else {
		log.Error().Err(err).Str("link", trashnode).Interface("parts", parts).Msg("could parse time format, ignoring")
	}

	// lookup origin path in extended attributes
	parentPath := fs.lu.InternalPath(filepath.Base(parentNode))
	if attrBytes, err := xattr.Get(parentPath, xattrs.TrashOriginAttr); err == nil {
		item.Ref = &provider.Reference{Path: filepath.Join(string(attrBytes), intermediatePath, filepath.Base(itemPath))}
	} else {
		log.Error().Err(err).Str("link", trashnode).Msg("could not read origin path, skipping")
		return nil, err
	}

	// TODO filter results by permission ... on the original parent? or the trashed node?
	// if it were on the original parent it would be possible to see files that were trashed before the current user got access
	// so -> check the trash node itself
	// hmm listing trash currently lists the current users trash or the 'root' trash. from ocs only the home storage is queried for trash items.
	// for now we can only really check if the current user is the owner
	return item, nil
}

func (fs *Decomposedfs) listTrashRoot(ctx context.Context, spaceID string) ([]*provider.RecycleItem, error) {
	log := appctx.GetLogger(ctx)
	items := make([]*provider.RecycleItem, 0)

	trashRoot := fs.getRecycleRoot(ctx, spaceID)
	f, err := os.Open(trashRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return items, nil
		}
		return nil, errors.Wrap(err, "tree: error listing "+trashRoot)
	}
	defer f.Close()

	names, err := f.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	for i := range names {
		trashnode, err := os.Readlink(filepath.Join(trashRoot, names[i]))
		if err != nil {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Msg("error reading trash link, skipping")
			continue
		}
		parts := strings.SplitN(filepath.Base(trashnode), node.TrashIDDelimiter, 2)
		if len(parts) != 2 {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("trashnode", trashnode).Interface("parts", parts).Msg("malformed trash link, skipping")
			continue
		}

		nodePath := fs.lu.InternalPath(filepath.Base(trashnode))
		md, err := os.Stat(nodePath)
		if err != nil {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("trashnode", trashnode). /*.Interface("parts", parts)*/ Msg("could not stat trash item, skipping")
			continue
		}

		item := &provider.RecycleItem{
			Type: getResourceType(md.IsDir()),
			Size: uint64(md.Size()),
			Key:  parts[0],
		}
		if deletionTime, err := time.Parse(time.RFC3339Nano, parts[1]); err == nil {
			item.DeletionTime = &types.Timestamp{
				Seconds: uint64(deletionTime.Unix()),
				// TODO nanos
			}
		} else {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("link", trashnode).Interface("parts", parts).Msg("could parse time format, ignoring")
		}

		// lookup origin path in extended attributes
		var attrBytes []byte
		if attrBytes, err = xattr.Get(nodePath, xattrs.TrashOriginAttr); err == nil {
			item.Ref = &provider.Reference{Path: string(attrBytes)}
		} else {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("link", trashnode).Msg("could not read origin path, skipping")
			continue
		}
		// TODO filter results by permission ... on the original parent? or the trashed node?
		// if it were on the original parent it would be possible to see files that were trashed before the current user got access
		// so -> check the trash node itself
		// hmm listing trash currently lists the current users trash or the 'root' trash. from ocs only the home storage is queried for trash items.
		// for now we can only really check if the current user is the owner
		items = append(items, item)
	}
	return items, nil
}

// RestoreRecycleItem restores the specified item
func (fs *Decomposedfs) RestoreRecycleItem(ctx context.Context, ref *provider.Reference, key, relativePath string, restoreRef *provider.Reference) error {
	if ref == nil {
		return errtypes.BadRequest("missing reference, needs a space id")
	}

	var targetNode *node.Node
	if restoreRef != nil {
		tn, err := fs.lu.NodeFromResource(ctx, restoreRef)
		if err != nil {
			return err
		}

		targetNode = tn
	}

	rn, parent, restoreFunc, err := fs.tp.RestoreRecycleItemFunc(ctx, ref.ResourceId.OpaqueId, key, relativePath, targetNode)
	if err != nil {
		return err
	}

	// check permissions of deleted node
	ok, err := fs.p.HasPermission(ctx, rn, func(rp *provider.ResourcePermissions) bool {
		return rp.RestoreRecycleItem
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(key)
	}

	// check we can write to the parent of the restore reference
	ps, err := fs.p.AssemblePermissions(ctx, parent)
	if err != nil {
		return errtypes.InternalError(err.Error())
	}

	// share receiver cannot restore to a shared resource to which she does not have write permissions.
	if !ps.InitiateFileUpload {
		return errtypes.PermissionDenied(key)
	}

	// Run the restore func
	return restoreFunc()
}

// PurgeRecycleItem purges the specified item
func (fs *Decomposedfs) PurgeRecycleItem(ctx context.Context, ref *provider.Reference, key, relativePath string) error {
	if ref == nil {
		return errtypes.BadRequest("missing reference, needs a space id")
	}
	rn, purgeFunc, err := fs.tp.PurgeRecycleItemFunc(ctx, ref.ResourceId.OpaqueId, key, relativePath)
	if err != nil {
		return err
	}

	// check permissions of deleted node
	ok, err := fs.p.HasPermission(ctx, rn, func(rp *provider.ResourcePermissions) bool {
		return rp.PurgeRecycle
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(key)
	}

	// Run the purge func
	return purgeFunc()
}

// EmptyRecycle empties the trash
func (fs *Decomposedfs) EmptyRecycle(ctx context.Context, ref *provider.Reference) error {
	if ref == nil || ref.ResourceId == nil || ref.ResourceId.OpaqueId == "" {
		return errtypes.BadRequest("spaceid must be set")
	}
	// TODO what permission should we check? we could check the root node of the user? or the owner permissions on his home root node?
	// The current impl will wipe your own trash. or when no user provided the trash of 'root'
	return os.RemoveAll(fs.getRecycleRoot(ctx, ref.ResourceId.StorageId))
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

func (fs *Decomposedfs) getRecycleRoot(ctx context.Context, spaceID string) string {
	return filepath.Join(fs.o.Root, "trash", spaceID)
}
