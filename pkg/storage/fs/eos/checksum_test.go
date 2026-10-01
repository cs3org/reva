// Copyright 2018-2026 CERN
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

package eos

import (
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
)

// Both spellings have to work: the binary client reads "adler", the gRPC one
// passes on the MGM's "adler32".
func TestChecksumFromEOS(t *testing.T) {
	tests := []struct {
		xsType string
		want   provider.ResourceChecksumType
	}{
		{"adler", provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32},
		{"adler32", provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32},
		{"ADLER32", provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32},
		{"md5", provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5},
		{"sha1", provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_SHA1},
		{"something-else", provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID},
		{"", provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID},
	}

	for _, tt := range tests {
		got := checksumFromEOS(&eosclient.Checksum{XSType: tt.xsType, XSSum: "762d090f"})
		if got.Type != tt.want {
			t.Errorf("checksumFromEOS(%q) type = %v, want %v", tt.xsType, got.Type, tt.want)
		}
		if got.Sum != "762d090f" {
			t.Errorf("checksumFromEOS(%q) sum = %q, want it passed through unchanged",
				tt.xsType, got.Sum)
		}
	}
}

// Leading zeros are part of the checksum: trimming them corrupted about one
// adler32 in 256.
func TestChecksumFromEOSKeepsLeadingZeros(t *testing.T) {
	got := checksumFromEOS(&eosclient.Checksum{XSType: "adler32", XSSum: "0062d090"})
	if got.Sum != "0062d090" {
		t.Errorf("sum = %q, want 0062d090 with its leading zeros intact", got.Sum)
	}
}
