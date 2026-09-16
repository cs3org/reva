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

package appregistry

import (
	"context"
	"sort"
	"testing"

	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	svcregistry "github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/registry/memory"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

type ByAddress []*registrypb.ProviderInfo

func (a ByAddress) Len() int           { return len(a) }
func (a ByAddress) Less(i, j int) bool { return a[i].Address < a[j].Address }
func (a ByAddress) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// The app registry discovers its providers in the process-wide service
// registry, which only accepts the first one installed, so the whole package
// shares these app providers.
func init() {
	reg := memory.New(nil)
	providers := []struct {
		address   string
		app       string
		mimeTypes []string
	}{
		{"text appprovider addr", "TextEditor", []string{"text/json", "text/xml"}},
		{"image appprovider addr", "ImageViewer", []string{"image/bmp"}},
		{"misc appprovider addr", "MiscEditor", []string{
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.oasis.opendocument.presentation",
			"application/vnd.apple.installer+xml",
		}},
	}

	for _, p := range providers {
		encoded, err := protojson.Marshal(&registrypb.ProviderInfo{Name: p.app, MimeTypes: p.mimeTypes})
		if err != nil {
			panic(err)
		}
		node := svcregistry.NewNode(p.address+"/appprovider", p.address, map[string]string{
			svcregistry.MetaTransport: svcregistry.TransportGRPC,
			svcregistry.MetaState:     svcregistry.StateReady,
			svcregistry.MetaApp:       string(encoded),
		})
		if err := reg.Add(svcregistry.NewService(service.NameAppProvider, []svcregistry.Node{node})); err != nil {
			panic(err)
		}
	}
	service.SetGlobalRegistry(reg)
}

var catalogue = []map[string]any{
	{"mime_type": "text/json", "extension": "json", "name": "JSON File", "icon": "https://example.org/icons&file=json.png", "default_app": "TextEditor"},
	{"mime_type": "text/xml", "extension": "xml", "name": "XML File", "icon": "https://example.org/icons&file=xml.png", "default_app": "TextEditor"},
	{"mime_type": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "extension": "doc", "name": "Word File", "default_app": "MiscEditor"},
	{"mime_type": "application/vnd.oasis.opendocument.presentation", "extension": "odf", "name": "OpenDocument File", "default_app": "MiscEditor"},
	{"mime_type": "application/vnd.apple.installer+xml", "extension": "mpkg", "name": "Mpkg File", "default_app": "MiscEditor"},
	{"mime_type": "image/bmp", "extension": "bmp", "name": "Image File", "default_app": "ImageViewer"},
}

// newService builds the app registry service over the discovered providers.
func newService(t *testing.T) *svc {
	t.Helper()

	s, err := New(context.Background(), map[string]any{"mime_types": catalogue})
	require.NoError(t, err)
	return s.(*svc)
}

func Test_ListAppProviders(t *testing.T) {
	got, err := newService(t).ListAppProviders(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, rpcv1beta1.Code_CODE_OK, got.Status.Code)
	sort.Sort(ByAddress(got.Providers))

	type discovered struct {
		address   string
		mimeTypes []string
	}
	byName := map[string]discovered{}
	for _, p := range got.Providers {
		byName[p.Name] = discovered{address: p.Address, mimeTypes: p.MimeTypes}
	}
	assert.Equal(t, map[string]discovered{
		"ImageViewer": {"image appprovider addr", []string{"image/bmp"}},
		"MiscEditor": {"misc appprovider addr", []string{
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.oasis.opendocument.presentation",
			"application/vnd.apple.installer+xml",
		}},
		"TextEditor": {"text appprovider addr", []string{"text/json", "text/xml"}},
	}, byName)
}

func Test_GetAppProviders(t *testing.T) {
	tests := []struct {
		name       string
		search     *providerv1beta1.ResourceInfo
		wantCode   rpcv1beta1.Code
		wantAppFor string
	}{
		{
			name:       "simple",
			search:     &providerv1beta1.ResourceInfo{MimeType: "text/json"},
			wantCode:   rpcv1beta1.Code_CODE_OK,
			wantAppFor: "TextEditor",
		},
		{
			name:       "more obscure MimeType",
			search:     &providerv1beta1.ResourceInfo{MimeType: "application/vnd.apple.installer+xml"},
			wantCode:   rpcv1beta1.Code_CODE_OK,
			wantAppFor: "MiscEditor",
		},
		{
			name:     "not existing MimeType",
			search:   &providerv1beta1.ResourceInfo{MimeType: "doesnot/exist"},
			wantCode: rpcv1beta1.Code_CODE_INTERNAL,
		},
		{
			name:     "empty MimeType",
			search:   &providerv1beta1.ResourceInfo{MimeType: ""},
			wantCode: rpcv1beta1.Code_CODE_INTERNAL,
		},
		{
			name:     "no data in resource info",
			search:   &providerv1beta1.ResourceInfo{},
			wantCode: rpcv1beta1.Code_CODE_INTERNAL,
		},
		{
			name:     "not valid MimeType",
			search:   &providerv1beta1.ResourceInfo{MimeType: "this/type\\IS.not?VALID@all"},
			wantCode: rpcv1beta1.Code_CODE_INTERNAL,
		},
	}

	s := newService(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.GetAppProviders(context.Background(), &registrypb.GetAppProvidersRequest{ResourceInfo: tt.search})
			require.NoError(t, err)
			assert.Equal(t, tt.wantCode, got.Status.Code)

			if tt.wantAppFor == "" {
				assert.Nil(t, got.Providers)
				return
			}
			require.Len(t, got.Providers, 1)
			assert.Equal(t, tt.wantAppFor, got.Providers[0].Name)
		})
	}
}

// App providers join by running, so there is nothing to register.
func Test_AddAppProvider(t *testing.T) {
	got, err := newService(t).AddAppProvider(context.Background(), &registrypb.AddAppProviderRequest{
		Provider: &registrypb.ProviderInfo{Name: "SomeApp", Address: "some addr"},
	})
	require.NoError(t, err)
	assert.Equal(t, rpcv1beta1.Code_CODE_UNIMPLEMENTED, got.Status.Code)
}

func Test_GetDefaultAppProviderForMimeType(t *testing.T) {
	got, err := newService(t).GetDefaultAppProviderForMimeType(context.Background(),
		&registrypb.GetDefaultAppProviderForMimeTypeRequest{MimeType: "text/json"})
	require.NoError(t, err)

	assert.Equal(t, rpcv1beta1.Code_CODE_OK, got.Status.Code)
	assert.Equal(t, "TextEditor", got.Provider.Name)
	assert.Equal(t, "text appprovider addr", got.Provider.Address)
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
	}{
		{
			name: "a configured catalogue",
			m:    map[string]any{"mime_types": catalogue},
		},
		{
			name: "no catalogue at all",
			m:    map[string]any{},
		},
		{
			name: "extra not existing field in setting",
			m:    map[string]any{"mime_types": catalogue, "doesnotexist": "doesnotexist"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(context.Background(), tt.m)
			require.NoError(t, err)
			assert.NotNil(t, got)
		})
	}
}
