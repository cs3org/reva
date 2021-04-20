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

package owncloud

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/utils/ace"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/gomodule/redigo/redis"
	"github.com/google/uuid"
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
	ocPrefix string = "user.oc."

	// idAttribute is the name of the filesystem extended attribute that is used to store the uuid in
	idAttribute string = ocPrefix + "id"

	// SharePrefix is the prefix for sharing related extended attributes
	sharePrefix       string = ocPrefix + "grant." // grants are similar to acls, but they are not propagated down the tree when being changed
	trashOriginPrefix string = ocPrefix + "o"
	mdPrefix          string = ocPrefix + "md."   // arbitrary metadata
	favPrefix         string = ocPrefix + "fav."  // favorite flag, per user
	etagPrefix        string = ocPrefix + "etag." // allow overriding a calculated etag with one from the extended attributes
	checksumPrefix    string = ocPrefix + "cs."
	checksumsKey      string = "http://owncloud.org/ns/checksums"
	favoriteKey       string = "http://owncloud.org/ns/favorite"
)

var defaultPermissions *provider.ResourcePermissions = &provider.ResourcePermissions{
	// no permissions
}
var ownerPermissions *provider.ResourcePermissions = &provider.ResourcePermissions{
	// all permissions
	AddGrant:             true,
	CreateContainer:      true,
	Delete:               true,
	GetPath:              true,
	GetQuota:             true,
	InitiateFileDownload: true,
	InitiateFileUpload:   true,
	ListContainer:        true,
	ListFileVersions:     true,
	ListGrants:           true,
	ListRecycle:          true,
	Move:                 true,
	PurgeRecycle:         true,
	RemoveGrant:          true,
	RestoreFileVersion:   true,
	RestoreRecycleItem:   true,
	Stat:                 true,
	UpdateGrant:          true,
}

func init() {
	registry.Register("owncloud", New)
}

type config struct {
	DataDirectory            string `mapstructure:"datadirectory"`
	UploadInfoDir            string `mapstructure:"upload_info_dir"`
	DeprecatedShareDirectory string `mapstructure:"sharedirectory"`
	ShareFolder              string `mapstructure:"share_folder"`
	UserLayout               string `mapstructure:"user_layout"`
	Redis                    string `mapstructure:"redis"`
	EnableHome               bool   `mapstructure:"enable_home"`
	Scan                     bool   `mapstructure:"scan"`
	UserProviderEndpoint     string `mapstructure:"userprovidersvc"`
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
		c.UserLayout = "{{.Id.OpaqueId}}"
	}
	if c.UploadInfoDir == "" {
		c.UploadInfoDir = "/var/tmp/reva/uploadinfo"
	}
	// fallback for old config
	if c.DeprecatedShareDirectory != "" {
		c.ShareFolder = c.DeprecatedShareDirectory
	}
	if c.ShareFolder == "" {
		c.ShareFolder = "/Shares"
	}
	// ensure share folder always starts with slash
	c.ShareFolder = filepath.Join("/", c.ShareFolder)

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
	c.DataDirectory = filepath.Clean(c.DataDirectory)

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

	return &ocfs{
		c:            c,
		pool:         pool,
		chunkHandler: chunking.NewChunkHandler(c.UploadInfoDir),
	}, nil
}

