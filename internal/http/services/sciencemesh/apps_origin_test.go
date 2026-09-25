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

	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
)

func TestSenderDiscoveryOrigin(t *testing.T) {
	tests := []struct {
		name    string
		protos  []*ocmpb.Protocol
		want    string
		wantErr string
	}{
		{
			name: "webdav preferred",
			protos: []*ocmpb.Protocol{
				webdavProtocol("https://dav.example/remote.php/dav"),
				webappProtocol("https://app.example/hub", launchSecret, []string{"must-exchange-token"}),
			},
			want: "https://dav.example",
		},
		{
			name: "webapp fallback",
			protos: []*ocmpb.Protocol{
				webdavProtocol("files/share"),
				webappProtocol("https://app.example:8443/hub", launchSecret, []string{"must-exchange-token"}),
			},
			want: "https://app.example:8443",
		},
		{
			name: "relative webdav does not hide absolute webapp",
			protos: []*ocmpb.Protocol{
				webdavProtocol("/remote.php/dav"),
				webappProtocol("https://app.example/hub", launchSecret, nil),
			},
			want: "https://app.example",
		},
		{
			name:    "missing",
			protos:  nil,
			wantErr: "absolute sender origin",
		},
		{
			name: "http webdav is rejected",
			protos: []*ocmpb.Protocol{
				webdavProtocol("http://dav.example/dav"),
				webappProtocol("https://app.example/hub", launchSecret, nil),
			},
			wantErr: "https",
		},
		{
			name: "userinfo rejected",
			protos: []*ocmpb.Protocol{
				webdavProtocol("https://user:pass@dav.example/dav"),
			},
			wantErr: "userinfo",
		},
		{
			name: "malformed",
			protos: []*ocmpb.Protocol{
				webdavProtocol("https://["),
			},
			wantErr: "malformed",
		},
		{
			name: "padded webdav is rejected",
			protos: []*ocmpb.Protocol{
				webdavProtocol(" https://dav.example/dav"),
				webappProtocol("https://app.example/hub", launchSecret, nil),
			},
			wantErr: "malformed",
		},
		{
			name: "missing host is not skipped",
			protos: []*ocmpb.Protocol{
				webdavProtocol("https:///remote.php/dav"),
				webappProtocol("https://app.example/hub", launchSecret, nil),
			},
			wantErr: "must be absolute",
		},
		{
			name: "network path is not skipped",
			protos: []*ocmpb.Protocol{
				webdavProtocol("//dav.example/remote.php/dav"),
				webappProtocol("https://app.example/hub", launchSecret, nil),
			},
			wantErr: "must be absolute",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := senderDiscoveryOrigin(tt.protos)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v", err)
				}
				if got != "" {
					t.Fatalf("origin %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("origin %q want %q", got, tt.want)
			}
		})
	}
}
