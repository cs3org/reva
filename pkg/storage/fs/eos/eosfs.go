// Copyright 2018-2026 CERN
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
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/bluele/gcache"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/spaces"

	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/mime"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/storage"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	eosbinary "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client/binary"
	eosgrpc "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client/grpc"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
	"github.com/cs3org/reva/v3/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/v3/pkg/storage/utils/grants"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

const (
	refTargetAttrKey = "reva.target"          // used as user attr to store a reference
	lwShareAttrKey   = "reva.lwshare"         // used to store grants to lightweight accounts
	lockPayloadKey   = "reva.lockpayload"     // used to store lock payloads
	eosLockKey       = "app.lock"             // this is the key known by EOS to enforce a lock.
	recycleIdKey     = "sys.forced.recycleid" // recycle id of the project
)

const (
	// SystemAttr is the system extended attribute.
	SystemAttr eosclient.AttrType = iota
	// UserAttr is the user extended attribute.
	UserAttr
)

var hiddenReg = regexp.MustCompile(`\.sys\..#.`)

var eosLockReg = regexp.MustCompile(`expires:\d+,type:[a-z]+,owner:.+:.+`)

func (c *Config) ApplyDefaults() {
	if !strings.HasPrefix(c.Namespace, "/") {
		c.Namespace = "/"
	}

	// Quota node defaults to namespace if empty
	if c.QuotaNode == "" {
		c.QuotaNode = c.Namespace
	}

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

	if c.UserIDCacheSize == 0 {
		c.UserIDCacheSize = 1000000
	}

	if c.UserIDCacheWarmupDepth == 0 {
		c.UserIDCacheWarmupDepth = 2
	}

	if c.TokenExpiry == 0 {
		c.TokenExpiry = 3600
	}

	if c.MaxRecycleEntries == 0 {
		c.MaxRecycleEntries = 2000
	}

	if c.MaxDaysInRecycleList == 0 {
		c.MaxDaysInRecycleList = 14
	}

	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

type Eosfs struct {
	c              eosclient.EOSClient
	conf           *Config
	chunkHandler   *chunking.ChunkHandler
	singleUserAuth eosclient.Authorization
	userIDCache    *ttlcache.Cache
	tokenCache     gcache.Cache
}

// NewEOSFS returns a storage.FS interface implementation that connects to an EOS instance.
func NewEOSFS(ctx context.Context, c *Config) (storage.FS, error) {
	c.ApplyDefaults()

	// bail out if keytab is not found.
	if c.UseKeytab {
		if _, err := os.Stat(c.Keytab); err != nil {
			err = errors.Wrapf(err, "eosfs: keytab not accessible at location: %s", err)
			return nil, err
		}
	}

	var eosClient eosclient.EOSClient
	var err error
	if c.UseGRPC {
		eosClientOpts := &eosgrpc.Options{
			XrdcopyBinary:      c.XrdcopyBinary,
			URL:                c.MasterURL,
			GrpcURI:            c.GrpcURI,
			CacheDirectory:     c.CacheDirectory,
			UseKeytab:          c.UseKeytab,
			Keytab:             c.Keytab,
			Authkey:            c.GRPCAuthkey,
			SecProtocol:        c.SecProtocol,
			VersionInvariant:   c.VersionInvariant,
			ReadUsesLocalTemp:  c.ReadUsesLocalTemp,
			WriteUsesLocalTemp: c.WriteUsesLocalTemp,
			TokenExpiry:        c.TokenExpiry,
			AllowInsecure:      c.AllowInsecure,
		}
		eosHTTPOpts := &eosgrpc.HTTPOptions{
			BaseURL:             c.MasterURL,
			MaxIdleConns:        c.MaxIdleConns,
			MaxConnsPerHost:     c.MaxConnsPerHost,
			MaxIdleConnsPerHost: c.MaxIdleConnsPerHost,
			IdleConnTimeout:     c.IdleConnTimeout,
			ClientCertFile:      c.ClientCertFile,
			ClientKeyFile:       c.ClientKeyFile,
			ClientCADirs:        c.ClientCADirs,
			ClientCAFiles:       c.ClientCAFiles,
			Authkey:             c.HTTPSAuthkey,
		}
		eosClient, err = eosgrpc.New(ctx, eosClientOpts, eosHTTPOpts)
		if err != nil {
			return nil, errors.Wrap(err, "error initializing eosclient")
		}
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
			TokenExpiry:         c.TokenExpiry,
		}
		eosClient, err = eosbinary.New(eosClientOpts)
	}

	if err != nil {
		return nil, errors.Wrap(err, "error initializing eosclient")
	}

	eosfs := &Eosfs{
		c:            eosClient,
		conf:         c,
		chunkHandler: chunking.NewChunkHandler(c.CacheDirectory),
		userIDCache:  ttlcache.NewCache(),
		tokenCache:   gcache.New(c.UserIDCacheSize).LFU().Build(),
	}

	eosfs.userIDCache.SetCacheSizeLimit(c.UserIDCacheSize)
	eosfs.userIDCache.SetExpirationReasonCallback(func(key string, reason ttlcache.EvictionReason, value any) {
		// We only set those keys with TTL which we weren't able to retrieve the last time
		// For those keys, try to contact the userprovider service again when they expire
		if reason == ttlcache.Expired {
			_, _ = eosfs.getUserIDGateway(context.Background(), key)
		}
	})

	go eosfs.userIDcacheWarmup()

	return eosfs, nil
}

