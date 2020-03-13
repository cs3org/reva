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
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/gofrs/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

func init() {
	registry.Register("ocis", New)
}

type config struct {
	// ocis fs works on top of a dir of uuid nodes
	DataDirectory string `mapstructure:"data_directory"`

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
		c.UserLayout = "{{.Username}}"
	}
	// c.DataDirectory should never end in / unless it is the root
	c.DataDirectory = path.Clean(c.DataDirectory)

	// TODO we need a lot more mimetypes
	mime.RegisterMime(".txt", "text/plain")
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
		path.Join(c.DataDirectory, "users"),
		path.Join(c.DataDirectory, "nodes"),
		path.Join(c.DataDirectory, "trash/files"),
		path.Join(c.DataDirectory, "trash/info"),
	}
	for _, v := range dataPaths {
		if err := os.MkdirAll(v, 0700); err != nil {
			logger.New().Error().Err(err).
				Str("path", v).
				Msg("could not create data dir")
		}
	}

	pw := &Path{
		DataDirectory: c.DataDirectory,
		EnableHome:    c.EnableHome,
		UserLayout:    c.UserLayout,
	}

	tp, err := NewTree(pw, c.DataDirectory)
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

// Home discovery

func (fs *ocisfs) CreateHome(ctx context.Context) error {
	if !fs.conf.EnableHome || fs.conf.UserLayout == "" {
		return errtypes.NotSupported("ocisfs: create home not supported")
	}

	u, err := getUser(ctx)
	if err != nil {
		err = errors.Wrap(err, "ocisfs: wrap: no user in ctx and home is enabled")
		return err
	}
	layout := templates.WithUser(u, fs.conf.UserLayout)
	home := path.Join(fs.conf.DataDirectory, "users", layout)

	_, err = os.Stat(home)
	if err == nil { // home already exists
		return nil
	}

	// create the users dir
	parent := path.Dir(home)
	err = os.MkdirAll(parent, 0700)
	if err != nil {
		// MkdirAll will return success on mkdir over an existing directory.
		return errors.Wrap(err, "ocisfs: error creating dir")
	}

	// create a directory node (with children subfolder)
	nodeID := uuid.Must(uuid.NewV4()).String()
	err = os.MkdirAll(path.Join(fs.conf.DataDirectory, "nodes", nodeID, "children"), 0700)
	if err != nil {
		return errors.Wrap(err, "ocisfs: error node dir")
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
	u, err := getUser(ctx)
	if err != nil {
		err = errors.Wrap(err, "ocisfs: wrap: no user in ctx and home is enabled")
		return "", err
	}
	layout := templates.WithUser(u, fs.conf.UserLayout)
	return path.Join(fs.conf.DataDirectory, layout), nil // TODO use a namespace?
}

// Tree persistence

// GetPathByID returns the fn pointed by the file id, without the internal namespace
func (fs *ocisfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	return fs.tp.GetPathByID(ctx, id)
}

func (fs *ocisfs) CreateDir(ctx context.Context, fn string) (err error) {
	parent := path.Dir(fn)
	var in string
	if in, err = fs.pw.Wrap(ctx, parent); err != nil {
		return
	}
	return fs.tp.CreateDir(ctx, in, path.Base(fn))
}

func (fs *ocisfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return fs.tp.CreateReference(ctx, path, targetURI)
}

func (fs *ocisfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
	var oldInternal, newInternal string
	if oldInternal, err = fs.pw.Resolve(ctx, oldRef); err != nil {
		return
	}

	if newInternal, err = fs.pw.Resolve(ctx, newRef); err != nil {
		// TODO might not exist ...
		return
	}
	return fs.tp.Move(ctx, oldInternal, newInternal)
}

func (fs *ocisfs) GetMD(ctx context.Context, ref *provider.Reference) (ri *provider.ResourceInfo, err error) {
	var in string
	if in, err = fs.pw.Resolve(ctx, ref); err != nil {
		return
	}
	var md os.FileInfo
	md, err = fs.tp.GetMD(ctx, in)
	if err != nil {
		return nil, err
	}
	return fs.normalize(ctx, md, in)
}

func (fs *ocisfs) ListFolder(ctx context.Context, ref *provider.Reference) (finfos []*provider.ResourceInfo, err error) {
	var in string
	if in, err = fs.pw.Resolve(ctx, ref); err != nil {
		return
	}
	var mds []os.FileInfo
	mds, err = fs.tp.ListFolder(ctx, in)
	if err != nil {
		return
	}

	for _, md := range mds {
		var ri *provider.ResourceInfo
		ri, err = fs.normalize(ctx, md, path.Join(in, "children", md.Name()))
		if err != nil {
			return
		}
		finfos = append(finfos, ri)
	}
	return
}

func (fs *ocisfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {
	var in string
	if in, err = fs.pw.Resolve(ctx, ref); err != nil {
		return
	}
	return fs.tp.Delete(ctx, in)
}

// arbitrary metadata persistence

func (fs *ocisfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	return errtypes.NotSupported("operation not supported: SetArbitraryMetadata")
}

func (fs *ocisfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	return errtypes.NotSupported("operation not supported: UnsetArbitraryMetadata")
}

// Data persistence

func (fs *ocisfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	var in string // the internal path of the file node

	p := ref.GetPath()
	if p != "" {
		if p == "/" {
			return fmt.Errorf("cannot upload into folder node /")
		}
		parent := path.Dir(p)
		name := path.Base(p)

		inp, err := fs.pw.Wrap(ctx, parent)

		if _, err := os.Stat(path.Join(inp, "children")); os.IsNotExist(err) {
			// TODO double check if node is a file
			return fmt.Errorf("cannot upload into folder node " + path.Join(inp, "children"))
		}
		childEntry := path.Join(inp, "children", name)

		// try to determine nodeID by reading link
		link, err := os.Readlink(childEntry)
		if os.IsNotExist(err) {
			// create a new file node
			nodeID := uuid.Must(uuid.NewV4()).String()

			in = path.Join(fs.conf.DataDirectory, "nodes", nodeID)

			err = os.MkdirAll(in, 0700)
			if err != nil {
				return errors.Wrap(err, "ocisfs: could not create node dir")
			}
			// create back link
			// we are not only linking back to the parent, but also to the filename
			link = "../" + path.Base(inp) + "/children/" + name
			err = os.Symlink(link, path.Join(in, "parentname"))
			if err != nil {
				return errors.Wrap(err, "ocisfs: could not symlink parent node")
			}

			// link child name to node
			err = os.Symlink("../../"+nodeID, path.Join(inp, "children", name))
			if err != nil {
				return errors.Wrap(err, "ocisfs: could not symlink child entry")
			}
		} else {
			// the nodeID is in the link
			// TODO check if link has correct beginning?
			nodeID := path.Base(link)
			in = path.Join(fs.conf.DataDirectory, "nodes", nodeID)
		}
	} else if ref.GetId() != nil {
		var err error
		if in, err = fs.pw.WrapID(ctx, ref.GetId()); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("invalid reference %+v", ref)
	}

	tmp, err := ioutil.TempFile(in, "._reva_atomic_upload")
	if err != nil {
		return errors.Wrap(err, "ocisfs: error creating tmp fn at "+in)
	}

	_, err = io.Copy(tmp, r)
	if err != nil {
		return errors.Wrap(err, "ocisfs: error writing to tmp file "+tmp.Name())
	}

	// TODO move old content to version
	_ = os.RemoveAll(path.Join(in, "content"))

	err = os.Rename(tmp.Name(), path.Join(in, "content"))
	if err != nil {
		return err
	}
	return fs.tp.Propagate(ctx, in)
}

func (fs *ocisfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	in, err := fs.pw.Resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocisfs: error resolving ref")
	}

	contentPath := path.Join(in, "content")

	r, err := os.Open(contentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(contentPath)
		}
		return nil, errors.Wrap(err, "ocisfs: error reading "+contentPath)
	}
	return r, nil
}

