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

package micro

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/v2/pkg/errtypes"
)

func TestFindProviders(t *testing.T) {

	testCases := []struct {
		name         string
		mimeTypes    []*mimeTypeConfig
		regProviders []*registrypb.ProviderInfo
		mimeType     string
		expectedRes  []*registrypb.ProviderInfo
		expectedErr  error
	}{
		//{
		//	name:        "no mime types registered",
		//	mimeTypes:   []*mimeTypeConfig{},
		//	mimeType:    "SOMETHING",
		//	expectedErr: errtypes.NotFound("application provider not found for mime type SOMETHING"),
		//},
		{
			name: "one provider registered for one mime type",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "some Address",
				},
			},
			regProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "127.0.0.1:9725",
					Name:      "some Name",
				},
			},
			mimeType:    "text/json",
			expectedErr: nil,
			expectedRes: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "some Address",
					Name:      "some Name",
				},
			},
		},
		//{
		//	name: "more providers registered for one mime type",
		//	mimeTypes: []*mimeTypeConfig{
		//		{
		//			MimeType:   "text/json",
		//			Extension:  "json",
		//			Name:       "JSON File",
		//			Icon:       "https://example.org/icons&file=json.png",
		//			DefaultApp: "provider2",
		//		},
		//	},
		//	regProviders: []*registrypb.ProviderInfo{
		//		{
		//			MimeTypes: []string{"text/json"},
		//			Address:   "ip-provider1",
		//			Name:      "provider1",
		//		},
		//		{
		//			MimeTypes: []string{"text/json"},
		//			Address:   "ip-provider2",
		//			Name:      "provider2",
		//		},
		//		{
		//			MimeTypes: []string{"text/json"},
		//			Address:   "ip-provider3",
		//			Name:      "provider3",
		//		},
		//	},
		//	mimeType:    "text/json",
		//	expectedErr: nil,
		//	expectedRes: []*registrypb.ProviderInfo{
		//		{
		//			MimeTypes: []string{"text/json"},
		//			Address:   "ip-provider1",
		//			Name:      "provider1",
		//		},
		//		{
		//			MimeTypes: []string{"text/json"},
		//			Address:   "ip-provider2",
		//			Name:      "provider2",
		//		},
		//		{
		//			MimeTypes: []string{"text/json"},
		//			Address:   "ip-provider3",
		//			Name:      "provider3",
		//		},
		//	},
		//},
		//{
		//	name: "more providers registered for different mime types",
		//	mimeTypes: []*mimeTypeConfig{
		//		{
		//			MimeType:   "text/json",
		//			Extension:  "json",
		//			Name:       "JSON File",
		//			Icon:       "https://example.org/icons&file=json.png",
		//			DefaultApp: "provider2",
		//		},
		//		{
		//			MimeType:   "text/xml",
		//			Extension:  "xml",
		//			Name:       "XML File",
		//			Icon:       "https://example.org/icons&file=xml.png",
		//			DefaultApp: "provider1",
		//		},
		//	},
		//	regProviders: []*registrypb.ProviderInfo{
		//		{
		//			MimeTypes: []string{"text/json", "text/xml"},
		//			Address:   "ip-provider1",
		//			Name:      "provider1",
		//		},
		//		{
		//			MimeTypes: []string{"text/json"},
		//			Address:   "ip-provider2",
		//			Name:      "provider2",
		//		},
		//		{
		//			MimeTypes: []string{"text/xml"},
		//			Address:   "ip-provider3",
		//			Name:      "provider3",
		//		},
		//	},
		//	mimeType:    "text/json",
		//	expectedErr: nil,
		//	expectedRes: []*registrypb.ProviderInfo{
		//		{
		//			MimeTypes: []string{"text/json", "text/xml"},
		//			Address:   "ip-provider1",
		//			Name:      "provider1",
		//		},
		//		{
		//			MimeTypes: []string{"text/json"},
		//			Address:   "ip-provider2",
		//			Name:      "provider2",
		//		},
		//	},
		//},
	}

	for _, tt := range testCases {

		t.Run(tt.name, func(t *testing.T) {

			ctx := context.TODO()

			registry, err := New(map[string]interface{}{
				"mime_types": tt.mimeTypes,
				"namespace":  "bazFoo",
			})
			if err != nil {
				t.Error("unexpected error creating the registry:", err)
			}

			// register all the providers
			for _, p := range tt.regProviders {
				err := registry.AddProvider(ctx, p)
				if err != nil {
					t.Error("unexpected error adding a new provider in the registry:", err)
				}
			}
			time.Sleep(time.Second)
			pr, _ := registry.ListProviders(ctx)
			fmt.Println(pr)
			providers, err := registry.FindProviders(ctx, tt.mimeType)

			// check that the error returned by FindProviders is the same as the expected
			if tt.expectedErr != err {
				t.Errorf("different error returned: got=%v expected=%v", err, tt.expectedErr)
			}

			if !providersEquals(providers, tt.expectedRes) {
				t.Errorf("providers list different from expected: \n\tgot=%v\n\texp=%v", providers, tt.expectedRes)
			}

		})

	}

}