type ocfs struct {
	c            *config
	pool         *redis.Pool
	chunkHandler *chunking.ChunkHandler
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
func (fs *ocfs) toInternalPath(ctx context.Context, sp string) (ip string) {
	if fs.c.EnableHome {
		u := user.ContextMustGetUser(ctx)
		layout := templates.WithUser(u, fs.c.UserLayout)
		ip = filepath.Join(fs.c.DataDirectory, layout, "files", sp)
	} else {
		// trim all /
		sp = strings.Trim(sp, "/")
		// p = "" or
		// p = <username> or
		// p = <username>/foo/bar.txt
		segments := strings.SplitN(sp, "/", 2)

		if len(segments) == 1 && segments[0] == "" {
			ip = fs.c.DataDirectory
			return
		}

		// parts[0] contains the username or userid.
		u, err := fs.getUser(ctx, segments[0])
		if err != nil {
			// TODO return invalid internal path?
			return
		}
		layout := templates.WithUser(u, fs.c.UserLayout)

		if len(segments) == 1 {
			// parts = "<username>"
			ip = filepath.Join(fs.c.DataDirectory, layout, "files")
		} else {
			// parts = "<username>", "foo/bar.txt"
			ip = filepath.Join(fs.c.DataDirectory, layout, "files", segments[1])
		}

	}
	return
}

func (fs *ocfs) toInternalShadowPath(ctx context.Context, sp string) (internal string) {
	if fs.c.EnableHome {
		u := user.ContextMustGetUser(ctx)
		layout := templates.WithUser(u, fs.c.UserLayout)
		internal = filepath.Join(fs.c.DataDirectory, layout, "shadow_files", sp)
	} else {
		// trim all /
		sp = strings.Trim(sp, "/")
		// p = "" or
		// p = <username> or
		// p = <username>/foo/bar.txt
		segments := strings.SplitN(sp, "/", 2)

		if len(segments) == 1 && segments[0] == "" {
			internal = fs.c.DataDirectory
			return
		}

		// parts[0] contains the username or userid.
		u, err := fs.getUser(ctx, segments[0])
		if err != nil {
			// TODO return invalid internal path?
			return
		}
		layout := templates.WithUser(u, fs.c.UserLayout)

		if len(segments) == 1 {
			// parts = "<username>"
			internal = filepath.Join(fs.c.DataDirectory, layout, "shadow_files")
		} else {
			// parts = "<username>", "foo/bar.txt"
			internal = filepath.Join(fs.c.DataDirectory, layout, "shadow_files", segments[1])
		}
	}
	return
}

// ownloud stores versions in the files_versions subfolder
// the incoming path starts with /<username>, so we need to insert the files subfolder into the path
// and prefix the data directory
// TODO the path handed to a storage provider should not contain the username
func (fs *ocfs) getVersionsPath(ctx context.Context, ip string) string {
	// ip = /path/to/data/<username>/files/foo/bar.txt
	// remove data dir
	if fs.c.DataDirectory != "/" {
		// fs.c.DataDirectory is a clean path, so it never ends in /
		ip = strings.TrimPrefix(ip, fs.c.DataDirectory)
	}
	// ip = /<username>/files/foo/bar.txt
	parts := strings.SplitN(ip, "/", 4)

	// parts[1] contains the username or userid.
	u, err := fs.getUser(ctx, parts[1])
	if err != nil {
		// TODO return invalid internal path?
		return ""
	}
	layout := templates.WithUser(u, fs.c.UserLayout)

	switch len(parts) {
	case 3:
		// parts = "", "<username>"
		return filepath.Join(fs.c.DataDirectory, layout, "files_versions")
	case 4:
		// parts = "", "<username>", "foo/bar.txt"
		return filepath.Join(fs.c.DataDirectory, layout, "files_versions", parts[3])
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
	return filepath.Join(fs.c.DataDirectory, layout, "files_trashbin/files"), nil
}

func (fs *ocfs) getVersionRecyclePath(ctx context.Context) (string, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
		return "", err
	}
	layout := templates.WithUser(u, fs.c.UserLayout)
	return filepath.Join(fs.c.DataDirectory, layout, "files_trashbin/files_versions"), nil
}

func (fs *ocfs) toStoragePath(ctx context.Context, ip string) (sp string) {
	if fs.c.EnableHome {
		u := user.ContextMustGetUser(ctx)
		layout := templates.WithUser(u, fs.c.UserLayout)
		trim := filepath.Join(fs.c.DataDirectory, layout, "files")
		sp = strings.TrimPrefix(ip, trim)
		// root directory
		if sp == "" {
			sp = "/"
		}
	} else {
		// ip = /data/<username>/files/foo/bar.txt
		// remove data dir
		if fs.c.DataDirectory != "/" {
			// fs.c.DataDirectory is a clean path, so it never ends in /
			ip = strings.TrimPrefix(ip, fs.c.DataDirectory)
			// ip = /<username>/files/foo/bar.txt
		}

		segments := strings.SplitN(ip, "/", 4)
		// parts = "", "<username>", "files", "foo/bar.txt"
		switch len(segments) {
		case 1:
			sp = "/"
		case 2:
			sp = filepath.Join("/", segments[1])
		case 3:
			sp = filepath.Join("/", segments[1])
		default:
			sp = filepath.Join("/", segments[1], segments[3])
		}
	}
	log := appctx.GetLogger(ctx)
	log.Debug().Str("driver", "ocfs").Str("ipath", ip).Str("spath", sp).Msg("toStoragePath")
	return
}

func (fs *ocfs) toStorageShadowPath(ctx context.Context, ip string) (sp string) {
	if fs.c.EnableHome {
		u := user.ContextMustGetUser(ctx)
		layout := templates.WithUser(u, fs.c.UserLayout)
		trim := filepath.Join(fs.c.DataDirectory, layout, "shadow_files")
		sp = strings.TrimPrefix(ip, trim)
	} else {
		// ip = /data/<username>/shadow_files/foo/bar.txt
		// remove data dir
		if fs.c.DataDirectory != "/" {
			// fs.c.DataDirectory is a clean path, so it never ends in /
			ip = strings.TrimPrefix(ip, fs.c.DataDirectory)
			// ip = /<username>/shadow_files/foo/bar.txt
		}

		segments := strings.SplitN(ip, "/", 4)
		// parts = "", "<username>", "shadow_files", "foo/bar.txt"
		switch len(segments) {
		case 1:
			sp = "/"
		case 2:
			sp = filepath.Join("/", segments[1])
		case 3:
			sp = filepath.Join("/", segments[1])
		default:
			sp = filepath.Join("/", segments[1], segments[3])
		}
	}
	appctx.GetLogger(ctx).Debug().Str("driver", "ocfs").Str("ipath", ip).Str("spath", sp).Msg("toStorageShadowPath")
	return
}

// TODO the owner needs to come from a different place
func (fs *ocfs) getOwner(ip string) string {
	ip = strings.TrimPrefix(ip, fs.c.DataDirectory)
	parts := strings.SplitN(ip, "/", 3)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// TODO cache user lookup
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
		appctx.GetLogger(ctx).
			Error().Err(err).
			Str("userprovidersvc", fs.c.UserProviderEndpoint).
			Str("usernameOrID", usernameOrID).
			Msg("could not get user provider client")
		return nil, err
	}
	res, err := c.GetUser(ctx, &userpb.GetUserRequest{
		UserId: &userpb.UserId{OpaqueId: usernameOrID},
	})
	if err != nil {
		appctx.GetLogger(ctx).
			Error().Err(err).
			Str("userprovidersvc", fs.c.UserProviderEndpoint).
			Str("usernameOrID", usernameOrID).
			Msg("could not get user")
		return nil, err
	}

	if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
		appctx.GetLogger(ctx).
			Error().
			Str("userprovidersvc", fs.c.UserProviderEndpoint).
			Str("usernameOrID", usernameOrID).
			Interface("status", res.Status).
			Msg("user not found")
		return nil, fmt.Errorf("user not found")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		appctx.GetLogger(ctx).
			Error().
			Str("userprovidersvc", fs.c.UserProviderEndpoint).
			Str("usernameOrID", usernameOrID).
			Interface("status", res.Status).
			Msg("user lookup failed")
		return nil, fmt.Errorf("user lookup failed")
	}
	return res.User, nil
}

