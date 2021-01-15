// Copyright 2018-2021 CERN
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
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage/utils/ace"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
	"github.com/rs/zerolog/log"
)

const (
	_shareTypesKey = "http://owncloud.org/ns/share-types"
	_userShareType = "0"

	_favoriteKey  = "http://owncloud.org/ns/favorite"
	_checksumsKey = "http://owncloud.org/ns/checksums"
)

// Node represents a node in the tree and provides methods to get a Parent or Child instance
type Node struct {
	lu       *Lookup
	ParentID string
	ID       string
	Name     string
	owner    *userpb.UserId
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
		n.owner = &userpb.UserId{}
		n.owner.OpaqueId = string(attrBytes)
	} else {
		return
	}
	// lookup ownerIdp in extended attributes
	if attrBytes, err = xattr.Get(deletedNodePath, ownerIDPAttr); err == nil {
		if n.owner == nil {
			n.owner = &userpb.UserId{}
		}
		n.owner.Idp = string(attrBytes)
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
func (n *Node) Owner() (o *userpb.UserId, err error) {
	if n.owner != nil {
		return n.owner, nil
	}

	// FIXME ... do we return the owner of the reference or the owner of the target?
	// we don't really know the owner of the target ... and as the reference may point anywhere we cannot really find out
	// but what are the permissions? all? none? the gateway has to fill in?
	// TODO what if this is a reference?
	nodePath := n.lu.toInternalPath(n.ID)
	// lookup parent id in extended attributes
	var attrBytes []byte
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, ownerIDAttr); err == nil {
		if n.owner == nil {
			n.owner = &userpb.UserId{}
		}
		n.owner.OpaqueId = string(attrBytes)
	} else {
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, ownerIDPAttr); err == nil {
		if n.owner == nil {
			n.owner = &userpb.UserId{}
		}
		n.owner.Idp = string(attrBytes)
	} else {
		return
	}
	return n.owner, err
}

// PermissionSet returns the permission set for the current user
// the parent nodes are not taken into account
func (n *Node) PermissionSet(ctx context.Context) *provider.ResourcePermissions {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		appctx.GetLogger(ctx).Debug().Interface("node", n).Msg("no user in context, returning default permissions")
		return noPermissions
	}
	if o, _ := n.Owner(); isSameUserID(u.Id, o) {
		return ownerPermissions
	}
	// read the permissions for the current user from the acls of the current node
	if np, err := n.ReadUserPermissions(ctx, u); err == nil {
		return np
	}
	return noPermissions
}

