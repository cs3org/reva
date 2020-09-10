// Copyright 2018-2020 CERN
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

package ocis

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

func (fs *ocisfs) PurgeRecycleItem(ctx context.Context, key string) error {
	return errtypes.NotSupported("operation not supported: PurgeRecycleItem")
}

func (fs *ocisfs) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("operation not supported: EmptyRecycle")
}

func (fs *ocisfs) ListRecycle(ctx context.Context) (items []*provider.RecycleItem, err error) {
	log := appctx.GetLogger(ctx)

	var trashRoot string
	if fs.pw.EnableHome {
		u := user.ContextMustGetUser(ctx)
		// TODO use layout, see Tree.Delete() for problem
		trashRoot = filepath.Join(fs.pw.Root, "trash", u.Id.OpaqueId)
	} else {
		trashRoot = filepath.Join(fs.pw.Root, "trash", "root")
	}

	items = make([]*provider.RecycleItem, 0)

	f, err := os.Open(trashRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return items, nil
		}
		return nil, errors.Wrap(err, "tree: error listing "+trashRoot)
	}

	names, err := f.Readdirnames(0)
	if err != nil {
		return nil, err
	}
	for i := range names {
		var link string
		link, err = os.Readlink(filepath.Join(trashRoot, names[i]))
		if err != nil {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Msg("error reading trash link, skipping")
			err = nil
			continue
		}
		parts := strings.SplitN(link, ".T.", 2)
		if len(parts) != 2 {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("link", link).Interface("parts", parts).Msg("malformed trash link, skipping")
			continue
		}

		nodePath := filepath.Join(fs.pw.Root, "nodes", filepath.Base(link))
		md, err := os.Stat(nodePath)
		if err != nil {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("link", link).Interface("parts", parts).Msg("could not stat trash item, skipping")
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
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("link", link).Interface("parts", parts).Msg("could parse time format, ignoring")
		}

		// lookup parent id in extended attributes
		var attrBytes []byte
		if attrBytes, err = xattr.Get(nodePath, trashOriginAttr); err == nil {
			item.Path = string(attrBytes)
		} else {
			log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("link", link).Msg("could not read origin path, skipping")
			continue
		}
		// lookup name in extended attributes
		/*
			if attrBytes, err = xattr.Get(nodePath, "user.ocis.name"); err == nil {
				item.Key = string(attrBytes)
			} else {
				log.Error().Err(err).Str("trashRoot", trashRoot).Str("name", names[i]).Str("link", link).Msg("could not read name, skipping")
				continue
			}
		*/

		items = append(items, item)
	}
	return
}

func (fs *ocisfs) RestoreRecycleItem(ctx context.Context, key string) (err error) {
	log := appctx.GetLogger(ctx)

	if key == "" {
		return errtypes.InternalError("key is empty")
	}

	var trashItem string
	if fs.pw.EnableHome {
		u := user.ContextMustGetUser(ctx)
		// TODO use layout, see Tree.Delete() for problem
		trashItem = filepath.Join(fs.pw.Root, "trash", u.Id.OpaqueId, key)
	} else {
		trashItem = filepath.Join(fs.pw.Root, "trash", "root", key)
	}

	var link string
	link, err = os.Readlink(trashItem)
	if err != nil {
		log.Error().Err(err).Str("trashItem", trashItem).Msg("error reading trash link")
		return
	}
	parts := strings.SplitN(link, ".T.", 2)
	if len(parts) != 2 {
		log.Error().Err(err).Str("trashItem", trashItem).Interface("parts", parts).Msg("malformed trash link")
		return
	}

	deletedNodePath := filepath.Join(fs.pw.Root, "nodes", filepath.Base(link))

	// get origin node
	origin := "/"

	// lookup parent id in extended attributes
	var attrBytes []byte
	if attrBytes, err = xattr.Get(deletedNodePath, trashOriginAttr); err == nil {
		origin = string(attrBytes)
	} else {
		log.Error().Err(err).Str("trashItem", trashItem).Str("link", link).Str("deletedNodePath", deletedNodePath).Msg("could not read origin path, restoring to /")
	}

	// link to origin
	var n *Node
	n, err = fs.pw.NodeFromPath(ctx, origin)
	if err != nil {
		return
	}

	if n.Exists {
		return errtypes.AlreadyExists("origin already exists")
	}

	// rename to node only name, so it is picked up by id
	nodePath := filepath.Join(fs.pw.Root, "nodes", parts[0])
	err = os.Rename(deletedNodePath, nodePath)
	if err != nil {
		return
	}

	// add the entry for the parent dir
	err = os.Symlink("../"+parts[0], filepath.Join(fs.pw.Root, "nodes", n.ParentID, n.Name))
	if err != nil {
		return
	}
	n.Exists = true

	if err = os.Remove(trashItem); err != nil {
		log.Error().Err(err).Str("trashItem", trashItem).Msg("error deleting trashitem")
	}
	return fs.tp.Propagate(ctx, n)

}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}
