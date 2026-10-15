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

package ocm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typepb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/mime"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/fs/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"github.com/studio-b12/gowebdav"
)

func init() {
	registry.Register("ocmreceived", New)
}

type cachedClient struct {
	client     *gowebdav.Client
	share      *ocmpb.ReceivedShare
	authHeader string
	mode       string
	// expiresAt is the absolute expiry of an exchanged access token.
	// Zero means the cache TTL alone decides, which is the legacy path.
	expiresAt time.Time
}

type driver struct {
	c               *config
	providerDomain  string
	ccache          *ttlcache.Cache
	discoveryCache  *ttlcache.Cache
	exchangeMiss    *ttlcache.Cache
	ocmClient       *ocmd.OCMClient
	webdavTransport http.RoundTripper
}

type config struct {
	GatewaySVC              string `mapstructure:"gatewaysvc"`
	ProviderDomain          string `mapstructure:"provider_domain"`
	OCMClientTimeout        int    `mapstructure:"ocm_timeout"`
	OCMClientInsecure       bool   `mapstructure:"ocm_insecure"`
	AllowLoopbackFederation bool   `mapstructure:"allow_loopback_federation"`
	UseEnvProxy             bool   `mapstructure:"ocm_use_env_proxy"`
	// AllowedFederationCIDRs admits explicit private ranges on this driver's
	// OCM client and WebDAV transport. Empty by default. Set the same
	// allowed_federation_cidrs key on both the storage provider and the data
	// provider; configuring one does not authorize the other.
	AllowedFederationCIDRs []string `mapstructure:"allowed_federation_cidrs"`
}

func (c *config) ApplyDefaults() {
	c.GatewaySVC = sharedconf.GetGatewaySVC(c.GatewaySVC)
	if c.OCMClientTimeout == 0 {
		c.OCMClientTimeout = 10
	}
}

// New creates an OCM storage driver.
// This driver exposes remote OCM resources to local users.
func New(ctx context.Context, m map[string]any) (storage.FS, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	if err := providerdomain.Validate(c.ProviderDomain); err != nil {
		return nil, errors.Wrap(err, "ocmreceived")
	}

	// Parse before caches or clients exist so an invalid element fails New
	// with no partial policy and no network client.
	allowedCIDRs, err := client.ParseFederationCIDRs(c.AllowedFederationCIDRs)
	if err != nil {
		return nil, errors.Wrapf(err, "ocmreceived: invalid allowed_federation_cidrs")
	}

	disco := ttlcache.NewCache()
	_ = disco.SetTTL(5 * time.Minute)
	// Hits must not extend the failure window, or a busy share would never
	// try token exchange again.
	miss := ttlcache.NewCache()
	_ = miss.SetTTL(exchangeFailureTTL)
	miss.SkipTTLExtensionOnHit(true)

	timeout := time.Duration(c.OCMClientTimeout) * time.Second
	insecure := c.OCMClientInsecure
	allowLoopback := c.AllowLoopbackFederation
	tcfg := client.TransportConfig{
		Timeout:                timeout,
		Insecure:               insecure,
		AllowLoopback:          allowLoopback,
		UseEnvProxy:            c.UseEnvProxy,
		AllowedFederationCIDRs: allowedCIDRs,
	}

	// Loopback is opt-in for local federation tests; production stays public-only.
	// Enforce at dial time (after DNS) so redirects and rebinding cannot hide a private target.
	d := &driver{
		c:               &c,
		providerDomain:  c.ProviderDomain,
		ccache:          ttlcache.NewCache(),
		discoveryCache:  disco,
		exchangeMiss:    miss,
		ocmClient:       ocmd.NewPublicOnlyClientWithConfig(tcfg),
		webdavTransport: client.NewPublicOnlyRoundTripper(tcfg),
	}
	return d, nil
}

