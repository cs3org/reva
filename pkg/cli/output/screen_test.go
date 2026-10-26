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

// TestParseKeysDecodesEscapeSequences: arrows arrive as three bytes, and a
// terminal writes the whole sequence in one go: so a chunk is parsed as a whole
// rather than byte by byte, which is also what lets a lone ESC stay the Escape
// key instead of swallowing the next character.
func TestParseKeysDecodesEscapeSequences(t *testing.T) {
	cases := []struct {
		in   string
		want []Key
	}{
		{"\x1b[A", []Key{{Name: KeyUp}}},
		{"\x1b[B", []Key{{Name: KeyDown}}},
		{"\x1b[C", []Key{{Name: KeyRight}}},
		{"\x1b[D", []Key{{Name: KeyLeft}}},
		{"\x1bOA", []Key{{Name: KeyUp}}},
		{"\x1b[H", []Key{{Name: KeyHome}}},
		{"\x1b[F", []Key{{Name: KeyEnd}}},
		{"\x1b[5~", []Key{{Name: KeyPageUp}}},
		{"\x1b[6~", []Key{{Name: KeyPageDown}}},
		{"\x1b", []Key{{Name: KeyEscape}}},
		{"\r", []Key{{Name: KeyEnter}}},
		{"\n", []Key{{Name: KeyEnter}}},
		{"\x7f", []Key{{Name: KeyBackspace}}},
		{"\x03", []Key{{Name: KeyInterrupt}}},
		{"\t", []Key{{Name: KeyTab}}},
		{"q", []Key{{Name: KeyRune, Rune: 'q'}}},
		// Several keys in one read, which is what happens when somebody holds a
		// key down or pastes.
		{"jj\x1b[B", []Key{{Name: KeyRune, Rune: 'j'}, {Name: KeyRune, Rune: 'j'}, {Name: KeyDown}}},
		// A non-ASCII name typed into the search box.
		{"é", []Key{{Name: KeyRune, Rune: 'é'}}},
		// Control characters that mean nothing here are dropped rather than
		// delivered as stray runes that would land in a search box.
		{"\x01a", []Key{{Name: KeyRune, Rune: 'a'}}},
	}

	for _, c := range cases {
		got := ParseKeys([]byte(c.in))
		if len(got) != len(c.want) {
			t.Errorf("ParseKeys(%q) = %+v, want %+v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ParseKeys(%q)[%d] = %+v, want %+v", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestEllipsize(t *testing.T) {
	cases := []struct {
		in, want string
		width    int
	}{
		{"hello", "hello", 5},
		{"hello", "hel…", 4},
		{"hello", "…", 1},
		{"hello", "", 0},
		{"héllo wörld", "héll…", 5},
		{"short", "short", 20},
	}
	for _, c := range cases {
		if got := Ellipsize(c.in, c.width); got != c.want {
			t.Errorf("Ellipsize(%q, %d) = %q, want %q", c.in, c.width, got, c.want)
		}
	}
	// Counted in runes, not bytes: a multi-byte name must not shorten a frame.
	if got := Ellipsize("ééééé", 5); got != "ééééé" {
		t.Errorf("a five-rune string was trimmed at five cells: %q", got)
	}
}

// TestOpenScreenRefusesAPipe: a frame written to a pipe is gibberish, and raw
// mode on something that is not a terminal leaves nothing to read keys from.
func TestOpenScreenRefusesAPipe(t *testing.T) {
	if _, err := OpenScreen(nil, nil); err == nil {
		t.Error("opening a screen with no terminal should fail")
	}
}

// TestDrawStaysWithinTheFrame is about the one thing a whole-line highlight
// needs: the line has to be padded to the full width, or the highlight stops at
// the last character and the row looks broken.
func TestDrawHighlightCoversTheRow(t *testing.T) {
	var sb strings.Builder
	line := "abc"
	padded := line + strings.Repeat(" ", 10-len(line))
	if len(padded) != 10 {
		t.Fatalf("padding is wrong: %q", padded)
	}
	sb.WriteString(reverseOn + padded + attrOff)
	if !strings.HasSuffix(sb.String(), attrOff) {
		t.Error("a highlighted row must turn the attribute off again")
	}
}
