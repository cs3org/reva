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
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	iofs "io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/logger"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/v2/pkg/storage/utils/filelocks"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

//go:generate make --no-print-directory -C ../../../../.. mockery NAME=Blobstore

// Blobstore defines an interface for storing blobs in a blobstore
type Blobstore interface {
	Upload(node *node.Node, reader io.Reader) error
	Download(node *node.Node) (io.ReadCloser, error)
	Delete(node *node.Node) error
}

// PathLookup defines the interface for the lookup component
type PathLookup interface {
	NodeFromResource(ctx context.Context, ref *provider.Reference) (*node.Node, error)
	NodeFromID(ctx context.Context, id *provider.ResourceId) (n *node.Node, err error)

	InternalRoot() string
	InternalPath(spaceID, nodeID string) string
	Path(ctx context.Context, n *node.Node, hasPermission node.PermissionFunc) (path string, err error)
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
func (t *Tree) Setup() error {
	// create data paths for internal layout
	dataPaths := []string{
		filepath.Join(t.root, "spaces"),
		// notes contain symlinks from nodes/<u-u-i-d>/uploads/<uploadid> to ../../uploads/<uploadid>
		// better to keep uploads on a fast / volatile storage before a workflow finally moves them to the nodes dir
		filepath.Join(t.root, "uploads"),
	}
	for _, v := range dataPaths {
		err := os.MkdirAll(v, 0700)
		if err != nil {
			return err
		}
	}
	// Run migrations & return
	return t.runMigrations()
}

func (t *Tree) moveNode(spaceID, nodeID string) error {
	dirPath := filepath.Join(t.root, "nodes", nodeID)
	f, err := os.Open(dirPath)
	if err != nil {
		return err
	}
	children, err := f.Readdir(0)
	if err != nil {
		return err
	}
	for _, child := range children {
		old := filepath.Join(t.root, "nodes", child.Name())
		new := filepath.Join(t.root, "spaces", lookup.Pathify(spaceID, 1, 2), "nodes", lookup.Pathify(child.Name(), 4, 2))
		if err := os.Rename(old, new); err != nil {
			logger.New().Error().Err(err).
				Str("space", spaceID).
				Str("nodes", child.Name()).
				Str("oldpath", old).
				Str("newpath", new).
				Msg("could not rename node")
		}
		if child.IsDir() {
			if err := t.moveNode(spaceID, child.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *Tree) moveSpaceType(spaceType string) error {
	dirPath := filepath.Join(t.root, "spacetypes", spaceType)
	f, err := os.Open(dirPath)
	if err != nil {
		return err
	}
	children, err := f.Readdir(0)
	if err != nil {
		return err
	}
	for _, child := range children {
		old := filepath.Join(t.root, "spacetypes", spaceType, child.Name())
		target, err := os.Readlink(old)
		if err != nil {
			logger.New().Error().Err(err).
				Str("space", spaceType).
				Str("nodes", child.Name()).
				Str("oldLink", old).
				Msg("could not read old symlink")
			continue
		}
		newDir := filepath.Join(t.root, "indexes", "by-type", spaceType)
		if err := os.MkdirAll(newDir, 0700); err != nil {
			logger.New().Error().Err(err).
				Str("space", spaceType).
				Str("nodes", child.Name()).
				Str("targetDir", newDir).
				Msg("could not read old symlink")
		}
		newLink := filepath.Join(newDir, child.Name())
		if err := os.Symlink(filepath.Join("..", target), newLink); err != nil {
			logger.New().Error().Err(err).
				Str("space", spaceType).
				Str("nodes", child.Name()).
				Str("oldpath", old).
				Str("newpath", newLink).
				Msg("could not rename node")
			continue
		}
		if err := os.Remove(old); err != nil {
			logger.New().Error().Err(err).
				Str("space", spaceType).
				Str("nodes", child.Name()).
				Str("oldLink", old).
				Msg("could not remove old symlink")
			continue
		}
	}
	if err := os.Remove(dirPath); err != nil {
		logger.New().Error().Err(err).
			Str("space", spaceType).
			Str("dir", dirPath).
			Msg("could not remove spaces folder, folder probably not empty")
	}
	return nil
}

// linkSpace creates a new symbolic link for a space with the given type st, and node id
func (t *Tree) linkSpaceNode(spaceType, spaceID string) {
	spaceTypesPath := filepath.Join(t.root, "spacetypes", spaceType, spaceID)
	expectedTarget := "../../spaces/" + lookup.Pathify(spaceID, 1, 2) + "/nodes/" + lookup.Pathify(spaceID, 4, 2)
	linkTarget, err := os.Readlink(spaceTypesPath)
	if errors.Is(err, os.ErrNotExist) {
		err = os.Symlink(expectedTarget, spaceTypesPath)
		if err != nil {
			logger.New().Error().Err(err).
				Str("space_type", spaceType).
				Str("space", spaceID).
				Msg("could not create symlink")
		}
	} else {
		if err != nil {
			logger.New().Error().Err(err).
				Str("space_type", spaceType).
				Str("space", spaceID).
				Msg("could not read symlink")
		}
		if linkTarget != expectedTarget {
			logger.New().Warn().
				Str("space_type", spaceType).
				Str("space", spaceID).
				Str("expected", expectedTarget).
				Str("actual", linkTarget).
				Msg("expected a different link target")
		}
	}
}

// isRootNode checks if a node is a space root
func isRootNode(nodePath string) bool {
	attr, err := xattrs.Get(nodePath, xattrs.ParentidAttr)
	return err == nil && attr == node.RootID
}

// GetMD returns the metadata of a node in the tree
func (t *Tree) GetMD(ctx context.Context, n *node.Node) (os.FileInfo, error) {
	md, err := os.Stat(n.InternalPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errtypes.NotFound(n.ID)
		}
		return nil, errors.Wrap(err, "tree: error stating "+n.ID)
	}

	return md, nil
}

// TouchFile creates a new empty file
func (t *Tree) TouchFile(ctx context.Context, n *node.Node) error {
	if n.Exists {
		return errtypes.AlreadyExists(n.ID)
	}

	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	n.Type = provider.ResourceType_RESOURCE_TYPE_FILE

	nodePath := n.InternalPath()
	if err := os.MkdirAll(filepath.Dir(nodePath), 0700); err != nil {
		return errors.Wrap(err, "Decomposedfs: error creating node")
	}
	_, err := os.Create(nodePath)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: error creating node")
	}
	_, err = os.Create(xattrs.MetadataPath(nodePath))
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: error creating node")
	}

	err = n.WriteAllNodeMetadata()
	if err != nil {
		return err
	}

	// link child name to parent if it is new
	childNameLink := filepath.Join(n.ParentPath(), n.Name)
	var link string
	link, err = os.Readlink(childNameLink)
	if err == nil && link != "../"+n.ID {
		if err = os.Remove(childNameLink); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not remove symlink child entry")
		}
	}
	if errors.Is(err, iofs.ErrNotExist) || link != "../"+n.ID {
		relativeNodePath := filepath.Join("../../../../../", lookup.Pathify(n.ID, 4, 2))
		if err = os.Symlink(relativeNodePath, childNameLink); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not symlink child entry")
		}
	}

	return t.Propagate(ctx, n, 0)
}

