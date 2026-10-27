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

// Package textdiff makes a unified diff of two texts, without diff(1).
package textdiff

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

// Options set the output of Unified.
type Options struct {
	// Context is the number of unchanged lines around each change.
	Context int
	// OldLabel and NewLabel are the names in the header.
	OldLabel, NewLabel string
	// Colour adds ANSI colours.
	Colour bool
}

const (
	red   = "\033[31m"
	green = "\033[32m"
	cyan  = "\033[36m"
	reset = "\033[0m"
)

// Unified returns a unified diff of old and new. It returns "" if they are equal.
func Unified(old, new string, opts Options) string {
	if opts.Context < 0 {
		opts.Context = 0
	}
	if old == new {
		return ""
	}

	a, b := splitLines(old), splitLines(new)
	ops := diff(a, b)

	var out strings.Builder
	for _, h := range hunks(ops, opts.Context) {
		fmt.Fprintf(&out, "%s@@ -%d,%d +%d,%d @@%s\n",
			colour(opts.Colour, cyan), h.oldStart, h.oldLines, h.newStart, h.newLines, colour(opts.Colour, reset))
		for _, op := range h.ops {
			switch op.kind {
			case opEqual:
				fmt.Fprintf(&out, " %s\n", op.text)
			case opDelete:
				fmt.Fprintf(&out, "%s-%s%s\n", colour(opts.Colour, red), op.text, colour(opts.Colour, reset))
			case opInsert:
				fmt.Fprintf(&out, "%s+%s%s\n", colour(opts.Colour, green), op.text, colour(opts.Colour, reset))
			}
		}
	}
	if out.Len() == 0 {
		return ""
	}

	header := fmt.Sprintf("--- %s\n+++ %s\n", opts.OldLabel, opts.NewLabel)
	return header + out.String()
}

// IsBinary reports whether content has a NUL byte, as diff(1) and git test it.
func IsBinary(content []byte) bool {
	limit := min(len(content), 8000)
	return bytes.IndexByte(content[:limit], 0) >= 0
}

func colour(on bool, code string) string {
	if !on {
		return ""
	}
	return code
}

// splitLines drops the empty last element of a text that ends with a newline.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

type op struct {
	kind opKind
	text string
}

// diff returns the edit script from a to b. It removes the common first and last
// lines first, so that a long file with a few changes is fast.
func diff(a, b []string) []op {
	var ops []op

	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix &&
		a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}

	for _, line := range a[:prefix] {
		ops = append(ops, op{opEqual, line})
	}
	ops = append(ops, myers(a[prefix:len(a)-suffix], b[prefix:len(b)-suffix])...)
	for _, line := range a[len(a)-suffix:] {
		ops = append(ops, op{opEqual, line})
	}
	return ops
}

// myers is the greedy algorithm of Myers (1986). It uses O(D²) memory for the
// trace, where D is the size of the difference.
func myers(a, b []string) []op {
	n, m := len(a), len(b)
	switch {
	case n == 0 && m == 0:
		return nil
	case n == 0:
		return allOf(opInsert, b)
	case m == 0:
		return allOf(opDelete, a)
	}

	maxD := n + m
	offset := maxD
	v := make([]int, 2*maxD+1)
	var trace [][]int

	for d := 0; d <= maxD; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k

			for x < n && y < m && a[x] == b[y] {
				x, y = x+1, y+1
			}
			v[offset+k] = x

			if x >= n && y >= m {
				return walkBack(a, b, trace, offset)
			}
		}
	}
	// Unreachable: an edit script of at most n+m always exists.
	return append(allOf(opDelete, a), allOf(opInsert, b)...)
}

func walkBack(a, b []string, trace [][]int, offset int) []op {
	var reversed []op
	x, y := len(a), len(b)

	for d := len(trace) - 1; d > 0; d-- {
		v := trace[d]
		k := x - y

		var prevK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x, y = x-1, y-1
			reversed = append(reversed, op{opEqual, a[x]})
		}
		if y > prevY {
			y--
			reversed = append(reversed, op{opInsert, b[y]})
		} else if x > prevX {
			x--
			reversed = append(reversed, op{opDelete, a[x]})
		}
		x, y = prevX, prevY
	}

	for x > 0 {
		x, y = x-1, y-1
		reversed = append(reversed, op{opEqual, a[x]})
	}

	ops := make([]op, 0, len(reversed))
	for _, r := range slices.Backward(reversed) {
		ops = append(ops, r)
	}
	return ops
}

func allOf(kind opKind, lines []string) []op {
	ops := make([]op, 0, len(lines))
	for _, l := range lines {
		ops = append(ops, op{kind, l})
	}
	return ops
}

type hunk struct {
	oldStart, oldLines int
	newStart, newLines int
	ops                []op
}

func hunks(ops []op, context int) []hunk {
	// The line numbers for the @@ header.
	oldNo := make([]int, len(ops))
	newNo := make([]int, len(ops))
	o, n := 1, 1
	for i, op := range ops {
		oldNo[i], newNo[i] = o, n
		switch op.kind {
		case opEqual:
			o, n = o+1, n+1
		case opDelete:
			o++
		case opInsert:
			n++
		}
	}

	var out []hunk
	for i := 0; i < len(ops); {
		if ops[i].kind == opEqual {
			i++
			continue
		}

		// Changes whose context would overlap go in one hunk.
		start := max(0, i-context)
		end := i
		for end < len(ops) {
			if ops[end].kind != opEqual {
				end++
				continue
			}
			gap := 0
			for end+gap < len(ops) && ops[end+gap].kind == opEqual {
				gap++
			}
			if gap > 2*context && end+gap < len(ops) {
				break
			}
			if end+gap >= len(ops) {
				break
			}
			end += gap
		}
		end = min(len(ops), end+context)

		h := hunk{oldStart: oldNo[start], newStart: newNo[start], ops: ops[start:end]}
		for _, op := range h.ops {
			switch op.kind {
			case opEqual:
				h.oldLines++
				h.newLines++
			case opDelete:
				h.oldLines++
			case opInsert:
				h.newLines++
			}
		}
		// As in diff(1), a hunk on an empty side starts at 0.
		if h.oldLines == 0 {
			h.oldStart = oldNo[start] - 1
		}
		if h.newLines == 0 {
			h.newStart = newNo[start] - 1
		}
		out = append(out, h)
		i = end
	}
	return out
}
