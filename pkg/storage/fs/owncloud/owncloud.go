// Copyright 2018-2019 CERN
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
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/gofrs/uuid"
	"github.com/gomodule/redigo/redis"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

const (
	// Currently,extended file attributes have four separated
	// namespaces (user, trusted, security and system) followed by a dot.
	// A non ruut user can only manipulate the user. namespace, which is what
	// we will use to store ownCloud specific metadata. To prevent name
	// collisions with other apps We are going to introduca a sub namespace
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
	//   - d delete - delete the file/directory. Some servers will allow a delete to occur if either this permission is set in the file/directory or if the delete-child permission is set in its parent direcory.
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
	// ext3 extended attributes must fit inside a signle filesystem block ... 4096 bytes
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
	sharePrefix string = "user.oc.acl."
)

func init() {
	registry.Register("owncloud", New)
}

type config struct {
	DataDirectory string `mapstructure:"datadirectory"`
	Scan          bool   `mapstructure:"scan"`
	Autocreate    bool   `mapstructure:"autocreate"`
	Redis         string `mapstructure:"redis"`
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
	// default to scanning if not configured
	if _, ok := m["scan"]; !ok {
		c.Scan = true
	}
	// default to autocreate if not configured
	if _, ok := m["scan"]; !ok {
		c.Autocreate = true
	}
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init(m)

	// c.DataDirectoryshould never end in / unless it is the root?
	c.DataDirectory = path.Clean(c.DataDirectory)

	// create datadir if it does not exist
	err = os.MkdirAll(c.DataDirectory, 0700)
	if err != nil {
		logger.New().Error().Err(err).
			Str("path", c.DataDirectory).
			Msg("could not create datadir")
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

	return &ocFS{c: c, pool: pool}, nil
}

type ocFS struct {
	c    *config
	pool *redis.Pool
}

func (fs *ocFS) Shutdown(ctx context.Context) error {
	return fs.pool.Close()
}

// scan files and add uuid to path mapping to kv store
func (fs *ocFS) scanFiles(ctx context.Context, conn redis.Conn) {
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

// ownloud stores files in the files subfolder
// the incoming path starts with /<username>, so we need to insert the files subfolder into the path
// and prefix the datadirectory
// TODO the path handed to a storage provider should not contain the username
func (fs *ocFS) getInternalPath(ctx context.Context, fn string) string {
	// p = /<username> or
	// p = /<username>/foo/bar.txt
	parts := strings.SplitN(fn, "/", 3)

	switch len(parts) {
	case 2:
		// parts = "", "<username>"
		return path.Join(fs.c.DataDirectory, parts[1], "files")
	case 3:
		// parts = "", "<username>", "foo/bar.txt"
		return path.Join(fs.c.DataDirectory, parts[1], "files", parts[2])
	default:
		return "" // TODO Must not happen?
	}
}

// ownloud stores versions in the files_versions subfolder
// the incoming path starts with /<username>, so we need to insert the files subfolder into the path
// and prefix the datadirectory
// TODO the path handed to a storage provider should not contain the username
func (fs *ocFS) getVersionsPath(ctx context.Context, np string) string {
	// np = /path/to/data/<username>/files/foo/bar.txt
	// remove data dir
	if fs.c.DataDirectory != "/" {
		// fs.c.DataDirectory is a clean puth, so it never ends in /
		np = strings.TrimPrefix(np, fs.c.DataDirectory)
	}
	// np = /<username>/files/foo/bar.txt
	parts := strings.SplitN(np, "/", 4)

	switch len(parts) {
	case 3:
		// parts = "", "<username>"
		return path.Join(fs.c.DataDirectory, parts[1], "files_versions")
	case 4:
		// parts = "", "<username>", "foo/bar.txt"
		return path.Join(fs.c.DataDirectory, parts[1], "files_versions", parts[3])
	default:
		return "" // TODO Must not happen?
	}

}

func (fs *ocFS) removeNamespace(ctx context.Context, np string) string {
	// np = /data/<username>/files/foo/bar.txt
	// remove data dir
	if fs.c.DataDirectory != "/" {
		// fs.c.DataDirectory is a clean puth, so it never ends in /
		np = strings.TrimPrefix(np, fs.c.DataDirectory)
		// np = /<username>/files/foo/bar.txt
	}

	parts := strings.SplitN(np, "/", 4)
	// parts = "", "<username>", "files", "foo/bar.txt"
	switch len(parts) {
	case 1:
		return "/"
	case 2:
		return path.Join("/", parts[1])
	case 3:
		return path.Join("/", parts[1])
	default:
		return path.Join("/", parts[1], parts[3])
	}
}

func getOwner(fn string) string {
	parts := strings.SplitN(fn, "/", 3)
	// parts = "", "<username>", "files", "foo/bar.txt"
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func (fs *ocFS) convertToResourceInfo(ctx context.Context, fi os.FileInfo, np string, c redis.Conn) *storageproviderv0alphapb.ResourceInfo {
	id := readOrCreateID(ctx, np, c)
	fn := fs.removeNamespace(ctx, path.Join("/", np))

	return &storageproviderv0alphapb.ResourceInfo{
		Id:            &storageproviderv0alphapb.ResourceId{OpaqueId: id},
		Path:          fn,
		Owner:         &typespb.UserId{OpaqueId: getOwner(fn)},
		Type:          getResourceType(fi.IsDir()),
		Etag:          calcEtag(ctx, fi),
		MimeType:      mime.Detect(fi.IsDir(), fn),
		Size:          uint64(fi.Size()),
		PermissionSet: &storageproviderv0alphapb.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Mtime: &typespb.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
			// TODO read nanos from where? Nanos:   fi.MTimeNanos,
		},
	}
}
func getResourceType(isDir bool) storageproviderv0alphapb.ResourceType {
	if isDir {
		return storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_FILE
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

func (fs *ocFS) autocreate(ctx context.Context, fsfn string) {
	if fs.c.Autocreate {
		parts := strings.SplitN(fsfn, "/files", 2)
		switch len(parts) {
		case 1:
			return // error? there is no files in here ...
		case 2:
			if parts[1] == "" {
				// nothing to do, fsfn is the home
			} else {
				// only create home
				fsfn = path.Join(parts[0], "files")
			}
			err := os.MkdirAll(fsfn, 0700)
			if err != nil {
				appctx.GetLogger(ctx).Debug().Err(err).
					Str("fsfn", fsfn).
					Msg("could not autocreate dir")
			}
		}
	}
}

func (fs *ocFS) getPath(ctx context.Context, id *storageproviderv0alphapb.ResourceId) (string, error) {
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
func (fs *ocFS) GetPathByID(ctx context.Context, id *storageproviderv0alphapb.ResourceId) (string, error) {
	np, err := fs.getPath(ctx, id)
	if err != nil {
		return "", err
	}
	return fs.removeNamespace(ctx, np), nil
}

// resolve takes in a request path or request id and converts it to a internal path.
func (fs *ocFS) resolve(ctx context.Context, ref *storageproviderv0alphapb.Reference) (string, error) {
	if ref.GetPath() != "" {
		return fs.getInternalPath(ctx, ref.GetPath()), nil
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

func (fs *ocFS) AddGrant(ctx context.Context, ref *storageproviderv0alphapb.Reference, g *storageproviderv0alphapb.Grant) error {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocFS: error resolving reference")
	}

	e, err := fs.getACE(g)
	if err != nil {
		return err
	}

	var attr string
	if g.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP {
		attr = sharePrefix + "g:" + e.Principal
	} else {
		attr = sharePrefix + "u:" + e.Principal
	}

	return xattr.Set(np, attr, getValue(e))
}

func getValue(e *ace) []byte {
	// first byte will be replaced after converting to byte array
	val := fmt.Sprintf("_t=%s:f=%s:p=%s", e.Type, e.Flags, e.Permissions)
	b := []byte(val)
	b[0] = 0 // indicalte key value
	return b
}

func getACEPerm(set *storageproviderv0alphapb.ResourcePermissions) (string, error) {
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

func (fs *ocFS) getACE(g *storageproviderv0alphapb.Grant) (*ace, error) {
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
	if g.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP {
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

func (fs *ocFS) ListGrants(ctx context.Context, ref *storageproviderv0alphapb.Reference) (grants []*storageproviderv0alphapb.Grant, err error) {
	log := appctx.GetLogger(ctx)
	var np string
	if np, err = fs.resolve(ctx, ref); err != nil {
		return nil, errors.Wrap(err, "ocFS: error resolving reference")
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

	grants = make([]*storageproviderv0alphapb.Grant, 0, len(aces))
	for i := range aces {
		grantee := &storageproviderv0alphapb.Grantee{
			Id:   &typespb.UserId{OpaqueId: aces[i].Principal},
			Type: fs.getGranteeType(aces[i]),
		}
		grants = append(grants, &storageproviderv0alphapb.Grant{
			Grantee:     grantee,
			Permissions: fs.getGrantPermissionSet(aces[i].Permissions),
		})
	}

	return grants, nil
}

func (fs *ocFS) getGranteeType(e *ace) storageproviderv0alphapb.GranteeType {
	if strings.Contains(e.Flags, "g") {
		return storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP
	}
	return storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER
}

func (fs *ocFS) getGrantPermissionSet(mode string) *storageproviderv0alphapb.ResourcePermissions {
	p := &storageproviderv0alphapb.ResourcePermissions{}
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

func (fs *ocFS) RemoveGrant(ctx context.Context, ref *storageproviderv0alphapb.Reference, g *storageproviderv0alphapb.Grant) (err error) {

	var np string
	if np, err = fs.resolve(ctx, ref); err != nil {
		return errors.Wrap(err, "ocFS: error resolving reference")
	}

	var attr string
	if g.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP {
		attr = sharePrefix + "g:" + g.Grantee.Id.OpaqueId
	} else {
		attr = sharePrefix + "u:" + g.Grantee.Id.OpaqueId
	}

	return xattr.Remove(np, attr)
}

func (fs *ocFS) UpdateGrant(ctx context.Context, ref *storageproviderv0alphapb.Reference, g *storageproviderv0alphapb.Grant) error {
	return fs.AddGrant(ctx, ref, g)
}

func (fs *ocFS) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

func (fs *ocFS) CreateDir(ctx context.Context, fn string) (err error) {
	np := fs.getInternalPath(ctx, fn)
	if err = os.Mkdir(np, 0700); err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fn)
		}
		// FIXME we also need already exists error, webdav expects 405 MethodNotAllowed
		return errors.Wrap(err, "ocFS: error creating dir "+np)
	}
	return nil
}

func (fs *ocFS) Delete(ctx context.Context, ref *storageproviderv0alphapb.Reference) (err error) {
	var np string
	if np, err = fs.resolve(ctx, ref); err != nil {
		return errors.Wrap(err, "ocFS: error resolving reference")
	}
	if err = os.Remove(np); err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fs.removeNamespace(ctx, np))
		}
		// try recursive delete
		if err = os.RemoveAll(np); err != nil {
			return errors.Wrap(err, "ocFS: error deleting "+np)
		}
	}
	return nil
}

func (fs *ocFS) Move(ctx context.Context, oldRef, newRef *storageproviderv0alphapb.Reference) (err error) {
	var oldName string
	if oldName, err = fs.resolve(ctx, oldRef); err != nil {
		return errors.Wrap(err, "ocFS: error resolving reference")
	}
	var newName string
	if newName, err = fs.resolve(ctx, newRef); err != nil {
		return errors.Wrap(err, "ocFS: error resolving reference")
	}
	if err = os.Rename(oldName, newName); err != nil {
		return errors.Wrap(err, "ocFS: error moving "+oldName+" to "+newName)
	}
	return nil
}

func (fs *ocFS) GetMD(ctx context.Context, ref *storageproviderv0alphapb.Reference) (*storageproviderv0alphapb.ResourceInfo, error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocFS: error resolving reference")
	}

	fs.autocreate(ctx, np)

	md, err := os.Stat(np)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.removeNamespace(ctx, np))
		}
		return nil, errors.Wrap(err, "ocFS: error stating "+np)
	}
	c := fs.pool.Get()
	defer c.Close()
	m := fs.convertToResourceInfo(ctx, md, np, c)

	return m, nil
}

func (fs *ocFS) ListFolder(ctx context.Context, ref *storageproviderv0alphapb.Reference) ([]*storageproviderv0alphapb.ResourceInfo, error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocFS: error resolving reference")
	}

	fs.autocreate(ctx, np)

	mds, err := ioutil.ReadDir(np)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.removeNamespace(ctx, np))
		}
		return nil, errors.Wrap(err, "ocFS: error listing "+np)
	}

	finfos := make([]*storageproviderv0alphapb.ResourceInfo, 0, len(mds))
	// TODO we should only open a connection if we need to set / store the fileid. no need to always open a connection when listing files
	c := fs.pool.Get()
	defer c.Close()
	for i := range mds {
		p := path.Join(np, mds[i].Name())
		m := fs.convertToResourceInfo(ctx, mds[i], p, c)
		finfos = append(finfos, m)
	}
	return finfos, nil
}