// calculateEtag returns a hash of fileid + tmtime (or mtime)
func calculateEtag(nodeID string, tmTime time.Time) (string, error) {
	h := md5.New()
	if _, err := io.WriteString(h, nodeID); err != nil {
		return "", err
	}
	if tb, err := tmTime.UTC().MarshalBinary(); err == nil {
		if _, err := h.Write(tb); err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	return `"` + hex.EncodeToString(h.Sum(nil)) + `"`, nil
}

// SetMtime sets the mtime and atime of a node
func (n *Node) SetMtime(ctx context.Context, mtime string) error {
	sublog := appctx.GetLogger(ctx).With().Interface("node", n).Logger()
	if mt, err := parseMTime(mtime); err == nil {
		nodePath := n.lu.toInternalPath(n.ID)
		// updating mtime also updates atime
		if err := os.Chtimes(nodePath, mt, mt); err != nil {
			sublog.Error().Err(err).
				Time("mtime", mt).
				Msg("could not set mtime")
			return errors.Wrap(err, "could not set mtime")
		}
	} else {
		sublog.Error().Err(err).
			Str("mtime", mtime).
			Msg("could not parse mtime")
		return errors.Wrap(err, "could not parse mtime")
	}
	return nil
}

// SetEtag sets the temporary etag of a node if it differs from the current etag
func (n *Node) SetEtag(ctx context.Context, val string) (err error) {
	sublog := appctx.GetLogger(ctx).With().Interface("node", n).Logger()
	nodePath := n.lu.toInternalPath(n.ID)
	var tmTime time.Time
	if tmTime, err = n.GetTMTime(); err != nil {
		// no tmtime, use mtime
		var fi os.FileInfo
		if fi, err = os.Lstat(nodePath); err != nil {
			return
		}
		tmTime = fi.ModTime()
	}
	var etag string
	if etag, err = calculateEtag(n.ID, tmTime); err != nil {
		return
	}

	// sanitize etag
	val = fmt.Sprintf("\"%s\"", strings.Trim(val, "\""))
	if etag == val {
		sublog.Debug().
			Str("etag", val).
			Msg("ignoring request to update identical etag")
		return nil
	}
	// etag is only valid until the calculated etag changes, is part of propagation
	return xattr.Set(nodePath, tmpEtagAttr, []byte(val))
}

// SetFavorite sets the favorite for the current user
// TODO we should not mess with the user here ... the favorites is now a user specific property for a file
// that cannot be mapped to extended attributes without leaking who has marked a file as a favorite
// it is a specific case of a tag, which is user individual as well
// TODO there are different types of tags
// 1. public that are managed by everyone
// 2. private tags that are only visible to the user
// 3. system tags that are only visible to the system
// 4. group tags that are only visible to a group ...
// urgh ... well this can be solved using different namespaces
// 1. public = p:
// 2. private = u:<uid>: for user specific
// 3. system = s: for system
// 4. group = g:<gid>:
// 5. app? = a:<aid>: for apps?
// obviously this only is secure when the u/s/g/a namespaces are not accessible by users in the filesystem
// public tags can be mapped to extended attributes
func (n *Node) SetFavorite(uid *userpb.UserId, val string) error {
	nodePath := n.lu.toInternalPath(n.ID)
	// the favorite flag is specific to the user, so we need to incorporate the userid
	fa := fmt.Sprintf("%s%s@%s", favPrefix, uid.GetOpaqueId(), uid.GetIdp())
	return xattr.Set(nodePath, fa, []byte(val))
}

// AsResourceInfo return the node as CS3 ResourceInfo
func (n *Node) AsResourceInfo(ctx context.Context, rp *provider.ResourcePermissions, mdKeys []string) (ri *provider.ResourceInfo, err error) {
	sublog := appctx.GetLogger(ctx).With().Interface("node", n).Logger()

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
		Id:            id,
		Path:          fn,
		Type:          nodeType,
		MimeType:      mime.Detect(nodeType == provider.ResourceType_RESOURCE_TYPE_CONTAINER, fn),
		Size:          uint64(fi.Size()),
		Target:        string(target),
		PermissionSet: rp,
	}

	if ri.Owner, err = n.Owner(); err != nil {
		sublog.Debug().Err(err).Msg("could not determine owner")
	}

	// TODO make etag of files use fileid and checksum

	var tmTime time.Time
	if tmTime, err = n.GetTMTime(); err != nil {
		// no tmtime, use mtime
		tmTime = fi.ModTime()
	}

	// use temporary etag if it is set
	if b, err := xattr.Get(nodePath, tmpEtagAttr); err == nil {
		ri.Etag = fmt.Sprintf(`"%x"`, string(b)) // TODO why do we convert string(b)? is the temporary etag stored as string? -> should we use bytes? use hex.EncodeToString?
	} else if ri.Etag, err = calculateEtag(n.ID, tmTime); err != nil {
		sublog.Debug().Err(err).Msg("could not calculate etag")
	}

	// mtime uses tmtime if present
	// TODO expose mtime and tmtime separately?
	un := tmTime.UnixNano()
	ri.Mtime = &types.Timestamp{
		Seconds: uint64(un / 1000000000),
		Nanos:   uint32(un % 1000000000),
	}

	mdKeysMap := make(map[string]struct{})
	for _, k := range mdKeys {
		mdKeysMap[k] = struct{}{}
	}

	var returnAllKeys bool
	if _, ok := mdKeysMap["*"]; len(mdKeys) == 0 || ok {
		returnAllKeys = true
	}

	metadata := map[string]string{}

	// read favorite flag for the current user
	if _, ok := mdKeysMap[_favoriteKey]; returnAllKeys || ok {
		favorite := ""
		if u, ok := user.ContextGetUser(ctx); ok {
			// the favorite flag is specific to the user, so we need to incorporate the userid
			if uid := u.GetId(); uid != nil {
				fa := fmt.Sprintf("%s%s@%s", favPrefix, uid.GetOpaqueId(), uid.GetIdp())
				if val, err := xattr.Get(nodePath, fa); err == nil {
					sublog.Debug().
						Str("favorite", fa).
						Msg("found favorite flag")
					favorite = string(val)
				}
			} else {
				sublog.Error().Err(errtypes.UserRequired("userrequired")).Msg("user has no id")
			}
		} else {
			sublog.Error().Err(errtypes.UserRequired("userrequired")).Msg("error getting user from ctx")
		}
		metadata[_favoriteKey] = favorite
	}

	// share indicator
	if _, ok := mdKeysMap[_shareTypesKey]; returnAllKeys || ok {
		if n.hasUserShares(ctx) {
			metadata[_shareTypesKey] = _userShareType
		}
	}

	// checksums
	if _, ok := mdKeysMap[_checksumsKey]; returnAllKeys || ok {
		// TODO which checksum was requested? sha1 adler32 or md5? for now hardcode sha1?
		if v, err := xattr.Get(nodePath, checksumPrefix+"sha1"); err == nil {
			ri.Checksum = &provider.ResourceChecksum{
				Type: storageprovider.PKG2GRPCXS("sha1"),
				Sum:  hex.EncodeToString(v),
			}
		} else {
			sublog.Error().Err(err).Str("cstype", "sha1").Msg("could not get checksum value")
		}
		if v, err := xattr.Get(nodePath, checksumPrefix+"md5"); err == nil {
			if ri.Opaque == nil {
				ri.Opaque = &types.Opaque{
					Map: map[string]*types.OpaqueEntry{},
				}
			}
			ri.Opaque.Map["md5"] = &types.OpaqueEntry{
				Decoder: "plain",
				Value:   []byte(hex.EncodeToString(v)),
			}
		} else {
			sublog.Error().Err(err).Str("cstype", "md5").Msg("could not get checksum value")
		}
		if v, err := xattr.Get(nodePath, checksumPrefix+"adler32"); err == nil {
			if ri.Opaque == nil {
				ri.Opaque = &types.Opaque{
					Map: map[string]*types.OpaqueEntry{},
				}
			}
			ri.Opaque.Map["adler32"] = &types.OpaqueEntry{
				Decoder: "plain",
				Value:   []byte(hex.EncodeToString(v)),
			}
		} else {
			sublog.Error().Err(err).Str("cstype", "adler32").Msg("could not get checksum value")
		}
	}

	// only read the requested metadata attributes
	attrs, err := xattr.List(nodePath)
	if err != nil {
		sublog.Error().Err(err).Msg("error getting list of extended attributes")
	} else {
		for i := range attrs {
			// filter out non-custom properties
			if !strings.HasPrefix(attrs[i], metadataPrefix) {
				continue
			}
			// only read when key was requested
			k := attrs[i][len(metadataPrefix):]
			if _, ok := mdKeysMap[k]; returnAllKeys || ok {
				if val, err := xattr.Get(nodePath, attrs[i]); err == nil {
					metadata[k] = string(val)
				} else {
					sublog.Error().Err(err).
						Str("entry", attrs[i]).
						Msg("error retrieving xattr metadata")
				}
			}

		}
	}
	ri.ArbitraryMetadata = &provider.ArbitraryMetadata{
		Metadata: metadata,
	}

	sublog.Debug().
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

// SetChecksum writes the checksum with the given checksum type to the extended attributes
func (n *Node) SetChecksum(csType string, bytes []byte) (err error) {
	return xattr.Set(n.lu.toInternalPath(n.ID), checksumPrefix+csType, bytes)
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

// ReadUserPermissions will assemble the permissions for the current user on the given node without parent nodes
func (n *Node) ReadUserPermissions(ctx context.Context, u *userpb.User) (ap *provider.ResourcePermissions, err error) {
	// check if the current user is the owner
	o, err := n.Owner()
	if err != nil {
		// TODO check if a parent folder has the owner set?
		appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not determine owner, returning default permissions")
		return noPermissions, err
	}
	if o.OpaqueId == "" {
		// this happens for root nodes in the storage. the extended attributes are set to emptystring to indicate: no owner
		// TODO what if no owner is set but grants are present?
		return noOwnerPermissions, nil
	}
	if isSameUserID(u.Id, o) {
		appctx.GetLogger(ctx).Debug().Interface("node", n).Msg("user is owner, returning owner permissions")
		return ownerPermissions, nil
	}

	ap = &provider.ResourcePermissions{}

	// for an efficient group lookup convert the list of groups to a map
	// groups are just strings ... groupnames ... or group ids ??? AAARGH !!!
	groupsMap := make(map[string]bool, len(u.Groups))
	for i := range u.Groups {
		groupsMap[u.Groups[i]] = true
	}

	var g *provider.Grant

	// we read all grantees from the node
	var grantees []string
	if grantees, err = n.ListGrantees(ctx); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("error listing grantees")
		return nil, err
	}

	// instead of making n getxattr syscalls we are going to list the acls and filter them here
	// we have two options here:
	// 1. we can start iterating over the acls / grants on the node or
	// 2. we can iterate over the number of groups
	// The current implementation tries to be defensive for cases where users have hundreds or thousands of groups, so we iterate over the existing acls.
	userace := grantPrefix + _userAcePrefix + u.Id.OpaqueId
	userFound := false
	for i := range grantees {
		switch {
		// we only need to find the user once
		case !userFound && grantees[i] == userace:
			g, err = n.ReadGrant(ctx, grantees[i])
		case strings.HasPrefix(grantees[i], grantPrefix+_groupAcePrefix): // only check group grantees
			gr := strings.TrimPrefix(grantees[i], grantPrefix+_groupAcePrefix)
			if groupsMap[gr] {
				g, err = n.ReadGrant(ctx, grantees[i])
			} else {
				// no need to check attribute
				continue
			}
		default:
			// no need to check attribute
			continue
		}

		switch {
		case err == nil:
			addPermissions(ap, g.GetPermissions())
		case isNoData(err):
			err = nil
			appctx.GetLogger(ctx).Error().Interface("node", n).Str("grant", grantees[i]).Interface("grantees", grantees).Msg("grant vanished from node after listing")
			// continue with next segment
		default:
			appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Str("grant", grantees[i]).Msg("error reading permissions")
			// continue with next segment
		}
	}

	appctx.GetLogger(ctx).Debug().Interface("permissions", ap).Interface("node", n).Interface("user", u).Msg("returning aggregated permissions")
	return ap, nil
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

func (n *Node) hasUserShares(ctx context.Context) bool {
	g, err := n.ListGrantees(ctx)
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("hasUserShares: listGrantees")
		return false
	}

	for i := range g {
		if strings.Contains(g[i], grantPrefix+_userAcePrefix) {
			return true
		}
	}
	return false
}
