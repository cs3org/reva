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

package local

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("local", New)
}

type config struct {
	Root        string `mapstructure:"root" docs:"/var/tmp/reva/;Path of root directory for user storage."`
	DisableHome bool   `mapstructure:"disable_home" docs:"false;Whether to not have individual home directories for users."`
	UserLayout  string `mapstructure:"user_layout" docs:"{{.Username}};Template for user home directories"`
	ShareFolder string `mapstructure:"share_folder" docs:"/MyShares;Path for storing share references."`
	Uploads     string `mapstructure:"uploads"`
	RecycleBin  string `mapstructure:"recycle_bin"`
	Versions    string `mapstructure:"versions"`
	Shadow      string `mapstructure:"shadow"`
	References  string `mapstructure:"references"`
}

func (c *config) init() {
	if c.Root == "" {
		c.Root = "/var/tmp/reva/data"
	}

	if c.UserLayout == "" {
		c.UserLayout = "{{.Username}}"
	}

	if c.ShareFolder == "" {
		c.ShareFolder = "/MyShares"
	}

	// ensure share folder always starts with slash
	c.ShareFolder = path.Join("/", c.ShareFolder)

	c.Uploads = path.Join(c.Root, ".uploads")
	c.Shadow = path.Join(c.Root, ".shadow")

	c.References = path.Join(c.Shadow, "references")
	c.RecycleBin = path.Join(c.Shadow, "recycle_bin")
	c.Versions = path.Join(c.Shadow, "versions")

}

type localfs struct {
	conf *config
	db   *sql.DB
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	c.init()

	namespaces := []string{c.Root, c.Uploads, c.Shadow, c.References, c.RecycleBin, c.Versions}

	// create namespaces if they do not exist
	for _, v := range namespaces {
		if err := os.MkdirAll(v, 0755); err != nil {
			return nil, errors.Wrap(err, "could not create home dir "+v)
		}
	}

	db, err := initializeDB(c.Root)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error initializing db")
	}

	return &localfs{conf: c, db: db}, nil
}

func (fs *localfs) Shutdown(ctx context.Context) error {
	err := fs.db.Close()
	if err != nil {
		return errors.Wrap(err, "localfs: error closing db connection")
	}
	return nil
}

func (fs *localfs) resolve(ctx context.Context, ref *provider.Reference) (string, error) {
	if ref.GetPath() != "" {
		return ref.GetPath(), nil
	}

	if ref.GetId() != nil {
		return fs.GetPathByID(ctx, ref.GetId())
	}

	// reference is invalid
	return "", fmt.Errorf("local: invalid reference %+v", ref)
}

