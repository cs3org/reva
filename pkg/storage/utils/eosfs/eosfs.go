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

package eosfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	gouser "os/user"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/eosclient"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/acl"
	"github.com/cs3org/reva/pkg/storage/utils/grants"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

const (
	refTargetAttrKey = "reva.target"
)

var hiddenReg = regexp.MustCompile(`\.sys\..#.`)

// Config holds the configuration details for the EOS fs.
type Config struct {
	// Namespace for metadata operations
	Namespace string `mapstructure:"namespace"`

	// ShadowNamespace for storing shadow data
	ShadowNamespace string `mapstructure:"shadow_namespace"`

	// UploadsNamespace for storing upload data
	UploadsNamespace string `mapstructure:"uploads_namespace"`

	// ShareFolder defines the name of the folder in the
	// shadowed namespace. Ex: /eos/user/.shadow/h/hugo/MyShares
	ShareFolder string `mapstructure:"share_folder"`

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

	// SecProtocol specifies the xrootd security protocol to use between the server and EOS.
	SecProtocol string `mapstructure:"sec_protocol"`

	// Keytab specifies the location of the keytab to use to authenticate to EOS.
	Keytab string `mapstructure:"keytab"`

	// SingleUsername is the username to use when SingleUserMode is enabled
	SingleUsername string `mapstructure:"single_username"`

	// UserLayout wraps the internal path with user information.
	// Example: if conf.Namespace is /eos/user and received path is /docs
	// and the UserLayout is {{.Username}} the internal path will be:
	// /eos/user/<username>/docs
	UserLayout string `mapstructure:"user_layout"`

	// Enables logging of the commands executed
	// Defaults to false
	EnableLogging bool `mapstructure:"enable_logging"`

	// ShowHiddenSysFiles shows internal EOS files like
	// .sys.v# and .sys.a# files.
	ShowHiddenSysFiles bool `mapstructure:"show_hidden_sys_files"`

	// ForceSingleUserMode will force connections to EOS to use SingleUsername
	ForceSingleUserMode bool `mapstructure:"force_single_user_mode"`

	// UseKeyTabAuth changes will authenticate requests by using an EOS keytab.
	UseKeytab bool `mapstructure:"use_keytab"`

	// EnableHome enables the creation of home directories.
	EnableHome bool `mapstructure:"enable_home"`
}

func (c *Config) init() {
	c.Namespace = path.Clean(c.Namespace)
	if !strings.HasPrefix(c.Namespace, "/") {
		c.Namespace = "/"
	}

	if c.ShadowNamespace == "" {
		c.ShadowNamespace = path.Join(c.Namespace, ".shadow")
	}

	if c.ShareFolder == "" {
		c.ShareFolder = "/MyShares"
	}
	// ensure share folder always starts with slash
	c.ShareFolder = path.Join("/", c.ShareFolder)

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

	if c.UserLayout == "" {
		c.UserLayout = "{{.Username}}" // TODO set better layout
	}
}

type eosfs struct {
	c    *eosclient.Client
	conf *Config
}

// NewEOSFS returns a storage.FS interface implementation that connects to an
// EOS instance
func NewEOSFS(c *Config) (storage.FS, error) {
	c.init()

	// bail out if keytab is not found.
	if c.UseKeytab {
		if _, err := os.Stat(c.Keytab); err != nil {
			err = errors.Wrapf(err, "eos: keytab not accessible at location: %s", err)
			return nil, err
		}
	}

	eosClientOpts := &eosclient.Options{
		XrdcopyBinary:       c.XrdcopyBinary,
		URL:                 c.MasterURL,
		EosBinary:           c.EosBinary,
		CacheDirectory:      c.CacheDirectory,
		ForceSingleUserMode: c.ForceSingleUserMode,
		SingleUsername:      c.SingleUsername,
		UseKeytab:           c.UseKeytab,
		Keytab:              c.Keytab,
		SecProtocol:         c.SecProtocol,
	}

	eosClient := eosclient.New(eosClientOpts)

	eosfs := &eosfs{
		c:    eosClient,
		conf: c,
	}

	return eosfs, nil
}