func (d *driver) newWebDAVClient(endpoint string, headers map[string]string) *gowebdav.Client {
	c := gowebdav.NewClient(endpoint, "", "")
	for key, value := range headers {
		c.SetHeader(key, value)
	}
	c.SetTransport(d.webdavTransport)
	return c
}

func shareInfoFromPath(path string) (*ocmpb.ShareId, string) {
	// the path is of the type /share_id[/rel_path]
	shareID, rel := router.ShiftPath(path)
	return &ocmpb.ShareId{OpaqueId: shareID}, rel
}

func shareInfoFromReference(ref *provider.Reference) (*ocmpb.ShareId, string) {
	if ref.ResourceId == nil {
		return shareInfoFromPath(ref.Path)
	}

	s := strings.SplitN(ref.ResourceId.OpaqueId, ":", 2)
	shareID := &ocmpb.ShareId{OpaqueId: s[0]}
	var path string
	if len(s) == 2 {
		path = s[1]
	}
	path = filepath.Join(path, ref.Path)

	return shareID, path
}

// fileShareName returns the name a single-file share exposes in its path, if the remote-provided name is usable.
func fileShareName(share *ocmpb.ReceivedShare) (string, bool) {
	name := share.GetName()
	if share.SharedResourceType != ocmpb.SharedResourceType_SHARE_RESOURCE_TYPE_FILE || name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}

// resolveRelPath maps rel onto the remote resource; a single-file share only contains its root and its own name.
func resolveRelPath(share *ocmpb.ReceivedShare, rel string) (string, error) {
	if share.SharedResourceType != ocmpb.SharedResourceType_SHARE_RESOURCE_TYPE_FILE {
		return rel, nil
	}
	p := filepath.Clean("/" + rel)
	if name, ok := fileShareName(share); p == "/" || ok && p == "/"+name {
		return "/", nil
	}
	return "", errtypes.NotFound(rel)
}

func (d *driver) getWebDAVFromShare(ctx context.Context, shareID *ocmpb.ShareId) (*ocmpb.ReceivedShare, string, string, error) {
	gw, err := service.Gateway(ctx)
	if err != nil {
		return nil, "", "", err
	}
	res, err := gw.GetReceivedOCMShare(ctx, &ocmpb.GetReceivedOCMShareRequest{
		Ref: &ocmpb.ShareReference{
			Spec: &ocmpb.ShareReference_Id{
				Id: shareID,
			},
		},
	})
	if err != nil {
		return nil, "", "", err
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return nil, "", "", errtypes.NotFound("share not found")
		}
		return nil, "", "", errtypes.InternalError(res.Status.Message)
	}

	dav, ok := getWebDAVProtocol(res.Share.Protocols)
	if !ok {
		return nil, "", "", errtypes.NotFound("share does not contain a WebDAV endpoint")
	}

	return res.Share, dav.Uri, dav.SharedSecret, nil
}

func getWebDAVProtocol(protocols []*ocmpb.Protocol) (*ocmpb.WebDAVProtocol, bool) {
	for _, p := range protocols {
		if dav, ok := p.Term.(*ocmpb.Protocol_WebdavOptions); ok {
			return dav.WebdavOptions, true
		}
	}
	return nil, false
}

func requiresExchange(protocols []*ocmpb.Protocol) bool {
	for _, p := range protocols {
		if dav, ok := p.Term.(*ocmpb.Protocol_WebdavOptions); ok {
			if slices.Contains(dav.WebdavOptions.Requirements, "must-exchange-token") {
				return true
			}
		}
	}
	return false
}

func (d *driver) getTokenEndpoint(ctx context.Context, share *ocmpb.ReceivedShare) (string, error) {
	offer, err := d.lookupSenderOffer(ctx, share)
	if err != nil {
		return "", err
	}
	// A negative cache entry has an empty endpoint. Strict callers still
	// receive NotFound and never a successful empty endpoint.
	if offer == nil || offer.tokenEndpoint == "" {
		idp := strings.TrimSpace(share.GetCreator().GetIdp())
		return "", errtypes.NotFound("sender discovery at " + idp + " has no tokenEndPoint")
	}
	return offer.tokenEndpoint, nil
}

