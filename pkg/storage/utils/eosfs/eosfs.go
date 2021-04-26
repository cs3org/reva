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

package eosfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/eosclient"
	"github.com/cs3org/reva/pkg/eosclient/eosbinary"
	"github.com/cs3org/reva/pkg/eosclient/eosgrpc"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/acl"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/storage/utils/grants"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"
)

const (
	refTargetAttrKey = "reva.target"
)

const (
	// SystemAttr is the system extended attribute.
	SystemAttr eosclient.AttrType = iota
	// UserAttr is the user extended attribute.
	UserAttr
)

var hiddenReg = regexp.MustCompile(`\.sys\..#.`)

func (c *Config) init() {
	c.Namespace = path.Clean(c.Namespace)
	if !strings.HasPrefix(c.Namespace, "/") {
		c.Namespace = "/"
	}

	if c.ShadowNamespace == "" {
		c.ShadowNamespace = path.Join(c.Namespace, ".shadow")
	}

	// Quota node defaults to namespace if empty
	if c.QuotaNode == "" {
		c.QuotaNode = c.Namespace
	}

	if c.DefaultQuotaBytes == 0 {
		c.DefaultQuotaBytes = 1000000000000 // 1 TB
	}
	if c.DefaultQuotaFiles == 0 {
		c.DefaultQuotaFiles = 1000000 // 1 Million
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
		c.XrdcopyBinary = "/opt/eos/xrootd/bin/xrdcopy"
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

	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

type eosfs struct {
	c             eosclient.EOSClient
	conf          *Config
	chunkHandler  *chunking.ChunkHandler
	singleUserUID string
	singleUserGID string
	userIDCache   sync.Map
}

// NewEOSFS returns a storage.FS interface implementation that connects to an EOS instance
func NewEOSFS(c *Config) (storage.FS, error) {
	c.init()

	// bail out if keytab is not found.
	if c.UseKeytab {
		if _, err := os.Stat(c.Keytab); err != nil {
			err = errors.Wrapf(err, "eos: keytab not accessible at location: %s", err)
			return nil, err
		}
	}

	var eosClient eosclient.EOSClient
	if c.UseGRPC {
		eosClientOpts := &eosgrpc.Options{
			XrdcopyBinary:       c.XrdcopyBinary,
			URL:                 c.MasterURL,
			GrpcURI:             c.GrpcURI,
			CacheDirectory:      c.CacheDirectory,
			ForceSingleUserMode: c.ForceSingleUserMode,
			SingleUsername:      c.SingleUsername,
			UseKeytab:           c.UseKeytab,
			Keytab:              c.Keytab,
			Authkey:             c.GRPCAuthkey,
			SecProtocol:         c.SecProtocol,
			VersionInvariant:    c.VersionInvariant,
		}
		eosClient = eosgrpc.New(eosClientOpts)
	} else {
		eosClientOpts := &eosbinary.Options{
			XrdcopyBinary:       c.XrdcopyBinary,
			URL:                 c.MasterURL,
			EosBinary:           c.EosBinary,
			CacheDirectory:      c.CacheDirectory,
			ForceSingleUserMode: c.ForceSingleUserMode,
			SingleUsername:      c.SingleUsername,
			UseKeytab:           c.UseKeytab,
			Keytab:              c.Keytab,
			SecProtocol:         c.SecProtocol,
			VersionInvariant:    c.VersionInvariant,
		}
		eosClient = eosbinary.New(eosClientOpts)
	}

	eosfs := &eosfs{
		c:            eosClient,
		conf:         c,
		chunkHandler: chunking.NewChunkHandler(c.CacheDirectory),
		userIDCache:  sync.Map{},
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

func (fs *eosfs) unwrap(ctx context.Context, internal string) (string, error) {
	log := appctx.GetLogger(ctx)
	layout := fs.getLayout(ctx)
	ns, err := fs.getNsMatch(internal, []string{fs.conf.Namespace, fs.conf.ShadowNamespace})
	if err != nil {
		return "", err
	}
	external, err := fs.unwrapInternal(ctx, ns, internal, layout)
	if err != nil {
		return "", err
	}
	log.Debug().Msgf("eos: unwrap: internal=%s external=%s", internal, external)
	return external, nil
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

func (fs *eosfs) getNsMatch(internal string, nss []string) (string, error) {
	var match string

	for _, ns := range nss {
		if strings.HasPrefix(internal, ns) && len(ns) > len(match) {
			match = ns
		}
	}

	if match == "" {
		return "", errtypes.NotFound(fmt.Sprintf("eos: path is outside namespaces: path=%s namespaces=%+v", internal, nss))
	}

	return match, nil
}

func (fs *eosfs) unwrapInternal(ctx context.Context, ns, np, layout string) (string, error) {
	log := appctx.GetLogger(ctx)
	trim := path.Join(ns, layout)

	if !strings.HasPrefix(np, trim) {
		return "", errtypes.NotFound(fmt.Sprintf("eos: path is outside the directory of the logged-in user: internal=%s trim=%s namespace=%+v", np, trim, ns))
	}

	external := strings.TrimPrefix(np, trim)

	if external == "" {
		external = "/"
	}

	log.Debug().Msgf("eos: unwrapInternal: trim=%s external=%s ns=%s np=%s", trim, external, ns, np)

	return external, nil
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

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return "", err
	}

	eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, uid, gid, fid)
	if err != nil {
		return "", errors.Wrap(err, "eos: error getting file info by inode")
	}

	return fs.unwrap(ctx, eosFileInfo.File)
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

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return "", err
	}

	eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, uid, gid, fileID)
	if err != nil {
		return "", errors.Wrap(err, "eos: error getting file info by inode")
	}

	return fs.unwrap(ctx, eosFileInfo.File)
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

	eosACL, err := fs.getEosACL(ctx, g)
	if err != nil {
		return err
	}

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
	}

	rootUID, rootGID, err := fs.getRootUIDAndGID(ctx)
	if err != nil {
		return err
	}

	err = fs.c.AddACL(ctx, uid, gid, rootUID, rootGID, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "eos: error adding acl")
	}

	return nil
}

