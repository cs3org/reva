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
	"google.golang.org/grpc/metadata"
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
	h, _ := newRecordingHandler(t, gw, obs)

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
		urls = append(urls, payload.AppURL)
	}
	if obs.discoverCalls != 2 || len(obs.origins) != 2 {
		t.Fatalf("discover %d origins %v", obs.discoverCalls, obs.origins)
	}
	if obs.origins[0] != "https://dav-a.example" || obs.origins[1] != "https://dav-b.example" {
		t.Fatalf("origins %v", obs.origins)
	}
	if urls[0] != "https://app.example/hub/a" || urls[1] != "https://app.example/hub/b" {
		t.Fatalf("app urls %v", urls)
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
