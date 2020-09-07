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

	// shares are persisted using extended attributes. We are going to mimic
	// NFS4 ACLs, with one extended attribute per share, following Access
	// Control Entries (ACEs). The following is taken from the nfs4_acl man page,
	// see https://linux.die.net/man/5/nfs4_acl:
	// the extended attributes will look like this
	// "user.oc.acl.<type>:<flags>:<principal>:<permissions>"
	// - *type* will be limited to A for now
	//     A: Allow - allow *principal* to perform actions requiring *permissions*
	//   In the future we can use:
	//     U: aUdit - log any attempted access by principal which requires
	//                permissions.
	//     L: aLarm - generate a system alarm at any attempted access by
	//                principal which requires permissions
	//   D for deny is not recommended
	// - *flags* for now empty or g for group, no inheritance yet
	//   - d directory-inherit - newly-created subdirectories will inherit the
	//                           ACE.
	//   - f file-inherit - newly-created files will inherit the ACE, minus its
	//                      inheritance flags. Newly-created subdirectories
	//                      will inherit the ACE; if directory-inherit is not
	//                      also specified in the parent ACE, inherit-only will
	//                      be added to the inherited ACE.
	//   - n no-propagate-inherit - newly-created subdirectories will inherit
	//                              the ACE, minus its inheritance flags.
	//   - i inherit-only - the ACE is not considered in permissions checks,
	//                      but it is heritable; however, the inherit-only
	//                      flag is stripped from inherited ACEs.
	// - *principal* a named user, group or special principal
	//   - the oidc sub@iss maps nicely to this
	//   - 'OWNER@', 'GROUP@', and 'EVERYONE@', which are, respectively, analogous to the POSIX user/group/other
	// - *permissions*
	//   - r read-data (files) / list-directory (directories)
	//   - w write-data (files) / create-file (directories)
	//   - a append-data (files) / create-subdirectory (directories)
	//   - x execute (files) / change-directory (directories)
	//   - d delete - delete the file/directory. Some servers will allow a delete to occur if either this permission is set in the file/directory or if the delete-child permission is set in its parent directory.
	//   - D delete-child - remove a file or subdirectory from within the given directory (directories only)
	//   - t read-attributes - read the attributes of the file/directory.
	//   - T write-attributes - write the attributes of the file/directory.
	//   - n read-named-attributes - read the named attributes of the file/directory.
	//   - N write-named-attributes - write the named attributes of the file/directory.
	//   - c read-ACL - read the file/directory NFSv4 ACL.
	//   - C write-ACL - write the file/directory NFSv4 ACL.
	//   - o write-owner - change ownership of the file/directory.
	//   - y synchronize - allow clients to use synchronous I/O with the server.
	// TODO implement OWNER@ as "user.oc.acl.A::OWNER@:rwaDxtTnNcCy"
	// attribute names are limited to 255 chars by the linux kernel vfs, values to 64 kb
	// ext3 extended attributes must fit inside a single filesystem block ... 4096 bytes
	// that leaves us with "user.oc.acl.A::someonewithaslightlylongersubject@whateverissuer:rwaDxtTnNcCy" ~80 chars
	// 4096/80 = 51 shares ... with luck we might move the actual permissions to the value, saving ~15 chars
	// 4096/64 = 64 shares ... still meh ... we can do better by using ints instead of strings for principals
	//   "user.ocis.acl.u:100000" is pretty neat, but we can still do better: base64 encode the int
	//   "user.ocis.acl.u:6Jqg" but base64 always has at least 4 chars, maybe hex is better for smaller numbers
	//   well use 4 chars in addition to the ace: "user.oc.acl.u:////" = 65535 -> 18 chars
	// 4096/18 = 227 shares
	// still ... ext attrs for this are not infinite scale ...
	// so .. attach shares via fileid.
	// <userhome>/metadata/<fileid>/shares, similar to <userhome>/files
	// <userhome>/metadata/<fileid>/shares/u/<issuer>/<subject>/A:fdi:rwaDxtTnNcCy permissions as filename to keep them in the stat cache?
	//
	// whatever ... 50 shares is good enough. If more is needed we can delegate to the metadata
	// if "user.oc.acl.M" is present look inside the metadata app.
	// - if we cannot set an ace we might get an io error.
	//   in that case convert all shares to metadata and try to set "user.oc.acl.m"
	//
	// what about metadata like share creator, share time, expiry?
	// - creator is same as owner, but can be set
	// - share date, or abbreviated st is a unix timestamp
	// - expiry is a unix timestamp
	// - can be put inside the value
	// - we need to reorder the fields:
	// "user.oc.acl.<u|g|o>:<principal>" -> "kv:t=<type>:f=<flags>:p=<permissions>:st=<share time>:c=<creator>:e=<expiry>:pw=<password>:n=<name>"
	// "user.oc.acl.<u|g|o>:<principal>" -> "v1:<type>:<flags>:<permissions>:<share time>:<creator>:<expiry>:<password>:<name>"
	// or the first byte determines the format
	// 0x00 = key value
	// 0x01 = v1 ...
	//
	// SharePrefix is the prefix for sharing related extended attributes
	sharePrefix string = "user.ocis.acl."
	favPrefix   string = "user.ocis.fav."  // favorite flag, per user
	etagPrefix  string = "user.ocis.etag." // allow overriding a calculated etag with one from the extended attributes
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

	fs.tp.CreateRoot(nodeID, u.Id)

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
