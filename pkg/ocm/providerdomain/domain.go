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
	"net"
	"strings"
	"unicode"
)

// Fixed reasons only. Callers keep the configured spelling, and these
// errors never include the rejected input.
var (
	errRequired = errors.New("provider_domain is required and must be a host-only DNS FQDN")
	errSpace    = errors.New("provider_domain must not contain whitespace")
	errASCII    = errors.New("provider_domain must be an ASCII DNS name")
	errHostOnly = errors.New(
		"provider_domain must be a host-only DNS FQDN without a scheme, " +
			"port, path, query, fragment, userinfo, or IP literal",
	)
	errIP          = errors.New("provider_domain must be a DNS name, not an IP address")
	errEmptyLabel  = errors.New("provider_domain has an empty DNS label")
	errTooLong     = errors.New("provider_domain is longer than 253 characters")
	errSingleLabel = errors.New("provider_domain is a single-label host")
	errBadLabel    = errors.New("provider_domain has an invalid DNS label")
)

// Validate checks that raw is an ASCII host-only DNS FQDN with at least two
// labels. It does not look up DNS, trim, or change case.
func Validate(raw string) error {
	if raw == "" {
		return errRequired
	}
	for _, r := range raw {
		if unicode.IsSpace(r) {
			return errSpace
		}
		if r > unicode.MaxASCII {
			return errASCII
		}
	}
	if strings.Contains(raw, "://") || strings.ContainsAny(raw, "/?#@:[") {
		return errHostOnly
	}
	if net.ParseIP(raw) != nil {
		return errIP
	}
	if strings.HasPrefix(raw, ".") || strings.HasSuffix(raw, ".") || strings.Contains(raw, "..") {
		return errEmptyLabel
	}
	if len(raw) > 253 {
		return errTooLong
	}
	labels := strings.Split(raw, ".")
	if len(labels) < 2 {
		return errSingleLabel
	}
	for _, label := range labels {
		if err := validateLabel(label); err != nil {
			return err
		}
	}
	return nil
}

func validateLabel(label string) error {
	if len(label) == 0 || len(label) > 63 {
		return errBadLabel
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-' && i > 0 && i < len(label)-1:
		default:
			return errBadLabel
		}
	}
	return nil
}
