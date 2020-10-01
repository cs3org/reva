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
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
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
func (t *Tree) GetMD(ctx context.Context, node *Node) (os.FileInfo, error) {
	md, err := os.Stat(t.lu.toInternalPath(node.ID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(node.ID)
		}
		return nil, errors.Wrap(err, "tree: error stating "+node.ID)
	}

	return md, nil
}

// GetPathByID returns the fn pointed by the file id, without the internal namespace
func (t *Tree) GetPathByID(ctx context.Context, id *provider.ResourceId) (relativeExternalPath string, err error) {
	var node *Node
	node, err = t.lu.NodeFromID(ctx, id)
	if err != nil {
		return
	}

	relativeExternalPath, err = t.lu.Path(ctx, node)
	return
}

// does not take care of linking back to parent
// TODO check if node exists?
func createNode(n *Node, owner *userpb.UserId) (err error) {
	// create a directory node
	nodePath := n.lu.toInternalPath(n.ID)
	if err = os.MkdirAll(nodePath, 0700); err != nil {
		return errors.Wrap(err, "ocisfs: error creating node")
	}

	return n.writeMetadata(owner)
}

// CreateDir creates a new directory entry in the tree
func (t *Tree) CreateDir(ctx context.Context, node *Node) (err error) {

	if node.Exists || node.ID != "" {
		return errtypes.AlreadyExists(node.ID) // path?
	}

	// create a directory node
	node.ID = uuid.New().String()

	if t.lu.Options.EnableHome {
		if u, ok := user.ContextGetUser(ctx); ok {
			err = createNode(node, u.Id)
		} else {
			log := appctx.GetLogger(ctx)
			log.Error().Msg("home support enabled but no user in context")
			err = errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
		}
	} else {
		err = createNode(node, nil)
	}
	if err != nil {
		return nil
	}

	// make child appear in listings
	err = os.Symlink("../"+node.ID, filepath.Join(t.lu.toInternalPath(node.ParentID), node.Name))
	if err != nil {
		return
	}
	return t.Propagate(ctx, node)
}

// Move replaces the target with the source
func (t *Tree) Move(ctx context.Context, oldNode *Node, newNode *Node) (err error) {
	// if target exists delete it without trashing it
	if newNode.Exists {
		// TODO make sure all children are deleted
		if err := os.RemoveAll(t.lu.toInternalPath(newNode.ID)); err != nil {
			return errors.Wrap(err, "ocisfs: Move: error deleting target node "+newNode.ID)
		}
	}
	// are we just renaming (parent stays the same)?
	if oldNode.ParentID == newNode.ParentID {

		parentPath := t.lu.toInternalPath(oldNode.ParentID)

		// rename child
		err = os.Rename(
			filepath.Join(parentPath, oldNode.Name),
			filepath.Join(parentPath, newNode.Name),
		)
		if err != nil {
			return errors.Wrap(err, "ocisfs: could not rename child")
		}

		// the new node id might be different, so we need to use the old nodes id
		tgtPath := t.lu.toInternalPath(oldNode.ID)

		// update name attribute
		if err := xattr.Set(tgtPath, nameAttr, []byte(newNode.Name)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set name attribute")
		}

		return t.Propagate(ctx, newNode)
	}

	// we are moving the node to a new parent, any target has been removed
	// bring old node to the new parent

	// rename child
	err = os.Rename(
		filepath.Join(t.lu.toInternalPath(oldNode.ParentID), oldNode.Name),
		filepath.Join(t.lu.toInternalPath(newNode.ParentID), newNode.Name),
	)
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not move child")
	}

	// update parentid and name
	tgtPath := t.lu.toInternalPath(newNode.ID)

	if err := xattr.Set(tgtPath, parentidAttr, []byte(newNode.ParentID)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set parentid attribute")
	}
	if err := xattr.Set(tgtPath, nameAttr, []byte(newNode.Name)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set name attribute")
	}

	// TODO inefficient because we might update several nodes twice, only propagate unchanged nodes?
	// collect in a list, then only stat each node once
	// also do this in a go routine ... webdav should check the etag async

	err = t.Propagate(ctx, oldNode)
	if err != nil {
		return errors.Wrap(err, "ocisfs: Move: could not propagate old node")
	}
	err = t.Propagate(ctx, newNode)
	if err != nil {
		return errors.Wrap(err, "ocisfs: Move: could not propagate new node")
	}
	return nil
}