func (fs *eosfs) getEosACL(ctx context.Context, g *provider.Grant) (*acl.Entry, error) {
	permissions, err := grants.GetACLPerm(g.Permissions)
	if err != nil {
		return nil, err
	}
	t, err := grants.GetACLType(g.Grantee.Type)
	if err != nil {
		return nil, err
	}

	var qualifier string
	if t == acl.TypeUser {
		// since EOS Citrine ACLs are stored with uid, we need to convert username to
		// uid only for users.
		qualifier, _, err = fs.getUIDGateway(ctx, g.Grantee.GetUserId())
		if err != nil {
			return nil, err
		}
	} else {
		qualifier = g.Grantee.GetGroupId().OpaqueId
	}

	eosACL := &acl.Entry{
		Qualifier:   qualifier,
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

	var recipient string
	if eosACLType == acl.TypeUser {
		// since EOS Citrine ACLs are stored with uid, we need to convert username to uid
		recipient, _, err = fs.getUIDGateway(ctx, g.Grantee.GetUserId())
		if err != nil {
			return err
		}
	} else {
		recipient = g.Grantee.GetGroupId().OpaqueId
	}

	eosACL := &acl.Entry{
		Qualifier: recipient,
		Type:      eosACLType,
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	fn := fs.wrap(ctx, p)

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
	}

	rootUID, rootGID, err := fs.getRootUIDAndGID(ctx)
	if err != nil {
		return err
	}

	err = fs.c.RemoveACL(ctx, uid, gid, rootUID, rootGID, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "eos: error removing acl")
	}
	return nil
}