func (fs *ocFS) Upload(ctx context.Context, ref *storageproviderv0alphapb.Reference, r io.ReadCloser) error {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocFS: error resolving reference")
	}

	// we cannot rely on /tmp as it can live in another partition and we can
	// hit invalid cross-device link errors, so we create the tmp file in the same directory
	// the file is supposed to be written.
	tmp, err := ioutil.TempFile(path.Dir(np), "._reva_atomic_upload")
	if err != nil {
		return errors.Wrap(err, "ocFS: error creating tmp fn at "+path.Dir(np))
	}

	_, err = io.Copy(tmp, r)
	if err != nil {
		return errors.Wrap(err, "ocFS: error writing to tmp file "+tmp.Name())
	}

	// if destination exists
	if _, err := os.Stat(np); err == nil {
		// copy attributes of existing file to tmp file
		if err := fs.copyMD(np, tmp.Name()); err != nil {
			return errors.Wrap(err, "ocFS: error copying metadata from "+np+" to "+tmp.Name())
		}
		// create revision
		if err := fs.archiveRevision(ctx, np); err != nil {
			return err
		}
	}

	// TODO(jfd): make sure rename is atomic, missing fsync ...
	if err := os.Rename(tmp.Name(), np); err != nil {
		return errors.Wrap(err, "ocFS: error renaming from "+tmp.Name()+" to "+np)
	}

	return nil
}

