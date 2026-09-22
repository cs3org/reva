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
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestCheckResolvedAddrRejectsIPv6Zone(t *testing.T) {
	t.Parallel()

	zoned := []struct {
		name    string
		address string
	}{
		{name: "nat64 metadata zone 0", address: "[64:ff9b::a9fe:a9fe%0]:443"},
		{name: "nat64 metadata zone eth0", address: "[64:ff9b::a9fe:a9fe%eth0]:443"},
		{name: "nat64 loopback", address: "[64:ff9b::7f00:1%0]:443"},
		{name: "local-use nat64", address: "[64:ff9b:1::1%0]:443"},
		{name: "documentation ipv6", address: "[2001:db8::1%0]:443"},
		{name: "unspecified ipv6", address: "[::%0]:443"},
		{name: "link-local", address: "[fe80::1%0]:443"},
		{name: "loopback", address: "[::1%0]:443"},
		{name: "ula", address: "[fc00::1%0]:443"},
		{name: "otherwise-public ipv6", address: "[2606:4700::1%0]:443"},
		{name: "mapped loopback", address: "[::ffff:127.0.0.1%0]:443"},
		{name: "mapped pcp exception", address: "[::ffff:192.0.0.9%0]:443"},
		{name: "mapped private ipv4 zone eth0", address: "[::ffff:10.1.2.3%eth0]:443"},
	}

	for _, tt := range zoned {
		for _, allowLoopback := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/allowLoopback=%t", tt.name, allowLoopback), func(t *testing.T) {
				t.Parallel()
				requireZonedRejection(t, tt.address, allowLoopback)
			})
		}
	}

	t.Run("url-encoded zone", func(t *testing.T) {
		t.Parallel()
		u, err := url.Parse("https://[64:ff9b::a9fe:a9fe%250]/")
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		address := net.JoinHostPort(u.Hostname(), "443")
		for _, allowLoopback := range []bool{false, true} {
			t.Run(fmt.Sprintf("allowLoopback=%t", allowLoopback), func(t *testing.T) {
				t.Parallel()
				requireZonedRejection(t, address, allowLoopback)
			})
		}
	})

	t.Run("unzoned nat64 metadata is non-public", func(t *testing.T) {
		t.Parallel()
		requireNonPublic(t, "[64:ff9b::a9fe:a9fe]:443", false)
		requireNonPublic(t, "[64:ff9b::a9fe:a9fe]:443", true)
	})

	t.Run("unzoned nat64 wrapping public ipv4 is allowed", func(t *testing.T) {
		t.Parallel()
		requireAllowed(t, "[64:ff9b::5db8:d822]:443", false)
		requireAllowed(t, "[64:ff9b::5db8:d822]:443", true)
	})

	t.Run("unzoned public ipv6 is allowed", func(t *testing.T) {
		t.Parallel()
		requireAllowed(t, "[2606:4700::1]:443", false)
		requireAllowed(t, "[2606:4700::1]:443", true)
	})

	t.Run("unzoned link-local is non-public", func(t *testing.T) {
		t.Parallel()
		requireNonPublic(t, "[fe80::1]:443", false)
		requireNonPublic(t, "[fe80::1]:443", true)
	})

	t.Run("unzoned unspecified ipv6 is non-public", func(t *testing.T) {
		t.Parallel()
		requireNonPublic(t, "[::]:443", false)
		requireNonPublic(t, "[::]:443", true)
	})

	t.Run("unzoned loopback allowed only when configured", func(t *testing.T) {
		t.Parallel()
		requireNonPublic(t, "[::1]:443", false)
		requireAllowed(t, "[::1]:443", true)
		requireNonPublic(t, "[::ffff:127.0.0.1]:443", false)
		requireAllowed(t, "[::ffff:127.0.0.1]:443", true)
	})

	t.Run("unzoned mapped pcp remains allowed", func(t *testing.T) {
		t.Parallel()
		requireAllowed(t, "[::ffff:192.0.0.9]:443", false)
		requireAllowed(t, "[::ffff:192.0.0.9]:443", true)
	})

	t.Run("unzoned mapped turn remains allowed", func(t *testing.T) {
		t.Parallel()
		requireAllowed(t, "[::ffff:192.0.0.10]:443", false)
		requireAllowed(t, "[::ffff:192.0.0.10]:443", true)
	})
}

func requireZonedRejection(t *testing.T, address string, allowLoopback bool) {
	t.Helper()
	err := checkResolvedAddr(address, allowLoopback)
	if !errors.Is(err, ErrPolicyViolation) {
		t.Errorf(
			"checkResolvedAddr(%q, allowLoopback=%t) error = %v, want errors.Is ErrPolicyViolation",
			address,
			allowLoopback,
			err,
		)
	}
	if err == nil {
		return
	}
	msg := err.Error()
	if !strings.Contains(msg, "zoned address") {
		t.Errorf(
			"checkResolvedAddr(%q, allowLoopback=%t) error %q, want substring %q",
			address,
			allowLoopback,
			msg,
			"zoned address",
		)
	}
	if !strings.Contains(msg, address) {
		t.Errorf(
			"checkResolvedAddr(%q, allowLoopback=%t) error %q, want full dial address",
			address,
			allowLoopback,
			msg,
		)
	}
}

func requireNonPublic(t *testing.T, address string, allowLoopback bool) {
	t.Helper()
	err := checkResolvedAddr(address, allowLoopback)
	if !errors.Is(err, ErrPolicyViolation) {
		t.Errorf(
			"checkResolvedAddr(%q, allowLoopback=%t) error = %v, want errors.Is ErrPolicyViolation",
			address,
			allowLoopback,
			err,
		)
	}
	if err == nil {
		return
	}
	msg := err.Error()
	if !strings.Contains(msg, "non-public address") {
		t.Errorf(
			"checkResolvedAddr(%q, allowLoopback=%t) error %q, want substring %q",
			address,
			allowLoopback,
			msg,
			"non-public address",
		)
	}
	if strings.Contains(msg, "zoned address") {
		t.Errorf(
			"checkResolvedAddr(%q, allowLoopback=%t) error %q, unzoned control must not use zoned rejection",
			address,
			allowLoopback,
			msg,
		)
	}
	if !strings.Contains(msg, address) {
		t.Errorf(
			"checkResolvedAddr(%q, allowLoopback=%t) error %q, want full dial address",
			address,
			allowLoopback,
			msg,
		)
	}
}

func requireAllowed(t *testing.T, address string, allowLoopback bool) {
	t.Helper()
	err := checkResolvedAddr(address, allowLoopback)
	if err != nil {
		t.Errorf("checkResolvedAddr(%q, allowLoopback=%t) error = %v, want nil", address, allowLoopback, err)
	}
}
