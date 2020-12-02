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
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage/utils/ace"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
	"github.com/rs/zerolog/log"
)

// Node represents a node in the tree and provides methods to get a Parent or Child instance
type Node struct {
	lu       *Lookup
	ParentID string
	ID       string
	Name     string
	ownerID  string // used to cache the owner id
	ownerIDP string // used to cache the owner idp
	Exists   bool
}

func (n *Node) writeMetadata(owner *userpb.UserId) (err error) {
	nodePath := n.lu.toInternalPath(n.ID)
	if err = xattr.Set(nodePath, parentidAttr, []byte(n.ParentID)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set parentid attribute")
	}
	if err = xattr.Set(nodePath, nameAttr, []byte(n.Name)); err != nil {
		return errors.Wrap(err, "ocisfs: could not set name attribute")
	}
	if owner == nil {
		if err = xattr.Set(nodePath, ownerIDAttr, []byte("")); err != nil {
			return errors.Wrap(err, "ocisfs: could not set empty owner id attribute")
		}
		if err = xattr.Set(nodePath, ownerIDPAttr, []byte("")); err != nil {
			return errors.Wrap(err, "ocisfs: could not set empty owner idp attribute")
		}
	} else {
		if err = xattr.Set(nodePath, ownerIDAttr, []byte(owner.OpaqueId)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set owner id attribute")
		}
		if err = xattr.Set(nodePath, ownerIDPAttr, []byte(owner.Idp)); err != nil {
			return errors.Wrap(err, "ocisfs: could not set owner idp attribute")
		}
	}
	return
}

// ReadRecycleItem reads a recycle item as a node
// TODO refactor the returned params into Node properties? would make all the path transformations go away...
func ReadRecycleItem(ctx context.Context, lu *Lookup, key string) (n *Node, trashItem string, deletedNodePath string, origin string, err error) {

	if key == "" {
		return nil, "", "", "", errtypes.InternalError("key is empty")
	}

	kp := strings.SplitN(key, ":", 2)
	if len(kp) != 2 {
		appctx.GetLogger(ctx).Error().Err(err).Str("key", key).Msg("malformed key")
		return
	}
	trashItem = filepath.Join(lu.Options.Root, "trash", kp[0], kp[1])

	var link string
	link, err = os.Readlink(trashItem)
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("trashItem", trashItem).Msg("error reading trash link")
		return
	}
	parts := strings.SplitN(filepath.Base(link), ".T.", 2)
	if len(parts) != 2 {
		appctx.GetLogger(ctx).Error().Err(err).Str("trashItem", trashItem).Interface("parts", parts).Msg("malformed trash link")
		return
	}

	n = &Node{
		lu: lu,
		ID: parts[0],
	}

	deletedNodePath = lu.toInternalPath(filepath.Base(link))

	// lookup parent id in extended attributes
	var attrBytes []byte
	if attrBytes, err = xattr.Get(deletedNodePath, parentidAttr); err == nil {
		n.ParentID = string(attrBytes)
	} else {
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, nameAttr); err == nil {
		n.Name = string(attrBytes)
	} else {
		return
	}
	// lookup ownerId in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, ownerIDAttr); err == nil {
		n.ownerID = string(attrBytes)
	} else {
		return
	}
	// lookup ownerIdp in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, ownerIDPAttr); err == nil {
		n.ownerIDP = string(attrBytes)
	} else {
		return
	}

	// get origin node
	origin = "/"

	// lookup origin path in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, trashOriginAttr); err == nil {
		origin = string(attrBytes)
	} else {
		log.Error().Err(err).Str("trashItem", trashItem).Str("link", link).Str("deletedNodePath", deletedNodePath).Msg("could not read origin path, restoring to /")
	}
	return
}

