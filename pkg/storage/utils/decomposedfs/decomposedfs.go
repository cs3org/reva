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

//go:generate make --no-print-directory -C ../../../.. mockery NAME=PermissionsChecker
//go:generate make --no-print-directory -C ../../../.. mockery NAME=CS3PermissionsClient
//go:generate make --no-print-directory -C ../../../.. mockery NAME=Tree

import (
	"context"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"syscall"

	cs3permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/logger"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs"
	"github.com/cs3org/reva/v2/pkg/storage/utils/filelocks"
	"github.com/cs3org/reva/v2/pkg/storage/utils/templates"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// name is the Tracer name used to identify this instrumentation library.
const tracerName = "decomposedfs"

// PermissionsChecker defines an interface for checking permissions on a Node
type PermissionsChecker interface {
	AssemblePermissions(ctx context.Context, n *node.Node) (ap provider.ResourcePermissions, err error)
}

// CS3PermissionsClient defines an interface for checking permissions against the CS3 permissions service
type CS3PermissionsClient interface {
	CheckPermission(ctx context.Context, in *cs3permissions.CheckPermissionRequest, opts ...grpc.CallOption) (*cs3permissions.CheckPermissionResponse, error)
}

// Tree is used to manage a tree hierarchy
type Tree interface {
	Setup() error

	GetMD(ctx context.Context, node *node.Node) (os.FileInfo, error)
	ListFolder(ctx context.Context, node *node.Node) ([]*node.Node, error)
	// CreateHome(owner *userpb.UserId) (n *node.Node, err error)
	CreateDir(ctx context.Context, node *node.Node) (err error)
	TouchFile(ctx context.Context, node *node.Node) error
	// CreateReference(ctx context.Context, node *node.Node, targetURI *url.URL) error
	Move(ctx context.Context, oldNode *node.Node, newNode *node.Node) (err error)
	Delete(ctx context.Context, node *node.Node) (err error)
	RestoreRecycleItemFunc(ctx context.Context, spaceid, key, trashPath string, target *node.Node) (*node.Node, *node.Node, func() error, error)
	PurgeRecycleItemFunc(ctx context.Context, spaceid, key, purgePath string) (*node.Node, func() error, error)

	WriteBlob(node *node.Node, reader io.Reader) error
	ReadBlob(node *node.Node) (io.ReadCloser, error)
	DeleteBlob(node *node.Node) error

	Propagate(ctx context.Context, node *node.Node) (err error)
}

// Decomposedfs provides the base for decomposed filesystem implementations
type Decomposedfs struct {
	lu                *lookup.Lookup
	tp                Tree
	o                 *options.Options
	p                 PermissionsChecker
	chunkHandler      *chunking.ChunkHandler
	permissionsClient CS3PermissionsClient
}

// NewDefault returns an instance with default components
func NewDefault(m map[string]interface{}, bs tree.Blobstore) (storage.FS, error) {
	o, err := options.New(m)
	if err != nil {
		return nil, err
	}

	lu := &lookup.Lookup{}
	p := node.NewPermissions(lu)

	lu.Options = o

	tp := tree.New(o.Root, o.TreeTimeAccounting, o.TreeSizeAccounting, lu, bs)

	permissionsClient, err := pool.GetPermissionsClient(o.PermissionsSVC, pool.WithTLSMode(o.PermTLSMode))
	if err != nil {
		return nil, err
	}

	return New(o, lu, p, tp, permissionsClient)
}

// New returns an implementation of the storage.FS interface that talks to
// a local filesystem.
func New(o *options.Options, lu *lookup.Lookup, p PermissionsChecker, tp Tree, permissionsClient CS3PermissionsClient) (storage.FS, error) {
	err := tp.Setup()
	if err != nil {
		logger.New().Error().Err(err).
			Msg("could not setup tree")
		return nil, errors.Wrap(err, "could not setup tree")
	}

	if o.MaxAcquireLockCycles != 0 {
		filelocks.SetMaxLockCycles(o.MaxAcquireLockCycles)
	}

	return &Decomposedfs{
		tp:                tp,
		lu:                lu,
		o:                 o,
		p:                 p,
		chunkHandler:      chunking.NewChunkHandler(filepath.Join(o.Root, "uploads")),
		permissionsClient: permissionsClient,
	}, nil
}

// Shutdown shuts down the storage
func (fs *Decomposedfs) Shutdown(ctx context.Context) error {
	return nil
}

// GetQuota returns the quota available
// TODO Document in the cs3 should we return quota or free space?
func (fs *Decomposedfs) GetQuota(ctx context.Context, ref *provider.Reference) (total uint64, inUse uint64, remaining uint64, err error) {
	var n *node.Node
	if ref == nil {
		err = errtypes.BadRequest("no space given")
		return 0, 0, 0, err
	}
	if n, err = fs.lu.NodeFromResource(ctx, ref); err != nil {
		return 0, 0, 0, err
	}

	if !n.Exists {
		err = errtypes.NotFound(filepath.Join(n.ParentID, n.Name))
		return 0, 0, 0, err
	}

	rp, err := fs.p.AssemblePermissions(ctx, n)
	switch {
	case err != nil:
		return 0, 0, 0, errtypes.InternalError(err.Error())
	case !rp.GetQuota && !fs.canListAllSpaces(ctx):
		f, _ := storagespace.FormatReference(ref)
		if rp.Stat {
			return 0, 0, 0, errtypes.PermissionDenied(f)
		}
		return 0, 0, 0, errtypes.NotFound(f)
	}

	// FIXME move treesize & quota to fieldmask
	ri, err := n.AsResourceInfo(ctx, &rp, []string{"treesize", "quota"}, []string{}, true)
	if err != nil {
		return 0, 0, 0, err
	}

	quotaStr := node.QuotaUnknown
	if ri.Opaque != nil && ri.Opaque.Map != nil && ri.Opaque.Map["quota"] != nil && ri.Opaque.Map["quota"].Decoder == "plain" {
		quotaStr = string(ri.Opaque.Map["quota"].Value)
	}

	remaining, err = node.GetAvailableSize(n.InternalPath())
	if err != nil {
		return 0, 0, 0, err
	}

	switch quotaStr {
	case node.QuotaUncalculated, node.QuotaUnknown:
		// best we can do is return current total
		// TODO indicate unlimited total? -> in opaque data?
	case node.QuotaUnlimited:
		total = 0
	default:
		total, err = strconv.ParseUint(quotaStr, 10, 64)
		if err != nil {
			return 0, 0, 0, err
		}

		if total <= remaining {
			// Prevent overflowing
			if ri.Size >= total {
				remaining = 0
			} else {
				remaining = total - ri.Size
			}
		}
	}

	return total, ri.Size, remaining, nil
}

// CreateHome creates a new home node for the given user
func (fs *Decomposedfs) CreateHome(ctx context.Context) (err error) {
	if fs.o.UserLayout == "" {
		return errtypes.NotSupported("Decomposedfs: CreateHome() home supported disabled")
	}

	u := ctxpkg.ContextMustGetUser(ctx)
	res, err := fs.CreateStorageSpace(ctx, &provider.CreateStorageSpaceRequest{
		Type:  spaceTypePersonal,
		Owner: u,
	})
	if err != nil {
		return err
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		return errtypes.NewErrtypeFromStatus(res.Status)
	}
	return nil
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

	rp, err := fs.p.AssemblePermissions(ctx, node)
	switch {
	case err != nil:
		return "", errtypes.InternalError(err.Error())
	case !rp.GetPath:
		f := storagespace.FormatResourceID(*id)
		if rp.Stat {
			return "", errtypes.PermissionDenied(f)
		}
		return "", errtypes.NotFound(f)
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
	// TODO check if user has access to root / space
	if !n.Exists {
		return errtypes.PreconditionFailed(parentRef.Path)
	}

	rp, err := fs.p.AssemblePermissions(ctx, n)
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !rp.CreateContainer:
		f, _ := storagespace.FormatReference(ref)
		if rp.Stat {
			return errtypes.PermissionDenied(f)
		}
		return errtypes.NotFound(f)
	}

	// Set space owner in context
	storagespace.ContextSendSpaceOwnerID(ctx, n.SpaceOwnerOrManager(ctx))

	// check lock
	if err := n.CheckLock(ctx); err != nil {
		return err
	}

	// verify child does not exist, yet
	if n, err = n.Child(ctx, name); err != nil {
		return
	}
	if n.Exists {
		return errtypes.AlreadyExists(ref.Path)
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
	parentRef := &provider.Reference{
		ResourceId: ref.ResourceId,
		Path:       path.Dir(ref.Path),
	}

	// verify parent exists
	parent, err := fs.lu.NodeFromResource(ctx, parentRef)
	if err != nil {
		return errtypes.InternalError(err.Error())
	}
	if !parent.Exists {
		return errtypes.NotFound(parentRef.Path)
	}

	n, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return errtypes.InternalError(err.Error())
	}

	rp, err := fs.p.AssemblePermissions(ctx, n)
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !rp.InitiateFileUpload:
		f, _ := storagespace.FormatReference(ref)
		if rp.Stat {
			return errtypes.PermissionDenied(f)
		}
		return errtypes.NotFound(f)
	}

	// Set space owner in context
	storagespace.ContextSendSpaceOwnerID(ctx, n.SpaceOwnerOrManager(ctx))

	// check lock
	if err := n.CheckLock(ctx); err != nil {
		return err
	}
	return fs.tp.TouchFile(ctx, n)
}

