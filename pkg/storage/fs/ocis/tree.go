package ocis

import (
	"context"
	"encoding/hex"
	"io/ioutil"
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
func (fs *Tree) GetMD(ctx context.Context, node *NodeInfo) (os.FileInfo, error) {
	md, err := os.Stat(filepath.Join(fs.DataDirectory, "nodes", node.ID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(node.ID)
		}
		return nil, errors.Wrap(err, "tree: error stating "+node.ID)
	}

	return md, nil
}

// GetPathByID returns the fn pointed by the file id, without the internal namespace
func (fs *Tree) GetPathByID(ctx context.Context, id *provider.ResourceId) (relativeExternalPath string, err error) {
	var node *NodeInfo
	node, err = fs.pw.WrapID(ctx, id)
	if err != nil {
		return
	}

	relativeExternalPath, err = fs.pw.Unwrap(ctx, node)
	return
}

// CreateDir creates a new directory entry in the tree
func (fs *Tree) CreateDir(ctx context.Context, node *NodeInfo) (err error) {

	// TODO always try to  fill node?
	if node.Exists || node.ID != "" { // child already exists
		return
	}

	// create a directory node (with children subfolder)
	node.ID = uuid.New().String()

	newPath := filepath.Join(fs.DataDirectory, "nodes", node.ID)

	err = os.MkdirAll(filepath.Join(newPath, "children"), 0700)
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not create node dir")
	}

	// create back link
	// we are not only linking back to the parent, but also to the filename
	err = os.Symlink("../"+node.ParentID+"/children/"+node.Name, filepath.Join(newPath, "parentname"))
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not symlink parent node")
	}

	// make child appear in listings
	err = os.Symlink("../../"+node.ID, filepath.Join(fs.DataDirectory, "nodes", node.ParentID, "children", node.Name))
	if err != nil {
		return
	}
	return fs.Propagate(ctx, node)
}

// CreateReference creates a new reference entry in the tree
func (fs *Tree) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return errtypes.NotSupported("operation not supported: CreateReference")
}

// Move replaces the target with the source
func (fs *Tree) Move(ctx context.Context, oldNode *NodeInfo, newNode *NodeInfo) (err error) {
	err = fs.pw.FillParentAndName(newNode)
	if os.IsNotExist(err) {
		err = nil
		return
	}

	// if target exists delete it without trashing it
	if newNode.Exists {
		err = fs.pw.FillParentAndName(newNode)
		if os.IsNotExist(err) {
			err = nil
			return
		}
		if err := os.RemoveAll(filepath.Join(fs.DataDirectory, "nodes", newNode.ID)); err != nil {
			return errors.Wrap(err, "ocisfs: Move: error deleting target node "+newNode.ID)
		}
	}
	// are we renaming?
	if oldNode.ParentID == newNode.ParentID {

		nodePath := filepath.Join(fs.DataDirectory, "nodes", oldNode.ID)

		// update back link
		// we are not only linking back to the parent, but also to the filename
		err = os.Remove(filepath.Join(nodePath, "parentname"))
		if err != nil {
			return errors.Wrap(err, "ocisfs: could not remove parent link")
		}
		err = os.Symlink("../"+oldNode.ParentID+"/children/"+newNode.Name, filepath.Join(nodePath, "parentname"))
		if err != nil {
			return errors.Wrap(err, "ocisfs: could not symlink parent")
		}

		// rename child
		err = os.Rename(
			filepath.Join(fs.DataDirectory, "nodes", oldNode.ParentID, "children", oldNode.Name),
			filepath.Join(fs.DataDirectory, "nodes", oldNode.ParentID, "children", newNode.Name),
		)
		if err != nil {
			return errors.Wrap(err, "ocisfs: could not rename symlink")
		}
		return fs.Propagate(ctx, oldNode)
	}
	// we are moving the node to a new parent, any target has been removed
	// bring old node to the new parent

	nodePath := filepath.Join(fs.DataDirectory, "nodes", oldNode.ID)

	// update back link
	// we are not only linking back to the parent, but also to the filename
	err = os.Remove(filepath.Join(nodePath, "parentname"))
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not remove parent link")
	}
	err = os.Symlink("../"+newNode.ParentID+"/children/"+newNode.Name, filepath.Join(nodePath, "parentname"))
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not symlink parent")
	}

	// rename child
	err = os.Rename(
		filepath.Join(fs.DataDirectory, "nodes", oldNode.ParentID, "children", oldNode.Name),
		filepath.Join(fs.DataDirectory, "nodes", newNode.ParentID, "children", newNode.Name),
	)
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not rename symlink")
	}

	// TODO inefficient because we might update several nodes twice, only propagate unchanged nodes?
	// collect in a list, then only stat each node once
	// also do this in a go routine ... webdav should check the etag async
	err = fs.Propagate(ctx, oldNode)
	if err != nil {
		return errors.Wrap(err, "ocisfs: Move: could not propagate old node")
	}
	err = fs.Propagate(ctx, newNode)
	if err != nil {
		return errors.Wrap(err, "ocisfs: Move: could not propagate old node")
	}
	return nil
}