func getUser(ctx context.Context) (*userpb.User, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired(""), "local: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

func (fs *localfs) wrap(ctx context.Context, p string) string {
	var internal string
	if !fs.conf.DisableHome {
		layout, err := fs.GetHome(ctx)
		if err != nil {
			panic(err)
		}
		internal = path.Join(fs.conf.Root, layout, p)
	} else {
		internal = path.Join(fs.conf.Root, p)
	}
	return internal
}

func (fs *localfs) wrapReferences(ctx context.Context, p string) string {
	var internal string
	if !fs.conf.DisableHome {
		layout, err := fs.GetHome(ctx)
		if err != nil {
			panic(err)
		}
		internal = path.Join(fs.conf.References, layout, "home", p)
	} else {
		internal = path.Join(fs.conf.References, "home", p)
	}
	return internal
}

func (fs *localfs) wrapRecycleBin(ctx context.Context, p string) string {
	var internal string
	if !fs.conf.DisableHome {
		layout, err := fs.GetHome(ctx)
		if err != nil {
			panic(err)
		}
		internal = path.Join(fs.conf.RecycleBin, layout, p)
	} else {
		internal = path.Join(fs.conf.RecycleBin, p)
	}
	return internal
}

func (fs *localfs) wrapVersions(ctx context.Context, p string) string {
	var internal string
	if !fs.conf.DisableHome {
		layout, err := fs.GetHome(ctx)
		if err != nil {
			panic(err)
		}
		internal = path.Join(fs.conf.Versions, layout, p)
	} else {
		internal = path.Join(fs.conf.Versions, p)
	}
	return internal
}

func (fs *localfs) unwrap(ctx context.Context, np string) string {
	ns := fs.getNsMatch(np, []string{fs.conf.Root, fs.conf.References, fs.conf.RecycleBin, fs.conf.Versions})
	var external string
	if !fs.conf.DisableHome {
		layout, err := fs.GetHome(ctx)
		if err != nil {
			panic(err)
		}
		ns = path.Join(ns, layout)
	}

	external = strings.TrimPrefix(np, ns)
	if external == "" {
		external = "/"
	}
	return external
}

func (fs *localfs) getNsMatch(internal string, nss []string) string {
	var match string
	for _, ns := range nss {
		if strings.HasPrefix(internal, ns) && len(ns) > len(match) {
			match = ns
		}
	}
	if match == "" {
		panic(fmt.Sprintf("local: path is outside namespaces: path=%s namespaces=%+v", internal, nss))
	}

	return match
}

func (fs *localfs) isShareFolder(ctx context.Context, p string) bool {
	return strings.HasPrefix(p, fs.conf.ShareFolder)
}

func (fs *localfs) isShareFolderRoot(ctx context.Context, p string) bool {
	return path.Clean(p) == fs.conf.ShareFolder
}

func (fs *localfs) isShareFolderChild(ctx context.Context, p string) bool {
	p = path.Clean(p)
	vals := strings.Split(p, fs.conf.ShareFolder+"/")
	return len(vals) > 1 && vals[1] != ""
}

func (fs *localfs) normalize(ctx context.Context, fi os.FileInfo, fn string) *provider.ResourceInfo {
	fp := fs.unwrap(ctx, path.Join("/", fn))
	owner, err := getUser(ctx)
	if err != nil {
		return nil
	}
	metadata, err := fs.retrieveArbitraryMetadata(ctx, fn)
	if err != nil {
		return nil
	}

	// A fileid is constructed like `fileid-url_encoded_path`. See GetPathByID for the inverse conversion
	md := &provider.ResourceInfo{
		Id:            &provider.ResourceId{OpaqueId: "fileid-" + url.QueryEscape(fp)},
		Path:          fp,
		Type:          getResourceType(fi.IsDir()),
		Etag:          calcEtag(ctx, fi),
		MimeType:      mime.Detect(fi.IsDir(), fp),
		Size:          uint64(fi.Size()),
		PermissionSet: &provider.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Mtime: &types.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
		},
		Owner:             owner.Id,
		ArbitraryMetadata: metadata,
	}

	return md
}

func (fs *localfs) convertToFileReference(ctx context.Context, fi os.FileInfo, fn string) *provider.ResourceInfo {
	info := fs.normalize(ctx, fi, fn)
	info.Type = provider.ResourceType_RESOURCE_TYPE_REFERENCE
	target, err := fs.getReferenceEntry(ctx, fn)
	if err != nil {
		return nil
	}
	info.Target = target
	return info
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

func (fs *localfs) retrieveArbitraryMetadata(ctx context.Context, fn string) (*provider.ArbitraryMetadata, error) {
	md, err := fs.getMetadata(ctx, fn)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error listing metadata")
	}
	var mdKey, mdVal string
	metadata := provider.ArbitraryMetadata{
		Metadata: map[string]string{},
	}

	for md.Next() {
		err = md.Scan(&mdKey, &mdVal)
		if err != nil {
			return nil, errors.Wrap(err, "localfs: error scanning db rows")
		}
		metadata.Metadata[mdKey] = mdVal
	}
	return &metadata, nil
}

// GetPathByID returns the path pointed by the file id
// In this implementation the file id is in the form `fileid-url_encoded_path`
func (fs *localfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	return url.QueryUnescape(strings.TrimPrefix(id.OpaqueId, "fileid-"))
}

