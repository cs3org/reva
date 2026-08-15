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

package ocmd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"

	"github.com/cs3org/reva/v3/pkg/errtypes"
)

const (
	defaultMaxRedirects  = 3
	maxRedirectsCeiling  = 10
	cloudMetadataIPv4    = "169.254.169.254"
	cloudMetadataAWSIPv6 = "fd00:ec2::254"
)

// UntrustedClientSecurity is the shared hatch and redirect policy for
// untrusted outbound OCM HTTP clients.
type UntrustedClientSecurity struct {
	MaxRedirects int      `mapstructure:"max_redirects"` // 0 => 3, ceiling 10
	AllowHTTP    bool     `mapstructure:"allow_http"`    // ocmreceived only
	AllowedCIDRs []string `mapstructure:"allowed_cidrs"` // ocmreceived only
	allowedNets  []*net.IPNet
}

// 64:ff9b::/96 embeds an IPv4 address in its low 32 bits (RFC 6052), so on a
// NAT64 network it can still reach internal hosts.
var nat64WellKnownPrefix = net.IPNet{IP: net.ParseIP("64:ff9b::"), Mask: net.CIDRMask(96, 128)}

// ErrNonHTTPS is returned when the untrusted client refuses a URL whose
// scheme is not allowed (https, or http only when AllowHTTP is set).
var ErrNonHTTPS = errors.New("untrusted OCM client requires https")

// ErrTooManyRedirects is the sentinel for a redirect chain that exceeds
// the configured cap. Callers should use errors.Is; the wrapped error
// includes the numeric cap.
var ErrTooManyRedirects = errors.New("untrusted OCM client: too many redirects")

// ErrNonPublicAddr is returned when dial Control refuses an address that
// is not allowed by the current policy (public-only, or PIN-when-set).
var ErrNonPublicAddr = errors.New("refusing to connect to non-public address")

// ErrHatchAllowHTTP is returned when a non-received consumer sets allow_http.
var ErrHatchAllowHTTP = errors.New("untrusted client security: allow_http is only valid for ocmreceived")

// ErrHatchAllowedCIDRs is returned when a non-received consumer sets allowed_cidrs.
var ErrHatchAllowedCIDRs = errors.New("untrusted client security: allowed_cidrs is only valid for ocmreceived")

var errInvalidCIDR = errors.New("invalid CIDR")

var errUnrestrictedCIDR = errors.New("unrestricted CIDR is not allowed")

var errNAT64Overlap = errors.New("CIDR overlaps NAT64 64:ff9b::/96")

// ApplyDefaults sets MaxRedirects 0 to 3 and clamps values above 10 to 10.
func (s *UntrustedClientSecurity) ApplyDefaults() {
	if s.MaxRedirects == 0 {
		s.MaxRedirects = defaultMaxRedirects
	}
	if s.MaxRedirects > maxRedirectsCeiling {
		s.MaxRedirects = maxRedirectsCeiling
	}
}

// Compile parses AllowedCIDRs into allowedNets. It rejects invalid CIDRs,
// 0.0.0.0/0, ::/0, and any CIDR that overlaps NAT64 64:ff9b::/96. On failure
// compiled nets are cleared so a retry cannot keep stale state.
func (s *UntrustedClientSecurity) Compile() error {
	s.allowedNets = nil

	nets := make([]*net.IPNet, 0, len(s.AllowedCIDRs))
	errs := make([]error, 0)
	for _, cidr := range s.AllowedCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid CIDR %q: %w", cidr, errors.Join(errInvalidCIDR, err)))
			continue
		}
		if isUnrestrictedCIDR(n) {
			errs = append(errs, fmt.Errorf("unrestricted CIDR %q is not allowed: %w", cidr, errUnrestrictedCIDR))
			continue
		}
		if cidrOverlapsNAT64(n) {
			errs = append(errs, fmt.Errorf("CIDR %q overlaps NAT64 64:ff9b::/96: %w", cidr, errNAT64Overlap))
			continue
		}
		nets = append(nets, n)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	s.allowedNets = nets
	return nil
}

// RejectHatch errors if AllowHTTP is true or AllowedCIDRs is non-empty.
// Non-received consumers call this in their New/start path.
func (s UntrustedClientSecurity) RejectHatch() error {
	if s.AllowHTTP {
		return ErrHatchAllowHTTP
	}
	if len(s.AllowedCIDRs) > 0 {
		return ErrHatchAllowedCIDRs
	}
	return nil
}

func (s UntrustedClientSecurity) maxRedirects() int {
	n := s.MaxRedirects
	if n <= 0 {
		return defaultMaxRedirects
	}
	if n > maxRedirectsCeiling {
		return maxRedirectsCeiling
	}
	return n
}

