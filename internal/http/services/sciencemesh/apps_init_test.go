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
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func TestAppsInitRejectsInvalidFederationCIDRs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cidrs []string
	}{
		{name: "invalid after valid", cidrs: []string{"10.0.0.0/8", "nope"}},
		{name: "public range", cidrs: []string{"8.8.8.8/32"}},
		{name: "host bits", cidrs: []string{"10.1.2.3/8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &appsHandler{}
			err := h.init(&config{
				ProviderDomain:         "receiver.example",
				AllowedFederationCIDRs: tt.cidrs,
			})
			if err == nil {
				t.Fatal("init succeeded for invalid federation CIDRs")
			}
			if h.launchClient != nil {
				t.Fatal("failed init retained a client")
			}
		})
	}

	t.Run("scalar list", func(t *testing.T) {
		t.Parallel()
		h := &appsHandler{}
		err := cfg.Decode(map[string]any{
			"gatewaysvc":               "grpc:0",
			"mesh_directory_url":       "https://dir.example",
			"provider_domain":          "receiver.example",
			"allowed_federation_cidrs": "10.0.0.0/8",
		}, &config{})
		if err == nil {
			t.Fatal("decode succeeded for scalar allowed_federation_cidrs")
		}
		if h.launchClient != nil {
			t.Fatal("decode failure retained a client")
		}
	})

	t.Run("mixed-type list", func(t *testing.T) {
		t.Parallel()
		h := &appsHandler{}
		err := cfg.Decode(map[string]any{
			"gatewaysvc":               "grpc:0",
			"mesh_directory_url":       "https://dir.example",
			"provider_domain":          "receiver.example",
			"allowed_federation_cidrs": []any{"10.0.0.0/8", 42},
		}, &config{})
		if err == nil {
			t.Fatal("decode succeeded for mixed-type allowed_federation_cidrs")
		}
		if h.launchClient != nil {
			t.Fatal("decode failure retained a client")
		}
	})
}

func TestAppsInitDefaultTransport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setCIDRs reports whether the input map includes the key.
		setCIDRs bool
		cidrs    any
	}{
		{name: "missing cidrs"},
		{name: "empty cidrs", setCIDRs: true, cidrs: []any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := map[string]any{
				"gatewaysvc":         "grpc:0",
				"mesh_directory_url": "https://dir.example",
				"provider_domain":    "receiver.example",
			}
			if tt.setCIDRs {
				input["allowed_federation_cidrs"] = tt.cidrs
			}
			var c config
			if err := cfg.Decode(input, &c); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if c.OCMClientTimeout != 10 {
				t.Fatalf("OCMClientTimeout = %d, want 10", c.OCMClientTimeout)
			}
			h := &appsHandler{}
			if err := h.init(&c); err != nil {
				t.Fatalf("init: %v", err)
			}
			ocmClient, ok := h.launchClient.(*ocmd.OCMClient)
			if !ok || ocmClient == nil {
				t.Fatalf("launchClient = %T, want *ocmd.OCMClient", h.launchClient)
			}
			if c.OCMClientUseEnvProxy {
				t.Fatal("omitted ocm_client_use_env_proxy must stay false")
			}
			assertProxyMode(t, ocmClientTransport(t, ocmClient), false)
			for _, endpoint := range []string{
				"https://192.168.1.50:9",
				"https://127.0.0.1:9",
			} {
				assertLaunchDenied(t, callRetainedDiscovery(t, ocmClient, endpoint), "discovery "+endpoint)
				assertLaunchDenied(t, callRetainedExchange(t, ocmClient, endpoint+"/ocm/token"), "exchange "+endpoint)
			}
		})
	}
}

func TestDecodeOCMClientUseEnvProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		set   bool
		value bool
		want  bool
	}{
		{name: "omitted", want: false},
		{name: "false", set: true, value: false, want: false},
		{name: "true", set: true, value: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := map[string]any{
				"gatewaysvc":         "grpc:0",
				"mesh_directory_url": "https://dir.example",
				"provider_domain":    "receiver.example",
			}
			if tt.set {
				input["ocm_client_use_env_proxy"] = tt.value
			}
			var c config
			if err := cfg.Decode(input, &c); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if c.OCMClientUseEnvProxy != tt.want {
				t.Fatalf("OCMClientUseEnvProxy = %v, want %v", c.OCMClientUseEnvProxy, tt.want)
			}
		})
	}
}

