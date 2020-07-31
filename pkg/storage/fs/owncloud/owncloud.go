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

package owncloud

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/gofrs/uuid"
	"github.com/gomodule/redigo/redis"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

const (
	// Currently,extended file attributes have four separated
	// namespaces (user, trusted, security and system) followed by a dot.
	// A non root user can only manipulate the user. namespace, which is what
	// we will use to store ownCloud specific metadata. To prevent name
	// collisions with other apps We are going to introduce a sub namespace
	// "user.oc."

	// idAttribute is the name of the filesystem extended attribute that is used to store the uuid in
	idAttribute string = "user.oc.id"

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
	//   "user.oc.acl.u:100000" is pretty neat, but we can still do better: base64 encode the int
	//   "user.oc.acl.u:6Jqg" but base64 always has at least 4 chars, maybe hex is better for smaller numbers
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
	sharePrefix       string = "user.oc.acl."
	trashOriginPrefix string = "user.oc.o"
	mdPrefix          string = "user.oc.md."   // arbitrary metadata
	favPrefix         string = "user.oc.fav."  // favorite flag, per user
	etagPrefix        string = "user.oc.etag." // allow overriding a calculated etag with one from the extended attributes
	//checksumPrefix    string = "user.oc.cs."   // TODO add checksum support
)

func init() {
	registry.Register("owncloud", New)
}

type config struct {
	DataDirectory        string `mapstructure:"datadirectory"`
	UploadInfoDir        string `mapstructure:"upload_info_dir"`
	ShareDirectory       string `mapstructure:"sharedirectory"`
	UserLayout           string `mapstructure:"user_layout"`
	Redis                string `mapstructure:"redis"`
	EnableHome           bool   `mapstructure:"enable_home"`
	Scan                 bool   `mapstructure:"scan"`
	UserProviderEndpoint string `mapstructure:"userprovidersvc"`
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
	if c.Redis == "" {
		c.Redis = ":6379"
	}
	if c.UserLayout == "" {
		c.UserLayout = "{{.Username}}"
	}
	if c.UploadInfoDir == "" {
		c.UploadInfoDir = "/var/tmp/reva/uploadinfo"
	}
	if c.ShareDirectory == "" {
		c.ShareDirectory = "/Shares"
	}
	// default to scanning if not configured
	if _, ok := m["scan"]; !ok {
		c.Scan = true
	}
	c.UserProviderEndpoint = sharedconf.GetGatewaySVC(c.UserProviderEndpoint)
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init(m)

	// c.DataDirectory should never end in / unless it is the root?
	c.DataDirectory = path.Clean(c.DataDirectory)

	// create datadir if it does not exist
	err = os.MkdirAll(c.DataDirectory, 0700)
	if err != nil {
		logger.New().Error().Err(err).
			Str("path", c.DataDirectory).
			Msg("could not create datadir")
	}

	err = os.MkdirAll(c.UploadInfoDir, 0700)
	if err != nil {
		logger.New().Error().Err(err).
			Str("path", c.UploadInfoDir).
			Msg("could not create uploadinfo dir")
	}

	pool := &redis.Pool{

		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,

		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", c.Redis)
			if err != nil {
				return nil, err
			}
			return c, err
		},

		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}

	return &ocfs{c: c, pool: pool}, nil
}

type ocfs struct {
	c    *config
	pool *redis.Pool
}

func (fs *ocfs) Shutdown(ctx context.Context) error {
	return fs.pool.Close()
}

// scan files and add uuid to path mapping to kv store
func (fs *ocfs) scanFiles(ctx context.Context, conn redis.Conn) {
	if fs.c.Scan {
		fs.c.Scan = false // TODO ... in progress use mutex ?
		log := appctx.GetLogger(ctx)
		log.Debug().Str("path", fs.c.DataDirectory).Msg("scanning data directory")
		err := filepath.Walk(fs.c.DataDirectory, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Error().Str("path", path).Err(err).Msg("error accessing path")
				return filepath.SkipDir
			}
			// TODO(jfd) skip versions folder only if direct in users home dir
			// we need to skip versions, otherwise a lookup by id might resolve to a version
			if strings.Contains(path, "files_versions") {
				log.Debug().Str("path", path).Err(err).Msg("skipping versions")
				return filepath.SkipDir
			}

			// reuse connection to store file ids
			id := readOrCreateID(context.Background(), path, nil)
			_, err = conn.Do("SET", id, path)
			if err != nil {
				log.Error().Str("path", path).Err(err).Msg("error caching id")
				// continue scanning
				return nil
			}

			log.Debug().Str("path", path).Str("id", id).Msg("scanned path")
			return nil
		})
		if err != nil {
			log.Error().Err(err).Str("path", fs.c.DataDirectory).Msg("error scanning data directory")
		}
	}
}

// owncloud stores files in the files subfolder
// the incoming path starts with /<username>, so we need to insert the files subfolder into the path
// and prefix the data directory
// TODO the path handed to a storage provider should not contain the username
func (fs *ocfs) wrap(ctx context.Context, fn string) (internal string) {
	if fs.c.EnableHome {
		u := user.ContextMustGetUser(ctx)
		layout := templates.WithUser(u, fs.c.UserLayout)
		internal = path.Join(fs.c.DataDirectory, layout, "files", fn)
	} else {
		// trim all /
		fn = strings.Trim(fn, "/")
		// p = "" or
		// p = <username> or
		// p = <username>/foo/bar.txt
		parts := strings.SplitN(fn, "/", 2)

		if len(parts) == 1 && parts[0] == "" {
			internal = fs.c.DataDirectory
			return
		}

		// parts[0] contains the username or userid.
		u, err := fs.getUser(ctx, parts[0])
		if err != nil {
			logger.New().Error().Err(err).
				Msg("could not get user")
			// TODO return invalid internal path?
			return
		}
		layout := templates.WithUser(u, fs.c.UserLayout)

		if len(parts) == 1 {
			// parts = "<username>"
			internal = path.Join(fs.c.DataDirectory, layout, "files")
		} else {
			// parts = "<username>", "foo/bar.txt"
			internal = path.Join(fs.c.DataDirectory, layout, "files", parts[1])
		}

	}
	return
}

func (fs *ocfs) wrapShadow(ctx context.Context, fn string) (internal string) {
	if fs.c.EnableHome {
		u := user.ContextMustGetUser(ctx)
		layout := templates.WithUser(u, fs.c.UserLayout)
		internal = path.Join(fs.c.DataDirectory, layout, "shadow_files", fn)
	} else {
		// trim all /
		fn = strings.Trim(fn, "/")
		// p = "" or
		// p = <username> or
		// p = <username>/foo/bar.txt
		parts := strings.SplitN(fn, "/", 2)

		if len(parts) == 1 && parts[0] == "" {
			internal = fs.c.DataDirectory
			return
		}

		// parts[0] contains the username or userid.
		u, err := fs.getUser(ctx, parts[0])
		if err != nil {
			logger.New().Error().Err(err).
				Msg("could not get user")
			// TODO return invalid internal path?
			return
		}
		layout := templates.WithUser(u, fs.c.UserLayout)

		if len(parts) == 1 {
			// parts = "<username>"
			internal = path.Join(fs.c.DataDirectory, layout, "shadow_files")
		} else {
			// parts = "<username>", "foo/bar.txt"
			internal = path.Join(fs.c.DataDirectory, layout, "shadow_files", parts[1])
		}
	}
	return
}

