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

package tree

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
	"github.com/rs/zerolog/log"
)

// go:generate mockery -name Blobstore

const (
	spaceTypePersonal = "personal"
	spaceTypeShare    = "share"
)

// Blobstore defines an interface for storing blobs in a blobstore
type Blobstore interface {
	Upload(key string, reader io.Reader) error
	Download(key string) (io.ReadCloser, error)
	Delete(key string) error
}

// PathLookup defines the interface for the lookup component
type PathLookup interface {
	NodeFromResource(ctx context.Context, ref *provider.Reference) (*node.Node, error)
	NodeFromID(ctx context.Context, id *provider.ResourceId) (n *node.Node, err error)
	RootNode(ctx context.Context) (node *node.Node, err error)

	InternalRoot() string
	InternalPath(ID string) string
	Path(ctx context.Context, n *node.Node) (path string, err error)
	ShareFolder() string
}

// Tree manages a hierarchical tree
type Tree struct {
	lookup    PathLookup
	blobstore Blobstore

	root               string
	treeSizeAccounting bool
	treeTimeAccounting bool
}

// PermissionCheckFunc defined a function used to check resource permissions
type PermissionCheckFunc func(rp *provider.ResourcePermissions) bool

// New returns a new instance of Tree
func New(root string, tta bool, tsa bool, lu PathLookup, bs Blobstore) *Tree {
	return &Tree{
		lookup:             lu,
		blobstore:          bs,
		root:               root,
		treeTimeAccounting: tta,
		treeSizeAccounting: tsa,
	}
}

// Setup prepares the tree structure
func (t *Tree) Setup(owner *userpb.UserId, propagateToRoot bool) error {
	// create data paths for internal layout
	dataPaths := []string{
		filepath.Join(t.root, "nodes"),
		// notes contain symlinks from nodes/<u-u-i-d>/uploads/<uploadid> to ../../uploads/<uploadid>
		// better to keep uploads on a fast / volatile storage before a workflow finally moves them to the nodes dir
		filepath.Join(t.root, "uploads"),
		filepath.Join(t.root, "trash"),
	}
	for _, v := range dataPaths {
		err := os.MkdirAll(v, 0700)
		if err != nil {
			return err
		}
	}

	// the root node has an empty name
	// the root node has no parent
	n := node.New("root", "", "", 0, "", nil, t.lookup)
	err := t.createNode(n, owner)
	if err != nil {
		return err
	}

	// set propagation flag
	v := []byte("0")
	if propagateToRoot {
		v = []byte("1")
	}
	if err = xattr.Set(n.InternalPath(), xattrs.PropagationAttr, v); err != nil {
		return err
	}

	// create spaces folder and iterate over existing nodes to populate it
	spacesPath := filepath.Join(t.root, "spaces")
	fi, err := os.Stat(spacesPath)
	if os.IsNotExist(err) {
		// create personal spaces dir
		if err := os.MkdirAll(filepath.Join(spacesPath, spaceTypePersonal), 0700); err != nil {
			return err
		}
		// create share spaces dir
		if err := os.MkdirAll(filepath.Join(spacesPath, spaceTypeShare), 0700); err != nil {
			return err
		}

		f, err := os.Open(filepath.Join(t.root, "nodes"))
		if err != nil {
			return err
		}
		nodes, err := f.Readdir(0)
		if err != nil {
			return err
		}

		for i := range nodes {
			nodePath := filepath.Join(t.root, "nodes", nodes[i].Name())

			// is it a user root? -> create personal space
			if isRootNode(nodePath) {
				// we can reuse the node id as the space id
				t.linkSpace(spaceTypePersonal, nodes[i].Name(), nodes[i].Name())
			}

			// is it a shared node? -> create share space
			if isSharedNode(nodePath) {
				// we can reuse the node id as the space id
				t.linkSpace(spaceTypeShare, nodes[i].Name(), nodes[i].Name())
			}
		}
	} else if !fi.IsDir() {
		// check if it is a directory
		return fmt.Errorf("%s is not a directory", spacesPath)
	}

	return nil
}

