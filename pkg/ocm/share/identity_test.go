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

package share

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestGrantKeyEncoding(t *testing.T) {
	// a.example.org is 13 bytes, "42" is 2 bytes.
	raw := append([]byte{0, 0, 0, 13}, []byte("a.example.org")...)
	raw = append(raw, 0, 0, 0, 2, '4', '2')
	sum := sha256.Sum256(raw)
	want := hex.EncodeToString(sum[:])
	got, err := GrantKey("a.example.org", "42")
	if err != nil || got != want || len(got) != 64 || strings.ToLower(got) != got {
		t.Fatalf("key %q, want %q (%v)", got, want, err)
	}
	// The length prefixes keep field boundaries apart.
	k1, _ := GrantKey("a.example.org", "142")
	k2, _ := GrantKey("a.example.org1", "42")
	if k1 == k2 || k1 == got {
		t.Fatal("ambiguous encoding")
	}
	// The remote ID is byte-exact.
	if k, _ := GrantKey("a.example.org", "42 "); k == got {
		t.Fatal("remote ID was trimmed")
	}
	for _, bad := range [][2]string{
		{"", "42"}, {"a.example.org", ""},
		{strings.Repeat("a", MaxGrantFieldLength+1), "42"},
		{"a.example.org", strings.Repeat("4", MaxGrantFieldLength+1)},
	} {
		if _, err := GrantKey(bad[0], bad[1]); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestCanonicalGrantOrigin(t *testing.T) {
	if got, err := CanonicalGrantOrigin("A.Example.ORG"); err != nil || got != "a.example.org" {
		t.Fatalf("CanonicalGrantOrigin(%q) = %q, %v", "A.Example.ORG", got, err)
	}
	// A scheme or any other invalid part is rejected, never stripped.
	for _, bad := range []string{"https://a.example.org", strings.Repeat("a", 256)} {
		if _, err := CanonicalGrantOrigin(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