// ownloud stores versions in the files_versions subfolder
// the incoming path starts with /<username>, so we need to insert the files subfolder into the path
// and prefix the data directory
// TODO the path handed to a storage provider should not contain the username
func (fs *ocfs) getVersionsPath(ctx context.Context, np string) string {
	// np = /path/to/data/<username>/files/foo/bar.txt
	// remove data dir
	if fs.c.DataDirectory != "/" {
		// fs.c.DataDirectory is a clean path, so it never ends in /
		np = strings.TrimPrefix(np, fs.c.DataDirectory)
	}
	// np = /<username>/files/foo/bar.txt
	parts := strings.SplitN(np, "/", 4)

	// parts[1] contains the username or userid.
	u, err := fs.getUser(ctx, parts[1])
	if err != nil {
		logger.New().Error().Err(err).
			Msg("could not get user")
		// TODO return invalid internal path?
		return ""
	}
	layout := templates.WithUser(u, fs.c.UserLayout)

	switch len(parts) {
	case 3:
		// parts = "", "<username>"
		return path.Join(fs.c.DataDirectory, layout, "files_versions")
	case 4:
		// parts = "", "<username>", "foo/bar.txt"
		return path.Join(fs.c.DataDirectory, layout, "files_versions", parts[3])
	default:
		return "" // TODO Must not happen?
	}

}

// owncloud stores trashed items in the files_trashbin subfolder of a users home
func (fs *ocfs) getRecyclePath(ctx context.Context) (string, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
		return "", err
	}
	layout := templates.WithUser(u, fs.c.UserLayout)
	return path.Join(fs.c.DataDirectory, layout, "files_trashbin/files"), nil
}

func (fs *ocfs) getVersionRecyclePath(ctx context.Context) (string, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
		return "", err
	}
	layout := templates.WithUser(u, fs.c.UserLayout)
	return path.Join(fs.c.DataDirectory, layout, "files_trashbin/files_versions"), nil
}

func (fs *ocfs) unwrap(ctx context.Context, internal string) (external string) {
	if fs.c.EnableHome {
		u := user.ContextMustGetUser(ctx)
		layout := templates.WithUser(u, fs.c.UserLayout)
		trim := path.Join(fs.c.DataDirectory, layout, "files")
		external = strings.TrimPrefix(internal, trim)
		// root directory
		if external == "" {
			external = "/"
		}
	} else {
		// np = /data/<username>/files/foo/bar.txt
		// remove data dir
		if fs.c.DataDirectory != "/" {
			// fs.c.DataDirectory is a clean path, so it never ends in /
			internal = strings.TrimPrefix(internal, fs.c.DataDirectory)
			// np = /<username>/files/foo/bar.txt
		}

		parts := strings.SplitN(internal, "/", 4)
		// parts = "", "<username>", "files", "foo/bar.txt"
		switch len(parts) {
		case 1:
			external = "/"
		case 2:
			external = path.Join("/", parts[1])
		case 3:
			external = path.Join("/", parts[1])
		default:
			external = path.Join("/", parts[1], parts[3])
		}
	}
	log := appctx.GetLogger(ctx)
	log.Debug().Msgf("ocfs: unwrap: internal=%s external=%s", internal, external)
	return
}

func (fs *ocfs) unwrapShadow(ctx context.Context, internal string) (external string) {
	if fs.c.EnableHome {
		u := user.ContextMustGetUser(ctx)
		layout := templates.WithUser(u, fs.c.UserLayout)
		trim := path.Join(fs.c.DataDirectory, layout, "shadow_files")
		external = strings.TrimPrefix(internal, trim)
	} else {
		// np = /data/<username>/shadow_files/foo/bar.txt
		// remove data dir
		if fs.c.DataDirectory != "/" {
			// fs.c.DataDirectory is a clean path, so it never ends in /
			internal = strings.TrimPrefix(internal, fs.c.DataDirectory)
			// np = /<username>/shadow_files/foo/bar.txt
		}

		parts := strings.SplitN(internal, "/", 4)
		// parts = "", "<username>", "shadow_files", "foo/bar.txt"
		switch len(parts) {
		case 1:
			external = "/"
		case 2:
			external = path.Join("/", parts[1])
		case 3:
			external = path.Join("/", parts[1])
		default:
			external = path.Join("/", parts[1], parts[3])
		}
	}
	log := appctx.GetLogger(ctx)
	log.Debug().Msgf("ocfs: unwrapShadow: internal=%s external=%s", internal, external)
	return
}

// TODO the owner needs to come from a different place
func (fs *ocfs) getOwner(internal string) string {
	internal = strings.TrimPrefix(internal, fs.c.DataDirectory)
	parts := strings.SplitN(internal, "/", 3)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func (fs *ocfs) getUser(ctx context.Context, usernameOrID string) (id *userpb.User, err error) {
	u := user.ContextMustGetUser(ctx)
	// check if username matches and id is set
	if u.Username == usernameOrID && u.Id != nil && u.Id.OpaqueId != "" {
		return u, nil
	}
	// check if userid matches and username is set
	if u.Id != nil && u.Id.OpaqueId == usernameOrID && u.Username != "" {
		return u, nil
	}
	// look up at the userprovider

	// parts[0] contains the username or userid. use  user service to look up id
	c, err := pool.GetUserProviderServiceClient(fs.c.UserProviderEndpoint)
	if err != nil {
		return nil, err
	}
	res, err := c.GetUser(ctx, &userpb.GetUserRequest{
		UserId: &userpb.UserId{OpaqueId: usernameOrID},
	})
	if err != nil {
		return nil, err
	}

	if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
		logger.New().Error().Str("code", string(res.Status.Code)).Msg("user not found")
		return nil, fmt.Errorf("user not found")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		logger.New().Error().Str("code", string(res.Status.Code)).Msg("user lookup failed")
		return nil, fmt.Errorf("user lookup failed")
	}
	return res.User, nil
}

