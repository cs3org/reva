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

package ocmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/pkg/errors"
)

// ErrTokenInvalid is the error returned by the invite-accepted
// endpoint when the token is not valid or not existing.
var ErrTokenInvalid = errors.New("the invitation token is invalid or not found")

// ErrServiceNotTrusted is the error returned by the invite-accepted
// endpoint when the service is not trusted to accept invitations.
var ErrServiceNotTrusted = errors.New("service is not trusted to accept invitations")

// ErrUserAlreadyAccepted is the error returned by the invite-accepted
// endpoint when a token was already used by a user in the remote cloud.
var ErrUserAlreadyAccepted = errors.New("invitation already accepted")

// ErrInvalidParameters is the error returned by the shares endpoint
// when the request does not contain required properties.
var ErrInvalidParameters = errors.New("invalid parameters")

// maxOCMResponseBytes caps attacker-influenced OCM HTTP response bodies.
const maxOCMResponseBytes = 1 << 20 // 1 MiB

var errOCMResponseTooLarge = errors.New("ocm response body exceeds size limit")

// OCMClient is the client for an OCM provider.
type OCMClient struct {
	client        *http.Client
	responseLimit int64
}

func resolveOCMResponseLimit(limit int64) int64 {
	if limit <= 0 {
		return maxOCMResponseBytes
	}
	return limit
}

// newOCMTransport builds the outbound OCM transport. It honors HTTP_PROXY,
// HTTPS_PROXY, and NO_PROXY. minVersion is the TLS floor from the service
// knob; 0 selects that knob's default. Values below the enforced floor are
// rejected.
func newOCMTransport(insecure bool, minVersion uint16) *http.Transport {
	var tr *http.Transport
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = dt.Clone()
	} else {
		tr = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}
	}
	tr.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: insecure,
		MinVersion:         resolveTLSMinVersion(minVersion),
	}
	return tr
}

// NewClient returns an OCM client for trusted, operator-configured endpoints.
// It does not apply public-only dial or redirect policy. responseLimit caps
// OCM response bodies; values <= 0 select the package default.
func NewClient(timeout time.Duration, insecure bool, responseLimit int64) *OCMClient {
	return &OCMClient{
		client: &http.Client{
			Transport: newOCMTransport(insecure, 0),
			Timeout:   timeout,
		},
		responseLimit: resolveOCMResponseLimit(responseLimit),
	}
}

// UntrustedHTTPTransport returns a RoundTripper with the same untrusted-client
// hardening as NewPublicOnlyClient. Use it with gowebdav.Client.SetTransport
// when the URL is peer-supplied: gowebdav cannot install CheckRedirect, so
// the RoundTripper walks req.Response and enforces the redirect cap itself.
// Owned http.Client callers should also set CheckRedirect to sec.CheckRedirect.
//
// Proxy is forced to nil. An environment proxy would make the dialer inspect
// the proxy address rather than the peer target, so the public-only Control
// guard could not enforce the target-IP policy.
//
// timeout is net.Dialer.Timeout: it bounds the TCP dial (DNS resolution +
// TCP connect). Control is the post-resolution, pre-dial policy gate that
// runs within that TCP dial. TLS is bounded separately by the transport's
// TLS handshake timeout. Request-level deadlines need http.Client.Timeout
// or gowebdav SetTimeout. minVersion 0 selects the TLS-knob default. sec
// must already be compiled.
func UntrustedHTTPTransport(
	timeout time.Duration,
	insecure bool,
	sec UntrustedClientSecurity,
	minVersion uint16,
) http.RoundTripper {
	tr := newOCMTransport(insecure, minVersion)
	// An env proxy would make Control inspect the proxy, not the peer target.
	tr.Proxy = nil
	tr.DialContext = (&net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control:   sec.dialControl,
	}).DialContext
	return &publicOnlyTransport{Transport: tr, sec: sec}
}

// NewPublicOnlyClient returns an OCMClient for hosts named by an untrusted
// caller. Dial Control enforces sec's address policy on every hop, including
// redirects, so DNS rebinding cannot sneak a private target past the first
// lookup. The initial URL and each redirect must satisfy requireScheme.
// responseLimit caps OCM response bodies; values <= 0 select the package default.
func NewPublicOnlyClient(
	timeout time.Duration,
	insecure bool,
	sec UntrustedClientSecurity,
	responseLimit int64,
	minVersion uint16,
) *OCMClient {
	return &OCMClient{
		client: &http.Client{
			Transport: UntrustedHTTPTransport(
				timeout,
				insecure,
				sec,
				minVersion,
			),
			Timeout:       timeout,
			CheckRedirect: sec.CheckRedirect,
		},
		responseLimit: resolveOCMResponseLimit(responseLimit),
	}
}

// HTTPTransport returns the live outbound RoundTripper.
func (c *OCMClient) HTTPTransport() http.RoundTripper {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Transport
}

// RequestTimeout returns the outbound HTTP client timeout.
func (c *OCMClient) RequestTimeout() time.Duration {
	if c == nil || c.client == nil {
		return 0
	}
	return c.client.Timeout
}

