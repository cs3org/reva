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
	"net/url"
	"strings"
	"testing"
)

func TestRequireHTTPSAppURI(t *testing.T) {
	const preserved = "https://app.example/hub/open?folder=1#lab"
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "absolute https keeps query and fragment", raw: preserved, want: preserved},
		{name: "preserves escaping", raw: "https://app.example/a%20b", want: "https://app.example/a%20b"},
		{name: "relative path", raw: "/apps/open", wantErr: "malformed"},
		{name: "path relative", raw: "apps/open", wantErr: "malformed"},
		{name: "network path", raw: "//evil.example/apps/open", wantErr: "malformed"},
		{name: "opaque", raw: "https:app.example/hub", wantErr: "malformed"},
		{name: "padded", raw: " https://app.example/hub", wantErr: "malformed"},
		{name: "blank", raw: " ", wantErr: "malformed"},
		{name: "userinfo", raw: "https://user:pass@app.example/hub", wantErr: "malformed"},
		{name: "http is rejected by launch policy", raw: "http://app.example/hub", wantErr: "https"},
		{name: "double scheme", raw: "https://https://app.example/hub", wantErr: "malformed"},
		{name: "missing hostname", raw: "https://:443/hub", wantErr: "malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := requireHTTPSAppURI(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || got != "" {
					t.Fatalf("got %q err %v", got, err)
				}
				if strings.Contains(err.Error(), "evil.example") || strings.Contains(err.Error(), "user:pass") {
					t.Fatalf("error leaked url material: %v", err)
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

func TestValidateLaunchURLRejectsMissingHostname(t *testing.T) {
	parsed, err := url.Parse("https://:443/hub")
	if err != nil {
		t.Fatal(err)
	}
	err = validateLaunchURL(parsed)
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("err = %v", err)
	}
}
