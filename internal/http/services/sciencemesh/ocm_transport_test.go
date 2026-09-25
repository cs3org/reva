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
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/ocm/client"
)

const invalidFederationCIDRText = "invalid federation CIDR"

func TestPublicOCMTransportConfigScalesTimeoutAndCopiesTLS(t *testing.T) {
	t.Parallel()

	c := &config{
		OCMClientTimeout:       7,
		OCMClientInsecure:      true,
		AllowedFederationCIDRs: []string{"10.1.2.0/24"},
	}
	cfg, err := c.publicOCMTransportConfig()
	if err != nil {
		t.Fatalf("publicOCMTransportConfig: %v", err)
	}
	if cfg.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v, want 7s (integer seconds scaled by time.Second)", cfg.Timeout)
	}
	if !cfg.Insecure {
		t.Error("Insecure = false, want true (copied from service config)")
	}
	if cfg.AllowLoopback {
		t.Error("AllowLoopback must stay false for ScienceMesh public discovery")
	}
	if cfg.UseEnvProxy {
		t.Error("UseEnvProxy must stay false for ScienceMesh public discovery")
	}

	// Exercise the actual constructor wiring: the helper output, fed to the
	// public-only constructor, installs a guarded dialer. An in-range address
	// passes the guard (then fails to connect); an unrelated private range is
	// denied by policy. This does not duplicate H1's normalization rules.
	httpClient := client.NewPublicOnlyHTTPClient(cfg)
	tr, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport: got %T, want *http.Transport", httpClient.Transport)
	}
	if err := dialTransport(tr, "10.1.2.3:9"); errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("in-range 10.1.2.3 denied by policy = %v, want a non-policy dial error", err)
	}
	if err := dialTransport(tr, "192.168.5.9:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("unlisted 192.168.5.9 = %v, want ErrPolicyViolation", err)
	}
}

func TestPublicOCMTransportConfigRejectsInvalidCIDRs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
	}{
		{name: "invalid after valid", values: []string{"10.0.0.0/8", "nope"}},
		{name: "public range", values: []string{"8.8.8.8/32"}},
		{name: "host bits", values: []string{"10.1.2.3/8"}},
		{name: "loopback", values: []string{"127.0.0.0/8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &config{AllowedFederationCIDRs: tt.values}
			_, err := c.publicOCMTransportConfig()
			if err == nil {
				t.Fatalf("publicOCMTransportConfig: expected error for %v", tt.values)
			}
			if !strings.Contains(err.Error(), invalidFederationCIDRText) {
				t.Fatalf("error = %v, want %q in message", err, invalidFederationCIDRText)
			}
		})
	}
}

func TestPublicOCMTransportConfigDefaultDeniesPrivate(t *testing.T) {
	t.Parallel()

	c := &config{}
	cfg, err := c.publicOCMTransportConfig()
	if err != nil {
		t.Fatalf("publicOCMTransportConfig: %v", err)
	}
	if cfg.AllowLoopback || cfg.UseEnvProxy {
		t.Fatal("default policy must keep AllowLoopback and UseEnvProxy false")
	}
	httpClient := client.NewPublicOnlyHTTPClient(cfg)
	tr, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport: got %T, want *http.Transport", httpClient.Transport)
	}
	if err := dialTransport(tr, "192.168.1.50:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("default 192.168.1.50 = %v, want ErrPolicyViolation", err)
	}
	if err := dialTransport(tr, "127.0.0.1:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("default loopback = %v, want ErrPolicyViolation", err)
	}
}

func TestPublicOCMTransportConfigImmutableToConfigMutation(t *testing.T) {
	t.Parallel()

	c := &config{
		OCMClientTimeout:       2,
		AllowedFederationCIDRs: []string{"10.1.2.0/24"},
	}
	cfg, err := c.publicOCMTransportConfig()
	if err != nil {
		t.Fatalf("publicOCMTransportConfig: %v", err)
	}
	httpClient := client.NewPublicOnlyHTTPClient(cfg)
	tr, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport: got %T, want *http.Transport", httpClient.Transport)
	}

	// Mutating the service config after the helper ran must not broaden the
	// built client's policy: 192.168.5.9 stays denied.
	c.AllowedFederationCIDRs[0] = "192.168.5.0/24"
	c.AllowedFederationCIDRs = append(c.AllowedFederationCIDRs, "fd00::/8")
	if err := dialTransport(tr, "192.168.5.9:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("after config mutation 192.168.5.9 = %v, want ErrPolicyViolation", err)
	}

	// Reassigning the runtime config's policy field after the client is built
	// must not affect the already-installed dialer either.
	other, err := client.ParseFederationCIDRs([]string{"192.168.5.0/24"})
	if err != nil {
		t.Fatalf("ParseFederationCIDRs: %v", err)
	}
	cfg.AllowedFederationCIDRs = other
	if err := dialTransport(tr, "192.168.5.9:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("after runtime reassignment 192.168.5.9 = %v, want ErrPolicyViolation", err)
	}
}

// dialTransport invokes the transport's guarded DialContext with a short
// timeout. A denied address returns ErrPolicyViolation; an admitted address
// fails to connect with a non-policy network or timeout error.
func dialTransport(tr *http.Transport, address string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	conn, err := tr.DialContext(ctx, "tcp", address)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}
