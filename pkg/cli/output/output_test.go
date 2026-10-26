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
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseFormat(t *testing.T) {
	for _, s := range []string{"table", "TABLE", "json", "csv"} {
		if _, err := ParseFormat(s); err != nil {
			t.Errorf("ParseFormat(%q) failed: %v", s, err)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Error("ParseFormat(\"yaml\") should fail")
	}
}

func sampleTable() Table {
	type entry struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	return Table{
		Headers: []string{"NAME", "SIZE"},
		Rows: [][]string{
			{"notes.txt", "1.2K"},
			{"data.bin", "3.0M"},
		},
		Items: []entry{
			{Name: "notes.txt", Size: 1258},
			{Name: "data.bin", Size: 3145728},
		},
	}
}

func TestRenderTable(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, FormatTable)
	if err := w.Render(sampleTable()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "NAME") {
		t.Errorf("header missing from table output:\n%s", out)
	}
	for _, want := range []string{"notes.txt", "1.2K", "data.bin", "3.0M"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderTableQuietDropsHeader(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, FormatTable, Quiet(true))
	if err := w.Render(sampleTable()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "NAME") {
		t.Errorf("quiet mode should not print headers:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "notes.txt") {
		t.Error("quiet mode dropped the rows as well as the header")
	}
}

// TestRenderJSONUsesStructuredItems is the point of keeping Items separate from
// Rows: a script consuming --output json must get the real size, not the
// human-rounded string.
func TestRenderJSONUsesStructuredItems(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, FormatJSON)
	if err := w.Render(sampleTable()); err != nil {
		t.Fatal(err)
	}

	var got []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].Size != 1258 {
		t.Errorf("size = %d, want the exact byte count 1258 rather than the display string", got[0].Size)
	}
}

func TestRenderJSONFallsBackToRows(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, FormatJSON)
	tbl := sampleTable()
	tbl.Items = nil
	if err := w.Render(tbl); err != nil {
		t.Fatal(err)
	}

	var got []map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 2 || got[0]["NAME"] != "notes.txt" {
		t.Errorf("fallback JSON is wrong: %+v", got)
	}
}

func TestRenderCSV(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, FormatCSV)
	if err := w.Render(sampleTable()); err != nil {
		t.Fatal(err)
	}
	want := "NAME,SIZE\nnotes.txt,1.2K\ndata.bin,3.0M\n"
	if buf.String() != want {
		t.Errorf("CSV output =\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestObject(t *testing.T) {
	type status struct {
		Provider string `json:"provider"`
		User     string `json:"user"`
	}
	v := status{Provider: "kerberos", User: "gdelmont"}
	fields := []Field{{"Provider", "kerberos"}, {"User", "gdelmont"}}

	t.Run("table", func(t *testing.T) {
		var buf bytes.Buffer
		if err := New(&buf, FormatTable).Object(v, fields...); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "Provider:") || !strings.Contains(out, "kerberos") {
			t.Errorf("key/value rendering is wrong:\n%s", out)
		}
	})

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		if err := New(&buf, FormatJSON).Object(v, fields...); err != nil {
			t.Fatal(err)
		}
		var got status
		if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("not valid JSON: %v", err)
		}
		if got != v {
			t.Errorf("got %+v, want %+v", got, v)
		}
	})
}

