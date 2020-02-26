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

package eos

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
	"github.com/cs3org/reva/pkg/storage/acl"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

func init() {
	registry.Register("eos", New)
}

var hiddenReg = regexp.MustCompile(`\.sys\..#.`)

type eosStorage struct {
	c    *eosclient.Client
	conf *config
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

	// ShadowNamespace for storing shadow data
	ShadowNamespace string `mapstructure:"shadow_namespace"`

	// ShadowShareFolder defines the name of the folder in the
	// shadowed namespace. Ex: /eos/user/.shadow/h/hugo/MyShares
	ShadowShareFolder string `mapstructure:"shadown_share_folder"`

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
	UseKeytab bool `mapstrucuture:"use_keytab"`

	// EnableHome enables the creation of home directories.
	EnableHome bool `mapstructure:"enable_home"`
}

func getUser(ctx context.Context) (*userpb.User, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired(""), "eos: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

func (c *config) init() {
	c.Namespace = path.Clean(c.Namespace)
	if !strings.HasPrefix(c.Namespace, "/") {
		c.Namespace = "/"
	}

	if c.ShadowNamespace == "" {
		c.ShadowNamespace = path.Join(c.Namespace, ".shadow")
	}

	if c.ShadowShareFolder == "" {
		c.ShadowShareFolder = path.Join(c.ShadowNamespace, "MyShares")
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

	// bail out if keytab is not found.
	if c.UseKeytab {
		if _, err := os.Stat(c.Keytab); err != nil {
			err = errors.Wrapf(err, "eos: keytab not accesible at location: %s", err)
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

	eosStorage := &eosStorage{
		c:    eosClient,
		conf: c,
	}

	return eosStorage, nil
}

func (fs *eosStorage) Shutdown(ctx context.Context) error {
	// TODO(labkode): in a grpc implementation we can close connections.
	return nil
}

func (fs *eosStorage) wrapShadow(ctx context.Context, fn string) (internal string) {
	if fs.conf.EnableHome && fs.conf.UserLayout != "" {
		u, err := getUser(ctx)
		if err != nil {
			err = errors.Wrap(err, "eos: wrap: no user in ctx and home is enabled")
			panic(err)
		}
		layout := templates.WithUser(u, fs.conf.UserLayout)
		internal = path.Join(fs.conf.ShadowNamespace, layout, fn)
	} else {
		internal = path.Join(fs.conf.ShadowNamespace, fn)
	}
	return
}

func (fs *eosStorage) wrap(ctx context.Context, fn string) (internal string) {
	if fs.conf.EnableHome && fs.conf.UserLayout != "" {
		u, err := getUser(ctx)
		if err != nil {
			err = errors.Wrap(err, "eos: wrap: no user in ctx and home is enabled")
			panic(err)
		}
		layout := templates.WithUser(u, fs.conf.UserLayout)
		internal = path.Join(fs.conf.Namespace, layout, fn)
	} else {
		internal = path.Join(fs.conf.Namespace, fn)
	}
	return
}

func (fs *eosStorage) unwrapShadow(ctx context.Context, np string) (external string) {
	if fs.conf.EnableHome && fs.conf.UserLayout != "" {
		u, err := getUser(ctx)
		if err != nil {
			err = errors.Wrap(err, "eos: unwrap: no user in ctx and home is enabled")
			panic(err)
		}
		layout := templates.WithUser(u, fs.conf.UserLayout)
		trim := path.Join(fs.conf.ShadowNamespace, layout)
		external = strings.TrimPrefix(np, trim)
	} else {
		external = strings.TrimPrefix(np, fs.conf.ShadowNamespace)
		if external == "" {
			external = "/"
		}
	}
	return
}

func (fs *eosStorage) unwrap(ctx context.Context, np string) (external string) {
	if fs.conf.EnableHome && fs.conf.UserLayout != "" {
		u, err := getUser(ctx)
		if err != nil {
			err = errors.Wrap(err, "eos: unwrap: no user in ctx and home is enabled")
			panic(err)
		}
		layout := templates.WithUser(u, fs.conf.UserLayout)
		trim := path.Join(fs.conf.Namespace, layout)
		external = strings.TrimPrefix(np, trim)
	} else {
		external = strings.TrimPrefix(np, fs.conf.Namespace)
		if external == "" {
			external = "/"
		}
	}
	return
}

func (fs *eosStorage) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
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

// resolve takes in a request path or request id and converts it to a internal path.
func (fs *eosStorage) resolve(ctx context.Context, u *userpb.User, ref *provider.Reference) (string, error) {
	if ref.GetPath() != "" {
		return fs.wrap(ctx, ref.GetPath()), nil
	}

	if ref.GetId() != nil {
		fn, err := fs.getPath(ctx, u, ref.GetId())
		if err != nil {
			return "", err
		}
		return fn, nil
	}

	// reference is invalid
	return "", fmt.Errorf("invalid reference %+v", ref)
}

func (fs *eosStorage) getPath(ctx context.Context, u *userpb.User, id *provider.ResourceId) (string, error) {
	fid, err := strconv.ParseUint(id.OpaqueId, 10, 64)
	if err != nil {
		return "", fmt.Errorf("error converting string to int for eos fileid: %s", id.OpaqueId)
	}
	eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, u.Username, fid)
	if err != nil {
		return "", errors.Wrap(err, "eos: error getting file info by inode")
	}
	return eosFileInfo.File, nil
}

func (fs *eosStorage) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	fn, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

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

func getEosACLType(gt provider.GranteeType) (string, error) {
	switch gt {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		return "u", nil
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		return "g", nil
	default:
		return "", errors.New("no eos acl for grantee type: " + gt.String())
	}
}

// TODO(labkode): fine grained permission controls.
func getEosACLPerm(set *provider.ResourcePermissions) (string, error) {
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

	// TODO sharing
	// TODO trash
	// TODO versions
	return b.String(), nil
}

func (fs *eosStorage) getEosACL(g *provider.Grant) (*acl.Entry, error) {
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

func (fs *eosStorage) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	return errtypes.NotSupported("eos: operation not supported")
}

func (fs *eosStorage) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	return errtypes.NotSupported("eos: operation not supported")
}

func (fs *eosStorage) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	eosACLType, err := getEosACLType(g.Grantee.Type)
	if err != nil {
		return err
	}

	fn, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	err = fs.c.RemoveACL(ctx, u.Username, fn, eosACLType, g.Grantee.Id.OpaqueId)
	if err != nil {
		return errors.Wrap(err, "eos: error removing acl")
	}
	return nil
}

func (fs *eosStorage) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	eosACL, err := fs.getEosACL(g)
	if err != nil {
		return errors.Wrap(err, "eos: error mapping acl")
	}

	fn, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	err = fs.c.AddACL(ctx, u.Username, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "eos: error updating acl")
	}
	return nil
}

