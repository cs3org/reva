// Copyright 2018-2024 CERN
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
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/bluele/gcache"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/pkg/appctx"

	"github.com/cs3org/reva/v3/pkg/eosclient"
	"github.com/cs3org/reva/v3/pkg/eosclient/eosbinary"
	"github.com/cs3org/reva/v3/pkg/eosclient/eosgrpc"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/mime"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
	"github.com/cs3org/reva/v3/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/v3/pkg/storage/utils/grants"
	"github.com/cs3org/reva/v3/pkg/storage/utils/templates"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

const (
	refTargetAttrKey = "reva.target"      // used as user attr to store a reference
	lwShareAttrKey   = "reva.lwshare"     // used to store grants to lightweight accounts
	lockPayloadKey   = "reva.lockpayload" // used to store lock payloads
	eosLockKey       = "app.lock"         // this is the key known by EOS to enforce a lock.
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
	c.Namespace = path.Clean(c.Namespace)
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
	eosfs.userIDCache.SetExpirationReasonCallback(func(key string, reason ttlcache.EvictionReason, value interface{}) {
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
	if !fs.conf.EnableHome {
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
}

func (fs *Eosfs) ListWithRegex(ctx context.Context, path, regex string, depth uint, user *userpb.User) ([]*provider.ResourceInfo, error) {
	userAuth, err := fs.getUserAuth(ctx, user, "")
	if err != nil {
		return nil, err
	}

	eosFileInfos, err := fs.c.ListWithRegex(ctx, userAuth, path, depth, regex)
	if err != nil {
		return nil, err
	}
	resourceInfos := []*provider.ResourceInfo{}

	for _, eosFileInfo := range eosFileInfos {

		// Remove the hidden folders in the topmost directory
		finfo, err := fs.convertToResourceInfo(ctx, eosFileInfo)
		if err == nil {
			resourceInfos = append(resourceInfos, finfo)
		}
	}

	return resourceInfos, err
}

func (fs *Eosfs) Shutdown(ctx context.Context) error {
	// TODO(labkode): in a grpc implementation we can close connections.
	return nil
}

func (fs *Eosfs) getLayout(ctx context.Context) (layout string) {
	if fs.conf.EnableHome {
		u := appctx.ContextMustGetUser(ctx)
		layout = templates.WithUser(u, fs.conf.UserLayout)
	}
	return
}

func (fs *Eosfs) getInternalHome(ctx context.Context) string {
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("get internal home: %+v", fs.conf.EnableHome)
	if !fs.conf.EnableHome {
		// TODO(lopresti): this is to be removed as we always want to support home,
		// cf. https://github.com/cs3org/reva/pull/4940
		return "/"
	}

	u := appctx.ContextMustGetUser(ctx)
	relativeHome := templates.WithUser(u, fs.conf.UserLayout)
	log.Info().Msgf("get internal home: %+v", relativeHome)
	return relativeHome
}

func (fs *Eosfs) wrap(ctx context.Context, fn string) (internal string) {
	if fs.conf.EnableHome {
		internal = path.Join(fs.conf.Namespace, fs.getInternalHome(ctx), fn)
	} else {
		internal = path.Join(fs.conf.Namespace, fn)
	}
	log := appctx.GetLogger(ctx)
	log.Debug().Msg("eosfs: wrap external=" + fn + " internal=" + internal)
	return
}

func (fs *Eosfs) unwrap(ctx context.Context, internal string) (string, error) {
	log := appctx.GetLogger(ctx)
	layout := fs.getLayout(ctx)
	ns, err := fs.getNsMatch(internal, []string{fs.conf.Namespace})
	if err != nil {
		return "", err
	}
	external, err := fs.unwrapInternal(ctx, ns, internal, layout)
	if err != nil {
		return "", err
	}
	log.Debug().Msgf("eosfs: unwrap: internal=%s external=%s", internal, external)
	return external, nil
}

func (fs *Eosfs) getNsMatch(internal string, nss []string) (string, error) {
	var match string

	for _, ns := range nss {
		if strings.HasPrefix(internal, ns) && len(ns) > len(match) {
			match = ns
		}
	}

	if match == "" {
		return "", errtypes.NotFound(fmt.Sprintf("eosfs: path is outside namespaces: path=%s namespaces=%+v", internal, nss))
	}

	return match, nil
}

func (fs *Eosfs) unwrapInternal(ctx context.Context, ns, np, layout string) (string, error) {
	trim := path.Join(ns, layout)

	if !strings.HasPrefix(np, trim) {
		return "", errtypes.NotFound(fmt.Sprintf("eosfs: path is outside the directory of the logged-in user: internal=%s trim=%s namespace=%+v", np, trim, ns))
	}

	external := strings.TrimPrefix(np, trim)

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

func (fs *Eosfs) getPath(ctx context.Context, id *provider.ResourceId) (string, error) {
	fid, err := strconv.ParseUint(id.OpaqueId, 10, 64)
	if err != nil {
		return "", fmt.Errorf("error converting string to int for eos fileid: %s", id.OpaqueId)
	}

	auth, err := fs.getDaemonAuth(ctx)
	if err != nil {
		return "", err
	}

	eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, auth, fid)
	if err != nil {
		return "", errors.Wrap(err, "eosfs: error getting file info by inode")
	}

	return fs.unwrap(ctx, eosFileInfo.File)
}

func (fs *Eosfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	fid, err := strconv.ParseUint(id.OpaqueId, 10, 64)
	if err != nil {
		return "", errors.Wrap(err, "eosfs: error parsing fileid string")
	}

	u, err := utils.GetUser(ctx)
	if err != nil {
		return "", errors.Wrap(err, "eosfs: no user in ctx")
	}

	var auth eosclient.Authorization
	if utils.IsLightweightUser(u) {
		auth, err = fs.getDaemonAuth(ctx)
	} else {
		auth, err = fs.getUserAuth(ctx, u, "")
	}
	if err != nil {
		return "", err
	}

	eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, auth, fid)
	if err != nil {
		return "", errors.Wrap(err, "eosfs: error getting file info by inode")
	}

	if perm := fs.permissionSet(ctx, eosFileInfo, nil); !perm.GetPath {
		return "", errtypes.PermissionDenied("eosfs: getting path for id not allowed")
	}

	return fs.unwrap(ctx, eosFileInfo.File)
}

func (fs *Eosfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	if len(md.Metadata) == 0 {
		return errtypes.BadRequest("eosfs: no metadata set")
	}

	fn, _, err := fs.resolveRefAndGetAuth(ctx, ref)
	if err != nil {
		return err
	}

	cboxAuth := utils.GetEmptyAuth()

	for k, v := range md.Metadata {
		if k == "" || v == "" {
			return errtypes.BadRequest(fmt.Sprintf("eosfs: key or value is empty: key:%s, value:%s", k, v))
		}

		// do not allow to override system-reserved keys
		if k == lockPayloadKey || k == eosLockKey || k == lwShareAttrKey || k == refTargetAttrKey {
			return errtypes.BadRequest(fmt.Sprintf("eosfs: key %s is reserved", k))
		}

		attr := &eosclient.Attribute{
			Type: UserAttr,
			Key:  k,
			Val:  v,
		}

		// TODO(labkode): SetArbitraryMetadata does not have semantics for recursivity.
		// We set it to false
		err := fs.c.SetAttr(ctx, cboxAuth, attr, false, false, fn, "")
		if err != nil {
			return errors.Wrap(err, "eosfs: error setting xattr in eos driver")
		}
	}
	return nil
}

func (fs *Eosfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	if len(keys) == 0 {
		return errtypes.BadRequest("eosfs: no keys set")
	}

	fn, _, err := fs.resolveRefAndGetAuth(ctx, ref)
	if err != nil {
		return err
	}

	cboxAuth := utils.GetEmptyAuth()

	for _, k := range keys {
		if k == "" {
			return errtypes.BadRequest("eosfs: key is empty")
		}

		attr := &eosclient.Attribute{
			Type: UserAttr,
			Key:  k,
		}

		err := fs.c.UnsetAttr(ctx, cboxAuth, attr, false, fn, "")

		if err != nil {
			if errors.Is(err, eosclient.AttrNotExistsError) {
				continue
			}
			return errors.Wrap(err, "eosfs: error unsetting xattr in eos driver")
		}
	}
	return nil
}

func (fs *Eosfs) EncodeAppName(a string) string {
	// this function returns the string to be used as EOS "app" tag, both in uploads and when handling locks;
	// note that the GET (and PUT) operations in eosbinary.go and eoshttp.go use a `reva_eosclient::read`
	// (resp. `write`) tag when no locks are involved.
	r := strings.NewReplacer(" ", "_")
	return "reva_eosclient::app_" + strings.ToLower(r.Replace(a))
}

func (fs *Eosfs) getLockPayloads(ctx context.Context, path string) (string, string, error) {
	// sys attributes want root auth, buddy
	cboxAuth := utils.GetEmptyAuth()

	data, err := fs.c.GetAttr(ctx, cboxAuth, "sys."+lockPayloadKey, path)
	if err != nil {
		return "", "", err
	}

	eoslock, err := fs.c.GetAttr(ctx, cboxAuth, "sys."+eosLockKey, path)
	if err != nil {
		return "", "", err
	}

	return data.Val, eoslock.Val, nil
}

func (fs *Eosfs) removeLockAttrs(ctx context.Context, path, app string) error {
	cboxAuth := utils.GetEmptyAuth()

	err := fs.c.UnsetAttr(ctx, cboxAuth, &eosclient.Attribute{
		Type: SystemAttr,
		Key:  eosLockKey,
	}, false, path, app)
	if err != nil {
		return errors.Wrap(err, "eosfs: error unsetting the eos lock")
	}

	err = fs.c.UnsetAttr(ctx, cboxAuth, &eosclient.Attribute{
		Type: SystemAttr,
		Key:  lockPayloadKey,
	}, false, path, app)
	if err != nil {
		return errors.Wrap(err, "eosfs: error unsetting the lock payload")
	}

	return nil
}

func (fs *Eosfs) getLock(ctx context.Context, user *userpb.User, path string, ref *provider.Reference) (*provider.Lock, error) {
	// the cs3apis require to have the read permission on the resource
	// to get the eventual lock.
	has, err := fs.userHasReadAccess(ctx, user, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error checking read access to resource")
	}
	if !has {
		return nil, errtypes.BadRequest("user has not read access on resource")
	}

	d, eosl, err := fs.getLockPayloads(ctx, path)
	if err != nil {
		if !errors.Is(err, eosclient.AttrNotExistsError) {
			return nil, errtypes.NotFound("lock not found for ref")
		}
	}

	l, err := decodeLock(d, eosl)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: malformed lock payload")
	}

	if time.Unix(int64(l.Expiration.Seconds), 0).Before(time.Now()) {
		// the lock expired
		if err := fs.removeLockAttrs(ctx, path, fs.EncodeAppName(l.AppName)); err != nil {
			return nil, err
		}
		return nil, errtypes.NotFound("lock not found for ref")
	}

	return l, nil
}

// GetLock returns an existing lock on the given reference.
func (fs *Eosfs) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	path, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error resolving reference")
	}
	user, err := utils.GetUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: no user in ctx")
	}

	// the cs3apis require to have the read permission on the resource
	// to get the eventual lock.
	has, err := fs.userHasReadAccess(ctx, user, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error checking read access to resource")
	}
	if !has {
		return nil, errtypes.BadRequest("user has no read access on resource")
	}

	path = fs.wrap(ctx, path)
	return fs.getLock(ctx, user, path, ref)
}