// permissionSet returns the permission set for the current user
func (fs *ocfs) permissionSet(ctx context.Context, owner *userpb.UserId) *provider.ResourcePermissions {
	if owner == nil {
		return &provider.ResourcePermissions{
			Stat: true,
		}
	}
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		return &provider.ResourcePermissions{
			// no permissions
		}
	}
	if u.Id == nil {
		return &provider.ResourcePermissions{
			// no permissions
		}
	}
	if u.Id.OpaqueId == owner.OpaqueId && u.Id.Idp == owner.Idp {
		return &provider.ResourcePermissions{
			// owner has all permissions
			AddGrant:             true,
			CreateContainer:      true,
			Delete:               true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
			InitiateFileUpload:   true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListGrants:           true,
			ListRecycle:          true,
			Move:                 true,
			PurgeRecycle:         true,
			RemoveGrant:          true,
			RestoreFileVersion:   true,
			RestoreRecycleItem:   true,
			Stat:                 true,
			UpdateGrant:          true,
		}
	}
	// TODO fix permissions for share recipients by traversing reading acls up to the root? cache acls for the parent node and reuse it
	return &provider.ResourcePermissions{
		AddGrant:             true,
		CreateContainer:      true,
		Delete:               true,
		GetPath:              true,
		GetQuota:             true,
		InitiateFileDownload: true,
		InitiateFileUpload:   true,
		ListContainer:        true,
		ListFileVersions:     true,
		ListGrants:           true,
		ListRecycle:          true,
		Move:                 true,
		PurgeRecycle:         true,
		RemoveGrant:          true,
		RestoreFileVersion:   true,
		RestoreRecycleItem:   true,
		Stat:                 true,
		UpdateGrant:          true,
	}
}
func (fs *ocfs) convertToResourceInfo(ctx context.Context, fi os.FileInfo, ip string, sp string, c redis.Conn, mdKeys []string) *provider.ResourceInfo {
	id := readOrCreateID(ctx, ip, c)

	etag := calcEtag(ctx, fi)

	if val, err := xattr.Get(ip, etagPrefix+etag); err == nil {
		appctx.GetLogger(ctx).Debug().
			Str("ipath", ip).
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

	if _, ok := mdKeysMap[favoriteKey]; returnAllKeys || ok {
		favorite := ""
		if u, ok := user.ContextGetUser(ctx); ok {
			// the favorite flag is specific to the user, so we need to incorporate the userid
			if uid := u.GetId(); uid != nil {
				fa := fmt.Sprintf("%s%s@%s", favPrefix, uid.GetOpaqueId(), uid.GetIdp())
				if val, err := xattr.Get(ip, fa); err == nil {
					appctx.GetLogger(ctx).Debug().
						Str("ipath", ip).
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

	list, err := xattr.List(ip)
	if err == nil {
		for _, entry := range list {
			// filter out non-custom properties
			if !strings.HasPrefix(entry, mdPrefix) {
				continue
			}
			if val, err := xattr.Get(ip, entry); err == nil {
				k := entry[len(mdPrefix):]
				if _, ok := mdKeysMap[k]; returnAllKeys || ok {
					metadata[k] = string(val)
				}
			} else {
				appctx.GetLogger(ctx).Error().Err(err).
					Str("entry", entry).
					Msg("error retrieving xattr metadata")
			}
		}
	} else {
		appctx.GetLogger(ctx).Error().Err(err).Msg("error getting list of extended attributes")
	}

	ri := &provider.ResourceInfo{
		Id:       &provider.ResourceId{OpaqueId: id},
		Path:     sp,
		Type:     getResourceType(fi.IsDir()),
		Etag:     etag,
		MimeType: mime.Detect(fi.IsDir(), ip),
		Size:     uint64(fi.Size()),
		Mtime: &types.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
			// TODO read nanos from where? Nanos:   fi.MTimeNanos,
		},
		ArbitraryMetadata: &provider.ArbitraryMetadata{
			Metadata: metadata,
		},
	}

	if owner, err := fs.getUser(ctx, fs.getOwner(ip)); err == nil {
		ri.Owner = owner.Id
	} else {
		appctx.GetLogger(ctx).Error().Err(err).Msg("error getting owner")
	}

	ri.PermissionSet = fs.permissionSet(ctx, ri.Owner)

	// checksums
	if !fi.IsDir() {
		if _, checksumRequested := mdKeysMap[checksumsKey]; returnAllKeys || checksumRequested {
			// TODO which checksum was requested? sha1 adler32 or md5? for now hardcode sha1?
			readChecksumIntoResourceChecksum(ctx, ip, storageprovider.XSSHA1, ri)
			readChecksumIntoOpaque(ctx, ip, storageprovider.XSMD5, ri)
			readChecksumIntoOpaque(ctx, ip, storageprovider.XSAdler32, ri)
		}
	}

	return ri
}
func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

func readOrCreateID(ctx context.Context, ip string, conn redis.Conn) string {
	log := appctx.GetLogger(ctx)

	// read extended file attribute for id
	// generate if not present
	var id []byte
	var err error
	if id, err = xattr.Get(ip, idAttribute); err != nil {
		log.Warn().Err(err).Str("driver", "owncloud").Str("ipath", ip).Msg("error reading file id")

		uuid := uuid.New()
		// store uuid
		id = uuid[:]
		if err := xattr.Set(ip, idAttribute, id); err != nil {
			log.Error().Err(err).Str("driver", "owncloud").Str("ipath", ip).Msg("error storing file id")
		}
		// TODO cache path for uuid in redis
		// TODO reuse conn?
		if conn != nil {
			_, err := conn.Do("SET", uuid.String(), ip)
			if err != nil {
				log.Error().Err(err).Str("driver", "owncloud").Str("ipath", ip).Msg("error caching id")
				// continue
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
	log := appctx.GetLogger(ctx)
	c := fs.pool.Get()
	defer c.Close()
	fs.scanFiles(ctx, c)
	ip, err := redis.String(c.Do("GET", id.OpaqueId))
	if err != nil {
		return "", errtypes.NotFound(id.OpaqueId)
	}

	idFromXattr, err := xattr.Get(ip, idAttribute)
	if err != nil {
		return "", errtypes.NotFound(id.OpaqueId)
	}

	uid, err := uuid.FromBytes(idFromXattr)
	if err != nil {
		log.Error().Err(err).Msg("error parsing uuid")
	}

	if uid.String() != id.OpaqueId {
		if _, err := c.Do("DEL", id.OpaqueId); err != nil {
			return "", err
		}
		return "", errtypes.NotFound(id.OpaqueId)
	}

	return ip, nil
}

// GetPathByID returns the storage relative path for the file id, without the internal namespace
func (fs *ocfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	ip, err := fs.getPath(ctx, id)
	if err != nil {
		return "", err
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.GetPath {
			return "", errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return "", errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return "", errors.Wrap(err, "ocfs: error reading permissions")
	}

	return fs.toStoragePath(ctx, ip), nil
}

// resolve takes in a request path or request id and converts it to an internal path.
func (fs *ocfs) resolve(ctx context.Context, ref *provider.Reference) (string, error) {
	if ref.GetPath() != "" {
		return fs.toInternalPath(ctx, ref.GetPath()), nil
	}

	if ref.GetId() != nil {
		ip, err := fs.getPath(ctx, ref.GetId())
		if err != nil {
			return "", err
		}
		return ip, nil
	}

	// reference is invalid
	return "", fmt.Errorf("invalid reference %+v", ref)
}

func (fs *ocfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	ip, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.AddGrant {
			return errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return errors.Wrap(err, "ocfs: error reading permissions")
	}

	e := ace.FromGrant(g)
	principal, value := e.Marshal()
	if err := xattr.Set(ip, sharePrefix+principal, value); err != nil {
		return err
	}
	return fs.propagate(ctx, ip)
}

// extractACEsFromAttrs reads ACEs in the list of attrs from the file
func extractACEsFromAttrs(ctx context.Context, ip string, attrs []string) (entries []*ace.ACE) {
	log := appctx.GetLogger(ctx)
	entries = []*ace.ACE{}
	for i := range attrs {
		if strings.HasPrefix(attrs[i], sharePrefix) {
			var value []byte
			var err error
			if value, err = xattr.Get(ip, attrs[i]); err != nil {
				log.Error().Err(err).Str("attr", attrs[i]).Msg("could not read attribute")
				continue
			}
			var e *ace.ACE
			principal := attrs[i][len(sharePrefix):]
			if e, err = ace.Unmarshal(principal, value); err != nil {
				log.Error().Err(err).Str("principal", principal).Str("attr", attrs[i]).Msg("could not unmarshal ace")
				continue
			}
			entries = append(entries, e)
		}
	}
	return
}

// TODO if user is owner but no acls found he can do everything?
// The owncloud driver does not integrate with the os so, for now, the owner can do everything, see ownerPermissions.
// Should this change we can store an acl for the owner in every node.
// We could also add default acls that can only the admin can set, eg for a read only storage?
// Someone needs to write to provide the content that should be read only, so this would likely be an acl for a group anyway.
// We need the storage relative path so we can calculate the permissions
// for the node based on all acls in the tree up to the root
func (fs *ocfs) readPermissions(ctx context.Context, ip string) (p *provider.ResourcePermissions, err error) {

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		appctx.GetLogger(ctx).Debug().Str("ipath", ip).Msg("no user in context, returning default permissions")
		return defaultPermissions, nil
	}
	// check if the current user is the owner
	if fs.getOwner(ip) == u.Id.OpaqueId {
		appctx.GetLogger(ctx).Debug().Str("ipath", ip).Msg("user is owner, returning owner permissions")
		return ownerPermissions, nil
	}

	// for non owners this is a little more complicated:
	aggregatedPermissions := &provider.ResourcePermissions{}
	// add default permissions
	addPermissions(aggregatedPermissions, defaultPermissions)

	// determine root
	rp := fs.toInternalPath(ctx, "")
	// TODO rp will be the datadir ... be we don't want to go up that high. The users home is far enough
	np := ip

	// for an efficient group lookup convert the list of groups to a map
	// groups are just strings ... groupnames ... or group ids ??? AAARGH !!!
	groupsMap := make(map[string]bool, len(u.Groups))
	for i := range u.Groups {
		groupsMap[u.Groups[i]] = true
	}

	var e *ace.ACE
	// for all segments, starting at the leaf
	for np != rp {

		var attrs []string
		if attrs, err = xattr.List(np); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("ipath", np).Msg("error listing attributes")
			return nil, err
		}

		userace := sharePrefix + "u:" + u.Id.OpaqueId
		userFound := false
		for i := range attrs {
			// we only need the find the user once per node
			switch {
			case !userFound && attrs[i] == userace:
				e, err = fs.readACE(ctx, np, "u:"+u.Id.OpaqueId)
			case strings.HasPrefix(attrs[i], sharePrefix+"g:"):
				g := strings.TrimPrefix(attrs[i], sharePrefix+"g:")
				if groupsMap[g] {
					e, err = fs.readACE(ctx, np, "g:"+g)
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
				addPermissions(aggregatedPermissions, e.Grant().GetPermissions())
				appctx.GetLogger(ctx).Debug().Str("ipath", np).Str("principal", strings.TrimPrefix(attrs[i], sharePrefix)).Interface("permissions", aggregatedPermissions).Msg("adding permissions")
			case isNoData(err):
				err = nil
				appctx.GetLogger(ctx).Error().Str("ipath", np).Str("principal", strings.TrimPrefix(attrs[i], sharePrefix)).Interface("attrs", attrs).Msg("no permissions found on node, but they were listed")
			default:
				appctx.GetLogger(ctx).Error().Err(err).Str("ipath", np).Str("principal", strings.TrimPrefix(attrs[i], sharePrefix)).Msg("error reading permissions")
				return nil, err
			}
		}

		np = filepath.Dir(np)
	}

	// 3. read user permissions until one is found?
	//   what if, when checking /a/b/c/d, /a/b has write permission, but /a/b/c has not?
	//      those are two shares one read only, and a higher one rw,
	//       should the higher one be used?
	//       or, since we did find a matching ace in a lower node use that because it matches the principal?
	//		this would allow ai user to share a folder rm but take away the write capability for eg a docs folder inside it.
	// 4. read group permissions until all groups of the user are matched?
	//    same as for user permission, but we need to keep going further up the tree until all groups of the user were matched.
	//    what if a user has thousands of groups?
	//      we will always have to walk to the root.
	//      but the same problem occurs for a user with 2 groups but where only one group was used to share.
	//      in any case we need to iterate the aces, not the number of groups of the user.
	//        listing the aces can be used to match the principals, we do not need to fully real all aces
	//   what if, when checking /a/b/c/d, /a/b has write permission for group g, but /a/b/c has an ace for another group h the user is also a member of?
	//     it would allow restricting a users permissions by resharing something with him with lower permission?
	//     so if you have reshare permissions you could accidentially restrict users access to a subfolder of a rw share to ro by sharing it to another group as ro when they are part of both groups
	//       it makes more sense to have explicit negative permissions

	// TODO we need to read all parents ... until we find a matching ace?
	appctx.GetLogger(ctx).Debug().Interface("permissions", aggregatedPermissions).Str("ipath", ip).Msg("returning aggregated permissions")
	return aggregatedPermissions, nil
}

func isNoData(err error) bool {
	if xerr, ok := err.(*xattr.Error); ok {
		if serr, ok2 := xerr.Err.(syscall.Errno); ok2 {
			return serr == syscall.ENODATA
		}
	}
	return false
}

// The os not exists error is buried inside the xattr error,
// so we cannot just use os.IsNotExists().
func isNotFound(err error) bool {
	if xerr, ok := err.(*xattr.Error); ok {
		if serr, ok2 := xerr.Err.(syscall.Errno); ok2 {
			return serr == syscall.ENOENT
		}
	}
	return false
}

func (fs *ocfs) readACE(ctx context.Context, ip string, principal string) (e *ace.ACE, err error) {
	var b []byte
	if b, err = xattr.Get(ip, sharePrefix+principal); err != nil {
		return nil, err
	}
	if e, err = ace.Unmarshal(principal, b); err != nil {
		return nil, err
	}
	return
}

// additive merging of permissions only
func addPermissions(p1 *provider.ResourcePermissions, p2 *provider.ResourcePermissions) {
	p1.AddGrant = p1.AddGrant || p2.AddGrant
	p1.CreateContainer = p1.CreateContainer || p2.CreateContainer
	p1.Delete = p1.Delete || p2.Delete
	p1.GetPath = p1.GetPath || p2.GetPath
	p1.GetQuota = p1.GetQuota || p2.GetQuota
	p1.InitiateFileDownload = p1.InitiateFileDownload || p2.InitiateFileDownload
	p1.InitiateFileUpload = p1.InitiateFileUpload || p2.InitiateFileUpload
	p1.ListContainer = p1.ListContainer || p2.ListContainer
	p1.ListFileVersions = p1.ListFileVersions || p2.ListFileVersions
	p1.ListGrants = p1.ListGrants || p2.ListGrants
	p1.ListRecycle = p1.ListRecycle || p2.ListRecycle
	p1.Move = p1.Move || p2.Move
	p1.PurgeRecycle = p1.PurgeRecycle || p2.PurgeRecycle
	p1.RemoveGrant = p1.RemoveGrant || p2.RemoveGrant
	p1.RestoreFileVersion = p1.RestoreFileVersion || p2.RestoreFileVersion
	p1.RestoreRecycleItem = p1.RestoreRecycleItem || p2.RestoreRecycleItem
	p1.Stat = p1.Stat || p2.Stat
	p1.UpdateGrant = p1.UpdateGrant || p2.UpdateGrant
}

func (fs *ocfs) ListGrants(ctx context.Context, ref *provider.Reference) (grants []*provider.Grant, err error) {
	log := appctx.GetLogger(ctx)
	var ip string
	if ip, err = fs.resolve(ctx, ref); err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.ListGrants {
			return nil, errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return nil, errors.Wrap(err, "ocfs: error reading permissions")
	}

	var attrs []string
	if attrs, err = xattr.List(ip); err != nil {
		// TODO err might be a not exists
		log.Error().Err(err).Msg("error listing attributes")
		return nil, err
	}

	log.Debug().Interface("attrs", attrs).Msg("read attributes")

	aces := extractACEsFromAttrs(ctx, ip, attrs)

	grants = make([]*provider.Grant, 0, len(aces))
	for i := range aces {
		grants = append(grants, aces[i].Grant())
	}

	return grants, nil
}

func (fs *ocfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {

	var ip string
	if ip, err = fs.resolve(ctx, ref); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.ListContainer {
			return errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return errors.Wrap(err, "ocfs: error reading permissions")
	}

	var attr string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		attr = sharePrefix + "g:" + g.Grantee.GetGroupId().OpaqueId
	} else {
		attr = sharePrefix + "u:" + g.Grantee.GetUserId().OpaqueId
	}

	if err = xattr.Remove(ip, attr); err != nil {
		return
	}

	return fs.propagate(ctx, ip)
}

func (fs *ocfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	ip, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.UpdateGrant {
			return errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return errors.Wrap(err, "ocfs: error reading permissions")
	}

	e := ace.FromGrant(g)
	principal, value := e.Marshal()
	if err := xattr.Set(ip, sharePrefix+principal, value); err != nil {
		return err
	}
	return fs.propagate(ctx, ip)
}

func (fs *ocfs) GetQuota(ctx context.Context) (uint64, uint64, error) {
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
		filepath.Join(fs.c.DataDirectory, layout, "files"),
		filepath.Join(fs.c.DataDirectory, layout, "files_trashbin"),
		filepath.Join(fs.c.DataDirectory, layout, "files_versions"),
		filepath.Join(fs.c.DataDirectory, layout, "uploads"),
		filepath.Join(fs.c.DataDirectory, layout, "shadow_files"),
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

func (fs *ocfs) CreateDir(ctx context.Context, sp string) (err error) {
	ip := fs.toInternalPath(ctx, sp)

	// check permissions of parent dir
	if perm, err := fs.readPermissions(ctx, filepath.Dir(ip)); err == nil {
		if !perm.CreateContainer {
			return errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return errors.Wrap(err, "ocfs: error reading permissions")
	}

	if err = os.Mkdir(ip, 0700); err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(sp)
		}
		// FIXME we also need already exists error, webdav expects 405 MethodNotAllowed
		return errors.Wrap(err, "ocfs: error creating dir "+ip)
	}
	return fs.propagate(ctx, ip)
}

func (fs *ocfs) isShareFolderChild(sp string) bool {
	return strings.HasPrefix(sp, fs.c.ShareFolder)
}

func (fs *ocfs) isShareFolderRoot(sp string) bool {
	return sp == fs.c.ShareFolder
}

func (fs *ocfs) CreateReference(ctx context.Context, sp string, targetURI *url.URL) error {
	if !fs.isShareFolderChild(sp) {
		return errtypes.PermissionDenied("ocfs: cannot create references outside the share folder: share_folder=" + "/Shares" + " path=" + sp)
	}

	ip := fs.toInternalShadowPath(ctx, sp)
	// TODO check permission?

	dir, _ := filepath.Split(ip)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Wrapf(err, "ocfs: error creating shadow path %s", dir)
	}

	f, err := os.Create(ip)
	if err != nil {
		return errors.Wrapf(err, "ocfs: error creating shadow file %s", ip)
	}

	err = xattr.FSet(f, mdPrefix+"target", []byte(targetURI.String()))
	if err != nil {
		return errors.Wrapf(err, "ocfs: error setting the target %s on the shadow file %s", targetURI.String(), ip)
	}
	return nil
}

func (fs *ocfs) setMtime(ctx context.Context, ip string, mtime string) error {
	log := appctx.GetLogger(ctx)
	if mt, err := parseMTime(mtime); err == nil {
		// updating mtime also updates atime
		if err := os.Chtimes(ip, mt, mt); err != nil {
			log.Error().Err(err).
				Str("ipath", ip).
				Time("mtime", mt).
				Msg("could not set mtime")
			return errors.Wrap(err, "could not set mtime")
		}
	} else {
		log.Error().Err(err).
			Str("ipath", ip).
			Str("mtime", mtime).
			Msg("could not parse mtime")
		return errors.Wrap(err, "could not parse mtime")
	}
	return nil
}
func (fs *ocfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	log := appctx.GetLogger(ctx)

	var ip string
	if ip, err = fs.resolve(ctx, ref); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.InitiateFileUpload { // TODO add dedicated permission?
			return errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return errors.Wrap(err, "ocfs: error reading permissions")
	}

	var fi os.FileInfo
	fi, err = os.Stat(ip)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return errors.Wrap(err, "ocfs: error stating "+ip)
	}

	errs := []error{}

	if md.Metadata != nil {
		if val, ok := md.Metadata["mtime"]; ok {
			err := fs.setMtime(ctx, ip, val)
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
					Str("ipath", ip).
					Str("etag", val).
					Msg("ignoring request to update identical etag")
			} else
			// etag is only valid until the calculated etag changes
			// TODO(jfd) cleanup in a batch job
			if err := xattr.Set(ip, etagPrefix+etag, []byte(val)); err != nil {
				log.Error().Err(err).
					Str("ipath", ip).
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
					if err := xattr.Set(ip, fa, []byte(val)); err != nil {
						log.Error().Err(err).
							Str("ipath", ip).
							Interface("user", u).
							Str("key", fa).
							Msg("could not set favorite flag")
						errs = append(errs, errors.Wrap(err, "could not set favorite flag"))
					}
				} else {
					log.Error().
						Str("ipath", ip).
						Interface("user", u).
						Msg("user has no id")
					errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "user has no id"))
				}
			} else {
				log.Error().
					Str("ipath", ip).
					Interface("user", u).
					Msg("error getting user from ctx")
				errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx"))
			}
			// remove from metadata
			delete(md.Metadata, "http://owncloud.org/ns/favorite")
		}
	}
	for k, v := range md.Metadata {
		if err := xattr.Set(ip, mdPrefix+k, []byte(v)); err != nil {
			log.Error().Err(err).
				Str("ipath", ip).
				Str("key", k).
				Str("val", v).
				Msg("could not set metadata")
			errs = append(errs, errors.Wrap(err, "could not set metadata"))
		}
	}
	switch len(errs) {
	case 0:
		return fs.propagate(ctx, ip)
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

	var ip string
	if ip, err = fs.resolve(ctx, ref); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.InitiateFileUpload { // TODO add dedicated permission?
			return errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return errors.Wrap(err, "ocfs: error reading permissions")
	}

	_, err = os.Stat(ip)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return errors.Wrap(err, "ocfs: error stating "+ip)
	}

	errs := []error{}
	for _, k := range keys {
		switch k {
		case "http://owncloud.org/ns/favorite":
			if u, ok := user.ContextGetUser(ctx); ok {
				// the favorite flag is specific to the user, so we need to incorporate the userid
				if uid := u.GetId(); uid != nil {
					fa := fmt.Sprintf("%s%s@%s", favPrefix, uid.GetOpaqueId(), uid.GetIdp())
					if err := xattr.Remove(ip, fa); err != nil {
						log.Error().Err(err).
							Str("ipath", ip).
							Interface("user", u).
							Str("key", fa).
							Msg("could not unset favorite flag")
						errs = append(errs, errors.Wrap(err, "could not unset favorite flag"))
					}
				} else {
					log.Error().
						Str("ipath", ip).
						Interface("user", u).
						Msg("user has no id")
					errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "user has no id"))
				}
			} else {
				log.Error().
					Str("ipath", ip).
					Interface("user", u).
					Msg("error getting user from ctx")
				errs = append(errs, errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx"))
			}
		default:
			if err = xattr.Remove(ip, mdPrefix+k); err != nil {
				// a non-existing attribute will return an error, which we can ignore
				// (using string compare because the error type is syscall.Errno and not wrapped/recognizable)
				if e, ok := err.(*xattr.Error); !ok || !(e.Err.Error() == "no data available" ||
					// darwin
					e.Err.Error() == "attribute not found") {
					log.Error().Err(err).
						Str("ipath", ip).
						Str("key", k).
						Msg("could not unset metadata")
					errs = append(errs, errors.Wrap(err, "could not unset metadata"))
				}
			}
		}
	}

	switch len(errs) {
	case 0:
		return fs.propagate(ctx, ip)
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
	var ip string
	if ip, err = fs.resolve(ctx, ref); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.Delete {
			return errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return errors.Wrap(err, "ocfs: error reading permissions")
	}

	_, err = os.Stat(ip)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return errors.Wrap(err, "ocfs: error stating "+ip)
	}

	rp, err := fs.getRecyclePath(ctx)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving recycle path")
	}

	if err := os.MkdirAll(rp, 0700); err != nil {
		return errors.Wrap(err, "ocfs: error creating trashbin dir "+rp)
	}

	// ip is the path on disk ... we need only the path relative to root
	origin := filepath.Dir(fs.toStoragePath(ctx, ip))

	err = fs.trash(ctx, ip, rp, origin)
	if err != nil {
		return errors.Wrapf(err, "ocfs: error deleting file %s", ip)
	}
	err = fs.trashVersions(ctx, ip, origin)
	if err != nil {
		return errors.Wrapf(err, "ocfs: error deleting versions of file %s", ip)
	}
	return nil
}