// check that all providers in the two lists are equals
func providersEquals(l1, l2 []*registrypb.ProviderInfo) bool {
	if len(l1) != len(l2) {
		return false
	}

	for i := 0; i < len(l1); i++ {
		if !equalsProviderInfo(l1[i], l2[i]) {
			return false
		}
	}
	return true
}

func TestFindProvidersWithPriority(t *testing.T) {

	testCases := []struct {
		name         string
		mimeTypes    []*mimeTypeConfig
		regProviders []*registrypb.ProviderInfo
		mimeType     string
		expectedRes  []*registrypb.ProviderInfo
		expectedErr  error
	}{
		{
			name:        "no mime types registered",
			mimeTypes:   []*mimeTypeConfig{},
			mimeType:    "SOMETHING",
			expectedErr: errtypes.NotFound("application provider not found for mime type SOMETHING"),
		},
		{
			name: "one provider registered for one mime type",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "some Address",
				},
			},
			regProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "some Address",
					Name:      "some Name",
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"priority": {
								Decoder: "plain",
								Value:   []byte("100"),
							},
						},
					},
				},
			},
			mimeType:    "text/json",
			expectedErr: nil,
			expectedRes: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "some Address",
					Name:      "some Name",
				},
			},
		},
		{
			name: "more providers registered for one mime type",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "provider2",
				},
			},
			regProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider1",
					Name:      "provider1",
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"priority": {
								Decoder: "plain",
								Value:   []byte("10"),
							},
						},
					},
				},
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider2",
					Name:      "provider2",
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"priority": {
								Decoder: "plain",
								Value:   []byte("20"),
							},
						},
					},
				},
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider3",
					Name:      "provider3",
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"priority": {
								Decoder: "plain",
								Value:   []byte("5"),
							},
						},
					},
				},
			},
			mimeType:    "text/json",
			expectedErr: nil,
			expectedRes: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider2",
					Name:      "provider2",
				},
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider1",
					Name:      "provider1",
				},
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider3",
					Name:      "provider3",
				},
			},
		},
		{
			name: "more providers registered for different mime types",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "provider2",
				},
				{
					MimeType:   "text/xml",
					Extension:  "xml",
					Name:       "XML File",
					Icon:       "https://example.org/icons&file=xml.png",
					DefaultApp: "provider1",
				},
			},
			regProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json", "text/xml"},
					Address:   "ip-provider1",
					Name:      "provider1",
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"priority": {
								Decoder: "plain",
								Value:   []byte("5"),
							},
						},
					},
				},
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider2",
					Name:      "provider2",
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"priority": {
								Decoder: "plain",
								Value:   []byte("100"),
							},
						},
					},
				},
				{
					MimeTypes: []string{"text/xml"},
					Address:   "ip-provider3",
					Name:      "provider3",
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"priority": {
								Decoder: "plain",
								Value:   []byte("20"),
							},
						},
					},
				},
			},
			mimeType:    "text/json",
			expectedErr: nil,
			expectedRes: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider2",
					Name:      "provider2",
				},
				{
					MimeTypes: []string{"text/json", "text/xml"},
					Address:   "ip-provider1",
					Name:      "provider1",
				},
			},
		},
		{
			name: "more providers registered for different mime types2",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "provider2",
				},
				{
					MimeType:   "text/xml",
					Extension:  "xml",
					Name:       "XML File",
					Icon:       "https://example.org/icons&file=xml.png",
					DefaultApp: "provider1",
				},
			},
			regProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json", "text/xml"},
					Address:   "ip-provider1",
					Name:      "provider1",
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"priority": {
								Decoder: "plain",
								Value:   []byte("5"),
							},
						},
					},
				},
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider2",
					Name:      "provider2",
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"priority": {
								Decoder: "plain",
								Value:   []byte("100"),
							},
						},
					},
				},
				{
					MimeTypes: []string{"text/xml"},
					Address:   "ip-provider3",
					Name:      "provider3",
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"priority": {
								Decoder: "plain",
								Value:   []byte("20"),
							},
						},
					},
				},
			},
			mimeType:    "text/xml",
			expectedErr: nil,
			expectedRes: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/xml"},
					Address:   "ip-provider3",
					Name:      "provider3",
				},
				{
					MimeTypes: []string{"text/json", "text/xml"},
					Address:   "ip-provider1",
					Name:      "provider1",
				},
			},
		},
	}

	for _, tt := range testCases {

		t.Run(tt.name, func(t *testing.T) {

			ctx := context.TODO()

			registry, err := New(map[string]interface{}{
				"mime_types": tt.mimeTypes,
			})
			if err != nil {
				t.Error("unexpected error creating the registry:", err)
			}

			// register all the providers
			for _, p := range tt.regProviders {
				err := registry.AddProvider(ctx, p)
				if err != nil {
					t.Error("unexpected error adding a new provider in the registry:", err)
				}
			}

			providers, err := registry.FindProviders(ctx, tt.mimeType)

			// check that the error returned by FindProviders is the same as the expected
			if tt.expectedErr != err {
				t.Errorf("different error returned: got=%v expected=%v", err, tt.expectedErr)
			}

			if !providersEquals(providers, tt.expectedRes) {
				t.Errorf("providers list different from expected: \n\tgot=%v\n\texp=%v", providers, tt.expectedRes)
			}

		})

	}

}