// CreateDir creates a new directory entry in the tree
func (t *Tree) CreateDir(ctx context.Context, n *node.Node) (err error) {
	if n.Exists {
		return errtypes.AlreadyExists(n.ID) // path?
	}

	// create a directory node
	n.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	if n.ID == "" {
		n.ID = uuid.New().String()
	}

	err = t.createNode(n)
	if err != nil {
		return
	}

	if err := n.SetTreeSize(0); err != nil {
		return err
	}

	// make child appear in listings
	relativeNodePath := filepath.Join("../../../../../", lookup.Pathify(n.ID, 4, 2))
	err = os.Symlink(relativeNodePath, filepath.Join(n.ParentPath(), n.Name))
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
	return t.Propagate(ctx, n, 0)
}

// Move replaces the target with the source
func (t *Tree) Move(ctx context.Context, oldNode *node.Node, newNode *node.Node) (err error) {
	if oldNode.SpaceID != newNode.SpaceID {
		// WebDAV RFC https://www.rfc-editor.org/rfc/rfc4918#section-9.9.4 says to use
		// > 502 (Bad Gateway) - This may occur when the destination is on another
		// > server and the destination server refuses to accept the resource.
		// > This could also occur when the destination is on another sub-section
		// > of the same server namespace.
		// but we only have a not supported error
		return errtypes.NotSupported("cannot move across spaces")
	}
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
	// are we just renaming (parent stays the same)?
	if oldNode.ParentID == newNode.ParentID {

		// parentPath := t.lookup.InternalPath(oldNode.SpaceID, oldNode.ParentID)
		parentPath := oldNode.ParentPath()

		// rename child
		err = os.Rename(
			filepath.Join(parentPath, oldNode.Name),
			filepath.Join(parentPath, newNode.Name),
		)
		if err != nil {
			return errors.Wrap(err, "Decomposedfs: could not rename child")
		}

		// update name attribute
		if err := oldNode.SetXattr(xattrs.NameAttr, newNode.Name); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not set name attribute")
		}

		return t.Propagate(ctx, newNode, 0)
	}

	// we are moving the node to a new parent, any target has been removed
	// bring old node to the new parent

	// rename child
	err = os.Rename(
		filepath.Join(oldNode.ParentPath(), oldNode.Name),
		filepath.Join(newNode.ParentPath(), newNode.Name),
	)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: could not move child")
	}

	// update target parentid and name
	if err := oldNode.SetXattr(xattrs.ParentidAttr, newNode.ParentID); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not set parentid attribute")
	}
	if err := oldNode.SetXattr(xattrs.NameAttr, newNode.Name); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not set name attribute")
	}

	// the size diff is the current treesize or blobsize of the old/source node
	var sizeDiff int64
	if oldNode.IsDir() {
		treeSize, err := oldNode.GetTreeSize()
		if err != nil {
			return err
		}
		sizeDiff = int64(treeSize)
	} else {
		sizeDiff = oldNode.Blobsize
	}

	// TODO inefficient because we might update several nodes twice, only propagate unchanged nodes?
	// collect in a list, then only stat each node once
	// also do this in a go routine ... webdav should check the etag async

	err = t.Propagate(ctx, oldNode, -sizeDiff)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: Move: could not propagate old node")
	}
	err = t.Propagate(ctx, newNode, sizeDiff)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: Move: could not propagate new node")
	}
	return nil
}

