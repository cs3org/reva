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

package cli

import (
	"io"

	"github.com/jedib0t/go-pretty/table"
)

// Table is a result for Runtime.Print: a table for a person, and Items as
// JSON. Without Items, the JSON is one object per row, keyed by the headers.
type Table struct {
	Headers []string
	Rows    [][]string
	Items   any
}

// Write writes the table for a person.
func (t Table) Write(w io.Writer) {
	tw := table.NewWriter()
	tw.SetOutputMirror(w)
	tw.AppendHeader(row(t.Headers))
	for _, r := range t.Rows {
		tw.AppendRow(row(r))
	}
	tw.Render()
}

func (t Table) items() any {
	if t.Items != nil {
		return t.Items
	}
	out := make([]map[string]string, 0, len(t.Rows))
	for _, r := range t.Rows {
		m := make(map[string]string, len(t.Headers))
		for i, h := range t.Headers {
			if i < len(r) {
				m[h] = r[i]
			}
		}
		out = append(out, m)
	}
	return out
}

func row(cells []string) table.Row {
	r := make(table.Row, len(cells))
	for i, c := range cells {
		r[i] = c
	}
	return r
}
