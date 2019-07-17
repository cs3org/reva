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

package eos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	gouser "os/user"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/cs3org/reva/pkg/storage/fs/registry"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/eosclient"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/acl"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
)

func init() {
	registry.Register("eos", New)
}

var hiddenReg = regexp.MustCompile(`\.sys\..#.`)

type contextUserRequiredErr string

func (err contextUserRequiredErr) Error() string   { return string(err) }
func (err contextUserRequiredErr) IsUserRequired() {}

type eosStorage struct {
	c             *eosclient.Client
	mountpoint    string
	showHiddenSys bool
	conf          *config
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Options are the configuration options to pass to the New function.
type config struct {
	// Namespace for metadata operations
	Namespace string `mapstructure:"namespace"`

	// Location of the eos binary.
	// Default is /usr/bin/eos.
	EosBinary string `mapstructure:"eos_binary"`

	// Location of the xrdcopy binary.
	// Default is /usr/bin/xrdcopy.
	XrdcopyBinary string `mapstructure:"xrdcopy_binary"`

	// URL of the Master EOS MGM.
	// Default is root://eos-example.org
	MasterURL string `mapstructure:"master_url"`

	// URL of the Slave EOS MGM.
	// Default is root://eos-example.org
	SlaveURL string `mapstructure:"slave_url"`

	// Location on the local fs where to store reads.
	// Defaults to os.TempDir()
	CacheDirectory string `mapstructure:"cache_directory"`

	// Enables logging of the commands executed
	// Defaults to false
	EnableLogging bool `mapstructure:"enable_logging"`

	// ShowHiddenSysFiles shows internal EOS files like
	// .sys.v# and .sys.a# files.
	ShowHiddenSysFiles bool `mapstructure:"show_hidden_sys_files"`

	// ForceSingleUserMode will force connections to EOS to use SingleUsername
	ForceSingleUserMode bool `mapstructure:"force_single_user_mode"`

	// SingleUsername is the username to use when SingleUserMode is enabled
	SingleUsername string `mapstructure:"single_username"`
}

func getUser(ctx context.Context) (*authv0alphapb.User, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(contextUserRequiredErr("userrequired"), "storage_eos: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

func (c *config) init() {
	c.Namespace = path.Clean(c.Namespace)
	if !strings.HasPrefix(c.Namespace, "/") {
		c.Namespace = "/"
	}

	if c.EosBinary == "" {
		c.EosBinary = "/usr/bin/eos"
	}

	if c.XrdcopyBinary == "" {
		c.XrdcopyBinary = "/usr/bin/xrdcopy"
	}

	if c.MasterURL == "" {
		c.MasterURL = "root://eos-example.org"
	}

	if c.SlaveURL == "" {
		c.SlaveURL = c.MasterURL
	}

	if c.CacheDirectory == "" {
		c.CacheDirectory = os.TempDir()
	}
}

// New returns a new implementation of the storage.FS interface that connects to EOS.
func New(m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	eosClientOpts := &eosclient.Options{
		XrdcopyBinary:       c.XrdcopyBinary,
		URL:                 c.MasterURL,
		EosBinary:           c.EosBinary,
		CacheDirectory:      c.CacheDirectory,
		ForceSingleUserMode: c.ForceSingleUserMode,
		SingleUsername:      c.SingleUsername,
	}

	eosClient := eosclient.New(eosClientOpts)

	eosStorage := &eosStorage{
		c:             eosClient,
		mountpoint:    c.Namespace,
		showHiddenSys: c.ShowHiddenSysFiles,
		conf:          c,
	}

	return eosStorage, nil
}

func (fs *eosStorage) Shutdown(ctx context.Context) error {
	// TODO(labkode): in a grpc implementation we can close connections.
	return nil
}

func (fs *eosStorage) getInternalPath(ctx context.Context, fn string) string {
	internalPath := path.Join(fs.mountpoint, fn)
	return internalPath
}

func (fs *eosStorage) removeNamespace(ctx context.Context, np string) string {
	p := strings.TrimPrefix(np, fs.mountpoint)
	if p == "" {
		p = "/"
	}

	return p
}

func (fs *eosStorage) GetPathByID(ctx context.Context, id string) (string, error) {
	u, err := getUser(ctx)
	if err != nil {
		return "", errors.Wrap(err, "storage_eos: no user in ctx")
	}

	// parts[0] = 868317, parts[1] = photos, ...
	parts := strings.Split(id, "/")
	fileID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return "", errors.Wrap(err, "storage_eos: error parsing fileid string")
	}

	eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, u.Username, fileID)
	if err != nil {
		return "", errors.Wrap(err, "storage_eos: error getting file info by inode")
	}

	fi := fs.convertToResourceInfo(ctx, eosFileInfo)
	return fi.Path, nil
}

func (fs *eosStorage) AddGrant(ctx context.Context, fn string, g *storageproviderv0alphapb.Grant) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}

	fn = fs.getInternalPath(ctx, fn)

	eosACL, err := fs.getEosACL(g)
	if err != nil {
		return err
	}

	err = fs.c.AddACL(ctx, u.Username, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "storage_eos: error adding acl")
	}

	return nil
}