func readChildNodeFromLink(path string) (string, error) {
	link, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	nodeID := strings.TrimLeft(link, "/.")
	nodeID = strings.ReplaceAll(nodeID, "/", "")
	return nodeID, nil
}

// ListFolder lists the content of a folder node
func (t *Tree) ListFolder(ctx context.Context, n *node.Node) ([]*node.Node, error) {
	dir := n.InternalPath()
	f, err := os.Open(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
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
		nodeID, err := readChildNodeFromLink(filepath.Join(dir, names[i]))
		if err != nil {
			return nil, err
		}

		child, err := node.ReadNode(ctx, t.lookup, n.SpaceID, nodeID, false)
		if err != nil {
			return nil, err
		}

		// prevent listing denied resources
		if child.IsDenied(ctx) {
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
		src := filepath.Join(n.ParentPath(), n.Name)
		return os.Remove(src)
	}

	// get the original path
	origin, err := t.lookup.Path(ctx, n, node.NoCheck)
	if err != nil {
		return
	}

	// set origin location in metadata
	nodePath := n.InternalPath()
	if err := n.SetXattr(xattrs.TrashOriginAttr, origin); err != nil {
		return err
	}

	var sizeDiff int64
	if n.IsDir() {
		treesize, err := n.GetTreeSize()
		if err != nil {
			return err // TODO calculate treesize if it is not set
		}
		sizeDiff = -int64(treesize)
	} else {
		sizeDiff = -n.Blobsize
	}

	deletionTime := time.Now().UTC().Format(time.RFC3339Nano)

	// Prepare the trash
	trashLink := filepath.Join(t.root, "spaces", lookup.Pathify(n.SpaceRoot.ID, 1, 2), "trash", lookup.Pathify(n.ID, 4, 2))
	if err := os.MkdirAll(filepath.Dir(trashLink), 0700); err != nil {
		// Roll back changes
		_ = n.RemoveXattr(xattrs.TrashOriginAttr)
		return err
	}

	// FIXME can we just move the node into the trash dir? instead of adding another symlink and appending a trash timestamp?
	// can we just use the mtime as the trash time?
	// TODO store a trashed by userid

	// first make node appear in the space trash
	// parent id and name are stored as extended attributes in the node itself
	err = os.Symlink("../../../../../nodes/"+lookup.Pathify(n.ID, 4, 2)+node.TrashIDDelimiter+deletionTime, trashLink)
	if err != nil {
		// Roll back changes
		_ = n.RemoveXattr(xattrs.TrashOriginAttr)
		return
	}

	// at this point we have a symlink pointing to a non existing destination, which is fine

	// rename the trashed node so it is not picked up when traversing up the tree and matches the symlink
	trashPath := nodePath + node.TrashIDDelimiter + deletionTime
	err = os.Rename(nodePath, trashPath)
	if err != nil {
		// To roll back changes
		// TODO remove symlink
		// Roll back changes
		_ = n.RemoveXattr(xattrs.TrashOriginAttr)
		return
	}
	err = os.Rename(xattrs.MetadataPath(nodePath), xattrs.MetadataPath(trashPath))
	if err != nil {
		_ = n.RemoveXattr(xattrs.TrashOriginAttr)
		_ = os.Rename(trashPath, nodePath)
		return
	}

	// Remove lock file if it exists
	_ = os.Remove(n.LockFilePath())

	// finally remove the entry from the parent dir
	err = os.Remove(filepath.Join(n.ParentPath(), n.Name))
	if err != nil {
		// To roll back changes
		// TODO revert the rename
		// TODO remove symlink
		// Roll back changes
		_ = n.RemoveXattr(xattrs.TrashOriginAttr)
		return
	}

	return t.Propagate(ctx, n, sizeDiff)
}

// RestoreRecycleItemFunc returns a node and a function to restore it from the trash.
func (t *Tree) RestoreRecycleItemFunc(ctx context.Context, spaceid, key, trashPath string, targetNode *node.Node) (*node.Node, *node.Node, func() error, error) {
	recycleNode, trashItem, deletedNodePath, origin, err := t.readRecycleItem(ctx, spaceid, key, trashPath)
	if err != nil {
		return nil, nil, nil, err
	}

	targetRef := &provider.Reference{
		ResourceId: &provider.ResourceId{SpaceId: spaceid, OpaqueId: spaceid},
		Path:       utils.MakeRelativePath(origin),
	}

	if targetNode == nil {
		targetNode, err = t.lookup.NodeFromResource(ctx, targetRef)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	if err := targetNode.CheckLock(ctx); err != nil {
		return nil, nil, nil, err
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
		err = os.Symlink("../../../../../"+lookup.Pathify(recycleNode.ID, 4, 2), filepath.Join(targetNode.ParentPath(), targetNode.Name))
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
			err = os.Rename(xattrs.MetadataPath(deletedNodePath), xattrs.MetadataPath(nodePath))
			if err != nil {
				return err
			}
		}

		targetNode.Exists = true
		// update name attribute
		if err := recycleNode.SetXattr(xattrs.NameAttr, targetNode.Name); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not set name attribute")
		}

		// set ParentidAttr to restorePath's node parent id
		if trashPath != "" {
			if err := recycleNode.SetXattr(xattrs.ParentidAttr, targetNode.ParentID); err != nil {
				return errors.Wrap(err, "Decomposedfs: could not set name attribute")
			}
		}

		// delete item link in trash
		deletePath := trashItem
		if trashPath != "" && trashPath != "/" {
			resolvedTrashRoot, err := filepath.EvalSymlinks(trashItem)
			if err != nil {
				return errors.Wrap(err, "Decomposedfs: could not resolve trash root")
			}
			deletePath = filepath.Join(resolvedTrashRoot, trashPath)
		}
		if err = os.Remove(deletePath); err != nil {
			log.Error().Err(err).Str("trashItem", trashItem).Msg("error deleting trash item")
		}

		var sizeDiff int64
		if recycleNode.IsDir() {
			treeSize, err := recycleNode.GetTreeSize()
			if err != nil {
				return err
			}
			sizeDiff = int64(treeSize)
		} else {
			sizeDiff = recycleNode.Blobsize
		}
		return t.Propagate(ctx, targetNode, sizeDiff)
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
		if err := t.removeNode(deletedNodePath, rn); err != nil {
			return err
		}

		// delete item link in trash
		deletePath := trashItem
		if path != "" && path != "/" {
			resolvedTrashRoot, err := filepath.EvalSymlinks(trashItem)
			if err != nil {
				return errors.Wrap(err, "Decomposedfs: could not resolve trash root")
			}
			deletePath = filepath.Join(resolvedTrashRoot, path)
		}
		if err = os.Remove(deletePath); err != nil {
			log.Error().Err(err).Str("deletePath", deletePath).Msg("error deleting trash item")
			return err
		}

		return nil
	}

	return rn, fn, nil
}

