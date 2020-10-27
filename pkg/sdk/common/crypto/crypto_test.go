/*
 * MIT License
 *
 * Copyright (c) 2020 Daniel Mueller
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

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
