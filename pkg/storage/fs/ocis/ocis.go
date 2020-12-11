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
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

const (
	// TODO the below comment is currently copied from the owncloud driver, revisit
	// Currently,extended file attributes have four separated
	// namespaces (user, trusted, security and system) followed by a dot.
	// A non root user can only manipulate the user. namespace, which is what
	// we will use to store ownCloud specific metadata. To prevent name
	// collisions with other apps We are going to introduce a sub namespace
	// "user.ocis."

	ocisPrefix   string = "user.ocis."
	parentidAttr string = ocisPrefix + "parentid"
	ownerIDAttr  string = ocisPrefix + "owner.id"
	ownerIDPAttr string = ocisPrefix + "owner.idp"
	// the base name of the node
	// updated when the file is renamed or moved
	nameAttr string = ocisPrefix + "name"

	// grantPrefix is the prefix for sharing related extended attributes
	grantPrefix    string = ocisPrefix + "grant."
	metadataPrefix string = ocisPrefix + "md."
	// TODO implement favorites metadata flag
	//favPrefix   string = ocisPrefix + "fav."  // favorite flag, per user

	// a temporary etag for a folder that is removed when the mtime propagation happens
	tmpEtagAttr   string = ocisPrefix + "tmp.etag"
	referenceAttr string = ocisPrefix + "cs3.ref" // arbitrary metadata
	//checksumPrefix    string = ocisPrefix + "cs."   // TODO add checksum support
	trashOriginAttr string = ocisPrefix + "trash.origin" // trash origin

	// we use a single attribute to enable or disable propagation of both: synctime and treesize
	propagationAttr string = ocisPrefix + "propagation"

	// the tree modification time of the tree below this node,
	// propagated when synctime_accounting is true and
	// user.ocis.propagation=1 is set
	// stored as a readable time.RFC3339Nano
	treeMTimeAttr string = ocisPrefix + "tmtime"

	// the size of the tree below this node,
	// propagated when treesize_accounting is true and
	// user.ocis.propagation=1 is set
	//treesizeAttr string = ocisPrefix + "treesize"
)

