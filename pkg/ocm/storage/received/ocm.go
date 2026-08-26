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
	"encoding/base64"
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
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typepb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/mime"
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
}

type driver struct {
	c              *config
	ccache         *ttlcache.Cache
	discoveryCache *ttlcache.Cache
	ocmClient      *ocmd.OCMClient
}

type config struct {
	GatewaySVC        string `mapstructure:"gatewaysvc"`
	OCMClientTimeout  int    `mapstructure:"ocm_timeout"`
	OCMClientInsecure bool   `mapstructure:"ocm_insecure"`
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

	disco := ttlcache.NewCache()
	_ = disco.SetTTL(5 * time.Minute)

	d := &driver{
		c:              &c,
		ccache:         ttlcache.NewCache(),
		discoveryCache: disco,
		ocmClient:      ocmd.NewClient(time.Duration(c.OCMClientTimeout)*time.Second, c.OCMClientInsecure),
	}
	return d, nil
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
	dav, ok := getWebDAVProtocol(share.Protocols)
	if !ok {
		return "", errtypes.NotFound("share has no WebDAV protocol")
	}

	parsed, err := url.Parse(dav.Uri)
	if err != nil {
		return "", errors.Wrap(err, "could not parse sender WebDAV URI")
	}
	origin := parsed.Scheme + "://" + parsed.Host

	if entry, err := d.discoveryCache.Get(origin); err == nil {
		return entry.(string), nil
	}

	disco, err := d.ocmClient.Discover(ctx, origin)
	if err != nil {
		return "", errors.Wrap(err, "OCM discovery failed for "+origin)
	}
	if disco.TokenEndPoint == "" {
		return "", errtypes.NotFound("sender discovery at " + origin + " has no tokenEndPoint")
	}

	_ = d.discoveryCache.Set(origin, disco.TokenEndPoint)
	return disco.TokenEndPoint, nil
}

func isWebDAV401(err error) bool {
	return gowebdav.IsErrCode(err, http.StatusUnauthorized)
}

func receiverClientID(ctx context.Context, share *ocmpb.ReceivedShare) string {
	if u, ok := appctx.ContextGetUser(ctx); ok && u.GetId() != nil && u.GetId().GetIdp() != "" {
		return u.GetId().GetIdp()
	}
	if share != nil && share.GetGrantee() != nil && share.GetGrantee().GetUserId() != nil {
		return share.GetGrantee().GetUserId().GetIdp()
	}
	return ""
}

func receiverClientIDWithLookup(ctx context.Context, share *ocmpb.ReceivedShare, lookup func(context.Context, *userpb.UserId) string) string {
	clientID := receiverClientID(ctx, share)
	if clientID == "" && lookup != nil && share != nil && share.GetGrantee() != nil && share.GetGrantee().GetUserId() != nil {
		clientID = lookup(ctx, share.GetGrantee().GetUserId())
	}
	return clientID
}

func (d *driver) lookupReceiverUserIDP(ctx context.Context, userID *userpb.UserId) string {
	if d == nil || userID == nil || userID.GetOpaqueId() == "" {
		return ""
	}

	gw, err := service.Gateway(ctx)
	if err != nil {
		return ""
	}

	res, err := gw.GetUser(ctx, &userpb.GetUserRequest{
		UserId:                 userID,
		SkipFetchingUserGroups: true,
	})
	if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK || res.GetUser() == nil || res.GetUser().GetId() == nil {
		return ""
	}
	return res.GetUser().GetId().GetIdp()
}

func (d *driver) exchangeAccessToken(ctx context.Context, share *ocmpb.ReceivedShare, tokenEndpoint, secret string) (string, error) {
	clientID := receiverClientIDWithLookup(ctx, share, d.lookupReceiverUserIDP)
	accessToken, _, err := d.ocmClient.ExchangeToken(ctx, tokenEndpoint, secret, clientID)
	if err != nil {
		return "", err
	}
	return accessToken, nil
}

