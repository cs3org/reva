// Copyright 2018-2021 CERN
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

package appregistry

import (
	"context"
	"sort"
	"testing"

	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/app/registry/static"
	"github.com/stretchr/testify/assert"
)

type ByAddress []*registrypb.ProviderInfo

func (a ByAddress) Len() int           { return len(a) }
func (a ByAddress) Less(i, j int) bool { return a[i].Address < a[j].Address }
func (a ByAddress) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func Test_ListAppProviders(t *testing.T) {
	tests := []struct {
		name  string
		rules map[string]interface{}
		want  *registrypb.ListAppProvidersResponse
	}{
		{
			name: "simple test",
			rules: map[string]interface{}{
				"text/json":         "some Address",
				"currently/ignored": "an other address",
			},

			// only Status and Providers will be asserted in the tests
			want: &registrypb.ListAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    1,
					Trace:   "00000000000000000000000000000000",
					Message: "",
				},
				Providers: []*registrypb.ProviderInfo{
					{Address: "some Address"},
					{Address: "an other address"},
				},
			},
		},
		{
			name:  "rules is nil",
			rules: nil,
			want: &registrypb.ListAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:  1,
					Trace: "00000000000000000000000000000000",
				},
				Providers: []*registrypb.ProviderInfo{
					{Address: ""},
				},
			},
		},
		{
			name:  "empty rules",
			rules: map[string]interface{}{},

			// only Status and Providers will be asserted in the tests
			want: &registrypb.ListAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    1,
					Trace:   "00000000000000000000000000000000",
					Message: "",
				},
				Providers: []*registrypb.ProviderInfo{
					{Address: ""},
				},
			},
		},
		{
			name: "rule value is nil",
			rules: map[string]interface{}{
				"text/json": nil,
			},

			// only Status and Providers will be asserted in the tests
			want: &registrypb.ListAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    1,
					Trace:   "00000000000000000000000000000000",
					Message: "",
				},
				Providers: []*registrypb.ProviderInfo{
					{Address: ""},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr, err := static.New(map[string]interface{}{"Rules": tt.rules})
			if err != nil {
				t.Errorf("could not create registry error = %v", err)
				return
			}

			ss := &svc{
				registry: rr,
			}
			got, err := ss.ListAppProviders(context.Background(), nil)

			if err != nil {
				t.Errorf("ListAppProviders() error = %v", err)
				return
			}
			assert.Equal(t, tt.want.Status, got.Status)
			sort.Sort(ByAddress(tt.want.Providers))
			sort.Sort(ByAddress(got.Providers))
			assert.Equal(t, tt.want.Providers, got.Providers)
		})
	}
}

func Test_GetAppProviders(t *testing.T) {
	rules := map[string]interface{}{
		"text/json": "JSON format",
		"image/bmp": "Windows OS/2 Bitmap Graphics",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "Microsoft Word (OpenXML)",
		"application/vnd.oasis.opendocument.presentation":                         "OpenDocument presentation document",
		"application/vnd.apple.installer+xml":                                     "Apple Installer Package",
		"text/xml":                                                                "XML",
	}

	tests := []struct {
		name   string
		search *providerv1beta1.ResourceInfo
		want   *registrypb.GetAppProvidersResponse
	}{
		{
			name:   "simple",
			search: &providerv1beta1.ResourceInfo{MimeType: "text/json"},
			// only Status and Providers will be asserted in the tests
			want: &registrypb.GetAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    1,
					Trace:   "00000000000000000000000000000000",
					Message: "",
				},
				Providers: []*registrypb.ProviderInfo{
					{Address: "JSON format"},
				},
			},
		},
		{
			name:   "more obscure MimeType",
			search: &providerv1beta1.ResourceInfo{MimeType: "application/vnd.apple.installer+xml"},
			want: &registrypb.GetAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    1,
					Trace:   "00000000000000000000000000000000",
					Message: "",
				},
				Providers: []*registrypb.ProviderInfo{
					{Address: "Apple Installer Package"},
				},
			},
		},
		{
			name:   "not existing MimeType",
			search: &providerv1beta1.ResourceInfo{MimeType: "doesnot/exist"},
			want: &registrypb.GetAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    15,
					Trace:   "00000000000000000000000000000000",
					Message: "error looking for the app provider",
				},
				Providers: nil,
			},
		},
		{
			name:   "empty MimeType",
			search: &providerv1beta1.ResourceInfo{MimeType: ""},
			want: &registrypb.GetAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    15,
					Trace:   "00000000000000000000000000000000",
					Message: "error looking for the app provider",
				},
				Providers: nil,
			},
		},
		{
			name:   "no data in resource info",
			search: &providerv1beta1.ResourceInfo{},
			want: &registrypb.GetAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    15,
					Trace:   "00000000000000000000000000000000",
					Message: "error looking for the app provider",
				},
				Providers: nil,
			},
		},
		{
			name:   "not valid MimeType",
			search: &providerv1beta1.ResourceInfo{MimeType: "this/type\\IS.not?VALID@all"},
			want: &registrypb.GetAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    15,
					Trace:   "00000000000000000000000000000000",
					Message: "error looking for the app provider",
				},
				Providers: nil,
			},
		},
	}

	rr, err := static.New(map[string]interface{}{"Rules": rules})
	if err != nil {
		t.Errorf("could not create registry error = %v", err)
		return
	}

	ss := &svc{
		registry: rr,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := registrypb.GetAppProvidersRequest{ResourceInfo: tt.search}
			got, err := ss.GetAppProviders(context.Background(), &req)

			if err != nil {
				t.Errorf("GetAppProviders() error = %v", err)
				return
			}
			assert.Equal(t, tt.want.Status, got.Status)
			sort.Sort(ByAddress(tt.want.Providers))
			sort.Sort(ByAddress(got.Providers))
			assert.Equal(t, tt.want.Providers, got.Providers)
		})
	}
}

func TestNew(t *testing.T) {

	tests := []struct {
		name    string
		m       map[string]interface{}
		rules   map[string]interface{}
		want    svc
		wantErr interface{}
	}{
		{
			name:    "no error",
			m:       map[string]interface{}{"Driver": "static"},
			wantErr: nil,
		},
		{
			name:    "not existing driver",
			m:       map[string]interface{}{"Driver": "doesnotexist"},
			wantErr: "driver not found: doesnotexist",
		},
		{
			name:    "empty",
			m:       map[string]interface{}{},
			wantErr: "driver not found: ",
		},
		{
			name:    "extra not existing field in setting",
			m:       map[string]interface{}{"Driver": "static", "doesnotexist": "doesnotexist"},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.m, nil)
			if err != nil {
				assert.Equal(t, tt.wantErr, err.Error())
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tt.wantErr, err)
			}
		})
	}
}
