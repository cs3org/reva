// Copyright 2018-2026 CERN
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
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
)

func TestReceivedRequirementsAgreeWithIngest(t *testing.T) {
	blank := []string{"blank"}
	tests := []struct {
		name string
		reqs []string
		want string
		mfa  bool
		ok   bool
	}{
		{name: "exchange token", reqs: []string{"must-exchange-token"}, ok: true},
		{name: "padded requirement", reqs: []string{" must-exchange-token"}, want: "malformed requirement"},
		{name: "blank requirement", reqs: []string{" "}, want: "malformed requirement"},
		{name: "unknown requirement", reqs: []string{"must-exchange-token", "must-sign"}, want: "unsupported requirement"},
		{name: "missing exchange", reqs: []string{"must-use-mfa"}, want: "must-exchange-token"},
		{name: "permanent mfa", reqs: []string{"must-exchange-token", "must-use-mfa"}, mfa: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer := &ocmd.Webapp{
				URI:          "https://app.example/hub",
				SharedSecret: "secret",
				Permissions:  []string{"read"},
				Requirements: tt.reqs,
				Targets:      []string{"blank"},
			}
			received := offer.ValidateReceived(blank)
			launch := ocmd.ValidateWebappLaunch(
				offer.URI,
				offer.SharedSecret,
				offer.Requirements,
				offer.Targets,
				blank,
			)
			if tt.ok {
				if received != nil || launch != nil {
					t.Fatalf("received %v launch %v", received, launch)
				}
				return
			}
			if tt.mfa {
				if !errors.Is(received, ocmd.ErrWebappMFAUnproven) || received.Error() != ocmd.ErrWebappMFAUnproven.Error() {
					t.Fatalf("received %v", received)
				}
				if !errors.Is(launch, ocmd.ErrWebappMFAUnproven) || launch.Error() != received.Error() {
					t.Fatalf("launch %v", launch)
				}
				return
			}
			if received == nil || launch == nil || received.Error() != launch.Error() || !strings.Contains(received.Error(), tt.want) {
				t.Fatalf("received %v launch %v", received, launch)
			}
		})
	}
}

func TestIngestHTTPDoesNotRelaxLaunchHTTPS(t *testing.T) {
	const uri = "http://app.example/hub"
	offer := &ocmd.Webapp{
		URI:          uri,
		SharedSecret: "secret",
		Permissions:  []string{"view"},
		Requirements: []string{"must-exchange-token"},
		Targets:      []string{"blank"},
	}
	if err := offer.ValidateReceived([]string{"blank"}); err != nil {
		t.Fatal(err)
	}
	got, err := ocmd.ValidateAbsoluteWebappURI(uri)
	if err != nil || got != uri {
		t.Fatalf("ingest uri %q err %v", got, err)
	}
	if _, err := requireHTTPSAppURI(uri); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("launch err %v", err)
	}
}