func (fs *Eosfs) setLock(ctx context.Context, lock *provider.Lock, path string) error {
	cboxAuth := utils.GetEmptyAuth()

	encodedLock, eosLock, err := fs.encodeLock(lock)
	if err != nil {
		return errors.Wrap(err, "eosfs: error encoding lock")
	}

	// set eos lock
	err = fs.c.SetAttr(ctx, cboxAuth, &eosclient.Attribute{
		Type: SystemAttr,
		Key:  eosLockKey,
		Val:  eosLock,
	}, false, false, path, fs.EncodeAppName(lock.AppName))
	switch {
	case errors.Is(err, eosclient.FileIsLockedError):
		return errtypes.Conflict("resource already locked")
	case err != nil:
		return errors.Wrap(err, "eosfs: error setting eos lock")
	}

	// set payload
	err = fs.c.SetAttr(ctx, cboxAuth, &eosclient.Attribute{
		Type: SystemAttr,
		Key:  lockPayloadKey,
		Val:  encodedLock,
	}, false, false, path, fs.EncodeAppName(lock.AppName))
	if err != nil {
		return errors.Wrap(err, "eosfs: error setting lock payload")
	}
	return nil
}

// SetLock puts a lock on the given reference.
func (fs *Eosfs) SetLock(ctx context.Context, ref *provider.Reference, l *provider.Lock) error {
	if l.Type == provider.LockType_LOCK_TYPE_SHARED {
		return errtypes.NotSupported("shared lock not yet implemented")
	}

	path, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}

	user, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}

	// the cs3apis require to have the write permission on the resource
	// to set a lock. because in eos we can set attrs even if the user does
	// not have the write permission, we need to check if the user that made
	// the request has it
	has, err := fs.userHasWriteAccess(ctx, user, ref)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("eosfs: cannot check if user %s has write access on resource", user.Username))
	}
	if !has {
		return errtypes.PermissionDenied(fmt.Sprintf("user %s has no write access on resource", user.Username))
	}

	// the user in the lock could differ from the user in the context
	// in that case, also the user in the lock MUST have the write permission
	if l.User != nil && !utils.UserEqual(user.Id, l.User) {
		has, err := fs.userIDHasWriteAccess(ctx, l.User, ref)
		if err != nil {
			return errors.Wrap(err, "eosfs: cannot check if user has write access on resource")
		}
		if !has {
			return errtypes.PermissionDenied(fmt.Sprintf("user %s has no write access on resource", user.Username))
		}
	}

	path = fs.wrap(ctx, path)
	return fs.setLock(ctx, l, path)
}