// TestAppsInitUseEnvProxyWiring checks the open-in-app client. Discover and
// ExchangeToken share that one public-only client.
func TestAppsInitUseEnvProxyWiring(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
	}{
		{name: "false", enabled: false},
		{name: "true", enabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &appsHandler{}
			if err := h.init(&config{
				ProviderDomain:       "receiver.example",
				OCMClientUseEnvProxy: tt.enabled,
			}); err != nil {
				t.Fatalf("init: %v", err)
			}
			ocmClient, ok := h.launchClient.(*ocmd.OCMClient)
			if !ok || ocmClient == nil {
				t.Fatalf("launchClient = %T, want *ocmd.OCMClient", h.launchClient)
			}
			assertProxyMode(t, ocmClientTransport(t, ocmClient), tt.enabled)
		})
	}
}

func TestAppsInitPropagatesFederationPolicy(t *testing.T) {
	t.Parallel()

	c := &config{
		ProviderDomain:    "receiver.example",
		OCMClientTimeout:  1,
		OCMClientInsecure: true,
		AllowedFederationCIDRs: []string{
			"10.50.0.0/16",
			"fd42:8c6d:7a10:23::/64",
		},
	}
	h := &appsHandler{}
	if err := h.init(c); err != nil {
		t.Fatalf("init: %v", err)
	}
	ocmClient, ok := h.launchClient.(*ocmd.OCMClient)
	if !ok || ocmClient == nil {
		t.Fatalf("launchClient = %T, want *ocmd.OCMClient", h.launchClient)
	}

	admitted := []string{
		"https://10.50.1.1:9",
		"https://[fd42:8c6d:7a10:23::1]:9",
	}
	for _, endpoint := range admitted {
		assertLaunchAdmitted(t, callRetainedDiscovery(t, ocmClient, endpoint), "discovery "+endpoint)
		assertLaunchAdmitted(t, callRetainedExchange(t, ocmClient, endpoint+"/ocm/token"), "exchange "+endpoint)
	}

	denied := []string{
		"https://10.9.9.9:9",
		"https://192.168.1.1:9",
		"https://[fd00::1]:9",
		"https://127.0.0.1:9",
	}
	for _, endpoint := range denied {
		assertLaunchDenied(t, callRetainedDiscovery(t, ocmClient, endpoint), "discovery "+endpoint)
		assertLaunchDenied(t, callRetainedExchange(t, ocmClient, endpoint+"/ocm/token"), "exchange "+endpoint)
	}

	c.AllowedFederationCIDRs[0] = "192.168.0.0/16"
	c.AllowedFederationCIDRs = append(c.AllowedFederationCIDRs, "fd00::/8")
	if h.launchClient != ocmClient {
		t.Fatal("config mutation replaced the retained client")
	}
	assertLaunchDenied(
		t,
		callRetainedDiscovery(t, ocmClient, "https://192.168.1.1:9"),
		"discovery after config mutation",
	)
	assertLaunchDenied(
		t,
		callRetainedExchange(t, ocmClient, "https://192.168.1.1:9/ocm/token"),
		"exchange after config mutation",
	)
}

func callRetainedDiscovery(t *testing.T, c *ocmd.OCMClient, endpoint string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	_, err := c.Discover(ctx, endpoint)
	return err
}

func callRetainedExchange(t *testing.T, c *ocmd.OCMClient, endpoint string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	_, _, err := c.ExchangeToken(ctx, endpoint, "code", "receiver.example")
	return err
}

func assertLaunchAdmitted(t *testing.T, err error, op string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s error = nil, want a network failure after the guard admits", op)
	}
	if errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("%s = %v, want the guard to admit", op, err)
	}
}

func assertLaunchDenied(t *testing.T, err error, op string) {
	t.Helper()
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("%s = %v, want ErrPolicyViolation", op, err)
	}
}
