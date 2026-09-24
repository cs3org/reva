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

package conversions

import (
	"testing"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
)

// receiverLocalConfigName is a stand-in for a receiver product name. The
// helper must ignore it and return only the persisted remote AppName.
const receiverLocalConfigName = "LocalPad"

func TestWebappMetadataForReceivedShare(t *testing.T) {
	tests := []struct {
		name    string
		share   *ocm.ReceivedShare
		wantNil bool
		appName string
	}{
		{
			name:    "non-empty persisted name",
			share:   shareWithWebapp("CodiMD"),
			appName: "CodiMD",
		},
		{
			name:    "second remote name",
			share:   shareWithWebapp("Etherpad"),
			appName: "Etherpad",
		},
		{
			name:    "escaped name",
			share:   shareWithWebapp(`say "hi" / path\ok`),
			appName: `say "hi" / path\ok`,
		},
		{
			name:    "markup name",
			share:   shareWithWebapp("<b>app</b>"),
			appName: "<b>app</b>",
		},
		{
			name:    "unicode name",
			share:   shareWithWebapp("caf\u00e9 \u4e2d"),
			appName: "caf\u00e9 \u4e2d",
		},
		{
			name:    "legacy empty name",
			share:   shareWithWebapp(""),
			appName: "",
		},
		{
			name:    "legacy whitespace name",
			share:   shareWithWebapp(" \t"),
			appName: " \t",
		},
		{
			name:    "webdav only",
			share:   shareWithProtocols(webdavProtocol("https://remote.example/dav/file")),
			wantNil: true,
		},
		{
			name:    "no protocol",
			share:   &ocm.ReceivedShare{},
			wantNil: true,
		},
		{
			name:    "nil share",
			share:   nil,
			wantNil: true,
		},
		{
			name:    "nil protocol entry",
			share:   shareWithProtocols(nil),
			wantNil: true,
		},
		{
			name: "nil webapp options",
			share: shareWithProtocols(&ocm.Protocol{
				Term: &ocm.Protocol_WebappOptions{},
			}),
			wantNil: true,
		},
		{
			name: "duplicate webapp protocols",
			share: shareWithProtocols(
				webappProtocol("CodiMD"),
				webappProtocol("CodiMD"),
			),
			wantNil: true,
		},
		{
			name: "duplicate webapp protocols with different names",
			share: shareWithProtocols(
				webappProtocol("CodiMD"),
				webappProtocol("Etherpad"),
			),
			wantNil: true,
		},
		{
			name: "webdav plus one webapp",
			share: shareWithProtocols(
				webdavProtocol("https://remote.example/dav/notes"),
				webappProtocol("CodiMD"),
			),
			appName: "CodiMD",
		},
		{
			name: "nil entry beside one webapp",
			share: shareWithProtocols(
				nil,
				webappProtocol("Etherpad"),
			),
			appName: "Etherpad",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WebappMetadataForReceivedShare(tt.share)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("metadata = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("metadata is nil")
			}
			if !got.Present {
				t.Fatal("Present = false, want true")
			}
			if got.AppName != tt.appName {
				t.Fatalf("AppName = %q, want %q", got.AppName, tt.appName)
			}
			if got.AppName == receiverLocalConfigName {
				t.Fatalf("AppName used receiver local config %q", receiverLocalConfigName)
			}
		})
	}
}

func TestWebappMetadataSharesDoNotInheritNames(t *testing.T) {
	first := WebappMetadataForReceivedShare(shareWithWebapp("CodiMD"))
	second := WebappMetadataForReceivedShare(shareWithWebapp("Etherpad"))
	if first == nil || second == nil {
		t.Fatal("expected metadata for both shares")
	}
	if first.AppName != "CodiMD" || second.AppName != "Etherpad" {
		t.Fatalf("names = %q, %q", first.AppName, second.AppName)
	}
	if first.AppName == receiverLocalConfigName || second.AppName == receiverLocalConfigName {
		t.Fatal("receiver local config name was substituted")
	}
	if first == second {
		t.Fatal("shares share one metadata value")
	}
}

func shareWithWebapp(appName string) *ocm.ReceivedShare {
	return shareWithProtocols(webappProtocol(appName))
}

func shareWithProtocols(protocols ...*ocm.Protocol) *ocm.ReceivedShare {
	return &ocm.ReceivedShare{
		Id:        &ocm.ShareId{OpaqueId: "share"},
		Name:      receiverLocalConfigName,
		Protocols: protocols,
	}
}

func webappProtocol(appName string) *ocm.Protocol {
	return &ocm.Protocol{
		Term: &ocm.Protocol_WebappOptions{
			WebappOptions: &ocm.WebappProtocol{
				Uri:          "https://remote.example/open",
				SharedSecret: "secret-must-stay-in-storage",
				AppName:      appName,
				AppIconHint:  "image/png",
				MediaTypes:   []string{"text/markdown"},
			},
		},
	}
}

func webdavProtocol(uri string) *ocm.Protocol {
	return &ocm.Protocol{
		Term: &ocm.Protocol_WebdavOptions{
			WebdavOptions: &ocm.WebDAVProtocol{
				Uri:          uri,
				SharedSecret: "webdav-secret-must-stay-in-storage",
			},
		},
	}
}