func (fs *eosfs) Shutdown(ctx context.Context) error {
	// TODO(labkode): in a grpc implementation we can close connections.
	return nil
}

func getUser(ctx context.Context) (*userpb.User, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired(""), "eos: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

func (fs *eosfs) wrapShadow(ctx context.Context, fn string) (internal string) {
	if fs.conf.EnableHome {
		layout, err := fs.getInternalHome(ctx)
		if err != nil {
			panic(err)
		}
		internal = path.Join(fs.conf.ShadowNamespace, layout, fn)
	} else {
		internal = path.Join(fs.conf.ShadowNamespace, fn)
	}
	return
}

func (fs *eosfs) wrap(ctx context.Context, fn string) (internal string) {
	if fs.conf.EnableHome {
		layout, err := fs.getInternalHome(ctx)
		if err != nil {
			panic(err)
		}
		internal = path.Join(fs.conf.Namespace, layout, fn)
	} else {
		internal = path.Join(fs.conf.Namespace, fn)
	}
	log := appctx.GetLogger(ctx)
	log.Debug().Msg("eos: wrap external=" + fn + " internal=" + internal)
	return
}

func (fs *eosfs) unwrap(ctx context.Context, internal string) (external string) {
	log := appctx.GetLogger(ctx)
	layout := fs.getLayout(ctx)
	ns := fs.getNsMatch(internal, []string{fs.conf.Namespace, fs.conf.ShadowNamespace})
	external = fs.unwrapInternal(ctx, ns, internal, layout)
	log.Debug().Msgf("eos: unwrap: internal=%s external=%s", internal, external)
	return
}

func (fs *eosfs) getLayout(ctx context.Context) (layout string) {
	if fs.conf.EnableHome {
		u, err := getUser(ctx)
		if err != nil {
			panic(err)
		}
		layout = templates.WithUser(u, fs.conf.UserLayout)
	}
	return
}

func (fs *eosfs) getNsMatch(internal string, nss []string) string {
	var match string

	for _, ns := range nss {
		if strings.HasPrefix(internal, ns) && len(ns) > len(match) {
			match = ns
		}
	}

	if match == "" {
		panic(fmt.Sprintf("eos: path is outside namespaces: path=%s namespaces=%+v", internal, nss))
	}

	return match
}

func (fs *eosfs) unwrapInternal(ctx context.Context, ns, np, layout string) (external string) {
	log := appctx.GetLogger(ctx)
	trim := path.Join(ns, layout)

	if !strings.HasPrefix(np, trim) {
		panic("eos: resource is outside the directory of the logged-in user: internal=" + np + " trim=" + trim + " namespace=" + ns)
	}

	external = strings.TrimPrefix(np, trim)

	if external == "" {
		external = "/"
	}

	log.Debug().Msgf("eos: unwrapInternal: trim=%s external=%s ns=%s np=%s", trim, external, ns, np)

	return
}

// resolve takes in a request path or request id and returns the unwrappedNominal path.
func (fs *eosfs) resolve(ctx context.Context, u *userpb.User, ref *provider.Reference) (string, error) {
	if ref.GetPath() != "" {
		return ref.GetPath(), nil
	}

	if ref.GetId() != nil {
		p, err := fs.getPath(ctx, u, ref.GetId())
		if err != nil {
			return "", err
		}

		return p, nil
	}

	// reference is invalid
	return "", fmt.Errorf("invalid reference %+v. id and path are missing", ref)
}

func (fs *eosfs) getPath(ctx context.Context, u *userpb.User, id *provider.ResourceId) (string, error) {
	fid, err := strconv.ParseUint(id.OpaqueId, 10, 64)
	if err != nil {
		return "", fmt.Errorf("error converting string to int for eos fileid: %s", id.OpaqueId)
	}

	eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, u.Username, fid)
	if err != nil {
		return "", errors.Wrap(err, "eos: error getting file info by inode")
	}

	return fs.unwrap(ctx, eosFileInfo.File), nil
}

func (fs *eosfs) isShareFolder(ctx context.Context, p string) bool {
	return strings.HasPrefix(p, fs.conf.ShareFolder)
}

