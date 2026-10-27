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

package admin

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli"
)

// nodeResult is the -o json form of the result of one node.
type nodeResult struct {
	Node   string          `json:"node"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// writeResults writes the results of a unary invocation. A list result is a
// table, anything else is key/value lines (rows of key and value with -o csv).
// It returns an error if a node failed, so that the exit code is 1.
func writeResults(w, errW io.Writer, results []*adminpb.NodeResult) error {
	failed := 0
	if rt.Output() == "json" {
		out := make([]nodeResult, 0, len(results))
		for _, r := range results {
			if r.Error != "" {
				failed++
			}
			out = append(out, nodeResult{Node: r.Node, Result: json.RawMessage(r.ResultJson), Error: r.Error})
		}
		if err := rt.Print(w, out, nil); err != nil {
			return err
		}
		return failedErr(failed, len(results))
	}

	multi := len(results) > 1
	var cols []string
	var rows []*object
	tabular := false
	type record struct {
		node string
		obj  *object
	}
	var records []record
	for _, r := range results {
		if r.Error != "" {
			fmt.Fprintf(errW, "%s: %s\n", r.Node, r.Error)
			failed++
			continue
		}
		if r.ResultJson == "" {
			records = append(records, record{r.Node, nil})
			continue
		}
		v, err := decodeOrdered([]byte(r.ResultJson))
		if err != nil {
			return fmt.Errorf("%s: bad result: %w", r.Node, err)
		}
		items, rest, ok := rowsOf(v)
		if !ok {
			records = append(records, record{r.Node, flatten(asObject(v))})
			continue
		}
		tabular = true
		for _, it := range items {
			row := flatten(it)
			if multi {
				withNode(row, r.Node)
			}
			for _, k := range row.keys {
				if !slices.Contains(cols, k) {
					cols = append(cols, k)
				}
			}
			rows = append(rows, row)
		}
		if rest != nil && len(rest.keys) > 0 {
			records = append(records, record{r.Node, flatten(rest)})
		}
	}

	if tabular {
		if len(rows) == 0 {
			rt.Msg(errW, "no results")
		} else {
			t := cli.Table{Headers: cols}
			for _, row := range rows {
				cells := make([]string, len(cols))
				for i, c := range cols {
					cells[i], _ = row.vals[c].(string)
				}
				t.Rows = append(t.Rows, cells)
			}
			if err := rt.Print(w, t, nil); err != nil {
				return err
			}
		}
	}
	if rt.Output() == "csv" && len(records) > 0 {
		t := cli.Table{Headers: []string{"key", "value"}}
		if multi {
			t.Headers = append([]string{nodeColumn}, t.Headers...)
		}
		for _, rec := range records {
			if rec.obj == nil {
				continue
			}
			for _, k := range rec.obj.keys {
				row := []string{k, fmt.Sprint(rec.obj.vals[k])}
				if multi {
					row = append([]string{rec.node}, row...)
				}
				t.Rows = append(t.Rows, row)
			}
		}
		if err := rt.Print(w, t, nil); err != nil {
			return err
		}
		return failedErr(failed, len(results))
	}
	nodeWidth := 0
	for _, rec := range records {
		nodeWidth = max(nodeWidth, len(rec.node))
	}
	for _, rec := range records {
		prefix := ""
		if multi {
			prefix = fmt.Sprintf("%-*s  ", nodeWidth, rec.node)
		}
		if rec.obj == nil || len(rec.obj.keys) == 0 {
			fmt.Fprintf(w, "%sok\n", prefix)
			continue
		}
		width := 0
		for _, k := range rec.obj.keys {
			width = max(width, len(k)+1)
		}
		for _, k := range rec.obj.keys {
			fmt.Fprintf(w, "%s%-*s %s\n", prefix, width, k+":", rec.obj.vals[k])
		}
	}
	return failedErr(failed, len(results))
}

// nodeColumn is the column of the node when the results come from several.
const nodeColumn = "node"

// withNode puts the node in the first column. It replaces a node field of
// the result, which has the same value.
func withNode(row *object, node string) {
	row.keys = append([]string{nodeColumn}, slices.DeleteFunc(row.keys, func(k string) bool { return k == nodeColumn })...)
	row.vals[nodeColumn] = node
}

func failedErr(failed, total int) error {
	if failed == 0 {
		return nil
	}
	return fmt.Errorf("the invocation failed on %d of %d instances", failed, total)
}

// streamWriter writes the results of a streaming invocation as they come.
// With -o table the columns are fixed by the first row, because the rows
// above are already printed: a key that comes later is not shown.
type streamWriter struct {
	w, errW io.Writer
	node    bool
	cols    []string
	widths  []int
	failed  int
	total   int
}

func (s *streamWriter) write(msg *adminpb.InvokeStreamResponse) error {
	s.total++
	if msg.Error != "" {
		s.failed++
	}
	if rt.Output() == "json" {
		b, err := json.Marshal(nodeResult{Node: msg.Node, Result: json.RawMessage(msg.ResultJson), Error: msg.Error})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(s.w, "%s\n", b)
		return err
	}
	if msg.Error != "" {
		fmt.Fprintf(s.errW, "%s: %s\n", msg.Node, msg.Error)
		return nil
	}
	if msg.ResultJson == "" {
		return nil
	}
	v, err := decodeOrdered([]byte(msg.ResultJson))
	if err != nil {
		return fmt.Errorf("%s: bad result: %w", msg.Node, err)
	}
	// An item is one row, unless it holds a list. The other fields of such
	// an item are only in -o json.
	items, _, ok := rowsOf(v)
	if !ok {
		items = []*object{asObject(v)}
	}
	for _, it := range items {
		row := flatten(it)
		if s.node {
			withNode(row, msg.Node)
		}
		if s.cols == nil {
			s.cols = row.keys
			cells := make([]string, len(s.cols))
			for i, c := range s.cols {
				cells[i] = c
				if rt.Output() != "csv" {
					cells[i] = strings.ToUpper(c)
				}
				s.widths = append(s.widths, max(len(c), len(row.vals[c].(string))))
			}
			s.line(cells)
		}
		cells := make([]string, len(s.cols))
		for i, c := range s.cols {
			cells[i], _ = row.vals[c].(string)
		}
		s.line(cells)
	}
	return nil
}

func (s *streamWriter) line(cells []string) {
	if rt.Output() == "csv" {
		cw := csv.NewWriter(s.w)
		_ = cw.Write(cells)
		cw.Flush()
		return
	}
	var b strings.Builder
	for i, c := range cells {
		if i == len(cells)-1 {
			b.WriteString(c)
			break
		}
		fmt.Fprintf(&b, "%-*s  ", s.widths[i], c)
	}
	fmt.Fprintln(s.w, b.String())
}

func (s *streamWriter) err() error { return failedErr(s.failed, s.total) }

// object is a JSON object that keeps the order of its keys, so that the
// columns follow the result and not the alphabet.
type object struct {
	keys []string
	vals map[string]any
}

func (o *object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := json.Marshal(o.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// decodeOrdered decodes one JSON value. An object is an *object, an array is
// a []any and a number is a json.Number, so that it prints as it came.
func decodeOrdered(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return decodeValue(dec)
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		o := &object{vals: map[string]any{}}
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			key := k.(string)
			if _, dup := o.vals[key]; !dup {
				o.keys = append(o.keys, key)
			}
			o.vals[key] = v
		}
		_, err = dec.Token()
		return o, err
	case json.Delim('['):
		a := []any{}
		for dec.More() {
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		_, err = dec.Token()
		return a, err
	}
	return tok, nil
}

// rowsOf returns the rows of a list result: an array of objects, or an
// object with exactly one array of objects. rest is the other fields of that
// object.
func rowsOf(v any) (rows []*object, rest *object, ok bool) {
	if a, isArray := v.([]any); isArray {
		rows, ok = objects(a)
		return rows, nil, ok
	}
	o, isObject := v.(*object)
	if !isObject {
		return nil, nil, false
	}
	list := ""
	for _, k := range o.keys {
		if a, isArray := o.vals[k].([]any); isArray {
			if _, isRows := objects(a); isRows {
				if list != "" {
					return nil, nil, false
				}
				list = k
			}
		}
	}
	if list == "" {
		return nil, nil, false
	}
	rows, _ = objects(o.vals[list].([]any))
	rest = &object{vals: o.vals}
	for _, k := range o.keys {
		if k != list {
			rest.keys = append(rest.keys, k)
		}
	}
	return rows, rest, true
}

// objects returns the elements of a, if they are all objects.
func objects(a []any) ([]*object, bool) {
	out := make([]*object, 0, len(a))
	for _, e := range a {
		o, ok := e.(*object)
		if !ok {
			return nil, false
		}
		out = append(out, o)
	}
	return out, true
}

// asObject makes a scalar result an object with one field, so that it
// prints as a key/value line.
func asObject(v any) *object {
	if o, ok := v.(*object); ok {
		return o
	}
	return &object{keys: []string{"result"}, vals: map[string]any{"result": v}}
}

// flatten returns the cells of o as strings. A nested object becomes keys
// joined with dots.
func flatten(o *object) *object {
	out := &object{vals: map[string]any{}}
	var walk func(prefix string, o *object)
	walk = func(prefix string, o *object) {
		for _, k := range o.keys {
			key := prefix + k
			if n, ok := o.vals[k].(*object); ok && len(n.keys) > 0 {
				walk(key+".", n)
				continue
			}
			out.keys = append(out.keys, key)
			out.vals[key] = cell(o.vals[k])
		}
	}
	walk("", o)
	return out
}

// cell is a value as a person reads it: a list of scalars is joined with
// commas, and a list of objects is compact JSON.
func cell(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, e := range v {
			switch e.(type) {
			case *object, []any:
				b, _ := json.Marshal(v)
				return string(b)
			}
			parts = append(parts, cell(e))
		}
		return strings.Join(parts, ",")
	}
	b, _ := json.Marshal(v)
	return string(b)
}