func role2CS3Permissions(mode string) *provider.ResourcePermissions {

	// TODO also check unix permissions for read access
	p := &provider.ResourcePermissions{}
	// r
	if strings.Contains(mode, "r") {
		p.Stat = true
		p.InitiateFileDownload = true
	}
	// w
	if strings.Contains(mode, "w") {
		p.CreateContainer = true
		p.InitiateFileUpload = true
		p.Delete = true
		if p.InitiateFileDownload {
			p.Move = true
		}
	}
	if strings.Contains(mode, "wo") {
		p.CreateContainer = true
		//	p.InitiateFileUpload = false // TODO only when the file exists
		p.Delete = false
	}
	if strings.Contains(mode, "!d") {
		p.Delete = false
	} else if strings.Contains(mode, "+d") {
		p.Delete = true
	}
	// x
	if strings.Contains(mode, "x") {
		p.ListContainer = true
	}

	return p
}

func cs3Permissions2Role(set *provider.ResourcePermissions) (string, error) {
	var b strings.Builder

	if set.Stat || set.InitiateFileDownload {
		b.WriteString("r")
	}
	if set.CreateContainer || set.InitiateFileUpload || set.Delete || set.Move {
		b.WriteString("w")
	}
	if set.ListContainer {
		b.WriteString("x")
	}

	if set.Delete {
		b.WriteString("+d")
	} else {
		b.WriteString("!d")
	}

	return b.String(), nil
}

func (fs *localfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}
	fn = fs.wrap(ctx, fn)

	role, err := cs3Permissions2Role(g.Permissions)
	if err != nil {
		return errors.Wrap(err, "localfs: unknown set permissions")
	}

	var grantee string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		grantee = fmt.Sprintf("g:%s@%s", g.Grantee.Id.OpaqueId, g.Grantee.Id.Idp)
	} else {
		grantee = fmt.Sprintf("u:%s@%s", g.Grantee.Id.OpaqueId, g.Grantee.Id.Idp)
	}

	err = fs.addToACLDB(ctx, fn, grantee, role)
	if err != nil {
		return errors.Wrap(err, "localfs: error adding entry to DB")
	}

	return fs.propagate(ctx, fn)
}

func (fs *localfs) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}
	fn = fs.wrap(ctx, fn)

	grants, err := fs.getACLs(ctx, fn)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error listing grants")
	}
	var granteeID, role string
	var grantList []*provider.Grant

	for grants.Next() {
		err = grants.Scan(&granteeID, &role)
		if err != nil {
			return nil, errors.Wrap(err, "localfs: error scanning db rows")
		}
		grantee := &provider.Grantee{
			Id:   &userpb.UserId{OpaqueId: granteeID[2:]},
			Type: fs.getGranteeType(string(granteeID[0])),
		}
		permissions := role2CS3Permissions(role)

		grantList = append(grantList, &provider.Grant{
			Grantee:     grantee,
			Permissions: permissions,
		})
	}
	return grantList, nil

}

func (fs *localfs) getGranteeType(granteeType string) provider.GranteeType {
	if granteeType == "g" {
		return provider.GranteeType_GRANTEE_TYPE_GROUP
	}
	return provider.GranteeType_GRANTEE_TYPE_USER
}

func (fs *localfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}
	fn = fs.wrap(ctx, fn)

	var grantee string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		grantee = fmt.Sprintf("g:%s@%s", g.Grantee.Id.OpaqueId, g.Grantee.Id.Idp)
	} else {
		grantee = fmt.Sprintf("u:%s@%s", g.Grantee.Id.OpaqueId, g.Grantee.Id.Idp)
	}

	err = fs.removeFromACLDB(ctx, fn, grantee)
	if err != nil {
		return errors.Wrap(err, "localfs: error removing from DB")
	}

	return fs.propagate(ctx, fn)
}

func (fs *localfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return fs.AddGrant(ctx, ref, g)
}

func (fs *localfs) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