// CreateReference creates a reference as a node folder with the target stored in extended attributes
// There is no difference between the /Shares folder and normal nodes because the storage is not supposed to be accessible
// without the storage provider. In effect everything is a shadow namespace.
// To mimic the eos and owncloud driver we only allow references as children of the "/Shares" folder
// FIXME: This comment should explain briefly what a reference is in this context.
func (fs *Decomposedfs) CreateReference(ctx context.Context, p string, targetURI *url.URL) (err error) {
	return errtypes.NotSupported("not implemented")
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

	rp, err := fs.p.AssemblePermissions(ctx, oldNode)
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !rp.Move:
		f, _ := storagespace.FormatReference(oldRef)
		if rp.Stat {
			return errtypes.PermissionDenied(f)
		}
		return errtypes.NotFound(f)
	}

	if newNode, err = fs.lu.NodeFromResource(ctx, newRef); err != nil {
		return
	}
	if newNode.Exists {
		err = errtypes.AlreadyExists(filepath.Join(newNode.ParentID, newNode.Name))
		return
	}

	rp, err = fs.p.AssemblePermissions(ctx, newNode)
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case oldNode.IsDir() && !rp.CreateContainer:
		f, _ := storagespace.FormatReference(newRef)
		if rp.Stat {
			return errtypes.PermissionDenied(f)
		}
		return errtypes.NotFound(f)
	case !oldNode.IsDir() && !rp.InitiateFileUpload:
		f, _ := storagespace.FormatReference(newRef)
		if rp.Stat {
			return errtypes.PermissionDenied(f)
		}
		return errtypes.NotFound(f)
	}

	// Set space owner in context
	storagespace.ContextSendSpaceOwnerID(ctx, newNode.SpaceOwnerOrManager(ctx))

	// check lock on source
	if err := oldNode.CheckLock(ctx); err != nil {
		return err
	}

	// check lock on target
	if err := newNode.CheckLock(ctx); err != nil {
		return err
	}

	return fs.tp.Move(ctx, oldNode, newNode)
}

