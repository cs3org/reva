package ocis

import (
	"context"
	"encoding/hex"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
	"github.com/rs/zerolog/log"
)

// Tree manages a hierarchical tree
type Tree struct {
	pw            PathWrapper
	DataDirectory string
}

// NewTree creates a new Tree instance
func NewTree(pw PathWrapper, dataDirectory string) (TreePersistence, error) {
	return &Tree{
		pw:            pw,
		DataDirectory: dataDirectory,
	}, nil
}

// GetMD returns the metadata of a node in the tree
func (t *Tree) GetMD(ctx context.Context, node *NodeInfo) (os.FileInfo, error) {
	md, err := os.Stat(filepath.Join(t.DataDirectory, "nodes", node.ID))
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
	var node *NodeInfo
	node, err = t.pw.NodeFromID(ctx, id)
	if err != nil {
		return
	}

	relativeExternalPath, err = t.pw.Path(ctx, node)
	return
}

// CreateDir creates a new directory entry in the tree
func (t *Tree) CreateDir(ctx context.Context, node *NodeInfo) (err error) {

	// TODO always try to fill node?
	if node.Exists || node.ID != "" { // child already exists
		return
	}

	// create a directory node
	node.ID = uuid.New().String()

	newPath := filepath.Join(t.DataDirectory, "nodes", node.ID)

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

	// make child appear in listings
	err = os.Symlink("../"+node.ID, filepath.Join(t.DataDirectory, "nodes", node.ParentID, node.Name))
	if err != nil {
		return
	}
	return t.Propagate(ctx, node)
}

// CreateReference creates a new reference entry in the tree
func (t *Tree) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return errtypes.NotSupported("operation not supported: CreateReference")
}

// Move replaces the target with the source
func (t *Tree) Move(ctx context.Context, oldNode *NodeInfo, newNode *NodeInfo) (err error) {
	// if target exists delete it without trashing it
	if newNode.Exists {
		// TODO make sure all children are deleted
		if err := os.RemoveAll(filepath.Join(t.DataDirectory, "nodes", newNode.ID)); err != nil {
			return errors.Wrap(err, "ocisfs: Move: error deleting target node "+newNode.ID)
		}
	}
	// are we renaming?
	if oldNode.ParentID == newNode.ParentID {

		parentPath := filepath.Join(t.DataDirectory, "nodes", oldNode.ParentID)

		// rename child
		err = os.Rename(
			filepath.Join(parentPath, oldNode.Name),
			filepath.Join(parentPath, newNode.Name),
		)
		if err != nil {
			return errors.Wrap(err, "ocisfs: could not rename child")
		}

		tgtPath := filepath.Join(t.DataDirectory, "nodes", newNode.ID)

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
		filepath.Join(t.DataDirectory, "nodes", oldNode.ParentID, oldNode.Name),
		filepath.Join(t.DataDirectory, "nodes", newNode.ParentID, newNode.Name),
	)
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not move child")
	}

	tgtPath := filepath.Join(t.DataDirectory, "nodes", newNode.ID)

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
func (t *Tree) ListFolder(ctx context.Context, node *NodeInfo) ([]*NodeInfo, error) {

	dir := filepath.Join(t.DataDirectory, "nodes", node.ID)
	f, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(dir)
		}
		return nil, errors.Wrap(err, "tree: error listing "+dir)
	}

	names, err := f.Readdirnames(0)
	nodes := []*NodeInfo{}
	for i := range names {
		link, err := os.Readlink(filepath.Join(dir, names[i]))
		if err != nil {
			// TODO log
			continue
		}
		n := &NodeInfo{
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
func (t *Tree) Delete(ctx context.Context, node *NodeInfo) (err error) {
	src := filepath.Join(t.DataDirectory, "nodes", node.ParentID, node.Name)
	err = os.Remove(src)
	if err != nil {
		return
	}

	nodePath := filepath.Join(t.DataDirectory, "nodes", node.ID)
	trashPath := nodePath + ".T." + time.Now().UTC().Format(time.RFC3339Nano)
	err = os.Rename(nodePath, trashPath)
	if err != nil {
		return
	}

	// make node appear in trash
	// parent id and name are stored as extended attributes in the node itself
	trashLink := filepath.Join(t.DataDirectory, "trash", node.ID)
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
func (t *Tree) Propagate(ctx context.Context, node *NodeInfo) (err error) {
	// generate an etag
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	// store in extended attribute
	etag := hex.EncodeToString(bytes)
	for err == nil && !node.IsRoot() {
		if err := xattr.Set(filepath.Join(t.DataDirectory, "nodes", node.ID), "user.ocis.etag", []byte(etag)); err != nil {
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
