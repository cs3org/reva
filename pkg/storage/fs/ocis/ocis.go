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

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const (
	// TODO the below comment is currently copied from the owncloud driver, revisit
	// Currently,extended file attributes have four separated
	// namespaces (user, trusted, security and system) followed by a dot.
	// A non root user can only manipulate the user. namespace, which is what
	// we will use to store ownCloud specific metadata. To prevent name
	// collisions with other apps We are going to introduce a sub namespace
	// "user.ocis."

	// SharePrefix is the prefix for sharing related extended attributes
	sharePrefix string = "user.ocis.acl."
	// TODO implement favorites metadata flag
	//favPrefix   string = "user.ocis.fav."  // favorite flag, per user
	// TODO use etag prefix instead of single etag property
	//etagPrefix  string = "user.ocis.etag." // allow overriding a calculated etag with one from the extended attributes
	//checksumPrefix    string = "user.ocis.cs."   // TODO add checksum support
)

func init() {
	registry.Register("ocis", New)
}

type config struct {
	// ocis fs works on top of a dir of uuid nodes
	Root string `mapstructure:"root"`

	// UserLayout wraps the internal path with user information.
	// Example: if conf.Namespace is /ocis/user and received path is /docs
	// and the UserLayout is {{.Username}} the internal path will be:
	// /ocis/user/<username>/docs
	UserLayout string `mapstructure:"user_layout"`

	// EnableHome enables the creation of home directories.
	EnableHome bool `mapstructure:"enable_home"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

func (c *config) init(m map[string]interface{}) {
	if c.UserLayout == "" {
		c.UserLayout = "{{.Id.OpaqueId}}"
	}
	// c.DataDirectory should never end in / unless it is the root
	c.Root = filepath.Clean(c.Root)
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init(m)

	dataPaths := []string{
		filepath.Join(c.Root, "users"),
		filepath.Join(c.Root, "nodes"),
		// notes contain symlinks from nodes/<u-u-i-d>/uploads/<uploadid> to ../../uploads/<uploadid>
		// better to keep uploads on a fast / volatile storage before a workflow finally moves them to the nodes dir
		filepath.Join(c.Root, "uploads"),
		filepath.Join(c.Root, "trash"),
	}
	for _, v := range dataPaths {
		if err := os.MkdirAll(v, 0700); err != nil {
			logger.New().Error().Err(err).
				Str("path", v).
				Msg("could not create data dir")
		}
	}

	pw := &Path{
		root:       c.Root,
		EnableHome: c.EnableHome,
		UserLayout: c.UserLayout,
	}

	tp, err := NewTree(pw, c.Root)
	if err != nil {
		return nil, err
	}

	return &ocisfs{
		conf: c,
		tp:   tp,
		pw:   pw,
	}, nil
}

type ocisfs struct {
	conf *config
	tp   TreePersistence
	pw   PathWrapper
}

func (fs *ocisfs) Shutdown(ctx context.Context) error {
	return nil
}

func (fs *ocisfs) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

// CreateHome creates a new root node that has no parent id
func (fs *ocisfs) CreateHome(ctx context.Context) error {
	if !fs.conf.EnableHome || fs.conf.UserLayout == "" {
		return errtypes.NotSupported("ocisfs: create home not supported")
	}

	u := user.ContextMustGetUser(ctx)
	layout := templates.WithUser(u, fs.conf.UserLayout)
	home := filepath.Join(fs.conf.Root, "users", layout)

	_, err := os.Stat(home)
	if err == nil { // home already exists
		return nil
	}

	// create the users dir
	parent := filepath.Dir(home)
	err = os.MkdirAll(parent, 0700)
	if err != nil {
		// MkdirAll will return success on mkdir over an existing directory.
		return errors.Wrap(err, "ocisfs: error creating dir")
	}

	// create a directory node
	nodeID := uuid.New().String()

	if _, err = fs.tp.CreateRoot(nodeID, u.Id); err != nil {
		return err
	}

	// link users home to node
	return os.Symlink("../nodes/"+nodeID, home)
}

// GetHome is called to look up the home path for a user
// It is NOT supposed to return the internal path but the external path
func (fs *ocisfs) GetHome(ctx context.Context) (string, error) {
	if !fs.conf.EnableHome || fs.conf.UserLayout == "" {
		return "", errtypes.NotSupported("ocisfs: get home not supported")
	}
	u := user.ContextMustGetUser(ctx)
	layout := templates.WithUser(u, fs.conf.UserLayout)
	return filepath.Join(fs.conf.Root, layout), nil // TODO use a namespace?
}

// Tree persistence

// GetPathByID returns the fn pointed by the file id, without the internal namespace
func (fs *ocisfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	return fs.tp.GetPathByID(ctx, id)
}

func (fs *ocisfs) CreateDir(ctx context.Context, fn string) (err error) {
	var node *Node
	if node, err = fs.pw.NodeFromPath(ctx, fn); err != nil {
		return
	}
	return fs.tp.CreateDir(ctx, node)
}

func (fs *ocisfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return fs.tp.CreateReference(ctx, path, targetURI)
}

func (fs *ocisfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
	var oldNode, newNode *Node
	if oldNode, err = fs.pw.NodeFromResource(ctx, oldRef); err != nil {
		return
	}
	if !oldNode.Exists {
		err = errtypes.NotFound(filepath.Join(oldNode.ParentID, oldNode.Name))
		return
	}

	if newNode, err = fs.pw.NodeFromResource(ctx, newRef); err != nil {
		return
	}
	return fs.tp.Move(ctx, oldNode, newNode)
}

func (fs *ocisfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (ri *provider.ResourceInfo, err error) {
	var node *Node
	if node, err = fs.pw.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}
	return node.AsResourceInfo(ctx)
}

func (fs *ocisfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) (finfos []*provider.ResourceInfo, err error) {
	var node *Node
	if node, err = fs.pw.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}
	var children []*Node
	children, err = fs.tp.ListFolder(ctx, node)
	if err != nil {
		return
	}

	for i := range children {
		if ri, err := children[i].AsResourceInfo(ctx); err == nil {
			finfos = append(finfos, ri)
		}
	}
	return
}

func (fs *ocisfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {
	var node *Node
	if node, err = fs.pw.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}
	return fs.tp.Delete(ctx, node)
}

// Data persistence

func (fs *ocisfs) ContentPath(node *Node) string {
	return filepath.Join(fs.conf.Root, "nodes", node.ID)
}

func (fs *ocisfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	node, err := fs.pw.NodeFromResource(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocisfs: error resolving ref")
	}

	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return nil, err
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
