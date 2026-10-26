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

package output

import (
	"os"
	"strings"
)

// LSColors is a parsed LS_COLORS database: the same one GNU ls reads, so a
// listing honours whatever the user already configured with dircolors rather
// than inventing a second scheme they would have to learn.
//
// Keys are either a two-letter type ("di" for a directory, "fi" for a regular
// file) or an extension pattern ("*.tar"). Values are SGR parameters, "01;34"
// and the like.
type LSColors map[string]string

// defaultLSColors is what ls falls back to when LS_COLORS is unset: bold blue
// directories and little else. Matching that is better than showing no colour
// at all, and better than picking something of our own.
var defaultLSColors = LSColors{
	"di": "01;34",
	"ln": "01;36",
	"ex": "01;32",
}

// LSColorsFromEnv reads LS_COLORS, falling back to ls's own defaults.
//
// LSCOLORS, the unrelated BSD variable with a different syntax, is deliberately
// not read: honouring half of it would colour a macOS listing in ways its owner
// did not ask for.
func LSColorsFromEnv() LSColors {
	return ParseLSColors(os.Getenv("LS_COLORS"))
}

// ParseLSColors parses the LS_COLORS syntax: colon-separated key=value pairs.
// Malformed entries are skipped rather than rejected: the variable is
// assembled by shell startup files and one bad entry should not cost the user
// every colour.
func ParseLSColors(s string) LSColors {
	if strings.TrimSpace(s) == "" {
		return defaultLSColors
	}

	out := LSColors{}
	for entry := range strings.SplitSeq(s, ":") {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return defaultLSColors
	}
	return out
}

// code returns the SGR parameters for an entry, or "" for no colour.
func (c LSColors) code(name string, isDir bool) string {
	if isDir {
		return c["di"]
	}

	// Extensions win over the generic file colour, longest first so that
	// "*.tar.gz" beats "*.gz" when both are configured.
	best, bestLen := "", -1
	lower := strings.ToLower(name)
	for key, value := range c {
		if !strings.HasPrefix(key, "*.") {
			continue
		}
		suffix := key[1:] // "*.tar" -> ".tar"
		// ls matches the extension case-insensitively.
		if strings.HasSuffix(lower, strings.ToLower(suffix)) && len(suffix) > bestLen {
			best, bestLen = value, len(suffix)
		}
	}
	if bestLen >= 0 {
		return best
	}
	return c["fi"]
}

// Apply wraps name in the colour configured for it. It returns name unchanged
// when no colour applies, so the caller never has to special-case that: and
// crucially, the returned string is longer than the name it wraps, so column
// widths must be computed from the plain name.
func (c LSColors) Apply(name string, isDir bool) string {
	code := c.code(name, isDir)
	if code == "" || code == "0" || code == "00" {
		return name
	}
	reset := c["rs"]
	if reset == "" {
		reset = "0"
	}
	return "\033[" + code + "m" + name + "\033[" + reset + "m"
}