// Version persistence

func (fs *ocisfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	return nil, errtypes.NotSupported("operation not supported: ListRevisions")
}
func (fs *ocisfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("operation not supported: DownloadRevision")
}

func (fs *ocisfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	return errtypes.NotSupported("operation not supported: RestoreRevision")
}

// Trash persistence

func (fs *ocisfs) PurgeRecycleItem(ctx context.Context, key string) error {
	return errtypes.NotSupported("operation not supported: PurgeRecycleItem")
}

func (fs *ocisfs) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("operation not supported: EmptyRecycle")
}

func (fs *ocisfs) ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("operation not supported: ListRecycle")
}

func (fs *ocisfs) RestoreRecycleItem(ctx context.Context, key string) error {
	return errtypes.NotSupported("operation not supported: RestoreRecycleItem")
}

// share persistence

func (fs *ocisfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("operation not supported: AddGrant")
}

func (fs *ocisfs) ListGrants(ctx context.Context, ref *provider.Reference) (grants []*provider.Grant, err error) {
	return nil, errtypes.NotSupported("operation not supported: ListGrants")
}

func (fs *ocisfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	return errtypes.NotSupported("operation not supported: RemoveGrant")
}

func (fs *ocisfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("operation not supported: UpdateGrant")
}

// supporting functions

func getUser(ctx context.Context) (*userpb.User, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired(""), "ocisfs: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

func (fs *ocisfs) normalize(ctx context.Context, fi os.FileInfo, internal string) (ri *provider.ResourceInfo, err error) {
	var fn string

	fn, err = fs.pw.Unwrap(ctx, path.Join("/", internal))
	if err != nil {
		return nil, err
	}
	// TODO GetMD should return the correct fileinfo
	nodeType := provider.ResourceType_RESOURCE_TYPE_INVALID
	if fi, err = os.Stat(path.Join(internal, "content")); err == nil {
		nodeType = provider.ResourceType_RESOURCE_TYPE_FILE
	} else if fi, err = os.Stat(path.Join(internal, "children")); err == nil {
		nodeType = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	} else if fi, err = os.Stat(path.Join(internal, "reference")); err == nil {
		// TODO handle references
		nodeType = provider.ResourceType_RESOURCE_TYPE_REFERENCE
	}

	var etag []byte
	if etag, err = xattr.Get(internal, "user.ocis.etag"); err != nil {
		logger.New().Error().Err(err).Msg("could not read etag")
	}
	ri = &provider.ResourceInfo{
		Id:       &provider.ResourceId{OpaqueId: path.Base(internal)},
		Path:     fn,
		Type:     nodeType,
		Etag:     string(etag),
		MimeType: mime.Detect(nodeType == provider.ResourceType_RESOURCE_TYPE_CONTAINER, fn),
		Size:     uint64(fi.Size()),
		// TODO fix permissions
		PermissionSet: &provider.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Mtime: &types.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
		},
	}

	logger.New().Debug().
		Interface("ri", ri).
		Msg("normalized")

	return ri, nil
}
