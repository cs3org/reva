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
	"syscall"
	"time"
)

// ErrPolicyViolation is returned when a public-only dial is blocked.
var ErrPolicyViolation = errors.New("ocm http policy violation")

const defaultTimeout = 10 * time.Second

// well-known NAT64 prefix embeds an IPv4 address in its low 32 bits (RFC 6052).
var nat64WellKnownPrefix = netip.MustParsePrefix("64:ff9b::/96")

// 100.64.0.0/10 is carrier-grade NAT, which netip.Addr.IsPrivate does not cover.
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

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

// NewTrustedHTTPClient returns an HTTP client that keeps environment proxy support.
func NewTrustedHTTPClient(cfg TransportConfig) *http.Client {
	cfg = normalizeTransportConfig(cfg)
	return &http.Client{
		Transport: cloneOCMTransport(cfg),
		Timeout:   cfg.Timeout,
	}
}

// NewPublicOnlyHTTPClient returns an HTTP client that only dials public addresses.
func NewPublicOnlyHTTPClient(cfg TransportConfig) *http.Client {
	cfg = normalizeTransportConfig(cfg)
	return &http.Client{
		Transport: newPublicOnlyTransport(cfg),
		Timeout:   cfg.Timeout,
	}
}

// NewPublicOnlyRoundTripper returns a public-only transport with the same address policy.
func NewPublicOnlyRoundTripper(cfg TransportConfig) http.RoundTripper {
	cfg = normalizeTransportConfig(cfg)
	return newPublicOnlyTransport(cfg)
}

func cloneOCMTransport(cfg TransportConfig) *http.Transport {
	tr := defaultTransportTemplate.Clone()
	if tr.TLSClientConfig != nil {
		tr.TLSClientConfig = tr.TLSClientConfig.Clone()
	} else {
		tr.TLSClientConfig = &tls.Config{}
	}
	tr.TLSClientConfig.InsecureSkipVerify = cfg.Insecure
	return tr
}

func newPublicOnlyTransport(cfg TransportConfig) *http.Transport {
	tr := cloneOCMTransport(cfg)
	// with a proxy the dial goes to the proxy, so Control never sees the target
	tr.Proxy = nil
	tr.DialContext = (&net.Dialer{
		Timeout:   cfg.Timeout,
		KeepAlive: 30 * time.Second,
		Control:   refuseNonPublicAddr(cfg.AllowLoopback),
	}).DialContext
	return tr
}

func refuseNonPublicAddr(allowLoopback bool) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		return checkResolvedAddr(address, allowLoopback)
	}
}

func checkResolvedAddr(address string, allowLoopback bool) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !isAllowedIP(ip, allowLoopback) {
		return fmt.Errorf("%w: refusing to connect to non-public address %s", ErrPolicyViolation, address)
	}
	return nil
}

func isAllowedIP(ip netip.Addr, allowLoopback bool) bool {
	if allowLoopback && effectiveAddr(ip).IsLoopback() {
		return true
	}
	return isPublicIP(ip)
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
	isLoopback := ip.IsLoopback()
	isPrivate := ip.IsPrivate()
	isUnspecified := ip.IsUnspecified()
	isLinkLocal := ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
	isMulticast := ip.IsMulticast() || ip.IsInterfaceLocalMulticast()
	if isLoopback || isPrivate || isUnspecified || isLinkLocal || isMulticast {
		return false
	}
	if cgnatPrefix.Contains(ip) {
		return false
	}
	return true
}
