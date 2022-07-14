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

package storagespace

import (
	"errors"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/utils"
)

func TestSplitStorageID(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			"storageid" + _storageIDDelimiter + "spaceid",
			[]string{"storageid", "spaceid"},
		},
		{
			"",
			nil,
		},
		{
			"spaceid",
			[]string{"", "spaceid"},
		},
	}

	for _, tt := range tests {
		storageID, spaceID := SplitStorageID(tt.input)
		switch {
		case tt.expected == nil:
			if spaceID != "" || storageID != "" {
				t.Errorf("Expected unwrap to return nil, got '%s' '%s'", spaceID, storageID)
			}
		case len(tt.expected) != 2:
			t.Error("testcase won't work with len(expected) != 2. Avoiding panic")
		case storageID != tt.expected[0]:
			t.Errorf("StorageID doesn't match, expected '%s' got '%s'", tt.expected[0], storageID)
		case spaceID != tt.expected[1]:
			t.Errorf("ProviderID doesn't match, expected '%s' got '%s'", tt.expected[1], spaceID)
		}
	}

}

func TestParseID(t *testing.T) {
	tests := []struct {
		input       string
		expected    provider.ResourceId
		expectedErr error
	}{
		{
			"spaceid" + _idDelimiter + "opaqueid",
			provider.ResourceId{SpaceId: "spaceid", OpaqueId: "opaqueid"},
			nil,
		},
		{
			"storageid" + _storageIDDelimiter + "spaceid" + _idDelimiter + "opaqueid",
			provider.ResourceId{StorageId: "storageid", SpaceId: "spaceid", OpaqueId: "opaqueid"},
			nil,
		},
		{
			"",
			provider.ResourceId{},
			ErrInvalidSpaceID,
		},
		{
			"spaceid",
			provider.ResourceId{SpaceId: "spaceid"},
			nil,
		},
	}

	for _, tt := range tests {
		rid, err := ParseID(tt.input)
		switch {
		case tt.expectedErr != nil:
			if !errors.Is(err, tt.expectedErr) {
				t.Errorf("Expected ParseID to return error %s, got %s", tt.expectedErr, err)
			}
		case rid.StorageId != tt.expected.StorageId:
			t.Errorf("StorageIDs don't match. Expected %s, got %s", tt.expected.StorageId, rid.StorageId)
		case rid.SpaceId != tt.expected.SpaceId:
			t.Errorf("SpaceIDs don't match. Expected %s, got %s", tt.expected.SpaceId, rid.SpaceId)
		case rid.OpaqueId != tt.expected.OpaqueId:
			t.Errorf("OpaqueIDs don't match. Expected %s, got %s", tt.expected.OpaqueId, rid.OpaqueId)
		}
	}

}

