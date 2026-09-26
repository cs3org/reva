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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
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

func TestOcmDiscoveryMatrix(t *testing.T) {
	restoreLocalWebapp(t)

	invalid := []string{
		"",
		" https://cernbox.cern.ch",
		"https://user:pass@cernbox.cern.ch",
		"https://https://cernbox.cern.ch",
	}
	for _, endpoint := range invalid {
		h := &wkocmHandler{}
		h.init(&OcmProviderConfig{
			Endpoint:       endpoint,
			EnableWebapp:   true,
			EnableCodeFlow: true,
		})
		assertDiscoveryShape(t, h, discoveryShape{
			enabled: false,
		})
		got, ready := LocalWebappReceiveTargets()
		if !ready || len(got) != 0 {
			t.Fatalf("published %#v ready %v for %q", got, ready, endpoint)
		}
	}

	cases := []struct {
		name     string
		webapp   bool
		codeFlow bool
		shape    discoveryShape
	}{
		{
			name: "webapp off code flow off",
		},
		{
			name:   "webapp on code flow off",
			webapp: true,
			shape:  discoveryShape{receiveBlank: true},
		},
		{
			name:     "webapp off code flow on",
			codeFlow: true,
			shape:    discoveryShape{endpoint: true},
		},
		{
			name:     "webapp on code flow on",
			webapp:   true,
			codeFlow: true,
			shape:    discoveryShape{send: true, receiveBlank: true, endpoint: true},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			h := &wkocmHandler{}
			h.init(&OcmProviderConfig{
				Endpoint:       "https://cernbox.cern.ch",
				EnableWebapp:   tt.webapp,
				EnableCodeFlow: tt.codeFlow,
			})
			tt.shape.enabled = true
			assertDiscoveryShape(t, h, tt.shape)
		})
	}

	t.Run("nil override uses published blank targets", func(t *testing.T) {
		h := &wkocmHandler{}
		h.init(&OcmProviderConfig{
			Endpoint:     "https://cernbox.cern.ch",
			EnableWebapp: true,
		})
		if got := ResolveLocalWebappReceiveTargets(nil); !slices.Equal(got, []string{"blank"}) {
			t.Fatalf("nil override %#v", got)
		}
	})

	t.Run("explicit empty override disables receipt", func(t *testing.T) {
		empty := []string{}
		if got := ResolveLocalWebappReceiveTargets(&empty); len(got) != 0 {
			t.Fatalf("empty override %#v", got)
		}
	})

	t.Run("unpublished local targets resolve empty", func(t *testing.T) {
		localWebappMu.Lock()
		localWebappReady = false
		localWebappTargets = nil
		localWebappMu.Unlock()
		if _, ready := LocalWebappReceiveTargets(); ready {
			t.Fatal("disabled publication reported ready")
		}
		if got := ResolveLocalWebappReceiveTargets(nil); len(got) != 0 {
			t.Fatalf("resolved %#v", got)
		}
	})
}

type discoveryShape struct {
	enabled      bool
	send         bool
	receiveBlank bool
	endpoint     bool
}

func assertDiscoveryShape(t *testing.T, h *wkocmHandler, want discoveryShape) {
	t.Helper()
	if h.data == nil {
		t.Fatal("missing discovery document")
	}
	if h.data.Enabled != want.enabled {
		t.Fatalf("enabled %v", h.data.Enabled)
	}
	if OCMAPIVersion != "1.4.0" {
		t.Fatalf("constant %s", OCMAPIVersion)
	}
	if h.data.APIVersion != OCMAPIVersion {
		t.Fatalf("apiVersion %s", h.data.APIVersion)
	}
	for _, name := range []string{"file", "folder"} {
		protocols, ok := protocolsNamed(h, name)
		if !want.enabled {
			if name == "folder" && !ok {
				continue
			}
			if ok && (hasKey(protocols, "webapp") || hasKey(protocols, "webapp-receive")) {
				t.Fatalf("%s advertised webapp while disabled: %#v", name, protocols)
			}
			continue
		}
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if hasKey(protocols, "webapp") != want.send {
			t.Fatalf("%s send present %v", name, hasKey(protocols, "webapp"))
		}
		raw, receive := protocols["webapp-receive"].(map[string]any)
		if want.receiveBlank {
			targets, ok := raw["targets"].([]string)
			if !receive || !ok || !slices.Equal(targets, []string{"blank"}) {
				t.Fatalf("%s receive %#v", name, protocols["webapp-receive"])
			}
		} else if receive {
			t.Fatalf("%s receive present %#v", name, protocols["webapp-receive"])
		}
	}
	hasToken := slices.Contains(h.data.Capabilities, "exchange-token")
	if want.endpoint {
		if h.data.TokenEndPoint != "https://cernbox.cern.ch/ocm/token" || !hasToken {
			t.Fatalf("endpoint %q caps %v", h.data.TokenEndPoint, h.data.Capabilities)
		}
	} else if h.data.TokenEndPoint != "" || hasToken {
		t.Fatalf("endpoint %q caps %v", h.data.TokenEndPoint, h.data.Capabilities)
	}
	if strings.Contains(h.data.TokenEndPoint, "user:pass") {
		t.Fatalf("token endpoint leaked userinfo")
	}
}

