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
	"strconv"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/node"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/xattrs"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
	"github.com/rs/zerolog/log"
)

// Tree manages a hierarchical tree
type Tree struct {
	lu *Lookup
}

// NewTree creates a new Tree instance
func NewTree(lu *Lookup) (TreePersistence, error) {
	return &Tree{
		lu: lu,
	}, nil
}

// GetMD returns the metadata of a node in the tree
func (t *Tree) GetMD(ctx context.Context, n *node.Node) (os.FileInfo, error) {
	md, err := os.Stat(n.InternalPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(n.ID)
		}
		return nil, errors.Wrap(err, "tree: error stating "+n.ID)
	}

	return md, nil
}

// GetPathByID returns the fn pointed by the file id, without the internal namespace
func (t *Tree) GetPathByID(ctx context.Context, id *provider.ResourceId) (relativeExternalPath string, err error) {
	var node *node.Node
	node, err = t.lu.NodeFromID(ctx, id)
	if err != nil {
		return
	}

	relativeExternalPath, err = t.lu.Path(ctx, node)
	return
}

// does not take care of linking back to parent
// TODO check if node exists?
func createNode(n *node.Node, owner *userpb.UserId) (err error) {
	// create a directory node
	nodePath := n.InternalPath()
	if err = os.MkdirAll(nodePath, 0700); err != nil {
		return errors.Wrap(err, "s3ngfs: error creating node")
	}

	return n.WriteMetadata(owner)
}

// CreateDir creates a new directory entry in the tree
func (t *Tree) CreateDir(ctx context.Context, n *node.Node) (err error) {

	if n.Exists || n.ID != "" {
		return errtypes.AlreadyExists(n.ID) // path?
	}

	// create a directory node
	n.ID = uuid.New().String()

	// who will become the owner? the owner of the parent node, not the current user
	var p *node.Node
	p, err = n.Parent()
	if err != nil {
		return
	}
	var owner *userpb.UserId
	owner, err = p.Owner()
	if err != nil {
		return
	}

	err = createNode(n, owner)
	if err != nil {
		return nil
	}

	// make child appear in listings
	err = os.Symlink("../"+n.ID, filepath.Join(t.lu.InternalPath(n.ParentID), n.Name))
	if err != nil {
		return
	}
	return t.Propagate(ctx, n)
}

// Move replaces the target with the source
func (t *Tree) Move(ctx context.Context, oldNode *node.Node, newNode *node.Node) (err error) {
	// if target exists delete it without trashing it
	if newNode.Exists {
		// TODO make sure all children are deleted
		if err := os.RemoveAll(newNode.InternalPath()); err != nil {
			return errors.Wrap(err, "s3ngfs: Move: error deleting target node "+newNode.ID)
		}
	}

	// Always target the old node ID for xattr updates.
	// The new node id is empty if the target does not exist
	// and we need to overwrite the new one when overwriting an existing path.
	tgtPath := oldNode.InternalPath()

	// are we just renaming (parent stays the same)?
	if oldNode.ParentID == newNode.ParentID {

		parentPath := t.lu.InternalPath(oldNode.ParentID)

		// rename child
		err = os.Rename(
			filepath.Join(parentPath, oldNode.Name),
			filepath.Join(parentPath, newNode.Name),
		)
		if err != nil {
			return errors.Wrap(err, "s3ngfs: could not rename child")
		}

		// update name attribute
		if err := xattr.Set(tgtPath, xattrs.NameAttr, []byte(newNode.Name)); err != nil {
			return errors.Wrap(err, "s3ngfs: could not set name attribute")
		}

		return t.Propagate(ctx, newNode)
	}

	// we are moving the node to a new parent, any target has been removed
	// bring old node to the new parent

	// rename child
	err = os.Rename(
		filepath.Join(t.lu.InternalPath(oldNode.ParentID), oldNode.Name),
		filepath.Join(t.lu.InternalPath(newNode.ParentID), newNode.Name),
	)
	if err != nil {
		return errors.Wrap(err, "s3ngfs: could not move child")
	}

	// update target parentid and name
	if err := xattr.Set(tgtPath, xattrs.ParentidAttr, []byte(newNode.ParentID)); err != nil {
		return errors.Wrap(err, "s3ngfs: could not set parentid attribute")
	}
	if err := xattr.Set(tgtPath, xattrs.NameAttr, []byte(newNode.Name)); err != nil {
		return errors.Wrap(err, "s3ngfs: could not set name attribute")
	}

	// TODO inefficient because we might update several nodes twice, only propagate unchanged nodes?
	// collect in a list, then only stat each node once
	// also do this in a go routine ... webdav should check the etag async

	err = t.Propagate(ctx, oldNode)
	if err != nil {
		return errors.Wrap(err, "s3ngfs: Move: could not propagate old node")
	}
	err = t.Propagate(ctx, newNode)
	if err != nil {
		return errors.Wrap(err, "s3ngfs: Move: could not propagate new node")
	}
	return nil
}

// ListFolder lists the content of a folder node
func (t *Tree) ListFolder(ctx context.Context, n *node.Node) ([]*node.Node, error) {
	dir := n.InternalPath()
	f, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(dir)
		}
		return nil, errors.Wrap(err, "tree: error listing "+dir)
	}
	defer f.Close()

	names, err := f.Readdirnames(0)
	if err != nil {
		return nil, err
	}
	nodes := []*node.Node{}
	for i := range names {
		link, err := os.Readlink(filepath.Join(dir, names[i]))
		if err != nil {
			// TODO log
			continue
		}
		blobSizeString, err := xattr.Get(link, xattrs.BlobsizeAttr)
		blobSize, err := strconv.ParseInt(string(blobSizeString), 10, 64)
		if err != nil {
			// TODO log
			continue
		}
		nodes = append(nodes, node.New(filepath.Base(link), n.ID, names[i], blobSize, nil, t.lu))
	}
	return nodes, nil
}

