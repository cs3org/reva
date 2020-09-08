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
	"encoding/hex"
	"math/rand"
	"net/url"
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
)

// Tree manages a hierarchical tree
type Tree struct {
	pw   PathWrapper
	Root string
}

// NewTree creates a new Tree instance
func NewTree(pw PathWrapper, root string) (TreePersistence, error) {
	return &Tree{
		pw:   pw,
		Root: root,
	}, nil
}

// GetMD returns the metadata of a node in the tree
func (t *Tree) GetMD(ctx context.Context, node *Node) (os.FileInfo, error) {
	md, err := os.Stat(filepath.Join(t.Root, "nodes", node.ID))
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
	node, err = t.pw.NodeFromID(ctx, id)
	if err != nil {
		return
	}

	relativeExternalPath, err = t.pw.Path(ctx, node)
	return
}

// CreateRoot creates a new root node with parentid = "root"
func (t *Tree) CreateRoot(id string, owner *userpb.UserId) (n *Node, err error) {
	n = &Node{
		ParentID: "root",
		pw:       t.pw,
		ID:       id,
	}

	// create a directory node
	nodePath := filepath.Join(t.Root, "nodes", n.ID)
	if err = os.MkdirAll(nodePath, 0700); err != nil {
		return nil, errors.Wrap(err, "ocisfs: error creating root node dir")
	}

	err = n.writeMetadata(nodePath, owner)

	n.Exists = true
	return
}

// CreateDir creates a new directory entry in the tree
// TODO use parentnode and name instead of node? would make the exists stuff clearer? maybe obsolete?
func (t *Tree) CreateDir(ctx context.Context, node *Node) (err error) {

	if node.Exists || node.ID != "" {
		return errtypes.AlreadyExists(node.ID) // path?
	}

	// create a directory node
	node.ID = uuid.New().String()

	newPath := filepath.Join(t.Root, "nodes", node.ID)

	err = os.MkdirAll(newPath, 0700)
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not create node dir")
	}

	if err := xattr.Set(newPath, "user.ocis.parentid", []byte(node.ParentID)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set parentid attribute")
	}
	if err := xattr.Set(newPath, "user.ocis.name", []byte(node.Name)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set name attribute")
	}
	if u, ok := user.ContextGetUser(ctx); ok {
		if err := xattr.Set(newPath, "user.ocis.owner.id", []byte(u.Id.OpaqueId)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set owner id attribute")
		}
		if err := xattr.Set(newPath, "user.ocis.owner.idp", []byte(u.Id.Idp)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set owner idp attribute")
		}
	} else {
		log := appctx.GetLogger(ctx)
		log.Error().Msg("home enabled but no user in context")
	}

	// make child appear in listings
	err = os.Symlink("../"+node.ID, filepath.Join(t.Root, "nodes", node.ParentID, node.Name))
	if err != nil {
		return
	}
	return t.Propagate(ctx, node)
}

// CreateReference creates a new reference entry in the tree
func (t *Tree) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	// TODO use symlink, but set extended attribute on link (not on target)
	return errtypes.NotSupported("operation not supported: CreateReference")
}

// Move replaces the target with the source
func (t *Tree) Move(ctx context.Context, oldNode *Node, newNode *Node) (err error) {
	// if target exists delete it without trashing it
	if newNode.Exists {
		// TODO make sure all children are deleted
		if err := os.RemoveAll(filepath.Join(t.Root, "nodes", newNode.ID)); err != nil {
			return errors.Wrap(err, "ocisfs: Move: error deleting target node "+newNode.ID)
		}
	}
	// are we just renaming (parent stays the same)?
	if oldNode.ParentID == newNode.ParentID {

		parentPath := filepath.Join(t.Root, "nodes", oldNode.ParentID)

		// rename child
		err = os.Rename(
			filepath.Join(parentPath, oldNode.Name),
			filepath.Join(parentPath, newNode.Name),
		)
		if err != nil {
			return errors.Wrap(err, "ocisfs: could not rename child")
		}

		tgtPath := filepath.Join(t.Root, "nodes", newNode.ID)

		// update name attribute
		if err := xattr.Set(tgtPath, "user.ocis.name", []byte(newNode.Name)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set name attribute")
		}

		return t.Propagate(ctx, newNode)
	}

	// we are moving the node to a new parent, any target has been removed
	// bring old node to the new parent

	// rename child
	err = os.Rename(
		filepath.Join(t.Root, "nodes", oldNode.ParentID, oldNode.Name),
		filepath.Join(t.Root, "nodes", newNode.ParentID, newNode.Name),
	)
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not move child")
	}

	// update parentid and name
	tgtPath := filepath.Join(t.Root, "nodes", newNode.ID)

	if err := xattr.Set(tgtPath, "user.ocis.parentid", []byte(newNode.ParentID)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set parentid attribute")
	}
	if err := xattr.Set(tgtPath, "user.ocis.name", []byte(newNode.Name)); err != nil {
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

	dir := filepath.Join(t.Root, "nodes", node.ID)
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
			pw:       t.pw,
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
	src := filepath.Join(t.Root, "nodes", node.ParentID, node.Name)
	err = os.Remove(src)
	if err != nil {
		return
	}

	nodePath := filepath.Join(t.Root, "nodes", node.ID)
	trashPath := nodePath + ".T." + time.Now().UTC().Format(time.RFC3339Nano)
	err = os.Rename(nodePath, trashPath)
	if err != nil {
		return
	}

	// make node appear in trash
	// parent id and name are stored as extended attributes in the node itself
	trashLink := filepath.Join(t.Root, "trash", node.ID)
	err = os.Symlink("../nodes/"+node.ID+".T."+time.Now().UTC().Format(time.RFC3339Nano), trashLink)
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
func (t *Tree) Propagate(ctx context.Context, node *Node) (err error) {
	// generate an etag
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	// store in extended attribute
	etag := hex.EncodeToString(bytes)
	for err == nil && !node.IsRoot() {
		if err := xattr.Set(filepath.Join(t.Root, "nodes", node.ID), "user.ocis.etag", []byte(etag)); err != nil {
			log := appctx.GetLogger(ctx)
			log.Error().Err(err).Msg("error storing file id")
		}
		// TODO propagate mtime
		// TODO size accounting

		if err != nil {
			err = errors.Wrap(err, "ocisfs: Propagate: readlink error")
			return
		}

		node, err = node.Parent()
	}
	return
}