func (fs *ocfs) convertToResourceInfo(ctx context.Context, fi os.FileInfo, np string, fn string, c redis.Conn, mdKeys []string) *provider.ResourceInfo {
	id := readOrCreateID(ctx, np, c)

	etag := calcEtag(ctx, fi)

	if val, err := xattr.Get(np, etagPrefix+etag); err == nil {
		appctx.GetLogger(ctx).Debug().
			Str("np", np).
			Str("calcetag", etag).
			Str("etag", string(val)).
			Msg("overriding calculated etag")
		etag = string(val)
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

	favoriteKey := "http://owncloud.org/ns/favorite"
	if _, ok := mdKeysMap[favoriteKey]; returnAllKeys || ok {
		favorite := ""
		if u, ok := user.ContextGetUser(ctx); ok {
			// the favorite flag is specific to the user, so we need to incorporate the userid
			if uid := u.GetId(); uid != nil {
				fa := fmt.Sprintf("%s%s@%s", favPrefix, uid.GetOpaqueId(), uid.GetIdp())
				if val, err := xattr.Get(np, fa); err == nil {
					appctx.GetLogger(ctx).Debug().
						Str("np", np).
						Str("favorite", string(val)).
						Str("username", u.GetUsername()).
						Msg("found favorite flag")
					favorite = string(val)
				}
			} else {
				appctx.GetLogger(ctx).Error().Err(errtypes.UserRequired("userrequired")).Msg("user has no id")
			}
		} else {
			appctx.GetLogger(ctx).Error().Err(errtypes.UserRequired("userrequired")).Msg("error getting user from ctx")
		}
		metadata[favoriteKey] = favorite
	}

	list, err := xattr.List(np)
	if err == nil {
		for _, entry := range list {
			// filter out non-custom properties
			if !strings.HasPrefix(entry, mdPrefix) {
				continue
			}
			if val, err := xattr.Get(np, entry); err == nil {
				k := entry[len(mdPrefix):]
				if _, ok := mdKeysMap[k]; returnAllKeys || ok {
					metadata[k] = string(val)
				}
			} else {
				appctx.GetLogger(ctx).Error().Err(err).
					Str("entry", entry).
					Msgf("error retrieving xattr metadata")
			}
		}
	} else {
		appctx.GetLogger(ctx).Error().Err(err).Msg("error getting list of extended attributes")
	}

	ri := &provider.ResourceInfo{
		Id:            &provider.ResourceId{OpaqueId: id},
		Path:          fn,
		Type:          getResourceType(fi.IsDir()),
		Etag:          etag,
		MimeType:      mime.Detect(fi.IsDir(), fn),
		Size:          uint64(fi.Size()),
		PermissionSet: &provider.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Mtime: &types.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
			// TODO read nanos from where? Nanos:   fi.MTimeNanos,
		},
		ArbitraryMetadata: &provider.ArbitraryMetadata{
			Metadata: metadata,
		},
	}

	if owner, err := fs.getUser(ctx, fs.getOwner(np)); err == nil {
		ri.Owner = owner.Id
	} else {
		appctx.GetLogger(ctx).Error().Err(err).Msg("error getting owner")
	}

	return ri
}
func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

func readOrCreateID(ctx context.Context, np string, conn redis.Conn) string {
	log := appctx.GetLogger(ctx)

	// read extended file attribute for id
	//generate if not present
	var id []byte
	var err error
	if id, err = xattr.Get(np, idAttribute); err != nil {
		log.Warn().Err(err).Msg("error reading file id")
		// try generating a uuid
		if uuid, err := uuid.NewV4(); err != nil {
			log.Error().Err(err).Msg("error generating fileid")
		} else {
			// store uuid
			id = uuid.Bytes()
			if err := xattr.Set(np, idAttribute, id); err != nil {
				log.Error().Err(err).Msg("error storing file id")
			}
			// TODO cache path for uuid in redis
			// TODO reuse conn?
			if conn != nil {
				_, err := conn.Do("SET", uuid.String(), np)
				if err != nil {
					log.Error().Str("path", np).Err(err).Msg("error caching id")
					// continue
				}
			}
		}
	}
	// todo sign metadata
	var uid uuid.UUID
	if uid, err = uuid.FromBytes(id); err != nil {
		log.Error().Err(err).Msg("error parsing uuid")
		return ""
	}
	return uid.String()
}

func (fs *ocfs) getPath(ctx context.Context, id *provider.ResourceId) (string, error) {
	c := fs.pool.Get()
	defer c.Close()
	fs.scanFiles(ctx, c)
	np, err := redis.String(c.Do("GET", id.OpaqueId))
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Interface("id", id).Msg("error looking up fileid")
		return "", err
	}
	return np, nil
}

// GetPathByID returns the fn pointed by the file id, without the internal namespace
func (fs *ocfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	np, err := fs.getPath(ctx, id)
	if err != nil {
		return "", err
	}
	return fs.unwrap(ctx, np), nil
}

// resolve takes in a request path or request id and converts it to a internal path.
func (fs *ocfs) resolve(ctx context.Context, ref *provider.Reference) (string, error) {
	if ref.GetPath() != "" {
		return fs.wrap(ctx, ref.GetPath()), nil
	}

	if ref.GetId() != nil {
		np, err := fs.getPath(ctx, ref.GetId())
		if err != nil {
			return "", err
		}
		return np, nil
	}

	// reference is invalid
	return "", fmt.Errorf("invalid reference %+v", ref)
}

func (fs *ocfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	e, err := fs.getACE(g)
	if err != nil {
		return err
	}

	var attr string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		attr = sharePrefix + "g:" + e.Principal
	} else {
		attr = sharePrefix + "u:" + e.Principal
	}

	if err := xattr.Set(np, attr, getValue(e)); err != nil {
		return err
	}
	return fs.propagate(ctx, np)
}

func getValue(e *ace) []byte {
	// first byte will be replaced after converting to byte array
	val := fmt.Sprintf("_t=%s:f=%s:p=%s", e.Type, e.Flags, e.Permissions)
	b := []byte(val)
	b[0] = 0 // indicate key value
	return b
}

func getACEPerm(set *provider.ResourcePermissions) (string, error) {
	var b strings.Builder

	if set.Stat || set.InitiateFileDownload || set.ListContainer {
		b.WriteString("r")
	}
	if set.InitiateFileUpload || set.Move {
		b.WriteString("w")
	}
	if set.CreateContainer {
		b.WriteString("a")
	}
	if set.Delete {
		b.WriteString("d")
	}

	// sharing
	if set.AddGrant || set.RemoveGrant || set.UpdateGrant {
		b.WriteString("C")
	}
	if set.ListGrants {
		b.WriteString("c")
	}

	// trash
	if set.ListRecycle {
		b.WriteString("u")
	}
	if set.RestoreRecycleItem {
		b.WriteString("U")
	}
	if set.PurgeRecycle {
		b.WriteString("P")
	}

	// versions
	if set.ListFileVersions {
		b.WriteString("v")
	}
	if set.RestoreFileVersion {
		b.WriteString("V")
	}

	// quota
	if set.GetQuota {
		b.WriteString("q")
	}
	// TODO set quota permission?
	// TODO GetPath
	return b.String(), nil
}