// linkSpace creates a new symbolic link for a space with the given type st, and node id
func (t *Tree) linkSpace(spaceType, spaceID, nodeID string) {
	spacesPath := filepath.Join(t.root, "spaces", spaceType, spaceID)
	expectedTarget := "../../nodes/" + nodeID
	linkTarget, err := os.Readlink(spacesPath)
	if errors.Is(err, os.ErrNotExist) {
		err = os.Symlink(expectedTarget, spacesPath)
		if err != nil {
			logger.New().Error().Err(err).
				Str("space_type", spaceType).
				Str("space", spaceID).
				Str("node", nodeID).
				Msg("could not create symlink")
		}
	} else {
		if err != nil {
			logger.New().Error().Err(err).
				Str("space_type", spaceType).
				Str("space", spaceID).
				Str("node", nodeID).
				Msg("could not read symlink")
		}
		if linkTarget != expectedTarget {
			logger.New().Warn().
				Str("space_type", spaceType).
				Str("space", spaceID).
				Str("node", nodeID).
				Str("expected", expectedTarget).
				Str("actual", linkTarget).
				Msg("expected a different link target")
		}
	}
}

func isRootNode(nodePath string) bool {
	attrBytes, err := xattr.Get(nodePath, xattrs.ParentidAttr)
	return err == nil && string(attrBytes) == "root"
}
func isSharedNode(nodePath string) bool {
	if attrs, err := xattr.List(nodePath); err == nil {
		for i := range attrs {
			if strings.HasPrefix(attrs[i], xattrs.GrantPrefix) {
				return true
			}
		}
	}
	return false
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

// CreateDir creates a new directory entry in the tree
func (t *Tree) CreateDir(ctx context.Context, n *node.Node) (err error) {

	if n.Exists {
		return errtypes.AlreadyExists(n.ID) // path?
	}

	// create a directory node
	if n.ID == "" {
		n.ID = uuid.New().String()
	}

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

	err = t.createNode(n, owner)
	if err != nil {
		return nil
	}

	// make child appear in listings
	err = os.Symlink("../"+n.ID, filepath.Join(t.lookup.InternalPath(n.ParentID), n.Name))
	if err != nil {
		// no better way to check unfortunately
		if !strings.Contains(err.Error(), "file exists") {
			return
		}

		// try to remove the node
		e := os.RemoveAll(n.InternalPath())
		if e != nil {
			appctx.GetLogger(ctx).Debug().Err(e).Msg("cannot delete node")
		}
		return errtypes.AlreadyExists(err.Error())
	}
	return t.Propagate(ctx, n)
}

// Move replaces the target with the source
func (t *Tree) Move(ctx context.Context, oldNode *node.Node, newNode *node.Node) (err error) {
	// if target exists delete it without trashing it
	if newNode.Exists {
		// TODO make sure all children are deleted
		if err := os.RemoveAll(newNode.InternalPath()); err != nil {
			return errors.Wrap(err, "Decomposedfs: Move: error deleting target node "+newNode.ID)
		}
	}

	// Always target the old node ID for xattr updates.
	// The new node id is empty if the target does not exist
	// and we need to overwrite the new one when overwriting an existing path.
	tgtPath := oldNode.InternalPath()

	// are we just renaming (parent stays the same)?
	if oldNode.ParentID == newNode.ParentID {

		parentPath := t.lookup.InternalPath(oldNode.ParentID)

		// rename child
		err = os.Rename(
			filepath.Join(parentPath, oldNode.Name),
			filepath.Join(parentPath, newNode.Name),
		)
		if err != nil {
			return errors.Wrap(err, "Decomposedfs: could not rename child")
		}

		// update name attribute
		if err := xattr.Set(tgtPath, xattrs.NameAttr, []byte(newNode.Name)); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not set name attribute")
		}

		return t.Propagate(ctx, newNode)
	}

	// we are moving the node to a new parent, any target has been removed
	// bring old node to the new parent

	// rename child
	err = os.Rename(
		filepath.Join(t.lookup.InternalPath(oldNode.ParentID), oldNode.Name),
		filepath.Join(t.lookup.InternalPath(newNode.ParentID), newNode.Name),
	)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: could not move child")
	}

	// update target parentid and name
	if err := xattr.Set(tgtPath, xattrs.ParentidAttr, []byte(newNode.ParentID)); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not set parentid attribute")
	}
	if err := xattr.Set(tgtPath, xattrs.NameAttr, []byte(newNode.Name)); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not set name attribute")
	}

	// TODO inefficient because we might update several nodes twice, only propagate unchanged nodes?
	// collect in a list, then only stat each node once
	// also do this in a go routine ... webdav should check the etag async

	err = t.Propagate(ctx, oldNode)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: Move: could not propagate old node")
	}
	err = t.Propagate(ctx, newNode)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: Move: could not propagate new node")
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

		child, err := node.ReadNode(ctx, t.lookup, filepath.Base(link))
		if err != nil {
			// TODO log
			continue
		}
		if child.SpaceRoot == nil {
			child.SpaceRoot = n.SpaceRoot
		}
		nodes = append(nodes, child)
	}
	return nodes, nil
}

