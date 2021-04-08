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

package node

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage/utils/ace"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/pkg/user"
)

// Define keys and values used in the node metadata
const (
	FavoriteKey   = "http://owncloud.org/ns/favorite"
	ShareTypesKey = "http://owncloud.org/ns/share-types"
	ChecksumsKey  = "http://owncloud.org/ns/checksums"
	UserShareType = "0"
	QuotaKey      = "quota"

	QuotaUncalculated = "-1"
	QuotaUnknown      = "-2"
	QuotaUnlimited    = "-3"
)

// Node represents a node in the tree and provides methods to get a Parent or Child instance
type Node struct {
	ParentID string
	ID       string
	Name     string
	Blobsize int64
	BlobID   string
	owner    *userpb.UserId
	Exists   bool

	lu PathLookup
}

// PathLookup defines the interface for the lookup component
type PathLookup interface {
	RootNode(ctx context.Context) (node *Node, err error)
	HomeOrRootNode(ctx context.Context) (node *Node, err error)

	InternalRoot() string
	InternalPath(ID string) string
	Path(ctx context.Context, n *Node) (path string, err error)
}

// New returns a new instance of Node
func New(id, parentID, name string, blobsize int64, blobID string, owner *userpb.UserId, lu PathLookup) *Node {
	if blobID == "" {
		blobID = uuid.New().String()
	}
	return &Node{
		ID:       id,
		ParentID: parentID,
		Name:     name,
		Blobsize: blobsize,
		owner:    owner,
		lu:       lu,
		BlobID:   blobID,
	}
}

// WriteMetadata writes the Node metadata to disk
func (n *Node) WriteMetadata(owner *userpb.UserId) (err error) {
	nodePath := n.InternalPath()
	if err = xattr.Set(nodePath, xattrs.ParentidAttr, []byte(n.ParentID)); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not set parentid attribute")
	}
	if err = xattr.Set(nodePath, xattrs.NameAttr, []byte(n.Name)); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not set name attribute")
	}
	if err = xattr.Set(nodePath, xattrs.BlobIDAttr, []byte(n.BlobID)); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not set blobid attribute")
	}
	if err = xattr.Set(nodePath, xattrs.BlobsizeAttr, []byte(fmt.Sprintf("%d", n.Blobsize))); err != nil {
		return errors.Wrap(err, "Decomposedfs: could not set blobsize attribute")
	}
	if owner == nil {
		if err = xattr.Set(nodePath, xattrs.OwnerIDAttr, []byte("")); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not set empty owner id attribute")
		}
		if err = xattr.Set(nodePath, xattrs.OwnerIDPAttr, []byte("")); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not set empty owner idp attribute")
		}
	} else {
		if err = xattr.Set(nodePath, xattrs.OwnerIDAttr, []byte(owner.OpaqueId)); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not set owner id attribute")
		}
		if err = xattr.Set(nodePath, xattrs.OwnerIDPAttr, []byte(owner.Idp)); err != nil {
			return errors.Wrap(err, "Decomposedfs: could not set owner idp attribute")
		}
	}
	return
}

// ReadNode creates a new instance from an id and checks if it exists
func ReadNode(ctx context.Context, lu PathLookup, id string) (n *Node, err error) {
	n = &Node{
		lu: lu,
		ID: id,
	}

	nodePath := n.InternalPath()

	// lookup parent id in extended attributes
	var attrBytes []byte
	attrBytes, err = xattr.Get(nodePath, xattrs.ParentidAttr)
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
	if attrBytes, err = xattr.Get(nodePath, xattrs.NameAttr); err == nil {
		n.Name = string(attrBytes)
	} else {
		return
	}
	// lookup blobID in extended attributes
	if attrBytes, err = xattr.Get(nodePath, xattrs.BlobIDAttr); err == nil {
		n.BlobID = string(attrBytes)
	} else {
		return
	}
	// Lookup blobsize
	var blobSize int64
	if blobSize, err = ReadBlobSizeAttr(nodePath); err == nil {
		n.Blobsize = blobSize
	} else {
		return
	}

	// Check if parent exists. Otherwise this node is part of a deleted subtree
	_, err = os.Stat(lu.InternalPath(n.ParentID))
	if err != nil {
		if isNotFound(err) {
			return nil, errtypes.NotFound(err.Error())
		}
		return nil, err
	}
	n.Exists = true
	return
}

