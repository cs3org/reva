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
	"strings"
	"time"
	"unicode"

	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

// launchClient is one client for discovery and token exchange; tests may install a factory.
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
	// Nil uses the published targets; an explicit empty set disables receipt.
	webappReceiveTargets *[]string
	// Nil uses the published MFA policy; tests may pin reject or off.
	mfaPolicy *string
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

func (h *appsHandler) shareInfo(p string) (*ocmpb.ShareId, string, error) {
	p = stripOCMMountPoint(p, h.ocmMountPoint)
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil, "", errtypes.BadRequest("invalid file identifier")
	}

	bang := strings.IndexByte(p, '!')
	slash := strings.IndexByte(p, '/')
	if bang >= 0 && (slash < 0 || bang < slash) {
		return parseResourceIDFileIdentifier(p)
	}
	return parsePathFileIdentifier(p)
}

func stripOCMMountPoint(p, mount string) string {
	if mount == "" {
		return p
	}
	if p == mount {
		return ""
	}
	if strings.HasPrefix(p, mount+"/") {
		return strings.TrimPrefix(p, mount)
	}
	return p
}

func parsePathFileIdentifier(p string) (*ocmpb.ShareId, string, error) {
	shareID, rel, _ := strings.Cut(p, "/")
	if invalidShareIdentifierComponent(shareID) {
		return nil, "", errtypes.BadRequest("invalid file identifier")
	}
	return &ocmpb.ShareId{OpaqueId: shareID}, rel, nil
}

func parseResourceIDFileIdentifier(p string) (*ocmpb.ShareId, string, error) {
	prefix, opaque, ok := strings.Cut(p, "!")
	if !ok || prefix == "" || opaque == "" {
		return nil, "", errtypes.BadRequest("invalid file identifier")
	}
	if invalidStoragePrefix(prefix) {
		return nil, "", errtypes.BadRequest("invalid file identifier")
	}
	if strings.Contains(prefix, "$") {
		storage, space, ok := strings.Cut(prefix, "$")
		if !ok || storage == "" || space == "" {
			return nil, "", errtypes.BadRequest("invalid file identifier")
		}
		if _, err := spaces.DecodeSpaceID(space); err != nil {
			return nil, "", errtypes.BadRequest("invalid file identifier")
		}
	}
	colon := strings.IndexByte(opaque, ':')
	if colon < 0 {
		return nil, "", errtypes.BadRequest("invalid file identifier")
	}
	shareID := opaque[:colon]
	rel := opaque[colon+1:]
	if invalidShareIdentifierComponent(shareID) {
		return nil, "", errtypes.BadRequest("invalid file identifier")
	}
	if strings.HasPrefix(rel, "//") {
		return nil, "", errtypes.BadRequest("invalid file identifier")
	}
	if strings.HasPrefix(rel, "/") {
		rel = rel[1:]
	}
	return &ocmpb.ShareId{OpaqueId: shareID}, rel, nil
}