func (fs *Eosfs) getUserFromID(ctx context.Context, userID *userpb.UserId) (*userpb.User, error) {
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.conf.GatewaySvc))
	if err != nil {
		return nil, err
	}
	res, err := client.GetUser(ctx, &userpb.GetUserRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, errtypes.InternalError(res.Status.Message)
	}
	return res.User, nil
}

func (fs *Eosfs) userHasWriteAccess(ctx context.Context, user *userpb.User, ref *provider.Reference) (bool, error) {
	ctx = appctx.ContextSetUser(ctx, user)
	resInfo, err := fs.GetMD(ctx, ref, nil)
	if err != nil {
		return false, err
	}
	return resInfo.PermissionSet.InitiateFileUpload, nil
}

func (fs *Eosfs) userIDHasWriteAccess(ctx context.Context, userID *userpb.UserId, ref *provider.Reference) (bool, error) {
	user, err := fs.getUserFromID(ctx, userID)
	if err != nil {
		return false, nil
	}
	return fs.userHasWriteAccess(ctx, user, ref)
}

func (fs *Eosfs) userHasReadAccess(ctx context.Context, user *userpb.User, ref *provider.Reference) (bool, error) {
	ctx = appctx.ContextSetUser(ctx, user)
	resInfo, err := fs.GetMD(ctx, ref, nil)
	if err != nil {
		return false, err
	}
	return resInfo.PermissionSet.InitiateFileDownload, nil
}

func (fs *Eosfs) encodeLock(l *provider.Lock) (string, string, error) {
	data, err := json.Marshal(l)
	if err != nil {
		return "", "", err
	}
	var a string
	if l.AppName != "" {
		// cf. upload implementation
		a = fs.EncodeAppName(l.AppName)
	} else {
		a = "*"
	}
	var u string
	if l.User != nil {
		u = l.User.OpaqueId
	} else {
		u = "*"
	}
	// the eos lock has hardcoded type "shared" because that's what eos supports. This is good enough
	// for apps via WOPI and for checkout/checkin behavior, not for "exclusive" (no read access unless holding the lock).
	return b64.StdEncoding.EncodeToString(data),
		fmt.Sprintf("expires:%d,type:shared,owner:%s:%s", l.Expiration.Seconds, u, a),
		nil
}

func decodeLock(content string, eosLock string) (*provider.Lock, error) {
	d, err := b64.StdEncoding.DecodeString(content)
	if err != nil {
		return nil, err
	}

	l := new(provider.Lock)
	err = json.Unmarshal(d, l)
	if err != nil {
		return nil, err
	}

	// validate that the eosLock respect the format, otherwise raise error
	if !eosLockReg.MatchString(eosLock) {
		return nil, errtypes.BadRequest("eos lock payload does not match expected format: " + eosLock)
	}

	return l, nil
}

// RefreshLock refreshes an existing lock on the given reference.
func (fs *Eosfs) RefreshLock(ctx context.Context, ref *provider.Reference, newLock *provider.Lock, existingLockID string) error {
	if newLock.Type == provider.LockType_LOCK_TYPE_SHARED {
		return errtypes.NotSupported("shared lock not yet implemented")
	}

	oldLock, err := fs.GetLock(ctx, ref)
	if err != nil {
		switch err.(type) {
		case errtypes.NotFound:
			// the lock does not exist
			return errtypes.BadRequest("file was not locked")
		default:
			return err
		}
	}

	user, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: error getting user")
	}

	// check if the holder is the same of the new lock
	if !sameHolder(oldLock, newLock) {
		return errtypes.BadRequest("caller does not hold the lock")
	}

	if existingLockID != "" && oldLock.LockId != existingLockID {
		return errtypes.BadRequest("lock id does not match")
	}

	path, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}
	path = fs.wrap(ctx, path)

	// the cs3apis require to have the write permission on the resource
	// to set a lock
	has, err := fs.userHasWriteAccess(ctx, user, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: cannot check if user has write access on resource")
	}
	if !has {
		return errtypes.PermissionDenied(fmt.Sprintf("user %s has no write access on resource", user.Username))
	}

	return fs.setLock(ctx, newLock, path)
}

func sameHolder(l1, l2 *provider.Lock) bool {
	same := true
	if l1.User != nil || l2.User != nil {
		same = utils.UserEqual(l1.User, l2.User)
	}
	if l1.AppName != "" || l2.AppName != "" {
		same = l1.AppName == l2.AppName
	}
	return same
}

// Unlock removes an existing lock from the given reference.
func (fs *Eosfs) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	oldLock, err := fs.GetLock(ctx, ref)
	if err != nil {
		switch err.(type) {
		case errtypes.NotFound:
			// the lock does not exist
			return errtypes.BadRequest("file was not locked")
		default:
			return err
		}
	}

	// check if the lock id of the lock corresponds to the stored lock
	if oldLock.LockId != lock.LockId {
		return errtypes.BadRequest("lock id does not match")
	}

	if !sameHolder(oldLock, lock) {
		return errtypes.BadRequest("caller does not hold the lock")
	}

	user, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: error getting user")
	}

	// the cs3apis require to have the write permission on the resource
	// to remove the lock
	has, err := fs.userHasWriteAccess(ctx, user, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: cannot check if user has write access on resource")
	}
	if !has {
		return errtypes.PermissionDenied(fmt.Sprintf("user %s has no write access on resource", user.Username))
	}

	path, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}
	path = fs.wrap(ctx, path)

	return fs.removeLockAttrs(ctx, path, fs.EncodeAppName(lock.AppName))
}

