package eos

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/cernbox/reva/pkg/storage/fs/registry"

	"github.com/cernbox/reva/pkg/eosclient"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/mime"
	"github.com/cernbox/reva/pkg/storage"
	"github.com/cernbox/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("eos", New)
}

var hiddenReg = regexp.MustCompile(`\.sys\..#.`)

var logger = log.New("eos")

type contextUserRequiredErr string

func (err contextUserRequiredErr) Error() string   { return string(err) }
func (err contextUserRequiredErr) IsUserRequired() {}

type eosStorage struct {
	c             *eosclient.Client
	mountpoint    string
	showHiddenSys bool
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
	// Default is root://eos-test.org
	MasterURL string `mapstructure:"master_url"`

	// URL of the Slave EOS MGM.
	// Default is root://eos-test.org
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

func getUser(ctx context.Context) (*user.User, error) {
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
	}

	return eosStorage, nil
}

func (fs *eosStorage) getInternalPath(ctx context.Context, fn string) string {
	internalPath := path.Join(fs.mountpoint, fn)
	msg := fmt.Sprintf("func=getInternalPath outter=%s inner=%s", fn, internalPath)
	logger.Println(ctx, msg)
	return internalPath
}

func (fs *eosStorage) removeNamespace(ctx context.Context, np string) string {
	p := strings.TrimPrefix(np, fs.mountpoint)
	if p == "" {
		p = "/"
	}

	msg := fmt.Sprintf("func=removeNamespace inner=%s outter=%s", np, p)
	logger.Println(ctx, msg)
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

	fi := fs.convertToMD(ctx, eosFileInfo)
	return fi.Path, nil
}

func (fs *eosStorage) SetACL(ctx context.Context, fn string, a *storage.ACL) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}

	fn = fs.getInternalPath(ctx, fn)

	eosACL := fs.getEosACL(a)

	err = fs.c.AddACL(ctx, u.Username, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "storage_eos: error adding acl")
	}

	return nil
}

func getEosACLType(aclType storage.ACLType) string {
	switch aclType {
	case storage.ACLTypeUser:
		return "u"
	case storage.ACLTypeGroup:
		return "g"
	default:
		panic(aclType)
	}
}

func getEosACLPerm(mode storage.ACLMode) string {
	switch mode {
	case storage.ACLModeReadOnly:
		return "rx"
	case storage.ACLModeReadWrite:
		return "rwx!d"
	default:
		panic(mode)
	}
}

func (fs *eosStorage) getEosACL(a *storage.ACL) *eosclient.ACL {
	eosACL := &eosclient.ACL{Target: a.Target}
	eosACL.Mode = getEosACLPerm(a.Mode)
	eosACL.Type = getEosACLType(a.Type)
	return eosACL
}

func (fs *eosStorage) UnsetACL(ctx context.Context, fn string, a *storage.ACL) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}

	eosACLType := getEosACLType(a.Type)

	fn = fs.getInternalPath(ctx, fn)

	err = fs.c.RemoveACL(ctx, u.Username, fn, eosACLType, a.Target)
	if err != nil {
		return errors.Wrap(err, "storage_eos: error removing acl")
	}
	return nil
}

func (fs *eosStorage) UpdateACL(ctx context.Context, fn string, a *storage.ACL) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}

	eosACL := fs.getEosACL(a)

	fn = fs.getInternalPath(ctx, fn)
	err = fs.c.AddACL(ctx, u.Username, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "storage_eos: error updating acl")
	}
	return nil
}

func (fs *eosStorage) GetACL(ctx context.Context, fn string, aclType storage.ACLType, target string) (*storage.ACL, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	fn = fs.getInternalPath(ctx, fn)
	eosACL, err := fs.c.GetACL(ctx, u.Username, fn, getEosACLType(aclType), target)
	if err != nil {
		return nil, err
	}

	acl := &storage.ACL{
		Target: eosACL.Target,
		Mode:   fs.getACLMode(eosACL.Mode),
		Type:   fs.getACLType(eosACL.Type),
	}
	return acl, nil
}

func (fs *eosStorage) ListACLs(ctx context.Context, fn string) ([]*storage.ACL, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	fn = fs.getInternalPath(ctx, fn)
	eosACLs, err := fs.c.ListACLs(ctx, u.Username, fn)
	if err != nil {
		return nil, err
	}

	acls := []*storage.ACL{}
	for _, a := range eosACLs {
		acl := &storage.ACL{
			Target: a.Target,
			Mode:   fs.getACLMode(a.Mode),
			Type:   fs.getACLType(a.Type),
		}
		acls = append(acls, acl)
	}

	return acls, nil
}

func (fs *eosStorage) getACLType(aclType string) storage.ACLType {
	switch aclType {
	case "u":
		return storage.ACLTypeUser
	case "g":
		return storage.ACLTypeGroup
	default:
		return storage.ACLTypeInvalid
	}
}
func (fs *eosStorage) getACLMode(mode string) storage.ACLMode {
	switch mode {
	case "rx":
		return storage.ACLModeReadOnly
	case "rwx!d":
		return storage.ACLModeReadWrite
	default:
		return storage.ACLModeInvalid
	}
}

