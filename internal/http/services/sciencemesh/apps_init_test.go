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
	"testing"
)

func scienceMeshServiceConfig(domain string, setDomain bool) map[string]any {
	cfg := map[string]any{
		"gatewaysvc":         "127.0.0.1:19000",
		"mesh_directory_url": "https://mesh.example.test",
	}
	if setDomain {
		cfg["provider_domain"] = domain
	}
	return cfg
}

func TestAppsServiceNewFailsOnAbsentProviderDomain(t *testing.T) {
	svc, err := New(context.Background(), scienceMeshServiceConfig("", false))
	if err == nil || svc != nil {
		t.Fatalf("absent provider_domain: svc=%v err=%v", svc, err)
	}

	svc, err = New(context.Background(), scienceMeshServiceConfig("", true))
	if err == nil || svc != nil {
		t.Fatalf("empty provider_domain: svc=%v err=%v", svc, err)
	}
}

func TestAppsServiceNewFailsOnInvalidProviderDomain(t *testing.T) {
	svc, err := New(context.Background(), scienceMeshServiceConfig("https://receiver.example.test", true))
	if err == nil || svc != nil {
		t.Fatalf("svc=%v err=%v", svc, err)
	}
}