func (s UntrustedClientSecurity) requireScheme(u *url.URL) error {
	if u != nil && u.Scheme == "https" {
		return nil
	}
	if s.AllowHTTP && u != nil && u.Scheme == "http" {
		return nil
	}
	scheme := ""
	if u != nil {
		scheme = u.Scheme
	}
	return fmt.Errorf("untrusted OCM client refuses scheme %q: %w", scheme, ErrNonHTTPS)
}

func (s UntrustedClientSecurity) tooManyRedirectsError() error {
	return fmt.Errorf("untrusted OCM client stopped after %d redirects: %w", s.maxRedirects(), ErrTooManyRedirects)
}

// CheckRedirect refuses more than maxRedirects hops (len(via) > max, not >=)
// and enforces requireScheme on each target.
func (s UntrustedClientSecurity) CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > s.maxRedirects() {
		return s.tooManyRedirectsError()
	}
	if req == nil {
		return s.requireScheme(nil)
	}
	return s.requireScheme(req.URL)
}

func (s UntrustedClientSecurity) refuseNonPublicAddr(network, addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !s.allowsDial(ip) {
		return fmt.Errorf("%w: %w", ErrNonPublicAddr, errtypes.BadRequest("refusing to connect to non-public address "+addr))
	}
	return nil
}

func (s UntrustedClientSecurity) dialControl(network, address string, _ syscall.RawConn) error {
	return s.refuseNonPublicAddr(network, address)
}

func (s UntrustedClientSecurity) allowsDial(ip net.IP) bool {
	if ip == nil {
		return false
	}
	unwrapped := unwrapNAT64(ip)
	if isHardDeniedUnlessExactHost(unwrapped) || isHardDeniedUnlessExactHost(ip) {
		return hasExactHostAllow(unwrapped, s.allowedNets) || hasExactHostAllow(ip, s.allowedNets)
	}
	if len(s.allowedNets) > 0 {
		return ipInAny(unwrapped, s.allowedNets) || ipInAny(ip, s.allowedNets)
	}
	return isPublicIP(unwrapped)
}

func defaultUntrustedSecurity() UntrustedClientSecurity {
	var s UntrustedClientSecurity
	s.ApplyDefaults()
	return s
}

func isUnrestrictedCIDR(n *net.IPNet) bool {
	if n == nil {
		return false
	}
	ones, bits := n.Mask.Size()
	if ones != 0 {
		return false
	}
	switch bits {
	case 32:
		return n.IP.Equal(net.IPv4zero)
	case 128:
		return n.IP.Equal(net.IPv6zero)
	default:
		return false
	}
}

func cidrOverlapsNAT64(n *net.IPNet) bool {
	if n == nil {
		return false
	}
	_, bits := n.Mask.Size()
	if bits == 32 {
		return false
	}
	return n.Contains(nat64WellKnownPrefix.IP) || nat64WellKnownPrefix.Contains(n.IP)
}

func unwrapNAT64(ip net.IP) net.IP {
	if ip == nil || !nat64WellKnownPrefix.Contains(ip) {
		return ip
	}
	v6 := ip.To16()
	if v6 == nil {
		return ip
	}
	return net.IPv4(v6[12], v6[13], v6[14], v6[15])
}

func ipInAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

func isHardDeniedUnlessExactHost(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLinkLocalUnicast() {
		return true
	}
	if ip.Equal(net.ParseIP(cloudMetadataIPv4)) {
		return true
	}
	if ip.Equal(net.ParseIP(cloudMetadataAWSIPv6)) {
		return true
	}
	return false
}

func isExactHostNet(n *net.IPNet) bool {
	if n == nil {
		return false
	}
	ones, bits := n.Mask.Size()
	return ones == bits && (bits == 32 || bits == 128)
}

func hasExactHostAllow(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n != nil && isExactHostNet(n) && n.Contains(ip) {
			return true
		}
	}
	return false
}

func isPublicIP(ip net.IP) bool {
	ip = unwrapNAT64(ip)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	// 100.64.0.0/10 is carrier-grade NAT, which net.IP.IsPrivate does not cover
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1]&0xc0 == 64 {
		return false
	}
	return true
}

// refuseNonPublicAddr is the default-hatch Control used by existing tests.
func refuseNonPublicAddr(network, address string, c syscall.RawConn) error {
	return defaultUntrustedSecurity().dialControl(network, address, c)
}

// PublicOnlyCheckRedirect is the default-hatch CheckRedirect (3 hops, https).
// Owned clients should prefer UntrustedClientSecurity.CheckRedirect.
func PublicOnlyCheckRedirect(req *http.Request, via []*http.Request) error {
	return defaultUntrustedSecurity().CheckRedirect(req, via)
}