func init() {
	registry.Register("ocis", New)
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

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (storage.FS, error) {
	o, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	o.init(m)

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

	lu := &Lookup{
		Options: o,
	}

	// the root node has an empty name
	// the root node has no parent
	if err = createNode(
		&Node{lu: lu, ID: "root"},
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

	return &ocisfs{
		tp:           tp,
		lu:           lu,
		o:            o,
		p:            &Permissions{lu: lu},
		chunkHandler: chunking.NewChunkHandler(filepath.Join(o.Root, "uploads")),
	}, nil
}

type ocisfs struct {
	tp           TreePersistence
	lu           *Lookup
	o            *Options
	p            *Permissions
	chunkHandler *chunking.ChunkHandler
}

func (fs *ocisfs) Shutdown(ctx context.Context) error {
	return nil
}

func (fs *ocisfs) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

// CreateHome creates a new root node that has no parent id
func (fs *ocisfs) CreateHome(ctx context.Context) (err error) {
	if !fs.o.EnableHome || fs.o.UserLayout == "" {
		return errtypes.NotSupported("ocisfs: CreateHome() home supported disabled")
	}

	var n, h *Node
	if n, err = fs.lu.RootNode(ctx); err != nil {
		return
	}
	h, err = fs.lu.WalkPath(ctx, n, fs.lu.mustGetUserLayout(ctx), func(ctx context.Context, n *Node) error {
		if !n.Exists {
			if err := fs.tp.CreateDir(ctx, n); err != nil {
				return err
			}
		}
		return nil
	})

	if fs.o.TreeTimeAccounting {
		homePath := h.lu.toInternalPath(h.ID)
		// mark the home node as the end of propagation
		if err = xattr.Set(homePath, propagationAttr, []byte("1")); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("node", h).Msg("could not mark home as propagation root")
			return
		}
	}
	return
}

// GetHome is called to look up the home path for a user
// It is NOT supposed to return the internal path but the external path
func (fs *ocisfs) GetHome(ctx context.Context) (string, error) {
	if !fs.o.EnableHome || fs.o.UserLayout == "" {
		return "", errtypes.NotSupported("ocisfs: GetHome() home supported disabled")
	}
	u := user.ContextMustGetUser(ctx)
	layout := templates.WithUser(u, fs.o.UserLayout)
	return filepath.Join(fs.o.Root, layout), nil // TODO use a namespace?
}

// Tree persistence

// GetPathByID returns the fn pointed by the file id, without the internal namespace
func (fs *ocisfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	return fs.tp.GetPathByID(ctx, id)
}

func (fs *ocisfs) CreateDir(ctx context.Context, fn string) (err error) {
	var n *Node
	if n, err = fs.lu.NodeFromPath(ctx, fn); err != nil {
		return
	}

	if n.Exists {
		return errtypes.AlreadyExists(fn)
	}

	pn, err := n.Parent()
	if err != nil {
		return errors.Wrap(err, "ocisfs: error getting parent "+n.ParentID)
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
		nodePath := n.lu.toInternalPath(n.ID)
		// mark the home node as the end of propagation
		if err = xattr.Set(nodePath, propagationAttr, []byte("1")); err != nil {
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
func (fs *ocisfs) CreateReference(ctx context.Context, p string, targetURI *url.URL) (err error) {

	p = strings.Trim(p, "/")
	parts := strings.Split(p, "/")

	if len(parts) != 2 {
		return errtypes.PermissionDenied("ocisfs: references must be a child of the share folder: share_folder=" + fs.o.ShareFolder + " path=" + p)
	}

	if parts[0] != strings.Trim(fs.o.ShareFolder, "/") {
		return errtypes.PermissionDenied("ocisfs: cannot create references outside the share folder: share_folder=" + fs.o.ShareFolder + " path=" + p)
	}

	// create Shares folder if it does not exist
	var n *Node
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

	internal := n.lu.toInternalPath(n.ID)
	if err = xattr.Set(internal, referenceAttr, []byte(targetURI.String())); err != nil {
		return errors.Wrapf(err, "ocisfs: error setting the target %s on the reference file %s", targetURI.String(), internal)
	}
	return nil
}

func (fs *ocisfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
	var oldNode, newNode *Node
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

func (fs *ocisfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (ri *provider.ResourceInfo, err error) {
	var node *Node
	if node, err = fs.lu.NodeFromResource(ctx, ref); err != nil {
		return
	}

	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}
	rp, err := fs.p.ReadUserPermissions(ctx, node)
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !rp.Stat:
		return nil, errtypes.PermissionDenied(node.ID)
	}

	return node.AsResourceInfo(ctx, rp, mdKeys)
}

func (fs *ocisfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) (finfos []*provider.ResourceInfo, err error) {
	var node *Node
	if node, err = fs.lu.NodeFromResource(ctx, ref); err != nil {
		return
	}

	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}

	rp, err := fs.p.ReadUserPermissions(ctx, node)
	switch {
	case err != nil:
		return nil, errtypes.InternalError(err.Error())
	case !rp.ListContainer:
		return nil, errtypes.PermissionDenied(node.ID)
	}

	var children []*Node
	children, err = fs.tp.ListFolder(ctx, node)
	if err != nil {
		return
	}

	for i := range children {
		np := rp
		addPermissions(np, node.PermissionSet(ctx))
		// TODO only add this childs permissions
		if ri, err := children[i].AsResourceInfo(ctx, np, mdKeys); err == nil {
			finfos = append(finfos, ri)
		}
	}
	return
}

func (fs *ocisfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {
	var node *Node
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

func (fs *ocisfs) ContentPath(n *Node) string {
	return n.lu.toInternalPath(n.ID)
}

func (fs *ocisfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	node, err := fs.lu.NodeFromResource(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocisfs: error resolving ref")
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

	contentPath := fs.ContentPath(node)

	r, err := os.Open(contentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(contentPath)
		}
		return nil, errors.Wrap(err, "ocisfs: error reading "+contentPath)
	}
	return r, nil
}

// arbitrary metadata persistence in metadata.go

// Version persistence in revisions.go

// Trash persistence in recycle.go

// share persistence in grants.go

func (fs *ocisfs) copyMD(s string, t string) (err error) {
	var attrs []string
	if attrs, err = xattr.List(s); err != nil {
		return err
	}
	for i := range attrs {
		if strings.HasPrefix(attrs[i], ocisPrefix) {
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
