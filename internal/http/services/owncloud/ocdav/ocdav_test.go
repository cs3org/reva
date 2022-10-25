// Copyright 2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
package ocdav

import (
	"testing"

	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storagespace"
)

func TestWrapResourceID(t *testing.T) {
	expected := "storageid" + "$" + "spaceid" + "!" + "opaqueid"
	wrapped := storagespace.FormatResourceID(providerv1beta1.ResourceId{StorageId: "storageid", SpaceId: "spaceid", OpaqueId: "opaqueid"})

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestNameNotEmptyRule(t *testing.T) {
	tests := map[string]bool{
		"":      false,
		" ":     false,
		"\n":    false,
		"name":  true,
		"empty": true,
	}

	rule := nameNotEmpty{}
	for name, expected := range tests {
		actual := rule.Test(name)
		if actual != expected {
			t.Errorf("For name %s the rule returned %t expected %t", name, actual, expected)
		}
	}
}

func TestNameDoesNotContainRule(t *testing.T) {
	tests := []struct {
		excludedChars string
		tests         map[string]bool
	}{
		{
			"a",
			map[string]bool{
				"foo": true,
				"bar": false,
			},
		},
		{
			"ab",
			map[string]bool{
				"foo": true,
				"bar": false,
				"car": false,
				"bor": false,
			},
		},
	}

	for _, tt := range tests {
		rule := nameDoesNotContain{chars: tt.excludedChars}
		for name, expected := range tt.tests {
			actual := rule.Test(name)
			if actual != expected {
				t.Errorf("For name %s the rule returned %t expected %t", name, actual, expected)
			}
		}
	}

}