func (t *Tree) removeNode(path string, n *node.Node) error {
	// delete the actual node
	if err := utils.RemoveItem(path); err != nil {
		log.Error().Err(err).Str("path", path).Msg("error purging node")
		return err
	}

	// delete blob from blobstore
	if n.BlobID != "" {
		if err := t.DeleteBlob(n); err != nil {
			log.Error().Err(err).Str("blobID", n.BlobID).Msg("error purging nodes blob")
			return err
		}
	}

	if err := utils.RemoveItem(xattrs.MetadataPath(path)); err != nil {
		log.Error().Err(err).Str("path", xattrs.MetadataPath(path)).Msg("error purging node metadata")
		return err
	}

	// delete revisions
	revs, err := filepath.Glob(n.InternalPath() + node.RevisionIDDelimiter + "*")
	if err != nil {
		log.Error().Err(err).Str("path", n.InternalPath()+node.RevisionIDDelimiter+"*").Msg("glob failed badly")
		return err
	}
	for _, rev := range revs {
		bID, err := node.ReadBlobIDAttr(rev)
		if err != nil {
			log.Error().Err(err).Str("revision", rev).Msg("error reading blobid attribute")
			return err
		}

		if err := utils.RemoveItem(rev); err != nil {
			log.Error().Err(err).Str("revision", rev).Msg("error removing revision node")
			return err
		}

		if bID != "" {
			if err := t.DeleteBlob(&node.Node{SpaceID: n.SpaceID, BlobID: bID}); err != nil {
				log.Error().Err(err).Str("revision", rev).Str("blobID", bID).Msg("error removing revision node blob")
				return err
			}
		}

	}

	return nil
}

