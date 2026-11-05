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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSenderDiscoveryOrigin(t *testing.T) {
	tests := []struct {
		name    string
		idp     string
		want    string
		wantErr string
	}{
		{
			name: "bare sender",
			idp:  "sender.example",
			want: "sender.example",
		},
		{
			name: "host and port",
			idp:  "127.0.0.1:443",
			want: "127.0.0.1:443",
		},
		{
			name: "padded sender",
			idp:  " sender.example\t",
			want: "sender.example",
		},
		{
			name:    "empty",
			wantErr: "sender idp",
		},
		{
			name:    "whitespace",
			idp:     " \t",
			wantErr: "sender idp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := senderDiscoveryOrigin(tt.idp)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v", err)
				}
				if got != "" {
					t.Fatalf("origin %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("origin %q want %q", got, tt.want)
			}
		})
	}
}

func TestWebappLaunchDiscoversAtSenderIdp(t *testing.T) {
	obs := &observeClient{token: launchToken}
	share := receivedWebappShare(
		"https://dav.example/remote.php/dav",
		"https://app.example/hub/open",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	share.Creator.Idp = " sender.example "
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)

	req, _ := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)
	launchedForm(t, rec)
	if obs.discoverCalls != 1 || len(obs.origins) != 1 || obs.origins[0] != "sender.example" {
		t.Fatalf("discover %d origins %v", obs.discoverCalls, obs.origins)
	}
}

func TestWebappLaunchSenderDiscoveryWithoutTokenEndpoint(t *testing.T) {
	obs := &observeClient{
		token:          launchToken,
		endpointByCall: []string{""},
	}
	share := receivedWebappShare(
		"https://dav.example/remote.php/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	share.Creator.Idp = "sender.example"
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)

	req, logs := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)
	assertLaunchRefused(t, rec, logs.String(), http.StatusBadRequest, "tokenEndPoint")
	if obs.discoverCalls != 1 || obs.exchangeCalls != 0 {
		t.Fatalf("discover %d exchange %d", obs.discoverCalls, obs.exchangeCalls)
	}
	if len(obs.origins) != 1 || obs.origins[0] != "sender.example" {
		t.Fatalf("origins %v", obs.origins)
	}
}