func (fs *ocfs) trash(ctx context.Context, ip string, rp string, origin string) error {
	// set origin location in metadata
	if err := xattr.Set(ip, trashOriginPrefix, []byte(origin)); err != nil {
		return err
	}

	// move to trash location
	dtime := time.Now().Unix()
	tgt := filepath.Join(rp, fmt.Sprintf("%s.d%d", filepath.Base(ip), dtime))
	if err := os.Rename(ip, tgt); err != nil {
		if os.IsExist(err) {
			// timestamp collision, try again with higher value:
			dtime++
			tgt := filepath.Join(rp, fmt.Sprintf("%s.d%d", filepath.Base(ip), dtime))
			if err := os.Rename(ip, tgt); err != nil {
				return errors.Wrap(err, "ocfs: could not move item to trash")
			}
		}
	}

	return fs.propagate(ctx, filepath.Dir(ip))
}

func (fs *ocfs) trashVersions(ctx context.Context, ip string, origin string) error {
	vp := fs.getVersionsPath(ctx, ip)
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
	var oldIP string
	if oldIP, err = fs.resolve(ctx, oldRef); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, oldIP); err == nil {
		if !perm.Move { // TODO add dedicated permission?
			return errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(oldIP)))
		}
		return errors.Wrap(err, "ocfs: error reading permissions")
	}

	var newIP string
	if newIP, err = fs.resolve(ctx, newRef); err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	// TODO check target permissions ... if it exists

	if err = os.Rename(oldIP, newIP); err != nil {
		return errors.Wrap(err, "ocfs: error moving "+oldIP+" to "+newIP)
	}
	if err := fs.propagate(ctx, newIP); err != nil {
		return err
	}
	if err := fs.propagate(ctx, filepath.Dir(oldIP)); err != nil {
		return err
	}
	return nil
}

