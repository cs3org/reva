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

package invoke

import (
	"context"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"
)

// shade is an argument type with a fixed set of values.
type shade string

func (shade) Values() []string { return []string{"light", "dark"} }

// TestTypedCatalog checks that the catalog describes each argument from its
// struct field: type, list, default and enum.
func TestTypedCatalog(t *testing.T) {
	type runID string
	h := Typed(func(context.Context, struct {
		Job      string        `arg:"job,required" help:"the job"`
		Limit    int           `arg:"limit" default:"10"`
		Port     *uint16       `arg:"port"`
		Ratio    float64       `arg:"ratio"`
		All      bool          `arg:"all"`
		Wait     time.Duration `arg:"wait" default:"5s"`
		Since    time.Time     `arg:"since"`
		IDs      []runID       `arg:"ids"`
		Addr     netip.Addr    `arg:"addr"`
		Level    string        `arg:"level" enum:"info,warn" default:"info"`
		Shades   []shade       `arg:"shades"`
		Untagged string
	}) (Result, error) {
		return nil, nil
	})
	want := []ArgSpec{
		{Name: "job", Description: "the job", Required: true, Type: "string"},
		{Name: "limit", Type: "int", Default: "10"},
		{Name: "port", Type: "uint"},
		{Name: "ratio", Type: "float"},
		{Name: "all", Type: "bool"},
		{Name: "wait", Type: "duration", Default: "5s"},
		{Name: "since", Type: "time"},
		{Name: "ids", Type: "string", Repeated: true},
		{Name: "addr", Type: "netip.Addr"},
		{Name: "level", Type: "string", Default: "info", Enum: []string{"info", "warn"}},
		{Name: "shades", Type: "string", Repeated: true, Enum: []string{"light", "dark"}},
	}
	if !reflect.DeepEqual(h.args, want) {
		t.Errorf("catalog:\n got  %+v\n want %+v", h.args, want)
	}
}

// TestTypedDefaultAndEnum checks that an absent argument takes its default
// and that a value outside the enum is refused before the handler runs.
func TestTypedDefaultAndEnum(t *testing.T) {
	type args struct {
		Limit  int     `arg:"limit" default:"10"`
		Level  string  `arg:"level" enum:"info,warn" default:"info"`
		Shades []shade `arg:"shades"`
		Owner  *string `arg:"owner" default:"root"`
	}
	set := NewSet()
	var got args
	set.Add("op", "").Handle(Typed(func(_ context.Context, a args) (Result, error) { got = a; return Result{}, nil }))

	if _, err := set.Invoke(context.Background(), "op", nil); err != nil {
		t.Fatal(err)
	}
	if got.Limit != 10 || got.Level != "info" || got.Owner == nil || *got.Owner != "root" {
		t.Errorf("defaults not applied: %+v", got)
	}
	if _, err := set.Invoke(context.Background(), "op", map[string]any{"level": "warn", "shades": "dark, light,"}); err != nil {
		t.Fatal(err)
	}
	if got.Level != "warn" || len(got.Shades) != 2 {
		t.Errorf("args: %+v", got)
	}
	for _, bad := range []map[string]any{{"level": "debug"}, {"level": ""}, {"shades": "light,grey"}} {
		if _, err := set.Invoke(context.Background(), "op", bad); err == nil || !strings.Contains(err.Error(), "is not one of") {
			t.Errorf("%v: err = %v, want an enum error", bad, err)
		}
	}
}

// TestTypedRejectsBadDefault checks that a wrong default fails when the
// handler is made, not at the first call.
func TestTypedRejectsBadDefault(t *testing.T) {
	for name, fn := range map[string]func(){
		"unparsable": func() {
			Typed(func(context.Context, struct {
				N int `arg:"n" default:"many"`
			}) (Result, error) {
				return nil, nil
			})
		},
		"outside the enum": func() {
			Typed(func(context.Context, struct {
				S string `arg:"s" enum:"a,b" default:"c"`
			}) (Result, error) {
				return nil, nil
			})
		},
		"required": func() {
			Typed(func(context.Context, struct {
				S string `arg:"s,required" default:"a"`
			}) (Result, error) {
				return nil, nil
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected a panic")
				}
			}()
			fn()
		})
	}
}
