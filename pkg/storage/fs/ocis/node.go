package ocis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// Node represents a node in the tree and provides methods to get a Parent or Child instance
type Node struct {
	pw       PathWrapper
	ParentID string
	ID       string
	Name     string
	ownerID  string
	ownerIDP string
	Exists   bool
}

// NewNode creates a new instance and checks if it exists
func NewNode(pw PathWrapper, id string) (n *Node, err error) {
	n = &Node{
		pw: pw,
		ID: id,
	}

	nodePath := filepath.Join(n.pw.Root(), "nodes", n.ID)

	// lookup parent id in extended attributes
	var attrBytes []byte
	if attrBytes, err = xattr.Get(nodePath, "user.ocis.parentid"); err == nil {
		n.ParentID = string(attrBytes)
	} else {
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, "user.ocis.name"); err == nil {
		n.Name = string(attrBytes)
	} else {
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
			}
			return
		}
	}

	n.Exists = true

	return
}

// Child returns the child node with the given name
func (n *Node) Child(name string) (c *Node, err error) {
	c = &Node{
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
func (n *Node) IsRoot() bool {
	return n.ParentID == "root"
}

// Parent returns the parent node
func (n *Node) Parent() (p *Node, err error) {
	if n.ParentID == "root" {
		return nil, fmt.Errorf("ocisfs: root has no parent")
	}
	p = &Node{
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
func (n *Node) Owner() (id string, idp string, err error) {
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
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, "user.ocis.owner.idp"); err == nil {
		n.ownerIDP = string(attrBytes)
	} else {
		return
	}
	return n.ownerID, n.ownerIDP, err
}

// AsResourceInfo return the node as CS3 ResourceInfo
func (n *Node) AsResourceInfo(ctx context.Context) (ri *provider.ResourceInfo, err error) {
	var fn string

	nodePath := filepath.Join(n.pw.Root(), "nodes", n.ID)

	var fi os.FileInfo

	nodeType := provider.ResourceType_RESOURCE_TYPE_INVALID
	if fi, err = os.Lstat(nodePath); err != nil {
		return
	}
	if fi.IsDir() {
		nodeType = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	} else if fi.Mode().IsRegular() {
		nodeType = provider.ResourceType_RESOURCE_TYPE_FILE
	} else if fi.Mode()&os.ModeSymlink != 0 {
		nodeType = provider.ResourceType_RESOURCE_TYPE_SYMLINK
		// TODO reference using ext attr on a symlink
		// nodeType = provider.ResourceType_RESOURCE_TYPE_REFERENCE
	}

	var etag []byte
	// TODO optionally store etag in new `root/attributes/<uuid>` file
	if etag, err = xattr.Get(nodePath, "user.ocis.etag"); err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Interface("node", n).Msg("could not read etag")
	}

	id := &provider.ResourceId{OpaqueId: n.ID}

	fn, err = n.pw.Path(ctx, n)
	if err != nil {
		return nil, err
	}
	ri = &provider.ResourceInfo{
		Id:       id,
		Path:     fn,
		Type:     nodeType,
		Etag:     string(etag),
		MimeType: mime.Detect(nodeType == provider.ResourceType_RESOURCE_TYPE_CONTAINER, fn),
		Size:     uint64(fi.Size()),
		// TODO fix permissions
		PermissionSet: &provider.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Mtime: &types.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
			// TODO read nanos from where? Nanos:   fi.MTimeNanos,
		},
	}

	if owner, idp, err := n.Owner(); err == nil {
		ri.Owner = &userpb.UserId{
			Idp:      idp,
			OpaqueId: owner,
		}
	}
	log := appctx.GetLogger(ctx)
	log.Debug().
		Interface("ri", ri).
		Msg("AsResourceInfo")

	return ri, nil
}