func TestAddProvider(t *testing.T) {

	testCases := []struct {
		name              string
		mimeTypes         []*mimeTypeConfig
		newProvider       *registrypb.ProviderInfo
		expectedProviders map[string][]*registrypb.ProviderInfo
	}{
		{
			name:      "no mime types defined - no initial providers",
			mimeTypes: []*mimeTypeConfig{},
			newProvider: &registrypb.ProviderInfo{
				MimeTypes: []string{"text/json"},
				Address:   "ip-provider1",
				Name:      "provider1",
			},
			expectedProviders: map[string][]*registrypb.ProviderInfo{
				"text/json": {
					{
						MimeTypes: []string{"text/json"},
						Address:   "ip-provider1",
						Name:      "provider1",
					},
				},
			},
		},
		{
			name: "one mime types defined - no initial providers - registering provider is the default",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "provider1",
				},
			},
			newProvider: &registrypb.ProviderInfo{
				MimeTypes: []string{"text/json"},
				Address:   "ip-provider1",
				Name:      "provider1",
			},
			expectedProviders: map[string][]*registrypb.ProviderInfo{
				"text/json": {
					{
						MimeTypes: []string{"text/json"},
						Address:   "ip-provider2",
						Name:      "provider2",
					},
					{
						MimeTypes: []string{"text/json"},
						Address:   "ip-provider1",
						Name:      "provider1",
					},
				},
			},
		},
		{
			name: "one mime types defined - default already registered",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "provider2",
				},
			},
			newProvider: &registrypb.ProviderInfo{
				MimeTypes: []string{"text/json"},
				Address:   "ip-provider1",
				Name:      "provider1",
			},
			expectedProviders: map[string][]*registrypb.ProviderInfo{
				"text/json": {
					{
						MimeTypes: []string{"text/json"},
						Address:   "ip-provider2",
						Name:      "provider2",
					},
					{
						MimeTypes: []string{"text/json"},
						Address:   "ip-provider1",
						Name:      "provider1",
					},
				},
			},
		},
		{
			name: "register a provider already registered",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "provider2",
				},
			},
			newProvider: &registrypb.ProviderInfo{
				MimeTypes: []string{"text/json"},
				Address:   "ip-provider1",
				Name:      "provider1",
			},
			expectedProviders: map[string][]*registrypb.ProviderInfo{
				"text/json": {
					{
						MimeTypes: []string{"text/json"},
						Address:   "ip-provider2",
						Name:      "provider2",
					},
					{
						MimeTypes: []string{"text/json"},
						Address:   "ip-provider1",
						Name:      "provider1",
					},
				},
			},
		},
		{
			name: "register a provider already registered supporting more mime types",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "provider2",
				},
				{
					MimeType:   "text/xml",
					Extension:  "xml",
					Name:       "XML File",
					Icon:       "https://example.org/icons&file=xml.png",
					DefaultApp: "provider1",
				},
			},
			newProvider: &registrypb.ProviderInfo{
				MimeTypes: []string{"text/json", "text/xml"},
				Address:   "ip-provider1",
				Name:      "provider1",
			},
			expectedProviders: map[string][]*registrypb.ProviderInfo{
				"text/json": {
					{
						MimeTypes: []string{"text/json"},
						Address:   "ip-provider2",
						Name:      "provider2",
					},
					{
						MimeTypes: []string{"text/json", "text/xml"},
						Address:   "ip-provider1",
						Name:      "provider1",
					},
				},
				"text/xml": {
					{
						MimeTypes: []string{"text/json", "text/xml"},
						Address:   "ip-provider1",
						Name:      "provider1",
					},
				},
			},
		},
		{
			name: "register a provider already registered supporting less mime types",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "provider2",
				},
				{
					MimeType:   "text/xml",
					Extension:  "xml",
					Name:       "XML File",
					Icon:       "https://example.org/icons&file=xml.png",
					DefaultApp: "provider1",
				},
			},
			newProvider: &registrypb.ProviderInfo{
				MimeTypes: []string{"text/json"},
				Address:   "ip-provider1",
				Name:      "provider1",
			},
			expectedProviders: map[string][]*registrypb.ProviderInfo{
				"text/json": {
					{
						MimeTypes: []string{"text/json"},
						Address:   "ip-provider2",
						Name:      "provider2",
					},
					{
						MimeTypes: []string{"text/json"},
						Address:   "ip-provider1",
						Name:      "provider1",
					},
				},
				"text/xml": {},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {

			ctx := context.TODO()

			registry, err := New(map[string]interface{}{
				//"providers":  tt.initProviders,
				"mime_types": tt.mimeTypes,
			})
			if err != nil {
				t.Error("unexpected error creating the registry:", err)
			}

			err = registry.AddProvider(ctx, tt.newProvider)
			if err != nil {
				t.Error("unexpected error adding a new provider:", err)
			}

			// test that the internal set of providers keep the new provider
			// and the key is the provider's address
			microReg := registry.(*manager)
			/*
				in, ok := microReg.providers[tt.newProvider.Address]
				if !ok {
					t.Error("cannot find a provider in the internal map with address", tt.newProvider.Address)
				}

				// check that the provider in the set is the same as the new one
				if !equalsProviderInfo(tt.newProvider, in) {
					t.Errorf("providers are different: got=%v expected=%v", in, tt.newProvider)
				}

			*/
			for mime, expAddrs := range tt.expectedProviders {
				mimeConfInterface, _ := microReg.mimetypes.Get(mime)
				addrsReg := mimeConfInterface.(*mimeTypeConfig).apps.getOrderedProviderByPriority()

				if !reflect.DeepEqual(expAddrs, addrsReg) {
					t.Errorf("list of addresses different from expected: \n\tgot=%v\n\texp=%v", addrsReg, expAddrs)
				}
			}

		})
	}

}