func (fs *eosfs) isShareFolderRoot(ctx context.Context, p string) bool {
	return path.Clean(p) == fs.conf.ShareFolder
}

func (fs *eosfs) isShareFolderChild(ctx context.Context, p string) bool {
	p = path.Clean(p)
	vals := strings.Split(p, fs.conf.ShareFolder+"/")
	return len(vals) > 1 && vals[1] != ""
}

func (fs *eosfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	u, err := getUser(ctx)
	if err != nil {
		return "", errors.Wrap(err, "eos: no user in ctx")
	}

	// parts[0] = 868317, parts[1] = photos, ...
	parts := strings.Split(id.OpaqueId, "/")
	fileID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return "", errors.Wrap(err, "eos: error parsing fileid string")
	}

	eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, u.Username, fileID)
	if err != nil {
		return "", errors.Wrap(err, "eos: error getting file info by inode")
	}

	fi := fs.convertToResourceInfo(ctx, eosFileInfo)
	return fi.Path, nil
}

func (fs *eosfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	return errtypes.NotSupported("eos: operation not supported")
}

func (fs *eosfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	return errtypes.NotSupported("eos: operation not supported")
}

func (fs *eosfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	fn := fs.wrap(ctx, p)

	eosACL, err := fs.getEosACL(g)
	if err != nil {
		return err
	}

	err = fs.c.AddACL(ctx, u.Username, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "eos: error adding acl")
	}

	return nil
}

func (fs *eosfs) getEosACL(g *provider.Grant) (*acl.Entry, error) {
	permissions, err := grants.GetACLPerm(g.Permissions)
	if err != nil {
		return nil, err
	}
	t, err := grants.GetACLType(g.Grantee.Type)
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

func (fs *eosfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	eosACLType, err := grants.GetACLType(g.Grantee.Type)
	if err != nil {
		return err
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	fn := fs.wrap(ctx, p)

	err = fs.c.RemoveACL(ctx, u.Username, fn, eosACLType, g.Grantee.Id.OpaqueId)
	if err != nil {
		return errors.Wrap(err, "eos: error removing acl")
	}
	return nil
}

func (fs *eosfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	eosACL, err := fs.getEosACL(g)
	if err != nil {
		return errors.Wrap(err, "eos: error mapping acl")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}
	fn := fs.wrap(ctx, p)

	err = fs.c.AddACL(ctx, u.Username, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "eos: error updating acl")
	}
	return nil
}

func (fs *eosfs) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}
	fn := fs.wrap(ctx, p)

	acls, err := fs.c.ListACLs(ctx, u.Username, fn)
	if err != nil {
		return nil, err
	}

	grantList := []*provider.Grant{}
	for _, a := range acls {
		grantee := &provider.Grantee{
			Id:   &userpb.UserId{OpaqueId: a.Qualifier},
			Type: grants.GetGranteeType(a.Type),
		}
		grantList = append(grantList, &provider.Grant{
			Grantee:     grantee,
			Permissions: grants.GetGrantPermissionSet(a.Permissions),
		})
	}

	return grantList, nil
}

func (fs *eosfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	log.Info().Msg("eos: get md for ref:" + ref.String())

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	// if path is home we need to add in the response any shadow folder in the shadow homedirectory.
	if fs.conf.EnableHome {
		if fs.isShareFolder(ctx, p) {
			return fs.getMDShareFolder(ctx, p, mdKeys)
		}
	}

	fn := fs.wrap(ctx, p)

	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, u.Username, fn)
	if err != nil {
		return nil, err
	}

	fi := fs.convertToResourceInfo(ctx, eosFileInfo)
	return fi, nil
}

func (fs *eosfs) getMDShareFolder(ctx context.Context, p string, mdKeys []string) (*provider.ResourceInfo, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	fn := fs.wrapShadow(ctx, p)
	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, u.Username, fn)
	if err != nil {
		return nil, err
	}
	// TODO(labkode): diff between root (dir) and children (ref)

	if fs.isShareFolderRoot(ctx, p) {
		return fs.convertToResourceInfo(ctx, eosFileInfo), nil
	}
	return fs.convertToFileReference(ctx, eosFileInfo), nil
}