func (fs *localfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	if !fs.isShareFolder(ctx, path) {
		return errtypes.PermissionDenied("localfs: cannot create references outside the share folder")
	}

	fn := fs.wrapReferences(ctx, path)

	err := os.MkdirAll(fn, 0700)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fn)
		}
		return errors.Wrap(err, "localfs: error creating dir "+fn)
	}

	if err = fs.addToReferencesDB(ctx, fn, targetURI.String()); err != nil {
		return errors.Wrap(err, "localfs: error adding entry to DB")
	}

	return fs.propagate(ctx, fn)
}

func (fs *localfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {

	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}

	if fs.isShareFolderRoot(ctx, np) {
		return errtypes.PermissionDenied("localfs: cannot set metadata for the virtual share folder")
	}

	if fs.isShareFolderChild(ctx, np) {
		np = fs.wrapReferences(ctx, np)
	} else {
		np = fs.wrap(ctx, np)
	}

	fi, err := os.Stat(np)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fs.unwrap(ctx, np))
		}
		return errors.Wrap(err, "localfs: error stating "+np)
	}

	if md.Metadata != nil {
		if val, ok := md.Metadata["mtime"]; ok {
			if mtime, err := parseMTime(val); err == nil {
				// updating mtime also updates atime
				if err := os.Chtimes(np, mtime, mtime); err != nil {
					return errors.Wrap(err, "could not set mtime")
				}
			} else {
				return errors.Wrap(err, "could not parse mtime")
			}
			delete(md.Metadata, "mtime")
		}

		if _, ok := md.Metadata["etag"]; ok {
			etag := calcEtag(ctx, fi)
			if etag != md.Metadata["etag"] {
				err = fs.addToMetadataDB(ctx, np, "etag", etag)
				if err != nil {
					return errors.Wrap(err, "localfs: error adding entry to DB")
				}
			}
			delete(md.Metadata, "etag")
		}

		if _, ok := md.Metadata["favorite"]; ok {
			u, err := getUser(ctx)
			if err != nil {
				return err
			}
			if uid := u.GetId(); uid != nil {
				usr := fmt.Sprintf("u:%s@%s", uid.GetOpaqueId(), uid.GetIdp())
				if err = fs.addToFavoritesDB(ctx, np, usr); err != nil {
					return errors.Wrap(err, "localfs: error adding entry to DB")
				}
			} else {
				return errors.Wrap(errtypes.UserRequired("userrequired"), "user has no id")
			}
			delete(md.Metadata, "favorite")
		}
	}

	for k, v := range md.Metadata {
		err = fs.addToMetadataDB(ctx, np, k, v)
		if err != nil {
			return errors.Wrap(err, "localfs: error adding entry to DB")
		}
	}

	return fs.propagate(ctx, np)
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

func (fs *localfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {

	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}

	if fs.isShareFolderRoot(ctx, np) {
		return errtypes.PermissionDenied("localfs: cannot set metadata for the virtual share folder")
	}

	if fs.isShareFolderChild(ctx, np) {
		np = fs.wrapReferences(ctx, np)
	} else {
		np = fs.wrap(ctx, np)
	}

	_, err = os.Stat(np)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fs.unwrap(ctx, np))
		}
		return errors.Wrap(err, "localfs: error stating "+np)
	}

	for _, k := range keys {
		switch k {
		case "favorite":
			u, err := getUser(ctx)
			if err != nil {
				return err
			}
			if uid := u.GetId(); uid != nil {
				usr := fmt.Sprintf("u:%s@%s", uid.GetOpaqueId(), uid.GetIdp())
				if err = fs.removeFromFavoritesDB(ctx, np, usr); err != nil {
					return errors.Wrap(err, "localfs: error removing entry from DB")
				}
			} else {
				return errors.Wrap(errtypes.UserRequired("userrequired"), "user has no id")
			}
		case "etag":
			return errors.Wrap(errtypes.NotSupported("unsetting etag not supported"), "could not unset metadata")
		case "mtime":
			return errors.Wrap(errtypes.NotSupported("unsetting mtime not supported"), "could not unset metadata")
		default:
			err = fs.removeFromMetadataDB(ctx, np, k)
			if err != nil {
				return errors.Wrap(err, "localfs: error adding entry to DB")
			}
		}
	}

	return fs.propagate(ctx, np)
}