// Propagate propagates changes to the root of the tree
func (t *Tree) Propagate(ctx context.Context, n *node.Node, sizeDiff int64) (err error) {
	sublog := appctx.GetLogger(ctx).With().Str("spaceid", n.SpaceID).Str("nodeid", n.ID).Logger()
	if !t.treeTimeAccounting && !t.treeSizeAccounting {
		// no propagation enabled
		sublog.Debug().Msg("propagation disabled")
		return
	}

	// is propagation enabled for the parent node?
	root := n.SpaceRoot

	// use a sync time and don't rely on the mtime of the current node, as the stat might not change when a rename happened too quickly
	sTime := time.Now().UTC()

	// we loop until we reach the root
	for err == nil && n.ID != root.ID {
		sublog.Debug().Msg("propagating")

		if n, err = n.Parent(); err != nil {
			break
		}

		sublog = sublog.With().Str("spaceid", n.SpaceID).Str("nodeid", n.ID).Logger()

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
				if err = n.SetTMTime(&sTime); err != nil {
					sublog.Error().Err(err).Time("tmtime", sTime).Msg("could not update tmtime of parent node")
				} else {
					sublog.Debug().Time("tmtime", sTime).Msg("updated tmtime of parent node")
				}
			}

			if err := n.UnsetTempEtag(); err != nil && !xattrs.IsAttrUnset(err) {
				sublog.Error().Err(err).Msg("could not remove temporary etag attribute")
			}
		}

		// size accounting
		if t.treeSizeAccounting && sizeDiff != 0 {
			// lock node before reading treesize
			nodeLock, err := filelocks.AcquireWriteLock(n.InternalPath())
			if err != nil {
				return err
			}
			// always unlock node
			releaseLock := func() {
				// ReleaseLock returns nil if already unlocked
				if err := filelocks.ReleaseLock(nodeLock); err != nil {
					sublog.Err(err).Msg("Decomposedfs: could not unlock parent node")
				}
			}
			defer releaseLock()

			var newSize uint64

			// read treesize
			treeSize, err := n.GetTreeSize()
			switch {
			case xattrs.IsAttrUnset(err):
				// fallback to calculating the treesize
				newSize, err = calculateTreeSize(ctx, n.InternalPath())
				if err != nil {
					return err
				}
			case err != nil:
				return err
			default:
				if sizeDiff > 0 {
					newSize = treeSize + uint64(sizeDiff)
				} else {
					newSize = treeSize - uint64(-sizeDiff)
				}
			}

			// update the tree size of the node
			if err = n.SetXattrWithLock(xattrs.TreesizeAttr, strconv.FormatUint(newSize, 10), nodeLock); err != nil {
				return err
			}

			// Release node lock early, returns nil if already unlocked
			err = filelocks.ReleaseLock(nodeLock)
			if err != nil {
				return errtypes.InternalError(err.Error())
			}

			sublog.Debug().Uint64("newSize", newSize).Msg("updated treesize of parent node")
		}

	}
	if err != nil {
		sublog.Error().Err(err).Msg("error propagating")
		return
	}
	return
}