func (fs *eosStorage) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	fn, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	acls, err := fs.c.ListACLs(ctx, u.Username, fn)
	if err != nil {
		return nil, err
	}

	grants := []*provider.Grant{}
	for _, a := range acls {
		grantee := &provider.Grantee{
			Id:   &userpb.UserId{OpaqueId: a.Qualifier},
			Type: fs.getGranteeType(a.Type),
		}
		grants = append(grants, &provider.Grant{
			Grantee:     grantee,
			Permissions: fs.getGrantPermissionSet(a.Permissions),
		})
	}

	return grants, nil
}

func (fs *eosStorage) getGranteeType(aclType string) provider.GranteeType {
	switch aclType {
	case "u":
		return provider.GranteeType_GRANTEE_TYPE_USER
	case "g":
		return provider.GranteeType_GRANTEE_TYPE_GROUP
	default:
		return provider.GranteeType_GRANTEE_TYPE_INVALID
	}
}

// TODO(labkode): add more fine grained controls.
// EOS acls are a mix of ACLs and POSIX permissions. More details can be found in
// https://github.com/cern-eos/eos/blob/master/doc/configuration/permission.rst
// TODO we need to evaluate all acls in the list at once to properly forbid (!) and overwrite (+) permissions
// This is ugly, because those are actually negative permissions ...
func (fs *eosStorage) getGrantPermissionSet(mode string) *provider.ResourcePermissions {

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

	// sharing
	// TODO AddGrant
	// TODO ListGrants
	// TODO RemoveGrant
	// TODO UpdateGrant

	// trash
	// TODO ListRecycle
	// TODO RestoreRecycleItem
	// TODO PurgeRecycle

	// versions
	// TODO ListFileVersions
	// TODO RestoreFileVersion

	// ?
	// TODO GetPath
	// TODO GetQuota
	return p
}

