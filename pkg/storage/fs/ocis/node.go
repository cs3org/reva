package ocis

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// NodeInfo allows referencing a node by id and optionally a relative path
type NodeInfo struct {
	ParentID string
	ID       string
	Name     string
	Exists   bool
}

// BecomeParent rewrites the internal state to point to the parent id
func (n *NodeInfo) BecomeParent() {
	n.ID = n.ParentID
	n.ParentID = ""
	n.Name = ""
	n.Exists = false
}

// Create creates a new node in the given root and add symlinks to parent node
// TODO use a reference to the tree to access tho root?
func (n *NodeInfo) Create(root string) (err error) {

	if n.ID != "" {
		return errors.Wrap(err, "ocisfs: node already his an id")
	}
	// create a new file node
	n.ID = uuid.New().String()

	nodePath := filepath.Join(root, "nodes", n.ID)

	err = os.MkdirAll(nodePath, 0700)
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not create node dir")
	}
	// create back link
	// we are not only linking back to the parent, but also to the filename
	link := "../" + n.ParentID + "/children/" + n.Name
	err = os.Symlink(link, filepath.Join(nodePath, "parentname"))
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not symlink parent node")
	}

	// link child name to node
	err = os.Symlink("../../"+n.ID, filepath.Join(root, "nodes", n.ParentID, "children", n.Name))
	if err != nil {
		return errors.Wrap(err, "ocisfs: could not symlink child entry")
	}

	return nil
}