func calculateTreeSize(ctx context.Context, childrenPath string) (uint64, error) {
	var size uint64

	f, err := os.Open(childrenPath)
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("childrenPath", childrenPath).Msg("could not open dir")
		return 0, err
	}
	defer f.Close()

	names, err := f.Readdirnames(0)
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("childrenPath", childrenPath).Msg("could not read dirnames")
		return 0, err
	}
	for i := range names {
		cPath := filepath.Join(childrenPath, names[i])
		resolvedPath, err := filepath.EvalSymlinks(cPath)
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("childpath", cPath).Msg("could not resolve child entry symlink")
			continue // continue after an error
		}

		// raw read of the attributes for performance reasons
		attribs, err := xattrs.All(resolvedPath)
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("childpath", cPath).Msg("could not read attributes of child entry")
			continue // continue after an error
		}
		sizeAttr := ""
		if attribs[xattrs.TypeAttr] == strconv.FormatUint(uint64(provider.ResourceType_RESOURCE_TYPE_FILE), 10) {
			sizeAttr = attribs[xattrs.BlobsizeAttr]
		} else {
			sizeAttr = attribs[xattrs.TreesizeAttr]
		}
		csize, err := strconv.ParseInt(sizeAttr, 10, 64)
		if err != nil {
			return 0, errors.Wrapf(err, "invalid blobsize xattr format")
		}
		size += uint64(csize)
	}
	return size, err
}

// WriteBlob writes a blob to the blobstore
func (t *Tree) WriteBlob(node *node.Node, reader io.Reader) error {
	return t.blobstore.Upload(node, reader)
}

// ReadBlob reads a blob from the blobstore
func (t *Tree) ReadBlob(node *node.Node) (io.ReadCloser, error) {
	if node.BlobID == "" {
		// there is no blob yet - we are dealing with a 0 byte file
		return io.NopCloser(bytes.NewReader([]byte{})), nil
	}
	return t.blobstore.Download(node)
}

// DeleteBlob deletes a blob from the blobstore
func (t *Tree) DeleteBlob(node *node.Node) error {
	if node == nil {
		return fmt.Errorf("could not delete blob, nil node was given")
	}
	if node.BlobID == "" {
		return fmt.Errorf("could not delete blob, node with empty blob id was given")
	}

	return t.blobstore.Delete(node)
}