func (fs *eosfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return fs.AddGrant(ctx, ref, g)
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

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return nil, err
	}

	acls, err := fs.c.ListACLs(ctx, uid, gid, fn)
	if err != nil {
		return nil, err
	}

	grantList := []*provider.Grant{}
	for _, a := range acls {
		var grantee *provider.Grantee
		if a.Type == acl.TypeUser {
			// EOS Citrine ACLs are stored with uid for users.
			// This needs to be resolved to the user opaque ID.
			qualifier, err := fs.getUserIDGateway(ctx, a.Qualifier)
			if err != nil {
				return nil, err
			}
			grantee = &provider.Grantee{
				Id:   &provider.Grantee_UserId{UserId: qualifier},
				Type: grants.GetGranteeType(a.Type),
			}
		} else {
			grantee = &provider.Grantee{
				Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: a.Qualifier}},
				Type: grants.GetGranteeType(a.Type),
			}
		}
		grantList = append(grantList, &provider.Grant{
			Grantee:     grantee,
			Permissions: grants.GetGrantPermissionSet(a.Permissions, true),
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

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return nil, err
	}

	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, uid, gid, fn)
	if err != nil {
		return nil, err
	}

	return fs.convertToResourceInfo(ctx, eosFileInfo, false)
}

func (fs *eosfs) getMDShareFolder(ctx context.Context, p string, mdKeys []string) (*provider.ResourceInfo, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	fn := fs.wrapShadow(ctx, p)

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return nil, err
	}

	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, uid, gid, fn)
	if err != nil {
		return nil, err
	}
	// TODO(labkode): diff between root (dir) and children (ref)

	if fs.isShareFolderRoot(ctx, p) {
		return fs.convertToResourceInfo(ctx, eosFileInfo, false)
	}
	return fs.convertToFileReference(ctx, eosFileInfo)
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

	// if path is home we need to add in the response any shadow folder in the shadow homedirectory.
	if fs.conf.EnableHome {
		log.Debug().Msg("home enabled")
		if strings.HasPrefix(p, "/") {
			return fs.listWithHome(ctx, "/", p)
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

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return nil, err
	}

	fn := fs.wrap(ctx, p)
	virtualView := false
	if !fs.conf.EnableHome && filepath.Dir(fn) == filepath.Clean(fs.conf.Namespace) {
		virtualView = true
	}

	eosFileInfos, err := fs.c.List(ctx, uid, gid, fn)
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

		// Remove the hidden folders in the topmost directory
		if finfo, err := fs.convertToResourceInfo(ctx, eosFileInfo, virtualView); err == nil && finfo.Path != "/" && !strings.HasPrefix(finfo.Path, "/.") {
			finfos = append(finfos, finfo)
		}
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

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return nil, err
	}

	fns := []string{fs.wrap(ctx, home), fs.wrapShadow(ctx, home)}

	finfos := []*provider.ResourceInfo{}
	for _, fn := range fns {
		eosFileInfos, err := fs.c.List(ctx, uid, gid, fn)
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

			if finfo, err := fs.convertToResourceInfo(ctx, eosFileInfo, false); err == nil && finfo.Path != "/" && !strings.HasPrefix(finfo.Path, "/.") {
				finfos = append(finfos, finfo)
			}
		}

	}
	return finfos, nil
}

func (fs *eosfs) listShareFolderRoot(ctx context.Context, p string) (finfos []*provider.ResourceInfo, err error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}
	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return nil, err
	}

	fn := fs.wrapShadow(ctx, p)

	eosFileInfos, err := fs.c.List(ctx, uid, gid, fn)
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

		if finfo, err := fs.convertToFileReference(ctx, eosFileInfo); err == nil {
			finfos = append(finfos, finfo)
		}
	}

	return finfos, nil
}