func (fs *ocFS) archiveRevision(ctx context.Context, np string) error {
	// move existing file to versions dir
	vp := fmt.Sprintf("%s.v%d", fs.getVersionsPath(ctx, np), time.Now().Unix())
	if err := os.MkdirAll(path.Dir(vp), 0700); err != nil {
		return errors.Wrap(err, "ocFS: error creating versions dir "+vp)
	}

	// TODO(jfd): make sure rename is atomic, missing fsync ...
	if err := os.Rename(np, vp); err != nil {
		return errors.Wrap(err, "ocFS: error renaming from "+np+" to "+vp)
	}

	return nil
}

func (fs *ocFS) copyMD(s string, t string) (err error) {
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

func (fs *ocFS) Download(ctx context.Context, ref *storageproviderv0alphapb.Reference) (io.ReadCloser, error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocFS: error resolving reference")
	}
	r, err := os.Open(np)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fs.removeNamespace(ctx, np))
		}
		return nil, errors.Wrap(err, "ocFS: error reading "+np)
	}
	return r, nil
}

func (fs *ocFS) ListRevisions(ctx context.Context, ref *storageproviderv0alphapb.Reference) ([]*storageproviderv0alphapb.FileVersion, error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "ocFS: error resolving reference")
	}
	vp := fs.getVersionsPath(ctx, np)

	fs.autocreate(ctx, vp)

	bn := path.Base(np)

	revisions := []*storageproviderv0alphapb.FileVersion{}
	mds, err := ioutil.ReadDir(path.Dir(vp))
	if err != nil {
		return nil, errors.Wrap(err, "ocFS: error reading"+path.Dir(vp))
	}
	for i := range mds {
		rev := fs.filterAsRevision(ctx, bn, mds[i])
		if rev != nil {
			revisions = append(revisions, rev)
		}

	}
	return revisions, nil
}

