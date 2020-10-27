// Copyright 2018-2020 CERN
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

package crypto_test

import (
	"fmt"
	"strings"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/pkg/sdk/common/crypto"
	testintl "github.com/cs3org/reva/pkg/sdk/common/testing"
)

func TestComputeChecksum(t *testing.T) {
	tests := map[string]struct {
		checksumType provider.ResourceChecksumType
		input        string
		wants        string
	}{
		"Unset":   {provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET, "Hello World!", ""},
		"Adler32": {provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32, "Hello World!", "1c49043e"},
		"SHA1":    {provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_SHA1, "Hello World!", "2ef7bde608ce5404e97d5f042f95f89f1c232871"},
		"MD5":     {provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5, "Hello World!", "ed076287532e86365e841e92bfc50d8c"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if checksum, err := crypto.ComputeChecksum(test.checksumType, strings.NewReader(test.input)); err == nil {
				if checksum != test.wants {
					t.Errorf(testintl.FormatTestResult("ComputeChecksum", test.wants, checksum, test.checksumType, test.input))
				}
			} else {
				t.Errorf(testintl.FormatTestError("ComputeChecksum", err))
			}
		})
	}

	// Check how ComputeChecksum reacts to an invalid checksum type
	if _, err := crypto.ComputeChecksum(provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID, nil); err == nil {
		t.Errorf(testintl.FormatTestError("ComputeChecksum", fmt.Errorf("accepted an invalid checksum type w/o erring"), provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID, nil))
	}
}

func TestGetChecksumTypeName(t *testing.T) {
	tests := map[provider.ResourceChecksumType]string{
		provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET:   "unset",
		provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_SHA1:    "sha1",
		provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32: "adler32",
		provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5:     "md5",
		provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID: "invalid",
	}

	for input, wants := range tests {
		if got := crypto.GetChecksumTypeName(input); got != wants {
			t.Errorf(testintl.FormatTestResult("GetChecksumTypeName", wants, got, input))
		}
	}
}
