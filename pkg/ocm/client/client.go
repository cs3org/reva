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

// Package client provides shared HTTP transports for outbound OCM calls.
package client

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// ErrPolicyViolation is returned when a public-only dial is blocked.
var ErrPolicyViolation = errors.New("ocm http policy violation")

const defaultTimeout = 10 * time.Second

// well-known NAT64 prefix embeds an IPv4 address in its low 32 bits (RFC 6052).
var nat64WellKnownPrefix = netip.MustParsePrefix("64:ff9b::/96")

// 64:ff9b:1::/48 is local-use NAT64 and is denied wholesale, not unwrapped.
var nat64LocalUsePrefix = netip.MustParsePrefix("64:ff9b:1::/48")

// 192.0.0.9 and 192.0.0.10 are globally reachable PCP and TURN anycast
// (RFC 7723/8155), so they stay allowed inside otherwise-denied 192.0.0.0/24.
var (
	pcpAnycastAddr  = netip.MustParseAddr("192.0.0.9")
	turnAnycastAddr = netip.MustParseAddr("192.0.0.10")
)

var deniedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	nat64LocalUsePrefix,
	netip.MustParsePrefix("2001:db8::/32"),
}

// defaultTransportTemplate is a clone of http.DefaultTransport taken at init.
var defaultTransportTemplate *http.Transport

func init() {
	dt, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		defaultTransportTemplate = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}
		return
	}
	defaultTransportTemplate = dt.Clone()
}

// TransportConfig is the runtime HTTP transport config for OCM clients.
type TransportConfig struct {
	Timeout       time.Duration
	Insecure      bool
	AllowLoopback bool
	// UseEnvProxy is the public-only environment-proxy opt-in. False is the
	// safe default. True installs http.ProxyFromEnvironment. Trusted clients
	// ignore this field and retain proxy support from #5674.
	UseEnvProxy bool
	// AllowedFederationCIDRs is an explicit private-network exception list for
	// the public-only dial guard. The zero value allows no private exceptions.
	// Trusted clients ignore this field.
	AllowedFederationCIDRs FederationCIDRs
}

// destinationPolicy is the per-dialer address policy. It is captured by value
// when the dialer is built so later caller config changes cannot affect it.
type destinationPolicy struct {
	allowLoopback bool
	cidrs         FederationCIDRs
}

// DefaultTransportConfig returns normalized runtime defaults.
func DefaultTransportConfig() TransportConfig {
	return TransportConfig{
		Timeout: defaultTimeout,
	}
}

func normalizeTransportConfig(cfg TransportConfig) TransportConfig {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return cfg
}

func newDestinationPolicy(cfg TransportConfig) destinationPolicy {
	return destinationPolicy{
		allowLoopback: cfg.AllowLoopback,
		cidrs:         cfg.AllowedFederationCIDRs.clone(),
	}
}

// NewTrustedHTTPClient returns an HTTP client that keeps environment proxy
// support from #5674. It ignores UseEnvProxy.
func NewTrustedHTTPClient(cfg TransportConfig) *http.Client {
	cfg = normalizeTransportConfig(cfg)
	return &http.Client{
		Transport: cloneOCMTransport(cfg),
		Timeout:   cfg.Timeout,
	}
}

// NewPublicOnlyHTTPClient returns an HTTP client that dials public addresses
// and, when set, AllowedFederationCIDRs. Direct mode (UseEnvProxy false) is
// the safe default. True installs http.ProxyFromEnvironment. A selected proxy
// hides the OCM target from Control; the guarded dialer classifies the proxy hop.
// Policy is HTTPS-only; initial plain HTTP is allowed only for literal loopback
// when AllowLoopback is true. AllowedFederationCIDRs does not permit plain HTTP.
// Non-HTTPS redirects are rejected (10-redirect limit). Literal HTTPS redirects
// use the same destination policy as dial Control.
func NewPublicOnlyHTTPClient(cfg TransportConfig) *http.Client {
	cfg = normalizeTransportConfig(cfg)
	pt, policy := newPublicOnlyParts(cfg)
	return &http.Client{
		Transport:     pt,
		Timeout:       cfg.Timeout,
		CheckRedirect: newPublicOnlyCheckRedirect(policy),
	}
}