func (fs *Eosfs) userIDcacheWarmup() {
	time.Sleep(2 * time.Second)
	ctx := context.Background()
	paths := []string{fs.wrap(ctx, "/")}

	for i := 0; i < fs.conf.UserIDCacheWarmupDepth; i++ {
		var newPaths []string
		for _, fn := range paths {
			if eosFileInfos, err := fs.c.List(ctx, utils.GetEmptyAuth(), fn); err == nil {
				for _, f := range eosFileInfos {
					_, _ = fs.getUserIDGateway(ctx, strconv.FormatUint(f.UID, 10))
					newPaths = append(newPaths, f.File)
				}
			}
		}
		paths = newPaths
	}
}

func (fs *Eosfs) Shutdown(ctx context.Context) error {
	// TODO(labkode): in a grpc implementation we can close connections.
	return nil
}

func (fs *Eosfs) GetHome(ctx context.Context) (string, error) {
	return "", errtypes.NotSupported("eosfs: get home not supported")
}

// CreateStorageSpace creates a storage space.
func (fs *Eosfs) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("eosfs: CreateStorageSpace is not supported")
}

func (fs *Eosfs) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	// ListStorageSpaces is implemented in the spacesregistry
	return nil, errtypes.NotSupported("eosfs: ListStorageSpaces is not supported")
}

// UpdateStorageSpace updates a storage space.
func (fs *Eosfs) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("eosfs: UpdateStorageSpace is not supported")
}

func (fs *Eosfs) wrap(ctx context.Context, fn string) (internal string) {
	internal = path.Join(fs.conf.Namespace, fn)
	log := appctx.GetLogger(ctx)
	log.Debug().Msg("eosfs: wrap external=" + fn + " internal=" + internal)
	return
}

func (fs *Eosfs) unwrap(ctx context.Context, internal string) (string, error) {
	log := appctx.GetLogger(ctx)
	ns, err := fs.getNsMatch(internal, []string{fs.conf.Namespace})
	if err != nil {
		return "", err
	}
	external, err := fs.unwrapInternal(ctx, ns, internal)
	if err != nil {
		return "", err
	}
	log.Debug().Msgf("eosfs: unwrap: internal=%s external=%s", internal, external)
	return external, nil
}

