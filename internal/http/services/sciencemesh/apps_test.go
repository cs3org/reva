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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type launchCtxKey struct{}

const (
	launchSecret        = "share-secret-value"
	launchToken         = "access-token-value"
	launchCtxVal        = "launch-ctx"
	launchAuthorization = "Bearer launch-auth-token"
)

func TestOpenInAppValidWebapp(t *testing.T) {
	obs := &observeClient{token: launchToken}
	share := receivedWebappShare(
		"https://dav.example/remote.php/dav/ocm/share",
		"https://app.example/hub/open?folder=1",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	share.Grantee = &ocmprovider.Grantee{
		Id: &ocmprovider.Grantee_UserId{
			UserId: &userpb.UserId{OpaqueId: "receiver", Idp: "idp.example"},
		},
	}
	gw := &fakeReceivedGateway{resp: okShareResponse(share)}
	h, seen := newRecordingHandler(t, gw, obs)

	req, logs := newLaunchRequest(t, "/ocm/share-1/dir/my file.txt")
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type %q", rec.Header().Get("Content-Type"))
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("payload keys = %#v", payload)
	}
	const wantURL = "https://app.example/hub/open/dir/my%20file.txt?folder=1"
	if payload["app_url"] != wantURL {
		t.Fatalf("app_url = %#v", payload["app_url"])
	}
	if payload["access_token"] != launchToken {
		t.Fatalf("access_token = %#v", payload["access_token"])
	}
	if strings.Contains(rec.Body.String(), launchSecret) {
		t.Fatal("response included the shared secret")
	}
	assertLogsClean(t, logs.String(), launchSecret, launchToken)
	if gw.opaqueID != "share-1" {
		t.Fatalf("looked up share %q", gw.opaqueID)
	}
	if obs.discoverCalls != 1 || obs.exchangeCalls != 1 {
		t.Fatalf("discover %d exchange %d", obs.discoverCalls, obs.exchangeCalls)
	}
	if len(obs.origins) != 1 || obs.origins[0] != "https://dav.example" {
		t.Fatalf("discovery origin %v", obs.origins)
	}
	if len(obs.clientIDs) != 1 || obs.clientIDs[0] != "receiver.example" {
		t.Fatalf("client_id %v", obs.clientIDs)
	}
	if len(obs.codes) != 1 || obs.codes[0] != launchSecret {
		t.Fatal("exchange did not use the shared secret as the code")
	}
	if obs.deadlines != 2 || obs.missingCtxValue {
		t.Fatalf("deadlines %d missing ctx %v", obs.deadlines, obs.missingCtxValue)
	}
	if seen.calls != 1 || seen.timeout != 3*time.Second || !seen.insecure {
		t.Fatalf("client init calls=%d timeout=%s insecure=%v", seen.calls, seen.timeout, seen.insecure)
	}
	if len(gw.contexts) != 1 {
		t.Fatalf("gateway calls %d", len(gw.contexts))
	}
	gwCtx := gw.contexts[0]
	if gwCtx.Value(launchCtxKey{}) != launchCtxVal {
		t.Fatal("gateway context lost the launch value")
	}
	md, ok := metadata.FromIncomingContext(gwCtx)
	if !ok {
		t.Fatal("gateway context has no incoming metadata")
	}
	if got := md.Get("authorization"); len(got) != 1 || got[0] != launchAuthorization {
		t.Fatalf("authorization metadata %v", got)
	}
}

func TestOpenInAppRootLaunchKeepsBareOpener(t *testing.T) {
	obs := &observeClient{token: launchToken}
	share := receivedWebappShare(
		"https://dav.example/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h, _ := newRecordingHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)

	req, _ := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)

	var payload openInAppResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("status %d decode %v body %s", rec.Code, err, rec.Body.String())
	}
	if payload.AppURL != "https://app.example/hub" {
		t.Fatalf("app_url = %q", payload.AppURL)
	}
	if strings.Contains(payload.AppURL, "share-1") || strings.Contains(payload.AppURL, "/lab") {
		t.Fatalf("root launch was rewritten: %s", payload.AppURL)
	}
}

