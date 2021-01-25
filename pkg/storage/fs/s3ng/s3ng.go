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

package s3ng

//go:generate mockery -name PermissionsChecker
//go:generate mockery -name Blobstore

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/blobstore"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/node"
	"github.com/cs3org/reva/pkg/storage/fs/s3ng/xattrs"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

func init() {
	registry.Register("s3ng", NewDefault)
}

func parseConfig(m map[string]interface{}) (*Options, error) {
	o := &Options{}
	if err := mapstructure.Decode(m, o); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return o, nil
}

func (o *Options) init(m map[string]interface{}) {
	if o.UserLayout == "" {
		o.UserLayout = "{{.Id.OpaqueId}}"
	}
	// ensure user layout has no starting or trailing /
	o.UserLayout = strings.Trim(o.UserLayout, "/")

	if o.ShareFolder == "" {
		o.ShareFolder = "/Shares"
	}
	// ensure share folder always starts with slash
	o.ShareFolder = filepath.Join("/", o.ShareFolder)

	// c.DataDirectory should never end in / unless it is the root
	o.Root = filepath.Clean(o.Root)
}

// PermissionsChecker defines an interface for checking permissions on a Node
type PermissionsChecker interface {
	AssemblePermissions(ctx context.Context, n *node.Node) (ap *provider.ResourcePermissions, err error)
	HasPermission(ctx context.Context, n *node.Node, check func(*provider.ResourcePermissions) bool) (can bool, err error)
}

// Blobstore defines an interface for storing blobs in a blobstore
type Blobstore interface {
	Upload(key string, reader io.Reader) error
	Download(key string) (io.ReadCloser, error)
	Delete(key string) error
}

// NewDefault returns an s3ng filestore using the default configuration
func NewDefault(m map[string]interface{}) (storage.FS, error) {
	o, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	lu := &Lookup{}
	p := node.NewPermissions(lu)
	bs, err := blobstore.New(o.S3Endpoint, o.S3Region, o.S3Bucket, o.S3AccessKey, o.S3SecretKey)
	if err != nil {
		return nil, err
	}

	return New(m, lu, p, bs)
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}, lu *Lookup, permissionsChecker PermissionsChecker, blobstore Blobstore) (storage.FS, error) {
	o, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	o.init(m)

	lu.Options = o

	// create data paths for internal layout
	dataPaths := []string{
		filepath.Join(o.Root, "nodes"),
		// notes contain symlinks from nodes/<u-u-i-d>/uploads/<uploadid> to ../../uploads/<uploadid>
		// better to keep uploads on a fast / volatile storage before a workflow finally moves them to the nodes dir
		filepath.Join(o.Root, "uploads"),
		filepath.Join(o.Root, "trash"),
	}
	for _, v := range dataPaths {
		if err := os.MkdirAll(v, 0700); err != nil {
			logger.New().Error().Err(err).
				Str("path", v).
				Msg("could not create data dir")
		}
	}

	// the root node has an empty name
	// the root node has no parent
	n := node.New("root", "", "", 0, nil, lu)
	if err = createNode(
		n,
		&userv1beta1.UserId{
			OpaqueId: o.Owner,
		},
	); err != nil {
		return nil, err
	}

	tp, err := NewTree(lu)
	if err != nil {
		return nil, err
	}

	if !o.S3ConfigComplete() {
		return nil, fmt.Errorf("S3 configuration incomplete")
	}

	return &s3ngfs{
		Blobstore:    blobstore,
		tp:           tp,
		lu:           lu,
		o:            o,
		p:            permissionsChecker,
		chunkHandler: chunking.NewChunkHandler(filepath.Join(o.Root, "uploads")),
	}, nil
}

type s3ngfs struct {
	Blobstore Blobstore

	lu           *Lookup
	tp           TreePersistence
	o            *Options
	p            PermissionsChecker
	chunkHandler *chunking.ChunkHandler
}

func (fs *s3ngfs) Shutdown(ctx context.Context) error {
	return nil
}

func (fs *s3ngfs) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

// CreateHome creates a new root node that has no parent id
func (fs *s3ngfs) CreateHome(ctx context.Context) (err error) {
	if !fs.o.EnableHome || fs.o.UserLayout == "" {
		return errtypes.NotSupported("s3ngfs: CreateHome() home supported disabled")
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
func (fs *s3ngfs) GetHome(ctx context.Context) (string, error) {
	if !fs.o.EnableHome || fs.o.UserLayout == "" {
		return "", errtypes.NotSupported("s3ngfs: GetHome() home supported disabled")
	}
	u := user.ContextMustGetUser(ctx)
	layout := templates.WithUser(u, fs.o.UserLayout)
	return filepath.Join(fs.o.Root, layout), nil // TODO use a namespace?
}

// Tree persistence

// GetPathByID returns the fn pointed by the file id, without the internal namespace
func (fs *s3ngfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	return fs.tp.GetPathByID(ctx, id)
}

func (fs *s3ngfs) CreateDir(ctx context.Context, fn string) (err error) {
	var n *node.Node
	if n, err = fs.lu.NodeFromPath(ctx, fn); err != nil {
		return
	}

	if n.Exists {
		return errtypes.AlreadyExists(fn)
	}
	pn, err := n.Parent()
	if err != nil {
		return errors.Wrap(err, "s3ngfs: error getting parent "+n.ParentID)
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
func (fs *s3ngfs) CreateReference(ctx context.Context, p string, targetURI *url.URL) (err error) {

	p = strings.Trim(p, "/")
	parts := strings.Split(p, "/")

	if len(parts) != 2 {
		return errtypes.PermissionDenied("s3ngfs: references must be a child of the share folder: share_folder=" + fs.o.ShareFolder + " path=" + p)
	}

	if parts[0] != strings.Trim(fs.o.ShareFolder, "/") {
		return errtypes.PermissionDenied("s3ngfs: cannot create references outside the share folder: share_folder=" + fs.o.ShareFolder + " path=" + p)
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

	if n, err = n.Child(parts[1]); err != nil {
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
		return errors.Wrapf(err, "s3ngfs: error setting the target %s on the reference file %s", targetURI.String(), internal)
	}
	return nil
}

func (fs *s3ngfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
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

func (fs *s3ngfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (ri *provider.ResourceInfo, err error) {
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

func (fs *s3ngfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) (finfos []*provider.ResourceInfo, err error) {
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

func (fs *s3ngfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {
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

// Data persistence
func (fs *s3ngfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	node, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "s3ngfs: error resolving ref")
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

	reader, err := fs.Blobstore.Download(node.ID)
	if err != nil {
		return nil, errors.Wrap(err, "s3ngfs: error download blob '"+node.ID+"'")
	}
	return reader, nil
}

// arbitrary metadata persistence in metadata.go

// Version persistence in revisions.go

// Trash persistence in recycle.go

// share persistence in grants.go

func (fs *s3ngfs) copyMD(s string, t string) (err error) {
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