func (fs *Eosfs) getNsMatch(internal string, nss []string) (string, error) {
	var match string

	// Ensure that `internal` ends in a trailing slash
	// Otherwise `/eos` would not be in the `/eos/` namespace
	if !strings.HasSuffix(internal, string(os.PathSeparator)) {
		internal += string(os.PathSeparator)
	}

	for _, ns := range nss {
		if strings.HasPrefix(internal, ns) && len(ns) > len(match) {
			match = ns
		}
	}

	if match == "" {
		return "", errtypes.NotFound(fmt.Sprintf("eosfs: path is outside namespaces: path=%s namespaces=%+v", internal, nss))
	}

	return path.Clean(match), nil
}

func (fs *Eosfs) unwrapInternal(ctx context.Context, ns, np string) (string, error) {

	if !strings.HasPrefix(np, ns) {
		return "", errtypes.NotFound(fmt.Sprintf("eosfs: path is outside the directory of the logged-in user: internal=%s namespace=%+v", np, ns))
	}

	external := strings.TrimPrefix(np, ns)

	if external == "" {
		external = "/"
	}

	return external, nil
}

func (fs *Eosfs) resolveRefAndGetAuth(ctx context.Context, ref *provider.Reference) (string, eosclient.Authorization, error) {
	p, err := fs.resolve(ctx, ref)
	if err != nil {
		return "", eosclient.Authorization{}, errors.Wrap(err, "eosfs: error resolving reference")
	}

	u, err := utils.GetUser(ctx)
	if err != nil {
		return "", eosclient.Authorization{}, errors.Wrap(err, "eosfs: no user in ctx")
	}
	fn := fs.wrap(ctx, p)
	auth, err := fs.getUserAuth(ctx, u, fn)
	if err != nil {
		return "", eosclient.Authorization{}, err
	}

	return fn, auth, nil
}

// resolve takes in a request path or request id and returns the unwrapped path.
func (fs *Eosfs) resolve(ctx context.Context, ref *provider.Reference) (string, error) {
	if ref.ResourceId != nil {
		p, err := fs.getPath(ctx, ref.ResourceId)
		if err != nil {
			return "", err
		}
		p = path.Join(p, ref.Path)
		return p, nil
	}
	if ref.Path != "" {
		return ref.Path, nil
	}

	// reference is invalid
	return "", fmt.Errorf("invalid reference %+v. at least resource_id or path must be set", ref)
}

// permissionSet returns the permission set for the current user.
func (fs *Eosfs) permissionSet(ctx context.Context, eosFileInfo *eosclient.FileInfo, owner *userpb.UserId) *provider.ResourcePermissions {
	u, ok := appctx.ContextGetUser(ctx)
	if !ok || u.Id == nil {
		return &provider.ResourcePermissions{
			// no permissions
		}
	}

	if role, ok := utils.HasPublicShareRole(u); ok {
		switch role {
		case "editor":
			return permissions.NewEditorRole().CS3ResourcePermissions()
		case "uploader":
			return permissions.NewUploaderRole().CS3ResourcePermissions()
		}
		return permissions.NewViewerRole().CS3ResourcePermissions()
	}

	if role, ok := utils.HasOCMShareRole(u); ok {
		if role == "editor" {
			return permissions.NewEditorRole().CS3ResourcePermissions()
		}
		return permissions.NewViewerRole().CS3ResourcePermissions()
	}

	if utils.UserEqual(u.Id, owner) {
		return permissions.NewManagerRole().CS3ResourcePermissions()
	}

	auth, err := fs.getUserAuth(ctx, u, eosFileInfo.File)
	if err != nil {
		return &provider.ResourcePermissions{
			// no permissions
		}
	}

	if eosFileInfo.SysACL == nil {
		return &provider.ResourcePermissions{
			// no permissions
		}
	}
	var perm provider.ResourcePermissions

	// as the lightweight acl are stored as normal attrs,
	// we need to add them in the sysacl entries

	for k, v := range eosFileInfo.Attrs {
		if e, ok := attrForLightweightACL(k, v); ok {
			eosFileInfo.SysACL.Entries = append(eosFileInfo.SysACL.Entries, e)
		}
	}

	userGroupsSet := makeSet(u.Groups)

	for _, e := range eosFileInfo.SysACL.Entries {
		userInGroup := e.Type == acl.TypeGroup && userGroupsSet.in(strings.ToLower(e.Qualifier))

		if (e.Type == acl.TypeUser && e.Qualifier == auth.Role.UID) || (e.Type == acl.TypeLightweight && e.Qualifier == u.Id.OpaqueId) || userInGroup {
			mergePermissions(&perm, grants.GetGrantPermissionSet(e.Permissions))
		}
	}

	// for normal files, we need to inherit also the lw acls
	// from the parent folder, as these, when creating a new
	// file are not inherited

	if utils.IsLightweightUser(u) && !eosFileInfo.IsDir {
		if parentPath, err := fs.unwrap(ctx, filepath.Dir(eosFileInfo.File)); err == nil {
			if parent, err := fs.GetMD(ctx, &provider.Reference{Path: parentPath}, nil); err == nil {
				mergePermissions(&perm, parent.PermissionSet)
			}
		}
	}

	return &perm
}