func (fs *ocfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	ip, err := fs.resolve(ctx, ref)
	if err != nil {
		// TODO return correct errtype
		if _, ok := err.(errtypes.IsNotFound); ok {
			return nil, err
		}
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}
	p := fs.toStoragePath(ctx, ip)

	if fs.c.EnableHome {
		if fs.isShareFolderChild(p) {
			return fs.getMDShareFolder(ctx, p, mdKeys)
		}
	}

	// If GetMD is called for a path shared with the user then the path is
	// already wrapped. (fs.resolve wraps the path)
	if strings.HasPrefix(p, fs.c.DataDirectory) {
		ip = p
	}
	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.Stat {
			return nil, errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return nil, errors.Wrap(err, "ocfs: error reading permissions")
	}

	md, err := os.Stat(ip)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return nil, errors.Wrap(err, "ocfs: error stating "+ip)
	}
	c := fs.pool.Get()
	defer c.Close()
	m := fs.convertToResourceInfo(ctx, md, ip, fs.toStoragePath(ctx, ip), c, mdKeys)

	return m, nil
}

func (fs *ocfs) getMDShareFolder(ctx context.Context, sp string, mdKeys []string) (*provider.ResourceInfo, error) {
	ip := fs.toInternalShadowPath(ctx, sp)

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.Stat {
			return nil, errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return nil, errors.Wrap(err, "ocfs: error reading permissions")
	}

	md, err := os.Stat(ip)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.toStorageShadowPath(ctx, ip))
		}
		return nil, errors.Wrapf(err, "ocfs: error stating %s", ip)
	}
	c := fs.pool.Get()
	defer c.Close()
	m := fs.convertToResourceInfo(ctx, md, ip, fs.toStorageShadowPath(ctx, ip), c, mdKeys)
	if !fs.isShareFolderRoot(sp) {
		m.Type = provider.ResourceType_RESOURCE_TYPE_REFERENCE
		ref, err := xattr.Get(ip, mdPrefix+"target")
		if err != nil {
			if isNotFound(err) {
				return nil, errtypes.NotFound(fs.toStorageShadowPath(ctx, ip))
			}
			return nil, err
		}
		m.Target = string(ref)
	}

	return m, nil
}

