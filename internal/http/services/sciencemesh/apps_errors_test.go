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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func TestOpenInAppFailures(t *testing.T) {
	secret := launchSecret
	token := launchToken
	validWebapp := func(uri string, reqs []string) *ocmpb.ReceivedShare {
		return receivedWebappShare("https://dav.example/dav", uri, secret, reqs)
	}

	tests := []struct {
		name         string
		file         string
		gw           *fakeReceivedGateway
		client       *observeClient
		domain       string
		wantStatus   int
		wantDiscover int
		wantExchange int
		wantText     string
		forbidURL    string
	}{
		{
			name: "nil response",
			file: "/ocm/share-1",
			gw:   &fakeReceivedGateway{},
			client: &observeClient{
				token: token,
			},
			wantStatus: http.StatusInternalServerError,
			wantText:   "missing share response",
		},
		{
			name: "missing status",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: &ocmpb.GetReceivedOCMShareResponse{
				Share: validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			}},
			client:     &observeClient{token: token},
			wantStatus: http.StatusInternalServerError,
			wantText:   "missing share response",
		},
		{
			name: "missing share",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: &ocmpb.GetReceivedOCMShareResponse{
				Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
			}},
			client:     &observeClient{token: token},
			wantStatus: http.StatusNotFound,
			wantText:   "missing share",
		},
		{
			name: "gateway not found",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: &ocmpb.GetReceivedOCMShareResponse{
				Status: &rpcv1beta1.Status{
					Code:    rpcv1beta1.Code_CODE_NOT_FOUND,
					Message: secret,
				},
			}},
			client:     &observeClient{token: token},
			wantStatus: http.StatusNotFound,
			wantText:   "received share not found",
		},
		{
			name: "gateway permission denied",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: &ocmpb.GetReceivedOCMShareResponse{
				Status: &rpcv1beta1.Status{
					Code:    rpcv1beta1.Code_CODE_PERMISSION_DENIED,
					Message: secret,
				},
			}},
			client:     &observeClient{token: token},
			wantStatus: http.StatusForbidden,
			wantText:   "received share access denied",
		},
		{
			name: "gateway unauthenticated",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: &ocmpb.GetReceivedOCMShareResponse{
				Status: &rpcv1beta1.Status{
					Code:    rpcv1beta1.Code_CODE_UNAUTHENTICATED,
					Message: secret,
				},
			}},
			client:     &observeClient{token: token},
			wantStatus: http.StatusUnauthorized,
			wantText:   "received share unauthenticated",
		},
		{
			name:       "gateway failure",
			file:       "/ocm/share-1",
			gw:         &fakeReceivedGateway{err: errors.New("gateway down " + secret)},
			client:     &observeClient{token: token},
			wantStatus: http.StatusInternalServerError,
			wantText:   "launch failed",
		},
		{
			name: "webdav only",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(&ocmpb.ReceivedShare{
				Protocols: []*ocmpb.Protocol{webdavProtocol("https://dav.example/dav")},
			})},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "webapp protocol",
		},
		{
			name: "missing protocol option",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(&ocmpb.ReceivedShare{
				Protocols: []*ocmpb.Protocol{{
					Term: &ocmpb.Protocol_WebappOptions{},
				}},
			})},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "missing options",
		},
		{
			name:       "missing uri",
			file:       "/ocm/share-1",
			gw:         &fakeReceivedGateway{resp: okShareResponse(validWebapp("  ", []string{"must-exchange-token"}))},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "missing uri",
		},
		{
			name: "missing secret",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(receivedWebappShare(
				"https://dav.example/dav",
				"https://app.example/hub",
				"  ",
				[]string{"must-exchange-token"},
			))},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "sharedSecret",
		},
		{
			name: "absent must-exchange-token",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-use-mfa"}),
			)},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "must-exchange-token",
		},
		{
			name:       "malformed app uri",
			file:       "/ocm/share-1",
			gw:         &fakeReceivedGateway{resp: okShareResponse(validWebapp("https://[", []string{"must-exchange-token"}))},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "malformed",
		},
		{
			name: "discovery error",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client:       &observeClient{discoverErr: errors.New("discovery unavailable " + secret), token: token},
			wantStatus:   http.StatusInternalServerError,
			wantDiscover: 1,
			wantText:     "launch failed",
		},
		{
			name: "missing endpoint",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client: &observeClient{
				token:          token,
				endpointByCall: []string{" "},
			},
			wantStatus:   http.StatusBadRequest,
			wantDiscover: 1,
			wantText:     "tokenEndPoint",
		},
		{
			name: "token error",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client:       &observeClient{exchangeErr: errtypes.PermissionDenied("token exchange was rejected"), token: token},
			wantStatus:   http.StatusForbidden,
			wantDiscover: 1,
			wantExchange: 1,
			wantText:     "permission denied",
		},
		{
			name: "invalid credentials",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client: &observeClient{
				exchangeErr: errtypes.InvalidCredentials("invalid_grant"),
				token:       token,
			},
			wantStatus:   http.StatusUnauthorized,
			wantDiscover: 1,
			wantExchange: 1,
			wantText:     "invalid_grant",
		},
		{
			name: "empty token",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client:       &observeClient{token: "  "},
			wantStatus:   http.StatusInternalServerError,
			wantDiscover: 1,
			wantExchange: 1,
			wantText:     "empty access token",
		},
		{
			name: "empty provider domain",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client:     &observeClient{token: token},
			domain:     " ",
			wantStatus: http.StatusBadRequest,
			wantText:   "provider_domain must not contain whitespace",
		},
		{
			name: "traversal",
			file: "/ocm/share-1/../secret",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "escapes the share",
			forbidURL:  "https://app.example/hub",
		},
		{
			name: "encoded traversal",
			file: "/ocm/share-1/%2e%2e/secret",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "escapes the share",
			forbidURL:  "https://app.example/hub",
		},
		{
			name: "absolute replacement",
			file: "/ocm/share-1/https://evil.example/x",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "invalid file path",
			forbidURL:  "https://app.example/hub",
		},
		{
			name: "ambiguous relative endpoint",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client: &observeClient{
				token:             token,
				endpointByCall:    []string{"token"},
				discoveryEndpoint: "not-a-base",
			},
			wantStatus:   http.StatusBadRequest,
			wantDiscover: 1,
			wantText:     "malformed",
			forbidURL:    "https://app.example/hub",
		},
		{
			name: "duplicate webapp",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(&ocmpb.ReceivedShare{
				Protocols: []*ocmpb.Protocol{
					webappProtocol("https://app.example/one", secret, []string{"must-exchange-token"}),
					webappProtocol("https://app.example/two", secret, []string{"must-exchange-token"}),
				},
			})},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "duplicate webapp",
		},
		{
			name: "relative app uri",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("/apps/open", []string{"must-exchange-token"}),
			)},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "malformed",
		},
		{
			name: "must-use-mfa",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token", "must-use-mfa"}),
			)},
			client:     &observeClient{token: token},
			wantStatus: http.StatusForbidden,
			wantText:   "must-use-mfa cannot be satisfied by this receiver",
		},
		{
			name: "missing exchange-token capability",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-exchange-token"}),
			)},
			client: &observeClient{
				token:        token,
				discoverBare: true,
			},
			wantStatus:   http.StatusBadRequest,
			wantDiscover: 1,
			wantText:     "exchange-token",
		},
		{
			name: "http webdav is not a webapp fallback",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(receivedWebappShare(
				"http://dav.example/dav",
				"https://app.example/hub",
				secret,
				[]string{"must-exchange-token"},
			))},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newRecordingHandler(t, tt.gw, tt.client)
			if tt.domain != "" {
				h.receiverDomain = tt.domain
			}
			req, logs := newLaunchRequest(t, tt.file)
			rec := httptest.NewRecorder()
			h.OpenInApp(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			wantCode := map[int]string{
				http.StatusBadRequest:          "INVALID_PARAMETER",
				http.StatusUnauthorized:        "UNAUTHENTICATED",
				http.StatusForbidden:           "UNTRUSTED_SERVICE",
				http.StatusNotFound:            "RESOURCE_NOT_FOUND",
				http.StatusInternalServerError: "SERVER_ERROR",
			}[tt.wantStatus]
			if !strings.Contains(rec.Body.String(), `"`+wantCode+`"`) {
				t.Fatalf("body %s missing %s", rec.Body.String(), wantCode)
			}
			if !strings.Contains(rec.Body.String(), tt.wantText) {
				t.Fatalf("body %s", rec.Body.String())
			}
			if tt.client.discoverCalls != tt.wantDiscover || tt.client.exchangeCalls != tt.wantExchange {
				t.Fatalf("discover %d exchange %d", tt.client.discoverCalls, tt.client.exchangeCalls)
			}
			if tt.forbidURL != "" && strings.Contains(rec.Body.String(), tt.forbidURL) {
				t.Fatalf("fell back to bare app URL: %s", rec.Body.String())
			}
			assertNotLeaked(t, rec.Body.String(), logs.String(), secret, token)
			if strings.Contains(rec.Body.String(), `"app_url"`) {
				t.Fatalf("error body looked like a launch payload: %s", rec.Body.String())
			}
		})
	}
}

