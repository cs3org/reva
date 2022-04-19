// Copyright 2021 CERN
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
package resourceid

import (
	"testing"

	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/utils"
)

func BenchmarkWrap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = wrap("storageid", "opaqueid", "!")
	}
}

func TestWrap(t *testing.T) {
	expected := "storageid!opaqueid"
	wrapped := wrap("storageid", "opaqueid", "!")

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestWrapResourceID(t *testing.T) {
	expected := "storageid" + _idDelimiter + "opaqueid"
	wrapped := OwnCloudResourceIDWrap(&providerv1beta1.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"})

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestWrapStorageID(t *testing.T) {
	expected := "providerid" + _providerDelimiter + "storageid"
	wrapped := StorageIDWrap("storageid", "providerid")

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestUnwrap(t *testing.T) {
	expected := []string{"storageid", "opaqueid"}
	sid, oid, err := unwrap("storageid!opaqueid", "!")
	if err != nil {
		t.Errorf("unwrap returned an error: %v", err)
	}

	if sid != expected[0] || oid != expected[1] {
		t.Errorf("wrapped id doesn't have the expected format: got (%s, %s) expected %s", sid, oid, expected)
	}
}

func BenchmarkUnwrap(b *testing.B) {
	delimiter := "!"
	for i := 0; i < b.N; i++ {
		_, _, _ = unwrap("storageid"+delimiter+"opaqueid", delimiter)
	}
}

func TestUnwrapResourceID(t *testing.T) {
	tests := []struct {
		input    string
		expected *providerv1beta1.ResourceId
	}{
		{
			"storageid" + _idDelimiter + "opaqueid",
			&providerv1beta1.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"},
		},
		{
			"providerid" + _providerDelimiter + "storageid" + _idDelimiter + "opaqueid",
			&providerv1beta1.ResourceId{StorageId: "providerid$storageid", OpaqueId: "opaqueid"},
		},
		{
			"",
			nil,
		},
		{
			"c",
			nil,
		},
	}

	for _, tt := range tests {
		rid := OwnCloudResourceIDUnwrap(tt.input)
		switch {
		case tt.expected == nil:
			if rid != nil {
				t.Errorf("Expected unwrap to return nil, got %v", rid)
			}
		case !utils.ResourceIDEqual(rid, tt.expected):
			t.Errorf("StorageID or OpaqueID doesn't match. Expected %v, got %v", tt.expected, rid)
		}
	}

}

func TestUnwrapStorageID(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			"providerid" + _providerDelimiter + "storageid",
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
		sid, spid := StorageIDUnwrap(tt.input)
		switch {
		case tt.expected == nil:
			if sid != "" || spid != "" {
				t.Errorf("Expected unwrap to return nil, got '%s' '%s'", sid, spid)
			}
		case len(tt.expected) != 2:
			t.Error("testcase won't work with len(expected) != 2. Avoiding panic")
		case sid != tt.expected[0]:
			t.Errorf("StorageID doesn't match, expected '%s' got '%s'", tt.expected[0], sid)
		case spid != tt.expected[1]:
			t.Errorf("ProviderID doesn't match, expected '%s' got '%s'", tt.expected[1], spid)
		}
	}

}
