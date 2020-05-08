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

	// Provides sqlite drivers
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	registry.Register("local", New)
}

type config struct {
	Root       string `mapstructure:"root"`
	EnableHome bool   `mapstructure:"enable_home"`
	UserLayout string `mapstructure:"user_layout"`
	Uploads    string `mapstructure:"uploads"`
	RecycleBin string `mapstructure:"recycle_bin"`
	Versions   string `mapstructure:"versions"`
	Shadow     string `mapstructure:"shadow"`
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

func initializeDB(root string) (*sql.DB, error) {
	dbPath := path.Join(root, "localfs.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error opening DB connection")
	}

	stmt, err := db.Prepare("CREATE TABLE IF NOT EXISTS recycled_entries (key TEXT PRIMARY KEY, path TEXT)")
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec()
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error executing create statement")
	}

	stmt, err = db.Prepare("CREATE TABLE IF NOT EXISTS acl (resource TEXT, grantee TEXT, role TEXT) PRIMARY KEY (resource, grantee)")
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec()
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error executing create statement")
	}

	return db, nil
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	// defaults for Root
	if c.Root == "" {
		c.Root = "/var/tmp/reva/"
	}

	if c.UserLayout == "" {
		c.UserLayout = "{{.Username}}"
	}

	c.Uploads = path.Join(c.Root, ".uploads")
	c.RecycleBin = path.Join(c.Root, ".recycle_bin")
	c.Versions = path.Join(c.Root, ".versions")
	c.Shadow = path.Join(c.Root, ".shadow")

	namespaces := []string{c.Root, c.Uploads, c.RecycleBin, c.Versions, c.Shadow}

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
		return fs.wrap(ctx, ref.GetPath()), nil
	}

	if ref.GetId() != nil {
		fn := path.Join("/", strings.TrimPrefix(ref.GetId().OpaqueId, "fileid-"))
		fn = fs.wrap(ctx, fn)
		return fn, nil
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

func (fs *localfs) addToRecycledDB(ctx context.Context, key, fileName string) error {
	stmt, err := fs.db.Prepare("INSERT INTO recycled_entries VALUES (?, ?)")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(key, fileName)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing insert statement")
	}
	return nil
}

func (fs *localfs) removeFromRecycledDB(ctx context.Context, key string) error {
	stmt, err := fs.db.Prepare("DELETE FROM recycled_entries WHERE key=?")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(key)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing delete statement")
	}
	return nil
}

func (fs *localfs) addToACLDB(ctx context.Context, resource, grantee, role string) error {
	stmt, err := fs.db.Prepare("INSERT INTO acl (resource, grantee, role) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE role=?")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(resource, grantee, role, role)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing insert statement")
	}
	return nil
}

func (fs *localfs) removeFromACLDB(ctx context.Context, resource, grantee string) error {
	stmt, err := fs.db.Prepare("DELETE FROM acl WHERE resource=? AND grantee=?")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(resource, grantee)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing delete statement")
	}
	return nil
}

func (fs *localfs) wrap(ctx context.Context, p string) string {
	var internal string
	if fs.conf.EnableHome {
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

func (fs *localfs) wrapRecycleBin(ctx context.Context, p string) string {
	var internal string
	if fs.conf.EnableHome {
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

func (fs *localfs) wrapShadow(ctx context.Context, p string) string {
	var internal string
	if fs.conf.EnableHome {
		layout, err := fs.GetHome(ctx)
		if err != nil {
			panic(err)
		}
		internal = path.Join(fs.conf.Shadow, layout, p)
	} else {
		internal = path.Join(fs.conf.Shadow, p)
	}
	return internal
}

func (fs *localfs) wrapVersions(ctx context.Context, p string) string {
	var internal string
	if fs.conf.EnableHome {
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
	ns := fs.getNsMatch(np, []string{fs.conf.Root, fs.conf.RecycleBin, fs.conf.Shadow, fs.conf.Versions})
	var external string
	if fs.conf.EnableHome {
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

func (fs *localfs) normalize(ctx context.Context, fi os.FileInfo, fn string) *provider.ResourceInfo {
	fn = fs.unwrap(ctx, path.Join("/", fn))
	md := &provider.ResourceInfo{
		Id:            &provider.ResourceId{OpaqueId: "fileid-" + strings.TrimPrefix(fn, "/")},
		Path:          fn,
		Type:          getResourceType(fi.IsDir()),
		Etag:          calcEtag(ctx, fi),
		MimeType:      mime.Detect(fi.IsDir(), fn),
		Size:          uint64(fi.Size()),
		PermissionSet: &provider.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Mtime: &types.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
		},
	}

	return md
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

// GetPathByID returns the path pointed by the file id
// In this implementation the file id is that path of the file without the first slash
// thus the file id always points to the filename
func (fs *localfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	return path.Join("/", strings.TrimPrefix(id.OpaqueId, "fileid-")), nil
}

func Role2CS3Permissions(r string) (*provider.ResourcePermissions, error) {
	switch r {
	case "viewer":
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
		}, nil
	case "editor":
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,

			Move:               true,
			InitiateFileUpload: true,
			RestoreFileVersion: true,
			RestoreRecycleItem: true,
			CreateContainer:    true,
			Delete:             true,
			PurgeRecycle:       true,
		}, nil
	case "owner":
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,

			Move:               true,
			InitiateFileUpload: true,
			RestoreFileVersion: true,
			RestoreRecycleItem: true,
			CreateContainer:    true,
			Delete:             true,
			PurgeRecycle:       true,

			AddGrant:    true,
			RemoveGrant: true, // TODO when are you able to unshare / delete
			UpdateGrant: true,
		}, nil
	default:
		return nil, errtypes.NotSupported("localfs: role not defined")
	}
}

func CS3Permissions2Role(rp *provider.ResourcePermissions) (string, error) {
	switch {
	case rp.AddGrant:
		return "owner", nil
	case rp.Move:
		return "editor", nil
	case rp.ListContainer:
		return "viewer", nil
	default:
		return "", errtypes.NotSupported("localfs: role not defined")
	}
}

func (fs *localfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}

	role, err := CS3Permissions2Role(g.Permissions)
	if err != nil {
		return errors.Wrap(err, "localfs: unknown set permissions")
	}

	var grantee string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		grantee = "g:" + g.Grantee.Id.OpaqueId
	} else {
		grantee = "u:" + g.Grantee.Id.OpaqueId
	}

	err = fs.addToACLDB(ctx, fn, grantee, role)
	if err != nil {
		return errors.Wrap(err, "localfs: error adding entry to DB")
	}

	return nil
}

func (fs *localfs) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	grants, err := fs.db.Query("SELECT grantee, role FROM acl WHERE resource=?", fn)
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

		permissions, err := Role2CS3Permissions(role)
		if err != nil {
			return nil, errors.Wrap(err, "localfs: unknown role")
		}

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

	var grantee string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		grantee = "g:" + g.Grantee.Id.OpaqueId
	} else {
		grantee = "u:" + g.Grantee.Id.OpaqueId
	}

	err = fs.removeFromACLDB(ctx, fn, grantee)
	if err != nil {
		return errors.Wrap(err, "localfs: error removing from DB")
	}

	return nil
}