func (fs *eosfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	log := appctx.GetLogger(ctx)
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	log.Debug().Msg("internal: " + p)

	// if path is home we need to add in the response any shadow folder in the shadow homedirectory.
	if fs.conf.EnableHome {
		log.Debug().Msg("home enabled")
		if strings.HasPrefix(p, "/") {
			return fs.listWithHome(ctx, "/", p)
		}
	}

	log.Debug().Msg("list with nominal home")
	return fs.listWithNominalHome(ctx, p)
}

func (fs *eosfs) listWithNominalHome(ctx context.Context, p string) (finfos []*provider.ResourceInfo, err error) {
	log := appctx.GetLogger(ctx)

	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	fn := fs.wrap(ctx, p)

	eosFileInfos, err := fs.c.List(ctx, u.Username, fn)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error listing")
	}

	for _, eosFileInfo := range eosFileInfos {
		// filter out sys files
		if !fs.conf.ShowHiddenSysFiles {
			base := path.Base(eosFileInfo.File)
			if hiddenReg.MatchString(base) {
				log.Debug().Msgf("eos: path is filtered because is considered hidden: path=%s hiddenReg=%s", base, hiddenReg)
				continue
			}
		}

		finfos = append(finfos, fs.convertToResourceInfo(ctx, eosFileInfo))
	}

	return finfos, nil
}

func (fs *eosfs) listWithHome(ctx context.Context, home, p string) ([]*provider.ResourceInfo, error) {
	log := appctx.GetLogger(ctx)
	if p == home {
		log.Debug().Msg("listing home")
		return fs.listHome(ctx, home)
	}

	if fs.isShareFolderRoot(ctx, p) {
		log.Debug().Msg("listing share root folder")
		return fs.listShareFolderRoot(ctx, p)
	}

	if fs.isShareFolderChild(ctx, p) {
		return nil, errtypes.PermissionDenied("eos: error listing folders inside the shared folder, only file references are stored inside")
	}

	// path points to a resource in the nominal home
	log.Debug().Msg("listing nominal home")
	return fs.listWithNominalHome(ctx, p)
}

func (fs *eosfs) listHome(ctx context.Context, home string) ([]*provider.ResourceInfo, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	fns := []string{fs.wrap(ctx, home), fs.wrapShadow(ctx, home)}

	finfos := []*provider.ResourceInfo{}
	for _, fn := range fns {
		eosFileInfos, err := fs.c.List(ctx, u.Username, fn)
		if err != nil {
			return nil, errors.Wrap(err, "eos: error listing")
		}

		for _, eosFileInfo := range eosFileInfos {
			// filter out sys files
			if !fs.conf.ShowHiddenSysFiles {
				base := path.Base(eosFileInfo.File)
				if hiddenReg.MatchString(base) {
					continue
				}

			}
			finfos = append(finfos, fs.convertToResourceInfo(ctx, eosFileInfo))
		}

	}
	return finfos, nil
}

func (fs *eosfs) listShareFolderRoot(ctx context.Context, p string) (finfos []*provider.ResourceInfo, err error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	fn := fs.wrapShadow(ctx, p)

	eosFileInfos, err := fs.c.List(ctx, u.Username, fn)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error listing")
	}

	for _, eosFileInfo := range eosFileInfos {
		// filter out sys files
		if !fs.conf.ShowHiddenSysFiles {
			base := path.Base(eosFileInfo.File)
			if hiddenReg.MatchString(base) {
				continue
			}
		}

		finfo := fs.convertToFileReference(ctx, eosFileInfo)
		finfos = append(finfos, finfo)
	}

	return finfos, nil
}

func (fs *eosfs) GetQuota(ctx context.Context) (int, int, error) {
	u, err := getUser(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "eos: no user in ctx")
	}

	qi, err := fs.c.GetQuota(ctx, u.Username, fs.conf.Namespace)
	if err != nil {
		err := errors.Wrap(err, "eosfs: error getting quota")
		return 0, 0, err
	}

	return qi.AvailableBytes, qi.UsedBytes, nil
}

