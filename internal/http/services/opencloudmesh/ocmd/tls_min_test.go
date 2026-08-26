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

package ocmd

import (
	"crypto/tls"
	"errors"
	"testing"
)

func TestParseTLSMinVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    uint16
		wantErr bool
	}{
		{name: "empty defaults to TLS 1.2", in: "", want: tls.VersionTLS12},
		{name: "1.2", in: "1.2", want: tls.VersionTLS12},
		{name: "1.3", in: "1.3", want: tls.VersionTLS13},
		{name: "1.1 rejected", in: "1.1", wantErr: true},
		{name: "1.0 rejected", in: "1.0", wantErr: true},
		{name: "bogus rejected", in: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseTLSMinVersion(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseTLSMinVersion(%q) = %#x, want error", tt.in, got)
				}
				if !errors.Is(err, ErrInvalidTLSMinVersion) {
					t.Fatalf("ParseTLSMinVersion(%q) error = %v, want ErrInvalidTLSMinVersion", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTLSMinVersion(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseTLSMinVersion(%q) = %#x, want %#x", tt.in, got, tt.want)
			}
		})
	}
}
