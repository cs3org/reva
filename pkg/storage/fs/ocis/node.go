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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// Node represents a node in the tree and provides methods to get a Parent or Child instance
type Node struct {
	pw       PathWrapper
	ParentID string
	ID       string
	Name     string
	ownerID  string // used to cache the owner id
	ownerIDP string // used to cache the owner idp
	Exists   bool
}

// CreateDir creates a new child directory node with a new id and the given name
// owner is optional
// TODO use in tree CreateDir
func (n *Node) CreateDir(pw PathWrapper, name string, owner *userpb.UserId) (c *Node, err error) {
	c = &Node{
		pw:       pw,
		ParentID: n.ID,
		ID:       uuid.New().String(),
		Name:     name,
	}

	// create a directory node
	nodePath := filepath.Join(pw.Root(), "nodes", c.ID)
	if err = os.MkdirAll(nodePath, 0700); err != nil {
		return nil, errors.Wrap(err, "ocisfs: error creating node child dir")
	}

	c.writeMetadata(nodePath, owner)

	c.Exists = true
	return
}

func (n *Node) writeMetadata(nodePath string, owner *userpb.UserId) (err error) {
	if err = xattr.Set(nodePath, "user.ocis.parentid", []byte(n.ParentID)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set parentid attribute")
	}
	if err = xattr.Set(nodePath, "user.ocis.name", []byte(n.Name)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set name attribute")
	}
	if owner != nil {
		if err = xattr.Set(nodePath, "user.ocis.owner.id", []byte(owner.OpaqueId)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set owner id attribute")
		}
		if err = xattr.Set(nodePath, "user.ocis.owner.idp", []byte(owner.Idp)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set owner idp attribute")
		}
	}
	return
}

// ReadNode creates a new instance from an id and checks if it exists
func ReadNode(pw PathWrapper, id string) (n *Node, err error) {
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
// TODO can be private as only the AsResourceInfo uses it
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