func (fs *eosfs) getInternalHome(ctx context.Context) (string, error) {
	if !fs.conf.EnableHome {
		return "", errtypes.NotSupported("eos: get home not supported")
	}

	u, err := getUser(ctx)
	if err != nil {
		err = errors.Wrap(err, "local: wrap: no user in ctx and home is enabled")
		return "", err
	}

	relativeHome := templates.WithUser(u, fs.conf.UserLayout)
	return relativeHome, nil
}

func (fs *eosfs) GetHome(ctx context.Context) (string, error) {
	if !fs.conf.EnableHome {
		return "", errtypes.NotSupported("eos: get home not supported")
	}

	// eos drive for homes assumes root(/) points to the user home.
	return "/", nil
}

func (fs *eosfs) createShadowHome(ctx context.Context) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	home := fs.wrapShadow(ctx, "/")
	_, err = fs.c.GetFileInfoByPath(ctx, "root", home)
	if err == nil { // home already exists
		return nil
	}

	// TODO(labkode): abort on any error that is not found
	if _, ok := err.(errtypes.IsNotFound); !ok {
		return errors.Wrap(err, "eos: error verifying if user home directory exists")
	}

	err = fs.createUserDir(ctx, u.Username, home)
	if err != nil {
		return err
	}
	shadowFolders := []string{fs.conf.ShareFolder}
	for _, sf := range shadowFolders {
		err = fs.createUserDir(ctx, u.Username, path.Join(home, sf))
		if err != nil {
			return err
		}
	}

	return nil
}

func (fs *eosfs) createNominalHome(ctx context.Context) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	home := fs.wrap(ctx, "/")
	_, err = fs.c.GetFileInfoByPath(ctx, "root", home)
	if err == nil { // home already exists
		return nil
	}

	// TODO(labkode): abort on any error that is not found
	if _, ok := err.(errtypes.IsNotFound); !ok {
		return errors.Wrap(err, "eos: error verifying if user home directory exists")
	}

	err = fs.createUserDir(ctx, u.Username, home)
	return err
}

func (fs *eosfs) CreateHome(ctx context.Context) error {
	if !fs.conf.EnableHome {
		return errtypes.NotSupported("eos: create home not supported")
	}

	if err := fs.createNominalHome(ctx); err != nil {
		return errors.Wrap(err, "eos: error creating nominal home")
	}

	if err := fs.createShadowHome(ctx); err != nil {
		return errors.Wrap(err, "eos: error creating shadow home")
	}

	return nil
}

func (fs *eosfs) createUserDir(ctx context.Context, username string, path string) error {
	err := fs.c.CreateDir(ctx, "root", path)
	if err != nil {
		// EOS will return success on mkdir over an existing directory.
		return errors.Wrap(err, "eos: error creating dir")
	}

	err = fs.c.Chown(ctx, "root", username, path)
	if err != nil {
		return errors.Wrap(err, "eos: error chowning directory")
	}

	err = fs.c.Chmod(ctx, "root", "2770", path)
	if err != nil {
		return errors.Wrap(err, "eos: error chmoding directory")
	}

	attrs := []*eosclient.Attribute{
		&eosclient.Attribute{
			Type: eosclient.SystemAttr,
			Key:  "mask",
			Val:  "700",
		},
		&eosclient.Attribute{
			Type: eosclient.SystemAttr,
			Key:  "allow.oc.sync",
			Val:  "1",
		},
		&eosclient.Attribute{
			Type: eosclient.SystemAttr,
			Key:  "mtime.propagation",
			Val:  "1",
		},
		&eosclient.Attribute{
			Type: eosclient.SystemAttr,
			Key:  "forced.atomic",
			Val:  "1",
		},
	}

	for _, attr := range attrs {
		err = fs.c.SetAttr(ctx, "root", attr, true, path)
		if err != nil {
			return errors.Wrap(err, "eos: error setting attribute")
		}
	}
	return nil
}