func attrForLightweightACL(k, v string) (*acl.Entry, bool) {
	ok := strings.HasPrefix(k, "sys."+lwShareAttrKey)
	if !ok {
		return nil, false
	}

	qualifier := strings.TrimPrefix(k, fmt.Sprintf("sys.%s.", lwShareAttrKey))

	attr := &acl.Entry{
		Type:        acl.TypeLightweight,
		Qualifier:   qualifier,
		Permissions: v,
	}
	return attr, true
}

type groupSet map[string]struct{}

func makeSet(lst []string) groupSet {
	s := make(map[string]struct{}, len(lst))
	for _, e := range lst {
		s[e] = struct{}{}
	}
	return s
}

func (s groupSet) in(group string) bool {
	_, ok := s[group]
	return ok
}

func mergePermissions(l *provider.ResourcePermissions, r *provider.ResourcePermissions) {
	l.AddGrant = l.AddGrant || r.AddGrant
	l.CreateContainer = l.CreateContainer || r.CreateContainer
	l.Delete = l.Delete || r.Delete
	l.GetPath = l.GetPath || r.GetPath
	l.GetQuota = l.GetQuota || r.GetQuota
	l.InitiateFileDownload = l.InitiateFileDownload || r.InitiateFileDownload
	l.InitiateFileUpload = l.InitiateFileUpload || r.InitiateFileUpload
	l.ListContainer = l.ListContainer || r.ListContainer
	l.ListFileVersions = l.ListFileVersions || r.ListFileVersions
	l.ListGrants = l.ListGrants || r.ListGrants
	l.ListRecycle = l.ListRecycle || r.ListRecycle
	l.Move = l.Move || r.Move
	l.PurgeRecycle = l.PurgeRecycle || r.PurgeRecycle
	l.RemoveGrant = l.RemoveGrant || r.RemoveGrant
	l.RestoreFileVersion = l.RestoreFileVersion || r.RestoreFileVersion
	l.RestoreRecycleItem = l.RestoreRecycleItem || r.RestoreRecycleItem
	l.Stat = l.Stat || r.Stat
	l.UpdateGrant = l.UpdateGrant || r.UpdateGrant
	l.DenyGrant = l.DenyGrant || r.DenyGrant
}

