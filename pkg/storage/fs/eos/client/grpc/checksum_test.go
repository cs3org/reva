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

package eosgrpc

import (
	"strings"
	"testing"
)

// The Fmd field is fixed width, so encoding all of it appends zeros that are
// not part of the checksum.
func TestChecksumHex(t *testing.T) {
	// A 32-byte field holding a 4-byte adler32, as the MGM sends it.
	field := make([]byte, 32)
	copy(field, []byte{0x76, 0x2d, 0x09, 0x0f})

	if got := checksumHex("adler32", field); got != "762d090f" {
		t.Errorf("checksumHex = %q, want 762d090f with no padding", got)
	}
	if got := checksumHex("adler", field); got != "762d090f" {
		t.Errorf("the binary client's spelling should work too, got %q", got)
	}
}

// Trimming the zeros off the end instead would eat a digit of any checksum
// ending in one.
func TestChecksumHexKeepsTrailingZerosOfTheChecksumItself(t *testing.T) {
	field := make([]byte, 32)
	copy(field, []byte{0x76, 0x2d, 0x00, 0x00})

	if got := checksumHex("adler32", field); got != "762d0000" {
		t.Errorf("checksumHex = %q, want 762d0000: those zeros are the checksum", got)
	}
}

// A type with no known width is encoded whole rather than guessed at.
func TestChecksumHexEncodesAnUnknownTypeWhole(t *testing.T) {
	field := make([]byte, 8)
	copy(field, []byte{0xaa, 0xbb})

	got := checksumHex("something-else", field)
	if got != "aabb000000000000" {
		t.Errorf("checksumHex = %q, want the whole field", got)
	}
	if strings.Count(got, "0") == 0 {
		t.Error("the padding should still be there for a type nobody knows the width of")
	}
}