// The os error is buried inside the fs.PathError error
func isNotDir(err error) bool {
	if perr, ok := err.(*fs.PathError); ok {
		if serr, ok2 := perr.Err.(syscall.Errno); ok2 {
			return serr == syscall.ENOTDIR
		}
	}
	return false
}

// Child returns the child node with the given name
func (n *Node) Child(ctx context.Context, name string) (*Node, error) {
	link, err := os.Readlink(filepath.Join(n.InternalPath(), name))
	if err != nil {
		if os.IsNotExist(err) || isNotDir(err) {
			c := &Node{
				lu:       n.lu,
				ParentID: n.ID,
				Name:     name,
			}
			return c, nil // if the file does not exist we return a node that has Exists = false
		}

		return nil, errors.Wrap(err, "Decomposedfs: Wrap: readlink error")
	}

	var c *Node
	if strings.HasPrefix(link, "../") {
		c, err = ReadNode(ctx, n.lu, filepath.Base(link))
		if err != nil {
			return nil, errors.Wrap(err, "could not read child node")
		}
	} else {
		return nil, fmt.Errorf("Decomposedfs: expected '../ prefix, got' %+v", link)
	}

	return c, nil
}

// Parent returns the parent node
func (n *Node) Parent() (p *Node, err error) {
	if n.ParentID == "" {
		return nil, fmt.Errorf("Decomposedfs: root has no parent")
	}
	p = &Node{
		lu: n.lu,
		ID: n.ParentID,
	}

	parentPath := n.lu.InternalPath(n.ParentID)

	// lookup parent id in extended attributes
	var attrBytes []byte
	if attrBytes, err = xattr.Get(parentPath, xattrs.ParentidAttr); err == nil {
		p.ParentID = string(attrBytes)
	} else {
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(parentPath, xattrs.NameAttr); err == nil {
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
	nodePath := n.InternalPath()
	// lookup parent id in extended attributes
	var attrBytes []byte
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, xattrs.OwnerIDAttr); err == nil {
		if n.owner == nil {
			n.owner = &userpb.UserId{}
		}
		n.owner.OpaqueId = string(attrBytes)
	} else {
		return
	}
	// lookup name in extended attributes
	if attrBytes, err = xattr.Get(nodePath, xattrs.OwnerIDPAttr); err == nil {
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
		return NoPermissions
	}
	if o, _ := n.Owner(); isSameUserID(u.Id, o) {
		return OwnerPermissions
	}
	// read the permissions for the current user from the acls of the current node
	if np, err := n.ReadUserPermissions(ctx, u); err == nil {
		return np
	}
	return NoPermissions
}

// InternalPath returns the internal path of the Node
func (n *Node) InternalPath() string {
	return n.lu.InternalPath(n.ID)
}

// CalculateEtag returns a hash of fileid + tmtime (or mtime)
func CalculateEtag(nodeID string, tmTime time.Time) (string, error) {
	return calculateEtag(nodeID, tmTime)
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
	return fmt.Sprintf(`"%x"`, h.Sum(nil)), nil
}

// SetMtime sets the mtime and atime of a node
func (n *Node) SetMtime(ctx context.Context, mtime string) error {
	sublog := appctx.GetLogger(ctx).With().Interface("node", n).Logger()
	if mt, err := parseMTime(mtime); err == nil {
		nodePath := n.lu.InternalPath(n.ID)
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
	nodePath := n.lu.InternalPath(n.ID)
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
	return xattr.Set(nodePath, xattrs.TmpEtagAttr, []byte(val))
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
	nodePath := n.lu.InternalPath(n.ID)
	// the favorite flag is specific to the user, so we need to incorporate the userid
	fa := fmt.Sprintf("%s%s@%s", xattrs.FavPrefix, uid.GetOpaqueId(), uid.GetIdp())
	return xattr.Set(nodePath, fa, []byte(val))
}

// AsResourceInfo return the node as CS3 ResourceInfo
func (n *Node) AsResourceInfo(ctx context.Context, rp *provider.ResourcePermissions, mdKeys []string) (ri *provider.ResourceInfo, err error) {
	sublog := appctx.GetLogger(ctx).With().Interface("node", n).Logger()

	var fn string
	nodePath := n.lu.InternalPath(n.ID)

	var fi os.FileInfo

	nodeType := provider.ResourceType_RESOURCE_TYPE_INVALID
	if fi, err = os.Lstat(nodePath); err != nil {
		return
	}

	var target []byte
	switch {
	case fi.IsDir():
		if target, err = xattr.Get(nodePath, xattrs.ReferenceAttr); err == nil {
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
		Size:          uint64(n.Blobsize),
		Target:        string(target),
		PermissionSet: rp,
	}

	if nodeType == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		ts, err := n.GetTreeSize()
		if err == nil {
			ri.Size = ts
		} else {
			ri.Size = 0 // make dirs always return 0 if it is unknown
			sublog.Debug().Err(err).Msg("could not read treesize")
		}
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
	if b, err := xattr.Get(nodePath, xattrs.TmpEtagAttr); err == nil {
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
	if _, ok := mdKeysMap[FavoriteKey]; returnAllKeys || ok {
		favorite := ""
		if u, ok := user.ContextGetUser(ctx); ok {
			// the favorite flag is specific to the user, so we need to incorporate the userid
			if uid := u.GetId(); uid != nil {
				fa := fmt.Sprintf("%s%s@%s", xattrs.FavPrefix, uid.GetOpaqueId(), uid.GetIdp())
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
		metadata[FavoriteKey] = favorite
	}

	// share indicator
	if _, ok := mdKeysMap[ShareTypesKey]; returnAllKeys || ok {
		if n.hasUserShares(ctx) {
			metadata[ShareTypesKey] = UserShareType
		}
	}

	// checksums
	if _, ok := mdKeysMap[ChecksumsKey]; (nodeType == provider.ResourceType_RESOURCE_TYPE_FILE) && returnAllKeys || ok {
		// TODO which checksum was requested? sha1 adler32 or md5? for now hardcode sha1?
		readChecksumIntoResourceChecksum(ctx, nodePath, storageprovider.XSSHA1, ri)
		readChecksumIntoOpaque(ctx, nodePath, storageprovider.XSMD5, ri)
		readChecksumIntoOpaque(ctx, nodePath, storageprovider.XSAdler32, ri)
	}
	// quota
	if _, ok := mdKeysMap[QuotaKey]; (nodeType == provider.ResourceType_RESOURCE_TYPE_CONTAINER) && returnAllKeys || ok {
		var quotaPath string
		if r, err := n.lu.HomeOrRootNode(ctx); err == nil {
			quotaPath = r.InternalPath()
			readQuotaIntoOpaque(ctx, quotaPath, ri)
		} else {
			sublog.Error().Err(err).Msg("error determining home or root node for quota")
		}
	}

	// only read the requested metadata attributes
	attrs, err := xattr.List(nodePath)
	if err != nil {
		sublog.Error().Err(err).Msg("error getting list of extended attributes")
	} else {
		for i := range attrs {
			// filter out non-custom properties
			if !strings.HasPrefix(attrs[i], xattrs.MetadataPrefix) {
				continue
			}
			// only read when key was requested
			k := attrs[i][len(xattrs.MetadataPrefix):]
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

func readChecksumIntoResourceChecksum(ctx context.Context, nodePath, algo string, ri *provider.ResourceInfo) {
	v, err := xattr.Get(nodePath, xattrs.ChecksumPrefix+algo)
	switch {
	case err == nil:
		ri.Checksum = &provider.ResourceChecksum{
			Type: storageprovider.PKG2GRPCXS(algo),
			Sum:  hex.EncodeToString(v),
		}
	case isNoData(err):
		appctx.GetLogger(ctx).Debug().Err(err).Str("nodepath", nodePath).Str("algorithm", algo).Msg("checksum not set")
	case isNotFound(err):
		appctx.GetLogger(ctx).Error().Err(err).Str("nodepath", nodePath).Str("algorithm", algo).Msg("file not fount")
	default:
		appctx.GetLogger(ctx).Error().Err(err).Str("nodepath", nodePath).Str("algorithm", algo).Msg("could not read checksum")
	}
}

func readChecksumIntoOpaque(ctx context.Context, nodePath, algo string, ri *provider.ResourceInfo) {
	v, err := xattr.Get(nodePath, xattrs.ChecksumPrefix+algo)
	switch {
	case err == nil:
		if ri.Opaque == nil {
			ri.Opaque = &types.Opaque{
				Map: map[string]*types.OpaqueEntry{},
			}
		}
		ri.Opaque.Map[algo] = &types.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte(hex.EncodeToString(v)),
		}
	case isNoData(err):
		appctx.GetLogger(ctx).Debug().Err(err).Str("nodepath", nodePath).Str("algorithm", algo).Msg("checksum not set")
	case isNotFound(err):
		appctx.GetLogger(ctx).Error().Err(err).Str("nodepath", nodePath).Str("algorithm", algo).Msg("file not fount")
	default:
		appctx.GetLogger(ctx).Error().Err(err).Str("nodepath", nodePath).Str("algorithm", algo).Msg("could not read checksum")
	}
}

// quota is always stored on the root node
func readQuotaIntoOpaque(ctx context.Context, nodePath string, ri *provider.ResourceInfo) {
	v, err := xattr.Get(nodePath, xattrs.QuotaAttr)
	switch {
	case err == nil:
		// make sure we have a proper signed int
		// we use the same magic numbers to indicate:
		// -1 = uncalculated
		// -2 = unknown
		// -3 = unlimited
		if _, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			if ri.Opaque == nil {
				ri.Opaque = &types.Opaque{
					Map: map[string]*types.OpaqueEntry{},
				}
			}
			ri.Opaque.Map[QuotaKey] = &types.OpaqueEntry{
				Decoder: "plain",
				Value:   v,
			}
		} else {
			appctx.GetLogger(ctx).Error().Err(err).Str("nodepath", nodePath).Str("quota", string(v)).Msg("malformed quota")
		}
	case isNoData(err):
		appctx.GetLogger(ctx).Debug().Err(err).Str("nodepath", nodePath).Msg("quota not set")
	case isNotFound(err):
		appctx.GetLogger(ctx).Error().Err(err).Str("nodepath", nodePath).Msg("file not found when reading quota")
	default:
		appctx.GetLogger(ctx).Error().Err(err).Str("nodepath", nodePath).Msg("could not read quota")
	}
}

// HasPropagation checks if the propagation attribute exists and is set to "1"
func (n *Node) HasPropagation() (propagation bool) {
	if b, err := xattr.Get(n.lu.InternalPath(n.ID), xattrs.PropagationAttr); err == nil {
		return string(b) == "1"
	}
	return false
}

// GetTMTime reads the tmtime from the extended attributes
func (n *Node) GetTMTime() (tmTime time.Time, err error) {
	var b []byte
	if b, err = xattr.Get(n.lu.InternalPath(n.ID), xattrs.TreeMTimeAttr); err != nil {
		return
	}
	return time.Parse(time.RFC3339Nano, string(b))
}

// SetTMTime writes the tmtime to the extended attributes
func (n *Node) SetTMTime(t time.Time) (err error) {
	return xattr.Set(n.lu.InternalPath(n.ID), xattrs.TreeMTimeAttr, []byte(t.UTC().Format(time.RFC3339Nano)))
}

// GetTreeSize reads the treesize from the extended attributes
func (n *Node) GetTreeSize() (treesize uint64, err error) {
	var b []byte
	if b, err = xattr.Get(n.InternalPath(), xattrs.TreesizeAttr); err != nil {
		return
	}
	return strconv.ParseUint(string(b), 10, 64)
}

// SetTreeSize writes the treesize to the extended attributes
func (n *Node) SetTreeSize(ts uint64) (err error) {
	return xattr.Set(n.InternalPath(), xattrs.TreesizeAttr, []byte(strconv.FormatUint(ts, 10)))
}

// SetChecksum writes the checksum with the given checksum type to the extended attributes
func (n *Node) SetChecksum(csType string, h hash.Hash) (err error) {
	return xattr.Set(n.lu.InternalPath(n.ID), xattrs.ChecksumPrefix+csType, h.Sum(nil))
}

// UnsetTempEtag removes the temporary etag attribute
func (n *Node) UnsetTempEtag() (err error) {
	if err = xattr.Remove(n.lu.InternalPath(n.ID), xattrs.TmpEtagAttr); err != nil {
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
		return NoPermissions, err
	}
	if o.OpaqueId == "" {
		// this happens for root nodes in the storage. the extended attributes are set to emptystring to indicate: no owner
		// TODO what if no owner is set but grants are present?
		return NoOwnerPermissions, nil
	}
	if isSameUserID(u.Id, o) {
		appctx.GetLogger(ctx).Debug().Interface("node", n).Msg("user is owner, returning owner permissions")
		return OwnerPermissions, nil
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
	userace := xattrs.GrantPrefix + xattrs.UserAcePrefix + u.Id.OpaqueId
	userFound := false
	for i := range grantees {
		switch {
		// we only need to find the user once
		case !userFound && grantees[i] == userace:
			g, err = n.ReadGrant(ctx, grantees[i])
		case strings.HasPrefix(grantees[i], xattrs.GrantPrefix+xattrs.GroupAcePrefix): // only check group grantees
			gr := strings.TrimPrefix(grantees[i], xattrs.GrantPrefix+xattrs.GroupAcePrefix)
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
			AddPermissions(ap, g.GetPermissions())
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
	if attrs, err = xattr.List(n.InternalPath()); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("error listing attributes")
		return nil, err
	}
	for i := range attrs {
		if strings.HasPrefix(attrs[i], xattrs.GrantPrefix) {
			grantees = append(grantees, attrs[i])
		}
	}
	return
}

// ReadGrant reads a CS3 grant
func (n *Node) ReadGrant(ctx context.Context, grantee string) (g *provider.Grant, err error) {
	var b []byte
	if b, err = xattr.Get(n.InternalPath(), grantee); err != nil {
		return nil, err
	}
	var e *ace.ACE
	if e, err = ace.Unmarshal(strings.TrimPrefix(grantee, xattrs.GrantPrefix), b); err != nil {
		return nil, err
	}
	return e.Grant(), nil
}

// ReadBlobSizeAttr reads the blobsize from the xattrs
func ReadBlobSizeAttr(path string) (int64, error) {
	attrBytes, err := xattr.Get(path, xattrs.BlobsizeAttr)
	if err != nil {
		return 0, errors.Wrapf(err, "error reading blobsize xattr")
	}
	blobSize, err := strconv.ParseInt(string(attrBytes), 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid blobsize xattr format")
	}
	return blobSize, nil
}

func (n *Node) hasUserShares(ctx context.Context) bool {
	g, err := n.ListGrantees(ctx)
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("hasUserShares: listGrantees")
		return false
	}

	for i := range g {
		if strings.Contains(g[i], xattrs.GrantPrefix+xattrs.UserAcePrefix) {
			return true
		}
	}
	return false
}

func isSameUserID(i *userpb.UserId, j *userpb.UserId) bool {
	switch {
	case i == nil, j == nil:
		return false
	case i.OpaqueId == j.OpaqueId && i.Idp == j.Idp:
		return true
	default:
		return false
	}
}

func parseMTime(v string) (t time.Time, err error) {
	p := strings.SplitN(v, ".", 2)
	var sec, nsec int64
	if sec, err = strconv.ParseInt(p[0], 10, 64); err == nil {
		if len(p) > 1 {
			nsec, err = strconv.ParseInt(p[1], 10, 64)
		}
	}
	return time.Unix(sec, nsec), err
}
