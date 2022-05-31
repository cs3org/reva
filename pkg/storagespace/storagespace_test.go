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
			"providerid" + _storageIDDelimiter + "storageid",
			[]string{"storageid", "providerid"},
		},
		{
			"",
			nil,
		},
		{
			"storageid",
			[]string{"storageid", ""},
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
		case spaceID != tt.expected[0]:
			t.Errorf("StorageID doesn't match, expected '%s' got '%s'", tt.expected[0], spaceID)
		case storageID != tt.expected[1]:
			t.Errorf("ProviderID doesn't match, expected '%s' got '%s'", tt.expected[1], storageID)
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
			provider.ResourceId{StorageId: "spaceid", OpaqueId: "opaqueid"},
			nil,
		},
		{
			"providerid" + _storageIDDelimiter + "spaceid" + _idDelimiter + "opaqueid",
			provider.ResourceId{StorageId: "providerid$spaceid", OpaqueId: "opaqueid"},
			nil,
		},
		{
			"",
			provider.ResourceId{},
			ErrInvalidSpaceID,
		},
		{
			"spaceid",
			provider.ResourceId{StorageId: "spaceid", OpaqueId: "spaceid"},
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
			t.Errorf("StorageIDs don't match. Expected %v, got %v", tt.expected, rid)
		case rid.OpaqueId != tt.expected.OpaqueId:
			t.Errorf("StorageIDs don't match. Expected %v, got %v", tt.expected, rid)
		}
	}

}

func TestFormatResourceID(t *testing.T) {
	expected := "storageid" + _idDelimiter + "opaqueid"
	wrapped := FormatResourceID(provider.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"})

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestFormatStorageID(t *testing.T) {
	expected := "providerid" + _storageIDDelimiter + "spaceid"
	wrapped := FormatStorageID("providerid", "spaceid")

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestFormatId(t *testing.T) {
	tests := []struct {
		pid         string
		sid         string
		oid         string
		expectation string
	}{
		{
			"",
			"",
			"oid",
			"oid",
		},
		{
			"",
			"sid",
			"oid",
			"sid!oid",
		},
		{
			"pid",
			"sid",
			"oid",
			"pid$sid!oid",
		},
	}

	for _, tt := range tests {
		id := FormatID(tt.pid, tt.sid, tt.oid)

		if id != tt.expectation {
			t.Errorf("Expected id %s got %s", tt.expectation, id)
		}
	}
}

func TestParseStorageSpaceReference(t *testing.T) {
	tests := []struct {
		sRef      string
		storageID string
		nodeID    string
		relPath   string
	}{
		{
			"1234!abcd/f1/f2",
			"1234",
			"abcd",
			"./f1/f2",
		},
		{
			"1234!abcd",
			"1234",
			"abcd",
			".",
		},
		{
			"01234$56789!abcd",
			"01234$56789",
			"abcd",
			".",
		},
	}
	for _, tt := range tests {
		ref, _ := ParseReference(tt.sRef)

		if ref.ResourceId.StorageId != tt.storageID {
			t.Errorf("Expected storageId %s got %s", tt.storageID, ref.ResourceId.StorageId)
		}
		if ref.ResourceId.OpaqueId != tt.nodeID {
			t.Errorf("Expected OpaqueId %s got %s", tt.nodeID, ref.ResourceId.OpaqueId)
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
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid"}, Path: "path"},
			expected: "storageid/path",
		},
		{
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"}, Path: "path"},
			expected: "storageid!opaqueid/path",
		},
		{
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"}, Path: "."},
			expected: "storageid!opaqueid",
		},
		{
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid"}, Path: "."},
			expected: "storageid",
		},
		{
			ref:      &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid"}},
			expected: "storageid",
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
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid"}, Path: "./path"},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", OpaqueId: "storageid"}, Path: "./path"},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid$spaceid"}, Path: "./path"},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid$spaceid", OpaqueId: "spaceid"}, Path: "./path"},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"}, Path: "./path"},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"}, Path: "./path"},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid$spaceid", OpaqueId: "opaqueid"}, Path: "./path"},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid$spaceid", OpaqueId: "opaqueid"}, Path: "./path"},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"}, Path: "."},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"}, Path: "."},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid"}, Path: "."},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", OpaqueId: "storageid"}, Path: "."},
		},
		{
			orig:     &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid"}},
			expected: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "storageid", OpaqueId: "storageid"}, Path: "."},
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