func (fs *Eosfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	fn, auth, err := fs.resolveRefAndGetAuth(ctx, ref)
	if err != nil {
		return err
	}

	cboxAuth := utils.GetEmptyAuth()

	eosACL, err := fs.getEosACL(ctx, g)
	if err != nil {
		return err
	}

	if eosACL.Type == acl.TypeLightweight {
		// The ACLs for a lightweight are not understandable by EOS
		// directly, but only from reva. So we have to store them
		// in an xattr named sys.reva.lwshare.<lw_account>, with value
		// the permissions.
		attr := &eosclient.Attribute{
			Type: SystemAttr,
			Key:  fmt.Sprintf("%s.%s", lwShareAttrKey, eosACL.Qualifier),
			Val:  eosACL.Permissions,
		}

		if err := fs.c.SetAttr(ctx, cboxAuth, attr, false, true, fn, ""); err != nil {
			return errors.Wrap(err, "eosfs: error adding acl for lightweight account")
		}
		return nil
	}

	err = fs.c.AddACL(ctx, auth, cboxAuth, fn, eosclient.StartPosition, eosACL)
	if err != nil {
		return errors.Wrap(err, "eosfs: error adding acl")
	}
	return nil
}

func (fs *Eosfs) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error {
	fn, auth, err := fs.resolveRefAndGetAuth(ctx, ref)
	if err != nil {
		return err
	}

	position := eosclient.EndPosition

	cboxAuth := utils.GetEmptyAuth()

	// empty permissions => deny
	grant := &provider.Grant{
		Grantee:     g,
		Permissions: &provider.ResourcePermissions{},
	}

	eosACL, err := fs.getEosACL(ctx, grant)
	if err != nil {
		return err
	}

	err = fs.c.AddACL(ctx, auth, cboxAuth, fn, position, eosACL)
	if err != nil {
		return errors.Wrap(err, "eosfs: error adding acl")
	}
	return nil
}

func (fs *Eosfs) getEosACL(ctx context.Context, g *provider.Grant) (*acl.Entry, error) {
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
		// if the grantee is a lightweight account, we need to set it accordingly
		if g.Grantee.GetUserId().Type == userpb.UserType_USER_TYPE_LIGHTWEIGHT ||
			g.Grantee.GetUserId().Type == userpb.UserType_USER_TYPE_FEDERATED {
			t = acl.TypeLightweight
			qualifier = g.Grantee.GetUserId().OpaqueId
		} else {
			// since EOS Citrine ACLs are stored with uid, we need to convert username to
			// uid only for users.
			auth, err := fs.getUIDGateway(ctx, g.Grantee.GetUserId())
			if err != nil {
				return nil, err
			}
			qualifier = auth.Role.UID
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

func (fs *Eosfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	fn, auth, err := fs.resolveRefAndGetAuth(ctx, ref)
	if err != nil {
		return err
	}

	cboxAuth := utils.GetEmptyAuth()

	eosACL, err := fs.getEosACL(ctx, g)
	if err != nil {
		return err
	}

	if eosACL.Type == acl.TypeLightweight {
		attr := &eosclient.Attribute{
			Type: SystemAttr,
			Key:  fmt.Sprintf("%s.%s", lwShareAttrKey, eosACL.Qualifier),
		}

		if err := fs.c.UnsetAttr(ctx, cboxAuth, attr, true, fn, ""); err != nil {
			return errors.Wrap(err, "eosfs: error removing acl for lightweight account")
		}
		return nil
	}

	err = fs.c.RemoveACL(ctx, auth, cboxAuth, fn, eosACL)
	if err != nil {
		return errors.Wrap(err, "eosfs: error removing acl")
	}
	return nil
}

func (fs *Eosfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return fs.AddGrant(ctx, ref, g)
}

func (fs *Eosfs) convertACLsToGrants(ctx context.Context, acls *acl.ACLs) ([]*provider.Grant, error) {
	res := make([]*provider.Grant, 0, len(acls.Entries))
	for _, a := range acls.Entries {
		var grantee *provider.Grantee
		switch {
		case a.Type == acl.TypeUser:
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
		case a.Type == acl.TypeGroup:
			grantee = &provider.Grantee{
				Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: a.Qualifier}},
				Type: grants.GetGranteeType(a.Type),
			}
		default:
			return nil, errtypes.InternalError(fmt.Sprintf("eosfs: acl type %s not recognised", a.Type))
		}
		res = append(res, &provider.Grant{
			Grantee:     grantee,
			Permissions: grants.GetGrantPermissionSet(a.Permissions),
		})
	}
	return res, nil
}

func isSysACLs(a *eosclient.Attribute) bool {
	return a.Type == SystemAttr && a.Key == "sys"
}

func isLightweightACL(a *eosclient.Attribute) bool {
	return a.Type == SystemAttr && strings.HasPrefix(a.Key, lwShareAttrKey)
}

func parseLightweightACL(a *eosclient.Attribute) *provider.Grant {
	qualifier := strings.TrimPrefix(a.Key, lwShareAttrKey+".")
	return &provider.Grant{
		Grantee: &provider.Grantee{
			Id: &provider.Grantee_UserId{UserId: &userpb.UserId{
				// FIXME: idp missing, maybe get the user_id from the user provider?
				Type:     userpb.UserType_USER_TYPE_LIGHTWEIGHT,
				OpaqueId: qualifier,
			}},
			Type: grants.GetGranteeType(acl.TypeLightweight),
		},
		Permissions: grants.GetGrantPermissionSet(a.Val),
	}
}

func (fs *Eosfs) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	fn, auth, err := fs.resolveRefAndGetAuth(ctx, ref)
	if err != nil {
		return nil, err
	}

	// This is invoked just to see if it fails, I know, it's ugly
	_, err = fs.c.GetAttrs(ctx, auth, fn)
	if err != nil {
		return nil, err
	}

	// Now we get the real info, I know, it's ugly
	cboxAuth := utils.GetEmptyAuth()

	attrs, err := fs.c.GetAttrs(ctx, cboxAuth, fn)
	if err != nil {
		return nil, err
	}

	grantList := []*provider.Grant{}
	for _, a := range attrs {
		switch {
		case isSysACLs(a):
			// EOS ACLs
			acls, err := acl.Parse(a.Val, acl.ShortTextForm)
			if err != nil {
				return nil, err
			}
			grants, err := fs.convertACLsToGrants(ctx, acls)
			if err != nil {
				return nil, err
			}
			grantList = append(grantList, grants...)
		case isLightweightACL(a):
			grantList = append(grantList, parseLightweightACL(a))
		}
	}

	return grantList, nil
}