// ListFolder lists the content of a folder node
func (t *Tree) ListFolder(ctx context.Context, node *Node) ([]*Node, error) {

	dir := t.lu.toInternalPath(node.ID)
	f, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(dir)
		}
		return nil, errors.Wrap(err, "tree: error listing "+dir)
	}

	names, err := f.Readdirnames(0)
	if err != nil {
		return nil, err
	}
	nodes := []*Node{}
	for i := range names {
		link, err := os.Readlink(filepath.Join(dir, names[i]))
		if err != nil {
			// TODO log
			continue
		}
		n := &Node{
			lu:       t.lu,
			ParentID: node.ID,
			ID:       filepath.Base(link),
			Name:     names[i],
			Exists:   true, // TODO
		}

		nodes = append(nodes, n)
	}
	return nodes, nil
}

// Delete deletes a node in the tree
func (t *Tree) Delete(ctx context.Context, node *Node) (err error) {

	// Prepare the trash
	// TODO use layout?, but it requires resolving the owners user if the username is used instead of the id.
	// the node knows the owner id so we use that for now
	ownerid, _, err := node.Owner()
	if err != nil {
		return
	}
	if ownerid == "" {
		// fall back to root trash
		ownerid = "root"
	}
	err = os.MkdirAll(filepath.Join(t.lu.Options.Root, "trash", ownerid), 0700)
	if err != nil {
		return
	}

	// get the original path
	origin, err := t.lu.Path(ctx, node)
	if err != nil {
		return
	}

	// remove the entry from the parent dir

	src := filepath.Join(t.lu.toInternalPath(node.ParentID), node.Name)
	err = os.Remove(src)
	if err != nil {
		return
	}

	// rename the trashed node so it is not picked up when traversing up the tree
	nodePath := t.lu.toInternalPath(node.ID)
	deletionTime := time.Now().UTC().Format(time.RFC3339Nano)
	trashPath := nodePath + ".T." + deletionTime
	err = os.Rename(nodePath, trashPath)
	if err != nil {
		return
	}
	// set origin location in metadata
	if err := xattr.Set(trashPath, trashOriginAttr, []byte(origin)); err != nil {
		return err
	}

	// make node appear in the owners (or root) trash
	// parent id and name are stored as extended attributes in the node itself
	trashLink := filepath.Join(t.lu.Options.Root, "trash", ownerid, node.ID)
	err = os.Symlink("../nodes/"+node.ID+".T."+deletionTime, trashLink)
	if err != nil {
		return
	}
	p, err := node.Parent()
	if err != nil {
		return
	}
	return t.Propagate(ctx, p)
}

// Propagate propagates changes to the root of the tree
func (t *Tree) Propagate(ctx context.Context, n *Node) (err error) {
	if !t.lu.Options.TreeTimeAccounting && !t.lu.Options.TreeSizeAccounting {
		// no propagation enabled
		log.Debug().Msg("propagation disabled")
		return
	}
	log := appctx.GetLogger(ctx)

	nodePath := t.lu.toInternalPath(n.ID)

	// is propagation enabled for the parent node?

	var root *Node
	if root, err = t.lu.HomeOrRootNode(ctx); err != nil {
		return
	}

	var fi os.FileInfo
	if fi, err = os.Stat(nodePath); err != nil {
		return err
	}

	var b []byte

	for err == nil && n.ID != root.ID {
		log.Debug().Interface("node", n).Msg("propagating")

		if n, err = n.Parent(); err != nil {
			break
		}

		// TODO none, sync and async?
		if !n.HasPropagation() {
			log.Debug().Interface("node", n).Str("attr", propagationAttr).Msg("propagation attribute not set or unreadable, not propagating")
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
				log.Error().Err(err).Interface("node", n).Msg("could not read tmtime attribute, overwriting")
				updateSyncTime = true
			case tmTime.Before(fi.ModTime()):
				log.Debug().Interface("node", n).Str("tmtime", string(b)).Str("mtime", fi.ModTime().UTC().Format(time.RFC3339Nano)).Msg("parent tmtime is older than node mtime, updating")
				updateSyncTime = true
			default:
				log.Debug().Interface("node", n).Str("tmtime", string(b)).Str("mtime", fi.ModTime().UTC().Format(time.RFC3339Nano)).Msg("parent tmtime is younger than node mtime, not updating")
			}

			if updateSyncTime {
				// update the tree time of the parent node
				if err = n.SetTMTime(fi.ModTime()); err != nil {
					log.Error().Err(err).Interface("node", n).Time("tmtime", fi.ModTime().UTC()).Msg("could not update tmtime of parent node")
					return
				}
				log.Debug().Interface("node", n).Time("tmtime", fi.ModTime().UTC()).Msg("updated tmtime of parent node")
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