// Delete deletes a node in the tree by moving it to the trash
func (t *Tree) Delete(ctx context.Context, n *node.Node) (err error) {
	deletingSharedResource := ctx.Value(appctx.DeletingSharedResource)

	if deletingSharedResource != nil && deletingSharedResource.(bool) {
		src := filepath.Join(t.lookup.InternalPath(n.ParentID), n.Name)
		return os.Remove(src)
	}
	// Prepare the trash
	err = os.MkdirAll(filepath.Join(t.root, "trash", n.SpaceRoot.ID), 0700)
	if err != nil {
		return
	}

	// get the original path
	origin, err := t.lookup.Path(ctx, n)
	if err != nil {
		return
	}

	// set origin location in metadata
	nodePath := n.InternalPath()
	if err := xattr.Set(nodePath, xattrs.TrashOriginAttr, []byte(origin)); err != nil {
		return err
	}

	deletionTime := time.Now().UTC().Format(time.RFC3339Nano)

	// first make node appear in the space trash
	// parent id and name are stored as extended attributes in the node itself
	trashLink := filepath.Join(t.root, "trash", n.SpaceRoot.ID, n.ID)
	err = os.Symlink("../../nodes/"+n.ID+".T."+deletionTime, trashLink)
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
	src := filepath.Join(t.lookup.InternalPath(n.ParentID), n.Name)
	err = os.Remove(src)
	if err != nil {
		// To roll back changes
		// TODO revert the rename
		// TODO remove symlink
		// TODO unset trashOriginAttr
		return
	}

	return t.Propagate(ctx, n)
}