func (fs *ocFS) filterAsRevision(ctx context.Context, bn string, md os.FileInfo) *storageproviderv0alphapb.FileVersion {
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
		return &storageproviderv0alphapb.FileVersion{
			Key:   version,
			Size:  uint64(md.Size()),
			Mtime: uint64(mtime),
		}
	}
	return nil
}

func (fs *ocFS) DownloadRevision(ctx context.Context, ref *storageproviderv0alphapb.Reference, revisionKey string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("download revision")
}

func (fs *ocFS) RestoreRevision(ctx context.Context, ref *storageproviderv0alphapb.Reference, revisionKey string) error {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "ocFS: error resolving reference")
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
	if err := fs.archiveRevision(ctx, np); err != nil {
		return err
	}

	destination, err := os.Create(np)
	if err != nil {
		// TODO(jfd) bring back revision in case sth goes wrong?
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	// TODO(jfd) bring back revision in case sth goes wrong?
	return err
}

func (fs *ocFS) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("empty recycle")
}

func (fs *ocFS) ListRecycle(ctx context.Context) ([]*storageproviderv0alphapb.RecycleItem, error) {
	return nil, errtypes.NotSupported("list recycle")
}

func (fs *ocFS) RestoreRecycleItem(ctx context.Context, key string) error {
	return errtypes.NotSupported("restore recycle")
}