func TestFormatResourceID(t *testing.T) {
	expected := "spaceid" + _idDelimiter + "opaqueid"
	wrapped := FormatResourceID(provider.ResourceId{SpaceId: "spaceid", OpaqueId: "opaqueid"})

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestFormatStorageID(t *testing.T) {
	expected := "storageid" + _storageIDDelimiter + "spaceid"
	wrapped := FormatStorageID("storageid", "spaceid")

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestParseStorageSpaceReference(t *testing.T) {
	tests := []struct {
		sRef        string
		resourceID  *provider.ResourceId
		relPath     string
		expectedErr error
	}{
		{
			"1234$abcd!5678/f1/f2",
			&provider.ResourceId{
				StorageId: "1234",
				SpaceId:   "abcd",
				OpaqueId:  "5678",
			},
			"./f1/f2",
			nil,
		},
		{
			"1234!abcd",
			&provider.ResourceId{
				SpaceId:  "1234",
				OpaqueId: "abcd",
			},
			".",
			nil,
		},
		{
			"01234$56789!abcd",
			&provider.ResourceId{
				StorageId: "01234",
				SpaceId:   "56789",
				OpaqueId:  "abcd",
			},
			".",
			nil,
		},
		{
			"",
			nil,
			"",
			ErrInvalidSpaceID,
		},
		{
			"01234$abcd",
			&provider.ResourceId{
				StorageId: "01234",
				SpaceId:   "abcd",
				OpaqueId:  "",
			},
			".",
			nil,
		},
		{
			"01234$!5678",
			&provider.ResourceId{
				StorageId: "01234",
				SpaceId:   "",
				OpaqueId:  "5678",
			},
			".",
			nil,
		},
		{
			"/f1/f2",
			nil,
			"",
			ErrInvalidSpaceID,
		},
	}
	for _, tt := range tests {
		ref, err := ParseReference(tt.sRef)

		if !errors.Is(err, tt.expectedErr) {
			t.Errorf("Expected error %s got %s", tt.expectedErr, err)
		}
		if ref.ResourceId != nil && !utils.ResourceIDEqual(ref.ResourceId, tt.resourceID) {
			t.Errorf("Expected ResourceID %s got %s", tt.resourceID, ref.ResourceId)
		}
		if ref.ResourceId == nil && tt.resourceID != nil {
			t.Errorf("Expected ResourceID to be %s got %s", tt.resourceID, ref.ResourceId)
		}
		if ref.Path != tt.relPath {
			t.Errorf("Expected path %s got %s", tt.relPath, ref.Path)
		}
	}
}

func TestFormatStorageSpaceReference(t *testing.T) {
	tests := []struct {
		ref         *provider.Reference
		expected    string
		expectedErr error
	}{
		{
			ref:         &provider.Reference{},
			expected:    "",
			expectedErr: ErrInvalidSpaceReference,
		},
		{
			ref:         &provider.Reference{ResourceId: &provider.ResourceId{}},
			expected:    "!",
			expectedErr: ErrInvalidSpaceReference,
		},
		{
			ref:         &provider.Reference{ResourceId: &provider.ResourceId{OpaqueId: "opaqueid"}, Path: "path"},
			expectedErr: ErrInvalidSpaceReference,
		},
		{
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid"}, Path: "path"},
			expected: "spaceid/path",
		},
		{
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid", OpaqueId: "opaqueid"}, Path: "path"},
			expected: "spaceid!opaqueid/path",
		},
		{
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", SpaceId: "spaceid", OpaqueId: "opaqueid"}, Path: "path"},
			expected: "storageid$spaceid!opaqueid/path",
		},
		{
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid", OpaqueId: "opaqueid"}, Path: "."},
			expected: "spaceid!opaqueid",
		},
		{
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid"}, Path: "."},
			expected: "spaceid",
		},
		{
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid"}},
			expected: "spaceid",
		},
	}

	for _, tt := range tests {
		result, err := FormatReference(tt.ref)
		if err != nil && !errors.Is(err, tt.expectedErr) {
			t.Errorf("FormateStorageSpaceRefence returned unexpected error: %v", err)
		}
		if err == nil && result != tt.expected {
			t.Errorf("Reference %v got formatted to %s, expected %s", tt.ref, result, tt.expected)
		}
	}
}

func TestFormatAndParseReference(t *testing.T) {
	tests := []struct {
		orig     *provider.Reference
		expected *provider.Reference
	}{
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid"}, Path: "./path"},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid"}, Path: "./path"},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", SpaceId: "spaceid"}, Path: "./path"},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", SpaceId: "spaceid"}, Path: "./path"},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid", OpaqueId: "opaqueid"}, Path: "./path"},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid", OpaqueId: "opaqueid"}, Path: "./path"},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", SpaceId: "spaceid", OpaqueId: "opaqueid"}, Path: "./path"},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", SpaceId: "spaceid", OpaqueId: "opaqueid"}, Path: "./path"},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid", OpaqueId: "opaqueid"}, Path: "."},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid", OpaqueId: "opaqueid"}, Path: "."},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid"}, Path: "."},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid"}, Path: "."},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid"}},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: "spaceid"}, Path: "."},
		},
	}

	for _, tt := range tests {
		formatted, _ := FormatReference(tt.orig)
		parsed, err := ParseReference(formatted)
		if err != nil {
			t.Errorf("failed to parse space reference: %s error: %s", formatted, err)
		}
		if !(utils.ResourceIDEqual(parsed.ResourceId, tt.expected.ResourceId) && parsed.Path == tt.expected.Path) {
			t.Errorf("Formatted then parsed references don't match the original got: %v expected %v", parsed, tt.expected)
		}
	}
}

func TestUpdateLegacyResourceID(t *testing.T) {
	tests := []struct {
		orig     provider.ResourceId
		expected provider.ResourceId
	}{
		{
			orig:     provider.ResourceId{StorageId: "storageid", SpaceId: "spaceid", OpaqueId: "opaqueid"},
			expected: provider.ResourceId{StorageId: "storageid", SpaceId: "spaceid", OpaqueId: "opaqueid"},
		},
		{
			orig:     provider.ResourceId{StorageId: "storageid$spaceid", SpaceId: "", OpaqueId: "opaqueid"},
			expected: provider.ResourceId{StorageId: "storageid", SpaceId: "spaceid", OpaqueId: "opaqueid"},
		},
	}

	for _, tt := range tests {
		updated := UpdateLegacyResourceID(tt.orig)
		if !(utils.ResourceIDEqual(&updated, &tt.expected)) {
			t.Errorf("Updating resourceid failed, got: %v expected %v", updated, tt.expected)
		}
	}
}