func (fs *Eosfs) convert(ctx context.Context, eosFileInfo *eosclient.FileInfo) (*provider.ResourceInfo, error) {
	p, err := fs.unwrap(ctx, eosFileInfo.File)
	if err != nil {
		return nil, err
	}

	size := eosFileInfo.Size
	if eosFileInfo.IsDir {
		size = eosFileInfo.TreeSize
	}

	owner, err := fs.getUserIDGateway(ctx, strconv.FormatUint(eosFileInfo.UID, 10))
	if err != nil {
		sublog := appctx.GetLogger(ctx).With().Logger()
		sublog.Warn().Uint64("uid", eosFileInfo.UID).Msg("could not lookup userid, leaving empty")
	}

	var xs provider.ResourceChecksum
	if eosFileInfo.Size != 0 && eosFileInfo.XS != nil {
		xs.Sum = strings.TrimLeft(eosFileInfo.XS.XSSum, "0")
		switch eosFileInfo.XS.XSType {
		case "adler":
			xs.Type = provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32
		default:
			xs.Type = provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID
		}
	}

	// filter 'sys' attrs
	filteredAttrs := make(map[string]string)
	for k, v := range eosFileInfo.Attrs {
		if !strings.HasPrefix(k, "sys") {
			filteredAttrs[k] = v
		}
		// we also want to expose the recycle id, if set
		// see https://eos-docs.web.cern.ch/diopside/manual/interfaces.html#recycle-bin
		if k == recycleIdKey {
			filteredAttrs["recycleid"] = v
		}
	}

	parseAndSetFavoriteAttr(ctx, filteredAttrs)

	info := &provider.ResourceInfo{
		Id: &provider.ResourceId{
			OpaqueId: fmt.Sprintf("%d", eosFileInfo.Inode),
			SpaceId:  spaces.PathToSpaceID(p)},
		Path:          p,
		Name:          path.Base(p),
		Owner:         owner,
		Etag:          fmt.Sprintf("\"%s\"", strings.Trim(eosFileInfo.ETag, "\"")),
		MimeType:      mime.Detect(eosFileInfo.IsDir, p),
		Size:          size,
		ParentId:      &provider.ResourceId{OpaqueId: fmt.Sprintf("%d", eosFileInfo.FID)},
		PermissionSet: fs.permissionSet(ctx, eosFileInfo, owner),
		Checksum:      &xs,
		Type:          getResourceType(eosFileInfo.IsDir),
		Mtime: &types.Timestamp{
			Seconds: eosFileInfo.MTimeSec,
			Nanos:   eosFileInfo.MTimeNanos,
		},
		ArbitraryMetadata: &provider.ArbitraryMetadata{
			Metadata: filteredAttrs,
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
	if eosFileInfo.Attrs[eosLockKey] != "" {
		// populate the lock if decodable, log failure (but move on) if not
		l, err := decodeLock(eosFileInfo.Attrs[lockPayloadKey], eosFileInfo.Attrs[eosLockKey])
		if err != nil {
			sublog := appctx.GetLogger(ctx).With().Logger()
			sublog.Warn().Interface("xattrs", eosFileInfo.Attrs).Msg("could not decode lock, leaving empty")
		} else {
			info.Lock = l
		}
	}
	if eosFileInfo.IsDir {
		info.Opaque.Map["disable_tus"] = &types.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte("true"),
		}
	}

	return info, nil
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

type eosSysMetadata struct {
	TreeSize  uint64 `json:"tree_size"`
	TreeCount uint64 `json:"tree_count"`
	File      string `json:"file"`
	Instance  string `json:"instance"`
}

func (fs *Eosfs) getEosMetadata(finfo *eosclient.FileInfo) []byte {
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

func parseAndSetFavoriteAttr(ctx context.Context, attrs map[string]string) {
	// Read and correctly set the favorite attr
	if user, ok := appctx.ContextGetUser(ctx); ok {
		if favAttrStr, ok := attrs[eosclient.FavoritesKey]; ok {
			favUsers, err := acl.Parse(favAttrStr, acl.ShortTextForm)
			if err != nil {
				return
			}
			for _, u := range favUsers.Entries {
				// Check if the current user has favorited this resource
				if u.Qualifier == user.Id.OpaqueId {
					// Set attr val to 1
					attrs[eosclient.FavoritesKey] = "1"
					return
				}
			}
		}
	}

	// Delete the favorite attr from the response
	delete(attrs, eosclient.FavoritesKey)
}