func (fs *eosfs) GetQuota(ctx context.Context) (uint64, uint64, error) {
	u, err := getUser(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "eos: no user in ctx")
	}

	rootUID, rootGID, err := fs.getRootUIDAndGID(ctx)
	if err != nil {
		return 0, 0, err
	}

	qi, err := fs.c.GetQuota(ctx, u.Username, rootUID, rootGID, fs.conf.QuotaNode)
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
	uid, gid, err := fs.getRootUIDAndGID(ctx)
	if err != nil {
		return nil
	}
	home := fs.wrapShadow(ctx, "/")
	shadowFolders := []string{fs.conf.ShareFolder}

	for _, sf := range shadowFolders {
		fn := path.Join(home, sf)
		_, err = fs.c.GetFileInfoByPath(ctx, uid, gid, fn)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); !ok {
				return errors.Wrap(err, "eos: error verifying if shadow directory exists")
			}
			err = fs.createUserDir(ctx, u, fn, false)
			if err != nil {
				return err
			}
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
	uid, gid, err := fs.getRootUIDAndGID(ctx)
	if err != nil {
		return nil
	}
	_, err = fs.c.GetFileInfoByPath(ctx, uid, gid, home)
	if err == nil { // home already exists
		return nil
	}

	if _, ok := err.(errtypes.IsNotFound); !ok {
		return errors.Wrap(err, "eos: error verifying if user home directory exists")
	}

	err = fs.createUserDir(ctx, u, home, false)
	if err != nil {
		err := errors.Wrap(err, "eosfs: error creating user dir")
		return err
	}

	// set quota for user
	quotaInfo := &eosclient.SetQuotaInfo{
		Username:  u.Username,
		MaxBytes:  fs.conf.DefaultQuotaBytes,
		MaxFiles:  fs.conf.DefaultQuotaFiles,
		QuotaNode: fs.conf.QuotaNode,
	}

	err = fs.c.SetQuota(ctx, uid, gid, quotaInfo)
	if err != nil {
		err := errors.Wrap(err, "eosfs: error setting quota")
		return err
	}

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

func (fs *eosfs) createUserDir(ctx context.Context, u *userpb.User, path string, recursiveAttr bool) error {
	uid, gid, err := fs.getRootUIDAndGID(ctx)
	if err != nil {
		return nil
	}

	chownUID, chownGID, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
	}

	err = fs.c.CreateDir(ctx, uid, gid, path)
	if err != nil {
		// EOS will return success on mkdir over an existing directory.
		return errors.Wrap(err, "eos: error creating dir")
	}

	err = fs.c.Chown(ctx, uid, gid, chownUID, chownGID, path)
	if err != nil {
		return errors.Wrap(err, "eos: error chowning directory")
	}

	err = fs.c.Chmod(ctx, uid, gid, "2770", path)
	if err != nil {
		return errors.Wrap(err, "eos: error chmoding directory")
	}

	attrs := []*eosclient.Attribute{
		&eosclient.Attribute{
			Type: SystemAttr,
			Key:  "mask",
			Val:  "700",
		},
		&eosclient.Attribute{
			Type: SystemAttr,
			Key:  "allow.oc.sync",
			Val:  "1",
		},
		&eosclient.Attribute{
			Type: SystemAttr,
			Key:  "mtime.propagation",
			Val:  "1",
		},
		&eosclient.Attribute{
			Type: SystemAttr,
			Key:  "forced.atomic",
			Val:  "1",
		},
	}

	for _, attr := range attrs {
		err = fs.c.SetAttr(ctx, uid, gid, attr, recursiveAttr, path)
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

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
	}

	log.Info().Msgf("eos: createdir: path=%s", p)

	if fs.isShareFolder(ctx, p) {
		return errtypes.PermissionDenied("eos: cannot create folder under the share folder")
	}

	fn := fs.wrap(ctx, p)
	return fs.c.CreateDir(ctx, uid, gid, fn)
}

