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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
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

// ErrURLValidationFailed is returned when URL validation fails.
var ErrURLValidationFailed = errors.New("URL validation failed")

// ErrResponseTooLarge is returned when response exceeds maximum allowed size.
var ErrResponseTooLarge = errors.New("response exceeds maximum allowed size")

// ClientSecurityConfig holds all security configuration options for the OCM client.
type ClientSecurityConfig struct {
	// Response limits
	MaxDiscoveryResponseBytes int64 `mapstructure:"max_discovery_response_bytes"`
	MaxShareResponseBytes     int64 `mapstructure:"max_share_response_bytes"`
	MaxErrorResponseBytes     int64 `mapstructure:"max_error_response_bytes"`
	MaxResponseHeaderBytes    int64 `mapstructure:"max_response_header_bytes"`

	// Timeouts
	ConnectionTimeout     time.Duration `mapstructure:"connection_timeout"`
	TLSHandshakeTimeout   time.Duration `mapstructure:"tls_handshake_timeout"`
	ResponseHeaderTimeout time.Duration `mapstructure:"response_header_timeout"`
	IdleConnTimeout       time.Duration `mapstructure:"idle_conn_timeout"`
	OverallTimeout        time.Duration `mapstructure:"overall_timeout"`

	// Redirect control
	MaxRedirects               int  `mapstructure:"max_redirects"`
	ValidateRedirectTargets    bool `mapstructure:"validate_redirect_targets"`
	AllowRedirectsToPrivateIPs bool `mapstructure:"allow_redirects_to_private_ips"`

	// SSRF prevention
	BlockPrivateIPs  bool     `mapstructure:"block_private_ips"`
	BlockLinkLocal   bool     `mapstructure:"block_link_local"`
	BlockLoopback    bool     `mapstructure:"block_loopback"`
	AllowedSchemes   []string `mapstructure:"allowed_schemes"`
	AllowedPorts     []int    `mapstructure:"allowed_ports"`
	ValidateIPAtDial bool     `mapstructure:"validate_ip_at_dial"`

	// TLS
	MinTLSVersion      uint16 `mapstructure:"min_tls_version"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
}

// ApplyDefaults sets default values for unset configuration options.
func (c *ClientSecurityConfig) ApplyDefaults() {
	// TODO(@MahdiBaghbani): default values are up for debate!
	if c.MaxDiscoveryResponseBytes == 0 {
		c.MaxDiscoveryResponseBytes = 25 * 1024 // 10 KB
	}
	if c.MaxShareResponseBytes == 0 {
		c.MaxShareResponseBytes = 50 * 1024 // 50 KB
	}
	if c.MaxErrorResponseBytes == 0 {
		c.MaxErrorResponseBytes = 50 * 1024 // 50 KB
	}
	if c.MaxResponseHeaderBytes == 0 {
		c.MaxResponseHeaderBytes = 4 * 1024 // 4 KB
	}
	if c.ConnectionTimeout == 0 {
		c.ConnectionTimeout = 5 * time.Second
	}
	if c.TLSHandshakeTimeout == 0 {
		c.TLSHandshakeTimeout = 5 * time.Second
	}
	if c.ResponseHeaderTimeout == 0 {
		c.ResponseHeaderTimeout = 10 * time.Second
	}
	if c.IdleConnTimeout == 0 {
		c.IdleConnTimeout = 30 * time.Second
	}
	if c.OverallTimeout == 0 {
		c.OverallTimeout = 30 * time.Second
	}
	if c.MaxRedirects == 0 {
		c.MaxRedirects = 3
	}
	c.ValidateRedirectTargets = true

	if len(c.AllowedSchemes) == 0 {
		// TODO(@MahdiBaghbani): @gplatcern we can allow http as well in dev env
		c.AllowedSchemes = []string{"https"}
	}
	if c.MinTLSVersion == 0 {
		c.MinTLSVersion = tls.VersionTLS12
	}
	c.ValidateIPAtDial = true

	// TODO(@MahdiBaghbani): @gplatcern we can allow these in dev env
	// BlockPrivateIPs, BlockLinkLocal, BlockLoopback default to true
	if !c.BlockPrivateIPs && !c.BlockLinkLocal && !c.BlockLoopback {
		// If none were set, apply secure defaults
		c.BlockPrivateIPs = true
		c.BlockLinkLocal = true
		c.BlockLoopback = true
	}
}

// OCMClient is the client for an OCM provider.
type OCMClient struct {
	client *http.Client
	cfg    *ClientSecurityConfig
}

// NewClientWithConfig creates a new OCMClient with security configuration.
func NewClientWithConfig(cfg *ClientSecurityConfig) *OCMClient {
	if cfg == nil {
		cfg = &ClientSecurityConfig{}
	}
	cfg.ApplyDefaults()

	baseDialer := &net.Dialer{
		Timeout:   cfg.ConnectionTimeout,
		KeepAlive: 30 * time.Second,
	}

	tr := &http.Transport{
		DialContext:            secureDialContext(cfg, baseDialer),
		TLSHandshakeTimeout:    cfg.TLSHandshakeTimeout,
		ResponseHeaderTimeout:  cfg.ResponseHeaderTimeout,
		IdleConnTimeout:        cfg.IdleConnTimeout,
		MaxResponseHeaderBytes: cfg.MaxResponseHeaderBytes,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.InsecureSkipVerify,
			MinVersion:         cfg.MinTLSVersion,
		},
	}

	client := &OCMClient{
		cfg: cfg,
	}
	client.client = &http.Client{
		Transport:     tr,
		Timeout:       cfg.OverallTimeout,
		CheckRedirect: redirectChecker(cfg, client),
	}

	return client
}

// NewClient returns a new OCMClient with backward compatible API.
// It creates a secure client with defaults, using the provided timeout and insecure settings.
func NewClient(timeout time.Duration, insecure bool) *OCMClient {
	cfg := &ClientSecurityConfig{
		OverallTimeout:     timeout,
		InsecureSkipVerify: insecure,
	}
	return NewClientWithConfig(cfg)
}

// isPrivateIP checks if an IP address is in a private/internal range based on configuration.
func isPrivateIP(ip net.IP, cfg *ClientSecurityConfig) bool {
	if ip == nil {
		return false
	}

	if cfg.BlockLoopback && ip.IsLoopback() {
		return true
	}

	if cfg.BlockLinkLocal && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return true
	}

	// RFC 1918 and IPv6 private
	if cfg.BlockPrivateIPs {
		privateBlocks := []string{
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"fc00::/7",
		}

		for _, block := range privateBlocks {
			_, subnet, err := net.ParseCIDR(block)
			if err != nil {
				continue
			}
			if subnet.Contains(ip) {
				return true
			}
		}
	}

	return false
}

// validateURL validates a URL against security configuration to prevent SSRF attacks.
func validateURL(u *url.URL, cfg *ClientSecurityConfig) error {
	if u == nil {
		return errors.Wrap(ErrURLValidationFailed, "nil URL")
	}

	schemeAllowed := false
	for _, allowedScheme := range cfg.AllowedSchemes {
		if strings.EqualFold(u.Scheme, allowedScheme) {
			schemeAllowed = true
			break
		}
	}
	if !schemeAllowed {
		return errors.Wrapf(ErrURLValidationFailed, "scheme %s not allowed", u.Scheme)
	}

	if len(cfg.AllowedPorts) > 0 {
		portStr := u.Port()
		if portStr != "" {
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return errors.Wrapf(ErrURLValidationFailed, "invalid port: %s", portStr)
			}
			portAllowed := false
			for _, allowedPort := range cfg.AllowedPorts {
				if port == allowedPort {
					portAllowed = true
					break
				}
			}
			if !portAllowed {
				return errors.Wrapf(ErrURLValidationFailed, "port %d not allowed", port)
			}
		}
	}

	host := u.Hostname()
	if host == "" {
		return errors.Wrap(ErrURLValidationFailed, "empty hostname")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return errors.Wrapf(ErrURLValidationFailed, "DNS resolution failed: %v", err)
	}

	for _, ip := range ips {
		if isPrivateIP(ip, cfg) {
			return errors.Wrapf(ErrURLValidationFailed, "private/internal IP address blocked: %s", ip)
		}
	}

	return nil
}

// secureDialContext creates a dialer that validates IPs at connection time to prevent DNS rebinding.
func secureDialContext(cfg *ClientSecurityConfig, baseDialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if cfg.ValidateIPAtDial {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, errors.Wrap(err, "failed to split host:port")
			}

			ip := net.ParseIP(host)
			if ip != nil {
				if isPrivateIP(ip, cfg) {
					return nil, errors.Errorf("private IP blocked at dial time: %s", ip)
				}
			} else {
				// Resolve hostname and validate all IPs
				ips, err := net.LookupIP(host)
				if err != nil {
					return nil, errors.Wrapf(err, "DNS resolution failed at dial time: %s", host)
				}
				for _, resolvedIP := range ips {
					if isPrivateIP(resolvedIP, cfg) {
						return nil, errors.Errorf("private IP blocked at dial time: %s (resolved from %s)", resolvedIP, host)
					}
				}
			}
		}

		return baseDialer.DialContext(ctx, network, addr)
	}
}

// redirectChecker creates a CheckRedirect function that validates redirect targets.
func redirectChecker(cfg *ClientSecurityConfig, client *OCMClient) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		// Enforce redirect count limit
		if len(via) >= cfg.MaxRedirects {
			return errors.Errorf("stopped after %d redirects", cfg.MaxRedirects)
		}

		// Validate redirect target if enabled
		if cfg.ValidateRedirectTargets {
			// Create a temporary config for redirect validation
			// Allow private IPs if AllowRedirectsToPrivateIPs is true
			redirectCfg := *cfg
			if cfg.AllowRedirectsToPrivateIPs {
				redirectCfg.BlockPrivateIPs = false
				redirectCfg.BlockLinkLocal = false
				redirectCfg.BlockLoopback = false
			}

			if err := validateURL(req.URL, &redirectCfg); err != nil {
				return errors.Wrapf(err, "redirect blocked: %s", req.URL)
			}
		}

		return nil
	}
}

// Discover returns a number of properties used to discover the capabilities offered by a remote cloud storage.
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1ocm-provider/get
func (c *OCMClient) Discover(ctx context.Context, endpoint string) (*wellknown.OcmDiscoveryData, error) {
	log := appctx.GetLogger(ctx)

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Wrapf(ErrURLValidationFailed, "invalid endpoint URL: %v", err)
	}
	if err := validateURL(endpointURL, c.cfg); err != nil {
		return nil, errors.Wrapf(err, "endpoint URL validation failed")
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.OverallTimeout)
	defer cancel()

	remoteurl, _ := url.JoinPath(endpoint, "/.well-known/ocm")
	body, err := c.fetchGETEndpoint(ctx, remoteurl, c.cfg.MaxDiscoveryResponseBytes, "discovery endpoint")
	if err != nil || len(body) == 0 {
		log.Debug().Err(err).Str("sender", remoteurl).Str("response", string(body)).Msg("invalid or empty response, falling back to legacy discovery")
		remoteurl, _ := url.JoinPath(endpoint, "/ocm-provider") // legacy discovery endpoint

		body, err = c.fetchGETEndpoint(ctx, remoteurl, c.cfg.MaxDiscoveryResponseBytes, "legacy discovery endpoint")
		if err != nil || len(body) == 0 {
			log.Warn().Err(err).Str("sender", remoteurl).Str("response", string(body)).Msg("invalid or empty response")
			return nil, errtypes.InternalError("Invalid response on OCM discovery")
		}
	}

	var disco wellknown.OcmDiscoveryData
	err = json.Unmarshal(body, &disco)
	if err != nil {
		log.Warn().Err(err).Str("sender", remoteurl).Str("response", string(body)).Msg("malformed response")
		return nil, errtypes.InternalError("Invalid payload on OCM discovery")
	}

	log.Debug().Str("sender", remoteurl).Any("response", disco).Msg("discovery response")
	return &disco, nil
}

// fetchGETEndpoint performs a GET request to an OCM endpoint with configurable response size limits.
func (c *OCMClient) fetchGETEndpoint(ctx context.Context, urlStr string, maxResponseBytes int64, errorContext string) ([]byte, error) {
	log := appctx.GetLogger(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating OCM GET request for %s", errorContext)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "error performing OCM GET request for %s", errorContext)
	}
	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			log.Warn().Err(err).Msg("error closing response body")
		}
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Warn().Str("url", urlStr).Int("status", resp.StatusCode).Str("context", errorContext).Msg("OCM endpoint returned non-OK status")
		return nil, errtypes.InternalError(fmt.Sprintf("Remote OCM endpoint (%s) returned status %d", errorContext, resp.StatusCode))
	}

	limitedBody := io.LimitReader(resp.Body, maxResponseBytes+1)
	body, err := io.ReadAll(limitedBody)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading response body for %s", errorContext)
	}
	// TODO(@MahdiBaghbani): @glpatcern so this would be only 1 byte bigger, should we tolerate it?
	if int64(len(body)) > maxResponseBytes {
		return nil, ErrResponseTooLarge
	}

	return body, nil
}

// NewShare sends a new OCM share to the remote system.
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1shares/post
func (c *OCMClient) NewShare(ctx context.Context, endpoint string, r *NewShareRequest) (*NewShareResponse, error) {
	log := appctx.GetLogger(ctx)

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Wrapf(ErrURLValidationFailed, "invalid endpoint URL: %v", err)
	}
	if err := validateURL(endpointURL, c.cfg); err != nil {
		return nil, errors.Wrapf(err, "endpoint URL validation failed")
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.OverallTimeout)
	defer cancel()

	urlStr, err := url.JoinPath(endpoint, "shares")
	if err != nil {
		return nil, err
	}
	body, err := r.toJSON()
	if err != nil {
		return nil, err
	}

	log.Info().Str("url", urlStr).Str("payload", string(body)).Msg("Sending OCM share")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "error creating request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request")
	}
	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			log.Warn().Err(err).Msg("error closing response body")
		}
	}(resp.Body)

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
		// Apply response size limit for successful responses
		limitedBody := io.LimitReader(r.Body, c.cfg.MaxShareResponseBytes+1)
		data, err := io.ReadAll(limitedBody)
		if err != nil {
			return nil, errors.Wrap(err, "error reading response body")
		}
		if int64(len(data)) > c.cfg.MaxShareResponseBytes {
			return nil, ErrResponseTooLarge
		}

		var res NewShareResponse
		err = json.NewDecoder(limitedBody).Decode(&res)

		if err != nil {
			return nil, errors.Wrap(err, "error decoding response body")
		}
		return &res, nil
	case http.StatusBadRequest:
		return nil, ErrInvalidParameters
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrServiceNotTrusted
	}

	limitedBody := io.LimitReader(r.Body, c.cfg.MaxErrorResponseBytes+1)
	body, err := io.ReadAll(limitedBody)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding response body")
	}
	// TODO(@MahdiBaghbani): @glpatcern so this would be only 1 byte bigger, should we tolerate it?
	if int64(len(body)) > c.cfg.MaxErrorResponseBytes {
		return nil, ErrResponseTooLarge
	}
	// TODO(@MahdiBaghbani): @glpatcern is string() necessary here?
	return nil, errtypes.InternalError(string(body))
}

// InviteAccepted informs the remote end that the invitation was accepted
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1invite-accepted/post
func (c *OCMClient) InviteAccepted(ctx context.Context, endpoint string, r *InviteAcceptedRequest) (*RemoteUser, error) {
	log := appctx.GetLogger(ctx)

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Wrapf(ErrURLValidationFailed, "invalid endpoint URL: %v", err)
	}
	if err := validateURL(endpointURL, c.cfg); err != nil {
		return nil, errors.Wrapf(err, "endpoint URL validation failed")
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.OverallTimeout)
	defer cancel()

	urlStr, err := url.JoinPath(endpoint, "invite-accepted")
	if err != nil {
		return nil, err
	}
	body, err := r.toJSON()
	if err != nil {
		return nil, err
	}

	log.Info().Str("url", urlStr).Str("payload", string(body)).Msg("Sending OCM invite-accepted")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "error creating request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request")
	}
	defer func(body io.ReadCloser) {
		err := body.Close()
		if err != nil {
			log.Warn().Err(err).Msg("error closing response body")
		}
	}(resp.Body)

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
		// Apply response size limit for successful responses
		limitedBody := io.LimitReader(r.Body, c.cfg.MaxShareResponseBytes+1)
		data, err := io.ReadAll(limitedBody)
		if err != nil {
			return nil, errors.Wrap(err, "error reading response body")
		}
		if int64(len(data)) > c.cfg.MaxShareResponseBytes {
			return nil, ErrResponseTooLarge
		}

		var u RemoteUser
		err = json.NewDecoder(limitedBody).Decode(&u)

		if err != nil {
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

	// Apply response size limit for error responses
	limitedBody := io.LimitReader(r.Body, c.cfg.MaxErrorResponseBytes+1)
	body, err := io.ReadAll(limitedBody)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding response body")
	}
	// TODO(@MahdiBaghbani): @glpatcern so this would be only 1 byte bigger, should we tolerate it?
	if int64(len(body)) > c.cfg.MaxErrorResponseBytes {
		return nil, ErrResponseTooLarge
	}
	// TODO(@MahdiBaghbani): @glpatcern is string() necessary here?
	return nil, errtypes.InternalError(string(body))
}

// NewNotification sends a notification to the remote end. Not implemented for now.
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1notifications/post
func (c *OCMClient) NewNotification(ctx context.Context, endpoint string, r *InviteAcceptedRequest) (*RemoteUser, error) {
	return nil, errtypes.NotSupported("not implemented")
}

// GetDirectoryService fetches a directory service listing from the given URL per OCM spec Appendix C.
func (c *OCMClient) GetDirectoryService(ctx context.Context, directoryURL string) (*DirectoryService, error) {
	log := appctx.GetLogger(ctx)

	endpointURL, err := url.Parse(directoryURL)
	if err != nil {
		return nil, errors.Wrapf(ErrURLValidationFailed, "invalid directory service URL: %v", err)
	}
	if err := validateURL(endpointURL, c.cfg); err != nil {
		return nil, errors.Wrapf(err, "directory service URL validation failed")
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.OverallTimeout)
	defer cancel()

	body, err := c.fetchGETEndpoint(ctx, directoryURL, c.cfg.MaxDiscoveryResponseBytes, "directory service")
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