func (fs *ocfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	log := appctx.GetLogger(ctx)

	ip, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}
	sp := fs.toStoragePath(ctx, ip)

	if fs.c.EnableHome {
		log.Debug().Msg("home enabled")
		if strings.HasPrefix(sp, "/") {
			// permissions checked in listWithHome
			return fs.listWithHome(ctx, "/", sp, mdKeys)
		}
	}

	log.Debug().Msg("list with nominal home")
	// permissions checked in listWithNominalHome
	return fs.listWithNominalHome(ctx, sp, mdKeys)
}

func (fs *ocfs) listWithNominalHome(ctx context.Context, ip string, mdKeys []string) ([]*provider.ResourceInfo, error) {

	// If a user wants to list a folder shared with him the path will already
	// be wrapped with the files directory path of the share owner.
	// In that case we don't want to wrap the path again.
	if !strings.HasPrefix(ip, fs.c.DataDirectory) {
		ip = fs.toInternalPath(ctx, ip)
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.ListContainer {
			return nil, errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return nil, errors.Wrap(err, "ocfs: error reading permissions")
	}

	mds, err := ioutil.ReadDir(ip)
	if err != nil {
		return nil, errors.Wrapf(err, "ocfs: error listing %s", ip)
	}
	c := fs.pool.Get()
	defer c.Close()
	finfos := []*provider.ResourceInfo{}
	for _, md := range mds {
		cp := filepath.Join(ip, md.Name())
		m := fs.convertToResourceInfo(ctx, md, cp, fs.toStoragePath(ctx, cp), c, mdKeys)
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
	ip := fs.toInternalPath(ctx, home)

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.ListContainer {
			return nil, errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return nil, errors.Wrap(err, "ocfs: error reading permissions")
	}

	mds, err := ioutil.ReadDir(ip)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error listing files")
	}

	c := fs.pool.Get()
	defer c.Close()

	finfos := []*provider.ResourceInfo{}
	for _, md := range mds {
		cp := filepath.Join(ip, md.Name())
		m := fs.convertToResourceInfo(ctx, md, cp, fs.toStoragePath(ctx, cp), c, mdKeys)
		finfos = append(finfos, m)
	}

	// list shadow_files
	ip = fs.toInternalShadowPath(ctx, home)
	mds, err = ioutil.ReadDir(ip)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error listing shadow_files")
	}
	for _, md := range mds {
		cp := filepath.Join(ip, md.Name())
		m := fs.convertToResourceInfo(ctx, md, cp, fs.toStorageShadowPath(ctx, cp), c, mdKeys)
		finfos = append(finfos, m)
	}
	return finfos, nil
}