func (fs *localfs) GetHome(ctx context.Context) (string, error) {
	if fs.conf.DisableHome {
		return "", errtypes.NotSupported("local: get home not supported")
	}

	u, err := getUser(ctx)
	if err != nil {
		err = errors.Wrap(err, "local: wrap: no user in ctx and home is enabled")
		return "", err
	}
	relativeHome := templates.WithUser(u, fs.conf.UserLayout)

	return relativeHome, nil
}

func (fs *localfs) CreateHome(ctx context.Context) error {
	if fs.conf.DisableHome {
		return errtypes.NotSupported("localfs: create home not supported")
	}

	homePaths := []string{fs.wrap(ctx, "/"), fs.wrapRecycleBin(ctx, "/"), fs.wrapVersions(ctx, "/"), fs.wrapReferences(ctx, fs.conf.ShareFolder)}

	for _, v := range homePaths {
		if err := fs.createHomeInternal(ctx, v); err != nil {
			return errors.Wrap(err, "local: error creating home dir "+v)
		}
	}

	return nil
}

func (fs *localfs) createHomeInternal(ctx context.Context, fn string) error {
	_, err := os.Stat(fn)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "local: error stating:"+fn)
		}
	}
	err = os.MkdirAll(fn, 0700)
	if err != nil {
		return errors.Wrap(err, "local: error creating dir:"+fn)
	}
	return nil
}

func (fs *localfs) CreateDir(ctx context.Context, fn string) error {

	if fs.isShareFolder(ctx, fn) {
		return errtypes.PermissionDenied("localfs: cannot create folder under the share folder")
	}

	fn = fs.wrap(ctx, fn)
	if _, err := os.Stat(fn); err == nil {
		return errtypes.AlreadyExists(fn)
	}
	err := os.Mkdir(fn, 0700)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fn)
		}
		return errors.Wrap(err, "localfs: error creating dir "+fn)
	}
	return nil
}

func (fs *localfs) Delete(ctx context.Context, ref *provider.Reference) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}

	if fs.isShareFolderRoot(ctx, fn) {
		return errtypes.PermissionDenied("localfs: cannot delete the virtual share folder")
	}

	var fp string
	if fs.isShareFolderChild(ctx, fn) {
		fp = fs.wrapReferences(ctx, fn)
	} else {
		fp = fs.wrap(ctx, fn)
	}

	_, err = os.Stat(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fn)
		}
		return errors.Wrap(err, "localfs: error stating "+fp)
	}

	key := fmt.Sprintf("%s.d%d", path.Base(fn), time.Now().UnixNano()/int64(time.Millisecond))
	if err := os.Rename(fp, fs.wrapRecycleBin(ctx, key)); err != nil {
		return errors.Wrap(err, "localfs: could not delete item")
	}

	err = fs.addToRecycledDB(ctx, key, fn)
	if err != nil {
		return errors.Wrap(err, "localfs: error adding entry to DB")
	}

	return fs.propagate(ctx, path.Dir(fp))
}

func (fs *localfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	oldName, err := fs.resolve(ctx, oldRef)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}

	newName, err := fs.resolve(ctx, newRef)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}

	if fs.isShareFolder(ctx, oldName) || fs.isShareFolder(ctx, newName) {
		return fs.moveReferences(ctx, oldName, newName)
	}

	oldName = fs.wrap(ctx, oldName)
	newName = fs.wrap(ctx, newName)

	if err := os.Rename(oldName, newName); err != nil {
		return errors.Wrap(err, "localfs: error moving "+oldName+" to "+newName)
	}

	if err := fs.copyMD(oldName, newName); err != nil {
		return errors.Wrap(err, "localfs: error copying metadata")
	}

	if err := fs.propagate(ctx, newName); err != nil {
		return err
	}
	if err := fs.propagate(ctx, path.Dir(oldName)); err != nil {
		return err
	}

	return nil
}

