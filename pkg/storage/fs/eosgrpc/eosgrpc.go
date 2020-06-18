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

package eosgrpc

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
	"github.com/cs3org/reva/pkg/eosclientgrpc"
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

const (
	refTargetAttrKey = "reva.target"
)

func init() {

	registry.Register("eosgrpc", New)
}

var hiddenReg = regexp.MustCompile(`\.sys\..#.`)

type eosfs struct {
	c    *eosclientgrpc.Client
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
	Namespace string `mapstructure:"namespace" docs:"/"`

	// ShadowNamespace for storing shadow data
	ShadowNamespace string `mapstructure:"shadow_namespace" docs:"/.shadow"`

	// ShareFolder defines the name of the folder in the
	// shadowed namespace. Ex: /eos/user/.shadow/h/hugo/MyShares
	ShareFolder string `mapstructure:"share_folder" docs:"/MyShares"`

	// Location of the eos binary.
	// Default is /usr/bin/eos.
	EosBinary string `mapstructure:"eos_binary" docs:"/usr/bin/eos"`

	// Location of the xrdcopy binary.
	// Default is /usr/bin/xrdcopy.
	XrdcopyBinary string `mapstructure:"xrdcopy_binary" docs:"/usr/bin/xrdcopy"`

	// URL of the Master EOS MGM.
	// Default is root://eos-example.org
	MasterURL string `mapstructure:"master_url" docs:"root://eos-example.org"`

	// URI of the EOS MGM grpc server
	// Default is empty
	GrpcURI string `mapstructure:"master_grpc_uri" docs:"root://eos-grpc-example.org"`

	// URL of the Slave EOS MGM.
	// Default is root://eos-example.org
	SlaveURL string `mapstructure:"slave_url" docs:"root://eos-example.org"`

	// Location on the local fs where to store reads.
	// Defaults to os.TempDir()
	CacheDirectory string `mapstructure:"cache_directory" docs:"/var/tmp/"`

	// SecProtocol specifies the xrootd security protocol to use between the server and EOS.
	SecProtocol string `mapstructure:"sec_protocol" docs:"-"`

	// Keytab specifies the location of the keytab to use to authenticate to EOS.
	Keytab string `mapstructure:"keytab"`

	// SingleUsername is the username to use when SingleUserMode is enabled
	SingleUsername string `mapstructure:"single_username"`

	// UserLayout wraps the internal path with user information.
	// Example: if conf.Namespace is /eos/user and received path is /docs
	// and the UserLayout is {{.Username}} the internal path will be:
	// /eos/user/<username>/docs
	UserLayout string `mapstructure:"user_layout" docs:"-"`

	// Enables logging of the commands executed
	// Defaults to false
	EnableLogging bool `mapstructure:"enable_logging" docs:"false"`

	// ShowHiddenSysFiles shows internal EOS files like
	// .sys.v# and .sys.a# files.
	ShowHiddenSysFiles bool `mapstructure:"show_hidden_sys_files" docs:"-"`

	// ForceSingleUserMode will force connections to EOS to use SingleUsername
	ForceSingleUserMode bool `mapstructure:"force_single_user_mode" docs:"false"`

	// UseKeyTabAuth changes will authenticate requests by using an EOS keytab.
	UseKeytab bool `mapstrucuture:"use_keytab" docs:"false"`

	// EnableHome enables the creation of home directories.
	EnableHome bool `mapstructure:"enable_home" docs:"false"`

	// Authkey is the key that authorizes this client to connect to the GRPC service
	// It's unclear whether this will be the final solution
	Authkey string `mapstructure:"authkey" docs:"-"`
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
	fmt.Printf("-- Initialising eosgrpc\n")

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

	eosClientOpts := &eosclientgrpc.Options{
		XrdcopyBinary:       c.XrdcopyBinary,
		URL:                 c.MasterURL,
		GrpcURI:             c.GrpcURI,
		CacheDirectory:      c.CacheDirectory,
		ForceSingleUserMode: c.ForceSingleUserMode,
		SingleUsername:      c.SingleUsername,
		UseKeytab:           c.UseKeytab,
		Keytab:              c.Keytab,
		Authkey:             c.Authkey,
		SecProtocol:         c.SecProtocol,
	}

	eosClient := eosclientgrpc.New(eosClientOpts)

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

func (fs *eosfs) wrapShadow(ctx context.Context, fn string) (internal string) {
	if fs.conf.EnableHome {
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

func (fs *eosfs) wrap(ctx context.Context, fn string) (internal string) {
	if fs.conf.EnableHome {
		layout, err := fs.GetHome(ctx)
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

// resolve takes in a request path or request id and returns the unwrapNominalped path.
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

func (fs *eosfs) getEosACL(g *provider.Grant) (*acl.Entry, error) {
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

func (fs *eosfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	return errtypes.NotSupported("eos: operation not supported")
}

func (fs *eosfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	return errtypes.NotSupported("eos: operation not supported")
}

func (fs *eosfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	eosACLType, err := getEosACLType(g.Grantee.Type)
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

func (fs *eosfs) getGranteeType(aclType string) provider.GranteeType {
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
func (fs *eosfs) getGrantPermissionSet(mode string) *provider.ResourcePermissions {

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

func (fs *eosfs) GetMD(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
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

	// if path is home we need to add in the response any shadow folder in the shadown homedirectory.
	if fs.conf.EnableHome {
		if fs.isShareFolder(ctx, p) {
			return fs.getMDShareFolder(ctx, p)
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

func (fs *eosfs) getMDShareFolder(ctx context.Context, p string) (*provider.ResourceInfo, error) {
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

func (fs *eosfs) ListFolder(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	// if path is home we need to add in the response any shadow folder in the shadown homedirectory.
	if fs.conf.EnableHome {
		home, err := fs.GetHome(ctx)
		if err != nil {
			err = errors.Wrap(err, "eos: error getting home")
			return nil, err
		}

		if strings.HasPrefix(p, home) {
			return fs.listWithHome(ctx, home, p)
		}
	}

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
	if p == home {
		return fs.listHome(ctx, home)
	}

	if fs.isShareFolderRoot(ctx, p) {
		return fs.listShareFolderRoot(ctx, p)
	}

	if fs.isShareFolderChild(ctx, p) {
		return nil, errtypes.PermissionDenied("eos: error listing folders inside the shared folder, only file references are stored inside")
	}

	// path points to a resource in the nominal home
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
	return fs.c.GetQuota(ctx, u.Username, fs.conf.Namespace)
}

func (fs *eosfs) GetHome(ctx context.Context) (string, error) {
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

	err = fs.c.Chmod(ctx, "root", "770", home)
	if err != nil {
		return errors.Wrap(err, "eos: error chmoding directory")
	}

	attrs := []*eosclientgrpc.Attribute{
		&eosclientgrpc.Attribute{
			Type: eosclientgrpc.SystemAttr,
			Key:  "mask",
			Val:  "700",
		},
		&eosclientgrpc.Attribute{
			Type: eosclientgrpc.SystemAttr,
			Key:  "allow.oc.sync",
			Val:  "1",
		},
		&eosclientgrpc.Attribute{
			Type: eosclientgrpc.SystemAttr,
			Key:  "mtime.propagation",
			Val:  "1",
		},
		&eosclientgrpc.Attribute{
			Type: eosclientgrpc.SystemAttr,
			Key:  "forced.atomic",
			Val:  "1",
		},
	}

	for _, attr := range attrs {
		err = fs.c.SetAttr(ctx, "root", attr, true, home)
		if err != nil {
			return errors.Wrap(err, "eos: error setting attribute")
		}

	}

	// create shadow folders
	shadowFolders := []string{fs.conf.ShareFolder}
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

	err = fs.c.Chmod(ctx, "root", "770", home)
	if err != nil {
		return errors.Wrap(err, "eos: error chmoding directory")
	}

	attrs := []*eosclientgrpc.Attribute{
		&eosclientgrpc.Attribute{
			Type: eosclientgrpc.SystemAttr,
			Key:  "mask",
			Val:  "700",
		},
		&eosclientgrpc.Attribute{
			Type: eosclientgrpc.SystemAttr,
			Key:  "allow.oc.sync",
			Val:  "1",
		},
		&eosclientgrpc.Attribute{
			Type: eosclientgrpc.SystemAttr,
			Key:  "mtime.propagation",
			Val:  "1",
		},
		&eosclientgrpc.Attribute{
			Type: eosclientgrpc.SystemAttr,
			Key:  "forced.atomic",
			Val:  "1",
		},
	}

	for _, attr := range attrs {
		err = fs.c.SetAttr(ctx, "root", attr, true, home)
		if err != nil {
			return errors.Wrap(err, "eos: error setting attribute")
		}

	}
	return nil
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
	attr := &eosclientgrpc.Attribute{
		Type: eosclientgrpc.UserAttr,
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

func (fs *eosfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return errtypes.PermissionDenied("eos: cannot download under the virtual share folder")
	}

	fn := fs.wrap(ctx, p)

	return fs.c.Write(ctx, u.Username, fn, r)
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

func (fs *eosfs) convertToRecycleItem(ctx context.Context, eosDeletedItem *eosclientgrpc.DeletedEntry) *provider.RecycleItem {
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

func (fs *eosfs) convertToRevision(ctx context.Context, eosFileInfo *eosclientgrpc.FileInfo) *provider.FileVersion {
	md := fs.convertToResourceInfo(ctx, eosFileInfo)
	revision := &provider.FileVersion{
		Key:   path.Base(md.Path),
		Size:  md.Size,
		Mtime: md.Mtime.Seconds, // TODO do we need nanos here?
	}
	return revision
}

func (fs *eosfs) convertToResourceInfo(ctx context.Context, eosFileInfo *eosclientgrpc.FileInfo) *provider.ResourceInfo {
	return fs.convert(ctx, eosFileInfo)
}

func (fs *eosfs) convertToFileReference(ctx context.Context, eosFileInfo *eosclientgrpc.FileInfo) *provider.ResourceInfo {
	info := fs.convert(ctx, eosFileInfo)
	info.Type = provider.ResourceType_RESOURCE_TYPE_REFERENCE
	val, ok := eosFileInfo.Attrs["user.reva.target"]
	if !ok || val == "" {
		panic("eos: reference does not contain target: target=" + val + " file=" + eosFileInfo.File)
	}
	info.Target = val
	return info
}

func (fs *eosfs) convert(ctx context.Context, eosFileInfo *eosclientgrpc.FileInfo) *provider.ResourceInfo {
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
				"eosgrpc": &types.OpaqueEntry{
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

func (fs *eosfs) getEosMetadata(finfo *eosclientgrpc.FileInfo) []byte {
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
