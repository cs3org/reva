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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
