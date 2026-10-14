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
	"reflect"
	"strings"
	"testing"

	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
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

func TestStoredWebappPermissionsUseShareFlags(t *testing.T) {
	read := sunetReadPermissions()
	got := storedWebappPermissions(read)
	if !reflect.DeepEqual(got, []string{"read"}) {
		t.Fatalf("read flags = %v", got)
	}
	if got := storedWebappPermissions(nil); len(got) != 0 {
		t.Fatalf("nil flags = %v", got)
	}
	view := &providerv1beta1.ResourcePermissions{
		GetPath:       true,
		ListContainer: true,
		Stat:          true,
	}
	if got := storedWebappPermissions(view); !reflect.DeepEqual(got, []string{"view"}) {
		t.Fatalf("view flags = %v", got)
	}
	write := &providerv1beta1.ResourcePermissions{
		InitiateFileUpload: true,
	}
	if got := storedWebappPermissions(write); !reflect.DeepEqual(got, []string{"write"}) {
		t.Fatalf("write flags = %v", got)
	}
	shareOnly := &providerv1beta1.ResourcePermissions{
		AddGrant: true,
	}
	if got := storedWebappPermissions(shareOnly); !reflect.DeepEqual(got, []string{"share"}) {
		t.Fatalf("share flags = %v", got)
	}
}