func getEosACLType(gt storageproviderv0alphapb.GranteeType) (string, error) {
	switch gt {
	case storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER:
		return "u", nil
	case storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP:
		return "g", nil
	default:
		return "", errors.New("no eos acl for grantee type: " + gt.String())
	}
}

// TODO(labkode): fine grained permission controls.
func getEosACLPerm(rp *storageproviderv0alphapb.ResourcePermissions) (string, error) {
	if rp.Delete {
		return "rwx!d", nil
	}

	return "rx", nil
}

func (fs *eosStorage) getEosACL(g *storageproviderv0alphapb.Grant) (*acl.Entry, error) {
	permissions, err := getEosACLPerm(g.Permissions)
	if err != nil {
		return nil, err
	}
	t, err := getEosACLType(g.Grantee.Type)
	if err != nil {
		return nil, err
	}
	eosACL := &acl.Entry{
		Qualifier:   g.Grantee.Id.OpaqueId,
		Permissions: permissions,
		Type:        t,
	}
	return eosACL, nil
}

func (fs *eosStorage) RemoveGrant(ctx context.Context, fn string, g *storageproviderv0alphapb.Grant) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}

	eosACLType, err := getEosACLType(g.Grantee.Type)
	if err != nil {
		return err
	}

	fn = fs.getInternalPath(ctx, fn)

	err = fs.c.RemoveACL(ctx, u.Username, fn, eosACLType, g.Grantee.Id.OpaqueId)
	if err != nil {
		return errors.Wrap(err, "storage_eos: error removing acl")
	}
	return nil
}

func (fs *eosStorage) UpdateGrant(ctx context.Context, fn string, g *storageproviderv0alphapb.Grant) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}

	eosACL, err := fs.getEosACL(g)
	if err != nil {
		return err
	}

	fn = fs.getInternalPath(ctx, fn)
	err = fs.c.AddACL(ctx, u.Username, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "storage_eos: error updating acl")
	}
	return nil
}

func (fs *eosStorage) ListGrants(ctx context.Context, fn string) ([]*storageproviderv0alphapb.Grant, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	fn = fs.getInternalPath(ctx, fn)
	acls, err := fs.c.ListACLs(ctx, u.Username, fn)
	if err != nil {
		return nil, err
	}

	grants := []*storageproviderv0alphapb.Grant{}
	for _, a := range acls {
		grantee := &storageproviderv0alphapb.Grantee{
			Id:   &typespb.UserId{OpaqueId: a.Qualifier},
			Type: fs.getGranteeType(a.Type),
		}
		grants = append(grants, &storageproviderv0alphapb.Grant{
			Grantee:     grantee,
			Permissions: fs.getGrantPermissionSet(a.Permissions),
		})
	}

	return grants, nil
}

func (fs *eosStorage) getGranteeType(aclType string) storageproviderv0alphapb.GranteeType {
	switch aclType {
	case "u":
		return storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER
	case "g":
		return storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP
	default:
		return storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_INVALID
	}
}

// TODO(labkode): add more fine grained controls.
func (fs *eosStorage) getGrantPermissionSet(mode string) *storageproviderv0alphapb.ResourcePermissions {
	// TODO(labkode) map other permissions.
	switch mode {
	case "rx":
		return &storageproviderv0alphapb.ResourcePermissions{
			ListContainer: true,
			//InitiateFileDownload: true,
			//Stat:          true,
		}
	case "rwx!d":
		return &storageproviderv0alphapb.ResourcePermissions{
			Move:            true,
			CreateContainer: true,
			ListContainer:   true,
		}
	default:
		// return no permissions are we do not know
		// what acl is this one.
		return &storageproviderv0alphapb.ResourcePermissions{} // default values are false
	}
}

