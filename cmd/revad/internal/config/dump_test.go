package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDumpMap(t *testing.T) {
	tests := []struct {
		in  map[string]any
		exp map[string]any
	}{
		{
			in:  map[string]any{},
			exp: map[string]any{},
		},
		{
			in: map[string]any{
				"simple": SimpleStruct{
					KeyA: "value_a",
					KeyB: "value_b",
				},
			},
			exp: map[string]any{
				"simple": map[string]any{
					"keya": "value_a",
					"keyb": "value_b",
				},
			},
		},
		{
			in: map[string]any{
				"simple": SimpleStruct{
					KeyA: "value_a",
					KeyB: "value_b",
				},
				"map": map[string]any{
					"mapa": "value_mapa",
					"mapb": "value_mapb",
				},
			},
			exp: map[string]any{
				"simple": map[string]any{
					"keya": "value_a",
					"keyb": "value_b",
				},
				"map": map[string]any{
					"mapa": "value_mapa",
					"mapb": "value_mapb",
				},
			},
		},
	}

	for _, tt := range tests {
		m := dumpMap(reflect.ValueOf(tt.in))
		assert.Equal(t, m, tt.exp)
	}
}

func TestDumpList(t *testing.T) {
	tests := []struct {
		in  []any
		exp []any
	}{
		{
			in:  []any{},
			exp: []any{},
		},
		{
			in:  []any{1, 2, 3, 4},
			exp: []any{1, 2, 3, 4},
		},
		{
			in: []any{
				map[string]any{
					"map": SimpleStruct{
						KeyA: "value_a",
						KeyB: "value_b",
					},
				},
				5,
				SimpleStruct{
					KeyA: "value_a",
					KeyB: "value_b",
				},
			},
			exp: []any{
				map[string]any{
					"map": map[string]any{
						"keya": "value_a",
						"keyb": "value_b",
					},
				},
				5,
				map[string]any{
					"keya": "value_a",
					"keyb": "value_b",
				},
			},
		},
	}

	for _, tt := range tests {
		l := dumpList(reflect.ValueOf(tt.in))
		assert.Equal(t, l, tt.exp)
	}
}

func TestDumpStruct(t *testing.T) {
	tests := []struct {
		in  any
		exp map[string]any
	}{
		{
			in: SimpleStruct{
				KeyA: "value_a",
				KeyB: "value_b",
			},
			exp: map[string]any{
				"keya": "value_a",
				"keyb": "value_b",
			},
		},
		{
			in: NestedStruct{
				Nested: SimpleStruct{
					KeyA: "value_a",
					KeyB: "value_b",
				},
				Value: 12,
			},
			exp: map[string]any{
				"nested": map[string]any{
					"keya": "value_a",
					"keyb": "value_b",
				},
				"value": 12,
			},
		},
		{
			in: StructWithNestedMap{
				Map: map[string]any{
					"keya": "value_a",
					"keyb": "value_b",
				},
			},
			exp: map[string]any{
				"map": map[string]any{
					"keya": "value_a",
					"keyb": "value_b",
				},
			},
		},
		{
			in: StructWithNestedList{
				List: []SimpleStruct{
					{
						KeyA: "value_a[1]",
						KeyB: "value_b[1]",
					},
					{
						KeyA: "value_a[2]",
						KeyB: "value_b[2]",
					},
				},
			},
			exp: map[string]any{
				"list": []any{
					map[string]any{
						"keya": "value_a[1]",
						"keyb": "value_b[1]",
					},
					map[string]any{
						"keya": "value_a[2]",
						"keyb": "value_b[2]",
					},
				},
			},
		},
		{
			in: Squashed{
				Squashed: SimpleStruct{
					KeyA: "value_a[1]",
					KeyB: "value_b[1]",
				},
				Simple: SimpleStruct{
					KeyA: "value_a[2]",
					KeyB: "value_b[2]",
				},
			},
			exp: map[string]any{
				"keya": "value_a[1]",
				"keyb": "value_b[1]",
				"Simple": map[string]any{
					"keya": "value_a[2]",
					"keyb": "value_b[2]",
				},
			},
		},
		{
			in: SquashedMap{
				Squashed: map[string]any{
					"keya": "val_a[1]",
					"keyb": "val_b[1]",
				},
				Simple: SimpleStruct{
					KeyA: "val_a[2]",
					KeyB: "val_b[2]",
				},
			},
			exp: map[string]any{
				"keya": "val_a[1]",
				"keyb": "val_b[1]",
				"simple": map[string]any{
					"keya": "val_a[2]",
					"keyb": "val_b[2]",
				},
			},
		},
	}

	for _, tt := range tests {
		s := dumpStruct(reflect.ValueOf(tt.in))
		assert.Equal(t, tt.exp, s)
	}
}