func isWebDAV401(err error) bool {
	return gowebdav.IsErrCode(err, http.StatusUnauthorized)
}

func (d *driver) exchangeAccessToken(ctx context.Context, tokenEndpoint, secret string) (string, int64, error) {
	if err := providerdomain.Validate(d.providerDomain); err != nil {
		return "", 0, err
	}
	accessToken, expiresIn, err := d.ocmClient.ExchangeToken(ctx, tokenEndpoint, secret, d.providerDomain)
	if err != nil {
		return "", 0, err
	}
	return accessToken, expiresIn, nil
}

func (d *driver) webdavClient(ctx context.Context, ref *provider.Reference) (*gowebdav.Client, *ocmpb.ReceivedShare, string, error) {
	log := appctx.GetLogger(ctx)
	id, rel := shareInfoFromReference(ref)

	share, endpoint, secret, err := d.getWebDAVFromShare(ctx, id)
	if err != nil {
		return nil, nil, "", err
	}
	if rel, err = resolveRelPath(share, rel); err != nil {
		return nil, nil, "", err
	}
	endpoint, err = url.PathUnescape(endpoint)
	if err != nil {
		return nil, nil, "", err
	}

	if !requiresExchange(share.Protocols) {
		if cc, ok := d.takeCachedClient(id.OpaqueId); ok {
			log.Info().
				Str("shareId", cc.share.GetId().GetOpaqueId()).
				Str("rel", rel).
				Str("mode", cc.mode).
				Msg("accessing OCM share via cached client")
			return cc.client, cc.share, rel, nil
		}
		cc, err := d.resolveOpportunistic(ctx, share, endpoint, secret, id.OpaqueId)
		if err != nil {
			return nil, nil, "", err
		}
		if cc != nil {
			log.Info().
				Str("endpoint", endpoint).
				Str("shareId", share.GetId().GetOpaqueId()).
				Str("mode", modeExchange).
				Msg("auth selected for remote OCM share")
			return cc.client, share, rel, nil
		}
		return d.legacyWebDAVClient(ctx, id.OpaqueId, share, endpoint, secret, rel)
	}

	// code-flow path: exchange token every time, no cache
	tokenEndpoint, err := d.getTokenEndpoint(ctx, share)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "could not discover token endpoint for code-flow share")
	}
	accessToken, _, err := d.exchangeAccessToken(ctx, tokenEndpoint, secret)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "token exchange failed")
	}
	c := d.newWebDAVClient(endpoint, map[string]string{
		"Authorization": "Bearer " + accessToken,
	})
	return c, share, rel, nil
}

// withExchangeRetry wraps a unary DAV operation (CreateDir, Delete, TouchFile).
func (d *driver) withExchangeRetry(ctx context.Context, ref *provider.Reference, fn func(*gowebdav.Client, string) error) error {
	_, _, _, err := retryUnauthorized(
		d,
		ctx,
		ref,
		"remote OCM access denied after token re-exchange",
		func(c *gowebdav.Client, rel string) (struct{}, error) {
			return struct{}{}, fn(c, rel)
		},
	)
	return err
}

func (d *driver) CreateDir(ctx context.Context, ref *provider.Reference) error {
	return d.withExchangeRetry(ctx, ref, func(c *gowebdav.Client, rel string) error {
		return c.MkdirAll(rel, 0)
	})
}

func (d *driver) Delete(ctx context.Context, ref *provider.Reference) error {
	return d.withExchangeRetry(ctx, ref, func(c *gowebdav.Client, rel string) error {
		return c.RemoveAll(rel)
	})
}