// ChildrenPath returns the absolute path to childrens in a node
// TODO move to node?
func (fs *Tree) ChildrenPath(node *NodeInfo) string {
	return filepath.Join(fs.DataDirectory, "nodes", node.ID, "children")
}

// ListFolder lists the children inside a folder
func (fs *Tree) ListFolder(ctx context.Context, node *NodeInfo) ([]*NodeInfo, error) {

	children := fs.ChildrenPath(node)
	f, err := os.Open(children)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(children)
		}
		return nil, errors.Wrap(err, "tree: error listing "+children)
	}

	names, err := f.Readdirnames(0)
	nodes := []*NodeInfo{}
	for i := range names {
		link, err := os.Readlink(filepath.Join(children, names[i]))
		if err != nil {
			// TODO log
			continue
		}
		n := &NodeInfo{
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
func (fs *Tree) Delete(ctx context.Context, node *NodeInfo) (err error) {
	err = fs.pw.FillParentAndName(node)
	if os.IsNotExist(err) {
		err = nil
		return
	}
	if err != nil {
		err = errors.Wrap(err, "ocisfs: Delete: FillParentAndName error")
		return
	}

	// remove child entry from dir

	os.Remove(filepath.Join(fs.DataDirectory, "nodes", node.ParentID, "children", node.Name))

	src := filepath.Join(fs.DataDirectory, "nodes", node.ID)
	trashpath := filepath.Join(fs.DataDirectory, "trash/files", node.ID)
	err = os.Rename(src, trashpath)
	if err != nil {
		return
	}

	// write a trash info ... slightly violating the freedesktop trash spec
	t := time.Now()
	// TODO store the original Path
	info := []byte("[Trash Info]\nParentID=" + node.ParentID + "\nDeletionDate=" + t.Format(time.RFC3339))
	infoPath := filepath.Join(fs.DataDirectory, "trash/info", node.ID+".trashinfo")
	err = ioutil.WriteFile(infoPath, info, 0700)
	if err != nil {
		return
	}

	return fs.Propagate(ctx, &NodeInfo{ID: node.ParentID})
}

// Propagate propagates changes to the root of the tree
func (fs *Tree) Propagate(ctx context.Context, node *NodeInfo) (err error) {
	// generate an etag
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	// store in extended attribute
	etag := hex.EncodeToString(bytes)
	for err == nil {
		if err := xattr.Set(filepath.Join(fs.DataDirectory, "nodes", node.ID), "user.ocis.etag", []byte(etag)); err != nil {
			log.Error().Err(err).Msg("error storing file id")
		}
		err = fs.pw.FillParentAndName(node)
		if os.IsNotExist(err) {
			err = nil
			return
		}
		if err != nil {
			err = errors.Wrap(err, "ocisfs: Propagate: readlink error")
			return
		}

		node.BecomeParent()
	}
	return
}