func (fs *eosStorage) GetMD(ctx context.Context, fn string) (*storageproviderv0alphapb.ResourceInfo, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	fn = fs.getInternalPath(ctx, fn)
	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, u.Username, fn)
	if err != nil {
		return nil, err
	}
	fi := fs.convertToResourceInfo(ctx, eosFileInfo)
	return fi, nil
}

func (fs *eosStorage) ListFolder(ctx context.Context, fn string) ([]*storageproviderv0alphapb.ResourceInfo, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: no user in ctx")
	}

	fn = fs.getInternalPath(ctx, fn)
	eosFileInfos, err := fs.c.List(ctx, u.Username, fn)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: error listing")
	}

	finfos := []*storageproviderv0alphapb.ResourceInfo{}
	for _, eosFileInfo := range eosFileInfos {
		// filter out sys files
		if !fs.showHiddenSys {
			base := path.Base(eosFileInfo.File)
			if hiddenReg.MatchString(base) {
				continue
			}

		}
		finfos = append(finfos, fs.convertToResourceInfo(ctx, eosFileInfo))
	}
	return finfos, nil
}

func (fs *eosStorage) GetQuota(ctx context.Context) (int, int, error) {
	u, err := getUser(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "storage_eos: no user in ctx")
	}
	return fs.c.GetQuota(ctx, u.Username, fs.conf.Namespace)
}

func (fs *eosStorage) CreateDir(ctx context.Context, fn string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}
	fn = fs.getInternalPath(ctx, fn)
	return fs.c.CreateDir(ctx, u.Username, fn)
}

func (fs *eosStorage) Delete(ctx context.Context, fn string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}
	fn = fs.getInternalPath(ctx, fn)
	return fs.c.Remove(ctx, u.Username, fn)
}

func (fs *eosStorage) Move(ctx context.Context, oldPath, newPath string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}
	oldPath = fs.getInternalPath(ctx, oldPath)
	newPath = fs.getInternalPath(ctx, newPath)
	return fs.c.Rename(ctx, u.Username, oldPath, newPath)
}

func (fs *eosStorage) Download(ctx context.Context, fn string) (io.ReadCloser, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: no user in ctx")
	}
	fn = fs.getInternalPath(ctx, fn)
	return fs.c.Read(ctx, u.Username, fn)
}

func (fs *eosStorage) Upload(ctx context.Context, fn string, r io.ReadCloser) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}
	fn = fs.getInternalPath(ctx, fn)
	return fs.c.Write(ctx, u.Username, fn, r)
}

func (fs *eosStorage) ListRevisions(ctx context.Context, fn string) ([]*storageproviderv0alphapb.FileVersion, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: no user in ctx")
	}
	fn = fs.getInternalPath(ctx, fn)
	eosRevisions, err := fs.c.ListVersions(ctx, u.Username, fn)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: error listing versions")
	}
	revisions := []*storageproviderv0alphapb.FileVersion{}
	for _, eosRev := range eosRevisions {
		rev := fs.convertToRevision(ctx, eosRev)
		revisions = append(revisions, rev)
	}
	return revisions, nil
}

func (fs *eosStorage) DownloadRevision(ctx context.Context, fn, revisionKey string) (io.ReadCloser, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: no user in ctx")
	}
	fn = fs.getInternalPath(ctx, fn)
	return fs.c.ReadVersion(ctx, u.Username, fn, revisionKey)
}

func (fs *eosStorage) RestoreRevision(ctx context.Context, fn, revisionKey string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}
	fn = fs.getInternalPath(ctx, fn)
	return fs.c.RollbackToVersion(ctx, u.Username, fn, revisionKey)
}

func (fs *eosStorage) EmptyRecycle(ctx context.Context, fn string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}
	return fs.c.PurgeDeletedEntries(ctx, u.Username)
}

func (fs *eosStorage) ListRecycle(ctx context.Context, fn string) ([]*storageproviderv0alphapb.RecycleItem, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: no user in ctx")
	}
	eosDeletedEntries, err := fs.c.ListDeletedEntries(ctx, u.Username)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: error listing deleted entries")
	}
	recycleEntries := []*storageproviderv0alphapb.RecycleItem{}
	for _, entry := range eosDeletedEntries {
		if !fs.showHiddenSys {
			base := path.Base(entry.RestorePath)
			if hiddenReg.MatchString(base) {
				continue
			}

		}
		recycleItem := fs.convertToRecycleItem(ctx, entry)
		recycleEntries = append(recycleEntries, recycleItem)
	}
	return recycleEntries, nil
}