func (fs *eosfs) CreateReference(ctx context.Context, p string, targetURI *url.URL) error {
	// TODO(labkode): for the time being we only allow to create references
	// on the virtual share folder to not pollute the nominal user tree.
	if !fs.isShareFolder(ctx, p) {
		return errtypes.PermissionDenied("eos: cannot create references outside the share folder: share_folder=" + fs.conf.ShareFolder + " path=" + p)
	}
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	fn := fs.wrapShadow(ctx, p)

	// TODO(labkode): with grpc we can create a file touching with xattrs.
	// Current mechanism is: touch to hidden dir, set xattr, rename.
	dir, base := path.Split(fn)
	tmp := path.Join(dir, fmt.Sprintf(".sys.reva#.%s", base))
	uid, gid, err := fs.getRootUIDAndGID(ctx)
	if err != nil {
		return nil
	}

	if err := fs.createUserDir(ctx, u, tmp, false); err != nil {
		err = errors.Wrapf(err, "eos: error creating temporary ref file")
		return err
	}

	// set xattr on ref
	attr := &eosclient.Attribute{
		Type: UserAttr,
		Key:  refTargetAttrKey,
		Val:  targetURI.String(),
	}

	if err := fs.c.SetAttr(ctx, uid, gid, attr, false, tmp); err != nil {
		err = errors.Wrapf(err, "eos: error setting reva.ref attr on file: %q", tmp)
		return err
	}

	// rename to have the file visible in user space.
	if err := fs.c.Rename(ctx, uid, gid, tmp, fn); err != nil {
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

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return fs.deleteShadow(ctx, p)
	}

	fn := fs.wrap(ctx, p)

	return fs.c.Remove(ctx, uid, gid, fn)
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

		uid, gid, err := fs.getUserUIDAndGID(ctx, u)
		if err != nil {
			return err
		}

		fn := fs.wrapShadow(ctx, p)
		return fs.c.Remove(ctx, uid, gid, fn)
	}

	return errors.New("eos: shadow delete of share folder that is neither root nor child. path=" + p)
}

func (fs *eosfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
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
	return fs.c.Rename(ctx, uid, gid, oldFn, newFn)
}

func (fs *eosfs) moveShadow(ctx context.Context, oldPath, newPath string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
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
	return fs.c.Rename(ctx, uid, gid, oldfn, newfn)
}

func (fs *eosfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return nil, err
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return nil, errtypes.PermissionDenied("eos: cannot download under the virtual share folder")
	}

	fn := fs.wrap(ctx, p)
	return fs.c.Read(ctx, uid, gid, fn)
}

func (fs *eosfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return nil, err
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return nil, errtypes.PermissionDenied("eos: cannot list revisions under the virtual share folder")
	}

	fn := fs.wrap(ctx, p)

	eosRevisions, err := fs.c.ListVersions(ctx, uid, gid, fn)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error listing versions")
	}
	revisions := []*provider.FileVersion{}
	for _, eosRev := range eosRevisions {
		if rev, err := fs.convertToRevision(ctx, eosRev); err == nil {
			revisions = append(revisions, rev)
		}
	}
	return revisions, nil
}

func (fs *eosfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return nil, err
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
	return fs.c.ReadVersion(ctx, uid, gid, fn, revisionKey)
}

func (fs *eosfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
	}

	p, err := fs.resolve(ctx, u, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	if fs.isShareFolder(ctx, p) {
		return errtypes.PermissionDenied("eos: cannot restore revision under the virtual share folder")
	}

	fn := fs.wrap(ctx, p)

	return fs.c.RollbackToVersion(ctx, uid, gid, fn, revisionKey)
}

func (fs *eosfs) PurgeRecycleItem(ctx context.Context, key string) error {
	return errtypes.NotSupported("eos: operation not supported")
}

func (fs *eosfs) EmptyRecycle(ctx context.Context) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
	}

	return fs.c.PurgeDeletedEntries(ctx, uid, gid)
}

func (fs *eosfs) ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error) {
	u, err := getUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return nil, err
	}

	eosDeletedEntries, err := fs.c.ListDeletedEntries(ctx, uid, gid)
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
		if recycleItem, err := fs.convertToRecycleItem(ctx, entry); err == nil {
			recycleEntries = append(recycleEntries, recycleItem)
		}
	}
	return recycleEntries, nil
}

func (fs *eosfs) RestoreRecycleItem(ctx context.Context, key, restorePath string) error {
	u, err := getUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return err
	}

	return fs.c.RestoreDeletedEntry(ctx, uid, gid, key)
}

