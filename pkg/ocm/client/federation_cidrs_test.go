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
	"net/netip"
	"testing"
)

func TestParseFederationCIDRsNilAndEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
	}{
		{name: "nil", values: nil},
		{name: "empty", values: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseFederationCIDRs(tt.values)
			if err != nil {
				t.Fatalf("ParseFederationCIDRs() error = %v", err)
			}
			if len(got.prefixes) != 0 {
				t.Fatalf("prefixes = %v, want none", got.prefixes)
			}
			if got.contains(netip.MustParseAddr("10.1.2.3")) {
				t.Fatal("zero policy admitted 10.1.2.3")
			}
		})
	}
}

func TestParseFederationCIDRsCanonical(t *testing.T) {
	t.Parallel()

	values := []string{
		"10.0.0.0/8",
		"10.1.2.0/24",
		"172.16.0.0/12",
		"172.31.5.0/24",
		"192.168.0.0/16",
		"192.168.50.10/32",
		"fc00::/7",
		"fd00::/8",
		"fd12:3456:789a::/48",
		"fd00::abcd/128",
	}
	got, err := ParseFederationCIDRs(values)
	if err != nil {
		t.Fatalf("ParseFederationCIDRs() error = %v", err)
	}
	if len(got.prefixes) != len(values) {
		t.Fatalf("len(prefixes) = %d, want %d", len(got.prefixes), len(values))
	}
	for _, raw := range values {
		prefix := netip.MustParsePrefix(raw)
		if !got.contains(prefix.Addr()) {
			t.Errorf("policy missing network address of %s", raw)
		}
	}
}

func TestParseFederationCIDRsDuplicateAndOverlap(t *testing.T) {
	t.Parallel()

	got, err := ParseFederationCIDRs([]string{
		"192.168.0.0/16",
		"10.0.0.0/8",
		"192.168.0.0/16",
		"10.1.2.0/24",
		"10.1.2.0/25",
	})
	if err != nil {
		t.Fatalf("ParseFederationCIDRs() error = %v", err)
	}
	if len(got.prefixes) != 4 {
		t.Fatalf("len(prefixes) = %d, want 4 (exact duplicate dropped, overlaps kept)", len(got.prefixes))
	}
	want := []string{"192.168.0.0/16", "10.0.0.0/8", "10.1.2.0/24", "10.1.2.0/25"}
	for i, raw := range want {
		if got.prefixes[i] != netip.MustParsePrefix(raw) {
			t.Errorf("prefixes[%d] = %s, want %s", i, got.prefixes[i], raw)
		}
	}
	if !got.contains(netip.MustParseAddr("10.9.9.9")) {
		t.Error("overlap handling dropped 10.0.0.0/8")
	}
	if !got.contains(netip.MustParseAddr("10.1.2.200")) {
		t.Error("overlap handling dropped 10.1.2.0/24")
	}
	if !got.contains(netip.MustParseAddr("10.1.3.1")) {
		t.Error("overlap handling dropped 10.0.0.0/8 coverage of 10.1.3.1")
	}
}

func TestParseFederationCIDRsInputMutation(t *testing.T) {
	t.Parallel()

	values := []string{"10.2.0.0/16", "fd00::/8"}
	got, err := ParseFederationCIDRs(values)
	if err != nil {
		t.Fatalf("ParseFederationCIDRs() error = %v", err)
	}
	values[0] = "11.0.0.0/8"
	values[1] = "not-a-prefix"
	if !got.contains(netip.MustParseAddr("10.2.1.1")) {
		t.Error("mutating the input slice changed the parsed policy")
	}
	if !got.contains(netip.MustParseAddr("fd00::1")) {
		t.Error("mutating the input slice dropped the ULA prefix")
	}
	if got.contains(netip.MustParseAddr("11.0.0.1")) {
		t.Error("parsed policy followed the mutated input")
	}
}

func TestParseFederationCIDRsAtomicRejection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
	}{
		{name: "invalid after valid", values: []string{"10.0.0.0/8", "nope", "192.168.0.0/16"}},
		{name: "invalid before valid", values: []string{"8.8.8.8/32", "10.0.0.0/8"}},
		{name: "duplicate then invalid", values: []string{"10.0.0.0/8", "10.0.0.0/8", "bogus"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseFederationCIDRs(tt.values)
			if !errors.Is(err, errInvalidFederationCIDR) {
				t.Fatalf("error = %v, want errors.Is invalid federation CIDR", err)
			}
			if len(got.prefixes) != 0 {
				t.Fatalf("partial policy prefixes = %v", got.prefixes)
			}
			if got.contains(netip.MustParseAddr("10.1.2.3")) {
				t.Fatal("rejected parse still admitted 10.1.2.3")
			}
		})
	}
}

func TestParseFederationCIDRsRejectsInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{name: "blank", value: ""},
		{name: "space", value: " "},
		{name: "padded prefix", value: " 10.0.0.0/8"},
		{name: "trailing space", value: "10.0.0.0/8 "},
		{name: "trailing newline", value: "10.0.0.0/8\n"},
		{name: "bare ipv4", value: "10.1.2.3"},
		{name: "bare ipv6", value: "fd00::1"},
		{name: "ipv4 host bits", value: "10.1.2.3/8"},
		{name: "ipv4 host bits 16", value: "192.168.1.1/16"},
		{name: "ipv6 host bits", value: "fd00::1/64"},
		{name: "ipv4 length too wide", value: "10.0.0.0/128"},
		{name: "ipv4 length 33", value: "192.168.1.1/33"},
		{name: "ipv6 length 129", value: "fc00::/129"},
		{name: "ipv4 default route", value: "0.0.0.0/0"},
		{name: "ipv6 default route", value: "::/0"},
		{name: "private slash zero", value: "10.0.0.0/0"},
		{name: "zone on ula", value: "fc00::1%eth0/128"},
		{name: "zone on link-local", value: "fe80::1%eth0/10"},
		{name: "mapped prefix", value: "::ffff:10.0.0.0/104"},
		{name: "mapped host", value: "::ffff:192.168.1.1/128"},
		{name: "well-known nat64", value: "64:ff9b::/96"},
		{name: "local nat64", value: "64:ff9b:1::/48"},
		{name: "nat64 host", value: "64:ff9b::a01:203/128"},
		{name: "public v4", value: "8.8.8.8/32"},
		{name: "loopback v4", value: "127.0.0.0/8"},
		{name: "loopback v6", value: "::1/128"},
		{name: "link-local v4", value: "169.254.0.0/16"},
		{name: "link-local v6", value: "fe80::/10"},
		{name: "cgnat", value: "100.64.0.0/10"},
		{name: "documentation v4", value: "192.0.2.0/24"},
		{name: "documentation v6", value: "2001:db8::/32"},
		{name: "test net 2", value: "198.51.100.0/24"},
		{name: "test net 3", value: "203.0.113.0/24"},
		{name: "multicast v4", value: "224.0.0.0/4"},
		{name: "multicast v6", value: "ff00::/8"},
		{name: "unspecified v4", value: "0.0.0.0/8"},
		{name: "unspecified v6", value: "::/128"},
		{name: "reserved v4", value: "240.0.0.0/4"},
		{name: "below 10/8", value: "9.255.255.255/32"},
		{name: "above 10/8", value: "11.0.0.0/32"},
		{name: "below 172.16/12", value: "172.15.255.255/32"},
		{name: "above 172.16/12", value: "172.32.0.0/32"},
		{name: "below 192.168/16", value: "192.167.255.255/32"},
		{name: "above 192.168/16", value: "192.169.0.0/32"},
		{name: "outside ula", value: "fb00::/8"},
		{name: "url", value: "https://10.0.0.0/8"},
		{name: "port", value: "10.1.2.3:443"},
		{name: "hostname", value: "example.com/32"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseFederationCIDRs([]string{"10.0.0.0/8", tt.value})
			if !errors.Is(err, errInvalidFederationCIDR) {
				t.Fatalf("ParseFederationCIDRs(%q) error = %v, want invalid federation CIDR", tt.value, err)
			}
			if len(got.prefixes) != 0 || got.contains(netip.MustParseAddr("10.0.0.1")) {
				t.Fatal("invalid entry left a usable policy")
			}
		})
	}
}

func TestParseFederationCIDRsSubnetBoundaries(t *testing.T) {
	t.Parallel()

	valid := []string{
		"10.0.0.0/8",
		"10.255.255.255/32",
		"172.16.0.0/12",
		"172.31.255.255/32",
		"192.168.0.0/16",
		"192.168.255.255/32",
		"fc00::/7",
		"fdff::/128",
	}
	got, err := ParseFederationCIDRs(valid)
	if err != nil {
		t.Fatalf("ParseFederationCIDRs(boundaries) error = %v", err)
	}
	if len(got.prefixes) != len(valid) {
		t.Fatalf("len(prefixes) = %d, want %d", len(got.prefixes), len(valid))
	}
}

func mustFederationCIDRs(t *testing.T, values ...string) FederationCIDRs {
	t.Helper()
	got, err := ParseFederationCIDRs(values)
	if err != nil {
		t.Fatalf("ParseFederationCIDRs(%q) error = %v", values, err)
	}
	return got
}
