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

package decomposedfs

// go:generate mockery -name PermissionsChecker
// go:generate mockery -name Tree

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// PermissionsChecker defines an interface for checking permissions on a Node
type PermissionsChecker interface {
	AssemblePermissions(ctx context.Context, n *node.Node) (ap *provider.ResourcePermissions, err error)
	HasPermission(ctx context.Context, n *node.Node, check func(*provider.ResourcePermissions) bool) (can bool, err error)
}

// Tree is used to manage a tree hierarchy
type Tree interface {
	Setup(owner string) error

	GetMD(ctx context.Context, node *node.Node) (os.FileInfo, error)
	ListFolder(ctx context.Context, node *node.Node) ([]*node.Node, error)
	// CreateHome(owner *userpb.UserId) (n *node.Node, err error)
	CreateDir(ctx context.Context, node *node.Node) (err error)
	// CreateReference(ctx context.Context, node *node.Node, targetURI *url.URL) error
	Move(ctx context.Context, oldNode *node.Node, newNode *node.Node) (err error)
	Delete(ctx context.Context, node *node.Node) (err error)
	RestoreRecycleItemFunc(ctx context.Context, key, restorePath string) (*node.Node, func() error, error)
	PurgeRecycleItemFunc(ctx context.Context, key string) (*node.Node, func() error, error)

	WriteBlob(key string, reader io.Reader) error
	ReadBlob(key string) (io.ReadCloser, error)
	DeleteBlob(key string) error

	Propagate(ctx context.Context, node *node.Node) (err error)
}

// Decomposedfs provides the base for decomposed filesystem implementations
type Decomposedfs struct {
	lu           *Lookup
	tp           Tree
	o            *options.Options
	p            PermissionsChecker
	chunkHandler *chunking.ChunkHandler
}

// NewDefault returns an instance with default components
func NewDefault(m map[string]interface{}, bs tree.Blobstore) (storage.FS, error) {
	o, err := options.New(m)
	if err != nil {
		return nil, err
	}

	lu := &Lookup{}
	p := node.NewPermissions(lu)

	lu.Options = o

	tp := tree.New(o.Root, o.TreeTimeAccounting, o.TreeSizeAccounting, lu, bs)
	return New(o, lu, p, tp)
}

// New returns an implementation of the storage.FS interface that talks to
// a local filesystem.
func New(o *options.Options, lu *Lookup, p PermissionsChecker, tp Tree) (storage.FS, error) {
	err := tp.Setup(o.Owner)
	if err != nil {
		logger.New().Error().Err(err).
			Msg("could not setup tree")
		return nil, errors.Wrap(err, "could not setup tree")
	}

	return &Decomposedfs{
		tp:           tp,
		lu:           lu,
		o:            o,
		p:            p,
		chunkHandler: chunking.NewChunkHandler(filepath.Join(o.Root, "uploads")),
	}, nil
}

// Shutdown shuts down the storage
func (fs *Decomposedfs) Shutdown(ctx context.Context) error {
	return nil
}

// GetQuota returns the quota available
// TODO Document in the cs3 should we return quota or free space?
func (fs *Decomposedfs) GetQuota(ctx context.Context) (total uint64, inUse uint64, err error) {
	var n *node.Node
	if n, err = fs.lu.HomeOrRootNode(ctx); err != nil {
		return 0, 0, err
	}

	if !n.Exists {
		err = errtypes.NotFound(filepath.Join(n.ParentID, n.Name))
		return 0, 0, err
	}

	rp, err := fs.p.AssemblePermissions(ctx, n)
	switch {
	case err != nil:
		return 0, 0, errtypes.InternalError(err.Error())
	case !rp.GetQuota:
		return 0, 0, errtypes.PermissionDenied(n.ID)
	}

	ri, err := n.AsResourceInfo(ctx, rp, []string{"treesize", "quota"})
	if err != nil {
		return 0, 0, err
	}

	quotaStr := node.QuotaUnknown
	if ri.Opaque != nil && ri.Opaque.Map != nil && ri.Opaque.Map["quota"] != nil && ri.Opaque.Map["quota"].Decoder == "plain" {
		quotaStr = string(ri.Opaque.Map["quota"].Value)
	}

	avail, err := fs.getAvailableSize(n.InternalPath())
	if err != nil {
		return 0, 0, err
	}
	total = avail + ri.Size

	switch {
	case quotaStr == node.QuotaUncalculated, quotaStr == node.QuotaUnknown, quotaStr == node.QuotaUnlimited:
	// best we can do is return current total
	// TODO indicate unlimited total? -> in opaque data?
	default:
		if quota, err := strconv.ParseUint(quotaStr, 10, 64); err == nil {
			if total > quota {
				total = quota
			}
		}
	}

	return total, ri.Size, nil
}

