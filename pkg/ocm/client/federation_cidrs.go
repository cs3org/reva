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

package client

import (
	"errors"
	"fmt"
	"net/netip"
)

// errInvalidFederationCIDR is the fixed parse failure for a federation CIDR list.
// Any invalid entry rejects the whole list.
var errInvalidFederationCIDR = errors.New("invalid federation CIDR")

// Supernets that may contain an explicit private exception. Anything else,
// including public, loopback, link-local, multicast, CGNAT, NAT64, and
// documentation space, is rejected because it is not wholly inside one of these.
var (
	federationRFC1918Ten     = netip.MustParsePrefix("10.0.0.0/8")
	federationRFC1918Twelve  = netip.MustParsePrefix("172.16.0.0/12")
	federationRFC1918Sixteen = netip.MustParsePrefix("192.168.0.0/16")
	federationULA            = netip.MustParsePrefix("fc00::/7")
)

// FederationCIDRs is an immutable explicit private-network exception list.
// The zero value allows no exceptions. ParseFederationCIDRs is the only
// population path; there is no setter and no exported prefix slice.
type FederationCIDRs struct {
	prefixes []netip.Prefix
}

// ParseFederationCIDRs parses an explicit private-network exception list.
// Each value must be a canonical netip.ParsePrefix string with no whitespace,
// zone, port, URL, or hostname, and the prefix must be wholly inside RFC 1918
// or IPv6 ULA (fc00::/7). A nil or empty list returns the zero policy. Any
// invalid entry rejects the whole list and returns no usable policy.
func ParseFederationCIDRs(values []string) (FederationCIDRs, error) {
	if len(values) == 0 {
		return FederationCIDRs{}, nil
	}

	prefixes := make([]netip.Prefix, 0, len(values))
	seen := make(map[netip.Prefix]struct{}, len(values))
	for _, raw := range values {
		prefix, err := parseFederationCIDR(raw)
		if err != nil {
			return FederationCIDRs{}, err
		}
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		prefixes = append(prefixes, prefix)
	}
	return FederationCIDRs{prefixes: prefixes}, nil
}

func parseFederationCIDR(raw string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		return netip.Prefix{}, invalidFederationCIDR(raw)
	}
	// Host bits must already be cleared. ParsePrefix keeps them, so require
	// the canonical masked prefix. That also rejects a missing slash only via
	// ParsePrefix; callers must not trim or rewrite the input.
	if prefix != prefix.Masked() || prefix.Bits() == 0 {
		return netip.Prefix{}, invalidFederationCIDR(raw)
	}
	if prefix.Addr().Is4In6() {
		return netip.Prefix{}, invalidFederationCIDR(raw)
	}
	if !prefixInFederationRange(prefix) {
		return netip.Prefix{}, invalidFederationCIDR(raw)
	}
	return prefix, nil
}

func invalidFederationCIDR(raw string) error {
	return fmt.Errorf("%w: %q", errInvalidFederationCIDR, raw)
}

func prefixInFederationRange(prefix netip.Prefix) bool {
	switch {
	case prefix.Addr().Is4():
		return prefixWhollyContained(federationRFC1918Ten, prefix) ||
			prefixWhollyContained(federationRFC1918Twelve, prefix) ||
			prefixWhollyContained(federationRFC1918Sixteen, prefix)
	case prefix.Addr().Is6():
		return prefixWhollyContained(federationULA, prefix)
	default:
		return false
	}
}

func prefixWhollyContained(outer, inner netip.Prefix) bool {
	if outer.Addr().BitLen() != inner.Addr().BitLen() {
		return false
	}
	if inner.Bits() < outer.Bits() {
		return false
	}
	return outer.Contains(inner.Addr())
}

func (c FederationCIDRs) contains(ip netip.Addr) bool {
	for _, prefix := range c.prefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// clone returns a policy that does not share its backing array. The zero
// policy stays zero so an empty list and a nil list compare the same.
func (c FederationCIDRs) clone() FederationCIDRs {
	if len(c.prefixes) == 0 {
		return FederationCIDRs{}
	}
	copied := make([]netip.Prefix, len(c.prefixes))
	copy(copied, c.prefixes)
	return FederationCIDRs{prefixes: copied}
}