func (fs *localfs) moveReferences(ctx context.Context, oldName, newName string) error {

	if fs.isShareFolderRoot(ctx, oldName) || fs.isShareFolderRoot(ctx, newName) {
		return errtypes.PermissionDenied("localfs: cannot move/rename the virtual share folder")
	}

	// only rename of the reference is allowed, hence having the same basedir
	bold, _ := path.Split(oldName)
	bnew, _ := path.Split(newName)

	if bold != bnew {
		return errtypes.PermissionDenied("localfs: cannot move references under the virtual share folder")
	}

	oldName = fs.wrapReferences(ctx, oldName)
	newName = fs.wrapReferences(ctx, newName)

	if err := os.Rename(oldName, newName); err != nil {
		return errors.Wrap(err, "localfs: error moving "+oldName+" to "+newName)
	}

	if err := fs.copyMD(oldName, newName); err != nil {
		return errors.Wrap(err, "localfs: error copying metadata")
	}

	if err := fs.propagate(ctx, newName); err != nil {
		return err
	}
	if err := fs.propagate(ctx, path.Dir(oldName)); err != nil {
		return err
	}

	return nil
}

func (fs *localfs) GetMD(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	if fs.isShareFolder(ctx, fn) {
		return fs.getMDShareFolder(ctx, fn)
	}

	fn = fs.wrap(ctx, fn)
	md, err := os.Stat(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error stating "+fn)
	}

	return fs.normalize(ctx, md, fn), nil
}

func (fs *localfs) getMDShareFolder(ctx context.Context, p string) (*provider.ResourceInfo, error) {

	fn := fs.wrapReferences(ctx, p)
	md, err := os.Stat(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error stating "+fn)
	}

	if fs.isShareFolderRoot(ctx, p) {
		return fs.normalize(ctx, md, fn), nil
	}
	return fs.convertToFileReference(ctx, md, fn), nil
}

func (fs *localfs) ListFolder(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	if fn == "/" {
		homeFiles, err := fs.listHome(ctx, fn)
		if err != nil {
			return nil, err
		}
		sharedReferences, err := fs.listShareFolderRoot(ctx, fn)
		if err != nil {
			return nil, err
		}
		return append(homeFiles, sharedReferences...), nil
	}

	if fs.isShareFolderRoot(ctx, fn) {
		return fs.listShareFolderRoot(ctx, fn)
	}

	if fs.isShareFolderChild(ctx, fn) {
		return nil, errtypes.PermissionDenied("localfs: error listing folders inside the shared folder, only file references are stored inside")
	}

	return fs.listHome(ctx, fn)
}

func (fs *localfs) listHome(ctx context.Context, home string) ([]*provider.ResourceInfo, error) {

	fn := fs.wrap(ctx, home)

	mds, err := ioutil.ReadDir(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error listing "+fn)
	}

	finfos := []*provider.ResourceInfo{}
	for _, md := range mds {
		finfos = append(finfos, fs.normalize(ctx, md, path.Join(fn, md.Name())))
	}
	return finfos, nil
}

func (fs *localfs) listShareFolderRoot(ctx context.Context, home string) ([]*provider.ResourceInfo, error) {

	fn := fs.wrapReferences(ctx, home)

	mds, err := ioutil.ReadDir(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error listing "+fn)
	}

	finfos := []*provider.ResourceInfo{}
	for _, md := range mds {
		var info *provider.ResourceInfo
		if fs.isShareFolderRoot(ctx, path.Join("/", md.Name())) {
			info = fs.normalize(ctx, md, path.Join(fn, md.Name()))
		} else {
			info = fs.convertToFileReference(ctx, md, path.Join(fn, md.Name()))
		}
		finfos = append(finfos, info)
	}
	return finfos, nil
}