func (fs *localfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return fs.AddGrant(ctx, ref, g)
}

func (fs *localfs) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}
func (fs *localfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return errtypes.NotSupported("local: operation not supported")
}

func (fs *localfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	return errtypes.NotSupported("local: operation not supported")
}

func (fs *localfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	return errtypes.NotSupported("local: operation not supported")
}

func (fs *localfs) GetHome(ctx context.Context) (string, error) {
	if !fs.conf.EnableHome {
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
	if !fs.conf.EnableHome {
		return errtypes.NotSupported("eos: create home not supported")
	}

	homePaths := []string{fs.wrap(ctx, "/"), fs.wrapRecycleBin(ctx, "/"), fs.wrapVersions(ctx, "/"), fs.wrapShadow(ctx, "/")}

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

	_, err = os.Stat(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fs.unwrap(ctx, fn))
		}
		return errors.Wrap(err, "localfs: error stating "+fn)
	}

	fileName := fs.unwrap(ctx, fn)
	key := fmt.Sprintf("%s.d%d", path.Base(fileName), time.Now().Unix())
	if err := os.Rename(fn, fs.wrapRecycleBin(ctx, key)); err != nil {
		return errors.Wrap(err, "localfs: could not delete item")
	}

	err = fs.addToRecycledDB(ctx, key, fileName)
	if err != nil {
		return errors.Wrap(err, "localfs: error adding entry to DB")
	}

	return nil
}

func (fs *localfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	oldName, err := fs.resolve(ctx, oldRef)
	if err != nil {
		return errors.Wrap(err, "error resolving ref")
	}

	newName, err := fs.resolve(ctx, newRef)
	if err != nil {
		return errors.Wrap(err, "error resolving ref")
	}

	if err := os.Rename(oldName, newName); err != nil {
		return errors.Wrap(err, "localfs: error moving "+oldName+" to "+newName)
	}
	return nil
}

func (fs *localfs) GetMD(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "error resolving ref")
	}

	md, err := os.Stat(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error stating "+fn)
	}

	return fs.normalize(ctx, md, fn), nil
}

func (fs *localfs) ListFolder(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "error resolving ref")
	}

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

func (fs *localfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	r, err := os.Open(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error reading "+fn)
	}
	return r, nil
}

func (fs *localfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	return nil, errtypes.NotSupported("list revisions")
}

func (fs *localfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("download revision")
}

func (fs *localfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	return errtypes.NotSupported("restore revision")
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

	var path string
	err = fs.db.QueryRow("SELECT path FROM recycled_entries WHERE key=?", md.Name()).Scan(&path)
	if err != nil {
		return nil
	}

	return &provider.RecycleItem{
		Type: getResourceType(md.IsDir()),
		Key:  md.Name(),
		Path: path,
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

	var path string
	err := fs.db.QueryRow("SELECT path FROM recycled_entries WHERE key=?", restoreKey).Scan(&path)
	if err != nil {
		return errors.Wrap(err, "localfs: invalid key")
	}

	originalPath := fs.wrap(ctx, path)
	rp := fs.wrapRecycleBin(ctx, restoreKey)

	if err := os.Rename(rp, originalPath); err != nil {
		return errors.Wrap(err, "ocfs: could not restore item")
	}

	err = fs.removeFromRecycledDB(ctx, restoreKey)
	if err != nil {
		return errors.Wrap(err, "localfs: error adding entry to DB")
	}

	return nil
}
