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
	"net/netip"
	"strings"
	"unicode/utf8"
)

var (
	errAddress = errors.New("not an OCM address")
	errHost    = errors.New("not an OCM host")
)

// ParseOCMAddress splits identifier "@" host [":" port] at the last "@".
// The identifier is returned byte for byte; the host is validated and lowercased.
func ParseOCMAddress(raw string) (id, host string, err error) {
	i := strings.LastIndexByte(raw, '@')
	if i <= 0 || !utf8.ValidString(raw[:i]) {
		return "", "", errAddress
	}
	host, err = CanonicalOCMHost(raw[i+1:])
	if err != nil {
		return "", "", errAddress
	}
	return raw[:i], host, nil
}

// CanonicalOCMHost validates host [":" port] with the RFC 3986 host grammar
// and lowercases it. It keeps the spelling otherwise, so that grant keys do not change.
func CanonicalOCMHost(raw string) (string, error) {
	name, port := raw, ""
	if strings.HasPrefix(raw, "[") {
		end := strings.IndexByte(raw, ']')
		if end < 0 || !validIPLiteral(raw[1:end]) {
			return "", errHost
		}
		name, port = raw[:end+1], raw[end+1:]
		if port != "" && port[0] != ':' {
			return "", errHost
		}
	} else {
		if i := strings.IndexByte(raw, ':'); i >= 0 {
			name, port = raw[:i], raw[i:]
		}
		if !validRegName(name) {
			return "", errHost
		}
	}
	if port != "" && strings.Trim(port[1:], "0123456789") != "" {
		return "", errHost
	}
	return strings.ToLower(raw), nil
}

func validIPLiteral(s string) bool {
	if strings.HasPrefix(s, "v") || strings.HasPrefix(s, "V") {
		version, suffix, ok := strings.Cut(s[1:], ".")
		return ok && version != "" && strings.Trim(version, hexDigits) == "" &&
			suffix != "" && strings.Trim(suffix, unreserved+subDelims+":") == ""
	}
	// An IP literal is IPv6 without a zone; bracketed IPv4 is not valid.
	ip, err := netip.ParseAddr(s)
	return err == nil && ip.Is6() && ip.Zone() == ""
}

const (
	hexDigits  = "0123456789abcdefABCDEF"
	unreserved = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	subDelims  = "!$&'()*+,;="
)

func validRegName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '%':
			if i+2 >= len(s) || strings.IndexByte(hexDigits, s[i+1]) < 0 || strings.IndexByte(hexDigits, s[i+2]) < 0 {
				return false
			}
			i += 2
		case strings.IndexByte(unreserved+subDelims, s[i]) < 0:
			return false
		}
	}
	return true
}