// CreateHome creates a new home node for the given user
func (fs *Decomposedfs) CreateHome(ctx context.Context) (err error) {
	if !fs.o.EnableHome || fs.o.UserLayout == "" {
		return errtypes.NotSupported("Decomposedfs: CreateHome() home supported disabled")
	}

	var n, h *node.Node
	if n, err = fs.lu.RootNode(ctx); err != nil {
		return
	}
	h, err = fs.lu.WalkPath(ctx, n, fs.lu.mustGetUserLayout(ctx), func(ctx context.Context, n *node.Node) error {
		if !n.Exists {
			if err := fs.tp.CreateDir(ctx, n); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return
	}

	// update the owner
	u := user.ContextMustGetUser(ctx)
	if err = h.WriteMetadata(u.Id); err != nil {
		return
	}

	if fs.o.TreeTimeAccounting {
		homePath := h.InternalPath()
		// mark the home node as the end of propagation
		if err = xattr.Set(homePath, xattrs.PropagationAttr, []byte("1")); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("node", h).Msg("could not mark home as propagation root")
			return
		}
	}
	return
}

// GetHome is called to look up the home path for a user
// It is NOT supposed to return the internal path but the external path
func (fs *Decomposedfs) GetHome(ctx context.Context) (string, error) {
	if !fs.o.EnableHome || fs.o.UserLayout == "" {
		return "", errtypes.NotSupported("Decomposedfs: GetHome() home supported disabled")
	}
	u := user.ContextMustGetUser(ctx)
	layout := templates.WithUser(u, fs.o.UserLayout)
	return filepath.Join(fs.o.Root, layout), nil // TODO use a namespace?
}

// GetPathByID returns the fn pointed by the file id, without the internal namespace
func (fs *Decomposedfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	node, err := fs.lu.NodeFromID(ctx, id)
	if err != nil {
		return "", err
	}

	return fs.lu.Path(ctx, node)
}

// CreateDir creates the specified directory
func (fs *Decomposedfs) CreateDir(ctx context.Context, fn string) (err error) {
	var n *node.Node
	if n, err = fs.lu.NodeFromPath(ctx, fn); err != nil {
		return
	}

	if n.Exists {
		return errtypes.AlreadyExists(fn)
	}
	pn, err := n.Parent()
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: error getting parent "+n.ParentID)
	}
	ok, err := fs.p.HasPermission(ctx, pn, func(rp *provider.ResourcePermissions) bool {
		return rp.CreateContainer
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(filepath.Join(n.ParentID, n.Name))
	}

	err = fs.tp.CreateDir(ctx, n)

	if fs.o.TreeTimeAccounting {
		nodePath := n.InternalPath()
		// mark the home node as the end of propagation
		if err = xattr.Set(nodePath, xattrs.PropagationAttr, []byte("1")); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not mark node to propagate")
			return
		}
	}
	return
}

// CreateReference creates a reference as a node folder with the target stored in extended attributes
// There is no difference between the /Shares folder and normal nodes because the storage is not supposed to be accessible without the storage provider.
// In effect everything is a shadow namespace.
// To mimic the eos end owncloud driver we only allow references as children of the "/Shares" folder
// TODO when home support is enabled should the "/Shares" folder still be listed?
func (fs *Decomposedfs) CreateReference(ctx context.Context, p string, targetURI *url.URL) (err error) {

	p = strings.Trim(p, "/")
	parts := strings.Split(p, "/")

	if len(parts) != 2 {
		return errtypes.PermissionDenied("Decomposedfs: references must be a child of the share folder: share_folder=" + fs.o.ShareFolder + " path=" + p)
	}

	if parts[0] != strings.Trim(fs.o.ShareFolder, "/") {
		return errtypes.PermissionDenied("Decomposedfs: cannot create references outside the share folder: share_folder=" + fs.o.ShareFolder + " path=" + p)
	}

	// create Shares folder if it does not exist
	var n *node.Node
	if n, err = fs.lu.NodeFromPath(ctx, fs.o.ShareFolder); err != nil {
		return errtypes.InternalError(err.Error())
	} else if !n.Exists {
		if err = fs.tp.CreateDir(ctx, n); err != nil {
			return
		}
	}

	if n, err = n.Child(ctx, parts[1]); err != nil {
		return errtypes.InternalError(err.Error())
	}

	if n.Exists {
		// TODO append increasing number to mountpoint name
		return errtypes.AlreadyExists(p)
	}

	if err = fs.tp.CreateDir(ctx, n); err != nil {
		return
	}

	internal := n.InternalPath()
	if err = xattr.Set(internal, xattrs.ReferenceAttr, []byte(targetURI.String())); err != nil {
		return errors.Wrapf(err, "Decomposedfs: error setting the target %s on the reference file %s", targetURI.String(), internal)
	}
	return nil
}