func (fs *localfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "error resolving ref")
	}

	// we cannot rely on /tmp as it can live in another partition and we can
	// hit invalid cross-device link errors, so we create the tmp file in the same directory
	// the file is supposed to be written.
	tmp, err := ioutil.TempFile(path.Dir(fn), "._reva_atomic_upload")
	if err != nil {
		return errors.Wrap(err, "localfs: error creating tmp fn at "+path.Dir(fn))
	}

	_, err = io.Copy(tmp, r)
	if err != nil {
		return errors.Wrap(err, "localfs: eror writing to tmp file "+tmp.Name())
	}

	// TODO(labkode): make sure rename is atomic, missing fsync ...
	if err := os.Rename(tmp.Name(), fn); err != nil {
		return errors.Wrap(err, "localfs: error renaming from "+tmp.Name()+" to "+fn)
	}

	return nil
}

func (fs *localfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64) (uploadID string, err error) {
	return "", errtypes.NotSupported("localfs: inititate file upload")
}

func (fs *localfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	if fs.isShareFolder(ctx, fn) {
		return nil, errtypes.PermissionDenied("localfs: cannot download under the virtual share folder")
	}

	fn = fs.wrap(ctx, fn)
	r, err := os.Open(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error reading "+fn)
	}
	return r, nil
}

func (fs *localfs) archiveRevision(ctx context.Context, np string) error {

	versionsDir := fs.wrapVersions(ctx, fs.unwrap(ctx, np))
	if err := os.MkdirAll(versionsDir, 0700); err != nil {
		return errors.Wrap(err, "localfs: error creating file versions dir "+versionsDir)
	}

	vp := path.Join(versionsDir, fmt.Sprintf("v%d", time.Now().UnixNano()/int64(time.Millisecond)))
	if err := os.Rename(np, vp); err != nil {
		return errors.Wrap(err, "localfs: error renaming from "+np+" to "+vp)
	}

	return nil
}

func (fs *localfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	if fs.isShareFolder(ctx, np) {
		return nil, errtypes.PermissionDenied("localfs: cannot list revisions under the virtual share folder")
	}

	versionsDir := fs.wrapVersions(ctx, np)
	revisions := []*provider.FileVersion{}
	mds, err := ioutil.ReadDir(versionsDir)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error reading"+versionsDir)
	}

	for i := range mds {
		// versions resemble v12345678
		version := mds[i].Name()[1:]

		mtime, err := strconv.Atoi(version)
		if err != nil {
			continue
		}
		revisions = append(revisions, &provider.FileVersion{
			Key:   version,
			Size:  uint64(mds[i].Size()),
			Mtime: uint64(mtime),
		})
	}
	return revisions, nil
}

func (fs *localfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	if fs.isShareFolder(ctx, np) {
		return nil, errtypes.PermissionDenied("localfs: cannot download revisions under the virtual share folder")
	}

	versionsDir := fs.wrapVersions(ctx, np)
	vp := path.Join(versionsDir, revisionKey)

	r, err := os.Open(vp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(vp)
		}
		return nil, errors.Wrap(err, "localfs: error reading "+vp)
	}

	return r, nil
}

func (fs *localfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	np, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}

	if fs.isShareFolder(ctx, np) {
		return errtypes.PermissionDenied("localfs: cannot restore revisions under the virtual share folder")
	}

	versionsDir := fs.wrapVersions(ctx, np)
	vp := path.Join(versionsDir, revisionKey)
	np = fs.wrap(ctx, np)

	// check revision exists
	vs, err := os.Stat(vp)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(revisionKey)
		}
		return errors.Wrap(err, "localfs: error stating "+vp)
	}

	if !vs.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", vp)
	}

	if err := fs.archiveRevision(ctx, np); err != nil {
		return err
	}

	if err := os.Rename(vp, np); err != nil {
		return errors.Wrap(err, "localfs: error renaming from "+vp+" to "+np)
	}

	return fs.propagate(ctx, np)
}

func (fs *localfs) PurgeRecycleItem(ctx context.Context, key string) error {
	rp := fs.wrapRecycleBin(ctx, key)

	if err := os.Remove(rp); err != nil {
		return errors.Wrap(err, "localfs: error deleting recycle item")
	}
	return nil
}

