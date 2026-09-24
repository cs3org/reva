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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
)

// GetUser must never supply the receiving client_id during a webapp launch.
// The provider FQDN comes from config, not from a gateway user lookup.
func (f *fakeReceivedGateway) GetUser(
	context.Context,
	*userpb.GetUserRequest,
	...grpc.CallOption,
) (*userpb.GetUserResponse, error) {
	if f.onGetUser != nil {
		f.onGetUser()
	}
	return nil, errors.New("GetUser must not supply the receiving client_id")
}

func TestAppsHandlerInitRejectsInvalidProviderDomain(t *testing.T) {
	tests := []struct {
		name   string
		domain string
	}{
		{name: "empty", domain: ""},
		{name: "whitespace", domain: " \t "},
		{name: "url", domain: "https://receiver.example.test"},
		{name: "port", domain: "receiver.example.test:443"},
		{name: "ip", domain: "192.0.2.10"},
		{name: "ip and port", domain: "127.0.0.1:54321"},
		{name: "single label", domain: "receiver"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &appsHandler{}
			err := h.init(&config{
				OCMMountPoint:  "/ocm",
				ProviderDomain: tt.domain,
			})
			if err == nil {
				t.Fatal("expected init error")
			}
		})
	}
}

func TestOpenInAppEmptyHandlerDoesNotExchange(t *testing.T) {
	tests := []struct {
		name   string
		domain string
	}{
		{name: "empty", domain: ""},
		{name: "whitespace", domain: " \t"},
		{name: "url", domain: "https://receiver.example.test"},
		{name: "port", domain: "receiver.example.test:443"},
		{name: "ip", domain: "127.0.0.1"},
		{name: "single label", domain: "receiver"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &observeClient{token: launchToken}
			share := receivedWebappShare(
				"https://dav.example/remote.php/dav/ocm/share",
				"https://app.example/hub",
				launchSecret,
				[]string{"must-exchange-token"},
			)
			share.Grantee = &ocmprovider.Grantee{
				Id: &ocmprovider.Grantee_UserId{
					UserId: &userpb.UserId{
						OpaqueId: "receiver",
						Idp:      "grantee.idp.example",
					},
				},
			}
			gw := &fakeReceivedGateway{
				resp: okShareResponse(share),
				onGetUser: func() {
					t.Fatal("GetUser must not supply the receiving client_id")
				},
			}
			installLaunchGateway(t, gw)
			h := &appsHandler{
				ocmMountPoint:  "/ocm",
				receiverDomain: tt.domain,
				newLaunchClient: func(time.Duration, bool) launchClient {
					return obs
				},
			}
			req, _ := newLaunchRequest(t, "/ocm/share-1")
			req = req.WithContext(appctx.ContextSetUser(req.Context(), &userpb.User{
				Id: &userpb.UserId{OpaqueId: "local-user", Idp: "user.idp.example"},
			}))
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

func TestOpenInAppClientIDIsDecodedProviderDomain(t *testing.T) {
	const want = "Receiver.Example.Test"
	var c config
	err := cfg.Decode(map[string]any{
		"gatewaysvc":          "127.0.0.1:19000",
		"mesh_directory_url":  "https://mesh.example.test",
		"provider_domain":     want,
		"ocm_mount_point":     "/ocm",
		"ocm_client_timeout":  3,
		"ocm_client_insecure": true,
	}, &c)
	if err != nil {
		t.Fatal(err)
	}
	if c.ProviderDomain != want {
		t.Fatalf("decoded provider_domain = %q", c.ProviderDomain)
	}

	tests := []struct {
		name         string
		userIDP      string
		granteeIDP   string
		clearUser    bool
		clearGrantee bool
	}{
		{
			name:       "distinct user and grantee idps",
			userIDP:    "user.idp.example",
			granteeIDP: "grantee.idp.example",
		},
		{
			name:         "absent user and grantee idp",
			clearUser:    true,
			clearGrantee: true,
		},
		{
			name:       "configured domain equals user idp",
			userIDP:    want,
			granteeIDP: "grantee.idp.example",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &observeClient{token: launchToken}
			share := receivedWebappShare(
				"https://dav.example/remote.php/dav/ocm/share",
				"https://app.example/hub",
				launchSecret,
				[]string{"must-exchange-token"},
			)
			if tt.clearGrantee {
				share.Grantee = nil
			} else {
				share.Grantee = &ocmprovider.Grantee{
					Id: &ocmprovider.Grantee_UserId{
						UserId: &userpb.UserId{
							OpaqueId: "receiver",
							Idp:      tt.granteeIDP,
						},
					},
				}
			}
			gw := &fakeReceivedGateway{
				resp: okShareResponse(share),
				onGetUser: func() {
					t.Fatal("GetUser must not supply the receiving client_id")
				},
			}
			installLaunchGateway(t, gw)
			h := &appsHandler{}
			if err := h.init(&c); err != nil {
				t.Fatal(err)
			}
			targets := []string{"blank"}
			h.webappReceiveTargets = &targets
			h.newLaunchClient = func(time.Duration, bool) launchClient {
				return obs
			}

			req, _ := newLaunchRequest(t, "/ocm/share-1")
			if !tt.clearUser {
				req = req.WithContext(appctx.ContextSetUser(req.Context(), &userpb.User{
					Id: &userpb.UserId{OpaqueId: "local-user", Idp: tt.userIDP},
				}))
			}
			rec := httptest.NewRecorder()
			h.OpenInApp(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			if obs.exchangeCalls != 1 {
				t.Fatalf("exchange calls %d", obs.exchangeCalls)
			}
			if len(obs.clientIDs) != 1 || obs.clientIDs[0] != want {
				t.Fatalf("client_id %v", obs.clientIDs)
			}
		})
	}
}

func TestOpenInAppEncodedClientIDUsesProviderDomain(t *testing.T) {
	const want = "receiver.example.test"
	var gotClientID string
	tokenCalls := 0
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/ocm":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":       true,
				"apiVersion":    "1.2.0",
				"endPoint":      srv.URL + "/ocm",
				"provider":      "reva",
				"resourceTypes": []any{},
				"capabilities":  []string{"exchange-token"},
				"tokenEndPoint": srv.URL + "/ocm/token",
			})
		case "/ocm/token":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			gotClientID = r.FormValue("client_id")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "jwt-tok",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	var c config
	if err := cfg.Decode(map[string]any{
		"gatewaysvc":          "127.0.0.1:19000",
		"mesh_directory_url":  "https://mesh.example.test",
		"provider_domain":     want,
		"ocm_mount_point":     "/ocm",
		"ocm_client_timeout":  3,
		"ocm_client_insecure": true,
	}, &c); err != nil {
		t.Fatal(err)
	}
	share := receivedWebappShare(
		srv.URL+"/remote.php/dav/ocm/share",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	installLaunchGateway(t, &fakeReceivedGateway{resp: okShareResponse(share)})
	h := &appsHandler{}
	if err := h.init(&c); err != nil {
		t.Fatal(err)
	}
	targets := []string{"blank"}
	h.webappReceiveTargets = &targets
	// NewPublicOnlyClient refuses loopback. This client is the real form encoder.
	h.newLaunchClient = func(timeout time.Duration, insecure bool) launchClient {
		if !insecure {
			t.Fatal("expected TLS-insecure launch client")
		}
		return ocmd.NewClient(timeout, insecure)
	}

	req, _ := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if tokenCalls != 1 {
		t.Fatalf("token calls = %d, want 1", tokenCalls)
	}
	if gotClientID == "" {
		t.Fatal("encoded client_id is empty")
	}
	if gotClientID != want {
		t.Fatalf("client_id = %q, want %q", gotClientID, want)
	}
}
