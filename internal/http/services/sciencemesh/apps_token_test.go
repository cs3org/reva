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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
)

func TestOpenInAppRelativeTokenEndpoint(t *testing.T) {
	obs := &observeClient{
		token:             launchToken,
		endpointByCall:    []string{"/ocm/token"},
		discoveryEndpoint: "https://sender.example/ocm/",
	}
	share := receivedWebappShare(
		"https://dav.example/remote.php/dav",
		"https://app.example/hub/open?folder=1#lab",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h, _ := newRecordingHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)
	req, _ := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var payload openInAppResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AppURL != "https://app.example/hub/open?folder=1#lab" {
		t.Fatalf("app_url = %q", payload.AppURL)
	}
	if len(obs.tokenURLs) != 1 || obs.tokenURLs[0] != "https://sender.example/ocm/token" {
		t.Fatalf("token url %v", obs.tokenURLs)
	}
	if len(obs.origins) != 1 || obs.origins[0] != "https://dav.example" {
		t.Fatalf("origin %v", obs.origins)
	}
}

func TestResolveTokenEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		disco   *wellknown.OcmDiscoveryData
		want    string
		wantErr string
	}{
		{
			name: "absolute",
			disco: &wellknown.OcmDiscoveryData{
				TokenEndPoint: "https://token.example/ocm/token",
			},
			want: "https://token.example/ocm/token",
		},
		{
			name: "relative against discovery endpoint",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint:      "https://sender.example/ocm",
				TokenEndPoint: "/ocm/token",
			},
			want: "https://sender.example/ocm/token",
		},
		{
			name: "path relative token",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint:      "https://sender.example/ocm",
				TokenEndPoint: "token",
			},
			want: "https://sender.example/token",
		},
		{
			name: "missing host is not relative",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint:      "",
				TokenEndPoint: "https:///token",
			},
			wantErr: "malformed",
		},
		{
			name: "network path is not relative",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint:      "https://sender.example/ocm",
				TokenEndPoint: "//evil.example/token",
			},
			wantErr: "malformed",
		},
		{
			name: "unsupported scheme without host",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint:      "",
				TokenEndPoint: "http:///token",
			},
			wantErr: "malformed",
		},
		{
			name: "missing endpoint",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint: "https://sender.example",
			},
			wantErr: "tokenEndPoint",
		},
		{
			name: "ambiguous relative endpoint",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint:      "",
				TokenEndPoint: "token",
			},
			wantErr: "malformed",
		},
		{
			name: "relative discovery endpoint",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint:      "/ocm",
				TokenEndPoint: "token",
			},
			wantErr: "malformed",
		},
		{
			name: "http absolute endpoint",
			disco: &wellknown.OcmDiscoveryData{
				TokenEndPoint: "http://token.example/ocm/token",
			},
			wantErr: "https",
		},
		{
			name: "userinfo",
			disco: &wellknown.OcmDiscoveryData{
				TokenEndPoint: "https://user:pass@token.example/token",
			},
			wantErr: "malformed",
		},
		{
			name:    "nil discovery",
			disco:   nil,
			wantErr: "missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.disco != nil && tt.disco.Capabilities == nil {
				tt.disco.Capabilities = []string{"exchange-token"}
			}
			got, err := resolveTokenEndpoint(tt.disco)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || got != "" {
					t.Fatalf("got %q err %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}
