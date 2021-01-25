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

package s3ng

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/node"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/xattrs"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// Recycle items are stored inside the node folder and start with the uuid of the deleted node.
// The `.T.` indicates it is a trash item and what follows is the timestamp of the deletion.
// The deleted file is kept in the same location/dir as the original node. This prevents deletes
// from triggering cross storage moves when the trash is accidentally stored on another partition,
// because the admin mounted a different partition there.
// TODO For an efficient listing of deleted nodes the ocis storages trash folder should have
// contain a directory with symlinks to trash files for every userid/"root"

func (fs *s3ngfs) ListRecycle(ctx context.Context) (items []*provider.RecycleItem, err error) {
	log := appctx.GetLogger(ctx)

	trashRoot := fs.getRecycleRoot(ctx)

	items = make([]*provider.RecycleItem, 0)

	// TODO how do we check if the storage allows listing the recycle for the current user? check owner of the root of the storage?
	// use permissions ReadUserPermissions?
	if fs.o.EnableHome {
		if !node.OwnerPermissions.ListContainer {
			log.Debug().Msg("owner not allowed to list trash")
			return items, errtypes.PermissionDenied("owner not allowed to list trash")
		}
	} else {
		if !node.NoPermissions.ListContainer {
			log.Debug().Msg("default permissions prevent listing trash")
			return items, errtypes.PermissionDenied("default permissions prevent listing trash")
		}
	}

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
		var trashnode string
		trashnode, err = os.Readlink(filepath.Join(trashRoot, names[i]))
		if err != nil {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Msg("error reading trash link, skipping")
			err = nil
			continue
		}
		parts := strings.SplitN(filepath.Base(trashnode), ".T.", 2)
		if len(parts) != 2 {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("trashnode", trashnode).Interface("parts", parts).Msg("malformed trash link, skipping")
			continue
		}

		nodePath := fs.lu.InternalPath(filepath.Base(trashnode))
		md, err := os.Stat(nodePath)
		if err != nil {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("trashnode", trashnode).Interface("parts", parts).Msg("could not stat trash item, skipping")
			continue
		}

		item := &provider.RecycleItem{
			Type: getResourceType(md.IsDir()),
			Size: uint64(md.Size()),
			Key:  filepath.Base(trashRoot) + ":" + parts[0], // glue using :, a / is interpreted as a path and only the node id will reach the other methods
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
			item.Path = string(attrBytes)
		} else {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("link", trashnode).Msg("could not read origin path, skipping")
			continue
		}
		// TODO filter results by permission ... on the original parent? or the trashed node?
		// if it were on the original parent it would be possible to see files that were trashed before the current user got access
		// so -> check the trash node itself
		// hmm listing trash currently lists the current users trash or the 'root' trash. from ocs only the home storage is queried for trash items.
		// for now we can only really check if the current user is the owner
		if attrBytes, err = xattr.Get(nodePath, xattrs.OwnerIDAttr); err == nil {
			if fs.o.EnableHome {
				u := user.ContextMustGetUser(ctx)
				if u.Id.OpaqueId != string(attrBytes) {
					log.Warn().Str("trashRoot", trashRoot).Str("name", names[i]).Str("link", trashnode).Msg("trash item not owned by current user, skipping")
					continue
				}
			}
		} else {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("link", trashnode).Msg("could not read owner, skipping")
			continue
		}

		items = append(items, item)
	}
	return
}

func (fs *s3ngfs) RestoreRecycleItem(ctx context.Context, key string) (err error) {
	log := appctx.GetLogger(ctx)

	var rn *node.Node
	var trashItem string
	var deletedNodePath string
	var origin string
	if rn, trashItem, deletedNodePath, origin, err = node.ReadRecycleItem(ctx, fs.lu, key); err != nil {
		return
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

	// link to origin
	var n *node.Node
	n, err = fs.lu.NodeFromPath(ctx, origin)
	if err != nil {
		return
	}

	if n.Exists {
		return errtypes.AlreadyExists("origin already exists")
	}

	// add the entry for the parent dir
	err = os.Symlink("../"+rn.ID, filepath.Join(fs.lu.InternalPath(n.ParentID), n.Name))
	if err != nil {
		return
	}

	// rename to node only name, so it is picked up by id
	nodePath := rn.InternalPath()
	err = os.Rename(deletedNodePath, nodePath)
	if err != nil {
		return
	}

	n.Exists = true

	// delete item link in trash
	if err = os.Remove(trashItem); err != nil {
		log.Error().Err(err).Str("trashItem", trashItem).Msg("error deleting trashitem")
	}
	return fs.tp.Propagate(ctx, n)

}

func (fs *s3ngfs) PurgeRecycleItem(ctx context.Context, key string) (err error) {
	log := appctx.GetLogger(ctx)

	var rn *node.Node
	var trashItem string
	var deletedNodePath string
	if rn, trashItem, deletedNodePath, _, err = node.ReadRecycleItem(ctx, fs.lu, key); err != nil {
		return
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

	if err = os.Remove(deletedNodePath); err != nil {
		log.Error().Err(err).Str("deletedNodePath", deletedNodePath).Msg("error deleting trash node")
		return
	}

	// delete blob from blobstore
	fs.Blobstore.Delete(rn.ID)

	// delete item link in trash
	if err = os.Remove(trashItem); err != nil {
		log.Error().Err(err).Str("trashItem", trashItem).Msg("error deleting trash item")
	}
	// TODO recursively delete all children
	return
}

func (fs *s3ngfs) EmptyRecycle(ctx context.Context) error {
	u, ok := user.ContextGetUser(ctx)
	// TODO what permission should we check? we could check the root node of the user? or the owner permissions on his home root node?
	// The current impl will wipe your own trash. or when no user provided the trash of 'root'
	if !ok {
		return os.RemoveAll(fs.getRecycleRoot(ctx))
	}

	// TODO use layout, see Tree.Delete() for problem
	return os.RemoveAll(filepath.Join(fs.o.Root, "trash", u.Id.OpaqueId))
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

func (fs *s3ngfs) getRecycleRoot(ctx context.Context) string {
	if fs.o.EnableHome {
		u := user.ContextMustGetUser(ctx)
		// TODO use layout, see Tree.Delete() for problem
		return filepath.Join(fs.o.Root, "trash", u.Id.OpaqueId)
	}
	return filepath.Join(fs.o.Root, "trash", "root")
}