func (fs *localfs) EmptyRecycle(ctx context.Context) error {
	rp := fs.wrapRecycleBin(ctx, "/")

	if err := os.RemoveAll(rp); err != nil {
		return errors.Wrap(err, "localfs: error deleting recycle files")
	}
	if err := fs.createHomeInternal(ctx, rp); err != nil {
		return errors.Wrap(err, "localfs: error deleting recycle files")
	}
	return nil
}

func (fs *localfs) convertToRecycleItem(ctx context.Context, rp string, md os.FileInfo) *provider.RecycleItem {
	// trashbin items have filename.txt.d12345678
	suffix := path.Ext(md.Name())
	if len(suffix) == 0 || !strings.HasPrefix(suffix, ".d") {
		return nil
	}

	trashtime := suffix[2:]
	ttime, err := strconv.Atoi(trashtime)
	if err != nil {
		return nil
	}

	filePath, err := fs.getRecycledEntry(ctx, md.Name())
	if err != nil {
		return nil
	}

	return &provider.RecycleItem{
		Type: getResourceType(md.IsDir()),
		Key:  md.Name(),
		Path: filePath,
		Size: uint64(md.Size()),
		DeletionTime: &types.Timestamp{
			Seconds: uint64(ttime),
		},
	}
}

func (fs *localfs) ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error) {

	rp := fs.wrapRecycleBin(ctx, "/")

	mds, err := ioutil.ReadDir(rp)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error listing deleted files")
	}
	items := []*provider.RecycleItem{}
	for i := range mds {
		ri := fs.convertToRecycleItem(ctx, rp, mds[i])
		if ri != nil {
			items = append(items, ri)
		}
	}
	return items, nil
}

func (fs *localfs) RestoreRecycleItem(ctx context.Context, restoreKey string) error {

	suffix := path.Ext(restoreKey)
	if len(suffix) == 0 || !strings.HasPrefix(suffix, ".d") {
		return errors.New("localfs: invalid trash item suffix")
	}

	filePath, err := fs.getRecycledEntry(ctx, restoreKey)
	if err != nil {
		return errors.Wrap(err, "localfs: invalid key")
	}

	var originalPath string
	if fs.isShareFolder(ctx, filePath) {
		originalPath = fs.wrapReferences(ctx, filePath)
	} else {
		originalPath = fs.wrap(ctx, filePath)
	}

	if _, err = os.Stat(originalPath); err == nil {
		return errors.New("localfs: can't restore - file already exists at original path")
	}

	rp := fs.wrapRecycleBin(ctx, restoreKey)
	if _, err = os.Stat(rp); err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(restoreKey)
		}
		return errors.Wrap(err, "localfs: error stating "+rp)
	}

	if err := os.Rename(rp, originalPath); err != nil {
		return errors.Wrap(err, "ocfs: could not restore item")
	}

	err = fs.removeFromRecycledDB(ctx, restoreKey)
	if err != nil {
		return errors.Wrap(err, "localfs: error adding entry to DB")
	}

	return fs.propagate(ctx, originalPath)
}

func (fs *localfs) propagate(ctx context.Context, leafPath string) error {

	var root string
	if fs.isShareFolderChild(ctx, leafPath) {
		root = fs.wrapReferences(ctx, "/")
	} else {
		root = fs.wrap(ctx, "/")
	}

	if !strings.HasPrefix(leafPath, root) {
		return errors.New("internal path outside root")
	}

	fi, err := os.Stat(leafPath)
	if err != nil {
		return err
	}

	parts := strings.Split(strings.TrimPrefix(leafPath, root), "/")
	// root never ents in / so the split returns an empty first element, which we can skip
	// we do not need to chmod the last element because it is the leaf path (< and not <= comparison)
	for i := 1; i < len(parts); i++ {
		if err := os.Chtimes(root, fi.ModTime(), fi.ModTime()); err != nil {
			return err
		}
		root = path.Join(root, parts[i])
	}
	return nil
}