func TestOpenInAppReceivedWebappRequirements(t *testing.T) {
	secret := launchSecret
	blank := []string{"blank"}
	tests := []struct {
		name         string
		targets      []string
		reqs         []string
		receiver     []string
		wantStatus   int
		wantText     string
		discoverZero bool
	}{
		{
			name:       "compatible blank target",
			targets:    blank,
			reqs:       []string{"must-exchange-token"},
			receiver:   blank,
			wantStatus: http.StatusOK,
		},
		{
			name:         "empty targets",
			targets:      nil,
			reqs:         []string{"must-exchange-token"},
			receiver:     blank,
			wantStatus:   http.StatusBadRequest,
			wantText:     "missing targets",
			discoverZero: true,
		},
		{
			name:         "no intersection",
			targets:      []string{"iframe"},
			reqs:         []string{"must-exchange-token"},
			receiver:     blank,
			wantStatus:   http.StatusBadRequest,
			wantText:     "no compatible target",
			discoverZero: true,
		},
		{
			name:         "unsupported target only",
			targets:      []string{"iframe"},
			reqs:         []string{"must-exchange-token"},
			receiver:     []string{"iframe"},
			wantStatus:   http.StatusBadRequest,
			wantText:     "no compatible target",
			discoverZero: true,
		},
		{
			name:       "mixed targets keep blank",
			targets:    []string{"iframe", "blank"},
			reqs:       []string{"must-exchange-token"},
			receiver:   blank,
			wantStatus: http.StatusOK,
		},
		{
			name:         "must-use-mfa fails closed",
			targets:      blank,
			reqs:         []string{"must-exchange-token", "must-use-mfa"},
			receiver:     blank,
			wantStatus:   http.StatusForbidden,
			wantText:     "must-use-mfa cannot be satisfied by this receiver",
			discoverZero: true,
		},
		{
			name:         "unknown must requirement",
			targets:      blank,
			reqs:         []string{"must-exchange-token", "must-sign"},
			receiver:     blank,
			wantStatus:   http.StatusBadRequest,
			wantText:     "unsupported requirement",
			discoverZero: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &observeClient{token: launchToken}
			share := receivedWebappShare(
				"https://dav.example/dav",
				"https://app.example/hub",
				secret,
				tt.reqs,
			)
			share.Protocols[1].GetWebappOptions().Targets = tt.targets
			h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)
			receiver := append([]string{}, tt.receiver...)
			h.webappReceiveTargets = &receiver
			req, _ := newLaunchRequest(t, "/ocm/share-1")
			rec := httptest.NewRecorder()
			h.OpenInApp(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			if tt.wantText != "" && !strings.Contains(rec.Body.String(), tt.wantText) {
				t.Fatalf("body %s", rec.Body.String())
			}
			if tt.discoverZero && (obs.discoverCalls != 0 || obs.exchangeCalls != 0) {
				t.Fatalf("discover %d exchange %d", obs.discoverCalls, obs.exchangeCalls)
			}
			if tt.wantStatus != http.StatusOK && obs.exchangeCalls != 0 {
				t.Fatalf("exchange calls %d", obs.exchangeCalls)
			}
			if tt.wantStatus == http.StatusOK && obs.exchangeCalls != 1 {
				t.Fatalf("exchange calls %d", obs.exchangeCalls)
			}
		})
	}
}