func (fs *ocfs) getACE(g *provider.Grant) (*ace, error) {
	permissions, err := getACEPerm(g.Permissions)
	if err != nil {
		return nil, err
	}
	e := &ace{
		Principal:   g.Grantee.Id.OpaqueId,
		Permissions: permissions,
		// TODO creator ...
		Type: "A",
	}
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		e.Flags = "g"
	}
	return e, nil
}

type ace struct {
	//NFSv4 acls
	Type        string // t
	Flags       string // f
	Principal   string // im key
	Permissions string // p

	// sharing specific
	ShareTime int    // s
	Creator   string // c
	Expires   int    // e
	Password  string // w passWord TODO h = hash
	Label     string // l
}

func unmarshalACE(v []byte) (*ace, error) {
	// first byte indicates type of value
	switch v[0] {
	case 0: // = ':' separated key=value pairs
		s := string(v[1:])
		return unmarshalKV(s)
	default:
		return nil, fmt.Errorf("unknown ace encoding")
	}
}

func unmarshalKV(s string) (*ace, error) {
	e := &ace{}
	r := csv.NewReader(strings.NewReader(s))
	r.Comma = ':'
	r.Comment = 0
	r.FieldsPerRecord = -1
	r.LazyQuotes = false
	r.TrimLeadingSpace = false
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) != 1 {
		return nil, fmt.Errorf("more than one row of ace kvs")
	}
	for i := range records[0] {
		kv := strings.Split(records[0][i], "=")
		switch kv[0] {
		case "t":
			e.Type = kv[1]
		case "f":
			e.Flags = kv[1]
		case "p":
			e.Permissions = kv[1]
		case "s":
			v, err := strconv.Atoi(kv[1])
			if err != nil {
				return nil, err
			}
			e.ShareTime = v
		case "c":
			e.Creator = kv[1]
		case "e":
			v, err := strconv.Atoi(kv[1])
			if err != nil {
				return nil, err
			}
			e.Expires = v
		case "w":
			e.Password = kv[1]
		case "l":
			e.Label = kv[1]
			// TODO default ... log unknown keys? or add as opaque? hm we need that for tagged shares ...
		}
	}
	return e, nil
}

// Parse parses an acl string with the given delimiter (LongTextForm or ShortTextForm)
func getACEs(ctx context.Context, fsfn string, attrs []string) (entries []*ace, err error) {
	log := appctx.GetLogger(ctx)
	entries = []*ace{}
	for i := range attrs {
		if strings.HasPrefix(attrs[i], sharePrefix) {
			principal := attrs[i][len(sharePrefix):]
			var value []byte
			if value, err = xattr.Get(fsfn, attrs[i]); err != nil {
				log.Error().Err(err).Str("attr", attrs[i]).Msg("could not read attribute")
				continue
			}
			var e *ace
			if e, err = unmarshalACE(value); err != nil {
				log.Error().Err(err).Str("attr", attrs[i]).Msg("could unmarshal ace")
				continue
			}
			e.Principal = principal[2:]
			// check consistency of Flags and principal type
			if strings.Contains(e.Flags, "g") {
				if principal[:1] != "g" {
					log.Error().Str("attr", attrs[i]).Interface("ace", e).Msg("inconsistent ace: expected group")
					continue
				}
			} else {
				if principal[:1] != "u" {
					log.Error().Str("attr", attrs[i]).Interface("ace", e).Msg("inconsistent ace: expected user")
					continue
				}
			}
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func (fs *ocfs) ListGrants(ctx context.Context, ref *provider.Reference) (grants []*provider.Grant, err error) {
	log := appctx.GetLogger(ctx)
	var np string
	if np, err = fs.resolve(ctx, ref); err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}
	var attrs []string
	if attrs, err = xattr.List(np); err != nil {
		log.Error().Err(err).Msg("error listing attributes")
		return nil, err
	}

	log.Debug().Interface("attrs", attrs).Msg("read attributes")
	// filter attributes
	var aces []*ace
	if aces, err = getACEs(ctx, np, attrs); err != nil {
		log.Error().Err(err).Msg("error getting aces")
		return nil, err
	}

	grants = make([]*provider.Grant, 0, len(aces))
	for i := range aces {
		grantee := &provider.Grantee{
			// TODO lookup uid from principal
			Id:   &userpb.UserId{OpaqueId: aces[i].Principal},
			Type: fs.getGranteeType(aces[i]),
		}
		grants = append(grants, &provider.Grant{
			Grantee:     grantee,
			Permissions: fs.getGrantPermissionSet(aces[i].Permissions),
		})
	}

	return grants, nil
}

func (fs *ocfs) getGranteeType(e *ace) provider.GranteeType {
	if strings.Contains(e.Flags, "g") {
		return provider.GranteeType_GRANTEE_TYPE_GROUP
	}
	return provider.GranteeType_GRANTEE_TYPE_USER
}

func (fs *ocfs) getGrantPermissionSet(mode string) *provider.ResourcePermissions {
	p := &provider.ResourcePermissions{}
	// r
	if strings.Contains(mode, "r") {
		p.Stat = true
		p.InitiateFileDownload = true
		p.ListContainer = true
	}
	// w
	if strings.Contains(mode, "w") {
		p.InitiateFileUpload = true
		if p.InitiateFileDownload {
			p.Move = true
		}
	}
	//a
	if strings.Contains(mode, "a") {
		// TODO append data to file permission?
		p.CreateContainer = true
	}
	//x
	//if strings.Contains(mode, "x") {
	// TODO execute file permission?
	// TODO change directory permission?
	//}
	//d
	if strings.Contains(mode, "d") {
		p.Delete = true
	}
	//D ?

	// sharing
	if strings.Contains(mode, "C") {
		p.AddGrant = true
		p.RemoveGrant = true
		p.UpdateGrant = true
	}
	if strings.Contains(mode, "c") {
		p.ListGrants = true
	}

	// trash
	if strings.Contains(mode, "u") { // u = undelete
		p.ListRecycle = true
	}
	if strings.Contains(mode, "U") {
		p.RestoreRecycleItem = true
	}
	if strings.Contains(mode, "P") {
		p.PurgeRecycle = true
	}

	// versions
	if strings.Contains(mode, "v") {
		p.ListFileVersions = true
	}
	if strings.Contains(mode, "V") {
		p.RestoreFileVersion = true
	}

	// ?
	// TODO GetPath
	if strings.Contains(mode, "q") {
		p.GetQuota = true
	}
	// TODO set quota permission?
	return p
}

func (fs *ocfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {

	var np string
	if np, err = fs.resolve(ctx, ref); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	var attr string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		attr = sharePrefix + "g:" + g.Grantee.Id.OpaqueId
	} else {
		attr = sharePrefix + "u:" + g.Grantee.Id.OpaqueId
	}

	return xattr.Remove(np, attr)
}

func (fs *ocfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return fs.AddGrant(ctx, ref, g)
}

func (fs *ocfs) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

func (fs *ocfs) CreateHome(ctx context.Context) error {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
		return err
	}
	layout := templates.WithUser(u, fs.c.UserLayout)

	homePaths := []string{
		path.Join(fs.c.DataDirectory, layout, "files"),
		path.Join(fs.c.DataDirectory, layout, "files_trashbin"),
		path.Join(fs.c.DataDirectory, layout, "files_versions"),
		path.Join(fs.c.DataDirectory, layout, "uploads"),
		path.Join(fs.c.DataDirectory, layout, "shadow_files"),
	}

	for _, v := range homePaths {
		if err := os.MkdirAll(v, 0700); err != nil {
			return errors.Wrap(err, "ocfs: error creating home path: "+v)
		}
	}

	return nil
}

// If home is enabled, the relative home is always the empty string
func (fs *ocfs) GetHome(ctx context.Context) (string, error) {
	if !fs.c.EnableHome {
		return "", errtypes.NotSupported("ocfs: get home not supported")
	}
	return "", nil
}

func (fs *ocfs) CreateDir(ctx context.Context, fn string) (err error) {
	np := fs.wrap(ctx, fn)
	if err = os.Mkdir(np, 0700); err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fn)
		}
		// FIXME we also need already exists error, webdav expects 405 MethodNotAllowed
		return errors.Wrap(err, "ocfs: error creating dir "+np)
	}
	return fs.propagate(ctx, np)
}