func protocolsNamed(h *wkocmHandler, name string) (map[string]any, bool) {
	for _, rt := range h.data.ResourceTypes {
		if rt.Name == name {
			return rt.Protocols, true
		}
	}
	return nil, false
}

func hasKey(protocols map[string]any, key string) bool {
	if protocols == nil {
		return false
	}
	_, ok := protocols[key]
	return ok
}

func TestOcmDiscoveryUserAgentsShareOneDocument(t *testing.T) {
	restoreLocalWebapp(t)
	h := &wkocmHandler{}
	h.init(&OcmProviderConfig{
		Endpoint:       "https://cernbox.cern.ch",
		EnableWebapp:   true,
		EnableCodeFlow: true,
	})
	before, err := json.Marshal(h.data)
	if err != nil {
		t.Fatal(err)
	}
	agents := []string{
		"curl/8.0.0",
		"Nextcloud Server Crawler",
		"Nextcloud Server Crawler/33.0.0",
		"Nextcloud Server Crawler/34.0.0",
		"Nextcloud Server Crawler/35.0.0",
		"Nextcloud Server Crawler/33.0.0 https://cloud.example",
		"Nextcloud Server Crawler/34.0.0 https://cloud.example",
		"Nextcloud Server Crawler/35.0.0 https://cloud.example",
		"Nextcloud-Server-Crawler/33.0.0",
		"Nextcloud-Server-Crawler/33.0.9",
		"Nextcloud-Server-Crawler/34.0.0",
		"Nextcloud-Server-Crawler/34.0.4",
		"Nextcloud-Server-Crawler/35.0.0",
		"Nextcloud-Server-Crawler/35.0.1",
		"Nextcloud-Server-Crawler/33.0.0; +https://cloud.example",
		"Nextcloud-Server-Crawler/33.0.9; +https://cloud.example",
		"Nextcloud-Server-Crawler/34.0.0; +https://cloud.example",
		"Nextcloud-Server-Crawler/34.0.4; +https://cloud.example",
		"Nextcloud-Server-Crawler/35.0.0; +https://cloud.example",
		"Nextcloud-Server-Crawler/35.0.1; +https://cloud.example",
	}
	bodies := make([][]byte, len(agents))
	var wg sync.WaitGroup
	for i, agent := range agents {
		wg.Add(1)
		go func(i int, agent string) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/.well-known/ocm", nil)
			req.Header.Set("User-Agent", agent)
			rr := httptest.NewRecorder()
			h.Ocm(rr, req)
			bodies[i] = append([]byte(nil), rr.Body.Bytes()...)
		}(i, agent)
	}
	wg.Wait()
	for i, body := range bodies {
		if !bytes.Equal(body, bodies[0]) {
			t.Fatalf("user agent %q changed the document", agents[i])
		}
		if !bytes.Contains(body, []byte(`"apiVersion": "1.4.0"`)) {
			t.Fatalf("user agent %q body %s", agents[i], body)
		}
		if bytes.Contains(body, []byte(`"apiVersion": "1.1"`)) {
			t.Fatalf("crawler override in %s", body)
		}
	}
	after, err := json.Marshal(h.data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("h.data changed\n%s", after)
	}
	if h.data.APIVersion != "1.4.0" {
		t.Fatalf("apiVersion %s", h.data.APIVersion)
	}
}

func restoreLocalWebapp(t *testing.T) {
	t.Helper()
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
}
