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
	"github.com/cs3org/reva/pkg/utils"
)

func BenchmarkWrap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = wrap("storageid", "opaqueid")
	}
}

func TestWrap(t *testing.T) {
	expected := "storageid" + idDelimiter + "opaqueid"
	wrapped := wrap("storageid", "opaqueid")

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestWrapResourceID(t *testing.T) {
	expected := "storageid" + idDelimiter + "opaqueid"
	wrapped := OwnCloudResourceIDWrap(&providerv1beta1.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"})

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func BenchmarkUnwrap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = unwrap("storageid" + idDelimiter + "opaqueid")
	}
}

func TestUnwrap(t *testing.T) {
	tests := []struct {
		input    string
		expected *providerv1beta1.ResourceId
	}{
		{
			"storageid" + idDelimiter + "opaqueid",
			&providerv1beta1.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"},
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

		if tt.expected == nil {
			if rid != nil {
				t.Errorf("Expected unwrap to return nil, got %v", rid)
			}
		} else if !utils.ResourceIDEqual(rid, tt.expected) {
			t.Error("StorageID or OpaqueID doesn't match")
		}
	}

}
