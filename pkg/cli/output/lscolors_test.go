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
	"strings"
	"testing"
)

func TestParseLSColors(t *testing.T) {
	c := ParseLSColors("di=01;34:fi=00:*.tar=01;31:*.tar.gz=01;35")

	if got := c["di"]; got != "01;34" {
		t.Errorf("di = %q", got)
	}
	if got := c["*.tar"]; got != "01;31" {
		t.Errorf("*.tar = %q", got)
	}
}

// TestParseLSColorsSkipsRubbish: the variable is assembled by shell startup
// files, and one malformed entry must not cost the user every colour.
func TestParseLSColorsSkipsRubbish(t *testing.T) {
	c := ParseLSColors("di=01;34:nonsense::=orphan:fi=00")
	if c["di"] != "01;34" || c["fi"] != "00" {
		t.Errorf("good entries were lost: %+v", c)
	}
	if _, ok := c["nonsense"]; ok {
		t.Error("an entry with no = should be skipped")
	}
}

// TestParseLSColorsFallsBackToDefaults: ls colours directories even with
// LS_COLORS unset, so an empty value must not mean "no colour at all".
func TestParseLSColorsFallsBackToDefaults(t *testing.T) {
	for _, in := range []string{"", "   "} {
		if got := ParseLSColors(in)["di"]; got == "" {
			t.Errorf("ParseLSColors(%q) left directories uncoloured", in)
		}
	}
}

func TestApplyColoursByType(t *testing.T) {
	c := ParseLSColors("di=01;34:fi=00:*.pdf=00;31")

	dir := c.Apply("Documents", true)
	if !strings.HasPrefix(dir, "\033[01;34m") || !strings.HasSuffix(dir, "\033[0m") {
		t.Errorf("directory not wrapped in its colour: %q", dir)
	}
	if !strings.Contains(dir, "Documents") {
		t.Errorf("the name was lost: %q", dir)
	}

	pdf := c.Apply("report.pdf", false)
	if !strings.HasPrefix(pdf, "\033[00;31m") {
		t.Errorf("extension colour not applied: %q", pdf)
	}

	// fi=00 means "no colour", and must not emit an escape at all.
	if got := c.Apply("notes.txt", false); got != "notes.txt" {
		t.Errorf("a file with colour 00 should be left alone, got %q", got)
	}
}

// TestApplyPrefersTheLongestExtension: with both configured, a .tar.gz is not
// a .gz.
func TestApplyPrefersTheLongestExtension(t *testing.T) {
	c := ParseLSColors("*.gz=01;31:*.tar.gz=01;35")
	if got := c.Apply("archive.tar.gz", false); !strings.HasPrefix(got, "\033[01;35m") {
		t.Errorf("longest extension should win, got %q", got)
	}
}

// TestApplyMatchesExtensionCaseInsensitively, as ls does.
func TestApplyMatchesExtensionCaseInsensitively(t *testing.T) {
	c := ParseLSColors("*.pdf=00;31")
	if got := c.Apply("REPORT.PDF", false); !strings.HasPrefix(got, "\033[00;31m") {
		t.Errorf("uppercase extension should match, got %q", got)
	}
}

// TestApplyHonoursConfiguredReset: rs is part of the database, and a terminal
// configured with a non-default reset should get it.
func TestApplyHonoursConfiguredReset(t *testing.T) {
	if got := ParseLSColors("rs=0:di=01;34").Apply("d", true); !strings.HasSuffix(got, "\033[0m") {
		t.Errorf("reset = %q", got)
	}
}

// TestUnknownEntryIsNotColoured: no rule, no escape sequence.
func TestUnknownEntryIsNotColoured(t *testing.T) {
	c := ParseLSColors("di=01;34")
	if got := c.Apply("notes.txt", false); got != "notes.txt" {
		t.Errorf("a file with no rule should be plain, got %q", got)
	}
}
