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
	"bytes"
	"io"
	"testing"

	"github.com/jedib0t/go-pretty/table"
)

func TestPrintCSV(t *testing.T) {
	r := &Runtime{output: "csv"}
	var b bytes.Buffer
	if err := r.Print(&b, Table{Headers: []string{"name", "size"}, Rows: [][]string{{"a, b", "1"}}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "name,size\n\"a, b\",1\n" {
		t.Errorf("table: %q", got)
	}
	b.Reset()
	if err := r.Print(&b, []string{"x"}, func(w io.Writer) {
		tw := NewTable(w)
		tw.AppendHeader(table.Row{"ID", "PATH"})
		tw.AppendRow(table.Row{"1", "/home"})
		tw.Render()
	}); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "ID,PATH\n1,/home\n" {
		t.Errorf("go-pretty table: %q", got)
	}
}
