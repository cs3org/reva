// Copyright 2018-2023 CERN
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

package ocmd

import "testing"

func TestValidateURIScheme(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		allowHTTP bool
		wantErr   bool
	}{
		{name: "https accepted", raw: "https://provider.example/dav", allowHTTP: false, wantErr: false},
		{name: "http rejected when disallowed", raw: "http://provider.example/dav", allowHTTP: false, wantErr: true},
		{name: "http accepted when allowed", raw: "http://provider.example/dav", allowHTTP: true, wantErr: false},
		{name: "ftp rejected", raw: "ftp://provider.example/x", allowHTTP: true, wantErr: true},
		{name: "file rejected", raw: "file:///etc/passwd", allowHTTP: true, wantErr: true},
		{name: "empty host rejected", raw: "https:///path", allowHTTP: false, wantErr: true},
		{name: "unparseable rejected", raw: "://nope", allowHTTP: true, wantErr: true},
		{name: "empty string rejected", raw: "", allowHTTP: true, wantErr: true},
		{name: "webapp template accepted", raw: "https://provider.example/open?f={fileid}&v={viewmode}", allowHTTP: false, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURIScheme(tt.raw, tt.allowHTTP)
			if tt.wantErr && err == nil {
				t.Errorf("validateURIScheme(%q, %v) = nil, want error", tt.raw, tt.allowHTTP)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateURIScheme(%q, %v) = %v, want nil", tt.raw, tt.allowHTTP, err)
			}
		})
	}
}
