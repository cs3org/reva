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
	"crypto/tls"
	"errors"
	"testing"

	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func TestConfigOCMClientResponseLimit(t *testing.T) {
	tests := []struct {
		name      string
		extra     map[string]any
		wantLimit int64
	}{
		{
			name: "parses configured limit",
			extra: map[string]any{
				"ocm_client_response_limit": 2097152,
			},
			wantLimit: 2097152,
		},
		{
			name:      "defaults when unset",
			extra:     map[string]any{},
			wantLimit: 1 << 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]any{"gatewaysvc": "gateway:9142"}
			for k, v := range tt.extra {
				input[k] = v
			}

			var c config
			if err := cfg.Decode(input, &c); err != nil {
				t.Fatalf("cfg.Decode: %v", err)
			}
			if c.OCMClientResponseLimit != tt.wantLimit {
				t.Errorf("OCMClientResponseLimit = %d, want %d", c.OCMClientResponseLimit, tt.wantLimit)
			}
		})
	}
}

func TestNewWiresTLSMinVersionToSharesTransport(t *testing.T) {
	t.Parallel()

	got, err := New(context.Background(), map[string]any{
		"gatewaysvc":                 "gateway:9142",
		"ocm_client_tls_min_version": "1.3",
		"token_managers": map[string]any{
			"jwt": map[string]any{
				"secret": "test-secret-for-ocm-new",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s, ok := got.(*svc)
	if !ok {
		t.Fatalf("got %T, want *svc", got)
	}
	if s.shares == nil || s.shares.untrustedTransport == nil {
		t.Fatal("shares untrusted transport is nil")
	}
	tr := innerUntrustedTransport(t, s.shares.untrustedTransport)
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("shares handler transport MinVersion = %v, want TLS 1.3", tr.TLSClientConfig)
	}
}

func TestNewRejectsInvalidTLSMinVersion(t *testing.T) {
	t.Parallel()

	_, err := New(context.Background(), map[string]any{
		"gatewaysvc":                 "gateway:9142",
		"ocm_client_tls_min_version": "1.1",
	})
	if err == nil {
		t.Fatal("expected ocmd.New to reject TLS min version 1.1")
	}
	if !errors.Is(err, ErrInvalidTLSMinVersion) {
		t.Fatalf("got %v, want ErrInvalidTLSMinVersion", err)
	}
}

func TestConfigOCMClientTLSMinVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		extra   map[string]any
		want    uint16
		wantErr bool
	}{
		{name: "empty defaults to TLS 1.2", extra: map[string]any{}, want: tls.VersionTLS12},
		{
			name:  "configured 1.3",
			extra: map[string]any{"ocm_client_tls_min_version": "1.3"},
			want:  tls.VersionTLS13,
		},
		{
			name:    "bogus rejected",
			extra:   map[string]any{"ocm_client_tls_min_version": "bogus"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := map[string]any{"gatewaysvc": "gateway:9142"}
			for k, v := range tt.extra {
				input[k] = v
			}

			var c config
			if err := cfg.Decode(input, &c); err != nil {
				t.Fatalf("cfg.Decode: %v", err)
			}
			got, err := ParseTLSMinVersion(c.OCMClientTLSMinVersion)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected invalid TLS min version to fail")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("ParseTLSMinVersion(%q) = %#x, want %#x", c.OCMClientTLSMinVersion, got, tt.want)
			}
			tr := innerUntrustedTransport(t, UntrustedHTTPTransport(
				0,
				false,
				testSec(),
				got,
			))
			if tr.TLSClientConfig.MinVersion != tt.want {
				t.Fatalf("transport MinVersion = %#x, want %#x", tr.TLSClientConfig.MinVersion, tt.want)
			}
		})
	}
}
