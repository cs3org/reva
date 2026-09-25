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

package wellknown

import (
	"slices"
	"strings"
	"testing"
)

func TestInitWithCodeFlowEnabled(t *testing.T) {
	h := &wkocmHandler{}
	h.init(&OcmProviderConfig{
		Endpoint:       "https://cernbox.cern.ch",
		OCMPrefix:      "ocm",
		EnableCodeFlow: true,
	})

	if h.data.TokenEndPoint == "" {
		t.Error("expected tokenEndPoint to be set when code-flow is enabled")
	}
	if h.data.TokenEndPoint != "https://cernbox.cern.ch/ocm/token" {
		t.Errorf("tokenEndPoint: got %s, want https://cernbox.cern.ch/ocm/token", h.data.TokenEndPoint)
	}

	found := slices.Contains(h.data.Capabilities, "exchange-token")
	if !found {
		t.Errorf("expected exchange-token capability, got %v", h.data.Capabilities)
	}
}

func TestInitWithCodeFlowDisabled(t *testing.T) {
	h := &wkocmHandler{}
	h.init(&OcmProviderConfig{
		Endpoint:       "https://cernbox.cern.ch",
		OCMPrefix:      "ocm",
		EnableCodeFlow: false,
	})

	if h.data.TokenEndPoint != "" {
		t.Errorf("expected empty tokenEndPoint when code-flow is disabled, got %s", h.data.TokenEndPoint)
	}

	for _, cap := range h.data.Capabilities {
		if cap == "exchange-token" {
			t.Error("exchange-token capability should not be present when code-flow is disabled")
		}
	}
}

func TestInitWithNoEndpoint(t *testing.T) {
	h := &wkocmHandler{}
	h.init(&OcmProviderConfig{
		EnableCodeFlow: true,
	})

	if h.data.Enabled {
		t.Error("expected discovery to be disabled when no endpoint is configured")
	}
	if h.data.TokenEndPoint != "" {
		t.Errorf("expected empty tokenEndPoint when disabled, got %s", h.data.TokenEndPoint)
	}
}

func TestInitCapabilitiesDoNotDuplicateExchangeToken(t *testing.T) {
	h := &wkocmHandler{}
	h.init(&OcmProviderConfig{
		Endpoint:       "https://cernbox.cern.ch",
		EnableCodeFlow: true,
	})

	count := 0
	for _, cap := range h.data.Capabilities {
		if strings.Contains(cap, "exchange-token") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 exchange-token capability, got %d in %v", count, h.data.Capabilities)
	}
}

func TestWebappReceiveDiscoveryAndLocalTargets(t *testing.T) {
	localWebappMu.Lock()
	prevReady := localWebappReady
	prevTargets := append([]string{}, localWebappTargets...)
	localWebappMu.Unlock()
	t.Cleanup(func() {
		localWebappMu.Lock()
		localWebappReady = prevReady
		localWebappTargets = prevTargets
		localWebappMu.Unlock()
	})

	t.Run("valid base publishes blank", func(t *testing.T) {
		h := &wkocmHandler{}
		h.init(&OcmProviderConfig{
			Endpoint:     "https://cernbox.cern.ch",
			EnableWebapp: true,
		})
		raw, ok := h.data.ResourceTypes[0].Protocols["webapp-receive"].(map[string]any)
		if !ok {
			t.Fatal("webapp-receive missing")
		}
		targets, ok := raw["targets"].([]string)
		if !ok || !slices.Equal(targets, []string{"blank"}) {
			t.Fatalf("advertised %#v", raw["targets"])
		}
		got, ready := LocalWebappReceiveTargets()
		if !ready || !slices.Equal(got, []string{"blank"}) {
			t.Fatalf("published %#v ready %v", got, ready)
		}
		if resolved := ResolveLocalWebappReceiveTargets(nil); !slices.Equal(resolved, []string{"blank"}) {
			t.Fatalf("nil override %#v", resolved)
		}
	})

	t.Run("explicit empty override disables receipt", func(t *testing.T) {
		empty := []string{}
		if resolved := ResolveLocalWebappReceiveTargets(&empty); len(resolved) != 0 {
			t.Fatalf("empty override %#v", resolved)
		}
	})

	t.Run("disabled webapp publishes no targets", func(t *testing.T) {
		h := &wkocmHandler{}
		h.init(&OcmProviderConfig{Endpoint: "https://cernbox.cern.ch"})
		if _, ok := h.data.ResourceTypes[0].Protocols["webapp-receive"]; ok {
			t.Fatal("webapp-receive advertised without targets")
		}
		if _, ok := h.data.ResourceTypes[0].Protocols["webdav"]; !ok {
			t.Fatal("webdav missing")
		}
		got, ready := LocalWebappReceiveTargets()
		if !ready || len(got) != 0 {
			t.Fatalf("published %#v ready %v", got, ready)
		}
	})

	t.Run("padded and double-scheme bases publish nothing", func(t *testing.T) {
		for _, endpoint := range []string{
			" https://cernbox.cern.ch",
			"https://https://cernbox.cern.ch",
			"https://:443",
		} {
			h := &wkocmHandler{}
			h.init(&OcmProviderConfig{Endpoint: endpoint, EnableWebapp: true})
			if _, ok := h.data.ResourceTypes[0].Protocols["webapp-receive"]; ok {
				t.Fatalf("webapp-receive advertised for %q", endpoint)
			}
			got, ready := LocalWebappReceiveTargets()
			if !ready || len(got) != 0 {
				t.Fatalf("published %#v ready %v for %q", got, ready, endpoint)
			}
		}
	})

	t.Run("userinfo base publishes nothing", func(t *testing.T) {
		h := &wkocmHandler{}
		h.init(&OcmProviderConfig{
			Endpoint:     "https://user:pass@cernbox.cern.ch",
			EnableWebapp: true,
		})
		if _, ok := h.data.ResourceTypes[0].Protocols["webapp-receive"]; ok {
			t.Fatal("webapp-receive advertised for a userinfo base")
		}
		got, ready := LocalWebappReceiveTargets()
		if !ready || len(got) != 0 {
			t.Fatalf("published %#v ready %v", got, ready)
		}
	})

	t.Run("unknown local targets return none", func(t *testing.T) {
		localWebappMu.Lock()
		localWebappReady = false
		localWebappTargets = nil
		localWebappMu.Unlock()
		if _, ready := LocalWebappReceiveTargets(); ready {
			t.Fatal("unknown targets reported ready")
		}
		if resolved := ResolveLocalWebappReceiveTargets(nil); len(resolved) != 0 {
			t.Fatalf("resolved %#v", resolved)
		}
	})
}
