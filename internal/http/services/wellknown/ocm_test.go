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

	found := false
	for _, cap := range h.data.Capabilities {
		if cap == "exchange-token" {
			found = true
			break
		}
	}
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
