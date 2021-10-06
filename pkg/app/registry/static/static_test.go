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

package static

import (
	"context"
	"testing"

	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	"github.com/stretchr/testify/assert"
)

var (
	 ctx = context.Background()

	microsoftProvider = &registrypb.ProviderInfo{
		Address:     "localhost:19000",
		Name:        "Microsoft Office",
		Description: "MS office 365",
		Icon:        "https://msp2l1160225102310.blob.core.windows.net/ms-p2-l1-160225-1023-13-assets/office_365_icon_en-US.png",
		MimeTypes: []string{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation", "application/vnd.oasis.opendocument.text", "application/vnd.oasis.opendocument.spreadsheet",
			"application/vnd.oasis.opendocument.presentation", "application/pdf"},
	}

	collaboraProvider = &registrypb.ProviderInfo{
		Address:     "localhost:18000",
		Name:        "Collabora",
		Description: "Collabora office editing apps",
		Icon:        "https://www.collaboraoffice.com/wp-content/uploads/2019/01/CP-icon.png",
		MimeTypes: []string{"application/vnd.oasis.opendocument.text", "application/vnd.oasis.opendocument.spreadsheet",
			"application/vnd.oasis.opendocument.presentation", "text/markdown"},
	}

	codimdProvider = &registrypb.ProviderInfo{
		Address:     "localhost:17000",
		Name:        "CodiMD",
		Description: "App for markdown files",
		Icon:        "https://avatars.githubusercontent.com/u/48181221?s=200&v=4",
		MimeTypes:   []string{"text/markdown", "application/compressed-markdown"},
	}

mimeTypesForCreation = []string{"application/pdf", "application/vnd.oasis.opendocument.text", "application/vnd.oasis.opendocument.spreadsheet",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}

	testConfig = map[string]interface{}{
		"mime_types": map[string]interface{}{
			"application/pdf": map[string]string{
				"extension":   "pdf",
				"name":        "PDF",
				"description": "PDF document",
				"icon":        "",
			},
			"application/vnd.oasis.opendocument.text": map[string]string{
				"extension":   "odt",
				"name":        "Open Document",
				"description": "OpenDocument text document",
				"icon":        "",
				"default_app": "Collabora",
			},
			"application/vnd.oasis.opendocument.spreadsheet": map[string]string{
				"extension":   "ods",
				"name":        "Open Spreadsheet",
				"description": "OpenDocument spreadsheet document",
				"icon":        "",
				"default_app": "Collabora",
			},
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document": map[string]string{
				"extension":   "docx",
				"name":        "Word Document",
				"description": "Microsoft Word document",
				"icon":        "",
				"default_app": "Microsoft Office",
			},
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": map[string]string{
				"extension":   "xlsx",
				"name":        "Excel Spreadsheet",
				"description": "Microsoft Excel document",
				"icon":        "",
				"default_app": "Microsoft Office",
			},
		},
	}
)

func mimeTypeAllowedForCreation(mimeType string) bool {
	for _, m := range mimeTypesForCreation {
		if m == mimeType {
			return true
		}
	}
	return false
}

func TestWithoutMimeTypesConfig(t *testing.T) {
	manager, err := New(map[string]interface{}{})
	assert.Empty(t, err)

	err = manager.AddProvider(ctx, microsoftProvider)
	assert.Empty(t, err)

	err = manager.AddProvider(ctx, collaboraProvider)
	assert.Empty(t, err)

	mimeTypes, err := manager.ListSupportedMimeTypes(ctx)
	assert.Empty(t, err)
	assert.Equal(t, len(mimeTypes), 8)

	err = manager.AddProvider(ctx, codimdProvider)
	assert.Empty(t, err)

	providers, err := manager.FindProviders(ctx, "text/markdown")
	assert.Empty(t, err)
	assert.ElementsMatch(t, []*registrypb.ProviderInfo{collaboraProvider, codimdProvider}, providers)

	mimeTypes, err = manager.ListSupportedMimeTypes(ctx)
	assert.Empty(t, err)
	assert.Equal(t, len(mimeTypes), 9)

	// default app is not set
	_, err = manager.GetDefaultProviderForMimeType(ctx, "application/vnd.oasis.opendocument.text")
	assert.Equal(t, err.Error(), "error: not found: default application provider not set for mime type application/vnd.oasis.opendocument.text")
}

func TestWithConfiguredMimeTypes(t *testing.T) {
	manager, err := New(testConfig)
	assert.Empty(t, err)

	err = manager.AddProvider(ctx, microsoftProvider)
	assert.Empty(t, err)

	err = manager.AddProvider(ctx, collaboraProvider)
	assert.Empty(t, err)

	mimeTypes, err := manager.ListSupportedMimeTypes(ctx)
	assert.Empty(t, err)
	assert.Equal(t, len(mimeTypes), 8)
	for _, m := range mimeTypes {
		assert.Equal(t, m.AllowCreation, mimeTypeAllowedForCreation(m.MimeType))
	}

	err = manager.AddProvider(ctx, codimdProvider)
	assert.Empty(t, err)

	providers, err := manager.FindProviders(ctx, "application/vnd.oasis.opendocument.spreadsheet")
	assert.Empty(t, err)
	assert.ElementsMatch(t, []*registrypb.ProviderInfo{collaboraProvider, microsoftProvider}, providers)

	mimeTypes, err = manager.ListSupportedMimeTypes(ctx)
	assert.Empty(t, err)
	assert.Equal(t, len(mimeTypes), 9)
	for _, m := range mimeTypes {
		assert.Equal(t, m.AllowCreation, mimeTypeAllowedForCreation(m.MimeType))
	}

	// default app is set
	defaultAppSet, err := manager.GetDefaultProviderForMimeType(ctx, "application/vnd.oasis.opendocument.text")
	assert.Empty(t, err)
	assert.Equal(t, collaboraProvider, defaultAppSet)

	// default app is not set
	_, err = manager.GetDefaultProviderForMimeType(ctx, "application/compressed-markdown")
	assert.Equal(t, err.Error(), "error: not found: default application provider not set for mime type application/compressed-markdown")
}
