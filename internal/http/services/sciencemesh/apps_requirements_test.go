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
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
)

func TestIngestHTTPDoesNotRelaxLaunchHTTPS(t *testing.T) {
	const uri = "http://app.example/hub"
	offer := &ocmd.Webapp{
		URI:          uri,
		SharedSecret: "secret",
		Permissions:  []string{"view"},
		Requirements: []string{"must-exchange-token"},
		Targets:      []string{"blank"},
	}
	if err := offer.ValidateReceived([]string{"blank"}, false); err != nil {
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