func (d *driver) TouchFile(ctx context.Context, ref *provider.Reference) error {
	return d.withExchangeRetry(ctx, ref, func(c *gowebdav.Client, rel string) error {
		return c.Write(rel, []byte{}, 0)
	})
}

// withExchangeMoveRetry resolves both refs, rejects cross-share moves,
// then retries on 401 like the unary helper.
func (d *driver) withExchangeMoveRetry(ctx context.Context, oldRef, newRef *provider.Reference, fn func(*gowebdav.Client, string, string) error) error {
	oldID, _ := shareInfoFromReference(oldRef)
	newID, relNew := shareInfoFromReference(newRef)
	if oldID.OpaqueId != newID.OpaqueId {
		return errtypes.BadRequest("cross-share move is not supported")
	}
	share, _, _, err := d.getWebDAVFromShare(ctx, oldID)
	if err != nil {
		return err
	}
	if relNew, err = resolveRelPath(share, relNew); err != nil {
		return err
	}
	_, _, _, err = retryUnauthorized(
		d,
		ctx,
		oldRef,
		"remote OCM access denied after token re-exchange",
		func(c *gowebdav.Client, relOld string) (struct{}, error) {
			return struct{}{}, fn(c, relOld, relNew)
		},
	)
	return err
}

func (d *driver) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	return d.withExchangeMoveRetry(ctx, oldRef, newRef, func(c *gowebdav.Client, relOld, relNew string) error {
		return c.Rename(relOld, relNew, false)
	})
}

func getResourceInfo(shareID *ocmpb.ShareId, relPath string) *provider.ResourceId {
	return &provider.ResourceId{
		OpaqueId: fmt.Sprintf("%s:%s", shareID.OpaqueId, relPath),
	}
}

func getPathFromShareIDAndRelPath(shareID *ocmpb.ShareId, relPath string) string {
	return filepath.Join("/", shareID.OpaqueId, relPath)
}

func convertStatToResourceInfo(f fs.FileInfo, share *ocmpb.ReceivedShare, relPath string) *provider.ResourceInfo {
	t := provider.ResourceType_RESOURCE_TYPE_FILE
	if f.IsDir() {
		t = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}

	var name string
	switch {
	case share.SharedResourceType == ocmpb.SharedResourceType_SHARE_RESOURCE_TYPE_FILE:
		name = share.Name
	case f.Name() != "":
		name = f.Name()
	default:
		// gowebdav's Stat only reads displayname, which reva doesn't serve
		name = filepath.Base(filepath.Join(share.Name, relPath))
	}

	pathRel := relPath
	if n, ok := fileShareName(share); ok {
		// expose the file name in the path, like single-file public links
		pathRel = n
	}

	webdav, _ := getWebDAVProtocol(share.Protocols)

	return &provider.ResourceInfo{
		Type:     t,
		Id:       getResourceInfo(share.Id, relPath),
		MimeType: mime.Detect(f.IsDir(), name),
		Path:     getPathFromShareIDAndRelPath(share.Id, pathRel),
		Name:     name,
		Size:     uint64(f.Size()),
		Mtime: &typepb.Timestamp{
			Seconds: uint64(f.ModTime().Unix()),
		},
		Owner:         share.Creator,
		PermissionSet: webdav.Permissions.Permissions,
		Checksum: &provider.ResourceChecksum{
			Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID,
		},
	}
}

// withExchangeStatRetry wraps a Stat-like operation with 401 retry.
// It returns fresh share metadata after retry so callers never use stale data.
func (d *driver) withExchangeStatRetry(ctx context.Context, ref *provider.Reference, fn func(*gowebdav.Client, string) (fs.FileInfo, error)) (fs.FileInfo, *ocmpb.ReceivedShare, string, error) {
	return retryUnauthorized(d, ctx, ref, "remote OCM access denied after token re-exchange", fn)
}