// Move moves a resource from one reference to another
func (fs *Decomposedfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
	var oldNode, newNode *node.Node
	if oldNode, err = fs.lu.NodeFromResource(ctx, oldRef); err != nil {
		return
	}

	if !oldNode.Exists {
		err = errtypes.NotFound(filepath.Join(oldNode.ParentID, oldNode.Name))
		return
	}

	ok, err := fs.p.HasPermission(ctx, oldNode, func(rp *provider.ResourcePermissions) bool {
		return rp.Move
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(oldNode.ID)
	}

	if newNode, err = fs.lu.NodeFromResource(ctx, newRef); err != nil {
		return
	}
	if newNode.Exists {
		err = errtypes.AlreadyExists(filepath.Join(newNode.ParentID, newNode.Name))
		return
	}

	return fs.tp.Move(ctx, oldNode, newNode)
}

// GetMD returns the metadata for the specified resource
func (fs *Decomposedfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (ri *provider.ResourceInfo, err error) {
	var node *node.Node
	if node, err = fs.lu.NodeFromResource(ctx, ref); err != nil {
		return
	}

	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}

	rp, err := fs.p.AssemblePermissions(ctx, node)
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !rp.Stat:
		return nil, errtypes.PermissionDenied(node.ID)
	}

	return node.AsResourceInfo(ctx, rp, mdKeys)
}

// ListFolder returns a list of resources in the specified folder
func (fs *Decomposedfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) (finfos []*provider.ResourceInfo, err error) {
	var n *node.Node
	if n, err = fs.lu.NodeFromResource(ctx, ref); err != nil {
		return
	}

	if !n.Exists {
		err = errtypes.NotFound(filepath.Join(n.ParentID, n.Name))
		return
	}

	rp, err := fs.p.AssemblePermissions(ctx, n)
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !rp.ListContainer:
		return nil, errtypes.PermissionDenied(n.ID)
	}

	var children []*node.Node
	children, err = fs.tp.ListFolder(ctx, n)
	if err != nil {
		return
	}

	for i := range children {
		np := rp
		// add this childs permissions
		node.AddPermissions(np, n.PermissionSet(ctx))
		if ri, err := children[i].AsResourceInfo(ctx, np, mdKeys); err == nil {
			finfos = append(finfos, ri)
		}
	}
	return
}

// Delete deletes the specified resource
func (fs *Decomposedfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {
	var node *node.Node
	if node, err = fs.lu.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}

	ok, err := fs.p.HasPermission(ctx, node, func(rp *provider.ResourcePermissions) bool {
		return rp.Delete
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(filepath.Join(node.ParentID, node.Name))
	}

	return fs.tp.Delete(ctx, node)
}

// Download returns a reader to the specified resource
func (fs *Decomposedfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	node, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error resolving ref")
	}

	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return nil, err
	}

	ok, err := fs.p.HasPermission(ctx, node, func(rp *provider.ResourcePermissions) bool {
		return rp.InitiateFileDownload
	})
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !ok:
		return nil, errtypes.PermissionDenied(filepath.Join(node.ParentID, node.Name))
	}

	reader, err := fs.tp.ReadBlob(node.BlobID)
	if err != nil {
		return nil, errors.Wrap(err, "Decomposedfs: error download blob '"+node.ID+"'")
	}
	return reader, nil
}

func (fs *Decomposedfs) copyMD(s string, t string) (err error) {
	var attrs []string
	if attrs, err = xattr.List(s); err != nil {
		return err
	}
	for i := range attrs {
		if strings.HasPrefix(attrs[i], xattrs.OcisPrefix) {
			var d []byte
			if d, err = xattr.Get(s, attrs[i]); err != nil {
				return err
			}
			if err = xattr.Set(t, attrs[i], d); err != nil {
				return err
			}
		}
	}
	return nil
}