func TestListSupportedMimeTypes(t *testing.T) {
	testCases := []struct {
		name         string
		mimeTypes    []*mimeTypeConfig
		newProviders []*registrypb.ProviderInfo
		expected     []*registrypb.MimeTypeInfo
	}{
		{
			name: "one mime type - no provider registered",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "provider2",
				},
			},
			newProviders: []*registrypb.ProviderInfo{},
			expected: []*registrypb.MimeTypeInfo{
				{
					MimeType:           "text/json",
					Ext:                "json",
					AppProviders:       []*registrypb.ProviderInfo{},
					Name:               "JSON File",
					Icon:               "https://example.org/icons&file=json.png",
					DefaultApplication: "provider2",
				},
			},
		},
		{
			name: "one mime type - only default provider registered",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "provider1",
				},
			},
			newProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider1",
					Name:      "provider1",
				},
			},
			expected: []*registrypb.MimeTypeInfo{
				{
					MimeType: "text/json",
					Ext:      "json",
					AppProviders: []*registrypb.ProviderInfo{
						{
							MimeTypes: []string{"text/json"},
							Address:   "ip-provider1",
							Name:      "provider1",
						},
					},
					DefaultApplication: "provider1",
					Name:               "JSON File",
					Icon:               "https://example.org/icons&file=json.png",
				},
			},
		},
		{
			name: "one mime type - more providers",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "JSON_DEFAULT_PROVIDER",
				},
			},
			newProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider2",
					Name:      "NOT_DEFAULT_PROVIDER",
				},
				{
					MimeTypes: []string{"text/json"},
					Address:   "ip-provider1",
					Name:      "JSON_DEFAULT_PROVIDER",
				},
			},
			expected: []*registrypb.MimeTypeInfo{
				{
					MimeType: "text/json",
					Ext:      "json",
					AppProviders: []*registrypb.ProviderInfo{
						{
							MimeTypes: []string{"text/json"},
							Address:   "ip-provider2",
							Name:      "NOT_DEFAULT_PROVIDER",
						},
						{
							MimeTypes: []string{"text/json"},
							Address:   "ip-provider1",
							Name:      "JSON_DEFAULT_PROVIDER",
						},
					},
					DefaultApplication: "JSON_DEFAULT_PROVIDER",
					Name:               "JSON File",
					Icon:               "https://example.org/icons&file=json.png",
				},
			},
		},
		{
			name: "multiple mime types",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "JSON_DEFAULT_PROVIDER",
				},
				{
					MimeType:   "text/xml",
					Extension:  "xml",
					Name:       "XML File",
					Icon:       "https://example.org/icons&file=xml.png",
					DefaultApp: "XML_DEFAULT_PROVIDER",
				},
			},
			newProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json", "text/xml"},
					Address:   "1",
					Name:      "NOT_DEFAULT_PROVIDER2",
				},
				{
					MimeTypes: []string{"text/xml"},
					Address:   "2",
					Name:      "NOT_DEFAULT_PROVIDER1",
				},
				{
					MimeTypes: []string{"text/xml", "text/json"},
					Address:   "3",
					Name:      "JSON_DEFAULT_PROVIDER",
				},
				{
					MimeTypes: []string{"text/xml", "text/json"},
					Address:   "4",
					Name:      "XML_DEFAULT_PROVIDER",
				},
			},
			expected: []*registrypb.MimeTypeInfo{
				{
					MimeType: "text/json",
					Ext:      "json",
					AppProviders: []*registrypb.ProviderInfo{
						{
							MimeTypes: []string{"text/json", "text/xml"},
							Address:   "1",
							Name:      "NOT_DEFAULT_PROVIDER2",
						},
						{
							MimeTypes: []string{"text/xml", "text/json"},
							Address:   "3",
							Name:      "JSON_DEFAULT_PROVIDER",
						},
						{
							MimeTypes: []string{"text/xml", "text/json"},
							Address:   "4",
							Name:      "XML_DEFAULT_PROVIDER",
						},
					},
					DefaultApplication: "JSON_DEFAULT_PROVIDER",
					Name:               "JSON File",
					Icon:               "https://example.org/icons&file=json.png",
				},
				{
					MimeType: "text/xml",
					Ext:      "xml",
					AppProviders: []*registrypb.ProviderInfo{
						{
							MimeTypes: []string{"text/json", "text/xml"},
							Address:   "1",
							Name:      "NOT_DEFAULT_PROVIDER2",
						},
						{
							MimeTypes: []string{"text/xml"},
							Address:   "2",
							Name:      "NOT_DEFAULT_PROVIDER1",
						},
						{
							MimeTypes: []string{"text/xml", "text/json"},
							Address:   "3",
							Name:      "JSON_DEFAULT_PROVIDER",
						},
						{
							MimeTypes: []string{"text/xml", "text/json"},
							Address:   "4",
							Name:      "XML_DEFAULT_PROVIDER",
						},
					},
					DefaultApplication: "XML_DEFAULT_PROVIDER",
					Name:               "XML File",
					Icon:               "https://example.org/icons&file=xml.png",
				},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {

			ctx := context.TODO()

			registry, err := New(map[string]interface{}{
				"mime_types": tt.mimeTypes,
			})
			if err != nil {
				t.Error("unexpected error creating the registry:", err)
			}

			// add all the providers
			for _, p := range tt.newProviders {
				err = registry.AddProvider(ctx, p)
				if err != nil {
					t.Error("unexpected error creating adding new providers:", err)
				}
			}

			got, err := registry.ListSupportedMimeTypes(ctx)
			if err != nil {
				t.Error("unexpected error listing supported mime types:", err)
			}

			if !mimeTypesEquals(got, tt.expected) {
				t.Errorf("mime types list different from expected: \n\tgot=%v\n\texp=%v", got, tt.expected)
			}

		})
	}
}