func (fs *eosfs) convertToRecycleItem(ctx context.Context, eosDeletedItem *eosclient.DeletedEntry) (*provider.RecycleItem, error) {
	path, err := fs.unwrap(ctx, eosDeletedItem.RestorePath)
	if err != nil {
		return nil, err
	}
	recycleItem := &provider.RecycleItem{
		Path:         path,
		Key:          eosDeletedItem.RestoreKey,
		Size:         eosDeletedItem.Size,
		DeletionTime: &types.Timestamp{Seconds: eosDeletedItem.DeletionMTime},
	}
	if eosDeletedItem.IsDir {
		recycleItem.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	} else {
		// TODO(labkode): if eos returns more types oin the future we need to map them.
		recycleItem.Type = provider.ResourceType_RESOURCE_TYPE_FILE
	}
	return recycleItem, nil
}

func (fs *eosfs) convertToRevision(ctx context.Context, eosFileInfo *eosclient.FileInfo) (*provider.FileVersion, error) {
	md, err := fs.convertToResourceInfo(ctx, eosFileInfo, false)
	if err != nil {
		return nil, err
	}
	revision := &provider.FileVersion{
		Key:   path.Base(md.Path),
		Size:  md.Size,
		Mtime: md.Mtime.Seconds, // TODO do we need nanos here?
		Etag:  md.Etag,
	}
	return revision, nil
}

func (fs *eosfs) convertToResourceInfo(ctx context.Context, eosFileInfo *eosclient.FileInfo, virtualView bool) (*provider.ResourceInfo, error) {
	return fs.convert(ctx, eosFileInfo, virtualView)
}

func (fs *eosfs) convertToFileReference(ctx context.Context, eosFileInfo *eosclient.FileInfo) (*provider.ResourceInfo, error) {
	info, err := fs.convert(ctx, eosFileInfo, false)
	if err != nil {
		return nil, err
	}
	info.Type = provider.ResourceType_RESOURCE_TYPE_REFERENCE
	val, ok := eosFileInfo.Attrs["user.reva.target"]
	if !ok || val == "" {
		return nil, errtypes.InternalError("eos: reference does not contain target: target=" + val + " file=" + eosFileInfo.File)
	}
	info.Target = val
	return info, nil
}

// permissionSet returns the permission set for the current user
func (fs *eosfs) permissionSet(ctx context.Context, eosFileInfo *eosclient.FileInfo, owner *userpb.UserId) *provider.ResourcePermissions {
	u, ok := user.ContextGetUser(ctx)
	if !ok || owner == nil || u.Id == nil {
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

	uid, gid, err := fs.getUserUIDAndGID(ctx, u)
	if err != nil {
		return &provider.ResourcePermissions{
			// no permissions
		}
	}

	var perm string
	for _, e := range eosFileInfo.SysACL.Entries {
		if e.Qualifier == uid || e.Qualifier == gid {
			perm = e.Permissions
		}
	}

	return grants.GetGrantPermissionSet(perm, eosFileInfo.IsDir)
}

func (fs *eosfs) convert(ctx context.Context, eosFileInfo *eosclient.FileInfo, virtualView bool) (*provider.ResourceInfo, error) {
	path, err := fs.unwrap(ctx, eosFileInfo.File)
	if err != nil {
		return nil, err
	}

	size := eosFileInfo.Size
	if eosFileInfo.IsDir {
		size = eosFileInfo.TreeSize
	}

	owner := &userpb.UserId{}
	if !virtualView {
		owner, err = fs.getUserIDGateway(ctx, strconv.FormatUint(eosFileInfo.UID, 10))
		if err != nil {
			sublog := appctx.GetLogger(ctx).With().Logger()
			sublog.Warn().Uint64("uid", eosFileInfo.UID).Msg("could not lookup userid, leaving empty")
		}
	}

	info := &provider.ResourceInfo{
		Id:            &provider.ResourceId{OpaqueId: fmt.Sprintf("%d", eosFileInfo.Inode)},
		Path:          path,
		Owner:         owner,
		Etag:          fmt.Sprintf("\"%s\"", strings.Trim(eosFileInfo.ETag, "\"")),
		MimeType:      mime.Detect(eosFileInfo.IsDir, path),
		Size:          size,
		PermissionSet: fs.permissionSet(ctx, eosFileInfo, owner),
		Mtime: &types.Timestamp{
			Seconds: eosFileInfo.MTimeSec,
			Nanos:   eosFileInfo.MTimeNanos,
		},
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"eos": {
					Decoder: "json",
					Value:   fs.getEosMetadata(eosFileInfo),
				},
			},
		},
	}

	if eosFileInfo.IsDir {
		info.Opaque.Map["disable_tus"] = &types.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte("true"),
		}
	}

	info.Type = getResourceType(eosFileInfo.IsDir)
	return info, nil
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