// Delete deletes a node in the tree
func (t *Tree) Delete(ctx context.Context, n *node.Node) (err error) {

	// Prepare the trash
	// TODO use layout?, but it requires resolving the owners user if the username is used instead of the id.
	// the node knows the owner id so we use that for now
	o, err := n.Owner()
	if err != nil {
		return
	}
	if o.OpaqueId == "" {
		// fall back to root trash
		o.OpaqueId = "root"
	}
	err = os.MkdirAll(filepath.Join(t.lu.Options.Root, "trash", o.OpaqueId), 0700)
	if err != nil {
		return
	}

	// get the original path
	origin, err := t.lu.Path(ctx, n)
	if err != nil {
		return
	}

	// set origin location in metadata
	nodePath := n.InternalPath()
	if err := xattr.Set(nodePath, xattrs.TrashOriginAttr, []byte(origin)); err != nil {
		return err
	}

	deletionTime := time.Now().UTC().Format(time.RFC3339Nano)

	// first make node appear in the owners (or root) trash
	// parent id and name are stored as extended attributes in the node itself
	trashLink := filepath.Join(t.lu.Options.Root, "trash", o.OpaqueId, n.ID)
	err = os.Symlink("../nodes/"+n.ID+".T."+deletionTime, trashLink)
	if err != nil {
		// To roll back changes
		// TODO unset trashOriginAttr
		return
	}

	// at this point we have a symlink pointing to a non existing destination, which is fine

	// rename the trashed node so it is not picked up when traversing up the tree and matches the symlink
	trashPath := nodePath + ".T." + deletionTime
	err = os.Rename(nodePath, trashPath)
	if err != nil {
		// To roll back changes
		// TODO remove symlink
		// TODO unset trashOriginAttr
		return
	}

	// finally remove the entry from the parent dir
	src := filepath.Join(t.lu.InternalPath(n.ParentID), n.Name)
	err = os.Remove(src)
	if err != nil {
		// To roll back changes
		// TODO revert the rename
		// TODO remove symlink
		// TODO unset trashOriginAttr
		return
	}

	p, err := n.Parent()
	if err != nil {
		return errors.Wrap(err, "s3ngfs: error getting parent "+n.ParentID)
	}
	return t.Propagate(ctx, p)
}

// Propagate propagates changes to the root of the tree
func (t *Tree) Propagate(ctx context.Context, n *node.Node) (err error) {
	if !t.lu.Options.TreeTimeAccounting && !t.lu.Options.TreeSizeAccounting {
		// no propagation enabled
		log.Debug().Msg("propagation disabled")
		return
	}
	log := appctx.GetLogger(ctx)

	// is propagation enabled for the parent node?

	var root *node.Node
	if root, err = t.lu.HomeOrRootNode(ctx); err != nil {
		return
	}

	// use a sync time and don't rely on the mtime of the current node, as the stat might not change when a rename happened too quickly
	sTime := time.Now().UTC()

	for err == nil && n.ID != root.ID {
		log.Debug().Interface("node", n).Msg("propagating")

		if n, err = n.Parent(); err != nil {
			break
		}

		// TODO none, sync and async?
		if !n.HasPropagation() {
			log.Debug().Interface("node", n).Str("attr", xattrs.PropagationAttr).Msg("propagation attribute not set or unreadable, not propagating")
			// if the attribute is not set treat it as false / none / no propagation
			return nil
		}

		if t.lu.Options.TreeTimeAccounting {
			// update the parent tree time if it is older than the nodes mtime
			updateSyncTime := false

			var tmTime time.Time
			tmTime, err = n.GetTMTime()
			switch {
			case err != nil:
				// missing attribute, or invalid format, overwrite
				log.Debug().Err(err).
					Interface("node", n).
					Msg("could not read tmtime attribute, overwriting")
				updateSyncTime = true
			case tmTime.Before(sTime):
				log.Debug().
					Interface("node", n).
					Time("tmtime", tmTime).
					Time("stime", sTime).
					Msg("parent tmtime is older than node mtime, updating")
				updateSyncTime = true
			default:
				log.Debug().
					Interface("node", n).
					Time("tmtime", tmTime).
					Time("stime", sTime).
					Dur("delta", sTime.Sub(tmTime)).
					Msg("parent tmtime is younger than node mtime, not updating")
			}

			if updateSyncTime {
				// update the tree time of the parent node
				if err = n.SetTMTime(sTime); err != nil {
					log.Error().Err(err).Interface("node", n).Time("tmtime", sTime).Msg("could not update tmtime of parent node")
					return
				}
				log.Debug().Interface("node", n).Time("tmtime", sTime).Msg("updated tmtime of parent node")
			}

			if err := n.UnsetTempEtag(); err != nil {
				log.Error().Err(err).Interface("node", n).Msg("could not remove temporary etag attribute")
			}

		}

		// TODO size accounting

	}
	if err != nil {
		log.Error().Err(err).Interface("node", n).Msg("error propagating")
		return
	}
	return
}
