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
	pw *Path
}

// NewTree creates a new Tree instance
func NewTree(pw *Path) (TreePersistence, error) {
	return &Tree{
		pw: pw,
	}, nil
}

// GetMD returns the metadata of a node in the tree
func (t *Tree) GetMD(ctx context.Context, node *Node) (os.FileInfo, error) {
	md, err := os.Stat(filepath.Join(t.pw.Root, "nodes", node.ID))
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

// does not take care of linking back to parent
// TODO check if node exists?
func createNode(n *Node, owner *userpb.UserId) (err error) {
	// create a directory node
	nodePath := filepath.Join(n.pw.Root, "nodes", n.ID)
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

	if t.pw.EnableHome {
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
	err = os.Symlink("../"+node.ID, filepath.Join(t.pw.Root, "nodes", node.ParentID, node.Name))
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
		if err := os.RemoveAll(filepath.Join(t.pw.Root, "nodes", newNode.ID)); err != nil {
			return errors.Wrap(err, "ocisfs: Move: error deleting target node "+newNode.ID)
		}
	}
	// are we just renaming (parent stays the same)?
	if oldNode.ParentID == newNode.ParentID {

		parentPath := filepath.Join(t.pw.Root, "nodes", oldNode.ParentID)

		// rename child
		err = os.Rename(
			filepath.Join(parentPath, oldNode.Name),
			filepath.Join(parentPath, newNode.Name),
		)
		if err != nil {
			return errors.Wrap(err, "ocisfs: could not rename child")
		}

		tgtPath := filepath.Join(t.pw.Root, "nodes", newNode.ID)

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
		filepath.Join(t.pw.Root, "nodes", oldNode.ParentID, oldNode.Name),
		filepath.Join(t.pw.Root, "nodes", newNode.ParentID, newNode.Name),
	)
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not move child")
	}

	// update parentid and name
	tgtPath := filepath.Join(t.pw.Root, "nodes", newNode.ID)

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

	dir := filepath.Join(t.pw.Root, "nodes", node.ID)
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
	src := filepath.Join(t.pw.Root, "nodes", node.ParentID, node.Name)
	err = os.Remove(src)
	if err != nil {
		return
	}

	nodePath := filepath.Join(t.pw.Root, "nodes", node.ID)
	trashPath := nodePath + ".T." + time.Now().UTC().Format(time.RFC3339Nano)
	err = os.Rename(nodePath, trashPath)
	if err != nil {
		return
	}

	// make node appear in trash
	// parent id and name are stored as extended attributes in the node itself
	trashLink := filepath.Join(t.pw.Root, "trash", node.ID)
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
	var root *Node

	if root, err = t.pw.HomeOrRootNode(ctx); err != nil {
		return
	}
	for err == nil && node.ID != root.ID { // TODO propagate up to where?
		if err := xattr.Set(filepath.Join(t.pw.Root, "nodes", node.ID), "user.ocis.etag", []byte(etag)); err != nil {
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