// TestMsgNeverContaminatesStdout guards the scripting contract: --output json
// must produce a parseable stream on stdout and nothing else.
func TestMsgNeverContaminatesStdout(t *testing.T) {
	var out, errBuf bytes.Buffer

	w := New(&out, FormatTable, Stderr(&errBuf))
	w.Msg("uploaded %d files", 3)
	if out.Len() != 0 {
		t.Errorf("Msg wrote to stdout: %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "uploaded 3 files") {
		t.Errorf("Msg did not reach stderr: %q", errBuf.String())
	}

	errBuf.Reset()
	jw := New(&out, FormatJSON, Stderr(&errBuf))
	jw.Msg("uploaded %d files", 3)
	if errBuf.Len() != 0 {
		t.Errorf("Msg should be suppressed under --output json, got %q", errBuf.String())
	}
}

func TestWarnSurvivesJSONButNotQuiet(t *testing.T) {
	var out, errBuf bytes.Buffer

	New(&out, FormatJSON, Stderr(&errBuf)).Warn("certificate verification disabled")
	if !strings.Contains(errBuf.String(), "certificate verification disabled") {
		t.Errorf("warnings must survive JSON mode, got %q", errBuf.String())
	}

	errBuf.Reset()
	New(&out, FormatTable, Stderr(&errBuf), Quiet(true)).Warn("something")
	if errBuf.Len() != 0 {
		t.Errorf("quiet should suppress warnings, got %q", errBuf.String())
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		// A bare number below 1K, exactly as ls -lh and du -h print it.
		{0, "0"},
		{999, "999"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1048576, "1.0M"},
		{3145728, "3.0M"},
		{1073741824, "1.0G"},
		{1099511627776, "1.0T"},
		{1125899906842624, "1.0P"},
	}
	for _, tt := range tests {
		if got := HumanSize(tt.in); got != tt.want {
			t.Errorf("HumanSize(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHumanTime(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	recent := time.Date(2026, 9, 10, 8, 30, 0, 0, time.UTC)
	if got := HumanTime(recent, now); got != "Sep 10 08:30" {
		t.Errorf("recent timestamp = %q, want time of day", got)
	}

	old := time.Date(2024, 3, 2, 8, 30, 0, 0, time.UTC)
	if got := HumanTime(old, now); !strings.Contains(got, "2024") {
		t.Errorf("old timestamp = %q, want the year", got)
	}

	if got := HumanTime(time.Time{}, now); got != "-" {
		t.Errorf("zero timestamp = %q, want %q", got, "-")
	}
}

func TestColorOnlyWhenEnabled(t *testing.T) {
	var plain, colored bytes.Buffer
	if err := New(&plain, FormatTable).Render(sampleTable()); err != nil {
		t.Fatal(err)
	}
	if err := New(&colored, FormatTable, Color(true)).Render(sampleTable()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "\033[") {
		t.Error("escape codes leaked into non-colour output")
	}
	if !strings.Contains(colored.String(), "\033[1m") {
		t.Error("colour output is missing the bold header")
	}
}

func TestIsTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if IsTerminal(f) {
		t.Error("a regular file should not be reported as a terminal")
	}
	if IsTerminal(nil) {
		t.Error("nil should not be reported as a terminal")
	}

	// /dev/null is a character device. A check based on os.ModeCharDevice would
	// call it a terminal, and a cron job with stdin redirected there would then
	// block forever on a confirmation prompt.
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()
	if IsTerminal(devNull) {
		t.Errorf("%s must not be reported as a terminal", os.DevNull)
	}
}

// TestRenderTableAlignsWithColour is the regression this renderer exists for.
// The header is emitted bold, and text/tabwriter counted the bytes of the ANSI
// escape as column width: so a coloured header sat four columns short of its
// own data and "space list" looked broken on any terminal.
//
// No test caught it because colour is off when stdout is not a terminal, which
// it never is under go test. So colour is forced on here.
func TestRenderTableAlignsWithColour(t *testing.T) {
	table := Table{
		Headers: []string{"ALIAS", "TYPE", "PATH"},
		Rows: [][]string{
			{"project/storage-ci", "project", "/eos/project/s/storage-ci"},
			{"home", "personal", "/eos/user/g/gdelmont"},
		},
	}

	var coloured, plain bytes.Buffer
	if err := New(&coloured, FormatTable, Color(true)).Render(table); err != nil {
		t.Fatal(err)
	}
	if err := New(&plain, FormatTable).Render(table); err != nil {
		t.Fatal(err)
	}

	// Strip the styling and the two must be identical: colour may change how a
	// cell looks, never where it sits.
	if got, want := stripSGR(coloured.String()), plain.String(); got != want {
		t.Errorf("colour changed the layout:\n got %q\nwant %q", got, want)
	}

	// And every row must start its columns at the same offsets as the header.
	// Searching for cell text would not do: "project" also occurs inside the
	// alias "project/storage-ci".
	lines := strings.Split(strings.TrimRight(plain.String(), "\n"), "\n")
	want := columnStarts(lines[0])
	for _, l := range lines[1:] {
		if got := columnStarts(l); !slices.Equal(got, want) {
			t.Errorf("columns start at %v in %q, but the header has %v", got, l, want)
		}
	}
}

// columnStarts returns the offset at which each column begins, a column being
// what follows the two-space gap the renderer writes.
func columnStarts(line string) []int {
	starts := []int{0}
	for i := 0; i+1 < len(line); i++ {
		if line[i] == ' ' && line[i+1] == ' ' {
			j := i
			for j < len(line) && line[j] == ' ' {
				j++
			}
			if j < len(line) {
				starts = append(starts, j)
			}
			i = j
		}
	}
	return starts
}

// TestRenderTableNoTrailingWhitespace: padding the final column leaves invisible
// spaces in anything that captures the output.
func TestRenderTableNoTrailingWhitespace(t *testing.T) {
	var buf bytes.Buffer
	if err := New(&buf, FormatTable, Color(true)).Render(sampleTable()); err != nil {
		t.Fatal(err)
	}
	for l := range strings.SplitSeq(strings.TrimRight(buf.String(), "\n"), "\n") {
		if l != strings.TrimRight(l, " ") {
			t.Errorf("line has trailing whitespace: %q", l)
		}
	}
}

// stripSGR removes ANSI styling so a coloured rendering can be compared against
// the plain one it must match.
func stripSGR(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\033' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func TestEmptyTablePrintsNothing(t *testing.T) {
	// A header on its own is not information. It says a listing was attempted,
	// which the command has already said in words, and it lands underneath the
	// message that explained the listing is empty.
	var buf bytes.Buffer
	w := New(&buf, FormatTable)
	if err := w.Render(Table{Headers: []string{"SLOT", "CONTENTS", "SIZE"}}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "" {
		t.Errorf("an empty table printed %q", buf.String())
	}
}

func TestEmptyJSONIsAnEmptyList(t *testing.T) {
	// Whatever is on the other end of a pipe needs a document either way.
	var buf bytes.Buffer
	w := New(&buf, FormatJSON)
	if err := w.Render(Table{Headers: []string{"SLOT"}, Items: []string{}}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Errorf("empty JSON = %q, want []", got)
	}
}

func TestEmptyCSVKeepsItsHeader(t *testing.T) {
	// A parser needs the shape of what it did not get.
	var buf bytes.Buffer
	w := New(&buf, FormatCSV)
	if err := w.Render(Table{Headers: []string{"SLOT", "CONTENTS"}}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "SLOT,CONTENTS" {
		t.Errorf("empty CSV = %q, want just the header", got)
	}
}
