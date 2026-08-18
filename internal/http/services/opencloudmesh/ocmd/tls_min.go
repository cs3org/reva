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
	"fmt"
)

// ErrInvalidTLSMinVersion is returned when a per-service TLS minimum
// enum string is not empty, "1.2", or "1.3".
var ErrInvalidTLSMinVersion = errors.New("invalid TLS min version")

// ParseTLSMinVersion maps a per-service TLS minimum enum string to a
// crypto/tls version. Empty and "1.2" mean TLS 1.2; "1.3" means TLS 1.3.
func ParseTLSMinVersion(s string) (uint16, error) {
	switch s {
	case "", "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("%w %q: want \"1.2\" or \"1.3\"", ErrInvalidTLSMinVersion, s)
	}
}

func resolveTLSMinVersion(minVersion uint16) uint16 {
	if minVersion == 0 {
		return tls.VersionTLS12
	}
	if minVersion < tls.VersionTLS12 {
		panic(fmt.Sprintf("ocm transport TLS min version %#x is below TLS 1.2", minVersion))
	}
	return minVersion
}
