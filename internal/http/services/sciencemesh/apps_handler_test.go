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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"google.golang.org/grpc/metadata"
)

func TestOpenInAppMustUseMFAOffPolicy(t *testing.T) {
	obs := &observeClient{token: launchToken}
	share := receivedWebappShare(
		"https://dav.example/remote.php/dav/ocm/share",
		"https://app.example/hub/open?folder=1",
		launchSecret,
		[]string{"must-exchange-token", "must-use-mfa"},
	)
	gw := &fakeReceivedGateway{resp: okShareResponse(share)}
	h := newTestHandler(t, gw, obs)
	off := wellknown.MFAPolicyOff
	h.mfaPolicy = &off

	req, logs := newLaunchRequest(t, "/ocm/share-1/dir/file.txt")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if h.launchClient != obs {
		t.Fatal("launch client was not retained")
	}
	if obs.discoverCalls != 1 || obs.exchangeCalls != 1 {
		t.Fatalf("discover %d exchange %d", obs.discoverCalls, obs.exchangeCalls)
	}
	assertLogsClean(t, logs.String(), launchSecret, launchToken)
}

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
	h := newTestHandler(t, gw, obs)

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
	if h.launchClient != obs {
		t.Fatal("launch client was not retained")
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

func TestOpenInAppChangedShareIsNotCached(t *testing.T) {
	obs := &observeClient{
		endpointByCall: []string{
			"https://token-a.example/ocm/token",
			"https://token-b.example/ocm/token",
		},
		tokenByCall: []string{"token-a", "token-b"},
	}
	first := receivedWebappShare(
		"https://dav-a.example/dav",
		"https://app.example/hub/a",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	second := receivedWebappShare(
		"https://dav-b.example/dav",
		"https://app.example/hub/b",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	gw := &fakeReceivedGateway{resps: []*ocmpb.GetReceivedOCMShareResponse{
		okShareResponse(first),
		okShareResponse(second),
	}}
	h := newTestHandler(t, gw, obs)

	var urls []string
	var tokens []string
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
		urls = append(urls, payload.AppURL)
		tokens = append(tokens, payload.AccessToken)
	}
	if gw.calls != 2 || gw.opaqueID != "share-1" {
		t.Fatalf("lookups %d share %q", gw.calls, gw.opaqueID)
	}
	if h.launchClient != obs {
		t.Fatal("repeated launch did not keep the same client")
	}
	if obs.discoverCalls != 2 || obs.exchangeCalls != 2 || len(obs.origins) != 2 {
		t.Fatalf(
			"discover %d exchange %d origins %v",
			obs.discoverCalls,
			obs.exchangeCalls,
			obs.origins,
		)
	}
	if obs.origins[0] != "https://dav-a.example" || obs.origins[1] != "https://dav-b.example" {
		t.Fatalf("origins %v", obs.origins)
	}
	if urls[0] != "https://app.example/hub/a" || urls[1] != "https://app.example/hub/b" {
		t.Fatalf("app urls %v", urls)
	}
	if len(obs.tokenURLs) != 2 ||
		obs.tokenURLs[0] != obs.endpointByCall[0] ||
		obs.tokenURLs[1] != obs.endpointByCall[1] {
		t.Fatalf("exchanged endpoints %v", obs.tokenURLs)
	}
	if tokens[0] != "token-a" || tokens[1] != "token-b" {
		t.Fatalf("tokens %v", tokens)
	}
}

func TestOpenInAppMalformedForm(t *testing.T) {
	gw := &fakeReceivedGateway{}
	obs := &observeClient{token: launchToken}
	h := newTestHandler(t, gw, obs)
	base, logs := newLaunchRequest(t, "/ocm/share-1")
	req := httptest.NewRequest(
		http.MethodPost,
		"/sciencemesh/open-in-app",
		strings.NewReader("file=%zz"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(base.Context())

	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, body)
	}
	invalidParameter := strings.Contains(body, `"INVALID_PARAMETER"`)
	unparsed := strings.Contains(body, "parameters could not be parsed")
	if !invalidParameter || !unparsed {
		t.Fatalf("body %s", body)
	}
	if gw.calls != 0 || obs.discoverCalls != 0 || obs.exchangeCalls != 0 {
		t.Fatalf(
			"gateway %d discover %d exchange %d",
			gw.calls,
			obs.discoverCalls,
			obs.exchangeCalls,
		)
	}
	assertNotLeaked(t, body, logs.String(), launchSecret, launchToken)
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

func launchWebappShare(t *testing.T) *ocmpb.ReceivedShare {
	t.Helper()
	return receivedWebappShare(
		"https://dav.example/dav",
		"https://app.example/hub/open?keep=1#frag",
		launchSecret,
		[]string{"must-exchange-token"},
	)
}

func TestOpenInAppFileIdentifierSuccess(t *testing.T) {
	const bareApp = "https://app.example/hub/open?keep=1#frag"
	validSpace := spaces.EncodeSpaceID("/teams/demo")
	withSpace := "storage-id$" + validSpace

	tests := []struct {
		name      string
		file      string
		wantShare string
		wantURL   string
	}{
		{
			name:      "exact gateway share id",
			file:      "/ocm/share-1",
			wantShare: "share-1",
			wantURL:   bareApp,
		},
		{
			name:      "resource root colon",
			file:      "/ocm/storage-id!share-1:",
			wantShare: "share-1",
			wantURL:   bareApp,
		},
		{
			name:      "resource root slash after colon",
			file:      "/ocm/storage-id!share-1:/",
			wantShare: "share-1",
			wantURL:   bareApp,
		},
		{
			name:      "nested resource id without space component",
			file:      "/ocm/storage-id!share-1:nested/file.txt",
			wantShare: "share-1",
			wantURL:   "https://app.example/hub/open/nested/file.txt?keep=1#frag",
		},
		{
			name:      "nested resource id without space and driver slash",
			file:      "/ocm/storage-id!share-1:/nested/file.txt",
			wantShare: "share-1",
			wantURL:   "https://app.example/hub/open/nested/file.txt?keep=1#frag",
		},
		{
			name:      "nested resource id with space component",
			file:      "/ocm/" + withSpace + "!share-1:nested/file.txt",
			wantShare: "share-1",
			wantURL:   "https://app.example/hub/open/nested/file.txt?keep=1#frag",
		},
		{
			name:      "nested resource id with received driver slash",
			file:      "/ocm/" + withSpace + "!share-1:/nested/file.txt",
			wantShare: "share-1",
			wantURL:   "https://app.example/hub/open/nested/file.txt?keep=1#frag",
		},
		{
			name:      "exact relative app url with spaces",
			file:      "/ocm/share-1/nested/my%20file.txt",
			wantShare: "share-1",
			wantURL:   "https://app.example/hub/open/nested/my%20file.txt?keep=1#frag",
		},
		{
			name:      "escaped characters as path data",
			file:      "/ocm/share-1/a%2bb%20c",
			wantShare: "share-1",
			wantURL:   "https://app.example/hub/open/a+b%20c?keep=1#frag",
		},
		{
			name:      "bang as path data",
			file:      "/ocm/share-1/my!file.txt",
			wantShare: "share-1",
			wantURL:   "https://app.example/hub/open/my%21file.txt?keep=1#frag",
		},
		{
			name:      "colon and bang as path form path data",
			file:      "/ocm/share-1/a:b!c",
			wantShare: "share-1",
			wantURL:   "https://app.example/hub/open/a:b%21c?keep=1#frag",
		},
		{
			name:      "colon as resource id suffix path data",
			file:      "/ocm/storage-id!share-1:dir/colon:name",
			wantShare: "share-1",
			wantURL:   "https://app.example/hub/open/dir/colon:name?keep=1#frag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &observeClient{token: launchToken}
			gw := &fakeReceivedGateway{resp: okShareResponse(launchWebappShare(t))}
			h := newTestHandler(t, gw, obs)
			req, logs := newLaunchRequest(t, tt.file)
			rec := httptest.NewRecorder()
			h.OpenInApp(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			var payload openInAppResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.AppURL != tt.wantURL {
				t.Fatalf("app_url = %q want %q", payload.AppURL, tt.wantURL)
			}
			if payload.AccessToken != launchToken {
				t.Fatalf("access_token = %q", payload.AccessToken)
			}
			if gw.opaqueID != tt.wantShare {
				t.Fatalf("gateway share %q", gw.opaqueID)
			}
			if obs.discoverCalls != 1 || obs.exchangeCalls != 1 {
				t.Fatalf("discover %d exchange %d", obs.discoverCalls, obs.exchangeCalls)
			}
			assertLogsClean(t, logs.String(), launchSecret, launchToken)
			if strings.Contains(rec.Body.String(), launchSecret) {
				t.Fatalf("response included the shared secret")
			}
		})
	}
}

func TestOpenInAppFileIdentifierRejectedBeforeRemote(t *testing.T) {
	validSpace := spaces.EncodeSpaceID("/teams/demo")
	tests := []struct {
		name   string
		file   string
		forbid string
	}{
		{
			name:   "internal whitespace in storage prefix",
			file:   "/ocm/stor age-id!LEAK-PARSE-1:",
			forbid: "LEAK-PARSE-1",
		},
		{
			name:   "missing colon",
			file:   "/ocm/storage-id!LEAK-PARSE-2",
			forbid: "LEAK-PARSE-2",
		},
		{
			name:   "malformed base32 space",
			file:   "/ocm/storage-id$not-valid-base32!LEAK-PARSE-3:",
			forbid: "LEAK-PARSE-3",
		},
		{
			name:   "double leading suffix slash",
			file:   "/ocm/storage-id!LEAK-PARSE-4://nested",
			forbid: "LEAK-PARSE-4",
		},
		{
			name:   "empty share id",
			file:   "/ocm/storage-id!:LEAK-PARSE-5",
			forbid: "LEAK-PARSE-5",
		},
		{
			name:   "malformed storage prefix with space component",
			file:   "/ocm/bad prefix$" + validSpace + "!LEAK-PARSE-6:",
			forbid: "LEAK-PARSE-6",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &observeClient{token: launchToken}
			gw := &fakeReceivedGateway{resp: okShareResponse(launchWebappShare(t))}
			h := newTestHandler(t, gw, obs)
			req, logs := newLaunchRequest(t, tt.file)
			rec := httptest.NewRecorder()
			h.OpenInApp(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, "invalid file identifier") {
				t.Fatalf("body %s", body)
			}
			if strings.Contains(body, tt.forbid) || strings.Contains(logs.String(), tt.forbid) {
				t.Fatalf("leaked identifier body=%s logs=%s", body, logs.String())
			}
			if gw.calls != 0 || obs.discoverCalls != 0 || obs.exchangeCalls != 0 {
				t.Fatalf("gateway %d discover %d exchange %d", gw.calls, obs.discoverCalls, obs.exchangeCalls)
			}
			assertNotLeaked(t, body, logs.String(), launchSecret, launchToken)
		})
	}
}

func TestOpenInAppSharePathPolicyBeforeExchange(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantText string
	}{
		{
			name:     "literal traversal suffix",
			file:     "/ocm/share-1/../secret",
			wantText: "escapes the share",
		},
		{
			name:     "encoded traversal suffix",
			file:     "/ocm/share-1/%2e%2e/secret",
			wantText: "escapes the share",
		},
		{
			name:     "repeatedly encoded traversal suffix",
			file:     "/ocm/share-1/%252e%252e/secret",
			wantText: "escapes the share",
		},
		{
			name:     "absolute suffix",
			file:     "/ocm/share-1//notes.md",
			wantText: "invalid share-relative path",
		},
		{
			name:     "literal single dot segment",
			file:     "/ocm/share-1/docs/./notes.md",
			wantText: "invalid share-relative path",
		},
		{
			name:     "empty interior segment",
			file:     "/ocm/share-1/docs//notes.md",
			wantText: "invalid share-relative path",
		},
		{
			name:     "encoded absolute suffix",
			file:     "/ocm/share-1/%2fnotes.md",
			wantText: "invalid share-relative path",
		},
		{
			name:     "repeatedly encoded absolute suffix",
			file:     "/ocm/share-1/%252fnotes.md",
			wantText: "invalid share-relative path",
		},
		{
			name:     "received resource dot segment",
			file:     "/ocm/storage-id!share-1:/docs/./notes.md",
			wantText: "invalid share-relative path",
		},
		{
			name:     "received resource empty segment",
			file:     "/ocm/storage-id!share-1:/docs//notes.md",
			wantText: "invalid share-relative path",
		},
		{
			name:     "received resource encoded absolute suffix",
			file:     "/ocm/storage-id!share-1:%2fnotes.md",
			wantText: "invalid share-relative path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &observeClient{token: launchToken}
			gw := &fakeReceivedGateway{resp: okShareResponse(launchWebappShare(t))}
			h := newTestHandler(t, gw, obs)
			req, logs := newLaunchRequest(t, tt.file)
			rec := httptest.NewRecorder()
			h.OpenInApp(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, "\"INVALID_PARAMETER\"") {
				t.Fatalf("body %s missing INVALID_PARAMETER", body)
			}
			if strings.Contains(body, tt.file) || strings.Contains(logs.String(), tt.file) {
				t.Fatalf("file identifier leaked: body=%s logs=%s", body, logs.String())
			}
			if unescaped, unescapeErr := fullyUnescape(tt.file); unescapeErr == nil && unescaped != tt.file {
				if strings.Contains(body, unescaped) || strings.Contains(logs.String(), unescaped) {
					t.Fatalf("decoded file identifier leaked: body=%s logs=%s", body, logs.String())
				}
			}
			if !strings.Contains(body, tt.wantText) {
				t.Fatalf("body %s", body)
			}
			if gw.calls != 1 {
				t.Fatalf("gateway calls %d", gw.calls)
			}
			if obs.discoverCalls != 0 || obs.exchangeCalls != 0 {
				t.Fatalf("discover %d exchange %d", obs.discoverCalls, obs.exchangeCalls)
			}
			if strings.Contains(body, "https://app.example/hub") {
				t.Fatalf("fell back to bare app URL: %s", body)
			}
			assertNotLeaked(t, body, logs.String(), launchSecret, launchToken)
		})
	}
}

func TestOpenInAppSecondShareIDNotCached(t *testing.T) {
	obs := &observeClient{token: launchToken}
	first := receivedWebappShare(
		"https://dav-a.example/dav",
		"https://app.example/hub/a",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	second := receivedWebappShare(
		"https://dav-b.example/dav",
		"https://app.example/hub/b",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	gw := &fakeReceivedGateway{resps: []*ocmpb.GetReceivedOCMShareResponse{
		okShareResponse(first),
		okShareResponse(second),
	}}
	h := newTestHandler(t, gw, obs)

	paths := []string{"/ocm/share-1", "/ocm/share-2"}
	wantURLs := []string{
		"https://app.example/hub/a",
		"https://app.example/hub/b",
	}
	for i, path := range paths {
		req, _ := newLaunchRequest(t, path)
		rec := httptest.NewRecorder()
		h.OpenInApp(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		var payload openInAppResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.AppURL != wantURLs[i] {
			t.Fatalf("app_url = %q", payload.AppURL)
		}
	}
	if gw.opaqueID != "share-2" {
		t.Fatalf("last lookup share %q", gw.opaqueID)
	}
}
