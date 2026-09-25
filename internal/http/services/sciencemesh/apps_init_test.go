// Copyright 2018-2024 CERN
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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func scienceMeshServiceConfig(domain string, setDomain bool) map[string]any {
	cfg := map[string]any{
		"gatewaysvc":         "127.0.0.1:19000",
		"mesh_directory_url": "https://mesh.example.test",
	}
	if setDomain {
		cfg["provider_domain"] = domain
	}
	return cfg
}

func TestAppsServiceNewFailsOnAbsentProviderDomain(t *testing.T) {
	svc, err := New(context.Background(), scienceMeshServiceConfig("", false))
	if err == nil || svc != nil {
		t.Fatalf("absent provider_domain: svc=%v err=%v", svc, err)
	}

	svc, err = New(context.Background(), scienceMeshServiceConfig("", true))
	if err == nil || svc != nil {
		t.Fatalf("empty provider_domain: svc=%v err=%v", svc, err)
	}
}

func TestAppsServiceNewFailsOnInvalidProviderDomain(t *testing.T) {
	tests := []struct {
		name   string
		domain string
	}{
		{name: "url", domain: "https://receiver.example.test"},
		{name: "port", domain: "receiver.example.test:443"},
		{name: "ip", domain: "192.0.2.10"},
		{name: "ip and port", domain: "127.0.0.1:54321"},
		{name: "single label", domain: "receiver"},
		{name: "trailing dot", domain: "receiver.example.test."},
		{name: "path", domain: "receiver.example.test/ocm"},
		{name: "query", domain: "receiver.example.test?x=1"},
		{name: "userinfo", domain: "user@receiver.example.test"},
		{name: "non-ascii", domain: "r\u00EBceiver.example.test"},
		{name: "whitespace", domain: "receiver .example.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := New(context.Background(), scienceMeshServiceConfig(tt.domain, true))
			if err == nil || svc != nil {
				t.Fatalf("svc=%v err=%v", svc, err)
			}
		})
	}
}

func TestAppsLaunchRuntimeRejectsInvalidReceiverDomainNoExchange(t *testing.T) {
	const valid = "receiver.example.test"
	svc, err := New(context.Background(), scienceMeshServiceConfig(valid, true))
	if err != nil || svc == nil {
		t.Fatalf("New: svc=%v err=%v", svc, err)
	}

	share := receivedWebappShare(
		"https://dav.example/remote.php/dav/ocm/share",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	installLaunchGateway(t, &fakeReceivedGateway{resp: okShareResponse(share)})

	tests := []struct {
		name   string
		domain string
	}{
		{name: "url", domain: "https://receiver.example.test"},
		{name: "ip", domain: "127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &observeClient{token: launchToken}
			h := &appsHandler{}
			if err := h.init(&config{
				OCMMountPoint:  "/ocm",
				ProviderDomain: valid,
			}); err != nil {
				t.Fatal(err)
			}
			targets := []string{"blank"}
			h.webappReceiveTargets = &targets
			h.receiverDomain = tt.domain
			h.newLaunchClient = func(time.Duration, bool) launchClient {
				return obs
			}

			req, _ := newLaunchRequest(t, "/ocm/share-1")
			rec := httptest.NewRecorder()
			h.OpenInApp(rec, req)
			if rec.Code == http.StatusOK {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			if obs.exchangeCalls != 0 {
				t.Fatalf("exchange calls %d", obs.exchangeCalls)
			}
			if obs.discoverCalls != 0 {
				t.Fatalf("discover calls %d", obs.discoverCalls)
			}
		})
	}
}
