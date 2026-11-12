// Copyright 2018-2024 CERN
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

package providerdomain

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParseOCMAddress(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		valid    bool
		wantID   string
		wantHost string
	}{
		{"dns", "alice@a.example.org", true, "alice", "a.example.org"},
		{"id_case", "Alice@a.example.org", true, "Alice", "a.example.org"},
		{"host_case", "alice@A.EXAMPLE.ORG", true, "alice", "a.example.org"},
		{"ipv4", "alice@192.0.2.1", true, "alice", "192.0.2.1"},
		{"ipv6", "alice@[2001:db8::1]", true, "alice", "[2001:db8::1]"},
		{"ipv6_zero", "alice@[::]", true, "alice", "[::]"},
		{"ipv6_full", "alice@[2001:0DB8:0000:0000:0000:0000:0000:0001]", true, "alice", "[2001:0db8:0000:0000:0000:0000:0000:0001]"},
		{"ipv6_ipv4_tail", "alice@[::ffff:192.0.2.1]", true, "alice", "[::ffff:192.0.2.1]"},
		{"ipfuture_lower", "alice@[v1.a:b]", true, "alice", "[v1.a:b]"},
		{"ipfuture_upper", "alice@[V1.a:b]", true, "alice", "[v1.a:b]"},
		{"port", "alice@a.example.org:8443", true, "alice", "a.example.org:8443"},
		{"ipv6_port", "alice@[2001:db8::1]:8443", true, "alice", "[2001:db8::1]:8443"},
		{"empty_port", "alice@a.example.org:", true, "alice", "a.example.org:"},
		{"percent_host", "alice@reg%41.example", true, "alice", "reg%41.example"},
		{"opaque_percent", "alice%literal@a.example.org", true, "alice%literal", "a.example.org"},
		{"opaque_star", "alice*literal@a.example.org", true, "alice*literal", "a.example.org"},
		{"opaque_trailing_space", "alice @a.example.org", true, "alice ", "a.example.org"},
		{"opaque_leading_space", " alice@a.example.org", true, " alice", "a.example.org"},
		{"opaque_space_only", " @a.example.org", true, " ", "a.example.org"},
		{"opaque_at", "alice@example.net@a.example.org", true, "alice@example.net", "a.example.org"},
		{"opaque_at_only", "@@a.example.org", true, "@", "a.example.org"},
		{"opaque_unicode", "älice@a.example.org", true, "älice", "a.example.org"},
		{"opaque_decomposed_unicode", "a\u0308lice@a.example.org", true, "a\u0308lice", "a.example.org"},
		{"opaque_replacement_character", "\ufffd@a.example.org", true, "\ufffd", "a.example.org"},
		{"empty_id", "@a.example.org", false, "", ""},
		{"empty_address", "", false, "", ""},
		{"missing_separator", "a.example.org", false, "", ""},
		{"empty_host", "alice@", false, "", ""},
		{"host_space", "alice@a.example.org ", false, "", ""},
		{"host_lf", "alice@a.example.org\n", false, "", ""},
		{"host_crlf", "alice@a.example.org\r\n", false, "", ""},
		{"host_path", "alice@a.example.org/path", false, "", ""},
		{"host_query", "alice@a.example.org?q=x", false, "", ""},
		{"host_fragment", "alice@a.example.org#x", false, "", ""},
		{"host_scheme", "alice@https://a.example.org", false, "", ""},
		{"bad_port", "alice@a.example.org:bad", false, "", ""},
		{"hex_is_not_ipv6", "alice@[dead]", false, "", ""},
		{"bad_group_count", "alice@[1:2:3:4:5:6:7]", false, "", ""},
		{"too_many_groups", "alice@[1:2:3:4:5:6:7:8:9]", false, "", ""},
		{"double_compression", "alice@[2001::db8::1]", false, "", ""},
		{"bad_ipv4_tail", "alice@[::ffff:999.0.0.1]", false, "", ""},
		{"bracketed_ipv4", "alice@[192.0.2.1]", false, "", ""},
		{"ipv6_zone", "alice@[fe80::1%eth0]", false, "", ""},
		{"ipvfuture_empty_version", "alice@[v.a]", false, "", ""},
		{"ipvfuture_empty_suffix", "alice@[v1.]", false, "", ""},
		{"ipvfuture_bad_version", "alice@[vG.a]", false, "", ""},
		{"literal_trailing_junk", "alice@[::1]junk", false, "", ""},
		{"invalid_percent_host", "alice@reg%4Z.example", false, "", ""},
		{"unbracketed_ipv6", "alice@2001:db8::1", false, "", ""},
		{"opaque_matching_host_suffix", "alice@a.example.org@a.example.org", true, "alice@a.example.org", "a.example.org"},
		{"opaque_repeated_host_suffix", "alice@a.example.org@a.example.org@a.example.org", true, "alice@a.example.org@a.example.org", "a.example.org"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, host, err := ParseOCMAddress(c.value)
			if (err == nil) != c.valid {
				t.Fatalf("valid=%t, want %t", err == nil, c.valid)
			}
			if err != nil && err.Error() != "not an OCM address" {
				t.Errorf("error %q", err)
			}
			if id != c.wantID || host != c.wantHost {
				t.Errorf("id %q host %q, want id %q host %q", id, host, c.wantID, c.wantHost)
			}
		})
	}
}

// Syntax acceptance does not prove that a host can be used for discovery.
func TestCanonicalOCMHostCorpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/hosts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Value     string `json:"value"`
		Valid     bool   `json:"valid"`
		Canonical string `json:"canonical"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("empty host corpus")
	}
	for _, c := range cases {
		got, err := CanonicalOCMHost(c.Value)
		if (err == nil) != c.Valid || got != c.Canonical {
			t.Errorf("%q: %q %v, want valid=%t %q", c.Value, got, err, c.Valid, c.Canonical)
		}
		if err != nil && err.Error() != "not an OCM host" {
			t.Errorf("%q: error %q", c.Value, err)
		}
	}
}

func TestParseOCMAddressRejectsInvalidUTF8(t *testing.T) {
	for _, raw := range []string{"\xff@a.example.org", "alice\xc0\xaf@a.example.org", "alice\xe2\x82@a.example.org"} {
		id, host, err := ParseOCMAddress(raw)
		if err == nil || err.Error() != "not an OCM address" || id != "" || host != "" {
			t.Errorf("%q: id %q host %q error %v", raw, id, host, err)
		}
	}
}
