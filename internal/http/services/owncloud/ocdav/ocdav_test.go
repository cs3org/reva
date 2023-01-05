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

package ocdav

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/utils/resourceid"
)

/*
The encodePath method as it is implemented currently is terribly inefficient.
As soon as there are a few special characters which need to be escaped the allocation count rises and the time spent too.
Adding more special characters increases the allocations and the time spent can rise up to a few milliseconds.
Granted this is not a lot on it's own but when a user has tens or hundreds of paths which need to be escaped and contain a few special characters
then this method alone will cost a huge amount of time.
*/
func BenchmarkEncodePath(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = encodePath("/some/path/Folder %^*(#1)")
	}
}

func TestWrapResourceID(t *testing.T) {
	expected := "storageid" + "!" + "opaqueid"
	wrapped := resourceid.OwnCloudResourceIDWrap(&providerv1beta1.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"})

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestExtractDestination(t *testing.T) {
	expected := "/dst"
	request := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/src", nil)
	request.Header.Set(HeaderDestination, "https://example.org/remote.php/dav/dst")

	ctx := context.WithValue(context.Background(), ctxKeyBaseURI, "remote.php/dav")
	destination, err := extractDestination(request.WithContext(ctx))
	if err != nil {
		t.Errorf("Expected err to be nil got %s", err)
	}

	if destination != expected {
		t.Errorf("Extracted destination is not expected, got %s want %s", destination, expected)
	}
}

func TestExtractDestinationWithoutHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/src", nil)

	_, err := extractDestination(request)
	if err == nil {
		t.Errorf("Expected err to be nil got %s", err)
	}

	if !errors.Is(err, errInvalidValue) {
		t.Errorf("Expected error invalid value, got %s", err)
	}
}

func TestExtractDestinationWithInvalidDestination(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/src", nil)
	request.Header.Set(HeaderDestination, "://example.org/remote.php/dav/dst")
	_, err := extractDestination(request)
	if err == nil {
		t.Errorf("Expected err to be nil got %s", err)
	}

	if !errors.Is(err, errInvalidValue) {
		t.Errorf("Expected error invalid value, got %s", err)
	}
}

func TestExtractDestinationWithDestinationWrongBaseURI(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/src", nil)
	request.Header.Set(HeaderDestination, "https://example.org/remote.php/dav/dst")

	ctx := context.WithValue(context.Background(), ctxKeyBaseURI, "remote.php/webdav")
	_, err := extractDestination(request.WithContext(ctx))
	if err == nil {
		t.Errorf("Expected err to be nil got %s", err)
	}

	if !errors.Is(err, errInvalidValue) {
		t.Errorf("Expected error invalid value, got %s", err)
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