func invalidStoragePrefix(prefix string) bool {
	if prefix == "" {
		return true
	}
	if strings.ContainsAny(prefix, "/\\") {
		return true
	}
	for _, r := range prefix {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func invalidShareIdentifierComponent(component string) bool {
	if component == "" || strings.TrimSpace(component) != component {
		return true
	}
	if component == "." || component == ".." {
		return true
	}
	return strings.ContainsAny(component, "/\\:!%")
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

	shareID, rel, err := h.shareInfo(filePath)
	if err != nil {
		writeLaunchError(w, r, err)
		return
	}
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

	receiverTargets := wellknown.ResolveLocalWebappReceiveTargets(h.webappReceiveTargets)
	admitMFA := wellknown.ResolveLocalMFAPolicy(h.mfaPolicy) == wellknown.MFAPolicyOff
	if err := ocmd.ValidateWebappLaunch(
		webapp.GetUri(),
		secret,
		webapp.GetRequirements(),
		webapp.GetTargets(),
		receiverTargets,
		admitMFA,
	); err != nil {
		if errors.Is(err, ocmd.ErrWebappMFAUnproven) {
			return fail(errtypes.PermissionDenied(ocmd.ErrWebappMFAUnproven.Error()), "")
		}
		if errors.Is(err, ocmd.ErrInvalidProtocolURI) {
			return fail(errtypes.BadRequest("malformed remote URL"), "")
		}
		return fail(errtypes.BadRequest(err.Error()), "")
	}

	if err := validateShareFilePath(rel); err != nil {
		return fail(err, "")
	}

	appURI, err := requireHTTPSAppURI(webapp.GetUri())
	if err != nil {
		return fail(err, "")
	}
	if strings.TrimSpace(h.receiverDomain) == "" {
		return fail(errtypes.BadRequest("provider domain is not configured"), "")
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

// senderDiscoveryOrigin prefers WebDAV; an invalid absolute WebDAV URL must not fall back to webapp.
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
	if strings.TrimSpace(raw) == "" {
		return "", false, nil
	}
	if strings.TrimSpace(raw) != raw {
		return "", false, errtypes.BadRequest("malformed remote URL")
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

// joinShareRelativePath leaves an empty relative path as the bare opener and inserts no share id.
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

// requireHTTPSAppURI accepts only https and returns the exact stored string.
func requireHTTPSAppURI(raw string) (string, error) {
	validated, err := ocmd.ValidateAbsoluteWebappURI(raw)
	if err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	parsed, err := url.Parse(validated)
	if err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	if err := validateLaunchURL(parsed); err != nil {
		return "", err
	}
	return validated, nil
}

// resolveTokenEndpoint applies https only for the launch hop.
func resolveTokenEndpoint(disco *wellknown.OcmDiscoveryData) (string, error) {
	tokenURL, err := ocmd.WebappTokenEndpoint(disco)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(tokenURL)
	if err != nil {
		return "", errtypes.BadRequest("malformed remote URL")
	}
	if err := validateLaunchURL(parsed); err != nil {
		return "", err
	}
	return tokenURL, nil
}

func pathRelativeURL(u *url.URL) bool {
	return u != nil && u.Scheme == "" && u.Host == "" && u.User == nil && u.Opaque == ""
}

func validateLaunchURL(u *url.URL) error {
	if u == nil {
		return errtypes.BadRequest("malformed remote URL")
	}
	if u.User != nil {
		return errtypes.BadRequest("remote URL must not include userinfo")
	}
	if u.Scheme == "" || u.Host == "" || u.Hostname() == "" || !u.IsAbs() {
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
	switch res.Status.Code {
	case rpcv1beta1.Code_CODE_OK:
	case rpcv1beta1.Code_CODE_NOT_FOUND:
		return nil, nil, errtypes.NotFound("received share not found")
	case rpcv1beta1.Code_CODE_PERMISSION_DENIED:
		return nil, nil, errtypes.PermissionDenied("received share access denied")
	case rpcv1beta1.Code_CODE_UNAUTHENTICATED:
		return nil, nil, errtypes.InvalidCredentials("received share unauthenticated")
	default:
		return nil, nil, errtypes.InternalError("received share lookup failed")
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
	found := []*ocmpb.WebappProtocol{}
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
		found = append(found, opts.WebappOptions)
	}
	switch len(found) {
	case 0:
		return nil, errtypes.BadRequest("share does not contain webapp protocol")
	case 1:
		return found[0], nil
	default:
		return nil, errtypes.BadRequest("duplicate webapp protocol")
	}
}

func redactLaunchError(err error, secret, token string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}

	var notFound errtypes.NotFound
	var badRequest errtypes.BadRequest
	var invalidCredentials errtypes.InvalidCredentials
	var permissionDenied errtypes.PermissionDenied
	var internal errtypes.InternalError
	switch {
	case errors.As(err, &notFound):
		return errtypes.NotFound(safeDetail(string(notFound), secret, token, "received share not found"))
	case errors.As(err, &badRequest):
		return errtypes.BadRequest(safeDetail(string(badRequest), secret, token, "invalid parameter"))
	case errors.As(err, &invalidCredentials):
		return errtypes.InvalidCredentials(safeDetail(string(invalidCredentials), secret, token, "unauthenticated"))
	case errors.As(err, &permissionDenied):
		return errtypes.PermissionDenied(safeDetail(string(permissionDenied), secret, token, "untrusted service"))
	case errors.As(err, &internal):
		return errtypes.InternalError(safeInternalDetail(string(internal), secret, token))
	default:
		return errtypes.InternalError("launch failed")
	}
}

// safeInternalDetails are fixed messages this receiver authors. Anything else,
// including a remote body or URL, becomes the generic launch failure.
var safeInternalDetails = map[string]struct{}{
	"missing share response":                        {},
	"gateway client is not available":               {},
	"launch client is not available":                {},
	"received share lookup failed":                  {},
	"token exchange returned an empty access token": {},
	"launch failed":                                 {},
}

func safeInternalDetail(detail, secret, token string) string {
	if _, ok := safeInternalDetails[detail]; !ok {
		return "launch failed"
	}
	return safeDetail(detail, secret, token, "launch failed")
}

func safeDetail(detail, secret, token, fallback string) string {
	if containsSensitive(detail, secret) || containsSensitive(detail, token) {
		return fallback
	}
	return detail
}

func containsSensitive(msg, secret string) bool {
	return secret != "" && strings.Contains(msg, secret)
}

func writeLaunchError(w http.ResponseWriter, r *http.Request, err error) {
	sanitized := redactLaunchError(err, "", "")
	var (
		notFound           errtypes.NotFound
		badRequest         errtypes.BadRequest
		invalidCredentials errtypes.InvalidCredentials
		permissionDenied   errtypes.PermissionDenied
		internal           errtypes.InternalError
	)
	switch {
	case errors.As(sanitized, &notFound):
		reqres.WriteError(w, r, reqres.APIErrorNotFound, notFound.Error(), sanitized)
	case errors.As(sanitized, &badRequest):
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, badRequest.Error(), sanitized)
	case errors.As(sanitized, &invalidCredentials):
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, invalidCredentials.Error(), sanitized)
	case errors.As(sanitized, &permissionDenied):
		reqres.WriteError(w, r, reqres.APIErrorUntrustedService, permissionDenied.Error(), sanitized)
	case errors.As(sanitized, &internal):
		reqres.WriteError(w, r, reqres.APIErrorServerError, internal.Error(), sanitized)
	default:
		reqres.WriteError(w, r, reqres.APIErrorServerError, "launch failed", sanitized)
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