func (fs *ocfs) listShareFolderRoot(ctx context.Context, sp string, mdKeys []string) ([]*provider.ResourceInfo, error) {
	ip := fs.toInternalShadowPath(ctx, sp)

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.ListContainer {
			return nil, errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return nil, errors.Wrap(err, "ocfs: error reading permissions")
	}

	mds, err := ioutil.ReadDir(ip)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return nil, errors.Wrap(err, "ocfs: error listing shadow_files")
	}

	c := fs.pool.Get()
	defer c.Close()

	finfos := []*provider.ResourceInfo{}
	for _, md := range mds {
		cp := filepath.Join(ip, md.Name())
		m := fs.convertToResourceInfo(ctx, md, cp, fs.toStorageShadowPath(ctx, cp), c, mdKeys)
		m.Type = provider.ResourceType_RESOURCE_TYPE_REFERENCE
		ref, err := xattr.Get(cp, mdPrefix+"target")
		if err != nil {
			return nil, err
		}
		m.Target = string(ref)
		finfos = append(finfos, m)
	}

	return finfos, nil
}

func (fs *ocfs) archiveRevision(ctx context.Context, vbp string, ip string) error {
	// move existing file to versions dir
	vp := fmt.Sprintf("%s.v%d", vbp, time.Now().Unix())
	if err := os.MkdirAll(filepath.Dir(vp), 0700); err != nil {
		return errors.Wrap(err, "ocfs: error creating versions dir "+vp)
	}

	// TODO(jfd): make sure rename is atomic, missing fsync ...
	if err := os.Rename(ip, vp); err != nil {
		return errors.Wrap(err, "ocfs: error renaming from "+ip+" to "+vp)
	}

	return nil
}

func (fs *ocfs) copyMD(s string, t string) (err error) {
	var attrs []string
	if attrs, err = xattr.List(s); err != nil {
		return err
	}
	for i := range attrs {
		if strings.HasPrefix(attrs[i], ocPrefix) {
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
	ip, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.InitiateFileDownload {
			return nil, errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return nil, errors.Wrap(err, "ocfs: error reading permissions")
	}

	r, err := os.Open(ip)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, ip))
		}
		return nil, errors.Wrap(err, "ocfs: error reading "+ip)
	}
	return r, nil
}

func (fs *ocfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	ip, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.ListFileVersions {
			return nil, errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return nil, errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return nil, errors.Wrap(err, "ocfs: error reading permissions")
	}

	vp := fs.getVersionsPath(ctx, ip)

	bn := filepath.Base(ip)

	revisions := []*provider.FileVersion{}
	mds, err := ioutil.ReadDir(filepath.Dir(vp))
	if err != nil {
		return nil, errors.Wrap(err, "ocfs: error reading"+filepath.Dir(vp))
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
			Etag:  calcEtag(ctx, md),
		}
	}
	return nil
}

func (fs *ocfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("download revision")
}

