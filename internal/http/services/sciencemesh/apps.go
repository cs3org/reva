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

package sciencemesh

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/service"
)

// launchClient is the remote discovery and token-exchange surface. Production
// uses one ocmd.NewPublicOnlyClient for both hops. Tests may install a factory.
type launchClient interface {
	Discover(ctx context.Context, endpoint string) (*wellknown.OcmDiscoveryData, error)
	ExchangeToken(ctx context.Context, tokenEndpoint, code, clientID string) (string, int64, error)
}

var _ launchClient = (*ocmd.OCMClient)(nil)

type appsHandler struct {
	ocmMountPoint   string
	receiverDomain  string
	clientTimeout   time.Duration
	clientInsecure  bool
	newLaunchClient func(timeout time.Duration, insecure bool) launchClient
}

type openInAppResponse struct {
	AppURL      string `json:"app_url"`
	AccessToken string `json:"access_token"`
}

func (h *appsHandler) init(c *config) error {
	h.ocmMountPoint = c.OCMMountPoint
	h.receiverDomain = c.ProviderDomain
	h.clientTimeout = time.Duration(c.OCMClientTimeout) * time.Second
	h.clientInsecure = c.OCMClientInsecure
	return nil
}

func (h *appsHandler) remoteClient() launchClient {
	if h.newLaunchClient != nil {
		return h.newLaunchClient(h.clientTimeout, h.clientInsecure)
	}
	return ocmd.NewPublicOnlyClient(h.clientTimeout, h.clientInsecure)
}

func (h *appsHandler) shareInfo(p string) (*ocmpb.ShareId, string) {
	p = strings.TrimPrefix(p, h.ocmMountPoint)
	shareID, rel := router.ShiftPath(p)
	if len(rel) > 0 {
		rel = rel[1:]
	}
	return &ocmpb.ShareId{OpaqueId: shareID}, rel
}

func (h *appsHandler) OpenInApp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "parameters could not be parsed", nil)
		return
	}

	filePath := r.Form.Get("file")
	if filePath == "" {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "missing file", nil)
		return
	}
	if err := validateShareFilePath(filePath); err != nil {
		writeLaunchError(w, r, err)
		return
	}

	shareID, rel := h.shareInfo(filePath)
	payload, err := h.buildLaunch(ctx, shareID, rel)
	if err != nil {
		writeLaunchError(w, r, err)
		return
	}
	writeLaunchJSON(w, r, payload)
}

func (h *appsHandler) buildLaunch(ctx context.Context, shareID *ocmpb.ShareId, rel string) (openInAppResponse, error) {
	var none openInAppResponse
	share, webapp, err := h.receivedWebapp(ctx, shareID)
	if err != nil {
		return none, redactLaunchError(err, "", "")
	}
	secret := webapp.GetSharedSecret()
	fail := func(err error, token string) (openInAppResponse, error) {
		return none, redactLaunchError(err, secret, token)
	}
	if rel != "" {
		if _, err = relativePathSegments(rel); err != nil {
			return fail(err, "")
		}
	}

	if strings.TrimSpace(h.receiverDomain) == "" {
		return fail(errtypes.BadRequest("provider domain is not configured"), "")
	}

	appRaw := strings.TrimSpace(webapp.GetUri())
	parsedApp, err := url.Parse(appRaw)
	if err != nil {
		return fail(errtypes.BadRequest("malformed remote URL"), "")
	}

	var appURI string
	relativeApp := pathRelativeURL(parsedApp)
	if !relativeApp {
		appURI, err = resolveAbsoluteAppURI(appRaw)
		if err != nil {
			return fail(err, "")
		}
		if _, err = joinShareRelativePath(appURI, rel); err != nil {
			return fail(err, "")
		}
	}

	origin, err := senderDiscoveryOrigin(share.GetProtocols())
	if err != nil {
		return fail(err, "")
	}

	client := h.remoteClient()
	if client == nil {
		return fail(errtypes.InternalError("launch client is not available"), "")
	}
	disco, err := client.Discover(ctx, origin)
	if err != nil {
		return fail(err, "")
	}
	if relativeApp {
		appURI, err = resolveRelativeAppURI(appRaw, disco)
		if err != nil {
			return fail(err, "")
		}
		if _, err = joinShareRelativePath(appURI, rel); err != nil {
			return fail(err, "")
		}
	}

	tokenURL, err := resolveTokenEndpoint(disco)
	if err != nil {
		return fail(err, "")
	}
	accessToken, _, err := client.ExchangeToken(
		ctx,
		tokenURL,
		secret,
		h.receiverDomain,
	)
	if err != nil {
		return fail(err, accessToken)
	}
	if strings.TrimSpace(accessToken) == "" {
		return fail(errtypes.InternalError("token exchange returned an empty access token"), "")
	}

	launched, err := joinShareRelativePath(appURI, rel)
	if err != nil {
		return fail(err, accessToken)
	}
	return openInAppResponse{
		AppURL:      launched,
		AccessToken: accessToken,
	}, nil
}

