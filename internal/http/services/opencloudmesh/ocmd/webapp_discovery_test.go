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

package ocmd

import (
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func TestWebappTokenEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		disco   *wellknown.OcmDiscoveryData
		want    string
		wantErr string
	}{
		{
			name: "absolute https",
			disco: &wellknown.OcmDiscoveryData{
				Capabilities:  []string{"exchange-token"},
				TokenEndPoint: "https://token.example/ocm/token?x=1",
			},
			want: "https://token.example/ocm/token?x=1",
		},
		{
			name: "absolute http is syntactically accepted",
			disco: &wellknown.OcmDiscoveryData{
				Capabilities:  []string{"exchange-token"},
				TokenEndPoint: "http://token.example/ocm/token",
			},
			want: "http://token.example/ocm/token",
		},
		{
			name: "relative against absolute discovery endpoint",
			disco: &wellknown.OcmDiscoveryData{
				Capabilities:  []string{"exchange-token"},
				Endpoint:      "https://sender.example/ocm/",
				TokenEndPoint: "token",
			},
			want: "https://sender.example/ocm/token",
		},
		{
			name:    "nil discovery",
			wantErr: "missing",
		},
		{
			name: "missing capability",
			disco: &wellknown.OcmDiscoveryData{
				TokenEndPoint: "https://token.example/ocm/token",
			},
			wantErr: "exchange-token",
		},
		{
			name: "blank token endpoint",
			disco: &wellknown.OcmDiscoveryData{
				Capabilities: []string{"exchange-token"},
			},
			wantErr: "tokenEndPoint",
		},
		{
			name: "network path",
			disco: &wellknown.OcmDiscoveryData{
				Capabilities:  []string{"exchange-token"},
				Endpoint:      "https://sender.example/ocm",
				TokenEndPoint: "//evil.example/token",
			},
			wantErr: "malformed",
		},
		{
			name: "userinfo",
			disco: &wellknown.OcmDiscoveryData{
				Capabilities:  []string{"exchange-token"},
				TokenEndPoint: "https://user:pass@token.example/token",
			},
			wantErr: "malformed",
		},
		{
			name: "relative without absolute base",
			disco: &wellknown.OcmDiscoveryData{
				Capabilities:  []string{"exchange-token"},
				Endpoint:      "/ocm",
				TokenEndPoint: "token",
			},
			wantErr: "malformed",
		},
		{
			name: "padded discovery base",
			disco: &wellknown.OcmDiscoveryData{
				Capabilities:  []string{"exchange-token"},
				Endpoint:      " https://sender.example/ocm/",
				TokenEndPoint: "token",
			},
			wantErr: "malformed",
		},
		{
			name: "double-scheme discovery base",
			disco: &wellknown.OcmDiscoveryData{
				Capabilities:  []string{"exchange-token"},
				Endpoint:      "https://https://sender.example/ocm",
				TokenEndPoint: "token",
			},
			wantErr: "malformed",
		},
		{
			name: "unsupported scheme",
			disco: &wellknown.OcmDiscoveryData{
				Capabilities:  []string{"exchange-token"},
				TokenEndPoint: "ftp://token.example/token",
			},
			wantErr: "malformed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := WebappTokenEndpoint(tt.disco)
			if tt.wantErr != "" {
				var bad errtypes.BadRequest
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || got != "" {
					t.Fatalf("got %q err %v", got, err)
				}
				if !errorsAsBadRequest(err, &bad) {
					t.Fatalf("err type %T", err)
				}
				if strings.Contains(err.Error(), "user:pass") || strings.Contains(err.Error(), "evil.example") {
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

func errorsAsBadRequest(err error, target *errtypes.BadRequest) bool {
	if err == nil {
		return false
	}
	got, ok := err.(errtypes.BadRequest)
	if !ok {
		return false
	}
	*target = got
	return true
}