// publicOnlyTransport enforces scheme and redirect-cap before the inner
// dialer. SetTransport-only clients (gowebdav) cannot install CheckRedirect,
// so RoundTrip walks req.Response as the redirect backstop.
type publicOnlyTransport struct {
	*http.Transport
	sec UntrustedClientSecurity
}

func (t *publicOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var u *url.URL
	if req != nil {
		u = req.URL
	}
	if err := t.sec.requireScheme(u); err != nil {
		return nil, err
	}
	if redirectHopCount(req) > t.sec.maxRedirects() {
		return nil, t.sec.tooManyRedirectsError()
	}
	return t.Transport.RoundTrip(req)
}

func redirectHopCount(req *http.Request) int {
	n := 0
	for req != nil && req.Response != nil {
		n++
		next := req.Response.Request
		if next == req {
			break
		}
		req = next
	}
	return n
}

// Discover returns a number of properties used to discover the capabilities offered by a remote cloud storage.
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1.well-known~1ocm/get
func (c *OCMClient) Discover(ctx context.Context, endpoint string) (*wellknown.OcmDiscoveryData, error) {
	log := appctx.GetLogger(ctx)

	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		if strings.HasPrefix(endpoint, "localhost") || strings.HasPrefix(endpoint, "127.0.0.1") {
			// for testing purposes we allow no TLS on localhost
			endpoint = "http://" + endpoint
		} else {
			endpoint = "https://" + endpoint
		}
	}
	remoteurl, _ := url.JoinPath(endpoint, "/.well-known/ocm")
	body, err := c.httpget(ctx, remoteurl)
	if stderrors.Is(err, errOCMResponseTooLarge) {
		return nil, err
	}
	if err != nil || len(body) == 0 {
		log.Debug().Err(err).Any("remote", remoteurl).Str("response", string(body)).Msg("invalid or empty response, falling back to legacy discovery")
		remoteurl, _ := url.JoinPath(endpoint, "/ocm-provider") // legacy discovery endpoint
		body, err = c.httpget(ctx, remoteurl)
		if stderrors.Is(err, errOCMResponseTooLarge) {
			return nil, err
		}
		if err != nil || len(body) == 0 {
			log.Warn().Err(err).Any("remote", remoteurl).Str("response", string(body)).Msg("invalid or empty response")
			return nil, errtypes.InternalError("Invalid response on OCM discovery")
		}
	}

	var disco wellknown.OcmDiscoveryData
	err = json.Unmarshal(body, &disco)
	if err != nil {
		log.Warn().Err(err).Any("remote", remoteurl).Str("response", string(body)).Msg("malformed response")
		return nil, errtypes.InternalError("Invalid payload on OCM discovery")
	}

	log.Debug().Any("remote", remoteurl).Any("response", disco).Msg("discovery response")
	return &disco, nil
}

func (c *OCMClient) httpget(ctx context.Context, url string) ([]byte, error) {
	log := appctx.GetLogger(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error creating OCM discovery request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error doing OCM discovery request")
	}
	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			log.Warn().Err(err).Msg("error closing response body")
		}
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, errtypes.InternalError("Remote does not offer a valid OCM discovery endpoint")
	}

	body, err := c.readOCMBody(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "malformed remote OCM discovery")
	}
	return body, nil
}

func (c *OCMClient) decodeOCMJSON(r io.Reader, v any) error {
	limited := io.LimitReader(r, c.responseLimit+1)
	err := json.NewDecoder(limited).Decode(v)
	// Drain leftover limited bytes. Decode can stop after a valid JSON value
	// while the body still exceeds the cap.
	_, drainErr := io.Copy(io.Discard, limited)
	if limited.(*io.LimitedReader).N == 0 {
		return errOCMResponseTooLarge
	}
	if err != nil {
		return err
	}
	return drainErr
}

func (c *OCMClient) readOCMBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, c.responseLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > c.responseLimit {
		return nil, errOCMResponseTooLarge
	}
	return body, nil
}

// NewShare sends a new OCM share to the remote system.
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1shares/post
func (c *OCMClient) NewShare(ctx context.Context, endpoint string, r *NewShareRequest) (*NewShareResponse, error) {
	url, err := url.JoinPath(endpoint, "shares")
	if err != nil {
		return nil, err
	}
	body, err := r.toJSON()
	if err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	log.Info().Str("url", url).Str("payload", string(body)).Msg("Sending OCM share")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "error creating request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request")
	}
	defer resp.Body.Close()

	sresp, err := c.parseNewShareResponse(resp)
	if sresp != nil {
		log.Info().Any("status", resp.Status).Any("shareResponse", sresp).Msg("remote OCM server responded")
	} else {
		log.Info().Err(err).Str("status", resp.Status).Msg("error in remote OCM server response")
	}
	return sresp, err
}

