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
		name      string
		providers []map[string]interface{}
		mimeTypes []map[string]interface{}
		want      *registrypb.ListAppProvidersResponse
	}{
		{
			name: "simple test",
			providers: []map[string]interface{}{
				{
					"address":   "some Address",
					"mimetypes": []string{"text/json"},
				},
				{
					"address":   "another address",
					"mimetypes": []string{"currently/ignored"},
				},
			},
			mimeTypes: []map[string]interface{}{
				{
					"mime_type":   "text/json",
					"extension":   "json",
					"name":        "JSON File",
					"icon":        "https://example.org/icons&file=json.png",
					"default_app": "some Address",
				},
				{
					"mime_type":   "currently/ignored",
					"extension":   "unknown",
					"name":        "Ignored file",
					"icon":        "https://example.org/icons&file=unknown.png",
					"default_app": "some Address",
				},
			},

			// only Status and Providers will be asserted in the tests
			want: &registrypb.ListAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    1,
					Trace:   "00000000000000000000000000000000",
					Message: "",
				},
				Providers: []*registrypb.ProviderInfo{
					{
						Address:   "some Address",
						MimeTypes: []string{"text/json"},
					},
					{
						Address:   "another address",
						MimeTypes: []string{"currently/ignored"},
					},
				},
			},
		},
		{
			name:      "providers is nil",
			providers: nil,
			mimeTypes: nil,
			want: &registrypb.ListAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:  1,
					Trace: "00000000000000000000000000000000",
				},
				Providers: []*registrypb.ProviderInfo{},
			},
		},
		{
			name:      "empty providers",
			providers: []map[string]interface{}{},
			mimeTypes: []map[string]interface{}{},

			// only Status and Providers will be asserted in the tests
			want: &registrypb.ListAppProvidersResponse{
				Status: &rpcv1beta1.Status{
					Code:    1,
					Trace:   "00000000000000000000000000000000",
					Message: "",
				},
				Providers: []*registrypb.ProviderInfo{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr, err := static.New(context.Background(), map[string]interface{}{"providers": tt.providers, "mime_types": tt.mimeTypes})
			if err != nil {
				t.Errorf("could not create registry error = %v", err)
				return
			}

			ss := &svc{
				reg: rr,
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
	providers := []map[string]interface{}{
		{
			"address":   "text appprovider addr",
			"mimetypes": []string{"text/json", "text/xml"},
		},
		{
			"address":   "image appprovider addr",
			"mimetypes": []string{"image/bmp"},
		},
		{
			"address":   "misc appprovider addr",
			"mimetypes": []string{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/vnd.oasis.opendocument.presentation", "application/vnd.apple.installer+xml"},
		},
	}

	mimeTypes := []map[string]string{
		{
			"mime_type":   "text/json",
			"extension":   "json",
			"name":        "JSON File",
			"icon":        "https://example.org/icons&file=json.png",
			"default_app": "some Address",
		},
		{
			"mime_type":   "text/xml",
			"extension":   "xml",
			"name":        "XML File",
			"icon":        "https://example.org/icons&file=xml.png",
			"default_app": "some Address",
		},
		{
			"mime_type":   "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"extension":   "doc",
			"name":        "Word File",
			"icon":        "https://example.org/icons&file=doc.png",
			"default_app": "some Address",
		},
		{
			"mime_type":   "application/vnd.oasis.opendocument.presentation",
			"extension":   "odf",
			"name":        "OpenDocument File",
			"icon":        "https://example.org/icons&file=odf.png",
			"default_app": "some Address",
		},
		{
			"mime_type":   "application/vnd.apple.installer+xml",
			"extension":   "mpkg",
			"name":        "Mpkg File",
			"icon":        "https://example.org/icons&file=mpkg.png",
			"default_app": "some Address",
		},
		{
			"mime_type":   "image/bmp",
			"extension":   "bmp",
			"name":        "Image File",
			"icon":        "https://example.org/icons&file=bmp.png",
			"default_app": "some Address",
		},
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
					{
						Address:   "text appprovider addr",
						MimeTypes: []string{"text/json", "text/xml"},
					},
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
					{
						Address:   "misc appprovider addr",
						MimeTypes: []string{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/vnd.oasis.opendocument.presentation", "application/vnd.apple.installer+xml"},
					},
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

	rr, err := static.New(context.Background(), map[string]interface{}{"providers": providers, "mime_types": mimeTypes})
	if err != nil {
		t.Errorf("could not create registry error = %v", err)
		return
	}

	ss := &svc{
		reg: rr,
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
		name      string
		m         map[string]interface{}
		providers map[string]interface{}
		want      svc
		wantErr   interface{}
	}{
		{
			name:    "no error",
			m:       map[string]interface{}{"Driver": "static"},
			wantErr: nil,
		},
		{
			name:    "not existing driver",
			m:       map[string]interface{}{"Driver": "doesnotexist"},
			wantErr: "error: not found: appregistrysvc: driver not found: doesnotexist",
		},
		{
			name:    "empty",
			m:       map[string]interface{}{},
			wantErr: nil,
		},
		{
			name:    "extra not existing field in setting",
			m:       map[string]interface{}{"Driver": "static", "doesnotexist": "doesnotexist"},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(context.Background(), tt.m)
			if err != nil {
				assert.Equal(t, tt.wantErr, err.Error())
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tt.wantErr, err)
			}
		})
	}
}