func (fs *Eosfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("eosfs: get md for ref:" + ref.String())

	if ref == nil {
		return nil, errtypes.BadRequest("No ref was given to GetMD")
	}

	p := ref.Path
	fn := fs.wrap(ctx, p)

	// We use daemon for auth because we need access to the file in order to stat it
	// We cannot use the current user, because the file may be a shared file
	// and lightweight accounts don't have a uid
	auth, err := fs.getDaemonAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting daemon auth")
	}

	if ref.ResourceId != nil {
		// Check if it's a version
		// Cannot check with (ResourceId.StorageId == "versions") because of the storage provider
		if strings.Contains(ref.ResourceId.OpaqueId, "@") {
			parts := strings.Split(ref.ResourceId.OpaqueId, "@")
			version := ""
			ref.ResourceId.OpaqueId, version = parts[0], parts[1]

			path, err := fs.getPath(ctx, ref.ResourceId)
			if err != nil {
				return nil, fmt.Errorf("error getting path for resource id: %s", ref.ResourceId.OpaqueId)
			}
			path = filepath.Join(fn, path)

			versionFolder := eosclient.GetVersionFolder(path)
			versionPath := filepath.Join(versionFolder, version)
			eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, auth, versionPath)
			if err != nil {
				return nil, fmt.Errorf("error getting file info by path: %s", versionPath)
			}

			return fs.convertToResourceInfo(ctx, eosFileInfo)
		}
		fid, err := strconv.ParseUint(ref.ResourceId.OpaqueId, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error converting string to int for eos fileid: %s", ref.ResourceId.OpaqueId)
		}

		eosFileInfo, err := fs.c.GetFileInfoByInode(ctx, auth, fid)
		if err != nil {
			log.Error().Err(err).Str("fid", strconv.Itoa(int(fid))).Msg("Failed to get file info by inode")
			return nil, err
		}

		if ref.Path != "" {
			fn := filepath.Join(eosFileInfo.File, ref.Path)
			eosFileInfo, err = fs.c.GetFileInfoByPath(ctx, auth, fn)
			if err != nil {
				log.Error().Err(err).Str("path", fn).Msg("Failed to get file info by path")
				return nil, err
			}
		}
		return fs.convertToResourceInfo(ctx, eosFileInfo)
	}

	eosFileInfo, err := fs.c.GetFileInfoByPath(ctx, auth, fn)
	if err != nil {
		log.Error().Err(err).Str("path", fn).Msg("Failed to get file info by path")
		return nil, err
	}

	return fs.convertToResourceInfo(ctx, eosFileInfo)
}

func (fs *Eosfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	p, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error resolving reference")
	}

	return fs.listWithNominalHome(ctx, p)
}

func (fs *Eosfs) listWithNominalHome(ctx context.Context, p string) (finfos []*provider.ResourceInfo, err error) {
	log := appctx.GetLogger(ctx)
	fn := fs.wrap(ctx, p)

	u, err := utils.GetUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: no user in ctx")
	}
	userAuth, err := fs.getUserAuth(ctx, u, fn)
	if err != nil {
		return nil, err
	}
	auth := utils.GetUserOrDaemonAuth(userAuth)

	eosFileInfos, err := fs.c.List(ctx, auth, fn)
	if err != nil {
		log.Error().Str("filename", fn).Err(err).Msg("eosfs: error listing")
		return nil, errors.Wrap(err, "eosfs: error listing")
	}

	for _, eosFileInfo := range eosFileInfos {
		// filter out sys files
		if !fs.conf.ShowHiddenSysFiles {
			base := path.Base(eosFileInfo.File)
			if hiddenReg.MatchString(base) {
				log.Debug().Msgf("eosfs: path is filtered because is considered hidden: path=%s hiddenReg=%s", base, hiddenReg)
				continue
			}
		}

		// Remove the hidden folders in the topmost directory
		if finfo, err := fs.convertToResourceInfo(ctx, eosFileInfo); err == nil &&
			finfo.Path != "/" && !strings.HasPrefix(finfo.Path, "/.") {
			finfos = append(finfos, finfo)
		}
	}

	return finfos, nil
}

// CreateStorageSpace creates a storage space.
func (fs *Eosfs) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, fmt.Errorf("unimplemented: CreateStorageSpace")
}

func (fs *Eosfs) GetQuota(ctx context.Context, ref *provider.Reference) (totalbytes, usedbytes uint64, err error) {
	u, err := utils.GetUser(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "eosfs: no user in ctx")
	}
	// lightweight accounts don't have quota nodes, so we're passing an empty string as path
	auth, err := fs.getUserAuth(ctx, u, "")
	if err != nil {
		return 0, 0, err
	}

	cboxAuth := utils.GetEmptyAuth()

	qi, err := fs.c.GetQuota(ctx, auth.Role.UID, cboxAuth, fs.conf.QuotaNode)
	if err != nil {
		err := errors.Wrap(err, "eosfs: error getting quota")
		return 0, 0, err
	}

	return qi.TotalBytes, qi.UsedBytes, nil
}

func (fs *Eosfs) GetHome(ctx context.Context) (string, error) {
	if !fs.conf.EnableHome {
		return "", errtypes.NotSupported("eosfs: get home not supported")
	}

	// eos drive for homes assumes root(/) points to the user home.
	return "/", nil
}

func (fs *Eosfs) createNominalHome(ctx context.Context) error {
	log := appctx.GetLogger(ctx)

	u, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}

	home := templates.WithUser(u, fs.conf.UserLayout)
	home = path.Join(fs.conf.Namespace, home)

	auth, err := fs.getUserAuth(ctx, u, "")
	if err != nil {
		return err
	}

	_, err = fs.c.GetFileInfoByPath(ctx, auth, home)
	if err == nil { // home already exists
		log.Error().Str("home", home).Msg("Home already exists")
		return nil
	}

	if _, ok := err.(errtypes.IsNotFound); !ok {
		return errors.Wrap(err, "eosfs: error verifying if user home directory exists")
	}

	log.Info().Interface("user", u.Id).Interface("home", home).Msg("creating user home")

	if fs.conf.CreateHomeHook != "" {
		hook := exec.Command(fs.conf.CreateHomeHook, u.Username, utils.UserTypeToString(u.Id.Type))
		err = hook.Run()
		log.Info().Interface("output", hook.Stdout).Err(err).Msg("create_home_hook output")
		if err != nil {
			return errors.Wrap(err, "eosfs: error running create home hook")
		}
	} else {
		log.Fatal().Msg("create_home_hook not configured")
		return errtypes.NotFound("eosfs: create home hook not configured")
	}

	return nil
}