func (c *OCMClient) parseNewShareResponse(r *http.Response) (*NewShareResponse, error) {
	switch r.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var res NewShareResponse
		if err := c.decodeOCMJSON(r.Body, &res); err != nil {
			return nil, errors.Wrap(err, "error decoding response body")
		}
		return &res, nil
	case http.StatusBadRequest:
		return nil, ErrInvalidParameters
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrServiceNotTrusted
	}

	body, err := c.readOCMBody(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding response body")
	}
	return nil, errtypes.InternalError(string(body))
}

// InviteAccepted informs the remote end that the invitation was accepted
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1invite-accepted/post
func (c *OCMClient) InviteAccepted(ctx context.Context, endpoint string, r *InviteAcceptedRequest) (*RemoteUser, error) {
	url, err := url.JoinPath(endpoint, "invite-accepted")
	if err != nil {
		return nil, err
	}
	body, err := r.toJSON()
	if err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	log.Info().Str("url", url).Str("payload", string(body)).Msg("Sending OCM invite-accepted")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "error creating request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request")
	}
	defer resp.Body.Close()

	u, err := c.parseInviteAcceptedResponse(resp)
	if u != nil {
		log.Info().Any("status", resp.Status).Any("remoteUser", u).Msg("remote OCM server responded")
	} else {
		log.Info().Err(err).Str("status", resp.Status).Msg("error in remote OCM server response")
	}
	return u, err
}

func (c *OCMClient) parseInviteAcceptedResponse(r *http.Response) (*RemoteUser, error) {
	switch r.StatusCode {
	case http.StatusOK:
		var u RemoteUser
		if err := c.decodeOCMJSON(r.Body, &u); err != nil {
			return nil, errors.Wrap(err, "error decoding response body")
		}
		return &u, nil
	case http.StatusBadRequest:
		return nil, ErrTokenInvalid
	case http.StatusConflict:
		return nil, ErrUserAlreadyAccepted
	case http.StatusForbidden:
		return nil, ErrServiceNotTrusted
	}

	body, err := c.readOCMBody(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding response body")
	}
	return nil, errtypes.InternalError(string(body))
}

// NewNotification sends a notification to the remote end. Not implemented for now.
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1notifications/post
func (c *OCMClient) NewNotification(ctx context.Context, endpoint string, r *InviteAcceptedRequest) (*RemoteUser, error) {
	return nil, errtypes.NotSupported("not implemented")
}

// ExchangeToken performs an OAuth2 authorization_code exchange against the
// sender's token endpoint, returning the short-lived access token and its TTL.
func (c *OCMClient) ExchangeToken(ctx context.Context, tokenEndpoint, code, clientID string) (string, int64, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	if clientID != "" {
		values.Set("client_id", clientID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return "", 0, errors.Wrap(err, "error creating token exchange request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", 0, errors.Wrap(err, "error sending token exchange request")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest:
		var errBody struct {
			Error string `json:"error"`
		}
		if err := c.decodeOCMJSON(resp.Body, &errBody); err != nil {
			if stderrors.Is(err, errOCMResponseTooLarge) {
				return "", 0, errors.Wrap(err, "error decoding token exchange response")
			}
			return "", 0, errtypes.InternalError("token exchange returned HTTP 400 (sender contract error)")
		}
		if errBody.Error == "invalid_grant" {
			return "", 0, errtypes.InvalidCredentials("token exchange: invalid_grant")
		}
		return "", 0, errtypes.InternalError("token exchange returned HTTP 400 (sender contract error)")
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", 0, errtypes.PermissionDenied("token exchange was rejected by the sender")
	default:
		return "", 0, errtypes.InternalError(fmt.Sprintf("token exchange returned HTTP %d", resp.StatusCode))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := c.decodeOCMJSON(resp.Body, &result); err != nil {
		return "", 0, errors.Wrap(err, "error decoding token exchange response")
	}
	if result.AccessToken == "" {
		return "", 0, errtypes.InternalError("token exchange response missing access_token")
	}
	return result.AccessToken, result.ExpiresIn, nil
}

// GetDirectoryService fetches a directory service listing from the given URL per OCM spec Appendix C.
func (c *OCMClient) GetDirectoryService(ctx context.Context, directoryURL string) (*DirectoryService, error) {
	log := appctx.GetLogger(ctx)

	// TODO(@MahdiBaghbani): the discover() should be changed into a generic function that can be used to fetch any OCM endpoint. I'll do it in the security PR to minimize conflicts.
	body, err := c.httpget(ctx, directoryURL)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching directory service")
	}

	var dirService DirectoryService
	if err := json.Unmarshal(body, &dirService); err != nil {
		log.Warn().Err(err).Str("url", directoryURL).Str("response", string(body)).Msg("malformed directory service response")
		return nil, errors.Wrap(err, "invalid directory service payload")
	}

	if dirService.Federation == "" {
		return nil, errtypes.InternalError("directory service missing required 'federation' field")
	}
	// Servers can be empty array, that's valid

	log.Debug().Str("url", directoryURL).Str("federation", dirService.Federation).Int("servers", len(dirService.Servers)).Msg("fetched directory service")
	return &dirService, nil
}
