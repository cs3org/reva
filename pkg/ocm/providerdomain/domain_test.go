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
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	tooLong := strings.Repeat("a", 63) + "." +
		strings.Repeat("b", 63) + "." +
		strings.Repeat("c", 63) + "." +
		strings.Repeat("d", 62)
	if len(tooLong) != 254 {
		t.Fatalf("too-long fixture length %d", len(tooLong))
	}
	labelTooLong := strings.Repeat("a", 64) + ".example"

	tests := []struct {
		name    string
		raw     string
		valid   bool
		wantErr error
	}{
		{name: "lower receiver", raw: "receiver.example.test", valid: true},
		{name: "two labels", raw: "a.b", valid: true},
		{name: "cernbox", raw: "cernbox.example.test", valid: true},
		{name: "cesnet", raw: "cesnet.example.test", valid: true},
		{name: "mixed case", raw: "Receiver.Example.Test", valid: true},
		{name: "mixed CERNBox", raw: "CERNBox.Example.Test", valid: true},
		{name: "multi label", raw: "a.b.c.d", valid: true},
		{name: "digit label", raw: "host1.example.test", valid: true},
		{name: "interior hyphen", raw: "my-host.example.test", valid: true},

		{name: "empty", raw: "", wantErr: errRequired},
		{name: "space", raw: " ", wantErr: errSpace},
		{name: "tab", raw: "\t", wantErr: errSpace},
		{name: "mixed whitespace", raw: " \t ", wantErr: errSpace},
		{name: "interior space", raw: "receiver .example.test", wantErr: errSpace},
		{name: "trailing space", raw: "receiver.example.test ", wantErr: errSpace},
		{name: "leading space", raw: " receiver.example.test", wantErr: errSpace},
		{name: "non-ascii", raw: "r\u00EBceiver.example.test", wantErr: errASCII},
		{name: "https scheme", raw: "https://receiver.example.test", wantErr: errHostOnly},
		{name: "http scheme", raw: "http://x.example.test", wantErr: errHostOnly},
		{name: "port", raw: "receiver.example.test:443", wantErr: errHostOnly},
		{name: "ip and port", raw: "127.0.0.1:54321", wantErr: errHostOnly},
		{name: "path", raw: "receiver.example.test/ocm", wantErr: errHostOnly},
		{name: "trailing slash", raw: "receiver.example.test/", wantErr: errHostOnly},
		{name: "query", raw: "receiver.example.test?x=1", wantErr: errHostOnly},
		{name: "fragment", raw: "receiver.example.test#frag", wantErr: errHostOnly},
		{name: "userinfo", raw: "user@receiver.example.test", wantErr: errHostOnly},
		{name: "ipv4 documentation", raw: "192.0.2.10", wantErr: errIP},
		{name: "ipv4 loopback", raw: "127.0.0.1", wantErr: errIP},
		{name: "ipv6 loopback", raw: "::1", wantErr: errHostOnly},
		{name: "ipv6 link local", raw: "fe80::1", wantErr: errHostOnly},
		{name: "single label", raw: "receiver", wantErr: errSingleLabel},
		{name: "localhost", raw: "localhost", wantErr: errSingleLabel},
		{name: "trailing dot", raw: "receiver.example.test.", wantErr: errEmptyLabel},
		{name: "leading dot", raw: ".receiver.example.test", wantErr: errEmptyLabel},
		{name: "empty middle label", raw: "receiver..example.test", wantErr: errEmptyLabel},
		{name: "too long", raw: tooLong, wantErr: errTooLong},
		{name: "invalid label char", raw: "re_ceiver.example.test", wantErr: errBadLabel},
		{name: "leading hyphen", raw: "-receiver.example.test", wantErr: errBadLabel},
		{name: "trailing hyphen", raw: "receiver-.example.test", wantErr: errBadLabel},
		{name: "label too long", raw: labelTooLong, wantErr: errBadLabel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(tt.raw)
			if tt.valid && err != nil {
				t.Fatalf("Validate error = %v", err)
			}
			if !tt.valid {
				if err == nil {
					t.Fatal("expected error")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				if strings.TrimSpace(tt.raw) != "" && strings.Contains(err.Error(), tt.raw) {
					t.Fatalf("error %q leaks raw input %q", err.Error(), tt.raw)
				}
			}
		})
	}
}