func (fs *ocfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	ip, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving reference")
	}

	// check permissions
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.RestoreFileVersion {
			return errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return errtypes.NotFound(fs.toStoragePath(ctx, filepath.Dir(ip)))
		}
		return errors.Wrap(err, "ocfs: error reading permissions")
	}

	vp := fs.getVersionsPath(ctx, ip)
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
	if err := fs.archiveRevision(ctx, fs.getVersionsPath(ctx, ip), ip); err != nil {
		return err
	}

	destination, err := os.Create(ip)
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
	return fs.propagate(ctx, ip)
}

func (fs *ocfs) PurgeRecycleItem(ctx context.Context, key string) error {
	rp, err := fs.getRecyclePath(ctx)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving recycle path")
	}
	ip := filepath.Join(rp, filepath.Clean(key))
	// TODO check permission?

	// check permissions
	/* are they stored in the trash?
	if perm, err := fs.readPermissions(ctx, ip); err == nil {
		if !perm.ListContainer {
			return nil, errtypes.PermissionDenied("")
		}
	} else {
		if isNotFound(err) {
			return nil, errtypes.NotFound(fs.unwrap(ctx, filepath.Dir(ip)))
		}
		return nil, errors.Wrap(err, "ocfs: error reading permissions")
	}
	*/

	err = os.Remove(ip)
	if err != nil {
		return errors.Wrap(err, "ocfs: error deleting recycle item")
	}
	err = os.RemoveAll(filepath.Join(filepath.Dir(rp), "versions", filepath.Clean(key)))
	if err != nil {
		return errors.Wrap(err, "ocfs: error deleting recycle item versions")
	}
	// TODO delete keyfiles, keys, share-keys
	return nil
}

func (fs *ocfs) EmptyRecycle(ctx context.Context) error {
	// TODO check permission? on what? user must be the owner
	rp, err := fs.getRecyclePath(ctx)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving recycle path")
	}
	err = os.RemoveAll(rp)
	if err != nil {
		return errors.Wrap(err, "ocfs: error deleting recycle files")
	}
	err = os.RemoveAll(filepath.Join(filepath.Dir(rp), "versions"))
	if err != nil {
		return errors.Wrap(err, "ocfs: error deleting recycle files versions")
	}
	// TODO delete keyfiles, keys, share-keys ... or just everything?
	return nil
}

func (fs *ocfs) convertToRecycleItem(ctx context.Context, rp string, md os.FileInfo) *provider.RecycleItem {
	// trashbin items have filename.ext.d12345678
	suffix := filepath.Ext(md.Name())
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
	if v, err = xattr.Get(filepath.Join(rp, md.Name()), trashOriginPrefix); err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Str("path", md.Name()).Msg("could not read origin")
		return nil
	}
	// ownCloud 10 stores the parent dir of the deleted item as the location in the oc_files_trashbin table
	// we use extended attributes for original location, but also only the parent location, which is why
	// we need to join and trim the path when listing it
	originalPath := filepath.Join(string(v), strings.TrimSuffix(filepath.Base(md.Name()), suffix))

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
	// TODO check permission? on what? user must be the owner?
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

func (fs *ocfs) RestoreRecycleItem(ctx context.Context, key, restorePath string) error {
	// TODO check permission? on what? user must be the owner?
	log := appctx.GetLogger(ctx)
	rp, err := fs.getRecyclePath(ctx)
	if err != nil {
		return errors.Wrap(err, "ocfs: error resolving recycle path")
	}
	src := filepath.Join(rp, filepath.Clean(key))

	suffix := filepath.Ext(src)
	if len(suffix) == 0 || !strings.HasPrefix(suffix, ".d") {
		log.Error().Str("key", key).Str("path", src).Msg("invalid trash item suffix")
		return nil
	}

	if restorePath == "" {
		v, err := xattr.Get(src, trashOriginPrefix)
		if err != nil {
			log.Error().Err(err).Str("key", key).Str("path", src).Msg("could not read origin")
		}
		restorePath = filepath.Join("/", filepath.Clean(string(v)), strings.TrimSuffix(filepath.Base(src), suffix))
	}
	tgt := fs.toInternalPath(ctx, restorePath)
	// move back to original location
	if err := os.Rename(src, tgt); err != nil {
		log.Error().Err(err).Str("key", key).Str("restorePath", restorePath).Str("src", src).Str("tgt", tgt).Msg("could not restore item")
		return errors.Wrap(err, "ocfs: could not restore item")
	}
	// unset trash origin location in metadata
	if err := xattr.Remove(tgt, trashOriginPrefix); err != nil {
		// just a warning, will be overwritten next time it is deleted
		log.Warn().Err(err).Str("key", key).Str("tgt", tgt).Msg("could not unset origin")
	}
	// TODO(jfd) restore versions

	return fs.propagate(ctx, tgt)
}

func (fs *ocfs) propagate(ctx context.Context, leafPath string) error {
	var root string
	if fs.c.EnableHome {
		root = fs.toInternalPath(ctx, "/")
	} else {
		owner := fs.getOwner(leafPath)
		root = fs.toInternalPath(ctx, owner)
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
		if err := os.Chtimes(filepath.Join(root), fi.ModTime(), fi.ModTime()); err != nil {
			appctx.GetLogger(ctx).Error().
				Err(err).
				Str("leafPath", leafPath).
				Str("root", root).
				Msg("could not propagate change")
			return err
		}
		root = filepath.Join(root, parts[i])
	}
	return nil
}

func readChecksumIntoResourceChecksum(ctx context.Context, nodePath, algo string, ri *provider.ResourceInfo) {
	v, err := xattr.Get(nodePath, checksumPrefix+algo)
	log := appctx.GetLogger(ctx).
		Debug().
		Err(err).
		Str("nodepath", nodePath).
		Str("algorithm", algo)
	switch {
	case err == nil:
		ri.Checksum = &provider.ResourceChecksum{
			Type: storageprovider.PKG2GRPCXS(algo),
			Sum:  hex.EncodeToString(v),
		}
	case isNoData(err):
		log.Msg("checksum not set")
	case isNotFound(err):
		log.Msg("file not found")
	default:
		log.Msg("could not read checksum")
	}
}

func readChecksumIntoOpaque(ctx context.Context, nodePath, algo string, ri *provider.ResourceInfo) {
	v, err := xattr.Get(nodePath, checksumPrefix+algo)
	log := appctx.GetLogger(ctx).
		Debug().
		Err(err).
		Str("nodepath", nodePath).
		Str("algorithm", algo)
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
		log.Msg("checksum not set")
	case isNotFound(err):
		log.Msg("file not found")
	default:
		log.Msg("could not read checksum")
	}
}

// TODO propagate etag and mtime or append event to history? propagate on disk ...
// - but propagation is a separate task. only if upload was successful ...