func (d *driver) webdavClient(ctx context.Context, ref *provider.Reference) (*gowebdav.Client, *ocmpb.ReceivedShare, string, error) {
	log := appctx.GetLogger(ctx)
	id, rel := shareInfoFromReference(ref)

	share, endpoint, secret, err := d.getWebDAVFromShare(ctx, id)
	if err != nil {
		return nil, nil, "", err
	}
	endpoint, err = url.PathUnescape(endpoint)
	if err != nil {
		return nil, nil, "", err
	}

	if !requiresExchange(share.Protocols) {
		// legacy path: check cache first
		if entry, err := d.ccache.Get(id.OpaqueId); err == nil {
			cc := entry.(*cachedClient)
			log.Info().Str("shareId", cc.share.GetId().GetOpaqueId()).Str("rel", rel).Msg("accessing OCM share via cached client")
			return cc.client, cc.share, rel, nil
		}

		// build legacy client: try bearer (OCM v1.1+), then basic (OCM v1.0)
		var c *gowebdav.Client
		var authHdr string
		bearerHdr := "Bearer " + secret
		c = gowebdav.NewClient(endpoint, "", "")
		c.SetHeader("Authorization", bearerHdr)
		_, err = c.Stat("")
		if err != nil {
			basicHdr := "Basic " + base64.StdEncoding.EncodeToString([]byte(secret+":"))
			c = gowebdav.NewClient(endpoint, "", "")
			c.SetHeader("Authorization", basicHdr)
			_, err2 := c.Stat("")
			if err2 != nil {
				log.Info().Any("former_error", err).Err(err2).Str("endpoint", endpoint).Str("shareId", share.GetId().GetOpaqueId()).Msg("failed accessing OCM share")
				return nil, nil, "", errtypes.InvalidCredentials("error accessing OCM share: " + err2.Error())
			}
			authHdr = basicHdr
			log.Info().Str("endpoint", endpoint).Str("shareId", share.GetId().GetOpaqueId()).Str("mode", "legacy").Msg("access to remote OCM share succeeded")
		} else {
			authHdr = bearerHdr
			log.Info().Str("endpoint", endpoint).Str("shareId", share.GetId().GetOpaqueId()).Str("mode", "bearer").Msg("access to remote OCM share succeeded")
		}

		d.ccache.SetWithTTL(id.OpaqueId, &cachedClient{client: c, share: share, authHeader: authHdr}, time.Hour)
		return c, share, rel, nil
	}

	// code-flow path: exchange token every time, no cache
	tokenEndpoint, err := d.getTokenEndpoint(ctx, share)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "could not discover token endpoint for code-flow share")
	}
	accessToken, err := d.exchangeAccessToken(ctx, share, tokenEndpoint, secret)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "token exchange failed")
	}
	c := gowebdav.NewClient(endpoint, "", "")
	c.SetHeader("Authorization", "Bearer "+accessToken)
	return c, share, rel, nil
}

// withExchangeRetry wraps a unary DAV operation (CreateDir, Delete, TouchFile).
// On 401 it invalidates the cache, re-calls webdavClient (which re-exchanges
// for code-flow shares), and retries once. Only a second 401 becomes
// InvalidCredentials; other second-attempt errors keep the underlying class.
func (d *driver) withExchangeRetry(ctx context.Context, ref *provider.Reference, fn func(*gowebdav.Client, string) error) error {
	client, _, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return err
	}
	if err = fn(client, rel); err == nil {
		return nil
	}
	if !isWebDAV401(err) {
		return err
	}
	id, _ := shareInfoFromReference(ref)
	d.ccache.Remove(id.OpaqueId)
	client, _, rel, err = d.webdavClient(ctx, ref)
	if err != nil {
		return err
	}
	if err = fn(client, rel); err != nil {
		if isWebDAV401(err) {
			return errtypes.InvalidCredentials("remote OCM access denied after token re-exchange")
		}
		return err
	}
	return nil
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
// then retries once on 401 like the unary helper.
func (d *driver) withExchangeMoveRetry(ctx context.Context, oldRef, newRef *provider.Reference, fn func(*gowebdav.Client, string, string) error) error {
	oldID, _ := shareInfoFromReference(oldRef)
	newID, relNew := shareInfoFromReference(newRef)
	if oldID.OpaqueId != newID.OpaqueId {
		return errtypes.BadRequest("cross-share move is not supported")
	}

	client, _, relOld, err := d.webdavClient(ctx, oldRef)
	if err != nil {
		return err
	}
	if err = fn(client, relOld, relNew); err == nil {
		return nil
	}
	if !isWebDAV401(err) {
		return err
	}
	d.ccache.Remove(oldID.OpaqueId)
	client, _, relOld, err = d.webdavClient(ctx, oldRef)
	if err != nil {
		return err
	}
	if err = fn(client, relOld, relNew); err != nil {
		if isWebDAV401(err) {
			return errtypes.InvalidCredentials("remote OCM access denied after token re-exchange")
		}
		return err
	}
	return nil
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
	if share.SharedResourceType == ocmpb.SharedResourceType_SHARE_RESOURCE_TYPE_FILE {
		name = share.Name
	} else {
		name = f.Name()
	}

	webdav, _ := getWebDAVProtocol(share.Protocols)

	return &provider.ResourceInfo{
		Type:     t,
		Id:       getResourceInfo(share.Id, relPath),
		MimeType: mime.Detect(f.IsDir(), f.Name()),
		Path:     getPathFromShareIDAndRelPath(share.Id, relPath),
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
	client, share, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return nil, nil, "", err
	}
	info, err := fn(client, rel)
	if err == nil {
		return info, share, rel, nil
	}
	if !isWebDAV401(err) {
		return nil, nil, "", err
	}
	id, _ := shareInfoFromReference(ref)
	d.ccache.Remove(id.OpaqueId)
	client, share, rel, err = d.webdavClient(ctx, ref)
	if err != nil {
		return nil, nil, "", err
	}
	info, err = fn(client, rel)
	if err != nil {
		if isWebDAV401(err) {
			return nil, nil, "", errtypes.InvalidCredentials("remote OCM access denied after token re-exchange")
		}
		return nil, nil, "", err
	}
	return info, share, rel, nil
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
	client, share, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return nil, nil, "", err
	}
	list, err := fn(client, rel)
	if err == nil {
		return list, share, rel, nil
	}
	if !isWebDAV401(err) {
		return nil, nil, "", err
	}
	id, _ := shareInfoFromReference(ref)
	d.ccache.Remove(id.OpaqueId)
	client, share, rel, err = d.webdavClient(ctx, ref)
	if err != nil {
		return nil, nil, "", err
	}
	list, err = fn(client, rel)
	if err != nil {
		if isWebDAV401(err) {
			return nil, nil, "", errtypes.InvalidCredentials("remote OCM access denied after token re-exchange")
		}
		return nil, nil, "", err
	}
	return list, share, rel, nil
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
func uploadOnFreshClient(endpoint, authHeader, rel string, body io.Reader) error {
	c := gowebdav.NewClient(endpoint, "", "")
	c.SetHeader("Authorization", authHeader)
	c.SetHeader(ocdav.HeaderUploadLength, "-1")
	return c.WriteStream(rel, body, 0)
}

