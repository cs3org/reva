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

// Package output renders command results as an aligned table, as JSON, or as
// CSV, so that every command supports --output without each one re-inventing
// formatting.
//
// A command builds a Table once, holding both the display strings and the
// structured value behind them. Table mode prints the strings; JSON mode
// marshals the structured value. Keeping the two separate is what stops JSON
// output from degrading into pre-formatted human text ("1.2 MiB" instead of
// 1258291), which would make it useless to a script.
package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// Format selects the rendering style.
type Format string

const (
	// FormatTable is aligned, human-readable columns.
	FormatTable Format = "table"
	// FormatJSON is a single JSON document, or newline-delimited JSON when
	// streaming.
	FormatJSON Format = "json"
	// FormatCSV is comma-separated values with a header row.
	FormatCSV Format = "csv"
)

// ParseFormat validates a --output value.
func ParseFormat(s string) (Format, error) {
	switch Format(strings.ToLower(s)) {
	case FormatTable:
		return FormatTable, nil
	case FormatJSON:
		return FormatJSON, nil
	case FormatCSV:
		return FormatCSV, nil
	default:
		return "", fmt.Errorf("unknown output format %q: want table, json, or csv", s)
	}
}

// Table is a rendered result: display strings for humans, and the structured
// value that JSON mode marshals.
type Table struct {
	// Headers label the columns. In CSV mode they become the header row; in
	// table mode they are printed unless the writer is quiet.
	Headers []string
	// Rows holds the display strings, one slice per row.
	Rows [][]string
	// Items is the structured value marshalled in JSON mode. When nil, JSON
	// mode falls back to objects built from Headers and Rows.
	Items any
}

// Writer renders results in the configured format.
type Writer struct {
	out    io.Writer
	err    io.Writer
	format Format
	quiet  bool
	color  bool
}

// Option configures a Writer.
type Option func(*Writer)

// Quiet suppresses headers and informational messages.
func Quiet(q bool) Option { return func(w *Writer) { w.quiet = q } }

// Color enables ANSI styling. Callers should pass IsTerminal(os.Stdout).
func Color(c bool) Option { return func(w *Writer) { w.color = c } }

// Stderr sets the stream used for informational messages. It defaults to
// os.Stderr so that messages never contaminate piped output.
func Stderr(e io.Writer) Option { return func(w *Writer) { w.err = e } }

// New returns a Writer rendering to out in the given format.
func New(out io.Writer, format Format, opts ...Option) *Writer {
	w := &Writer{out: out, err: os.Stderr, format: format}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Format reports the writer's format, for commands that need to vary behaviour
// (for example suppressing a progress bar under --output json).
func (w *Writer) Format() Format { return w.format }

// IsQuiet reports whether informational output is suppressed.
func (w *Writer) IsQuiet() bool { return w.quiet }

// UsesColor reports whether ANSI styling is on, for callers that render their own
// coloured output rather than going through a table.
func (w *Writer) UsesColor() bool { return w.color }

// Render writes a complete table in the writer's format.
func (w *Writer) Render(t Table) error {
	switch w.format {
	case FormatJSON:
		return w.renderJSON(t)
	case FormatCSV:
		return w.renderCSV(t)
	default:
		return w.renderTable(t)
	}
}

// Object writes a single structured value: a JSON document in JSON mode, an
// aligned key/value list otherwise. Used by commands like status and stat whose
// result is one record rather than a list.
func (w *Writer) Object(v any, fields ...Field) error {
	if w.format == FormatJSON {
		return w.encodeJSON(v)
	}
	t := Table{Headers: nil}
	for _, f := range fields {
		t.Rows = append(t.Rows, []string{f.Name + ":", f.Value})
	}
	if w.format == FormatCSV {
		t.Headers = []string{"field", "value"}
		return w.renderCSV(t)
	}
	return w.renderTable(t)
}

// Field is one line of an Object rendering.
type Field struct {
	Name  string
	Value string
}

// Line writes one literal line of result output to stdout, for a command whose
// natural rendering is not a table of labelled columns: ls, which has to look
// like ls. It writes nothing in JSON or CSV mode, where the structured
// rendering is the authoritative one and a stray line would corrupt it.
func (w *Writer) Line(format string, args ...any) {
	if w.format != FormatTable {
		return
	}
	fmt.Fprintf(w.out, format+"\n", args...)
}

// Msg writes an informational line to stderr. It is suppressed when quiet, and
// in JSON mode, so that --output json produces a parseable stream and nothing
// else.
func (w *Writer) Msg(format string, args ...any) {
	if w.quiet || w.format == FormatJSON {
		return
	}
	fmt.Fprintf(w.err, format+"\n", args...)
}

// Warn writes a warning to stderr. Unlike Msg it survives JSON mode, because a
// warning the user cannot see is worse than one that interleaves with a pipe,
// but it is still suppressed when quiet.
func (w *Writer) Warn(format string, args ...any) {
	if w.quiet {
		return
	}
	prefix := "warning: "
	if w.color {
		prefix = "\033[33mwarning:\033[0m "
	}
	fmt.Fprintf(w.err, prefix+format+"\n", args...)
}

func (w *Writer) renderTable(t Table) error {
	// Widths are computed here rather than by text/tabwriter because the header
	// is emitted bold: tabwriter counts the bytes of an ANSI escape as width, so
	// a coloured header was padded four columns short and every column after the
	// first sat out of line with its heading.
	// Nothing to show means nothing is shown. A lone header is not information:
	// it says a listing was attempted, which the command already said, and it
	// arrives underneath whatever message explained that the listing is empty.
	// CSV keeps its header, because a parser on the other end needs it to know
	// the shape of what it did not get.
	if len(t.Rows) == 0 {
		return nil
	}

	rows := make([][]string, 0, len(t.Rows)+1)
	showHeader := len(t.Headers) > 0 && !w.quiet
	if showHeader {
		rows = append(rows, t.Headers)
	}
	rows = append(rows, t.Rows...)

	widths := columnWidths(rows)
	for i, row := range rows {
		bold := showHeader && i == 0
		if _, err := fmt.Fprintln(w.out, w.formatRow(row, widths, bold)); err != nil {
			return err
		}
	}
	return nil
}

// columnWidths returns the width each column needs, measured on the text as
// written: no cell here carries styling, which is what makes this safe.
func columnWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			for len(widths) <= i {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], len([]rune(cell)))
		}
	}
	return widths
}

