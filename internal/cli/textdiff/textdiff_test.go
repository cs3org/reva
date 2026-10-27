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

package textdiff

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func opts() Options {
	return Options{Context: 3, OldLabel: "old", NewLabel: "new"}
}

func TestIdenticalIsEmpty(t *testing.T) {
	if got := Unified("same\ntext\n", "same\ntext\n", opts()); got != "" {
		t.Errorf("identical texts produced a diff:\n%s", got)
	}
	if got := Unified("", "", opts()); got != "" {
		t.Errorf("two empty texts produced a diff:\n%s", got)
	}
}

func TestOneLineChanged(t *testing.T) {
	got := Unified("a\nb\nc\n", "a\nB\nc\n", opts())
	want := `--- old
+++ new
@@ -1,3 +1,3 @@
 a
-b
+B
 c
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAddedAndRemoved(t *testing.T) {
	// Added to an empty file.
	got := Unified("", "one\ntwo\n", opts())
	if !strings.Contains(got, "+one") || !strings.Contains(got, "+two") {
		t.Errorf("additions missing:\n%s", got)
	}
	if strings.Contains(got, "-") && !strings.Contains(got, "--- old") {
		t.Errorf("nothing was removed, so nothing should be marked so:\n%s", got)
	}

	// Emptied.
	got = Unified("one\ntwo\n", "", opts())
	if !strings.Contains(got, "-one") || !strings.Contains(got, "-two") {
		t.Errorf("removals missing:\n%s", got)
	}
}

// TestUnchangedStretchIsSkipped is the point of hunks: a change at each end of a
// long file must not print the whole file.
func TestUnchangedStretchIsSkipped(t *testing.T) {
	var a, b []string
	a = append(a, "first")
	b = append(b, "FIRST")
	for i := range 100 {
		line := fmt.Sprintf("line %d", i)
		a = append(a, line)
		b = append(b, line)
	}
	a = append(a, "last")
	b = append(b, "LAST")

	got := Unified(strings.Join(a, "\n")+"\n", strings.Join(b, "\n")+"\n", opts())

	// Counted on the opening marker: each header contains "@@" twice.
	if n := strings.Count(got, "@@ -"); n != 2 {
		t.Errorf("want two hunks, got %d:\n%s", n, got)
	}
	// The middle is untouched and must not be printed.
	if strings.Contains(got, "line 50") {
		t.Errorf("the unchanged middle was printed:\n%s", got)
	}
	for _, want := range []string{"-first", "+FIRST", "-last", "+LAST"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

// TestNearbyChangesShareAHunk: two changes closer together than twice the
// context belong in one hunk, or the output repeats the lines between them.
func TestNearbyChangesShareAHunk(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\ng\n"
	new := "A\nb\nc\nd\ne\nf\nG\n"
	got := Unified(old, new, Options{Context: 3, OldLabel: "old", NewLabel: "new"})
	if n := strings.Count(got, "@@ -"); n != 1 {
		t.Errorf("want one hunk for changes 6 lines apart with context 3, got %d:\n%s", n, got)
	}
}

// TestHunkHeaderCountsLines: the @@ header has to describe the hunk, because
// tools that consume a diff rely on it and a human reads it to find the place.
func TestHunkHeaderCountsLines(t *testing.T) {
	got := Unified("a\nb\nc\n", "a\nx\ny\nc\n", opts())
	if !strings.Contains(got, "@@ -1,3 +1,4 @@") {
		t.Errorf("header does not describe the hunk:\n%s", got)
	}
}

func TestContextZero(t *testing.T) {
	got := Unified("a\nb\nc\n", "a\nB\nc\n", Options{Context: 0, OldLabel: "old", NewLabel: "new"})
	if strings.Contains(got, " a") || strings.Contains(got, " c") {
		t.Errorf("context 0 should print no unchanged lines:\n%s", got)
	}
	if !strings.Contains(got, "-b") || !strings.Contains(got, "+B") {
		t.Errorf("the change itself is missing:\n%s", got)
	}
}

func TestColour(t *testing.T) {
	o := opts()
	o.Colour = true
	got := Unified("a\n", "b\n", o)
	if !strings.Contains(got, red) || !strings.Contains(got, green) {
		t.Errorf("colour was asked for and not used:\n%q", got)
	}
	if !strings.HasSuffix(strings.TrimRight(got, "\n"), reset) {
		t.Errorf("colour must be turned off again:\n%q", got)
	}

	o.Colour = false
	if got := Unified("a\n", "b\n", o); strings.Contains(got, "\033[") {
		t.Errorf("colour leaked into a plain diff:\n%q", got)
	}
}

func TestNoTrailingNewline(t *testing.T) {
	// "a" and "a\n" are one line either way, so this is about not inventing a
	// second, empty one.
	if got := Unified("a", "a\n", opts()); got != "" {
		t.Errorf("a missing final newline should not read as a change:\n%q", got)
	}
	if got := Unified("a\nb", "a\nB", opts()); !strings.Contains(got, "-b") {
		t.Errorf("a change on an unterminated last line was missed:\n%s", got)
	}
}

func TestIsBinary(t *testing.T) {
	if IsBinary([]byte("plain text\nwith lines\n")) {
		t.Error("text was called binary")
	}
	if !IsBinary([]byte("PNG\x00\x01\x02")) {
		t.Error("a NUL byte means binary")
	}
	if IsBinary(nil) {
		t.Error("nothing is not binary")
	}
	// Only the start is examined, which is what keeps this cheap on a large file.
	long := append([]byte(strings.Repeat("text\n", 4000)), 0)
	if IsBinary(long) {
		t.Error("a NUL past the sniffed prefix should not count")
	}
}

// TestDiffIsMinimalAndReconstructs is the property that matters: applying the
// edit script to the old text has to produce the new one, and adding a line
// should cost one line rather than rewriting the file.
func TestDiffIsMinimalAndReconstructs(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for range 200 {
		a := randomLines(rng, rng.Intn(25))
		b := mutate(rng, a)

		ops := diff(a, b)

		// Reconstruct both sides from the script.
		var gotOld, gotNew []string
		for _, op := range ops {
			switch op.kind {
			case opEqual:
				gotOld = append(gotOld, op.text)
				gotNew = append(gotNew, op.text)
			case opDelete:
				gotOld = append(gotOld, op.text)
			case opInsert:
				gotNew = append(gotNew, op.text)
			}
		}
		if strings.Join(gotOld, "\n") != strings.Join(a, "\n") {
			t.Fatalf("script does not reproduce the old text\n a=%v\n b=%v\n got=%v", a, b, gotOld)
		}
		if strings.Join(gotNew, "\n") != strings.Join(b, "\n") {
			t.Fatalf("script does not reproduce the new text\n a=%v\n b=%v\n got=%v", a, b, gotNew)
		}
	}
}

func TestInsertingOneLineCostsOneLine(t *testing.T) {
	var a []string
	for i := range 50 {
		a = append(a, fmt.Sprintf("line %d", i))
	}
	b := append(append([]string{}, a[:25]...), append([]string{"inserted"}, a[25:]...)...)

	ops := diff(a, b)
	var changed int
	for _, op := range ops {
		if op.kind != opEqual {
			changed++
		}
	}
	if changed != 1 {
		t.Errorf("inserting one line produced %d changes, want 1", changed)
	}
}

func randomLines(rng *rand.Rand, n int) []string {
	out := make([]string, 0, n)
	for range n {
		out = append(out, fmt.Sprintf("l%d", rng.Intn(8)))
	}
	return out
}

// mutate makes a plausible edit: a few insertions, deletions and changes.
func mutate(rng *rand.Rand, a []string) []string {
	b := append([]string{}, a...)
	for range rng.Intn(6) {
		switch rng.Intn(3) {
		case 0:
			at := rng.Intn(len(b) + 1)
			b = append(b[:at], append([]string{fmt.Sprintf("new%d", rng.Intn(5))}, b[at:]...)...)
		case 1:
			if len(b) > 0 {
				at := rng.Intn(len(b))
				b = append(b[:at], b[at+1:]...)
			}
		case 2:
			if len(b) > 0 {
				b[rng.Intn(len(b))] = fmt.Sprintf("chg%d", rng.Intn(5))
			}
		}
	}
	return b
}