func (d *driver) GetMD(ctx context.Context, ref *provider.Reference, _ []string) (*provider.ResourceInfo, error) {
	log := appctx.GetLogger(ctx)
	info, share, rel, err := d.withExchangeStatRetry(ctx, ref, func(c *gowebdav.Client, rel string) (fs.FileInfo, error) {
		return c.Stat(rel)
	})
	if err != nil {
		log.Error().Err(err).Msg("error stating resource")
		if gowebdav.IsErrNotFound(err) || strings.HasSuffix(err.Error(), "404") {
			return nil, errtypes.NotFound(ref.GetPath())
		}
		return nil, err
	}
	return convertStatToResourceInfo(info, share, rel), nil
}

// withExchangeListRetry wraps a ReadDir-like operation with 401 retry.
func (d *driver) withExchangeListRetry(ctx context.Context, ref *provider.Reference, fn func(*gowebdav.Client, string) ([]fs.FileInfo, error)) ([]fs.FileInfo, *ocmpb.ReceivedShare, string, error) {
	return retryUnauthorized(d, ctx, ref, "remote OCM access denied after token re-exchange", fn)
}

func (d *driver) ListFolder(ctx context.Context, ref *provider.Reference, _ []string) ([]*provider.ResourceInfo, error) {
	list, share, rel, err := d.withExchangeListRetry(ctx, ref, func(c *gowebdav.Client, rel string) ([]fs.FileInfo, error) {
		return c.ReadDir(rel)
	})
	if err != nil {
		return nil, err
	}
	res := make([]*provider.ResourceInfo, 0, len(list))
	for _, r := range list {
		res = append(res, convertStatToResourceInfo(r, share, filepath.Join(rel, r.Name())))
	}
	return res, nil
}

func (d *driver) InitiateUpload(ctx context.Context, ref *provider.Reference, _ int64, _ map[string]string) (map[string]string, error) {
	shareID, rel := shareInfoFromReference(ref)
	p := getPathFromShareIDAndRelPath(shareID, rel)

	return map[string]string{
		"simple": p,
	}, nil
}

// uploadOnFreshClient creates a one-off DAV client for the upload so that
// Upload-Length: -1 does not leak onto a cached client used by later operations.
func (d *driver) uploadOnFreshClient(endpoint, authHeader, rel string, body io.Reader) error {
	c := d.newWebDAVClient(endpoint, map[string]string{
		"Authorization":          authHeader,
		ocdav.HeaderUploadLength: "-1",
	})
	return c.WriteStream(rel, body, 0)
}

func (d *driver) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, _ map[string]string) error {
	defer r.Close()

	buf, err := io.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "error buffering upload body")
	}

	id, rel := shareInfoFromReference(ref)
	rejected := ""
	for attempt := 0; attempt < 3; attempt++ {
		share, endpoint, secret, err := d.getWebDAVFromShare(ctx, id)
		if err != nil {
			return err
		}
		if rel, err = resolveRelPath(share, rel); err != nil {
			return err
		}
		endpoint, err = url.PathUnescape(endpoint)
		if err != nil {
			return err
		}
		if rejected == modeExchange {
			d.rememberExchangeFailure(id.OpaqueId)
			_ = d.ccache.Remove(id.OpaqueId)
		}
		authHeader, err := d.uploadAuth(ctx, share, endpoint, secret, id)
		if err != nil {
			return err
		}
		mode := d.sessionMode(id.OpaqueId, share.GetProtocols())
		err = d.uploadOnFreshClient(endpoint, authHeader, rel, bytes.NewReader(buf))
		if err == nil {
			return nil
		}
		if !isWebDAV401(err) {
			return err
		}
		// A second 401 on anything except an exchanged token, or a 401 after
		// the legacy fallback, is an access denial.
		gaveUp := attempt == 2 || (attempt == 1 && mode != modeExchange)
		if gaveUp {
			return errtypes.InvalidCredentials("remote OCM upload denied after token re-exchange")
		}
		_ = d.ccache.Remove(id.OpaqueId)
		if attempt == 1 && mode == modeExchange {
			rejected = modeExchange
		}
	}
	return errtypes.InvalidCredentials("remote OCM upload denied after token re-exchange")
}