func (fs *eosfs) CreateDir(ctx context.Context, p string) error {
	log := appctx.GetLogger(ctx)
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	log.Info().Msgf("eos: createdir: path=%s", p)

	if fs.isShareFolder(ctx, p) {
		return errtypes.PermissionDenied("eos: cannot create folder under the share folder")
	}

	fn := fs.wrap(ctx, p)
	return fs.c.CreateDir(ctx, u.Username, fn)
}

func (fs *eosfs) CreateReference(ctx context.Context, p string, targetURI *url.URL) error {
	// TODO(labkode): for the time being we only allow to create references
	// on the virtual share folder to not pollute the nominal user tree.

	if !fs.isShareFolder(ctx, p) {
		return errtypes.PermissionDenied("eos: cannot create references outside the share folder: share_folder=" + fs.conf.ShareFolder + " path=" + p)
	}

	fn := fs.wrapShadow(ctx, p)

	// TODO(labkode): with grpc we can create a file touching with xattrs.
	// Current mechanism is: touch to hidden dir, set xattr, rename.
	dir, base := path.Split(fn)
	tmp := path.Join(dir, fmt.Sprintf(".sys.reva#.%s", base))
	if err := fs.c.CreateDir(ctx, "root", tmp); err != nil {
		err = errors.Wrapf(err, "eos: error creating temporary ref file")
		return err
	}

	// set xattr on ref
	attr := &eosclient.Attribute{
		Type: eosclient.UserAttr,
		Key:  refTargetAttrKey,
		Val:  targetURI.String(),
	}

	if err := fs.c.SetAttr(ctx, "root", attr, false, tmp); err != nil {
		err = errors.Wrapf(err, "eos: error setting reva.ref attr on file: %q", tmp)
		return err
	}

	// rename to have the file visible in user space.
	if err := fs.c.Rename(ctx, "root", tmp, fn); err != nil {
		err = errors.Wrapf(err, "eos: error renaming from: %q to %q", tmp, fn)
		return err
	}

	return nil
}

func (fs *eosfs) Delete(ctx context.Context, ref *provider.Reference) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return fs.deleteShadow(ctx, p)
	}

	fn := fs.wrap(ctx, p)

	return fs.c.Remove(ctx, u.Username, fn)
}

func (fs *eosfs) deleteShadow(ctx context.Context, p string) error {
	if fs.isShareFolderRoot(ctx, p) {
		return errtypes.PermissionDenied("eos: cannot delete the virtual share folder")
	}

	if fs.isShareFolderChild(ctx, p) {
		u, err := getUser(ctx)
		if err != nil {
			return errors.Wrap(err, "eos: no user in ctx")
		}
		fn := fs.wrapShadow(ctx, p)
		return fs.c.Remove(ctx, u.Username, fn)
	}

	panic("eos: shadow delete of share folder that is neither root nor child. path=" + p)
}

func (fs *eosfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	oldPath, err := fs.resolve(ctx, u, oldRef)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	newPath, err := fs.resolve(ctx, u, newRef)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, oldPath) || fs.isShareFolder(ctx, newPath) {
		return fs.moveShadow(ctx, oldPath, newPath)
	}

	oldFn := fs.wrap(ctx, oldPath)
	newFn := fs.wrap(ctx, newPath)
	return fs.c.Rename(ctx, u.Username, oldFn, newFn)
}

func (fs *eosfs) moveShadow(ctx context.Context, oldPath, newPath string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	if fs.isShareFolderRoot(ctx, oldPath) || fs.isShareFolderRoot(ctx, newPath) {
		return errtypes.PermissionDenied("eos: cannot move/rename the virtual share folder")
	}

	// only rename of the reference is allowed, hence having the same basedir
	bold, _ := path.Split(oldPath)
	bnew, _ := path.Split(newPath)

	if bold != bnew {
		return errtypes.PermissionDenied("eos: cannot move references under the virtual share folder")
	}

	oldfn := fs.wrapShadow(ctx, oldPath)
	newfn := fs.wrapShadow(ctx, newPath)
	return fs.c.Rename(ctx, u.Username, oldfn, newfn)
}

func (fs *eosfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return nil, errtypes.PermissionDenied("eos: cannot download under the virtual share folder")
	}

	fn := fs.wrap(ctx, p)

	return fs.c.Read(ctx, u.Username, fn)
}