func (fs *Eosfs) CreateHome(ctx context.Context) error {
	if !fs.conf.EnableHomeCreation {
		return errtypes.NotSupported("eosfs: create home not supported")
	}

	if err := fs.createNominalHome(ctx); err != nil {
		return errors.Wrap(err, "eosfs: error creating nominal home")
	}

	return nil
}

func (fs *Eosfs) CreateDir(ctx context.Context, ref *provider.Reference) error {
	log := appctx.GetLogger(ctx)

	p, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}
	u, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}

	// We need the auth corresponding to the parent directory
	// as the file might not exist at the moment
	fn := fs.wrap(ctx, p)
	auth, err := fs.getUserAuth(ctx, u, path.Dir(fn))
	if err != nil {
		return err
	}

	log.Info().Msgf("eosfs: createdir: path=%s", fn)
	return fs.c.CreateDir(ctx, auth, fn)
}

// TouchFile as defined in the storage.FS interface.
func (fs *Eosfs) TouchFile(ctx context.Context, ref *provider.Reference) error {
	log := appctx.GetLogger(ctx)

	fn, auth, err := fs.resolveRefAndGetAuth(ctx, ref)
	if err != nil {
		return err
	}
	log.Info().Msgf("eosfs: touch file: path=%s", fn)

	return fs.c.Touch(ctx, auth, fn)
}

func (fs *Eosfs) CreateReference(ctx context.Context, p string, targetURI *url.URL) error {
	_, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}

	// TODO(labkode): with the grpc plugin we can create a file touching with xattrs.
	// Current mechanism is: touch to hidden location, set xattr, rename.
	fn := fs.wrap(ctx, p)
	dir, base := path.Split(fn)
	tmp := path.Join(dir, fmt.Sprintf(".sys.reva#.%s", base))
	cboxAuth := utils.GetEmptyAuth()

	if err := fs.c.CreateDir(ctx, cboxAuth, tmp); err != nil {
		err = errors.Wrapf(err, "eosfs: error creating temporary ref file")
		return err
	}

	// set xattr on ref
	attr := &eosclient.Attribute{
		Type: UserAttr,
		Key:  refTargetAttrKey,
		Val:  targetURI.String(),
	}

	if err := fs.c.SetAttr(ctx, cboxAuth, attr, false, false, tmp, ""); err != nil {
		err = errors.Wrapf(err, "eosfs: error setting reva.ref attr on file: %q", tmp)
		return err
	}

	// rename to have the file visible in user space.
	if err := fs.c.Rename(ctx, cboxAuth, tmp, fn); err != nil {
		err = errors.Wrapf(err, "eosfs: error renaming from: %q to %q", tmp, fn)
		return err
	}

	return nil
}

func (fs *Eosfs) Delete(ctx context.Context, ref *provider.Reference) error {
	p, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}
	u, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}

	fn := fs.wrap(ctx, p)
	auth, err := fs.getUserAuth(ctx, u, fn)
	if err != nil {
		return err
	}

	return fs.c.Remove(ctx, auth, fn, false)
}

func (fs *Eosfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	u, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}

	oldPath, err := fs.resolve(ctx, oldRef)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}
	newPath, err := fs.resolve(ctx, newRef)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}

	oldFn := fs.wrap(ctx, oldPath)
	newFn := fs.wrap(ctx, newPath)
	auth, err := fs.getUserAuth(ctx, u, oldFn)
	if err != nil {
		return err
	}

	return fs.c.Rename(ctx, auth, oldFn, newFn)
}

func (fs *Eosfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	fn, auth, err := fs.resolveRefAndGetAuth(ctx, ref)
	if err != nil {
		return nil, err
	}

	return fs.c.Read(ctx, auth, fn)
}

func (fs *Eosfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	var auth eosclient.Authorization
	var fn string
	var err error

	if !fs.conf.EnableHome {
		// We need to access the revisions for a non-home reference.
		// We'll get the owner of the particular resource and impersonate them
		// if we have access to it.
		md, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			return nil, err
		}
		fn = fs.wrap(ctx, md.Path)

		if md.PermissionSet.ListFileVersions {
			user := appctx.ContextMustGetUser(ctx)
			auth, err = fs.getEOSToken(ctx, user, fn)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, errtypes.PermissionDenied("eosfs: user doesn't have permissions to list revisions")
		}
	} else {
		var userAuth eosclient.Authorization
		fn, userAuth, err = fs.resolveRefAndGetAuth(ctx, ref)
		if err != nil {
			return nil, err
		}
		auth = utils.GetUserOrDaemonAuth(userAuth)
	}

	eosRevisions, err := fs.c.ListVersions(ctx, auth, fn)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error listing versions")
	}
	revisions := []*provider.FileVersion{}
	for _, eosRev := range eosRevisions {
		if rev, err := fs.convertToRevision(ctx, eosRev); err == nil {
			revisions = append(revisions, rev)
		}
	}
	return revisions, nil
}

func (fs *Eosfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	var auth eosclient.Authorization
	var fn string
	var err error

	if !fs.conf.EnableHome {
		// We need to access the revisions for a non-home reference.
		// We'll get the owner of the particular resource and impersonate them
		// if we have access to it.
		md, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			return nil, err
		}
		fn = fs.wrap(ctx, md.Path)

		if md.PermissionSet.InitiateFileDownload {
			user := appctx.ContextMustGetUser(ctx)
			auth, err = fs.getEOSToken(ctx, user, fn)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, errtypes.PermissionDenied("eosfs: user doesn't have permissions to download revisions")
		}
	} else {
		fn, auth, err = fs.resolveRefAndGetAuth(ctx, ref)
		if err != nil {
			return nil, err
		}
	}
	return fs.c.ReadVersion(ctx, auth, fn, revisionKey)
}

