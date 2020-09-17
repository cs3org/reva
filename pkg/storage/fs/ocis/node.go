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
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	pw       *Path
	ParentID string
	ID       string
	Name     string
	ownerID  string // used to cache the owner id
	ownerIDP string // used to cache the owner idp
	Exists   bool
}

func (n *Node) writeMetadata(owner *userpb.UserId) (err error) {
	nodePath := filepath.Join(n.pw.Root, "nodes", n.ID)
	if err = xattr.Set(nodePath, parentidAttr, []byte(n.ParentID)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set parentid attribute")
	}
	if err = xattr.Set(nodePath, nameAttr, []byte(n.Name)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set name attribute")
	}
	if owner != nil {
		if err = xattr.Set(nodePath, ownerIDAttr, []byte(owner.OpaqueId)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set owner id attribute")
		}
		if err = xattr.Set(nodePath, ownerIDPAttr, []byte(owner.Idp)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set owner idp attribute")
		}
	}
	return
}

// ReadNode creates a new instance from an id and checks if it exists
func ReadNode(ctx context.Context, pw *Path, id string) (n *Node, err error) {
	n = &Node{
		pw: pw,
		ID: id,
	}

	nodePath := filepath.Join(n.pw.Root, "nodes", n.ID)

	// lookup parent id in extended attributes
	var attrBytes []byte
	if attrBytes, err = xattr.Get(nodePath, parentidAttr); err == nil {
		n.ParentID = string(attrBytes)
	} else {
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, nameAttr); err == nil {
		n.Name = string(attrBytes)
	} else {
		return
	}

	var root *Node
	if root, err = pw.HomeOrRootNode(ctx); err != nil {
		return
	}
	parentID := n.ParentID

	log := appctx.GetLogger(ctx)
	for parentID != root.ID {
		log.Debug().Interface("node", n).Str("root.ID", root.ID).Msg("ReadNode()")
		// walk to root to check node is not part of a deleted subtree
		parentPath := filepath.Join(n.pw.Root, "nodes", parentID)

		if attrBytes, err = xattr.Get(parentPath, parentidAttr); err == nil {
			parentID = string(attrBytes)
			log.Debug().Interface("node", n).Str("root.ID", root.ID).Str("parentID", parentID).Msg("ReadNode() found parent")
		} else {
			log.Error().Err(err).Interface("node", n).Str("root.ID", root.ID).Msg("ReadNode()")
			if os.IsNotExist(err) {
				return
			}
			return
		}
	}

	n.Exists = true
	log.Debug().Interface("node", n).Msg("ReadNode() found node")

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
	if link, err = os.Readlink(filepath.Join(n.pw.Root, "nodes", n.ID, name)); os.IsNotExist(err) {
		err = nil // if the file does not exist we return a node that has Exists = false
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

// Parent returns the parent node
func (n *Node) Parent() (p *Node, err error) {
	if n.ParentID == "" {
		return nil, fmt.Errorf("ocisfs: root has no parent")
	}
	p = &Node{
		pw: n.pw,
		ID: n.ParentID,
	}

	parentPath := filepath.Join(n.pw.Root, "nodes", n.ParentID)

	// lookup parent id in extended attributes
	var attrBytes []byte
	if attrBytes, err = xattr.Get(parentPath, parentidAttr); err == nil {
		p.ParentID = string(attrBytes)
	} else {
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(parentPath, nameAttr); err == nil {
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

	nodePath := filepath.Join(n.pw.Root, "nodes", n.ParentID)
	// lookup parent id in extended attributes
	var attrBytes []byte
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, ownerIDAttr); err == nil {
		n.ownerID = string(attrBytes)
	} else {
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, ownerIDPAttr); err == nil {
		n.ownerIDP = string(attrBytes)
	} else {
		return
	}
	return n.ownerID, n.ownerIDP, err
}

// AsResourceInfo return the node as CS3 ResourceInfo
func (n *Node) AsResourceInfo(ctx context.Context) (ri *provider.ResourceInfo, err error) {
	log := appctx.GetLogger(ctx)

	var fn string
	nodePath := filepath.Join(n.pw.Root, "nodes", n.ID)

	var fi os.FileInfo

	nodeType := provider.ResourceType_RESOURCE_TYPE_INVALID
	if fi, err = os.Lstat(nodePath); err != nil {
		return
	}

	var target []byte
	switch {
	case fi.IsDir():
		if target, err = xattr.Get(nodePath, referenceAttr); err == nil {
			nodeType = provider.ResourceType_RESOURCE_TYPE_REFERENCE
		} else {
			nodeType = provider.ResourceType_RESOURCE_TYPE_CONTAINER
		}
	case fi.Mode().IsRegular():
		nodeType = provider.ResourceType_RESOURCE_TYPE_FILE
	case fi.Mode()&os.ModeSymlink != 0:
		nodeType = provider.ResourceType_RESOURCE_TYPE_SYMLINK
		// TODO reference using ext attr on a symlink
		// nodeType = provider.ResourceType_RESOURCE_TYPE_REFERENCE
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
		MimeType: mime.Detect(nodeType == provider.ResourceType_RESOURCE_TYPE_CONTAINER, fn),
		Size:     uint64(fi.Size()),
		// TODO fix permissions
		PermissionSet: &provider.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Target:        string(target),
	}

	if owner, idp, err := n.Owner(); err == nil {
		ri.Owner = &userpb.UserId{
			Idp:      idp,
			OpaqueId: owner,
		}
	}

	// etag currently is a hash of fileid + tmtime (or mtime)
	// TODO make etag of files use fileid and checksum
	// TODO implment adding temporery etag in an attribute to restore backups
	h := md5.New()
	if _, err := io.WriteString(h, n.ID); err != nil {
		return nil, err
	}
	var b []byte
	var tmTime time.Time
	if b, err = xattr.Get(nodePath, treeMTimeAttr); err == nil {
		if tmTime, err = time.Parse(time.RFC3339Nano, string(b)); err != nil {
			// invalid format, overwrite
			log.Error().Err(err).Interface("node", n).Str("tmtime", string(b)).Msg("invalid format, ignoring")
			tmTime = fi.ModTime()
		}
	} else {
		// no tmtime, use mtime
		tmTime = fi.ModTime()
	}
	if tb, err := tmTime.UTC().MarshalBinary(); err == nil {
		if _, err := h.Write(tb); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	// use temporary etag if it is set
	if b, err := xattr.Get(nodePath, tmpEtagAttr); err == nil {
		ri.Etag = string(b)
	} else {
		ri.Etag = fmt.Sprintf("%x", h.Sum(nil))
	}

	// mtime uses tmtime if present
	// TODO expose mtime and tmtime separately?
	un := tmTime.UnixNano()
	ri.Mtime = &types.Timestamp{
		Seconds: uint64(un / 1000000000),
		Nanos:   uint32(un % 1000000000),
	}

	// TODO only read the requested metadata attributes
	if attrs, err := xattr.List(nodePath); err == nil {
		ri.ArbitraryMetadata = &provider.ArbitraryMetadata{
			Metadata: map[string]string{},
		}
		for i := range attrs {
			if strings.HasPrefix(attrs[i], metadataPrefix) {
				k := strings.TrimPrefix(attrs[i], metadataPrefix)
				if v, err := xattr.Get(nodePath, attrs[i]); err == nil {
					ri.ArbitraryMetadata.Metadata[k] = string(v)
				} else {
					log.Error().Err(err).Interface("node", n).Str("attr", attrs[i]).Msg("could not get attribute value")
				}
			}
		}
	} else {
		log.Error().Err(err).Interface("node", n).Msg("could not list attributes")
	}

	log.Debug().
		Interface("ri", ri).
		Msg("AsResourceInfo")

	return ri, nil
}