func (fs *eosStorage) RestoreRecycleItem(ctx context.Context, fn, key string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}
	return fs.c.RestoreDeletedEntry(ctx, u.Username, key)
}

func (fs *eosStorage) convertToRecycleItem(ctx context.Context, eosDeletedItem *eosclient.DeletedEntry) *storageproviderv0alphapb.RecycleItem {
	recycleItem := &storageproviderv0alphapb.RecycleItem{
		Path:         fs.removeNamespace(ctx, eosDeletedItem.RestorePath),
		Key:          eosDeletedItem.RestoreKey,
		Size:         eosDeletedItem.Size,
		DeletionTime: &typespb.Timestamp{Seconds: eosDeletedItem.DeletionMTime / 1000}, // TODO(labkode): check if eos time is millis or nanos
	}
	if eosDeletedItem.IsDir {
		recycleItem.Type = storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER
	} else {
		// TODO(labkode): if eos returns more types oin the future we need to map them.
		recycleItem.Type = storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_FILE
	}
	return recycleItem
}

func (fs *eosStorage) convertToRevision(ctx context.Context, eosFileInfo *eosclient.FileInfo) *storageproviderv0alphapb.FileVersion {
	md := fs.convertToResourceInfo(ctx, eosFileInfo)
	revision := &storageproviderv0alphapb.FileVersion{
		Key:   path.Base(md.Path),
		Size:  md.Size,
		Mtime: md.Mtime.Seconds, // TODO do we need nanos here?
	}
	return revision
}

func (fs *eosStorage) convertToResourceInfo(ctx context.Context, eosFileInfo *eosclient.FileInfo) *storageproviderv0alphapb.ResourceInfo {
	path := fs.removeNamespace(ctx, eosFileInfo.File)
	size := eosFileInfo.Size
	if eosFileInfo.IsDir {
		size = eosFileInfo.TreeSize
	}
	username, err := getUsername(eosFileInfo.UID)
	if err != nil {
		log := appctx.GetLogger(ctx)
		log.Warn().Uint64("uid", eosFileInfo.UID).Msg("could not lookup userid, leaving empty")
		username = "" // TODO(labkode): should we abort here?
	}

	return &storageproviderv0alphapb.ResourceInfo{
		Id:            &storageproviderv0alphapb.ResourceId{OpaqueId: fmt.Sprintf("%d", eosFileInfo.Inode)},
		Path:          path,
		Owner:         &typespb.UserId{OpaqueId: username},
		Type:          getResourceType(eosFileInfo.IsDir),
		Etag:          eosFileInfo.ETag,
		MimeType:      mime.Detect(eosFileInfo.IsDir, path),
		Size:          size,
		PermissionSet: &storageproviderv0alphapb.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Mtime: &typespb.Timestamp{
			Seconds: eosFileInfo.MTimeSec,
			Nanos:   eosFileInfo.MTimeNanos,
		},
		Opaque: &typespb.Opaque{
			Map: map[string]*typespb.OpaqueEntry{
				"eos": &typespb.OpaqueEntry{
					Decoder: "json",
					Value:   fs.getEosMetadata(eosFileInfo),
				},
			},
		},
	}
}

func getResourceType(isDir bool) storageproviderv0alphapb.ResourceType {
	if isDir {
		return storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_FILE
}

func getUsername(uid uint64) (string, error) {
	s := strconv.FormatUint(uid, 10)
	user, err := gouser.LookupId(s)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

type eosSysMetadata struct {
	TreeSize  uint64 `json:"tree_size"`
	TreeCount uint64 `json:"tree_count"`
	File      string `json:"file"`
	Instance  string `json:"instance"`
}

func (fs *eosStorage) getEosMetadata(finfo *eosclient.FileInfo) []byte {
	sys := &eosSysMetadata{
		File:     finfo.File,
		Instance: finfo.Instance,
	}

	if finfo.IsDir {
		sys.TreeCount = finfo.TreeCount
		sys.TreeSize = finfo.TreeSize
	}

	v, _ := json.Marshal(sys)
	return v
}