// NewPublicOnlyRoundTripper returns a public-only transport with the same
// address policy, proxy contract, and scheme policy as NewPublicOnlyHTTPClient:
// HTTPS-only, with initial plain HTTP only for literal loopback when
// AllowLoopback is true. Callers that follow redirects themselves still hit
// the req.Response scheme check; dial Control applies the same address policy.
func NewPublicOnlyRoundTripper(cfg TransportConfig) http.RoundTripper {
	cfg = normalizeTransportConfig(cfg)
	pt, _ := newPublicOnlyParts(cfg)
	return pt
}

// newPublicOnlyParts captures one immutable destination policy and installs it
// on both the scheme wrapper and the dial Control.
func newPublicOnlyParts(cfg TransportConfig) (*publicOnlyTransport, destinationPolicy) {
	policy := newDestinationPolicy(cfg)
	return &publicOnlyTransport{
		base:   newPublicOnlyTransport(cfg, policy),
		policy: policy,
	}, policy
}

func cloneOCMTransport(cfg TransportConfig) *http.Transport {
	tr := defaultTransportTemplate.Clone()
	if tr.TLSClientConfig != nil {
		tr.TLSClientConfig = tr.TLSClientConfig.Clone()
	} else {
		tr.TLSClientConfig = &tls.Config{}
	}
	tr.TLSClientConfig.InsecureSkipVerify = cfg.Insecure
	tr.TLSClientConfig.MinVersion = tls.VersionTLS12
	return tr
}

func newPublicOnlyTransport(cfg TransportConfig, policy destinationPolicy) *http.Transport {
	tr := cloneOCMTransport(cfg)
	// False is the safe default and leaves Proxy nil. True installs
	// http.ProxyFromEnvironment. Control sees the proxy hop when one is
	// selected, so a selected proxy hides the OCM target from Control. Go
	// reads HTTP_PROXY, HTTPS_PROXY, and NO_PROXY once per process on the
	// first ProxyFromEnvironment call. Restart the process after changing
	// those variables.
	if cfg.UseEnvProxy {
		tr.Proxy = http.ProxyFromEnvironment
	} else {
		tr.Proxy = nil
	}
	tr.DialContext = newPublicOnlyDialerWithPolicy(cfg, policy).DialContext
	return tr
}

func newPublicOnlyDialer(cfg TransportConfig) *net.Dialer {
	return newPublicOnlyDialerWithPolicy(cfg, newDestinationPolicy(cfg))
}

func newPublicOnlyDialerWithPolicy(cfg TransportConfig, policy destinationPolicy) *net.Dialer {
	return &net.Dialer{
		Timeout:   cfg.Timeout,
		KeepAlive: 30 * time.Second,
		Control:   refuseNonPublicAddr(policy),
	}
}

// publicOnlyTransport enforces scheme policy for every request, including
// schemes net/http would reject with its own error before dialing, such as
// ftp, gopher, and file.
type publicOnlyTransport struct {
	base   *http.Transport
	policy destinationPolicy
}

// HTTPTransport returns the guarded base *http.Transport for a public-only
// wrapper. It returns the transport itself for a trusted client. It returns
// nil for other round trippers. External package tests use it to inspect
// proxy, dialer, and TLS settings. Production code must not use it to bypass
// the scheme wrapper.
func HTTPTransport(rt http.RoundTripper) *http.Transport {
	if pt, ok := rt.(*publicOnlyTransport); ok {
		return pt.base
	}
	if tr, ok := rt.(*http.Transport); ok {
		return tr
	}
	return nil
}