func (d *driver) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, _ map[string]string) error {
	defer r.Close()

	buf, err := io.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "error buffering upload body")
	}

	id, rel := shareInfoFromReference(ref)
	share, endpoint, secret, err := d.getWebDAVFromShare(ctx, id)
	if err != nil {
		return err
	}
	endpoint, err = url.PathUnescape(endpoint)
	if err != nil {
		return err
	}

	authHeader, err := d.uploadAuth(ctx, share, endpoint, secret, id)
	if err != nil {
		return err
	}

	err = uploadOnFreshClient(endpoint, authHeader, rel, bytes.NewReader(buf))
	if err == nil {
		return nil
	}
	if !isWebDAV401(err) {
		return err
	}

	// retry once: re-derive auth (re-exchange for code-flow, same creds for legacy)
	d.ccache.Remove(id.OpaqueId)
	share, endpoint, secret, err = d.getWebDAVFromShare(ctx, id)
	if err != nil {
		return err
	}
	endpoint, err = url.PathUnescape(endpoint)
	if err != nil {
		return err
	}
	authHeader, err = d.uploadAuth(ctx, share, endpoint, secret, id)
	if err != nil {
		return err
	}
	if err = uploadOnFreshClient(endpoint, authHeader, rel, bytes.NewReader(buf)); err != nil {
		if isWebDAV401(err) {
			return errtypes.InvalidCredentials("remote OCM upload denied after token re-exchange")
		}
		return err
	}
	return nil
}

// uploadAuth returns the Authorization header value for an upload attempt.
// For code-flow shares it performs a token exchange; for legacy shares it
// returns the cached auth header (Bearer or Basic) if available, falling
// back to Bearer when the cache has not yet been populated.
func (d *driver) uploadAuth(ctx context.Context, share *ocmpb.ReceivedShare, endpoint, secret string, id *ocmpb.ShareId) (string, error) {
	if requiresExchange(share.Protocols) {
		tokenEndpoint, err := d.getTokenEndpoint(ctx, share)
		if err != nil {
			return "", errors.Wrap(err, "could not discover token endpoint for upload")
		}
		accessToken, err := d.exchangeAccessToken(ctx, share, tokenEndpoint, secret)
		if err != nil {
			return "", errors.Wrap(err, "token exchange failed for upload")
		}
		return "Bearer " + accessToken, nil
	}
	// legacy: use the auth header established during the first webdavClient probe
	if entry, err := d.ccache.Get(id.OpaqueId); err == nil {
		return entry.(*cachedClient).authHeader, nil
	}
	return "Bearer " + secret, nil
}

// withExchangeDownloadOpenRetry wraps ReadStream open with a single 401 retry.
// Once a stream is returned, no mid-stream recovery is attempted.
func (d *driver) withExchangeDownloadOpenRetry(ctx context.Context, ref *provider.Reference, fn func(*gowebdav.Client, string) (io.ReadCloser, error)) (io.ReadCloser, error) {
	client, _, rel, err := d.webdavClient(ctx, ref)
	if err != nil {
		return nil, err
	}
	rc, err := fn(client, rel)
	if err == nil {
		return rc, nil
	}
	if !isWebDAV401(err) {
		return nil, err
	}
	id, _ := shareInfoFromReference(ref)
	d.ccache.Remove(id.OpaqueId)
	client, _, rel, err = d.webdavClient(ctx, ref)
	if err != nil {
		return nil, err
	}
	rc, err = fn(client, rel)
	if err != nil {
		if isWebDAV401(err) {
			return nil, errtypes.InvalidCredentials("remote OCM download denied after token re-exchange")
		}
		return nil, err
	}
	return rc, nil
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