func (fs *ocfs) isShareFolderChild(p string) bool {
	return strings.HasPrefix(p, fs.c.ShareDirectory)
}

func (fs *ocfs) isShareFolderRoot(p string) bool {
	return p == fs.c.ShareDirectory
}

func (fs *ocfs) CreateReference(ctx context.Context, p string, targetURI *url.URL) error {
	if !fs.isShareFolderChild(p) {
		return errtypes.PermissionDenied("ocfs: cannot create references outside the share folder: share_folder=" + "/Shares" + " path=" + p)
	}

	fn := fs.wrapShadow(ctx, p)

	dir, _ := path.Split(fn)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Wrapf(err, "ocfs: error creating shadow path %s", dir)
	}

	f, err := os.Create(fn)
	if err != nil {
		return errors.Wrapf(err, "ocfs: error creating shadow file %s", fn)
	}

	err = xattr.FSet(f, mdPrefix+"target", []byte(targetURI.String()))
	if err != nil {
		return errors.Wrapf(err, "ocfs: error setting the target %s on the shadow file %s", targetURI.String(), fn)
	}
	return nil
}

func (fs *ocfs) setMtime(ctx context.Context, np string, mtimeString string) error {
	log := appctx.GetLogger(ctx)
	if mtime, err := parseMTime(mtimeString); err == nil {
		// updating mtime also updates atime
		if err := os.Chtimes(np, mtime, mtime); err != nil {
			log.Error().Err(err).
				Str("np", np).
				Time("mtime", mtime).
				Msg("could not set mtime")
			return errors.Wrap(err, "could not set mtime")
		}
	} else {
		log.Error().Err(err).
			Str("np", np).
			Str("mtimeString", mtimeString).
			Msg("could not parse mtime")
		return errors.Wrap(err, "could not parse mtime")
	}
	return nil
}
func (fs *ocfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	log := appctx.GetLogger(ctx)

	var np string
	if np, err = fs.resolve(ctx, ref); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	var fi os.FileInfo
	fi, err = os.Stat(np)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fs.unwrap(ctx, np))
		}
		return errors.Wrap(err, "ocfs: error stating "+np)
	}

	errs := []error{}

	if md.Metadata != nil {
		if val, ok := md.Metadata["mtime"]; ok {
			err := fs.setMtime(ctx, np, val)
			if err != nil {
				errs = append(errs, errors.Wrap(err, "could not set mtime"))
			}
			// remove from metadata
			delete(md.Metadata, "mtime")
		}
		// TODO(jfd) special handling for atime?
		// TODO(jfd) allow setting birth time (btime)?
		// TODO(jfd) any other metadata that is interesting? fileid?
		if val, ok := md.Metadata["etag"]; ok {
			etag := calcEtag(ctx, fi)
			val = fmt.Sprintf("\"%s\"", strings.Trim(val, "\""))
			if etag == val {
				log.Debug().
					Str("np", np).
					Str("etag", val).
					Msg("ignoring request to update identical etag")
			} else
			// etag is only valid until the calculated etag changes
			// TODO(jfd) cleanup in a batch job
			if err := xattr.Set(np, etagPrefix+etag, []byte(val)); err != nil {
				log.Error().Err(err).
					Str("np", np).
					Str("calcetag", etag).
					Str("etag", val).
					Msg("could not set etag")
				errs = append(errs, errors.Wrap(err, "could not set etag"))
			}
			delete(md.Metadata, "etag")
		}
		if val, ok := md.Metadata["http://owncloud.org/ns/favorite"]; ok {
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
			if u, ok := user.ContextGetUser(ctx); ok {
				// the favorite flag is specific to the user, so we need to incorporate the userid
				if uid := u.GetId(); uid != nil {
					fa := fmt.Sprintf("%s%s@%s", favPrefix, uid.GetOpaqueId(), uid.GetIdp())
					if err := xattr.Set(np, fa, []byte(val)); err != nil {
						log.Error().Err(err).
							Str("np", np).
							Interface("user", u).
							Str("key", fa).
							Msg("could not set favorite flag")
						errs = append(errs, errors.Wrap(err, "could not set favorite flag"))
					}
				} else {
					log.Error().
						Str("np", np).
						Interface("user", u).
						Msg("user has no id")
					errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "user has no id"))
				}
			} else {
				log.Error().
					Str("np", np).
					Interface("user", u).
					Msg("error getting user from ctx")
				errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx"))
			}
			// remove from metadata
			delete(md.Metadata, "http://owncloud.org/ns/favorite")
		}
	}
	for k, v := range md.Metadata {
		if err := xattr.Set(np, mdPrefix+k, []byte(v)); err != nil {
			log.Error().Err(err).
				Str("np", np).
				Str("key", k).
				Str("val", v).
				Msg("could not set metadata")
			errs = append(errs, errors.Wrap(err, "could not set metadata"))
		}
	}
	switch len(errs) {
	case 0:
		return fs.propagate(ctx, np)
	case 1:
		return errs[0]
	default:
		// TODO how to return multiple errors?
		return errors.New("multiple errors occurred, see log for details")
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

func (fs *ocfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	log := appctx.GetLogger(ctx)

	var np string
	if np, err = fs.resolve(ctx, ref); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	_, err = os.Stat(np)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fs.unwrap(ctx, np))
		}
		return errors.Wrap(err, "ocfs: error stating "+np)
	}

	errs := []error{}
	for _, k := range keys {
		switch k {
		case "http://owncloud.org/ns/favorite":
			if u, ok := user.ContextGetUser(ctx); ok {
				// the favorite flag is specific to the user, so we need to incorporate the userid
				if uid := u.GetId(); uid != nil {
					fa := fmt.Sprintf("%s%s@%s", favPrefix, uid.GetOpaqueId(), uid.GetIdp())
					if err := xattr.Remove(np, fa); err != nil {
						log.Error().Err(err).
							Str("np", np).
							Interface("user", u).
							Str("key", fa).
							Msg("could not unset favorite flag")
						errs = append(errs, errors.Wrap(err, "could not unset favorite flag"))
					}
				} else {
					log.Error().
						Str("np", np).
						Interface("user", u).
						Msg("user has no id")
					errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "user has no id"))
				}
			} else {
				log.Error().
					Str("np", np).
					Interface("user", u).
					Msg("error getting user from ctx")
				errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx"))
			}
		default:
			if err = xattr.Remove(np, mdPrefix+k); err != nil {
				// a non-existing attribute will return an error, which we can ignore
				// (using string compare because the error type is syscall.Errno and not wrapped/recognizable)
				if e, ok := err.(*xattr.Error); !ok || e.Err.Error() != "no data available" {
					log.Error().Err(err).
						Str("np", np).
						Str("key", k).
						Msg("could not unset metadata")
					errs = append(errs, errors.Wrap(err, "could not unset metadata"))
				}
			}
		}
	}

	switch len(errs) {
	case 0:
		return fs.propagate(ctx, np)
	case 1:
		return errs[0]
	default:
		// TODO how to return multiple errors?
		return errors.New("multiple errors occurred, see log for details")
	}
}