func (fs *eosStorage) GetMD(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}
	fn, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, u.Username, fn)
	if err != nil {
		return nil, err
	}
	fi := fs.convertToResourceInfo(ctx, eosFileInfo)
	return fi, nil
}

func (fs *eosStorage) ListFolder(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	fn, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	eosFileInfos, err := fs.c.List(ctx, u.Username, fn)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error listing")
	}

	finfos := []*provider.ResourceInfo{}
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
	return finfos, nil
}

func (fs *eosStorage) GetQuota(ctx context.Context) (int, int, error) {
	u, err := getUser(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "eos: no user in ctx")
	}
	return fs.c.GetQuota(ctx, u.Username, fs.conf.Namespace)
}

func (fs *eosStorage) GetHome(ctx context.Context) (string, error) {
	if !fs.conf.EnableHome {
		return "", errtypes.NotSupported("eos: get home not supported")
	}

	home := fs.wrap(ctx, "/")
	return home, nil
}

func (fs *eosStorage) createShadowHome(ctx context.Context) error {
	home := fs.wrapShadow(ctx, "/")
	_, err := fs.c.GetFileInfoByPath(ctx, "root", home)
	if err == nil { // home already exists
		return nil
	}

	// TODO(labkode): abort on any error that is not found
	if _, ok := err.(errtypes.IsNotFound); !ok {
		return errors.Wrap(err, "eos: error verifying if user home directory exists")
	}

	// TODO(labkode): only trigger creation on not found, copy from CERNBox logic.
	err = fs.c.CreateDir(ctx, "root", home)
	if err != nil {
		// EOS will return success on mkdir over an existing directory.
		return errors.Wrap(err, "eos: error creating dir")
	}

	err = fs.c.Chmod(ctx, "root", "2770", home)
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
		err = fs.c.SetAttr(ctx, "root", attr, home)
		if err != nil {
			return errors.Wrap(err, "eos: error setting attribute")
		}

	}

	// create shadow folders
	shadowFolders := []string{fs.conf.ShadowShareFolder}
	for _, sf := range shadowFolders {
		sf = path.Join(home, sf)
		err = fs.c.CreateDir(ctx, "root", sf)
		if err != nil {
			// EOS will return success on mkdir over an existing directory.
			return errors.Wrap(err, "eos: error creating dir")
		}

	}

	return nil
}

func (fs *eosStorage) createNominalHome(ctx context.Context) error {
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

	// TODO(labkode): only trigger creation on not found, copy from CERNBox logic.
	err = fs.c.CreateDir(ctx, "root", home)
	if err != nil {
		// EOS will return success on mkdir over an existing directory.
		return errors.Wrap(err, "eos: error creating dir")
	}
	err = fs.c.Chown(ctx, "root", u.Username, home)
	if err != nil {
		return errors.Wrap(err, "eos: error chowning directory")
	}

	err = fs.c.Chmod(ctx, "root", "2770", home)
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
		err = fs.c.SetAttr(ctx, "root", attr, home)
		if err != nil {
			return errors.Wrap(err, "eos: error setting attribute")
		}

	}
	return nil
}