func (fs *eosfs) extractUIDAndGID(u *userpb.User) (string, string, error) {
	var uid, gid string
	if u.Opaque != nil && u.Opaque.Map != nil {
		if uidObj, ok := u.Opaque.Map["uid"]; ok {
			if uidObj.Decoder == "plain" {
				uid = string(uidObj.Value)
			}
		}
		if gidObj, ok := u.Opaque.Map["gid"]; ok {
			if gidObj.Decoder == "plain" {
				gid = string(gidObj.Value)
			}
		}
	}
	if uid == "" || gid == "" {
		return "", "", errors.New("eos: uid or gid missing for user")
	}
	return uid, gid, nil
}

func (fs *eosfs) getUIDGateway(ctx context.Context, u *userpb.UserId) (string, string, error) {
	client, err := pool.GetGatewayServiceClient(fs.conf.GatewaySvc)
	if err != nil {
		return "", "", errors.Wrap(err, "eos: error getting gateway grpc client")
	}
	getUserResp, err := client.GetUser(ctx, &userpb.GetUserRequest{
		UserId: u,
	})
	if err != nil {
		return "", "", errors.Wrap(err, "eos: error getting user")
	}
	if getUserResp.Status.Code != rpc.Code_CODE_OK {
		return "", "", errors.Wrap(err, "eos: grpc get user failed")
	}
	return fs.extractUIDAndGID(getUserResp.User)
}

func (fs *eosfs) getUserIDGateway(ctx context.Context, uid string) (*userpb.UserId, error) {
	if userIDInterface, ok := fs.userIDCache.Load(uid); ok {
		return userIDInterface.(*userpb.UserId), nil
	}
	client, err := pool.GetGatewayServiceClient(fs.conf.GatewaySvc)
	if err != nil {
		return nil, errors.Wrap(err, "eos: error getting gateway grpc client")
	}
	getUserResp, err := client.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim: "uid",
		Value: uid,
	})
	if err != nil {
		return nil, errors.Wrap(err, "eos: error getting user")
	}
	if getUserResp.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.Wrap(err, "eos: grpc get user failed")
	}

	fs.userIDCache.Store(uid, getUserResp.User.Id)
	return getUserResp.User.Id, nil
}

func (fs *eosfs) getUserUIDAndGID(ctx context.Context, u *userpb.User) (string, string, error) {
	if fs.conf.ForceSingleUserMode {
		if fs.singleUserUID != "" && fs.singleUserGID != "" {
			return fs.singleUserUID, fs.singleUserGID, nil
		}
		uid, gid, err := fs.getUIDGateway(ctx, &userpb.UserId{OpaqueId: fs.conf.SingleUsername})
		fs.singleUserUID = uid
		fs.singleUserGID = gid
		return fs.singleUserUID, fs.singleUserGID, err
	}
	return fs.extractUIDAndGID(u)
}

func (fs *eosfs) getRootUIDAndGID(ctx context.Context) (string, string, error) {
	if fs.conf.ForceSingleUserMode {
		if fs.singleUserUID != "" && fs.singleUserGID != "" {
			return fs.singleUserUID, fs.singleUserGID, nil
		}
		uid, gid, err := fs.getUIDGateway(ctx, &userpb.UserId{OpaqueId: fs.conf.SingleUsername})
		fs.singleUserUID = uid
		fs.singleUserGID = gid
		return fs.singleUserUID, fs.singleUserGID, err
	}
	return "0", "0", nil
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