// uploadAuth returns the Authorization header value for an upload attempt.
// Strict shares exchange a token on every call. Other shares reuse a cached
// header, try an opportunistic exchange, then use selectLegacyWebDAVAuth.
func (d *driver) uploadAuth(ctx context.Context, share *ocmpb.ReceivedShare, endpoint, secret string, id *ocmpb.ShareId) (string, error) {
	log := appctx.GetLogger(ctx)
	if requiresExchange(share.Protocols) {
		tokenEndpoint, err := d.getTokenEndpoint(ctx, share)
		if err != nil {
			return "", errors.Wrap(err, "could not discover token endpoint for upload")
		}
		accessToken, _, err := d.exchangeAccessToken(ctx, tokenEndpoint, secret)
		if err != nil {
			return "", errors.Wrap(err, "token exchange failed for upload")
		}
		return "Bearer " + accessToken, nil
	}
	if cc, ok := d.takeCachedClient(id.OpaqueId); ok {
		return cc.authHeader, nil
	}
	cc, err := d.resolveOpportunistic(ctx, share, endpoint, secret, id.OpaqueId)
	if err != nil {
		return "", err
	}
	if cc != nil {
		log.Info().
			Str("endpoint", endpoint).
			Str("shareId", share.GetId().GetOpaqueId()).
			Str("mode", modeExchange).
			Msg("auth selected for remote OCM share")
		return cc.authHeader, nil
	}
	authHeader, _, _, err := d.selectLegacyWebDAVAuth(
		ctx,
		share,
		endpoint,
		secret,
	)
	if err != nil {
		return "", err
	}
	return authHeader, nil
}

// withExchangeDownloadOpenRetry wraps ReadStream open with retryUnauthorized
// (re-open, re-exchange, one further legacy attempt).
// Once a stream is returned, no mid-stream recovery is attempted.
func (d *driver) withExchangeDownloadOpenRetry(ctx context.Context, ref *provider.Reference, fn func(*gowebdav.Client, string) (io.ReadCloser, error)) (io.ReadCloser, error) {
	rc, _, _, err := retryUnauthorized(d, ctx, ref, "remote OCM download denied after token re-exchange", fn)
	return rc, err
}

func (d *driver) Download(ctx context.Context, ref *provider.Reference, ranges []storage.Range) (io.ReadCloser, error) {
	if len(ranges) > 0 {
		return nil, errtypes.NotSupported("Download with ranges is not supported with this storage driver")
	}
	return d.withExchangeDownloadOpenRetry(ctx, ref, func(c *gowebdav.Client, rel string) (io.ReadCloser, error) {
		return c.ReadStream(rel)
	})
}

func (d *driver) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	shareID, rel := shareInfoFromReference(&provider.Reference{
		ResourceId: id,
	})
	return getPathFromShareIDAndRelPath(shareID, rel), nil
}

func (d *driver) Shutdown(ctx context.Context) error {
	return nil
}

func (d *driver) CreateHome(ctx context.Context) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) GetHome(ctx context.Context) (string, error) {
	return "", errtypes.NotSupported("operation not supported")
}

func (d *driver) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) ListRecycle(ctx context.Context, basePath, key, relativePath string, from, to *typepb.Timestamp) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) GetQuota(ctx context.Context, ref *provider.Reference) ( /*TotalBytes*/ uint64 /*UsedBytes*/, uint64, error) {
	return 0, 0, errtypes.NotSupported("operation not supported")
}

func (d *driver) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock, existingLockID string) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("operation not supported")
}

func (d *driver) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("operation not supported")
}

func (d *driver) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("operation not supported")
}
