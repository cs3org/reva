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
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/tree"
	"github.com/cs3org/reva/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/codes"
)

// PermissionsChecker defines an interface for checking permissions on a Node
type PermissionsChecker interface {
	AssemblePermissions(ctx context.Context, n *node.Node) (ap provider.ResourcePermissions, err error)
	HasPermission(ctx context.Context, n *node.Node, check func(*provider.ResourcePermissions) bool) (can bool, err error)
}

// Tree is used to manage a tree hierarchy
type Tree interface {
	Setup(owner *userpb.UserId, propagateToRoot bool) error

	GetMD(ctx context.Context, node *node.Node) (os.FileInfo, error)
	ListFolder(ctx context.Context, node *node.Node) ([]*node.Node, error)
	// CreateHome(owner *userpb.UserId) (n *node.Node, err error)
	CreateDir(ctx context.Context, node *node.Node) (err error)
	// CreateReference(ctx context.Context, node *node.Node, targetURI *url.URL) error
	Move(ctx context.Context, oldNode *node.Node, newNode *node.Node) (err error)
	Delete(ctx context.Context, node *node.Node) (err error)
	RestoreRecycleItemFunc(ctx context.Context, spaceid, key, trashPath string, target *node.Node) (*node.Node, *node.Node, func() error, error)
	PurgeRecycleItemFunc(ctx context.Context, spaceid, key, purgePath string) (*node.Node, func() error, error)

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

	o.GatewayAddr = sharedconf.GetGatewaySVC(o.GatewayAddr)
	return New(o, lu, p, tp)
}

// when enable home is false we want propagation to root if tree size or mtime accounting is enabled
func enablePropagationForRoot(o *options.Options) bool {
	return (o.TreeSizeAccounting || o.TreeTimeAccounting)
}