func (fs *eosfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return nil, errtypes.PermissionDenied("eos: cannot list revisions under the virtual share folder")
	}

	fn := fs.wrap(ctx, p)

	eosRevisions, err := fs.c.ListVersions(ctx, u.Username, fn)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error listing versions")
	}
	revisions := []*provider.FileVersion{}
	for _, eosRev := range eosRevisions {
		rev := fs.convertToRevision(ctx, eosRev)
		revisions = append(revisions, rev)
	}
	return revisions, nil
}

func (fs *eosfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return nil, errtypes.PermissionDenied("eos: cannot download revision under the virtual share folder")
	}

	fn := fs.wrap(ctx, p)

	fn = fs.wrap(ctx, fn)
	return fs.c.ReadVersion(ctx, u.Username, fn, revisionKey)
}

func (fs *eosfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return errtypes.PermissionDenied("eos: cannot restore revision under the virtual share folder")
	}

	fn := fs.wrap(ctx, p)

	return fs.c.RollbackToVersion(ctx, u.Username, fn, revisionKey)
}

func (fs *eosfs) PurgeRecycleItem(ctx context.Context, key string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}
	return fs.c.RestoreDeletedEntry(ctx, u.Username, key)
}

func (fs *eosfs) EmptyRecycle(ctx context.Context) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}
	return fs.c.PurgeDeletedEntries(ctx, u.Username)
}

func (fs *eosfs) ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}
	eosDeletedEntries, err := fs.c.ListDeletedEntries(ctx, u.Username)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error listing deleted entries")
	}
	recycleEntries := []*provider.RecycleItem{}
	for _, entry := range eosDeletedEntries {
		if !fs.conf.ShowHiddenSysFiles {
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

func (fs *eosfs) RestoreRecycleItem(ctx context.Context, key string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}
	return fs.c.RestoreDeletedEntry(ctx, u.Username, key)
}

func (fs *eosfs) convertToRecycleItem(ctx context.Context, eosDeletedItem *eosclient.DeletedEntry) *provider.RecycleItem {
	path := fs.unwrap(ctx, eosDeletedItem.RestorePath)
	recycleItem := &provider.RecycleItem{
		Path:         path,
		Key:          eosDeletedItem.RestoreKey,
		Size:         eosDeletedItem.Size,
		DeletionTime: &types.Timestamp{Seconds: eosDeletedItem.DeletionMTime / 1000}, // TODO(labkode): check if eos time is millis or nanos
	}
	if eosDeletedItem.IsDir {
		recycleItem.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	} else {
		// TODO(labkode): if eos returns more types oin the future we need to map them.
		recycleItem.Type = provider.ResourceType_RESOURCE_TYPE_FILE
	}
	return recycleItem
}

func (fs *eosfs) convertToRevision(ctx context.Context, eosFileInfo *eosclient.FileInfo) *provider.FileVersion {
	md := fs.convertToResourceInfo(ctx, eosFileInfo)
	revision := &provider.FileVersion{
		Key:   path.Base(md.Path),
		Size:  md.Size,
		Mtime: md.Mtime.Seconds, // TODO do we need nanos here?
	}
	return revision
}

func (fs *eosfs) convertToResourceInfo(ctx context.Context, eosFileInfo *eosclient.FileInfo) *provider.ResourceInfo {
	return fs.convert(ctx, eosFileInfo)
}

func (fs *eosfs) convertToFileReference(ctx context.Context, eosFileInfo *eosclient.FileInfo) *provider.ResourceInfo {
	info := fs.convert(ctx, eosFileInfo)
	info.Type = provider.ResourceType_RESOURCE_TYPE_REFERENCE
	val, ok := eosFileInfo.Attrs["user.reva.target"]
	if !ok || val == "" {
		panic("eos: reference does not contain target: target=" + val + " file=" + eosFileInfo.File)
	}
	info.Target = val
	return info
}

func (fs *eosfs) convert(ctx context.Context, eosFileInfo *eosclient.FileInfo) *provider.ResourceInfo {
	path := fs.unwrap(ctx, eosFileInfo.File)

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

	info := &provider.ResourceInfo{
		Id:            &provider.ResourceId{OpaqueId: fmt.Sprintf("%d", eosFileInfo.Inode)},
		Path:          path,
		Owner:         &userpb.UserId{OpaqueId: username},
		Etag:          fmt.Sprintf("\"%s\"", strings.Trim(eosFileInfo.ETag, "\"")),
		MimeType:      mime.Detect(eosFileInfo.IsDir, path),
		Size:          size,
		PermissionSet: &provider.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Mtime: &types.Timestamp{
			Seconds: eosFileInfo.MTimeSec,
			Nanos:   eosFileInfo.MTimeNanos,
		},
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"eos": &types.OpaqueEntry{
					Decoder: "json",
					Value:   fs.getEosMetadata(eosFileInfo),
				},
			},
		},
	}

	info.Type = getResourceType(eosFileInfo.IsDir)
	return info
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
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