func TestSetDefaultProviderForMimeType(t *testing.T) {
	testCases := []struct {
		name          string
		mimeTypes     []*mimeTypeConfig
		initProviders []*registrypb.ProviderInfo
		newDefault    struct {
			mimeType string
			provider *registrypb.ProviderInfo
		}
		newProviders []*registrypb.ProviderInfo
	}{
		{
			name: "set new default - no new providers",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "JSON_DEFAULT_PROVIDER",
				},
			},
			initProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "1",
					Name:      "JSON_DEFAULT_PROVIDER",
				},
				{
					MimeTypes: []string{"text/json"},
					Address:   "2",
					Name:      "NEW_DEFAULT",
				},
			},
			newDefault: struct {
				mimeType string
				provider *registrypb.ProviderInfo
			}{
				mimeType: "text/json",
				provider: &registrypb.ProviderInfo{
					MimeTypes: []string{"text/json"},
					Address:   "2",
					Name:      "NEW_DEFAULT",
				},
			},
			newProviders: []*registrypb.ProviderInfo{},
		},
		{
			name: "set default - other providers (one is the previous default)",
			mimeTypes: []*mimeTypeConfig{
				{
					MimeType:   "text/json",
					Extension:  "json",
					Name:       "JSON File",
					Icon:       "https://example.org/icons&file=json.png",
					DefaultApp: "JSON_DEFAULT_PROVIDER",
				},
			},
			initProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "4",
					Name:      "NO_DEFAULT_PROVIDER",
				},
				{
					MimeTypes: []string{"text/json"},
					Address:   "2",
					Name:      "NEW_DEFAULT",
				},
			},
			newDefault: struct {
				mimeType string
				provider *registrypb.ProviderInfo
			}{
				mimeType: "text/json",
				provider: &registrypb.ProviderInfo{
					MimeTypes: []string{"text/json"},
					Address:   "2",
					Name:      "NEW_DEFAULT",
				},
			},
			newProviders: []*registrypb.ProviderInfo{
				{
					MimeTypes: []string{"text/json"},
					Address:   "1",
					Name:      "JSON_DEFAULT_PROVIDER",
				},
				{
					MimeTypes: []string{"text/json"},
					Address:   "3",
					Name:      "OTHER_PROVIDER",
				},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {

			ctx := context.TODO()

			registry, err := New(map[string]interface{}{
				"providers":  tt.initProviders,
				"mime_types": tt.mimeTypes,
			})
			if err != nil {
				t.Error("unexpected error creating a new registry:", err)
			}

			err = registry.SetDefaultProviderForMimeType(ctx, tt.newDefault.mimeType, tt.newDefault.provider)
			if err != nil {
				t.Error("unexpected error setting a default provider for mime type:", err)
			}

			// add other provider to move things around internally :)
			for _, p := range tt.newProviders {
				err = registry.AddProvider(ctx, p)
				if err != nil {
					t.Error("unexpected error adding a new provider:", err)
				}
			}

			// check if the new default is the one set
			got, err := registry.GetDefaultProviderForMimeType(ctx, tt.newDefault.mimeType)
			if err != nil {
				t.Error("unexpected error getting the default app provider:", err)
			}

			if !equalsProviderInfo(got, tt.newDefault.provider) {
				t.Errorf("provider differ from expected:\n\tgot=%v\n\texp=%v", got, tt.newDefault.provider)
			}

		})
	}
}

func mimeTypesEquals(l1, l2 []*registrypb.MimeTypeInfo) bool {
	if len(l1) != len(l2) {
		return false
	}

	for i := 0; i < len(l1); i++ {
		if !equalsMimeTypeInfo(l1[i], l2[i]) {
			return false
		}
	}
	return true
}

func equalsMimeTypeInfo(m1, m2 *registrypb.MimeTypeInfo) bool {
	return m1.Description == m2.Description &&
		m1.AllowCreation == m2.AllowCreation &&
		providersEquals(m1.AppProviders, m2.AppProviders) &&
		m1.Ext == m2.Ext &&
		m1.MimeType == m2.MimeType &&
		m1.Name == m2.Name &&
		m1.DefaultApplication == m2.DefaultApplication
}