// formatRow pads a row to the column widths, styling after measuring. The last
// cell is not padded, so nothing carries trailing whitespace into a capture.
func (w *Writer) formatRow(row []string, widths []int, bold bool) string {
	const gap = 2
	var b strings.Builder
	for i, cell := range row {
		if i > 0 {
			b.WriteString(strings.Repeat(" ", gap))
		}
		text := cell
		if bold && w.color {
			text = "\033[1m" + cell + "\033[0m"
		}
		b.WriteString(text)
		if i < len(row)-1 && i < len(widths) {
			b.WriteString(strings.Repeat(" ", max(widths[i]-len([]rune(cell)), 0)))
		}
	}
	return b.String()
}

func (w *Writer) renderCSV(t Table) error {
	cw := csv.NewWriter(w.out)
	if len(t.Headers) > 0 {
		if err := cw.Write(t.Headers); err != nil {
			return err
		}
	}
	for _, row := range t.Rows {
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func (w *Writer) renderJSON(t Table) error {
	if t.Items != nil {
		return w.encodeJSON(t.Items)
	}
	// No structured value was supplied: build objects from the display strings
	// so that --output json still produces something usable.
	objs := make([]map[string]string, 0, len(t.Rows))
	for _, row := range t.Rows {
		obj := make(map[string]string, len(row))
		for i, cell := range row {
			key := fmt.Sprintf("column%d", i)
			if i < len(t.Headers) {
				key = t.Headers[i]
			}
			obj[key] = cell
		}
		objs = append(objs, obj)
	}
	return w.encodeJSON(objs)
}

func (w *Writer) encodeJSON(v any) error {
	enc := json.NewEncoder(w.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// -- formatting helpers -------------------------------------------------------

const (
	_ = 1 << (10 * iota)
	kib
	mib
	gib
	tib
	pib
)

// HumanSize renders a byte count the way ls -h does.
func HumanSize(n int64) string {
	switch {
	case n < kib:
		// A bare number below 1K, as ls -lh and du -h print it: "3", not "3B".
		return fmt.Sprintf("%d", n)
	case n < mib:
		return fmt.Sprintf("%.1fK", float64(n)/kib)
	case n < gib:
		return fmt.Sprintf("%.1fM", float64(n)/mib)
	case n < tib:
		return fmt.Sprintf("%.1fG", float64(n)/gib)
	case n < pib:
		return fmt.Sprintf("%.1fT", float64(n)/tib)
	default:
		return fmt.Sprintf("%.1fP", float64(n)/pib)
	}
}

// HumanTime renders a timestamp the way ls -l does: time of day for the last
// six months, year otherwise. The reference time is a parameter so the
// behaviour is testable.
func HumanTime(t, now time.Time) string {
	if t.IsZero() {
		return "-"
	}
	if now.Sub(t) < 180*24*time.Hour && t.Before(now.Add(24*time.Hour)) {
		return t.Format("Jan _2 15:04")
	}
	return t.Format("Jan _2  2006")
}

// IsTerminal reports whether f is an interactive terminal. It decides colour,
// progress rendering, and whether there is anyone to answer a confirmation
// prompt.
//
// This asks the terminal driver rather than checking for a character device.
// /dev/null is a character device, so the cheaper test would call a cron job
// with redirected stdin "interactive": and then block it forever on a prompt
// nobody can answer.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// TerminalWidth returns the width of f in columns, or 80 when it is not a
// terminal or the size cannot be determined. Used to lay out a listing in
// columns the way ls does.
func TerminalWidth(f *os.File) int {
	if f == nil {
		return 80
	}
	if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}