// Delete is actually only a move to trash
//
// This is a first optimistic approach.
// When a file has versions and we want to delete the file it could happen that
// the service crashes before all moves are finished.
// That would result in invalid state like the main files was moved but the
// versions were not.
// We will live with that compromise since this storage driver will be
// deprecated soon.
func (fs *ocfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {

	var np string
	if np, err = fs.resolve(ctx, ref); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	_, err = os.Stat(np)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fs.unwrap(ctx, np))
		}
		return errors.Wrap(err, "ocfs: error stating "+np)
	}

	rp, err := fs.getRecyclePath(ctx)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving recycle path")
	}

	if err := os.MkdirAll(rp, 0700); err != nil {
		return errors.Wrap(err, "ocfs: error creating trashbin dir "+rp)
	}

	// np is the path on disk ... we need only the path relative to root
	origin := path.Dir(fs.unwrap(ctx, np))

	err = fs.trash(ctx, np, rp, origin)
	if err != nil {
		return errors.Wrapf(err, "ocfs: error deleting file %s", np)
	}
	err = fs.trashVersions(ctx, np, origin)
	if err != nil {
		return errors.Wrapf(err, "ocfs: error deleting versions of file %s", np)
	}
	return nil
}

func (fs *ocfs) trash(ctx context.Context, np string, rp string, origin string) error {
	// set origin location in metadata
	if err := xattr.Set(np, trashOriginPrefix, []byte(origin)); err != nil {
		return err
	}

	// move to trash location
	dtime := time.Now().Unix()
	tgt := path.Join(rp, fmt.Sprintf("%s.d%d", path.Base(np), dtime))
	if err := os.Rename(np, tgt); err != nil {
		if os.IsExist(err) {
			// timestamp collision, try again with higher value:
			dtime++
			tgt := path.Join(rp, fmt.Sprintf("%s.d%d", path.Base(np), dtime))
			if err := os.Rename(np, tgt); err != nil {
				return errors.Wrap(err, "ocfs: could not move item to trash")
			}
		}
	}

	return fs.propagate(ctx, path.Dir(np))
}

func (fs *ocfs) trashVersions(ctx context.Context, np string, origin string) error {
	vp := fs.getVersionsPath(ctx, np)
	vrp, err := fs.getVersionRecyclePath(ctx)
	if err != nil {
		return errors.Wrap(err, "error resolving versions recycle path")
	}

	if err := os.MkdirAll(vrp, 0700); err != nil {
		return errors.Wrap(err, "ocfs: error creating trashbin dir "+vrp)
	}

	// Ignore error since the only possible error is malformed pattern.
	versions, _ := filepath.Glob(vp + ".v*")
	for _, v := range versions {
		err := fs.trash(ctx, v, vrp, origin)
		if err != nil {
			return errors.Wrap(err, "ocfs: error deleting file "+v)
		}
	}
	return nil
}

func (fs *ocfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
	var oldName string
	if oldName, err = fs.resolve(ctx, oldRef); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}
	var newName string
	if newName, err = fs.resolve(ctx, newRef); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}
	if err = os.Rename(oldName, newName); err != nil {
		return errors.Wrap(err, "ocfs: error moving "+oldName+" to "+newName)
	}
	if err := fs.propagate(ctx, newName); err != nil {
		return err
	}
	if err := fs.propagate(ctx, path.Dir(oldName)); err != nil {
		return err
	}
	return nil
}

func (fs *ocfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}
	p := fs.unwrap(ctx, np)

	if fs.c.EnableHome {
		if fs.isShareFolderChild(p) {
			return fs.getMDShareFolder(ctx, p, mdKeys)
		}
	}

	// If GetMD is called for a path shared with the user then the path is
	// already wrapped. (fs.resolve wraps the path)
	if strings.HasPrefix(p, fs.c.DataDirectory) {
		np = p
	}
	md, err := os.Stat(np)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.unwrap(ctx, np))
		}
		return nil, errors.Wrap(err, "ocfs: error stating "+np)
	}
	c := fs.pool.Get()
	defer c.Close()
	m := fs.convertToResourceInfo(ctx, md, np, fs.unwrap(ctx, np), c, mdKeys)

	return m, nil
}

func (fs *ocfs) getMDShareFolder(ctx context.Context, p string, mdKeys []string) (*provider.ResourceInfo, error) {
	fn := fs.wrapShadow(ctx, p)
	md, err := os.Stat(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.unwrapShadow(ctx, fn))
		}
		return nil, errors.Wrapf(err, "ocfs: error stating %s", fn)
	}
	c := fs.pool.Get()
	defer c.Close()
	m := fs.convertToResourceInfo(ctx, md, fn, fs.unwrapShadow(ctx, fn), c, mdKeys)
	if !fs.isShareFolderRoot(p) {
		m.Type = provider.ResourceType_RESOURCE_TYPE_REFERENCE
		ref, err := xattr.Get(fn, mdPrefix+"target")
		if err != nil {
			return nil, err
		}
		m.Target = string(ref)
	}

	return m, nil
}