func (fs *eosfs) getEosMetadata(finfo *eosclient.FileInfo) []byte {
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

/*
	Merge shadow on requests for /home ?

	No - GetHome(ctx context.Context) (string, error)
	No -CreateHome(ctx context.Context) error
	No - CreateDir(ctx context.Context, fn string) error
	No -Delete(ctx context.Context, ref *provider.Reference) error
	No -Move(ctx context.Context, oldRef, newRef *provider.Reference) error
	No -GetMD(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error)
	Yes -ListFolder(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error)
	No -Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error
	No -Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error)
	No -ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error)
	No -DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error)
	No -RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error
	No ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error)
	No RestoreRecycleItem(ctx context.Context, key string) error
	No PurgeRecycleItem(ctx context.Context, key string) error
	No EmptyRecycle(ctx context.Context) error
	? GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error)
	No AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	No RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	No UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	No ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error)
	No GetQuota(ctx context.Context) (int, int, error)
	No CreateReference(ctx context.Context, path string, targetURI *url.URL) error
	No Shutdown(ctx context.Context) error
	No SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error
	No UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error
*/

/*
	Merge shadow on requests for /home/MyShares ?

	No - GetHome(ctx context.Context) (string, error)
	No -CreateHome(ctx context.Context) error
	No - CreateDir(ctx context.Context, fn string) error
	Maybe -Delete(ctx context.Context, ref *provider.Reference) error
	No -Move(ctx context.Context, oldRef, newRef *provider.Reference) error
	Yes -GetMD(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error)
	Yes -ListFolder(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error)
	No -Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error
	No -Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error)
	No -ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error)
	No -DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error)
	No -RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error
	No ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error)
	No RestoreRecycleItem(ctx context.Context, key string) error
	No PurgeRecycleItem(ctx context.Context, key string) error
	No EmptyRecycle(ctx context.Context) error
	?  GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error)
	No AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	No RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	No UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	No ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error)
	No GetQuota(ctx context.Context) (int, int, error)
	No CreateReference(ctx context.Context, path string, targetURI *url.URL) error
	No Shutdown(ctx context.Context) error
	No SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error
	No UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error
*/

/*
	Merge shadow on requests for /home/MyShares/file-reference ?

	No - GetHome(ctx context.Context) (string, error)
	No -CreateHome(ctx context.Context) error
	No - CreateDir(ctx context.Context, fn string) error
	Maybe -Delete(ctx context.Context, ref *provider.Reference) error
	Yes -Move(ctx context.Context, oldRef, newRef *provider.Reference) error
	Yes -GetMD(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error)
	No -ListFolder(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error)
	No -Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error
	No -Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error)
	No -ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error)
	No -DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error)
	No -RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error
	No ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error)
	No RestoreRecycleItem(ctx context.Context, key string) error
	No PurgeRecycleItem(ctx context.Context, key string) error
	No EmptyRecycle(ctx context.Context) error
	?  GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error)
	No AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	No RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	No UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error
	No ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error)
	No GetQuota(ctx context.Context) (int, int, error)
	No CreateReference(ctx context.Context, path string, targetURI *url.URL) error
	No Shutdown(ctx context.Context) error
	Maybe SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error
	Maybe UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error
*/