func (fs *Eosfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	log := appctx.GetLogger(ctx)
	var auth eosclient.Authorization
	var fn string
	var err error

	if !fs.conf.EnableHome {
		// We need to access the revisions for a non-home reference.
		// We'll get the owner of the particular resource and impersonate them
		// if we have access to it.
		md, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			return err
		}
		fn = fs.wrap(ctx, md.Path)

		if md.PermissionSet.RestoreFileVersion {
			user := appctx.ContextMustGetUser(ctx)
			auth, err = fs.getEOSToken(ctx, user, fn)
			if err != nil {
				return err
			}
		} else {
			return errtypes.PermissionDenied("eosfs: user doesn't have permissions to restore revisions")
		}
	} else {
		fn, auth, err = fs.resolveRefAndGetAuth(ctx, ref)
		if err != nil {
			return err
		}
	}

	log.Debug().Any("auth", auth).Any("file", fn).Any("revision", revisionKey).Msg("eosfs RestoreRevision")
	return fs.c.RollbackToVersion(ctx, auth, fn, revisionKey)
}

func (fs *Eosfs) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return errtypes.NotSupported("eosfs: operation not supported")
}

func (fs *Eosfs) EmptyRecycle(ctx context.Context) error {
	u, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}
	auth, err := fs.getUserAuth(ctx, u, "")
	if err != nil {
		return err
	}

	return fs.c.PurgeDeletedEntries(ctx, auth)
}

func (fs *Eosfs) ListRecycle(ctx context.Context, basePath, key, relativePath string, from, to *types.Timestamp) ([]*provider.RecycleItem, error) {
	var auth eosclient.Authorization

	if !fs.conf.EnableHome && fs.conf.AllowPathRecycleOperations && basePath != "/" {
		// We need to access the recycle bin for a non-home reference.
		// We'll get the owner of the particular resource and impersonate them
		// if we have access to it.
		md, err := fs.GetMD(ctx, &provider.Reference{Path: basePath}, nil)
		if err != nil {
			return nil, err
		}
		if md.PermissionSet.ListRecycle {
			auth, err = fs.getUIDGateway(ctx, md.Owner)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, errtypes.PermissionDenied("eosfs: user doesn't have permissions to restore recycled items")
		}
	} else {
		// We just act on the logged-in user's recycle bin
		u, err := utils.GetUser(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "eosfs: no user in ctx")
		}
		auth, err = fs.getUserAuth(ctx, u, "")
		if err != nil {
			return nil, err
		}
	}

	var dateFrom, dateTo time.Time
	if from != nil && to != nil {
		dateFrom = time.Unix(int64(from.Seconds), 0)
		dateTo = time.Unix(int64(to.Seconds), 0)
		if dateFrom.AddDate(0, 0, fs.conf.MaxDaysInRecycleList).Before(dateTo) {
			return nil, errtypes.BadRequest("eosfs: too many days requested in listing the recycle bin")
		}
	} else {
		// if no date range was given, list up to two days ago
		dateTo = time.Now()
		dateFrom = dateTo.AddDate(0, 0, -2)
	}

	sublog := appctx.GetLogger(ctx).With().Logger()
	sublog.Debug().Time("from", dateFrom).Time("to", dateTo).Msg("executing ListDeletedEntries")
	eosDeletedEntries, err := fs.c.ListDeletedEntries(ctx, auth, fs.conf.MaxRecycleEntries, dateFrom, dateTo)
	if err != nil {
		switch err.(type) {
		case errtypes.IsBadRequest:
			return nil, errtypes.BadRequest("eosfs: too many entries found in listing the recycle bin")
		default:
			return nil, errors.Wrap(err, "eosfs: error listing deleted entries")
		}
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

func (fs *Eosfs) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	var auth eosclient.Authorization

	if !fs.conf.EnableHome && fs.conf.AllowPathRecycleOperations && basePath != "/" {
		// We need to access the recycle bin for a non-home reference.
		// We'll get the owner of the particular resource and impersonate them
		// if we have access to it.
		md, err := fs.GetMD(ctx, &provider.Reference{Path: basePath}, nil)
		if err != nil {
			return err
		}
		if md.PermissionSet.RestoreRecycleItem {
			auth, err = fs.getUIDGateway(ctx, md.Owner)
			if err != nil {
				return err
			}
		} else {
			return errtypes.PermissionDenied("eosfs: user doesn't have permissions to restore recycled items")
		}
	} else {
		// We just act on the logged-in user's recycle bin
		u, err := utils.GetUser(ctx)
		if err != nil {
			return errors.Wrap(err, "eosfs: no user in ctx")
		}
		auth, err = fs.getUserAuth(ctx, u, "")
		if err != nil {
			return err
		}
	}

	return fs.c.RestoreDeletedEntry(ctx, auth, key)
}

func (fs *Eosfs) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("list storage spaces")
}

// UpdateStorageSpace updates a storage space.
func (fs *Eosfs) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("update storage space")
}