// New returns an implementation of the storage.FS interface that talks to
// a local filesystem.
func New(o *options.Options, lu *Lookup, p PermissionsChecker, tp Tree) (storage.FS, error) {
	err := tp.Setup(&userpb.UserId{
		OpaqueId: o.Owner,
		Idp:      o.OwnerIDP,
		Type:     userpb.UserType(userpb.UserType_value[o.OwnerType]),
	}, enablePropagationForRoot(o))
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
func (fs *Decomposedfs) GetQuota(ctx context.Context, ref *provider.Reference) (total uint64, inUse uint64, err error) {
	var n *node.Node
	if ref != nil {
		if n, err = fs.lu.NodeFromResource(ctx, ref); err != nil {
			return 0, 0, err
		}
	} else {
		if n, err = fs.lu.RootNode(ctx); err != nil {
			return 0, 0, err
		}
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

	ri, err := n.AsResourceInfo(ctx, &rp, []string{"treesize", "quota"}, true)
	if err != nil {
		return 0, 0, err
	}

	quotaStr := node.QuotaUnknown
	if ri.Opaque != nil && ri.Opaque.Map != nil && ri.Opaque.Map["quota"] != nil && ri.Opaque.Map["quota"].Decoder == "plain" {
		quotaStr = string(ri.Opaque.Map["quota"].Value)
	}

	avail, err := node.GetAvailableSize(n.InternalPath())
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
	if fs.o.UserLayout == "" {
		return errtypes.NotSupported("Decomposedfs: CreateHome() home supported disabled")
	}

	var n, h *node.Node
	if n, err = fs.lu.RootNode(ctx); err != nil {
		return
	}
	h, err = fs.lu.WalkPath(ctx, n, fs.lu.mustGetUserLayout(ctx), false, func(ctx context.Context, n *node.Node) error {
		if !n.Exists {
			if err := fs.tp.CreateDir(ctx, n); err != nil {
				return err
			}
		}
		return nil
	})

	// make sure to delete the created directory if things go wrong
	defer func() {
		if err != nil {
			// do not catch the error to not shadow the original error
			if tmpErr := fs.tp.Delete(ctx, n); tmpErr != nil {
				appctx.GetLogger(ctx).Error().Err(tmpErr).Msg("Can not revert file system change after error")
			}
		}
	}()

	if err != nil {
		return
	}

	// update the owner
	u := ctxpkg.ContextMustGetUser(ctx)
	if err = h.WriteAllNodeMetadata(u.Id); err != nil {
		return
	}

	if fs.o.TreeTimeAccounting || fs.o.TreeSizeAccounting {
		// mark the home node as the end of propagation
		if err = h.SetMetadata(xattrs.PropagationAttr, "1"); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("node", h).Msg("could not mark home as propagation root")
			return
		}
	}

	if err := h.SetMetadata(xattrs.SpaceNameAttr, u.DisplayName); err != nil {
		return err
	}

	// add storage space
	if err := fs.createStorageSpace(ctx, "personal", h.ID); err != nil {
		return err
	}

	return
}

// The os not exists error is buried inside the xattr error,
// so we cannot just use os.IsNotExists().
func isAlreadyExists(err error) bool {
	if xerr, ok := err.(*os.LinkError); ok {
		if serr, ok2 := xerr.Err.(syscall.Errno); ok2 {
			return serr == syscall.EEXIST
		}
	}
	return false
}

// GetHome is called to look up the home path for a user
// It is NOT supposed to return the internal path but the external path
func (fs *Decomposedfs) GetHome(ctx context.Context) (string, error) {
	if fs.o.UserLayout == "" {
		return "", errtypes.NotSupported("Decomposedfs: GetHome() home supported disabled")
	}
	u := ctxpkg.ContextMustGetUser(ctx)
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
func (fs *Decomposedfs) CreateDir(ctx context.Context, ref *provider.Reference) (err error) {
	name := path.Base(ref.Path)
	if name == "" || name == "." || name == "/" {
		return errtypes.BadRequest("Invalid path: " + ref.Path)
	}

	parentRef := &provider.Reference{
		ResourceId: ref.ResourceId,
		Path:       path.Dir(ref.Path),
	}

	// verify parent exists
	var n *node.Node
	if n, err = fs.lu.NodeFromResource(ctx, parentRef); err != nil {
		return
	}
	if !n.Exists {
		return errtypes.NotFound(parentRef.Path)
	}

	ok, err := fs.p.HasPermission(ctx, n, func(rp *provider.ResourcePermissions) bool {
		return rp.CreateContainer
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(filepath.Join(n.ParentID, n.Name))
	}

	// check lock
	if err := n.CheckLock(ctx); err != nil {
		return err
	}

	// verify child does not exist, yet
	if n, err = n.Child(ctx, name); err != nil {
		return
	}
	if n.Exists {
		return errtypes.AlreadyExists(parentRef.Path)
	}

	if err = fs.tp.CreateDir(ctx, n); err != nil {
		return
	}

	if fs.o.TreeTimeAccounting || fs.o.TreeSizeAccounting {
		// mark the home node as the end of propagation
		if err = n.SetMetadata(xattrs.PropagationAttr, "1"); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("node", n).Msg("could not mark node to propagate")

			// FIXME: This does not return an error at all, but results in a severe situation that the
			// part tree is not marked for propagation
			return
		}
	}
	return
}

// TouchFile as defined in the storage.FS interface
func (fs *Decomposedfs) TouchFile(ctx context.Context, ref *provider.Reference) error {
	return fmt.Errorf("unimplemented: TouchFile")
}

// CreateReference creates a reference as a node folder with the target stored in extended attributes
// There is no difference between the /Shares folder and normal nodes because the storage is not supposed to be accessible
// without the storage provider. In effect everything is a shadow namespace.
// To mimic the eos and owncloud driver we only allow references as children of the "/Shares" folder
// FIXME: This comment should explain briefly what a reference is in this context.
func (fs *Decomposedfs) CreateReference(ctx context.Context, p string, targetURI *url.URL) (err error) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(ctx, "CreateReference")
	defer span.End()

	p = strings.Trim(p, "/")
	parts := strings.Split(p, "/")

	if len(parts) != 2 {
		err := errtypes.PermissionDenied("Decomposedfs: references must be a child of the share folder: share_folder=" + fs.o.ShareFolder + " path=" + p)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if parts[0] != strings.Trim(fs.o.ShareFolder, "/") {
		err := errtypes.PermissionDenied("Decomposedfs: cannot create references outside the share folder: share_folder=" + fs.o.ShareFolder + " path=" + p)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// create Shares folder if it does not exist
	var parentNode *node.Node
	var parentCreated, childCreated bool // defaults to false
	if parentNode, err = fs.lu.NodeFromResource(ctx, &provider.Reference{Path: fs.o.ShareFolder}); err != nil {
		err := errtypes.InternalError(err.Error())
		span.SetStatus(codes.Error, err.Error())
		return err
	} else if !parentNode.Exists {
		if err = fs.tp.CreateDir(ctx, parentNode); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		parentCreated = true
	}

	var childNode *node.Node
	// clean up directories created here on error
	defer func() {
		if err != nil {
			// do not catch the error to not shadow the original error
			if childCreated && childNode != nil {
				if tmpErr := fs.tp.Delete(ctx, childNode); tmpErr != nil {
					appctx.GetLogger(ctx).Error().Err(tmpErr).Str("node_id", childNode.ID).Msg("Can not clean up child node after error")
				}
			}
			if parentCreated && parentNode != nil {
				if tmpErr := fs.tp.Delete(ctx, parentNode); tmpErr != nil {
					appctx.GetLogger(ctx).Error().Err(tmpErr).Str("node_id", parentNode.ID).Msg("Can not clean up parent node after error")
				}

			}
		}
	}()

	if childNode, err = parentNode.Child(ctx, parts[1]); err != nil {
		return errtypes.InternalError(err.Error())
	}

	if childNode.Exists {
		// TODO append increasing number to mountpoint name
		err := errtypes.AlreadyExists(p)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if err := fs.tp.CreateDir(ctx, childNode); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	childCreated = true

	if err := n.SetMetadata(xattrs.ReferenceAttr, targetURI.String()); err != nil {
		// the reference could not be set - that would result in an lost reference?
		err := errors.Wrapf(err, "Decomposedfs: error setting the target %s on the reference file %s",
			targetURI.String(),
			n.InternalPath(),
		)
		span.SetStatus(codes.Error, err.Error())
		return err
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

	// check lock on target
	if err := newNode.CheckLock(ctx); err != nil {
		return err
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

	return node.AsResourceInfo(ctx, &rp, mdKeys, utils.IsRelativeReference(ref))
}

// ListFolder returns a list of resources in the specified folder
func (fs *Decomposedfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) (finfos []*provider.ResourceInfo, err error) {
	var n *node.Node
	if n, err = fs.lu.NodeFromResource(ctx, ref); err != nil {
		return
	}

	ctx, span := rtrace.Provider.Tracer("decomposedfs").Start(ctx, "ListFolder")
	defer span.End()

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
		pset := n.PermissionSet(ctx)
		node.AddPermissions(&np, &pset)
		if ri, err := children[i].AsResourceInfo(ctx, &np, mdKeys, utils.IsRelativeReference(ref)); err == nil {
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

	if err := node.CheckLock(ctx); err != nil {
		return err
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

// GetLock returns an existing lock on the given reference
func (fs *Decomposedfs) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
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
	return node.ReadLock(ctx)
}

// SetLock puts a lock on the given reference
func (fs *Decomposedfs) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	node, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: error resolving ref")
	}

	if !node.Exists {
		return errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
	}

	ok, err := fs.p.HasPermission(ctx, node, func(rp *provider.ResourcePermissions) bool {
		return rp.InitiateFileUpload
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(filepath.Join(node.ParentID, node.Name))
	}

	return node.SetLock(ctx, lock)
}

// RefreshLock refreshes an existing lock on the given reference
func (fs *Decomposedfs) RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	if lock.LockId == "" {
		return errtypes.BadRequest("missing lockid")
	}

	node, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: error resolving ref")
	}

	if !node.Exists {
		return errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
	}

	ok, err := fs.p.HasPermission(ctx, node, func(rp *provider.ResourcePermissions) bool {
		return rp.InitiateFileUpload
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(filepath.Join(node.ParentID, node.Name))
	}

	return node.RefreshLock(ctx, lock)
}

// Unlock removes an existing lock from the given reference
func (fs *Decomposedfs) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	if lock.LockId == "" {
		return errtypes.BadRequest("missing lockid")
	}

	node, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "Decomposedfs: error resolving ref")
	}

	if !node.Exists {
		return errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
	}

	ok, err := fs.p.HasPermission(ctx, node, func(rp *provider.ResourcePermissions) bool {
		return rp.InitiateFileUpload // TODO do we need a dedicated permission?
	})
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !ok:
		return errtypes.PermissionDenied(filepath.Join(node.ParentID, node.Name))
	}

	return node.Unlock(ctx, lock)
}