func (h *appsHandler) receivedWebapp(
	ctx context.Context,
	id *ocmpb.ShareId,
) (*ocmpb.ReceivedShare, *ocmpb.WebappProtocol, error) {
	gatewayClient, err := service.Gateway(ctx)
	if err != nil {
		return nil, nil, err
	}
	if gatewayClient == nil {
		return nil, nil, errtypes.InternalError("gateway client is not available")
	}

	res, err := gatewayClient.GetReceivedOCMShare(ctx, &ocmpb.GetReceivedOCMShareRequest{
		Ref: &ocmpb.ShareReference{
			Spec: &ocmpb.ShareReference_Id{
				Id: id,
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if res == nil || res.Status == nil {
		return nil, nil, errtypes.InternalError("missing share response")
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		if res.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND {
			return nil, nil, errtypes.NotFound(res.Status.Message)
		}
		return nil, nil, errtypes.InternalError(res.Status.Message)
	}
	if res.Share == nil {
		return nil, nil, errtypes.NotFound("missing share")
	}

	webapp, err := requireWebappProtocol(res.Share.Protocols)
	if err != nil {
		return nil, nil, err
	}
	return res.Share, webapp, nil
}

func requireWebappProtocol(protocols []*ocmpb.Protocol) (*ocmpb.WebappProtocol, error) {
	for _, p := range protocols {
		if p == nil {
			continue
		}
		opts, ok := p.Term.(*ocmpb.Protocol_WebappOptions)
		if !ok {
			continue
		}
		if opts == nil || opts.WebappOptions == nil {
			return nil, errtypes.BadRequest("webapp protocol missing options")
		}
		webapp := opts.WebappOptions
		if strings.TrimSpace(webapp.Uri) == "" {
			return nil, errtypes.BadRequest("webapp protocol missing uri")
		}
		if strings.TrimSpace(webapp.SharedSecret) == "" {
			return nil, errtypes.BadRequest("webapp protocol missing shared secret")
		}
		if !slices.Contains(webapp.Requirements, "must-exchange-token") {
			return nil, errtypes.BadRequest("webapp protocol does not require token exchange")
		}
		return webapp, nil
	}
	return nil, errtypes.BadRequest("share does not contain webapp protocol")
}

// senderDiscoveryOrigin prefers an absolute WebDAV origin and falls back to an
// absolute webapp origin. Path-relative values are skipped. A scheme, host,
// userinfo, or unsupported scheme is an error and is not replaced.
func senderDiscoveryOrigin(protocols []*ocmpb.Protocol) (string, error) {
	if origin, err := firstAbsoluteOrigin(protocolURIs(protocols, "webdav")); err != nil || origin != "" {
		return origin, err
	}
	if origin, err := firstAbsoluteOrigin(protocolURIs(protocols, "webapp")); err != nil || origin != "" {
		return origin, err
	}
	return "", errtypes.NotFound("share has no absolute sender origin")
}

func protocolURIs(protocols []*ocmpb.Protocol, kind string) []string {
	uris := []string{}
	for _, p := range protocols {
		if p == nil {
			continue
		}
		switch kind {
		case "webdav":
			opts, ok := p.Term.(*ocmpb.Protocol_WebdavOptions)
			if !ok || opts == nil || opts.WebdavOptions == nil {
				continue
			}
			uris = append(uris, opts.WebdavOptions.Uri)
		case "webapp":
			opts, ok := p.Term.(*ocmpb.Protocol_WebappOptions)
			if !ok || opts == nil || opts.WebappOptions == nil {
				continue
			}
			uris = append(uris, opts.WebappOptions.Uri)
		}
	}
	return uris
}

func firstAbsoluteOrigin(raws []string) (string, error) {
	for _, raw := range raws {
		origin, ok, err := absoluteOrigin(raw)
		if err != nil {
			return "", err
		}
		if ok {
			return origin, nil
		}
	}
	return "", nil
}

func absoluteOrigin(raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false, errtypes.BadRequest("malformed remote URL")
	}
	if pathRelativeURL(parsed) {
		return "", false, nil
	}
	if err := validateLaunchURL(parsed); err != nil {
		return "", false, err
	}
	return parsed.Scheme + "://" + parsed.Host, true, nil
}

func resolveAbsoluteAppURI(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	if err := validateLaunchURL(parsed); err != nil {
		return "", err
	}
	return raw, nil
}

func resolveRelativeAppURI(raw string, disco *wellknown.OcmDiscoveryData) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	if !pathRelativeURL(parsed) {
		return resolveAbsoluteAppURI(raw)
	}
	base, err := absoluteDiscoveryBase(disco)
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(parsed)
	if err := validateLaunchURL(resolved); err != nil {
		return "", err
	}
	return resolved.String(), nil
}

func resolveTokenEndpoint(disco *wellknown.OcmDiscoveryData) (string, error) {
	if disco == nil || strings.TrimSpace(disco.TokenEndPoint) == "" {
		return "", errtypes.NotFound("sender discovery has no tokenEndPoint")
	}
	parsed, err := url.Parse(strings.TrimSpace(disco.TokenEndPoint))
	if err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	if pathRelativeURL(parsed) {
		base, baseErr := absoluteDiscoveryBase(disco)
		if baseErr != nil {
			return "", baseErr
		}
		parsed = base.ResolveReference(parsed)
	}
	if err := validateLaunchURL(parsed); err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func absoluteDiscoveryBase(disco *wellknown.OcmDiscoveryData) (*url.URL, error) {
	if disco == nil {
		return nil, errtypes.BadRequest("discovery response has no absolute base for a relative endpoint")
	}
	parsed, err := url.Parse(strings.TrimSpace(disco.Endpoint))
	if err != nil || parsed == nil || !parsed.IsAbs() || parsed.Host == "" {
		return nil, errtypes.BadRequest("discovery response has no absolute base for a relative endpoint")
	}
	if err := validateLaunchURL(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// pathRelativeURL is a reference with no scheme and no authority.
// Network-path references and scheme-bearing values are not path-relative.
func pathRelativeURL(u *url.URL) bool {
	return u != nil && u.Scheme == "" && u.Host == "" && u.User == nil
}

func validateLaunchURL(u *url.URL) error {
	if u == nil {
		return errtypes.BadRequest("malformed remote URL")
	}
	if u.User != nil {
		return errtypes.BadRequest("remote URL must not include userinfo")
	}
	if u.Scheme == "" || u.Host == "" || !u.IsAbs() {
		return errtypes.BadRequest("remote URL must be absolute")
	}
	if u.Scheme != "https" {
		return errtypes.BadRequest("remote URL must use https")
	}
	malformedHost := u.Host == "https:" || u.Host == "http:" || strings.Contains(u.Host, "://")
	malformedPath := strings.HasPrefix(u.Path, "//http://") || strings.HasPrefix(u.Path, "//https://")
	if malformedHost || malformedPath {
		return errtypes.BadRequest("malformed remote URL")
	}
	return nil
}

func validateShareFilePath(filePath string) error {
	unescaped, err := fullyUnescape(filePath)
	if err != nil {
		return errtypes.BadRequest("malformed file path")
	}
	if strings.Contains(unescaped, `\`) || strings.Contains(unescaped, "://") {
		return errtypes.BadRequest("invalid file path")
	}
	for _, seg := range strings.Split(unescaped, "/") {
		if seg == ".." {
			return errtypes.BadRequest("file path escapes the share")
		}
	}
	return nil
}

// joinShareRelativePath appends a share-relative file path. An empty relative
// path keeps the bare opener unchanged. Share identity is not added here.
func joinShareRelativePath(appURI, rel string) (string, error) {
	if rel == "" {
		return appURI, nil
	}
	segments, err := relativePathSegments(rel)
	if err != nil {
		return "", err
	}
	escaped := make([]string, len(segments))
	for i, seg := range segments {
		escaped[i] = url.PathEscape(seg)
	}
	joined, err := url.JoinPath(appURI, escaped...)
	if err != nil {
		return "", errtypes.BadRequest("invalid share-relative path")
	}
	parsed, err := url.Parse(joined)
	if err != nil {
		return "", errtypes.BadRequest("invalid share-relative path")
	}
	if err := validateLaunchURL(parsed); err != nil {
		return "", err
	}
	suffix := "/" + strings.Join(escaped, "/")
	if !strings.HasSuffix(parsed.EscapedPath(), suffix) {
		return "", errtypes.BadRequest("invalid share-relative path")
	}
	return joined, nil
}

func relativePathSegments(rel string) ([]string, error) {
	unescaped, err := fullyUnescape(rel)
	if err != nil {
		return nil, errtypes.BadRequest("malformed share-relative path")
	}
	absolutePath := strings.HasPrefix(unescaped, "/")
	hasBackslash := strings.Contains(unescaped, `\`)
	hasScheme := strings.Contains(unescaped, "://")
	if unescaped == "" || absolutePath || hasBackslash || hasScheme {
		return nil, errtypes.BadRequest("invalid share-relative path")
	}
	parsed, err := url.Parse(unescaped)
	if err != nil || parsed.IsAbs() {
		return nil, errtypes.BadRequest("invalid share-relative path")
	}
	parts := strings.Split(unescaped, "/")
	segments := make([]string, 0, len(parts))
	for _, seg := range parts {
		if seg == "" || seg == "." || seg == ".." {
			return nil, errtypes.BadRequest("invalid share-relative path")
		}
		segments = append(segments, seg)
	}
	return segments, nil
}

func fullyUnescape(raw string) (string, error) {
	current := raw
	for range 8 {
		next, err := url.PathUnescape(current)
		if err != nil {
			return "", err
		}
		if next == current {
			return current, nil
		}
		current = next
	}
	return "", errors.New("too many escape layers")
}

func redactLaunchError(err error, secret, token string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if containsSensitive(msg, secret) || containsSensitive(msg, token) {
		return errors.New("launch failed")
	}
	return err
}

func containsSensitive(msg, secret string) bool {
	return secret != "" && strings.Contains(msg, secret)
}

func writeLaunchError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		notFound           errtypes.NotFound
		badRequest         errtypes.BadRequest
		invalidCredentials errtypes.InvalidCredentials
		permissionDenied   errtypes.PermissionDenied
	)
	switch {
	case errors.As(err, &notFound):
		reqres.WriteError(w, r, reqres.APIErrorNotFound, notFound.Error(), err)
	case errors.As(err, &badRequest):
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, badRequest.Error(), err)
	case errors.As(err, &invalidCredentials):
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, invalidCredentials.Error(), err)
	case errors.As(err, &permissionDenied):
		reqres.WriteError(w, r, reqres.APIErrorUntrustedService, permissionDenied.Error(), err)
	default:
		msg := "launch failed"
		if err != nil && err.Error() != "" {
			msg = err.Error()
		}
		reqres.WriteError(w, r, reqres.APIErrorServerError, msg, err)
	}
}

// writeLaunchJSON marshals the payload before committing a successful status.
func writeLaunchJSON(w http.ResponseWriter, r *http.Request, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error marshalling JSON response", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(encoded); err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing launch response")
	}
}
