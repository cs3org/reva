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
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/pkg/errors"
)

// DefaultResponseLimit is the maximum OCM control-plane response body size.
const DefaultResponseLimit int64 = 1 << 20

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

// ErrResponseTooLarge is returned when an OCM control-plane response
// body exceeds DefaultResponseLimit.
var ErrResponseTooLarge = errors.New("ocm response body exceeds size limit")

func readOCMBody(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrResponseTooLarge
	}
	return data, nil
}

func decodeOCMJSON(body io.Reader, limit int64, dst any) error {
	data, err := readOCMBody(body, limit)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// OCMClient is the client for an OCM provider.
type OCMClient struct {
	client *http.Client
}

// NewClient returns a new trusted OCMClient that honors HTTP proxy environment variables.
func NewClient(timeout time.Duration, insecure bool) *OCMClient {
	return NewClientWithConfig(client.TransportConfig{
		Timeout:  timeout,
		Insecure: insecure,
	})
}

// NewPublicOnlyClient returns an OCMClient that only dials public addresses, for
// discovery of hosts named by an untrusted caller. The check is in the dialer so
// it also covers redirects and DNS rebinding.
func NewPublicOnlyClient(timeout time.Duration, insecure bool) *OCMClient {
	return NewPublicOnlyClientWithConfig(client.TransportConfig{
		Timeout:  timeout,
		Insecure: insecure,
	})
}

// NewClientWithConfig returns a trusted OCMClient using cfg.
func NewClientWithConfig(cfg client.TransportConfig) *OCMClient {
	return &OCMClient{
		client: client.NewTrustedHTTPClient(cfg),
	}
}

// NewPublicOnlyClientWithConfig returns a public-only OCMClient using cfg.
func NewPublicOnlyClientWithConfig(cfg client.TransportConfig) *OCMClient {
	return &OCMClient{
		client: client.NewPublicOnlyHTTPClient(cfg),
	}
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
	body, err := c.httpget(ctx, remoteurl, DefaultResponseLimit)
	if err != nil || len(body) == 0 {
		log.Debug().Err(err).Any("remote", remoteurl).Str("response", string(body)).Msg("invalid or empty response, falling back to legacy discovery")
		remoteurl, _ := url.JoinPath(endpoint, "/ocm-provider") // legacy discovery endpoint
		body, err = c.httpget(ctx, remoteurl, DefaultResponseLimit)
		if err != nil || len(body) == 0 {
			log.Warn().Err(err).Any("remote", remoteurl).Str("response", string(body)).Msg("invalid or empty response")
			if stderrors.Is(err, ErrResponseTooLarge) || stderrors.Is(err, client.ErrPolicyViolation) {
				return nil, err
			}
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

func (c *OCMClient) httpget(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error creating OCM discovery request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error doing OCM discovery request")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if _, err := readOCMBody(resp.Body, limit); err != nil {
			return nil, err
		}
		return nil, errtypes.InternalError("Remote does not offer a valid OCM discovery endpoint")
	}

	body, err := readOCMBody(resp.Body, limit)
	if err != nil {
		if err == ErrResponseTooLarge {
			return nil, err
		}
		return nil, errors.Wrap(err, "malformed remote OCM discovery")
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
		err := decodeOCMJSON(r.Body, DefaultResponseLimit, &res)
		return &res, err
	case http.StatusBadRequest:
		return nil, ErrInvalidParameters
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrServiceNotTrusted
	}

	body, err := readOCMBody(r.Body, DefaultResponseLimit)
	if err != nil {
		if err == ErrResponseTooLarge {
			return nil, err
		}
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
		if err := decodeOCMJSON(r.Body, DefaultResponseLimit, &u); err != nil {
			if err == ErrResponseTooLarge {
				return nil, err
			}
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

	body, err := readOCMBody(r.Body, DefaultResponseLimit)
	if err != nil {
		if err == ErrResponseTooLarge {
			return nil, err
		}
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
		// success, decode below
	case http.StatusBadRequest:
		data, err := readOCMBody(resp.Body, DefaultResponseLimit)
		if err != nil {
			return "", 0, err
		}
		var errBody struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(data, &errBody); err == nil && errBody.Error == "invalid_grant" {
			return "", 0, errtypes.InvalidCredentials("token exchange: invalid_grant")
		}
		return "", 0, errtypes.InternalError("token exchange returned HTTP 400 (sender contract error)")
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", 0, errtypes.PermissionDenied("token exchange was rejected by the sender")
	default:
		if _, err := readOCMBody(resp.Body, DefaultResponseLimit); err != nil {
			return "", 0, err
		}
		return "", 0, errtypes.InternalError(fmt.Sprintf("token exchange returned HTTP %d", resp.StatusCode))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := decodeOCMJSON(resp.Body, DefaultResponseLimit, &result); err != nil {
		if err == ErrResponseTooLarge {
			return "", 0, err
		}
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
	body, err := c.httpget(ctx, directoryURL, DefaultResponseLimit)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching directory service")
	}

	var dirService DirectoryService
	if err := json.Unmarshal(body, &dirService); err != nil {
		log.Warn().Err(err).Str("url", directoryURL).Str("response", string(body)).Msg("malformed directory service response")
		return nil, errors.Wrap(err, "invalid directory service payload")
	}

	// Validate required fields
	if dirService.Federation == "" {
		return nil, errtypes.InternalError("directory service missing required 'federation' field")
	}
	// Servers can be empty array, that's valid

	log.Debug().Str("url", directoryURL).Str("federation", dirService.Federation).Int("servers", len(dirService.Servers)).Msg("fetched directory service")
	return &dirService, nil
}