// GetMD returns the metadata for the specified resource
func (fs *Decomposedfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string, fieldMask []string) (ri *provider.ResourceInfo, err error) {
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
		f, _ := storagespace.FormatReference(ref)
		return nil, errtypes.NotFound(f)
	}

	md, err := node.AsResourceInfo(ctx, &rp, mdKeys, fieldMask, utils.IsRelativeReference(ref))
	if err != nil {
		return nil, err
	}

	addSpace := len(fieldMask) == 0
	for _, p := range fieldMask {
		if p == "space" || p == "*" {
			addSpace = true
			break
		}
	}
	if addSpace {
		if md.Space, err = fs.storageSpaceFromNode(ctx, node, true); err != nil {
			return nil, err
		}
	}

	return md, nil
}

// ListFolder returns a list of resources in the specified folder
func (fs *Decomposedfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string, fieldMask []string) (finfos []*provider.ResourceInfo, err error) {
	var n *node.Node
	if n, err = fs.lu.NodeFromResource(ctx, ref); err != nil {
		return
	}

	ctx, span := appctx.GetTracerProvider(ctx).Tracer(tracerName).Start(ctx, "ListFolder")
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
		f, _ := storagespace.FormatReference(ref)
		if rp.Stat {
			return nil, errtypes.PermissionDenied(f)
		}
		return nil, errtypes.NotFound(f)
	}

	var children []*node.Node
	children, err = fs.tp.ListFolder(ctx, n)
	if err != nil {
		return
	}

	for i := range children {
		np := rp
		// add this childs permissions
		pset, _ := n.PermissionSet(ctx)
		node.AddPermissions(&np, &pset)
		if ri, err := children[i].AsResourceInfo(ctx, &np, mdKeys, fieldMask, utils.IsRelativeReference(ref)); err == nil {
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
		return errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
	}

	rp, err := fs.p.AssemblePermissions(ctx, node)
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !rp.Delete:
		f, _ := storagespace.FormatReference(ref)
		if rp.Stat {
			return errtypes.PermissionDenied(f)
		}
		return errtypes.NotFound(f)
	}

	// Set space owner in context
	storagespace.ContextSendSpaceOwnerID(ctx, node.SpaceOwnerOrManager(ctx))

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

	rp, err := fs.p.AssemblePermissions(ctx, node)
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !rp.InitiateFileDownload:
		f, _ := storagespace.FormatReference(ref)
		if rp.Stat {
			return nil, errtypes.PermissionDenied(f)
		}
		return nil, errtypes.NotFound(f)
	}

	reader, err := fs.tp.ReadBlob(node)
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

	rp, err := fs.p.AssemblePermissions(ctx, node)
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !rp.InitiateFileDownload:
		f, _ := storagespace.FormatReference(ref)
		if rp.Stat {
			return nil, errtypes.PermissionDenied(f)
		}
		return nil, errtypes.NotFound(f)
	}

	return node.ReadLock(ctx, false)
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

	rp, err := fs.p.AssemblePermissions(ctx, node)
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !rp.InitiateFileUpload:
		f, _ := storagespace.FormatReference(ref)
		if rp.Stat {
			return errtypes.PermissionDenied(f)
		}
		return errtypes.NotFound(f)
	}

	return node.SetLock(ctx, lock)
}

// RefreshLock refreshes an existing lock on the given reference
func (fs *Decomposedfs) RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock, existingLockID string) error {
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

	rp, err := fs.p.AssemblePermissions(ctx, node)
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !rp.InitiateFileUpload:
		f, _ := storagespace.FormatReference(ref)
		if rp.Stat {
			return errtypes.PermissionDenied(f)
		}
		return errtypes.NotFound(f)
	}

	return node.RefreshLock(ctx, lock, existingLockID)
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

	rp, err := fs.p.AssemblePermissions(ctx, node)
	switch {
	case err != nil:
		return errtypes.InternalError(err.Error())
	case !rp.InitiateFileUpload: // TODO do we need a dedicated permission?
		f, _ := storagespace.FormatReference(ref)
		if rp.Stat {
			return errtypes.PermissionDenied(f)
		}
		return errtypes.NotFound(f)
	}

	return node.Unlock(ctx, lock)
}