// TODO check if node exists?
func (t *Tree) createNode(n *node.Node) (err error) {
	// create a directory node
	nodePath := n.InternalPath()
	if err := os.MkdirAll(nodePath, 0700); err != nil {
		return errors.Wrap(err, "Decomposedfs: error creating node")
	}
	_, err = os.Create(xattrs.MetadataPath(nodePath))
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: error creating node")
	}

	return n.WriteAllNodeMetadata()
}

// TODO refactor the returned params into Node properties? would make all the path transformations go away...
func (t *Tree) readRecycleItem(ctx context.Context, spaceID, key, path string) (recycleNode *node.Node, trashItem string, deletedNodePath string, origin string, err error) {
	if key == "" {
		return nil, "", "", "", errtypes.InternalError("key is empty")
	}

	var nodeID string

	trashItem = filepath.Join(t.lookup.InternalRoot(), "spaces", lookup.Pathify(spaceID, 1, 2), "trash", lookup.Pathify(key, 4, 2))
	resolvedTrashItem, err := filepath.EvalSymlinks(trashItem)
	if err != nil {
		return
	}
	deletedNodePath = filepath.Join(resolvedTrashItem, path)
	nodeIDRegep := regexp.MustCompile(`.*/nodes/([^.]*).*`)
	nodeID = nodeIDRegep.ReplaceAllString(deletedNodePath, "$1")
	nodeID = strings.ReplaceAll(nodeID, "/", "")

	recycleNode = node.New(spaceID, nodeID, "", "", 0, "", nil, t.lookup)
	recycleNode.SpaceRoot, err = node.ReadNode(ctx, t.lookup, spaceID, spaceID, false)
	if err != nil {
		return
	}

	var attrStr string
	// lookup blobID in extended attributes
	if attrStr, err = xattrs.Get(deletedNodePath, xattrs.TypeAttr); err == nil {
		var typeAttr int64
		typeAttr, err = strconv.ParseInt(attrStr, 10, 64)
		if err != nil {
			return
		}
		recycleNode.Type = provider.ResourceType(typeAttr)
	} else {
		return
	}

	// lookup blobID in extended attributes
	if attrStr, err = xattrs.Get(deletedNodePath, xattrs.BlobIDAttr); err == nil {
		recycleNode.BlobID = attrStr
	} else {
		return
	}

	// lookup blobSize in extended attributes
	if recycleNode.Blobsize, err = xattrs.GetInt64(deletedNodePath, xattrs.BlobsizeAttr); err != nil {
		return
	}

	// lookup parent id in extended attributes
	if attrStr, err = xattrs.Get(deletedNodePath, xattrs.ParentidAttr); err == nil {
		recycleNode.ParentID = attrStr
	} else {
		return
	}

	// lookup name in extended attributes
	if attrStr, err = xattrs.Get(deletedNodePath, xattrs.NameAttr); err == nil {
		recycleNode.Name = attrStr
	} else {
		return
	}

	// get origin node, is relative to space root
	origin = "/"

	// lookup origin path in extended attributes
	if attrStr, err = xattrs.Get(resolvedTrashItem, xattrs.TrashOriginAttr); err == nil {
		origin = filepath.Join(attrStr, path)
	} else {
		log.Error().Err(err).Str("trashItem", trashItem).Str("deletedNodePath", deletedNodePath).Msg("could not read origin path, restoring to /")
	}

	return
}

// appendChildren appends `n` and all its children to `nodes`
func appendChildren(ctx context.Context, n *node.Node, nodes []*node.Node) ([]*node.Node, error) {
	nodes = append(nodes, n)

	children, err := os.ReadDir(n.InternalPath())
	if err != nil {
		// TODO: How to differentiate folders from files?
		return nodes, nil
	}

	for _, c := range children {
		cn, err := n.Child(ctx, c.Name())
		if err != nil {
			// continue?
			return nil, err
		}
		nodes, err = appendChildren(ctx, cn, nodes)
		if err != nil {
			// continue?
			return nil, err
		}
	}

	return nodes, nil
}