func (t *publicOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := checkRequestScheme(req, t.policy.allowLoopback); err != nil {
		if req != nil && req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func (t *publicOnlyTransport) CloseIdleConnections() {
	t.base.CloseIdleConnections()
}

func checkRequestScheme(req *http.Request, allowLoopback bool) error {
	if req == nil || req.URL == nil {
		return fmt.Errorf("%w: missing request URL", ErrPolicyViolation)
	}
	switch req.URL.Scheme {
	case "https":
		return nil
	case "http":
		// redirected client requests carry the response that created them.
		// loopback HTTP is an initial-request test-mode exception only.
		if req.Response != nil {
			return fmt.Errorf("%w: refusing http redirect", ErrPolicyViolation)
		}
		if !allowLoopback || !isLiteralLoopbackHost(req.URL.Hostname()) {
			return fmt.Errorf("%w: refusing scheme %q", ErrPolicyViolation, req.URL.Scheme)
		}
		return nil
	default:
		return fmt.Errorf("%w: refusing scheme %q", ErrPolicyViolation, req.URL.Scheme)
	}
}

func isLiteralLoopbackHost(host string) bool {
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return effectiveAddr(ip).IsLoopback()
}

func newPublicOnlyCheckRedirect(policy destinationPolicy) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("%w: stopped after 10 redirects", ErrPolicyViolation)
		}
		if req == nil || req.URL == nil || req.URL.Scheme != "https" {
			return fmt.Errorf("%w: refusing non-https redirect", ErrPolicyViolation)
		}
		return checkRedirectDestination(req.URL, policy)
	}
}

func checkRedirectDestination(u *url.URL, policy destinationPolicy) error {
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: redirect missing host", ErrPolicyViolation)
	}
	if _, err := netip.ParseAddr(host); err != nil {
		return nil
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	return checkResolvedAddr(net.JoinHostPort(host, port), policy)
}

func refuseNonPublicAddr(policy destinationPolicy) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		return checkResolvedAddr(address, policy)
	}
}

func checkResolvedAddr(address string, policy destinationPolicy) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf(
			"%w: refusing to connect to malformed address %s: %w",
			ErrPolicyViolation,
			address,
			err,
		)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w: refusing to connect to non-public address %s", ErrPolicyViolation, address)
	}
	// Reject every parsed IP with a non-empty IPv6 zone before classification.
	// netip.Prefix.Contains returns false for zoned addresses, so NAT64/denied-prefix
	// checks would be bypassed; Unmap drops the zone, so a zoned IPv4-mapped or zoned public IPv6 cannot
	// be safely normalized. Intentional and conservative for a public-only client; AllowLoopback must not permit zoned ::1.
	if ip.Zone() != "" {
		return fmt.Errorf(
			"%w: refusing to connect to zoned address %s",
			ErrPolicyViolation,
			address,
		)
	}
	if !isAllowedIP(ip, policy) {
		return fmt.Errorf("%w: refusing to connect to non-public address %s", ErrPolicyViolation, address)
	}
	return nil
}

func isAllowedIP(ip netip.Addr, policy destinationPolicy) bool {
	normalized := effectiveAddr(ip)
	if policy.allowLoopback && normalized.IsLoopback() {
		return true
	}
	if isPublicIP(ip) {
		return true
	}
	// Loopback is only the AllowLoopback flag. A CIDR match cannot admit it,
	// and neither can any other non-private denial (link-local, CGNAT, and so on).
	if !normalized.IsPrivate() {
		return false
	}
	return policy.cidrs.contains(normalized)
}

func effectiveAddr(ip netip.Addr) netip.Addr {
	ip = ip.Unmap()
	if !nat64WellKnownPrefix.Contains(ip) {
		return ip
	}
	octets := ip.As16()
	return netip.AddrFrom4([4]byte{octets[12], octets[13], octets[14], octets[15]})
}

func isPublicIP(ip netip.Addr) bool {
	ip = effectiveAddr(ip)
	if ip == pcpAnycastAddr || ip == turnAnycastAddr {
		return true
	}
	isLoopback := ip.IsLoopback()
	isPrivate := ip.IsPrivate()
	isUnspecified := ip.IsUnspecified()
	isLinkLocal := ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
	isMulticast := ip.IsMulticast() || ip.IsInterfaceLocalMulticast()
	if isLoopback || isPrivate || isUnspecified || isLinkLocal || isMulticast {
		return false
	}
	for _, prefix := range deniedPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}
