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
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
)

func testSec() UntrustedClientSecurity {
	return defaultUntrustedSecurity()
}

func TestUntrustedClientSecurityApplyDefaults(t *testing.T) {
	t.Parallel()

	t.Run("zero becomes three", func(t *testing.T) {
		s := UntrustedClientSecurity{}
		s.ApplyDefaults()
		if s.MaxRedirects != 3 {
			t.Fatalf("MaxRedirects = %d, want 3", s.MaxRedirects)
		}
	})

	t.Run("values above 10 clamp to 10", func(t *testing.T) {
		s := UntrustedClientSecurity{MaxRedirects: 11}
		s.ApplyDefaults()
		if s.MaxRedirects != 10 {
			t.Fatalf("MaxRedirects = %d, want 10", s.MaxRedirects)
		}
	})

	t.Run("explicit value in range is kept", func(t *testing.T) {
		s := UntrustedClientSecurity{MaxRedirects: 7}
		s.ApplyDefaults()
		if s.MaxRedirects != 7 {
			t.Fatalf("MaxRedirects = %d, want 7", s.MaxRedirects)
		}
	})
}

func TestUntrustedClientSecurityCompile(t *testing.T) {
	t.Parallel()

	t.Run("invalid CIDR", func(t *testing.T) {
		s := UntrustedClientSecurity{AllowedCIDRs: []string{"not-a-cidr"}}
		if err := s.Compile(); err == nil {
			t.Fatal("expected invalid CIDR error")
		}
	})

	t.Run("rejects 0.0.0.0/0", func(t *testing.T) {
		s := UntrustedClientSecurity{AllowedCIDRs: []string{"0.0.0.0/0"}}
		if err := s.Compile(); err == nil {
			t.Fatal("expected unrestricted IPv4 CIDR error")
		}
	})

	t.Run("rejects ::/0", func(t *testing.T) {
		s := UntrustedClientSecurity{AllowedCIDRs: []string{"::/0"}}
		if err := s.Compile(); err == nil {
			t.Fatal("expected unrestricted IPv6 CIDR error")
		}
	})

	t.Run("rejects NAT64 overlap", func(t *testing.T) {
		s := UntrustedClientSecurity{AllowedCIDRs: []string{"64:ff9b::/96"}}
		if err := s.Compile(); err == nil {
			t.Fatal("expected NAT64 overlap error")
		}
	})

	t.Run("returns an error for every rejection", func(t *testing.T) {
		s := UntrustedClientSecurity{AllowedCIDRs: []string{"not-a-cidr", "0.0.0.0/0", "::/0"}}
		err := s.Compile()
		if err == nil {
			t.Fatal("expected joined compile errors")
		}
		if !errors.Is(err, errInvalidCIDR) {
			t.Fatalf("joined error missing invalid CIDR: %v", err)
		}
		if !errors.Is(err, errUnrestrictedCIDR) {
			t.Fatalf("joined error missing unrestricted CIDR: %v", err)
		}
	})

	t.Run("clears stale compiled nets after failure", func(t *testing.T) {
		s := UntrustedClientSecurity{AllowedCIDRs: []string{"10.0.0.0/8"}}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if !s.allowsDial(net.ParseIP("10.1.2.3")) {
			t.Fatal("expected PIN allow before failed recompile")
		}
		s.AllowedCIDRs = []string{"0.0.0.0/0"}
		if err := s.Compile(); err == nil {
			t.Fatal("expected recompile to fail")
		}
		if s.allowsDial(net.ParseIP("10.1.2.3")) {
			t.Fatal("stale compiled nets must not survive a failed Compile")
		}
	})
}

func TestUntrustedClientSecurityAllowsDialPIN(t *testing.T) {
	t.Parallel()

	t.Run("empty hatch is public-only", func(t *testing.T) {
		s := testSec()
		if !s.allowsDial(net.ParseIP("8.8.8.8")) {
			t.Fatal("empty hatch must allow a public IP")
		}
		if s.allowsDial(net.ParseIP("10.1.2.3")) {
			t.Fatal("empty hatch must refuse a private IP")
		}
	})

	t.Run("non-empty PIN allows listed only", func(t *testing.T) {
		s := UntrustedClientSecurity{AllowedCIDRs: []string{"10.0.0.0/8"}}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if !s.allowsDial(net.ParseIP("10.1.2.3")) {
			t.Fatal("PIN must allow an address in the listed net")
		}
		if s.allowsDial(net.ParseIP("8.8.8.8")) {
			t.Fatal("PIN must refuse a public IP that is not listed")
		}
	})

	t.Run("link-local denied unless exact /32", func(t *testing.T) {
		s := UntrustedClientSecurity{AllowedCIDRs: []string{"169.254.0.0/16"}}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if s.allowsDial(net.ParseIP("169.254.169.254")) {
			t.Fatal("listing 169.254.0.0/16 must not allow 169.254.169.254")
		}
		if s.allowsDial(net.ParseIP("169.254.1.1")) {
			t.Fatal("link-local must stay denied without an exact host entry")
		}

		s = UntrustedClientSecurity{AllowedCIDRs: []string{"169.254.169.254/32"}}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if !s.allowsDial(net.ParseIP("169.254.169.254")) {
			t.Fatal("exact /32 must allow the metadata host")
		}
		if s.allowsDial(net.ParseIP("169.254.1.1")) {
			t.Fatal("exact /32 must not allow a different link-local host")
		}
	})

	t.Run("ipv6 metadata and nat64 denied unless exact host", func(t *testing.T) {
		meta6 := net.ParseIP(cloudMetadataAWSIPv6)
		s := UntrustedClientSecurity{AllowedCIDRs: []string{"fd00:ec2::/64"}}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if s.allowsDial(meta6) {
			t.Fatal("listing fd00:ec2::/64 must not allow fd00:ec2::254")
		}
		if !s.allowsDial(net.ParseIP("fd00:ec2::1")) {
			t.Fatal("PIN fd00:ec2::/64 must still allow a non-metadata address")
		}

		s = UntrustedClientSecurity{AllowedCIDRs: []string{"fd00:ec2::254/128"}}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if !s.allowsDial(meta6) {
			t.Fatal("exact /128 must allow the IPv6 metadata host")
		}
		if s.allowsDial(net.ParseIP("fd00:ec2::1")) {
			t.Fatal("exact /128 must not allow a different IPv6 host")
		}

		nat64Meta := net.ParseIP("64:ff9b::a9fe:a9fe")
		s = UntrustedClientSecurity{AllowedCIDRs: []string{"169.254.0.0/16"}}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if s.allowsDial(nat64Meta) {
			t.Fatal("listing 169.254.0.0/16 must not allow NAT64 metadata")
		}

		s = UntrustedClientSecurity{AllowedCIDRs: []string{"169.254.169.254/32"}}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if !s.allowsDial(nat64Meta) {
			t.Fatal("exact IPv4 /32 must allow the NAT64-wrapped metadata host")
		}
	})
}