func (fs *ocfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	log := appctx.GetLogger(ctx)

	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}
	p := fs.unwrap(ctx, np)

	if fs.c.EnableHome {
		log.Debug().Msg("home enabled")
		if strings.HasPrefix(p, "/") {
			return fs.listWithHome(ctx, "/", p, mdKeys)
		}
	}

	log.Debug().Msg("list with nominal home")
	return fs.listWithNominalHome(ctx, p, mdKeys)
}

func (fs *ocfs) listWithNominalHome(ctx context.Context, p string, mdKeys []string) ([]*provider.ResourceInfo, error) {
	fn := p
	// If a user wants to list a folder shared with him the path will already
	// be wrapped with the files directory path of the share owner.
	// In that case we don't want to wrap the path again.
	if !strings.HasPrefix(p, fs.c.DataDirectory) {
		fn = fs.wrap(ctx, p)
	}
	mds, err := ioutil.ReadDir(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "ocfs: error listing %s", fn)
	}
	c := fs.pool.Get()
	defer c.Close()
	finfos := []*provider.ResourceInfo{}
	for _, md := range mds {
		p := path.Join(fn, md.Name())
		m := fs.convertToResourceInfo(ctx, md, p, fs.unwrap(ctx, p), c, mdKeys)
		finfos = append(finfos, m)
	}
	return finfos, nil
}

func (fs *ocfs) listWithHome(ctx context.Context, home, p string, mdKeys []string) ([]*provider.ResourceInfo, error) {
	log := appctx.GetLogger(ctx)
	if p == home {
		log.Debug().Msg("listing home")
		return fs.listHome(ctx, home, mdKeys)
	}

	if fs.isShareFolderRoot(p) {
		log.Debug().Msg("listing share folder root")
		return fs.listShareFolderRoot(ctx, p, mdKeys)
	}

	if fs.isShareFolderChild(p) {
		return nil, errtypes.PermissionDenied("ocfs: error listing folders inside the shared folder, only file references are stored inside")
	}

	log.Debug().Msg("listing nominal home")
	return fs.listWithNominalHome(ctx, p, mdKeys)
}

func (fs *ocfs) listHome(ctx context.Context, home string, mdKeys []string) ([]*provider.ResourceInfo, error) {
	// list files
	fn := fs.wrap(ctx, home)
	mds, err := ioutil.ReadDir(fn)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error listing files")
	}

	c := fs.pool.Get()
	defer c.Close()

	finfos := []*provider.ResourceInfo{}
	for _, md := range mds {
		p := path.Join(fn, md.Name())
		m := fs.convertToResourceInfo(ctx, md, p, fs.unwrap(ctx, p), c, mdKeys)
		finfos = append(finfos, m)
	}

	// list shadow_files
	fn = fs.wrapShadow(ctx, home)
	mds, err = ioutil.ReadDir(fn)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error listing shadow_files")
	}
	for _, md := range mds {
		p := path.Join(fn, md.Name())
		m := fs.convertToResourceInfo(ctx, md, p, fs.unwrapShadow(ctx, p), c, mdKeys)
		finfos = append(finfos, m)
	}
	return finfos, nil
}

func (fs *ocfs) listShareFolderRoot(ctx context.Context, p string, mdKeys []string) ([]*provider.ResourceInfo, error) {
	fn := fs.wrapShadow(ctx, p)
	mds, err := ioutil.ReadDir(fn)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error listing shadow_files")
	}

	c := fs.pool.Get()
	defer c.Close()

	finfos := []*provider.ResourceInfo{}
	for _, md := range mds {
		p := path.Join(fn, md.Name())
		m := fs.convertToResourceInfo(ctx, md, p, fs.unwrapShadow(ctx, p), c, mdKeys)
		m.Type = provider.ResourceType_RESOURCE_TYPE_REFERENCE
		ref, err := xattr.Get(p, mdPrefix+"target")
		if err != nil {
			return nil, err
		}
		m.Target = string(ref)
		finfos = append(finfos, m)
	}

	return finfos, nil
}

func (fs *ocfs) archiveRevision(ctx context.Context, vbp string, np string) error {
	// move existing file to versions dir
	vp := fmt.Sprintf("%s.v%d", vbp, time.Now().Unix())
	if err := os.MkdirAll(path.Dir(vp), 0700); err != nil {
		return errors.Wrap(err, "ocfs: error creating versions dir "+vp)
	}

	// TODO(jfd): make sure rename is atomic, missing fsync ...
	if err := os.Rename(np, vp); err != nil {
		return errors.Wrap(err, "ocfs: error renaming from "+np+" to "+vp)
	}

	return nil
}

func (fs *ocfs) copyMD(s string, t string) (err error) {
	var attrs []string
	if attrs, err = xattr.List(s); err != nil {
		return err
	}
	for i := range attrs {
		if strings.HasPrefix(attrs[i], "user.oc.") {
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

func (fs *ocfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}
	r, err := os.Open(np)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.unwrap(ctx, np))
		}
		return nil, errors.Wrap(err, "ocfs: error reading "+np)
	}
	return r, nil
}

func (fs *ocfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}
	vp := fs.getVersionsPath(ctx, np)

	bn := path.Base(np)

	revisions := []*provider.FileVersion{}
	mds, err := ioutil.ReadDir(path.Dir(vp))
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error reading"+path.Dir(vp))
	}
	for i := range mds {
		rev := fs.filterAsRevision(ctx, bn, mds[i])
		if rev != nil {
			revisions = append(revisions, rev)
		}

	}
	return revisions, nil
}

func (fs *ocfs) filterAsRevision(ctx context.Context, bn string, md os.FileInfo) *provider.FileVersion {
	if strings.HasPrefix(md.Name(), bn) {
		// versions have filename.ext.v12345678
		version := md.Name()[len(bn)+2:] // truncate "<base filename>.v" to get version mtime
		mtime, err := strconv.Atoi(version)
		if err != nil {
			log := appctx.GetLogger(ctx)
			log.Error().Err(err).Str("path", md.Name()).Msg("invalid version mtime")
			return nil
		}
		// TODO(jfd) trashed versions are in the files_trashbin/versions folder ... not relevant here
		return &provider.FileVersion{
			Key:   version,
			Size:  uint64(md.Size()),
			Mtime: uint64(mtime),
		}
	}
	return nil
}

func (fs *ocfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("download revision")
}

func (fs *ocfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}
	vp := fs.getVersionsPath(ctx, np)
	rp := vp + ".v" + revisionKey

	// check revision exists
	rs, err := os.Stat(rp)
	if err != nil {
		return err
	}

	if !rs.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", rp)
	}

	source, err := os.Open(rp)
	if err != nil {
		return err
	}
	defer source.Close()

	// destination should be available, otherwise we could not have navigated to its revisions
	if err := fs.archiveRevision(ctx, fs.getVersionsPath(ctx, np), np); err != nil {
		return err
	}

	destination, err := os.Create(np)
	if err != nil {
		// TODO(jfd) bring back revision in case sth goes wrong?
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)

	if err != nil {
		return err
	}

	// TODO(jfd) bring back revision in case sth goes wrong?
	return fs.propagate(ctx, np)
}

