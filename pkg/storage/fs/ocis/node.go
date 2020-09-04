package ocis

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// NodeInfo allows referencing a node by id and optionally a relative path
type NodeInfo struct {
	pw       PathWrapper
	ParentID string
	ID       string
	Name     string
	ownerID  string
	ownerIDP string
	Exists   bool
}

// NewNode creates a new instance and checks if it exists
func NewNode(pw PathWrapper, id string) (n *NodeInfo, err error) {
	n = &NodeInfo{
		pw: pw,
		ID: id,
	}

	nodePath := filepath.Join(n.pw.Root(), "nodes", n.ID)

	// lookup parent id in extended attributes
	var attrBytes []byte
	if attrBytes, err = xattr.Get(nodePath, "user.ocis.parentid"); err == nil {
		n.ParentID = string(attrBytes)
	} else {
		// TODO log error
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, "user.ocis.name"); err == nil {
		n.Name = string(attrBytes)
	} else {
		// TODO log error
		return
	}

	parentID := n.ParentID

	for parentID != "root" {
		// walk to root to check node is not part of a deleted subtree
		parentPath := filepath.Join(n.pw.Root(), "nodes", parentID)

		if attrBytes, err = xattr.Get(parentPath, "user.ocis.parentid"); err == nil {
			parentID = string(attrBytes)
		} else {
			if os.IsNotExist(err) {
				return
			} else {
				// TODO log error
				return
			}
		}
	}

	n.Exists = true

	return
}

// Child returns the child node with the given name
func (n *NodeInfo) Child(name string) (c *NodeInfo, err error) {
	c = &NodeInfo{
		pw:       n.pw,
		ParentID: n.ID,
		Name:     name,
	}
	var link string
	link, err = os.Readlink(filepath.Join(n.pw.Root(), "nodes", n.ID, name))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		err = errors.Wrap(err, "ocisfs: Wrap: readlink error")
		return
	}
	if strings.HasPrefix(link, "../") {
		c.Exists = true
		c.ID = filepath.Base(link)
	} else {
		err = fmt.Errorf("ocisfs: expected '../ prefix, got' %+v", link)
	}
	return
}

// IsRoot returns true when the node is the root of a tree
func (n *NodeInfo) IsRoot() bool {
	return n.ParentID == "root"
}

// Parent returns the parent node
func (n *NodeInfo) Parent() (p *NodeInfo, err error) {
	if n.ParentID == "root" {
		return nil, fmt.Errorf("ocisfs: root has no parent")
	}
	p = &NodeInfo{
		pw: n.pw,
		ID: n.ParentID,
	}

	parentPath := filepath.Join(n.pw.Root(), "nodes", n.ParentID)

	// lookup parent id in extended attributes
	var attrBytes []byte
	if attrBytes, err = xattr.Get(parentPath, "user.ocis.parentid"); err == nil {
		p.ParentID = string(attrBytes)
	} else {
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(parentPath, "user.ocis.name"); err == nil {
		p.Name = string(attrBytes)
	} else {
		return
	}

	// check node exists
	if _, err := os.Stat(parentPath); err == nil {
		p.Exists = true
	}
	return
}

// Owner returns the cached owner id or reads it from the extended attributes
func (n *NodeInfo) Owner() (id string, idp string, err error) {
	if n.ownerID != "" && n.ownerIDP != "" {
		return n.ownerID, n.ownerIDP, nil
	}

	nodePath := filepath.Join(n.pw.Root(), "nodes", n.ParentID)
	// lookup parent id in extended attributes
	var attrBytes []byte
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, "user.ocis.owner.id"); err == nil {
		n.ownerID = string(attrBytes)
	} else {
		// TODO log error
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, "user.ocis.owner.idp"); err == nil {
		n.ownerIDP = string(attrBytes)
	} else {
		// TODO log error
	}
	return n.ownerID, n.ownerIDP, err
}