func (fs *Eosfs) convertToRecycleItem(ctx context.Context, eosDeletedItem *eosclient.DeletedEntry) (*provider.RecycleItem, error) {
	path, err := fs.unwrap(ctx, eosDeletedItem.RestorePath)
	if err != nil {
		return nil, err
	}
	recycleItem := &provider.RecycleItem{
		Ref:          &provider.Reference{Path: path},
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

func (fs *Eosfs) convertToRevision(ctx context.Context, eosFileInfo *eosclient.FileInfo) (*provider.FileVersion, error) {
	md, err := fs.convertToResourceInfo(ctx, eosFileInfo)
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

func (fs *Eosfs) convertToResourceInfo(ctx context.Context, eosFileInfo *eosclient.FileInfo) (*provider.ResourceInfo, error) {
	return fs.convert(ctx, eosFileInfo)
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
			return conversions.NewEditorRole().CS3ResourcePermissions()
		case "uploader":
			return conversions.NewUploaderRole().CS3ResourcePermissions()
		}
		return conversions.NewViewerRole().CS3ResourcePermissions()
	}

	if role, ok := utils.HasOCMShareRole(u); ok {
		if role == "editor" {
			return conversions.NewEditorRole().CS3ResourcePermissions()
		}
		return conversions.NewViewerRole().CS3ResourcePermissions()
	}

	if utils.UserEqual(u.Id, owner) {
		return conversions.NewManagerRole().CS3ResourcePermissions()
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
	}

	parseAndSetFavoriteAttr(ctx, filteredAttrs)

	info := &provider.ResourceInfo{
		Id:            &provider.ResourceId{OpaqueId: fmt.Sprintf("%d", eosFileInfo.Inode)},
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

func (fs *Eosfs) extractUIDAndGID(u *userpb.User) (eosclient.Authorization, error) {
	if u.UidNumber == 0 {
		return eosclient.Authorization{}, errors.New("eosfs: uid missing for user")
	}
	if u.GidNumber == 0 {
		return eosclient.Authorization{}, errors.New("eosfs: gid missing for user")
	}
	return eosclient.Authorization{Role: eosclient.Role{UID: strconv.FormatInt(u.UidNumber, 10), GID: strconv.FormatInt(u.GidNumber, 10)}}, nil
}

func (fs *Eosfs) getUIDGateway(ctx context.Context, u *userpb.UserId) (eosclient.Authorization, error) {
	log := appctx.GetLogger(ctx)
	if userIDInterface, err := fs.userIDCache.Get(u.OpaqueId); err == nil {
		log.Debug().Msg("eosfs: found cached user " + u.OpaqueId)
		return fs.extractUIDAndGID(userIDInterface.(*userpb.User))
	}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.conf.GatewaySvc))
	if err != nil {
		return eosclient.Authorization{}, errors.Wrap(err, "eosfs: error getting gateway grpc client")
	}
	getUserResp, err := client.GetUser(ctx, &userpb.GetUserRequest{
		UserId:                 u,
		SkipFetchingUserGroups: true,
	})
	if err != nil {
		_ = fs.userIDCache.SetWithTTL(u.OpaqueId, &userpb.User{}, 12*time.Hour)
		return eosclient.Authorization{}, errors.Wrap(err, "eosfs: error getting user")
	}
	if getUserResp.Status.Code != rpc.Code_CODE_OK {
		_ = fs.userIDCache.SetWithTTL(u.OpaqueId, &userpb.User{}, 12*time.Hour)
		return eosclient.Authorization{}, status.NewErrorFromCode(getUserResp.Status.Code, "eosfs")
	}

	_ = fs.userIDCache.Set(u.OpaqueId, getUserResp.User)
	return fs.extractUIDAndGID(getUserResp.User)
}

func (fs *Eosfs) getUserIDGateway(ctx context.Context, uid string) (*userpb.UserId, error) {
	log := appctx.GetLogger(ctx)
	// Handle the case of root
	if uid == "0" {
		return nil, errtypes.BadRequest("eosfs: cannot return root user")
	}

	if userIDInterface, err := fs.userIDCache.Get(uid); err == nil {
		log.Debug().Msg("eosfs: found cached uid " + uid)
		return userIDInterface.(*userpb.UserId), nil
	}

	log.Debug().Msg("eosfs: retrieving user from gateway for uid " + uid)
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.conf.GatewaySvc))
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error getting gateway grpc client")
	}
	getUserResp, err := client.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim:                  "uid",
		Value:                  uid,
		SkipFetchingUserGroups: true,
	})
	if err != nil {
		// Insert an empty object in the cache so that we don't make another call
		// for a specific amount of time
		_ = fs.userIDCache.SetWithTTL(uid, &userpb.UserId{}, 12*time.Hour)
		return nil, errors.Wrap(err, "eosfs: error getting user")
	}
	if getUserResp.Status.Code != rpc.Code_CODE_OK {
		// Insert an empty object in the cache so that we don't make another call
		// for a specific amount of time
		_ = fs.userIDCache.SetWithTTL(uid, &userpb.UserId{}, 12*time.Hour)
		return nil, status.NewErrorFromCode(getUserResp.Status.Code, "eosfs")
	}

	_ = fs.userIDCache.Set(uid, getUserResp.User.Id)
	return getUserResp.User.Id, nil
}

func (fs *Eosfs) getUserAuth(ctx context.Context, u *userpb.User, fn string) (eosclient.Authorization, error) {
	if fs.conf.ForceSingleUserMode {
		if fs.singleUserAuth.Role.UID != "" && fs.singleUserAuth.Role.GID != "" {
			return fs.singleUserAuth, nil
		}
		var err error
		fs.singleUserAuth, err = fs.getUIDGateway(ctx, &userpb.UserId{OpaqueId: fs.conf.SingleUsername})
		return fs.singleUserAuth, err
	}

	if utils.IsLightweightUser(u) {
		return fs.getEOSToken(ctx, u, fn)
	}

	return fs.extractUIDAndGID(u)
}

// Generate an EOS token that acts on behalf of the owner of the file `fn`
func (fs *Eosfs) getEOSToken(ctx context.Context, u *userpb.User, fn string) (eosclient.Authorization, error) {
	if fn == "" {
		return eosclient.Authorization{}, errtypes.BadRequest("eosfs: path cannot be empty")
	}

	daemonAuth, err := fs.getDaemonAuth(ctx)
	info, err := fs.c.GetFileInfoByPath(ctx, daemonAuth, fn)
	if err != nil {
		return eosclient.Authorization{}, err
	}
	auth := eosclient.Authorization{
		Role: eosclient.Role{
			UID: strconv.FormatUint(info.UID, 10),
			GID: strconv.FormatUint(info.GID, 10),
		},
	}

	perm := "rwx"
	for _, e := range info.SysACL.Entries {
		if e.Type == acl.TypeLightweight && e.Qualifier == u.Id.OpaqueId {
			perm = e.Permissions
			break
		}
	}

	p := path.Clean(fn)
	for p != "." && p != fs.conf.Namespace {
		key := p + "!" + perm
		if tknIf, err := fs.tokenCache.Get(key); err == nil {
			return eosclient.Authorization{Token: tknIf.(string)}, nil
		}
		p = path.Dir(p)
	}

	if info.IsDir {
		// EOS expects directories to have a trailing slash when generating tokens
		fn = path.Clean(fn) + "/"
	}
	tkn, err := fs.c.GenerateToken(ctx, auth, fn, &acl.Entry{Permissions: perm})
	if err != nil {
		return eosclient.Authorization{}, err
	}

	key := path.Clean(fn) + "!" + perm
	_ = fs.tokenCache.SetWithExpire(key, tkn, time.Second*time.Duration(fs.conf.TokenExpiry))

	return eosclient.Authorization{Token: tkn}, nil
}

// Returns an eosclient.Authorization object with the uid/gid of the daemon user
// This is a system user with read-only access to files.
// We use it e.g. when retrieving metadata from a file when accessing through a guest account,
// so we can look up which user to impersonate (i.e. the owner)
func (fs *Eosfs) getDaemonAuth(ctx context.Context) (eosclient.Authorization, error) {
	if fs.conf.ForceSingleUserMode {
		if fs.singleUserAuth.Role.UID != "" && fs.singleUserAuth.Role.GID != "" {
			return fs.singleUserAuth, nil
		}
		var err error
		fs.singleUserAuth, err = fs.getUIDGateway(ctx, &userpb.UserId{OpaqueId: fs.conf.SingleUsername})
		return fs.singleUserAuth, err
	}
	return utils.GetDaemonAuth(), nil
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