func (fs *eosStorage) CreateHome(ctx context.Context) error {
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

func (fs *eosStorage) CreateDir(ctx context.Context, fn string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	fn = fs.wrap(ctx, fn)
	return fs.c.CreateDir(ctx, u.Username, fn)
}

func (fs *eosStorage) CreateReference(ctx context.Context, p string, targetURI *url.URL) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	ref := &provider.Reference{
		Spec: &provider.Reference_Path{
			Path: p,
		},
	}

	fn, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	// TODO(labkode): with grpc we can touch with xattrs.
	// Current mechanism is: touch to hidden file, set xattr, rename.
	dir, base := path.Split(fn)
	tmp := path.Join(dir, fmt.Sprintf(".sys.r#.%s", base))
	if err := fs.c.Touch(ctx, u.Username, tmp); err != nil {
		err = errors.Wrapf(err, "eos: error creating temporary ref file")
		return err
	}

	// set xattr on ref
	attr := &eosclient.Attribute{
		Type: eosclient.UserAttr,
		Key:  "reva.ref",
		Val:  targetURI.String(),
	}

	if err := fs.c.SetAttr(ctx, u.Username, attr, tmp); err != nil {
		err = errors.Wrapf(err, "eos: error setting reva.ref attr on file: %q", tmp)
		return err
	}

	// rename to have the file visible in user space.
	if err := fs.c.Rename(ctx, u.Username, tmp, fn); err != nil {
		err = errors.Wrapf(err, "eos: error renaming from: %q to %q", tmp, fn)
		return err
	}

	return nil
}

func (fs *eosStorage) Delete(ctx context.Context, ref *provider.Reference) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	fn, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	return fs.c.Remove(ctx, u.Username, fn)
}

func (fs *eosStorage) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
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

	return fs.c.Rename(ctx, u.Username, oldPath, newPath)
}

func (fs *eosStorage) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}
	fn := fs.wrap(ctx, p)

	return fs.c.Read(ctx, u.Username, fn)
}

func (fs *eosStorage) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}
	fn := fs.wrap(ctx, p)

	return fs.c.Write(ctx, u.Username, fn, r)
}

func (fs *eosStorage) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
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

func (fs *eosStorage) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}
	fn := fs.wrap(ctx, p)

	fn = fs.wrap(ctx, fn)
	return fs.c.ReadVersion(ctx, u.Username, fn, revisionKey)
}

func (fs *eosStorage) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}
	fn := fs.wrap(ctx, p)

	return fs.c.RollbackToVersion(ctx, u.Username, fn, revisionKey)
}

func (fs *eosStorage) PurgeRecycleItem(ctx context.Context, key string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "storage_eos: no user in ctx")
	}
	return fs.c.RestoreDeletedEntry(ctx, u.Username, key)
}

func (fs *eosStorage) EmptyRecycle(ctx context.Context) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}
	return fs.c.PurgeDeletedEntries(ctx, u.Username)
}

func (fs *eosStorage) ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error) {
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

func (fs *eosStorage) RestoreRecycleItem(ctx context.Context, key string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}
	return fs.c.RestoreDeletedEntry(ctx, u.Username, key)
}

func (fs *eosStorage) convertToRecycleItem(ctx context.Context, eosDeletedItem *eosclient.DeletedEntry) *provider.RecycleItem {
	recycleItem := &provider.RecycleItem{
		Path:         fs.unwrap(ctx, eosDeletedItem.RestorePath),
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

func (fs *eosStorage) convertToRevision(ctx context.Context, eosFileInfo *eosclient.FileInfo) *provider.FileVersion {
	md := fs.convertToResourceInfo(ctx, eosFileInfo)
	revision := &provider.FileVersion{
		Key:   path.Base(md.Path),
		Size:  md.Size,
		Mtime: md.Mtime.Seconds, // TODO do we need nanos here?
	}
	return revision
}

func (fs *eosStorage) convertToResourceInfo(ctx context.Context, eosFileInfo *eosclient.FileInfo) *provider.ResourceInfo {
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

	return &provider.ResourceInfo{
		Id:            &provider.ResourceId{OpaqueId: fmt.Sprintf("%d", eosFileInfo.Inode)},
		Path:          path,
		Owner:         &userpb.UserId{OpaqueId: username},
		Type:          getResourceType(eosFileInfo.IsDir),
		Etag:          eosFileInfo.ETag,
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