func TestUntrustedClientSecurityCheckRedirect(t *testing.T) {
	t.Parallel()

	s := testSec()
	httpsURL, err := url.Parse("https://example.com/next")
	if err != nil {
		t.Fatal(err)
	}
	req := &http.Request{URL: httpsURL}

	via3 := []*http.Request{{}, {}, {}}
	if err := s.CheckRedirect(req, via3); err != nil {
		t.Fatalf("len(via)=3 must be allowed with max 3: %v", err)
	}

	via4 := []*http.Request{{}, {}, {}, {}}
	if err := s.CheckRedirect(req, via4); err == nil {
		t.Fatal("len(via)=4 must be refused with max 3")
	} else if !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("got %v, want ErrTooManyRedirects", err)
	}
}

func TestUntrustedClientSecurityRequireScheme(t *testing.T) {
	t.Parallel()

	httpsURL, err := url.Parse("https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	httpURL, err := url.Parse("http://example.com/")
	if err != nil {
		t.Fatal(err)
	}

	s := testSec()
	if err := s.requireScheme(httpsURL); err != nil {
		t.Fatalf("https must be allowed: %v", err)
	}
	if err := s.requireScheme(httpURL); err == nil {
		t.Fatal("http must be refused when AllowHTTP is false")
	} else if !errors.Is(err, ErrNonHTTPS) {
		t.Fatalf("got %v, want ErrNonHTTPS", err)
	}

	s.AllowHTTP = true
	if err := s.requireScheme(httpURL); err != nil {
		t.Fatalf("http must be allowed when AllowHTTP is true: %v", err)
	}
}

func TestUntrustedClientSecurityRejectHatch(t *testing.T) {
	t.Parallel()

	if err := (UntrustedClientSecurity{}).RejectHatch(); err != nil {
		t.Fatalf("empty hatch must be accepted: %v", err)
	}
	if err := (UntrustedClientSecurity{AllowHTTP: true}).RejectHatch(); err == nil {
		t.Fatal("AllowHTTP must be rejected for non-received consumers")
	} else if !errors.Is(err, ErrHatchAllowHTTP) {
		t.Fatalf("got %v, want ErrHatchAllowHTTP", err)
	}
	if err := (UntrustedClientSecurity{AllowedCIDRs: []string{"10.0.0.0/8"}}).RejectHatch(); err == nil {
		t.Fatal("AllowedCIDRs must be rejected for non-received consumers")
	} else if !errors.Is(err, ErrHatchAllowedCIDRs) {
		t.Fatalf("got %v, want ErrHatchAllowedCIDRs", err)
	}
}

func TestOCMDNewRejectsHatch(t *testing.T) {
	t.Parallel()

	t.Run("allow_http", func(t *testing.T) {
		_, err := New(context.Background(), map[string]any{
			"gatewaysvc": "gateway:9142",
			"ocm_client_security": map[string]any{
				"allow_http": true,
			},
		})
		if err == nil {
			t.Fatal("expected ocmd.New to reject allow_http")
		}
		if !errors.Is(err, ErrHatchAllowHTTP) {
			t.Fatalf("got %v, want ErrHatchAllowHTTP", err)
		}
	})

	t.Run("allowed_cidrs", func(t *testing.T) {
		_, err := New(context.Background(), map[string]any{
			"gatewaysvc": "gateway:9142",
			"ocm_client_security": map[string]any{
				"allowed_cidrs": []string{"10.0.0.0/8"},
			},
		})
		if err == nil {
			t.Fatal("expected ocmd.New to reject allowed_cidrs")
		}
		if !errors.Is(err, ErrHatchAllowedCIDRs) {
			t.Fatalf("got %v, want ErrHatchAllowedCIDRs", err)
		}
	})
}