func (fs *ocfs) PurgeRecycleItem(ctx context.Context, key string) error {
	rp, err := fs.getRecyclePath(ctx)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving recycle path")
	}
	ip := path.Join(rp, path.Clean(key))

	err = os.Remove(ip)
	if err != nil {
		return errors.Wrap(err, "ocfs: error deleting recycle item")
	}
	err = os.RemoveAll(path.Join(path.Dir(rp), "versions", path.Clean(key)))
	if err != nil {
		return errors.Wrap(err, "ocfs: error deleting recycle item versions")
	}
	// TODO delete keyfiles, keys, share-keys
	return nil
}

func (fs *ocfs) EmptyRecycle(ctx context.Context) error {
	rp, err := fs.getRecyclePath(ctx)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving recycle path")
	}
	err = os.RemoveAll(rp)
	if err != nil {
		return errors.Wrap(err, "ocfs: error deleting recycle files")
	}
	err = os.RemoveAll(path.Join(path.Dir(rp), "versions"))
	if err != nil {
		return errors.Wrap(err, "ocfs: error deleting recycle files versions")
	}
	// TODO delete keyfiles, keys, share-keys ... or just everything?
	return nil
}

func (fs *ocfs) convertToRecycleItem(ctx context.Context, rp string, md os.FileInfo) *provider.RecycleItem {
	// trashbin items have filename.ext.d12345678
	suffix := path.Ext(md.Name())
	if len(suffix) == 0 || !strings.HasPrefix(suffix, ".d") {
		log := appctx.GetLogger(ctx)
		log.Error().Str("path", md.Name()).Msg("invalid trash item suffix")
		return nil
	}
	trashtime := suffix[2:] // truncate "d" to get trashbin time
	ttime, err := strconv.Atoi(trashtime)
	if err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Str("path", md.Name()).Msg("invalid trash time")
		return nil
	}
	var v []byte
	if v, err = xattr.Get(path.Join(rp, md.Name()), trashOriginPrefix); err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Str("path", md.Name()).Msg("could not read origin")
		return nil
	}
	// ownCloud 10 stores the parent dir of the deleted item as the location in the oc_files_trashbin table
	// we use extended attributes for original location, but also only the parent location, which is why
	// we need to join and trim the path when listing it
	originalPath := path.Join(string(v), strings.TrimSuffix(path.Base(md.Name()), suffix))

	return &provider.RecycleItem{
		Type: getResourceType(md.IsDir()),
		Key:  md.Name(),
		// TODO do we need to prefix the path? it should be relative to this storage root, right?
		Path: originalPath,
		Size: uint64(md.Size()),
		DeletionTime: &types.Timestamp{
			Seconds: uint64(ttime),
			// no nanos available
		},
	}
}

func (fs *ocfs) ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error) {
	rp, err := fs.getRecyclePath(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving recycle path")
	}

	// list files folder
	mds, err := ioutil.ReadDir(rp)
	if err != nil {
		log := appctx.GetLogger(ctx)
		log.Debug().Err(err).Str("path", rp).Msg("trash not readable")
		// TODO jfd only ignore not found errors
		return []*provider.RecycleItem{}, nil
	}
	// TODO (jfd) limit and offset
	items := []*provider.RecycleItem{}
	for i := range mds {
		ri := fs.convertToRecycleItem(ctx, rp, mds[i])
		if ri != nil {
			items = append(items, ri)
		}

	}
	return items, nil
}

func (fs *ocfs) RestoreRecycleItem(ctx context.Context, key string) error {
	log := appctx.GetLogger(ctx)
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		return errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
	}
	rp, err := fs.getRecyclePath(ctx)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving recycle path")
	}
	src := path.Join(rp, path.Clean(key))

	suffix := path.Ext(src)
	if len(suffix) == 0 || !strings.HasPrefix(suffix, ".d") {
		log.Error().Str("path", src).Msg("invalid trash item suffix")
		return nil
	}

	origin := "/"
	if v, err := xattr.Get(src, trashOriginPrefix); err != nil {
		log.Error().Err(err).Str("path", src).Msg("could not read origin")
	} else {
		origin = path.Clean(string(v))
	}
	layout := templates.WithUser(u, fs.c.UserLayout)
	tgt := path.Join(fs.wrap(ctx, path.Join("/", layout, origin)), strings.TrimSuffix(path.Base(src), suffix))
	// move back to original location
	if err := os.Rename(src, tgt); err != nil {
		log.Error().Err(err).Str("path", src).Msg("could not restore item")
		return errors.Wrap(err, "ocfs: could not restore item")
	}
	// unset trash origin location in metadata
	if err := xattr.Remove(tgt, trashOriginPrefix); err != nil {
		// just a warning, will be overwritten next time it is deleted
		log.Warn().Err(err).Str("path", tgt).Msg("could not unset origin")
	}
	// TODO(jfd) restore versions

	return fs.propagate(ctx, tgt)
}

func (fs *ocfs) propagate(ctx context.Context, leafPath string) error {
	var root string
	if fs.c.EnableHome {
		root = fs.wrap(ctx, "/")
	} else {
		owner := fs.getOwner(leafPath)
		root = fs.wrap(ctx, owner)
	}
	if !strings.HasPrefix(leafPath, root) {
		err := errors.New("internal path outside root")
		appctx.GetLogger(ctx).Error().
			Err(err).
			Str("leafPath", leafPath).
			Str("root", root).
			Msg("could not propagate change")
		return err
	}

	fi, err := os.Stat(leafPath)
	if err != nil {
		appctx.GetLogger(ctx).Error().
			Err(err).
			Str("leafPath", leafPath).
			Str("root", root).
			Msg("could not propagate change")
		return err
	}

	parts := strings.Split(strings.TrimPrefix(leafPath, root), "/")
	// root never ends in / so the split returns an empty first element, which we can skip
	// we do not need to chmod the last element because it is the leaf path (< and not <= comparison)
	for i := 1; i < len(parts); i++ {
		appctx.GetLogger(ctx).Debug().
			Str("leafPath", leafPath).
			Str("root", root).
			Int("i", i).
			Interface("parts", parts).
			Msg("propagating change")
		if err := os.Chtimes(path.Join(root), fi.ModTime(), fi.ModTime()); err != nil {
			appctx.GetLogger(ctx).Error().
				Err(err).
				Str("leafPath", leafPath).
				Str("root", root).
				Msg("could not propagate change")
			return err
		}
		root = path.Join(root, parts[i])
	}
	return nil
}

// TODO propagate etag and mtime or append event to history? propagate on disk ...
// - but propagation is a separate task. only if upload was successful ...