func TestOpenInAppRepeatedLaunchDiscoversIndependently(t *testing.T) {
	obs := &observeClient{
		endpointByCall: []string{
			"https://token-a.example/ocm/token",
			"https://token-b.example/ocm/token",
		},
		tokenByCall: []string{"token-a", "token-b"},
	}
	share := receivedWebappShare(
		"https://dav.example/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h, seen := newRecordingHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)

	var urls []string
	for range 2 {
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
		urls = append(urls, payload.AccessToken)
	}
	if obs.discoverCalls != 2 || seen.calls != 2 {
		t.Fatalf("discover %d client builds %d", obs.discoverCalls, seen.calls)
	}
	if len(obs.tokenURLs) != 2 || obs.tokenURLs[0] != obs.endpointByCall[0] || obs.tokenURLs[1] != obs.endpointByCall[1] {
		t.Fatalf("exchanged endpoints %v", obs.tokenURLs)
	}
	if urls[0] != "token-a" || urls[1] != "token-b" {
		t.Fatalf("tokens %v", urls)
	}
}

func TestAppsHandlerHasNoLaunchCache(t *testing.T) {
	typ := reflect.TypeOf(appsHandler{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		desc := strings.ToLower(field.Name + " " + field.Type.String())
		if strings.Contains(desc, "cache") {
			t.Fatalf("launch handler has cache field %s", field.Name)
		}
	}
}

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
					Message: "share not found",
				},
			}},
			client:     &observeClient{token: token},
			wantStatus: http.StatusNotFound,
			wantText:   "share not found",
		},
		{
			name:       "gateway failure",
			file:       "/ocm/share-1",
			gw:         &fakeReceivedGateway{err: errors.New("gateway down")},
			client:     &observeClient{token: token},
			wantStatus: http.StatusInternalServerError,
			wantText:   "gateway down",
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
			wantText:   "shared secret",
		},
		{
			name: "absent must-exchange-token",
			file: "/ocm/share-1",
			gw: &fakeReceivedGateway{resp: okShareResponse(
				validWebapp("https://app.example/hub", []string{"must-invite"}),
			)},
			client:     &observeClient{token: token},
			wantStatus: http.StatusBadRequest,
			wantText:   "token exchange",
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
			client:       &observeClient{discoverErr: errors.New("discovery unavailable"), token: token},
			wantStatus:   http.StatusInternalServerError,
			wantDiscover: 1,
			wantText:     "discovery unavailable",
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
			wantStatus:   http.StatusNotFound,
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
			wantText:   "provider domain",
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
			wantText:     "absolute base",
			forbidURL:    "https://app.example/hub",
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
			if rec.Code == http.StatusOK {
				t.Fatal("failure committed HTTP 200")
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

func TestOpenInAppSensitiveRemoteErrorIsRedacted(t *testing.T) {
	obs := &observeClient{
		token:       launchToken,
		exchangeErr: fmt.Errorf("rejected %s %s", launchSecret, launchToken),
	}
	share := receivedWebappShare(
		"https://dav.example/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h, _ := newRecordingHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)
	req, logs := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("sensitive error committed HTTP 200")
	}
	assertNotLeaked(t, rec.Body.String(), logs.String(), launchSecret, launchToken)
}

func TestOpenInAppCanceledContext(t *testing.T) {
	obs := &observeClient{token: launchToken}
	share := receivedWebappShare(
		"https://dav.example/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h, _ := newRecordingHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)
	req, _ := newLaunchRequest(t, "/ocm/share-1")
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("canceled launch succeeded")
	}
	if obs.discoverCalls != 1 || obs.exchangeCalls != 0 {
		t.Fatalf("discover %d exchange %d", obs.discoverCalls, obs.exchangeCalls)
	}
	if obs.missingCtxValue {
		t.Fatal("discovery did not receive the request context")
	}
}

func TestOpenInAppRelativeDiscoveryResolution(t *testing.T) {
	obs := &observeClient{
		token:             launchToken,
		endpointByCall:    []string{"/ocm/token"},
		discoveryEndpoint: "https://sender.example/ocm",
	}
	share := receivedWebappShare(
		"https://dav.example/remote.php/dav",
		"/apps/open",
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
	if payload.AppURL != "https://sender.example/apps/open" {
		t.Fatalf("app_url = %q", payload.AppURL)
	}
	if len(obs.tokenURLs) != 1 || obs.tokenURLs[0] != "https://sender.example/ocm/token" {
		t.Fatalf("token url %v", obs.tokenURLs)
	}
	if len(obs.origins) != 1 || obs.origins[0] != "https://dav.example" {
		t.Fatalf("origin %v", obs.origins)
	}
}

func TestOpenInAppPublicOnlyRejectsBeforeContact(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var accepted atomic.Int32
	go func() {
		for {
			conn, accErr := ln.Accept()
			if accErr != nil {
				return
			}
			accepted.Add(1)
			_ = conn.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	tests := []struct {
		name       string
		webdav     string
		insecure   bool
		wantRefuse bool
		wantScheme bool
	}{
		{
			name:       "loopback discovery",
			webdav:     fmt.Sprintf("https://127.0.0.1:%d/dav", port),
			insecure:   false,
			wantRefuse: true,
		},
		{
			name:       "loopback discovery with insecure tls",
			webdav:     fmt.Sprintf("https://127.0.0.1:%d/dav", port),
			insecure:   true,
			wantRefuse: true,
		},
		{
			name:       "http loopback is scheme rejection",
			webdav:     fmt.Sprintf("http://127.0.0.1:%d/dav", port),
			wantScheme: true,
		},
		{
			name:       "private discovery",
			webdav:     "https://10.2.3.4/dav",
			wantRefuse: true,
		},
		{
			name:       "carrier grade nat discovery",
			webdav:     "https://100.64.0.1/dav",
			wantRefuse: true,
		},
		{
			name:       "link-local discovery",
			webdav:     "https://169.254.169.254/dav",
			wantRefuse: true,
		},
		{
			name:       "ipv6 link-local discovery",
			webdav:     "https://[fe80::1]/dav",
			wantRefuse: true,
		},
		{
			name:       "ipv6 loopback discovery",
			webdav:     "https://[::1]/dav",
			wantRefuse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := accepted.Load()
			share := receivedWebappShare(
				tt.webdav,
				"https://app.example/hub",
				launchSecret,
				[]string{"must-exchange-token"},
			)
			h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, nil)
			h.clientTimeout = time.Second
			h.clientInsecure = tt.insecure
			req, logs := newLaunchRequest(t, "/ocm/share-1")
			rec := httptest.NewRecorder()
			h.OpenInApp(rec, req)
			time.Sleep(30 * time.Millisecond)

			if rec.Code == http.StatusOK {
				t.Fatalf("non-public launch succeeded: %s", rec.Body.String())
			}
			body := rec.Body.String()
			// Discover replaces a refused dial with a generic discovery error.
			// The dial guard is visible in the request log, before any accept.
			if tt.wantRefuse {
				if !strings.Contains(logs.String(), "non-public") && !strings.Contains(logs.String(), "refusing") {
					t.Fatalf("dial guard did not refuse: body %s log %s", body, logs.String())
				}
			}
			if tt.wantScheme {
				if strings.Contains(body, "refusing") || strings.Contains(body, "non-public") ||
					strings.Contains(logs.String(), "refusing") || strings.Contains(logs.String(), "non-public") {
					t.Fatal("scheme rejection was reported as address rejection")
				}
				if !strings.Contains(body, "https") {
					t.Fatalf("body %s", body)
				}
			}
			if accepted.Load() != before {
				t.Fatalf("destination accepted %d connections", accepted.Load()-before)
			}
			assertNotLeaked(t, body, logs.String(), launchSecret, launchToken)
		})
	}
}

func TestOpenInAppPublicDiscoveryPrivateToken(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var accepted atomic.Int32
	go func() {
		for {
			conn, accErr := ln.Accept()
			if accErr != nil {
				return
			}
			accepted.Add(1)
			_ = conn.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	tokenURL := fmt.Sprintf("https://127.0.0.1:%d/ocm/token", port)

	share := receivedWebappShare(
		"https://192.0.2.1/remote.php/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, nil)
	h.clientTimeout = time.Second
	h.clientInsecure = true
	var gotTimeout time.Duration
	var gotInsecure bool
	h.newLaunchClient = func(timeout time.Duration, insecure bool) launchClient {
		gotTimeout = timeout
		gotInsecure = insecure
		return &publicOnlyScript{
			real:          ocmd.NewPublicOnlyClient(timeout, insecure),
			tokenEndpoint: tokenURL,
		}
	}

	req, logs := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)
	time.Sleep(30 * time.Millisecond)

	if rec.Code == http.StatusOK {
		t.Fatalf("private token hop succeeded: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "non-public") && !strings.Contains(rec.Body.String(), "refusing") {
		t.Fatalf("body %s", rec.Body.String())
	}
	if accepted.Load() != 0 {
		t.Fatalf("token endpoint accepted %d connections", accepted.Load())
	}
	if gotTimeout != time.Second || !gotInsecure {
		t.Fatalf("timeout %s insecure %v", gotTimeout, gotInsecure)
	}
	assertNotLeaked(t, rec.Body.String(), logs.String(), launchSecret, launchToken)
}

func TestOpenInAppPublicOnlyScriptRefusesPrivateDiscovery(t *testing.T) {
	share := receivedWebappShare(
		"https://10.9.8.7/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, nil)
	h.clientTimeout = time.Second
	h.clientInsecure = false
	script := &publicOnlyScript{
		real:          ocmd.NewPublicOnlyClient(time.Second, false),
		tokenEndpoint: "https://token.example/ocm/token",
	}
	h.newLaunchClient = func(timeout time.Duration, insecure bool) launchClient {
		if timeout != time.Second || insecure {
			t.Errorf("timeout %s insecure %v", timeout, insecure)
		}
		return script
	}
	req, logs := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("private discovery succeeded")
	}
	if script.discoverCalls != 1 || script.exchangeCalls != 0 {
		t.Fatalf("discover %d exchange %d", script.discoverCalls, script.exchangeCalls)
	}
	if !strings.Contains(logs.String(), "non-public") && !strings.Contains(logs.String(), "refusing") {
		t.Fatalf("body %s log %s", rec.Body.String(), logs.String())
	}
}

func TestOpenInAppPublicOnlyIgnoresProxy(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "proxy", http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("https_proxy", proxy.URL)

	share := receivedWebappShare(
		"https://10.9.8.7/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, nil)
	h.clientTimeout = time.Second
	h.clientInsecure = true
	req, logs := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("proxied launch succeeded")
	}
	if hits.Load() != 0 {
		t.Fatalf("proxy received %d requests", hits.Load())
	}
	// A configured proxy would dial the proxy address. Proxy=nil dials the target.
	// The request URL omits the default port, so ":443" is the dial address.
	if !strings.Contains(logs.String(), "10.9.8.7:443") {
		t.Fatalf("dial did not target the share host: %s", logs.String())
	}
	if strings.Contains(logs.String(), proxy.Listener.Addr().String()) {
		t.Fatalf("dial targeted the proxy: %s", logs.String())
	}
}

func TestOpenInAppMissingFile(t *testing.T) {
	h := &appsHandler{ocmMountPoint: "/ocm"}
	req, _ := newLaunchRequest(t, "")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing file") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestSenderDiscoveryOrigin(t *testing.T) {
	tests := []struct {
		name    string
		protos  []*ocmpb.Protocol
		want    string
		wantErr string
	}{
		{
			name: "webdav preferred",
			protos: []*ocmpb.Protocol{
				webdavProtocol("https://dav.example/remote.php/dav"),
				webappProtocol("https://app.example/hub", launchSecret, []string{"must-exchange-token"}),
			},
			want: "https://dav.example",
		},
		{
			name: "webapp fallback",
			protos: []*ocmpb.Protocol{
				webdavProtocol("files/share"),
				webappProtocol("https://app.example:8443/hub", launchSecret, []string{"must-exchange-token"}),
			},
			want: "https://app.example:8443",
		},
		{
			name: "relative webdav does not hide absolute webapp",
			protos: []*ocmpb.Protocol{
				webdavProtocol("/remote.php/dav"),
				webappProtocol("https://app.example/hub", launchSecret, nil),
			},
			want: "https://app.example",
		},
		{
			name:    "missing",
			protos:  nil,
			wantErr: "absolute sender origin",
		},
		{
			name: "http webdav is rejected",
			protos: []*ocmpb.Protocol{
				webdavProtocol("http://dav.example/dav"),
				webappProtocol("https://app.example/hub", launchSecret, nil),
			},
			wantErr: "https",
		},
		{
			name: "userinfo rejected",
			protos: []*ocmpb.Protocol{
				webdavProtocol("https://user:pass@dav.example/dav"),
			},
			wantErr: "userinfo",
		},
		{
			name: "malformed",
			protos: []*ocmpb.Protocol{
				webdavProtocol("https://["),
			},
			wantErr: "malformed",
		},
		{
			name: "missing host is not skipped",
			protos: []*ocmpb.Protocol{
				webdavProtocol("https:///remote.php/dav"),
				webappProtocol("https://app.example/hub", launchSecret, nil),
			},
			wantErr: "must be absolute",
		},
		{
			name: "network path is not skipped",
			protos: []*ocmpb.Protocol{
				webdavProtocol("//dav.example/remote.php/dav"),
				webappProtocol("https://app.example/hub", launchSecret, nil),
			},
			wantErr: "must be absolute",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := senderDiscoveryOrigin(tt.protos)
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
			wantErr: "must be absolute",
		},
		{
			name: "network path is not relative",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint:      "https://sender.example/ocm",
				TokenEndPoint: "//evil.example/token",
			},
			wantErr: "must be absolute",
		},
		{
			name: "unsupported scheme without host",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint:      "",
				TokenEndPoint: "http:///token",
			},
			wantErr: "must be absolute",
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
			wantErr: "absolute base",
		},
		{
			name: "relative discovery endpoint",
			disco: &wellknown.OcmDiscoveryData{
				Endpoint:      "/ocm",
				TokenEndPoint: "token",
			},
			wantErr: "absolute base",
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
			wantErr: "userinfo",
		},
		{
			name:    "nil discovery",
			disco:   nil,
			wantErr: "tokenEndPoint",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func TestResolveRelativeAppURI(t *testing.T) {
	base := &wellknown.OcmDiscoveryData{Endpoint: "https://sender.example/ocm"}
	tests := []struct {
		name    string
		raw     string
		disco   *wellknown.OcmDiscoveryData
		want    string
		wantErr string
	}{
		{
			name:  "absolute path",
			raw:   "/apps/open",
			disco: base,
			want:  "https://sender.example/apps/open",
		},
		{
			name:  "path relative",
			raw:   "apps/open",
			disco: base,
			want:  "https://sender.example/apps/open",
		},
		{
			name:    "missing host is not relative",
			raw:     "https:///apps/open",
			disco:   &wellknown.OcmDiscoveryData{},
			wantErr: "must be absolute",
		},
		{
			name:    "network path is not relative",
			raw:     "//evil.example/apps/open",
			disco:   base,
			wantErr: "must be absolute",
		},
		{
			name:    "unsupported scheme",
			raw:     "http://app.example/hub",
			disco:   base,
			wantErr: "https",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRelativeAppURI(tt.raw, tt.disco)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || got != "" {
					t.Fatalf("got %q err %v", got, err)
				}
				if strings.Contains(got, "evil.example") || strings.Contains(err.Error(), "sender.example") {
					t.Fatalf("resolved a malformed reference: %q %v", got, err)
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

func TestJoinShareRelativePath(t *testing.T) {
	const app = "https://app.example/hub/open?folder=1&b=2#section"
	tests := []struct {
		name    string
		app     string
		rel     string
		want    string
		wantErr bool
	}{
		{
			name: "empty relative path",
			app:  app,
			rel:  "",
			want: app,
		},
		{
			name: "nested path",
			app:  "https://app.example/hub/open",
			rel:  "dir/sub/file.txt",
			want: "https://app.example/hub/open/dir/sub/file.txt",
		},
		{
			name: "spaces",
			app:  "https://app.example/hub/open?folder=1",
			rel:  "my file.txt",
			want: "https://app.example/hub/open/my%20file.txt?folder=1",
		},
		{
			name: "escaping",
			app:  app,
			rel:  "a+b%20c?d",
			want: "https://app.example/hub/open/a+b%20c%3Fd?folder=1&b=2#section",
		},
		{
			name: "preserved query and path",
			app:  app,
			rel:  "dir/file.txt",
			want: "https://app.example/hub/open/dir/file.txt?folder=1&b=2#section",
		},
		{
			name:    "malformed relative path",
			app:     app,
			rel:     "%zz",
			wantErr: true,
		},
		{
			name:    "malformed app uri",
			app:     "https://[",
			rel:     "file.txt",
			wantErr: true,
		},
		{
			name:    "traversal",
			app:     app,
			rel:     "../secret",
			wantErr: true,
		},
		{
			name:    "nested traversal",
			app:     app,
			rel:     "foo/../../etc",
			wantErr: true,
		},
		{
			name:    "encoded traversal",
			app:     app,
			rel:     "%2e%2e/secret",
			wantErr: true,
		},
		{
			name:    "absolute path",
			app:     app,
			rel:     "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "absolute url",
			app:     app,
			rel:     "https://evil.example/x",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := joinShareRelativePath(tt.app, tt.rel)
			if tt.wantErr {
				if err == nil || got != "" {
					t.Fatalf("got %q err %v", got, err)
				}
				if strings.Contains(got, tt.app) {
					t.Fatalf("fell back to %q", got)
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

func TestWriteLaunchJSONEncoderFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	req, _ := newLaunchRequest(t, "/ocm/share-1")
	writeLaunchJSON(rec, req, make(chan int))
	if rec.Code == http.StatusOK {
		t.Fatal("encoder failure committed HTTP 200")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "app_url") || strings.Contains(rec.Body.String(), launchToken) {
		t.Fatalf("body %s", rec.Body.String())
	}
	var apiErr reqres.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatal(err)
	}
	if apiErr.Code != reqres.APIErrorServerError {
		t.Fatalf("code %s", apiErr.Code)
	}
}

func TestWriteLaunchJSONWriterFailure(t *testing.T) {
	req, logs := newLaunchRequest(t, "/ocm/share-1")
	w := &failingResponseWriter{header: make(http.Header)}
	writeLaunchJSON(w, req, openInAppResponse{
		AppURL:      "https://app.example/hub",
		AccessToken: launchToken,
	})
	if w.status != http.StatusOK {
		t.Fatalf("status %d", w.status)
	}
	if w.writes != 1 {
		t.Fatalf("writes %d body %s", w.writes, w.body)
	}
	if strings.Contains(w.body, "SERVER_ERROR") {
		t.Fatalf("second error was appended: %s", w.body)
	}
	if !strings.Contains(logs.String(), "error writing launch response") {
		t.Fatalf("log %q", logs.String())
	}
	assertLogsClean(t, logs.String(), launchSecret, launchToken)
}

type observeClient struct {
	discoverErr       error
	exchangeErr       error
	token             string
	tokenByCall       []string
	endpointByCall    []string
	discoveryEndpoint string
	discoverCalls     int
	exchangeCalls     int
	origins           []string
	tokenURLs         []string
	codes             []string
	clientIDs         []string
	deadlines         int
	missingCtxValue   bool
}

func (c *observeClient) note(ctx context.Context) {
	if _, ok := ctx.Deadline(); ok {
		c.deadlines++
	}
	if ctx.Value(launchCtxKey{}) != launchCtxVal {
		c.missingCtxValue = true
	}
}

func (c *observeClient) Discover(ctx context.Context, endpoint string) (*wellknown.OcmDiscoveryData, error) {
	c.discoverCalls++
	c.origins = append(c.origins, endpoint)
	c.note(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.discoverErr != nil {
		return nil, c.discoverErr
	}
	tokenEndpoint := "https://token.example/ocm/token"
	if n := len(c.endpointByCall); n > 0 {
		idx := c.discoverCalls - 1
		if idx >= n {
			idx = n - 1
		}
		tokenEndpoint = c.endpointByCall[idx]
	}
	base := c.discoveryEndpoint
	if base == "" {
		base = "https://sender.example/ocm"
	}
	return &wellknown.OcmDiscoveryData{
		Endpoint:      base,
		TokenEndPoint: tokenEndpoint,
	}, nil
}

func (c *observeClient) ExchangeToken(
	ctx context.Context,
	tokenEndpoint, code, clientID string,
) (string, int64, error) {
	c.exchangeCalls++
	c.tokenURLs = append(c.tokenURLs, tokenEndpoint)
	c.codes = append(c.codes, code)
	c.clientIDs = append(c.clientIDs, clientID)
	c.note(ctx)
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	token := c.token
	if n := len(c.tokenByCall); n > 0 {
		idx := c.exchangeCalls - 1
		if idx >= n {
			idx = n - 1
		}
		token = c.tokenByCall[idx]
	}
	if c.exchangeErr != nil {
		return token, 0, c.exchangeErr
	}
	return token, 30, nil
}

// publicOnlyScript simulates a successful public discovery reply and sends
// every other hop through the real public-only client.
type publicOnlyScript struct {
	real          *ocmd.OCMClient
	tokenEndpoint string
	discoverCalls int
	exchangeCalls int
}

func (p *publicOnlyScript) Discover(ctx context.Context, endpoint string) (*wellknown.OcmDiscoveryData, error) {
	p.discoverCalls++
	if endpointIsNonPublic(endpoint) {
		return p.real.Discover(ctx, endpoint)
	}
	return &wellknown.OcmDiscoveryData{
		Endpoint:      "https://sender.example/ocm",
		TokenEndPoint: p.tokenEndpoint,
	}, nil
}

func (p *publicOnlyScript) ExchangeToken(
	ctx context.Context,
	tokenEndpoint, code, clientID string,
) (string, int64, error) {
	p.exchangeCalls++
	return p.real.ExchangeToken(ctx, tokenEndpoint, code, clientID)
}

func endpointIsNonPublic(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

type clientConfigSeen struct {
	calls    int
	timeout  time.Duration
	insecure bool
}

type fakeReceivedGateway struct {
	gateway.GatewayAPIClient
	resp     *ocmpb.GetReceivedOCMShareResponse
	err      error
	opaqueID string
	calls    int
	contexts []context.Context
}

func (f *fakeReceivedGateway) GetReceivedOCMShare(
	ctx context.Context,
	req *ocmpb.GetReceivedOCMShareRequest,
	_ ...grpc.CallOption,
) (*ocmpb.GetReceivedOCMShareResponse, error) {
	f.calls++
	f.contexts = append(f.contexts, ctx)
	if req != nil && req.GetRef() != nil && req.GetRef().GetId() != nil {
		f.opaqueID = req.GetRef().GetId().GetOpaqueId()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

type failingResponseWriter struct {
	header http.Header
	status int
	writes int
	body   string
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *failingResponseWriter) Write(p []byte) (int, error) {
	w.writes++
	w.body += string(p)
	return 0, errors.New("write failed")
}

type launchGatewayResolver struct {
	service.Clients
	mu sync.Mutex
	gw gateway.GatewayAPIClient
}

func (r *launchGatewayResolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gw == nil {
		return nil, errors.New("test gateway is not set")
	}
	return r.gw, nil
}

var (
	launchResolverOnce sync.Once
	launchResolver     = &launchGatewayResolver{}
)

func installLaunchGateway(t *testing.T, gw gateway.GatewayAPIClient) {
	t.Helper()
	launchResolverOnce.Do(func() {
		service.SetGlobal(launchResolver)
	})
	launchResolver.mu.Lock()
	launchResolver.gw = gw
	launchResolver.mu.Unlock()
}

func newTestHandler(t *testing.T, gw gateway.GatewayAPIClient, client launchClient) *appsHandler {
	t.Helper()
	installLaunchGateway(t, gw)
	h := &appsHandler{}
	if err := h.init(&config{
		OCMMountPoint:     "/ocm",
		ProviderDomain:    "receiver.example",
		OCMClientTimeout:  3,
		OCMClientInsecure: true,
	}); err != nil {
		t.Fatal(err)
	}
	if client != nil {
		h.newLaunchClient = func(time.Duration, bool) launchClient {
			return client
		}
	}
	return h
}

func newRecordingHandler(
	t *testing.T,
	gw gateway.GatewayAPIClient,
	client launchClient,
) (*appsHandler, *clientConfigSeen) {
	t.Helper()
	h := newTestHandler(t, gw, nil)
	seen := &clientConfigSeen{}
	h.newLaunchClient = func(timeout time.Duration, insecure bool) launchClient {
		seen.calls++
		seen.timeout = timeout
		seen.insecure = insecure
		_ = ocmd.NewPublicOnlyClient(timeout, insecure)
		return client
	}
	return h, seen
}

func newLaunchRequest(t *testing.T, filePath string) (*http.Request, *bytes.Buffer) {
	t.Helper()
	form := url.Values{}
	if filePath != "" {
		form.Set("file", filePath)
	}
	req := httptest.NewRequest(http.MethodPost, "/sciencemesh/open-in-app", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var logs bytes.Buffer
	logger := zerolog.New(&logs)
	ctx := appctx.WithLogger(req.Context(), &logger)
	ctx = context.WithValue(ctx, launchCtxKey{}, launchCtxVal)
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", launchAuthorization))
	return req.WithContext(ctx), &logs
}

func okShareResponse(share *ocmpb.ReceivedShare) *ocmpb.GetReceivedOCMShareResponse {
	return &ocmpb.GetReceivedOCMShareResponse{
		Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
		Share:  share,
	}
}

func receivedWebappShare(webdavURI, appURI, secret string, reqs []string) *ocmpb.ReceivedShare {
	protocols := []*ocmpb.Protocol{}
	if webdavURI != "" {
		protocols = append(protocols, webdavProtocol(webdavURI))
	}
	protocols = append(protocols, webappProtocol(appURI, secret, reqs))
	return &ocmpb.ReceivedShare{
		Id:        &ocmpb.ShareId{OpaqueId: "share-1"},
		Protocols: protocols,
	}
}

func webdavProtocol(uri string) *ocmpb.Protocol {
	return &ocmpb.Protocol{
		Term: &ocmpb.Protocol_WebdavOptions{
			WebdavOptions: &ocmpb.WebDAVProtocol{Uri: uri},
		},
	}
}

func webappProtocol(uri, secret string, reqs []string) *ocmpb.Protocol {
	return &ocmpb.Protocol{
		Term: &ocmpb.Protocol_WebappOptions{
			WebappOptions: &ocmpb.WebappProtocol{
				Uri:          uri,
				SharedSecret: secret,
				Requirements: reqs,
			},
		},
	}
}

func assertLogsClean(t *testing.T, logs, secret, token string) {
	t.Helper()
	if secret != "" && strings.Contains(logs, secret) {
		t.Fatalf("log leaked shared secret: %s", logs)
	}
	if token != "" && strings.Contains(logs, token) {
		t.Fatalf("log leaked access token: %s", logs)
	}
}

func assertNotLeaked(t *testing.T, body, logs, secret, token string) {
	t.Helper()
	assertLogsClean(t, logs, secret, token)
	if secret != "" && strings.Contains(body, secret) {
		t.Fatalf("body leaked shared secret: %s", body)
	}
	if token != "" && strings.Contains(body, token) {
		t.Fatalf("body leaked access token: %s", body)
	}
}
