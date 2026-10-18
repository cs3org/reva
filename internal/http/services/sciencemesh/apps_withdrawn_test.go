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

	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
)

// A stale frontend action on a withdrawn share renders the safe error page
// and never discovers, exchanges or returns a token.
func TestWebappLaunchWithdrawnShareIsRefusedBeforeExchange(t *testing.T) {
	obs := &observeClient{token: launchToken}
	share := receivedWebappShare(
		"https://dav.example/remote.php/dav/ocm/share",
		"https://app.example/hub/open",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	share.State = ocmpb.ShareState_SHARE_STATE_INVALID
	share.Hidden = true
	gw := &fakeReceivedGateway{resp: okShareResponse(share)}
	h := newTestHandler(t, gw, obs)

	req, logs := newLaunchRequest(t, "/ocm/share-1/doc.txt")
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "This share is no longer available.") {
		t.Fatalf("body %s", body)
	}
	if strings.Contains(body, launchToken) || strings.Contains(body, launchSecret) || strings.Contains(body, "access_token") {
		t.Fatal("error page carries a credential or a launch form")
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'none'") {
		t.Fatalf("csp %q", csp)
	}
	if obs.discoverCalls != 0 || obs.exchangeCalls != 0 {
		t.Fatalf("discover %d exchange %d", obs.discoverCalls, obs.exchangeCalls)
	}
	assertLogsClean(t, logs.String(), launchSecret, launchToken)
}

// Hidden alone is not a withdrawal: a hidden active share still launches.
func TestWebappLaunchHiddenActiveShareStillLaunches(t *testing.T) {
	obs := &observeClient{token: launchToken}
	share := receivedWebappShare(
		"https://dav.example/remote.php/dav/ocm/share",
		"https://app.example/hub/open",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	share.Hidden = true
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, obs)
	req, _ := newLaunchRequest(t, "/ocm/share-1/doc.txt")
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)
	if _, token := launchedForm(t, rec); token != launchToken {
		t.Fatalf("token %q", token)
	}
}