// RestoreRecycleItemFunc returns a node and a function to restore it from the trash.
func (t *Tree) RestoreRecycleItemFunc(ctx context.Context, spaceid, key, trashPath string, targetNode *node.Node) (*node.Node, *node.Node, func() error, error) {
	recycleNode, trashItem, deletedNodePath, origin, err := t.readRecycleItem(ctx, spaceid, key, trashPath)
	if err != nil {
		return nil, nil, nil, err
	}

	targetRef := &provider.Reference{
		ResourceId: &provider.ResourceId{StorageId: spaceid, OpaqueId: spaceid},
		Path:       utils.MakeRelativePath(origin),
	}

	if targetNode == nil {
		targetNode, err = t.lookup.NodeFromResource(ctx, targetRef)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	parent, err := targetNode.Parent()
	if err != nil {
		return nil, nil, nil, err
	}

	fn := func() error {
		if targetNode.Exists {
			return errtypes.AlreadyExists("origin already exists")
		}

		// add the entry for the parent dir
		err = os.Symlink("../"+recycleNode.ID, filepath.Join(t.lookup.InternalPath(targetNode.ParentID), targetNode.Name))
		if err != nil {
			return err
		}

		// rename to node only name, so it is picked up by id
		nodePath := recycleNode.InternalPath()

		// attempt to rename only if we're not in a subfolder
		if deletedNodePath != nodePath {
			err = os.Rename(deletedNodePath, nodePath)
			if err != nil {
				return err
			}
		}

		// the new node will inherit the permissions of its parent
		p, err := targetNode.Parent()
		if err != nil {
			return err
		}

		po, err := p.Owner()
		if err != nil {
			return err
		}

		if err := recycleNode.ChangeOwner(po); err != nil {
			return err
		}

		targetNode.Exists = true
		// update name attribute
		if err := xattr.Set(nodePath, xattrs.NameAttr, []byte(targetNode.Name)); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not set name attribute")
		}

		// set ParentidAttr to restorePath's node parent id
		if trashPath != "" {
			if err := xattr.Set(nodePath, xattrs.ParentidAttr, []byte(targetNode.ParentID)); err != nil {
				return errors.Wrap(err, "Decomposedfs: could not set name attribute")
			}
		}

		// delete item link in trash
		if err = os.Remove(trashItem); err != nil {
			log.Error().Err(err).Str("trashItem", trashItem).Msg("error deleting trashitem")
		}
		return t.Propagate(ctx, targetNode)
	}
	return recycleNode, parent, fn, nil
}

// PurgeRecycleItemFunc returns a node and a function to purge it from the trash
func (t *Tree) PurgeRecycleItemFunc(ctx context.Context, spaceid, key string, path string) (*node.Node, func() error, error) {
	rn, trashItem, deletedNodePath, _, err := t.readRecycleItem(ctx, spaceid, key, path)
	if err != nil {
		return nil, nil, err
	}

	fn := func() error {
		if err := os.RemoveAll(deletedNodePath); err != nil {
			log.Error().Err(err).Str("deletedNodePath", deletedNodePath).Msg("error deleting trash node")
			return err
		}

		// delete blob from blobstore
		if rn.BlobID != "" {
			if err = t.DeleteBlob(rn.BlobID); err != nil {
				log.Error().Err(err).Str("trashItem", trashItem).Msg("error deleting trash item blob")
				return err
			}
		}

		// delete item link in trash
		if err = os.Remove(trashItem); err != nil {
			log.Error().Err(err).Str("trashItem", trashItem).Msg("error deleting trash item")
			return err
		}

		return nil
	}

	return rn, fn, nil
}

