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
	"strings"
	"testing"
)

func TestJoinShareRelativePath(t *testing.T) {
	const app = "https://app.example/hub/open?folder=1&b=2#section"
	tests := []struct {
		name    string
		app     string
		rel     string
		want    string
		wantErr bool
	}{
		{
			name: "empty relative path",
			app:  app,
			rel:  "",
			want: app,
		},
		{
			name: "nested path",
			app:  "https://app.example/hub/open",
			rel:  "dir/sub/file.txt",
			want: "https://app.example/hub/open/dir/sub/file.txt",
		},
		{
			name: "spaces",
			app:  "https://app.example/hub/open?folder=1",
			rel:  "my file.txt",
			want: "https://app.example/hub/open/my%20file.txt?folder=1",
		},
		{
			name: "escaping",
			app:  app,
			rel:  "a+b%20c?d",
			want: "https://app.example/hub/open/a+b%20c%3Fd?folder=1&b=2#section",
		},
		{
			name: "preserved query and path",
			app:  app,
			rel:  "dir/file.txt",
			want: "https://app.example/hub/open/dir/file.txt?folder=1&b=2#section",
		},
		{
			name:    "malformed relative path",
			app:     app,
			rel:     "%zz",
			wantErr: true,
		},
		{
			name:    "malformed app uri",
			app:     "https://[",
			rel:     "file.txt",
			wantErr: true,
		},
		{
			name:    "traversal",
			app:     app,
			rel:     "../secret",
			wantErr: true,
		},
		{
			name:    "nested traversal",
			app:     app,
			rel:     "foo/../../etc",
			wantErr: true,
		},
		{
			name:    "encoded traversal",
			app:     app,
			rel:     "%2e%2e/secret",
			wantErr: true,
		},
		{
			name:    "absolute path",
			app:     app,
			rel:     "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "absolute url",
			app:     app,
			rel:     "https://evil.example/x",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := joinShareRelativePath(tt.app, tt.rel)
			if tt.wantErr {
				if err == nil || got != "" {
					t.Fatalf("got %q err %v", got, err)
				}
				if strings.Contains(got, tt.app) {
					t.Fatalf("fell back to %q", got)
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
