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

package sciencemesh

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func requiredScienceMeshConfig() map[string]any {
	return map[string]any{
		"gatewaysvc":         "gateway:9142",
		"mesh_directory_url": "https://mesh.example",
		"provider_domain":    "example.org",
	}
}

func TestNewRejectsHatch(t *testing.T) {
	t.Parallel()

	t.Run("allow_http", func(t *testing.T) {
		m := requiredScienceMeshConfig()
		m["ocm_client_security"] = map[string]any{"allow_http": true}
		_, err := New(context.Background(), m)
		if err == nil {
			t.Fatal("expected sciencemesh.New to reject allow_http")
		}
		if !errors.Is(err, ocmd.ErrHatchAllowHTTP) {
			t.Fatalf("got %v, want ocmd.ErrHatchAllowHTTP", err)
		}
	})

	t.Run("allowed_cidrs", func(t *testing.T) {
		m := requiredScienceMeshConfig()
		m["ocm_client_security"] = map[string]any{"allowed_cidrs": []string{"10.0.0.0/8"}}
		_, err := New(context.Background(), m)
		if err == nil {
			t.Fatal("expected sciencemesh.New to reject allowed_cidrs")
		}
		if !errors.Is(err, ocmd.ErrHatchAllowedCIDRs) {
			t.Fatalf("got %v, want ocmd.ErrHatchAllowedCIDRs", err)
		}
	})
}

func TestConfigOCMClientLimits(t *testing.T) {
	tests := []struct {
		name            string
		extra           map[string]any
		wantLimit       int
		wantTimeout     int
		wantDialTimeout int
	}{
		{
			name: "parses flat keys",
			extra: map[string]any{
				"ocm_client_response_limit": 2097152,
				"ocm_client_timeout":        45,
				"ocm_client_dial_timeout":   2,
			},
			wantLimit:       2097152,
			wantTimeout:     45,
			wantDialTimeout: 2,
		},
		{
			name:            "defaults when unset",
			extra:           map[string]any{},
			wantLimit:       1 << 20,
			wantTimeout:     10,
			wantDialTimeout: 10,
		},
		{
			name: "negative timeouts use defaults",
			extra: map[string]any{
				"ocm_client_timeout":      -1,
				"ocm_client_dial_timeout": -3,
			},
			wantLimit:       1 << 20,
			wantTimeout:     10,
			wantDialTimeout: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := requiredScienceMeshConfig()
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
			if c.OCMClientTimeout != tt.wantTimeout {
				t.Errorf("OCMClientTimeout = %d, want %d", c.OCMClientTimeout, tt.wantTimeout)
			}
			if c.OCMClientDialTimeout != tt.wantDialTimeout {
				t.Errorf("OCMClientDialTimeout = %d, want %d", c.OCMClientDialTimeout, tt.wantDialTimeout)
			}
		})
	}
}

func TestNewWiresTLSMinVersionToDiscoverTransport(t *testing.T) {
	t.Parallel()

	m := requiredScienceMeshConfig()
	m["ocm_client_tls_min_version"] = "1.3"
	got, err := New(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := got.(*svc)
	if !ok {
		t.Fatalf("got %T, want *svc", got)
	}
	if s.wayf == nil || s.wayf.untrustedClient == nil {
		t.Fatal("untrusted discover client is nil")
	}
	tr := innerUntrustedHTTPTransport(t, s.wayf.untrustedClient.Transport)
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("discover transport MinVersion = %v, want TLS 1.3", tr.TLSClientConfig)
	}
}

func TestNewRejectsInvalidTLSMinVersion(t *testing.T) {
	t.Parallel()

	m := requiredScienceMeshConfig()
	m["ocm_client_tls_min_version"] = "1.1"
	_, err := New(context.Background(), m)
	if err == nil {
		t.Fatal("expected sciencemesh.New to reject TLS min version 1.1")
	}
	if !errors.Is(err, ocmd.ErrInvalidTLSMinVersion) {
		t.Fatalf("got %v, want ocmd.ErrInvalidTLSMinVersion", err)
	}
}

func TestConfigOCMClientTLSMinVersion(t *testing.T) {
	t.Parallel()

	input := requiredScienceMeshConfig()
	input["ocm_client_tls_min_version"] = "1.3"
	var c config
	if err := cfg.Decode(input, &c); err != nil {
		t.Fatalf("cfg.Decode: %v", err)
	}
	got, err := ocmd.ParseTLSMinVersion(c.OCMClientTLSMinVersion)
	if err != nil {
		t.Fatal(err)
	}
	if got != tls.VersionTLS13 {
		t.Fatalf("ParseTLSMinVersion(%q) = %#x, want TLS 1.3", c.OCMClientTLSMinVersion, got)
	}
}