// Propagate propagates changes to the root of the tree
func (t *Tree) Propagate(ctx context.Context, n *node.Node) (err error) {
	sublog := appctx.GetLogger(ctx).With().Interface("node", n).Logger()
	if !t.treeTimeAccounting && !t.treeSizeAccounting {
		// no propagation enabled
		sublog.Debug().Msg("propagation disabled")
		return
	}

	// is propagation enabled for the parent node?

	var root *node.Node
	if n.SpaceRoot == nil {
		if root, err = t.lookup.RootNode(ctx); err != nil {
			return
		}
	} else {
		root = n.SpaceRoot
	}

	// use a sync time and don't rely on the mtime of the current node, as the stat might not change when a rename happened too quickly
	sTime := time.Now().UTC()

	// we loop until we reach the root
	for err == nil && n.ID != root.ID {
		sublog.Debug().Msg("propagating")

		if n, err = n.Parent(); err != nil {
			break
		}

		sublog = sublog.With().Interface("node", n).Logger()

		// TODO none, sync and async?
		if !n.HasPropagation() {
			sublog.Debug().Str("attr", xattrs.PropagationAttr).Msg("propagation attribute not set or unreadable, not propagating")
			// if the attribute is not set treat it as false / none / no propagation
			return nil
		}

		if t.treeTimeAccounting {
			// update the parent tree time if it is older than the nodes mtime
			updateSyncTime := false

			var tmTime time.Time
			tmTime, err = n.GetTMTime()
			switch {
			case err != nil:
				// missing attribute, or invalid format, overwrite
				sublog.Debug().Err(err).
					Msg("could not read tmtime attribute, overwriting")
				updateSyncTime = true
			case tmTime.Before(sTime):
				sublog.Debug().
					Time("tmtime", tmTime).
					Time("stime", sTime).
					Msg("parent tmtime is older than node mtime, updating")
				updateSyncTime = true
			default:
				sublog.Debug().
					Time("tmtime", tmTime).
					Time("stime", sTime).
					Dur("delta", sTime.Sub(tmTime)).
					Msg("parent tmtime is younger than node mtime, not updating")
			}

			if updateSyncTime {
				// update the tree time of the parent node
				if err = n.SetTMTime(sTime); err != nil {
					sublog.Error().Err(err).Time("tmtime", sTime).Msg("could not update tmtime of parent node")
				} else {
					sublog.Debug().Time("tmtime", sTime).Msg("updated tmtime of parent node")
				}
			}

			if err := n.UnsetTempEtag(); err != nil {
				sublog.Error().Err(err).Msg("could not remove temporary etag attribute")
			}
		}

		// size accounting
		if t.treeSizeAccounting {
			// update the treesize if it differs from the current size
			updateTreeSize := false

			var treeSize, calculatedTreeSize uint64
			calculatedTreeSize, err = calculateTreeSize(ctx, n.InternalPath())
			if err != nil {
				continue
			}

			treeSize, err = n.GetTreeSize()
			switch {
			case err != nil:
				// missing attribute, or invalid format, overwrite
				sublog.Debug().Err(err).Msg("could not read treesize attribute, overwriting")
				updateTreeSize = true
			case treeSize != calculatedTreeSize:
				sublog.Debug().
					Uint64("treesize", treeSize).
					Uint64("calculatedTreeSize", calculatedTreeSize).
					Msg("parent treesize is different then calculated treesize, updating")
				updateTreeSize = true
			default:
				sublog.Debug().
					Uint64("treesize", treeSize).
					Uint64("calculatedTreeSize", calculatedTreeSize).
					Msg("parent size matches calculated size, not updating")
			}

			if updateTreeSize {
				// update the tree time of the parent node
				if err = n.SetTreeSize(calculatedTreeSize); err != nil {
					sublog.Error().Err(err).Uint64("calculatedTreeSize", calculatedTreeSize).Msg("could not update treesize of parent node")
				} else {
					sublog.Debug().Uint64("calculatedTreeSize", calculatedTreeSize).Msg("updated treesize of parent node")
				}
			}
		}
	}
	if err != nil {
		sublog.Error().Err(err).Msg("error propagating")
		return
	}
	return
}

func calculateTreeSize(ctx context.Context, nodePath string) (uint64, error) {
	var size uint64

	f, err := os.Open(nodePath)
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("nodepath", nodePath).Msg("could not open dir")
		return 0, err
	}
	defer f.Close()

	names, err := f.Readdirnames(0)
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("nodepath", nodePath).Msg("could not read dirnames")
		return 0, err
	}
	for i := range names {
		cPath := filepath.Join(nodePath, names[i])
		info, err := os.Stat(cPath)
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("childpath", cPath).Msg("could not stat child entry")
			continue // continue after an error
		}
		if !info.IsDir() {
			blobSize, err := node.ReadBlobSizeAttr(cPath)
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Str("childpath", cPath).Msg("could not read blobSize xattr")
				continue // continue after an error
			}
			size += uint64(blobSize)
		} else {
			// read from attr
			var b []byte
			// xattr.Get will follow the symlink
			if b, err = xattr.Get(cPath, xattrs.TreesizeAttr); err != nil {
				// TODO recursively descend and recalculate treesize
				continue // continue after an error
			}
			csize, err := strconv.ParseUint(string(b), 10, 64)
			if err != nil {
				// TODO recursively descend and recalculate treesize
				continue // continue after an error
			}
			size += csize
		}
	}
	return size, err

}

// WriteBlob writes a blob to the blobstore
func (t *Tree) WriteBlob(key string, reader io.Reader) error {
	return t.blobstore.Upload(key, reader)
}