// ReadNode creates a new instance from an id and checks if it exists
func ReadNode(ctx context.Context, lu *Lookup, id string) (n *Node, err error) {
	n = &Node{
		lu: lu,
		ID: id,
	}

	nodePath := lu.toInternalPath(n.ID)

	// lookup parent id in extended attributes
	var attrBytes []byte
	attrBytes, err = xattr.Get(nodePath, parentidAttr)
	switch {
	case err == nil:
		n.ParentID = string(attrBytes)
	case isNoData(err):
		return nil, errtypes.InternalError(err.Error())
	case isNotFound(err):
		return n, nil // swallow not found, the node defaults to exists = false
	default:
		return nil, errtypes.InternalError(err.Error())
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, nameAttr); err == nil {
		n.Name = string(attrBytes)
	} else {
		return
	}

	var root *Node
	if root, err = lu.HomeOrRootNode(ctx); err != nil {
		return
	}
	parentID := n.ParentID

	log := appctx.GetLogger(ctx)
	for parentID != root.ID {
		log.Debug().Interface("node", n).Str("root.ID", root.ID).Msg("ReadNode()")
		// walk to root to check node is not part of a deleted subtree

		if attrBytes, err = xattr.Get(lu.toInternalPath(parentID), parentidAttr); err == nil {
			parentID = string(attrBytes)
			log.Debug().Interface("node", n).Str("root.ID", root.ID).Str("parentID", parentID).Msg("ReadNode() found parent")
		} else {
			log.Error().Err(err).Interface("node", n).Str("root.ID", root.ID).Msg("ReadNode()")
			if isNotFound(err) {
				return nil, errtypes.NotFound(err.Error())
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
		lu:       n.lu,
		ParentID: n.ID,
		Name:     name,
	}
	var link string
	if link, err = os.Readlink(filepath.Join(n.lu.toInternalPath(n.ID), name)); os.IsNotExist(err) {
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
		lu: n.lu,
		ID: n.ParentID,
	}

	parentPath := n.lu.toInternalPath(n.ParentID)

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

	nodePath := n.lu.toInternalPath(n.ID)
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
	nodePath := n.lu.toInternalPath(n.ID)

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

	fn, err = n.lu.Path(ctx, n)
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
	var tmTime time.Time
	if tmTime, err = n.GetTMTime(); err != nil {
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
		ri.Etag = fmt.Sprintf(`"%x"`, string(b))
	} else {
		ri.Etag = fmt.Sprintf(`"%x"`, h.Sum(nil))
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

// HasPropagation checks if the propagation attribute exists and is set to "1"
func (n *Node) HasPropagation() (propagation bool) {
	if b, err := xattr.Get(n.lu.toInternalPath(n.ID), propagationAttr); err == nil {
		return string(b) == "1"
	}
	return false
}

// GetTMTime reads the tmtime from the extended attributes
func (n *Node) GetTMTime() (tmTime time.Time, err error) {
	var b []byte
	if b, err = xattr.Get(n.lu.toInternalPath(n.ID), treeMTimeAttr); err != nil {
		return
	}
	return time.Parse(time.RFC3339Nano, string(b))
}

// SetTMTime writes the tmtime to the extended attributes
func (n *Node) SetTMTime(t time.Time) (err error) {
	return xattr.Set(n.lu.toInternalPath(n.ID), treeMTimeAttr, []byte(t.UTC().Format(time.RFC3339Nano)))
}

// UnsetTempEtag removes the temporary etag attribute
func (n *Node) UnsetTempEtag() (err error) {
	if err = xattr.Remove(n.lu.toInternalPath(n.ID), tmpEtagAttr); err != nil {
		if e, ok := err.(*xattr.Error); ok && (e.Err.Error() == "no data available" ||
			// darwin
			e.Err.Error() == "attribute not found") {
			return nil
		}
	}
	return err
}

// ListGrantees lists the grantees of the current node
// We don't want to wast time and memory by creating grantee objects.
// The function will return a list of opaque strings that can be used to make a ReadGrant call
func (n *Node) ListGrantees(ctx context.Context) (grantees []string, err error) {
	var attrs []string
	if attrs, err = xattr.List(n.lu.toInternalPath(n.ID)); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("error listing attributes")
		return nil, err
	}
	for i := range attrs {
		if strings.HasPrefix(attrs[i], grantPrefix) {
			grantees = append(grantees, attrs[i])
		}
	}
	return
}

// ReadGrant reads a CS3 grant
func (n *Node) ReadGrant(ctx context.Context, grantee string) (g *provider.Grant, err error) {
	var b []byte
	if b, err = xattr.Get(n.lu.toInternalPath(n.ID), grantee); err != nil {
		return nil, err
	}
	var e *ace.ACE
	if e, err = ace.Unmarshal(strings.TrimPrefix(grantee, grantPrefix), b); err != nil {
		return nil, err
	}
	return e.Grant(), nil
}