func (fs *eosStorage) GetMD(ctx context.Context, fn string) (*storage.MD, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	fn = fs.getInternalPath(ctx, fn)
	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, u.Username, fn)
	if err != nil {
		return nil, err
	}
	fi := fs.convertToMD(ctx, eosFileInfo)
	return fi, nil
}

func (fs *eosStorage) ListFolder(ctx context.Context, fn string) ([]*storage.MD, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: no user in ctx")
	}

	fn = fs.getInternalPath(ctx, fn)
	eosFileInfos, err := fs.c.List(ctx, u.Username, fn)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: errong listing")
	}

	finfos := []*storage.MD{}
	for _, eosFileInfo := range eosFileInfos {
		// filter out sys files
		if !fs.showHiddenSys {
			base := path.Base(eosFileInfo.File)
			if hiddenReg.MatchString(base) {
				continue
			}

		}
		finfos = append(finfos, fs.convertToMD(ctx, eosFileInfo))
	}
	return finfos, nil
}

func (fs *eosStorage) GetQuota(ctx context.Context, fn string) (int, int, error) {
	u, err := getUser(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "storage_eos: no user in ctx")
	}
	fn = fs.getInternalPath(ctx, fn)
	return fs.c.GetQuota(ctx, u.Username, fn)
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

func (fs *eosStorage) ListRevisions(ctx context.Context, fn string) ([]*storage.Revision, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: no user in ctx")
	}
	fn = fs.getInternalPath(ctx, fn)
	eosRevisions, err := fs.c.ListVersions(ctx, u.Username, fn)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: error listing versions")
	}
	revisions := []*storage.Revision{}
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

func (fs *eosStorage) ListRecycle(ctx context.Context, fn string) ([]*storage.RecycleItem, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: no user in ctx")
	}
	eosDeletedEntries, err := fs.c.ListDeletedEntries(ctx, u.Username)
	if err != nil {
		return nil, errors.Wrap(err, "storage_eos: error listing deleted entries")
	}
	recycleEntries := []*storage.RecycleItem{}
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

func (fs *eosStorage) convertToRecycleItem(ctx context.Context, eosDeletedItem *eosclient.DeletedEntry) *storage.RecycleItem {
	recycleItem := &storage.RecycleItem{
		RestorePath: fs.removeNamespace(ctx, eosDeletedItem.RestorePath),
		RestoreKey:  eosDeletedItem.RestoreKey,
		Size:        eosDeletedItem.Size,
		DelMtime:    eosDeletedItem.DeletionMTime,
		IsDir:       eosDeletedItem.IsDir,
	}
	return recycleItem
}

func (fs *eosStorage) convertToRevision(ctx context.Context, eosFileInfo *eosclient.FileInfo) *storage.Revision {
	md := fs.convertToMD(ctx, eosFileInfo)
	revision := &storage.Revision{
		RevKey: path.Base(md.Path),
		Size:   md.Size,
		Mtime:  md.Mtime,
		IsDir:  md.IsDir,
	}
	return revision
}
func (fs *eosStorage) convertToMD(ctx context.Context, eosFileInfo *eosclient.FileInfo) *storage.MD {
	finfo := new(storage.MD)
	finfo.ID = fmt.Sprintf("%d", eosFileInfo.Inode)
	finfo.Path = fs.removeNamespace(ctx, eosFileInfo.File)
	finfo.Mtime = eosFileInfo.MTime
	finfo.IsDir = eosFileInfo.IsDir
	// FIXME the etag of dirs does not change, only mtime and size are propagated, so we have to calculate an etag for dirs
	if eosFileInfo.IsDir {
		h := md5.New()
		binary.Write(h, binary.LittleEndian, eosFileInfo.MTime)
		binary.Write(h, binary.LittleEndian, eosFileInfo.Inode)
		io.WriteString(h, eosFileInfo.Instance)
		binary.Write(h, binary.LittleEndian, eosFileInfo.TreeSize)
		finfo.Etag = fmt.Sprintf(`"%x"`, h.Sum(nil))
		finfo.Size = eosFileInfo.TreeSize
	} else {
		finfo.Etag = eosFileInfo.ETag
		finfo.Size = eosFileInfo.Size
	}
	finfo.Mime = mime.Detect(finfo.IsDir, finfo.Path)
	finfo.Sys = fs.getEosMetadata(eosFileInfo)
	finfo.Permissions = &storage.Permissions{Read: true, Write: true, Share: true}

	return finfo
}

type eosSysMetadata struct {
	TreeSize  uint64
	TreeCount uint64
	File      string
	Instance  string
}

func (fs *eosStorage) getEosMetadata(finfo *eosclient.FileInfo) map[string]interface{} {
	sys := &eosSysMetadata{
		File:     finfo.File,
		Instance: finfo.Instance,
	}

	if finfo.IsDir {
		sys.TreeCount = finfo.TreeCount
		sys.TreeSize = finfo.TreeSize
	}
	return map[string]interface{}{"eos": sys}
}