// ReadBlob reads a blob from the blobstore
func (t *Tree) ReadBlob(key string) (io.ReadCloser, error) {
	return t.blobstore.Download(key)
}

// DeleteBlob deletes a blob from the blobstore
func (t *Tree) DeleteBlob(key string) error {
	if key == "" {
		return fmt.Errorf("could not delete blob, empty key was given")
	}

	return t.blobstore.Delete(key)
}

// TODO check if node exists?
func (t *Tree) createNode(n *node.Node, owner *userpb.UserId) (err error) {
	// create a directory node
	nodePath := n.InternalPath()
	if err = os.MkdirAll(nodePath, 0700); err != nil {
		return errors.Wrap(err, "Decomposedfs: error creating node")
	}

	return n.WriteMetadata(owner)
}

// TODO refactor the returned params into Node properties? would make all the path transformations go away...
func (t *Tree) readRecycleItem(ctx context.Context, spaceid, key, path string) (recycleNode *node.Node, trashItem string, deletedNodePath string, origin string, err error) {
	if key == "" {
		return nil, "", "", "", errtypes.InternalError("key is empty")
	}

	trashItem = filepath.Join(t.lookup.InternalRoot(), "trash", spaceid, key, path)

	var link string
	link, err = os.Readlink(trashItem)
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("trashItem", trashItem).Msg("error reading trash link")
		return
	}

	var attrBytes []byte
	trashNodeID := filepath.Base(link)
	deletedNodePath = t.lookup.InternalPath(trashNodeID)

	owner := &userpb.UserId{}
	// lookup ownerId in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, xattrs.OwnerIDAttr); err == nil {
		owner.OpaqueId = string(attrBytes)
	} else {
		return
	}
	// lookup ownerIdp in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, xattrs.OwnerIDPAttr); err == nil {
		owner.Idp = string(attrBytes)
	} else {
		return
	}
	// lookup ownerType in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, xattrs.OwnerTypeAttr); err == nil {
		owner.Type = utils.UserTypeMap(string(attrBytes))
	} else {
		return
	}

	recycleNode = node.New(trashNodeID, "", "", 0, "", owner, t.lookup)
	// lookup blobID in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, xattrs.BlobIDAttr); err == nil {
		recycleNode.BlobID = string(attrBytes)
	} else {
		return
	}

	// lookup parent id in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, xattrs.ParentidAttr); err == nil {
		recycleNode.ParentID = string(attrBytes)
	} else {
		return
	}

	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, xattrs.NameAttr); err == nil {
		recycleNode.Name = string(attrBytes)
	} else {
		return
	}

	// look up space root from the trashed node
	err = recycleNode.FindStorageSpaceRoot()

	if path == "" || path == "/" {
		parts := strings.SplitN(filepath.Base(link), ".T.", 2)
		if len(parts) != 2 {
			appctx.GetLogger(ctx).Error().Err(err).Str("trashItem", trashItem).Interface("parts", parts).Msg("malformed trash link")
			return
		}
		// update the node id, drop the `.T.{timestamp}` suffix
		recycleNode.ID = parts[0]
	}

	// get origin node, is relative to space root
	origin = "/"

	deletedNodeRootPath := deletedNodePath
	if path != "" && path != "/" {
		trashItemRoot := filepath.Join(t.lookup.InternalRoot(), "trash", spaceid, key)
		var rootLink string
		rootLink, err = os.Readlink(trashItemRoot)
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("trashItem", trashItem).Msg("error reading trash link")
			return
		}
		deletedNodeRootPath = t.lookup.InternalPath(filepath.Base(rootLink))
	}
	// lookup origin path in extended attributes
	if attrBytes, err = xattr.Get(deletedNodeRootPath, xattrs.TrashOriginAttr); err == nil {
		origin = filepath.Join(string(attrBytes), path)
	} else {
		log.Error().Err(err).Str("trashItem", trashItem).Str("link", link).Str("deletedNodePath", deletedNodePath).Msg("could not read origin path, restoring to /")
	}

	return
}
