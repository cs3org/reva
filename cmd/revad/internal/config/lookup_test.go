package config

import (
	"reflect"
	"testing"
)

type SimpleStruct struct {
	KeyA string `key:"keya"`
	KeyB string `key:"keyb"`
}

type NestedStruct struct {
	Nested SimpleStruct `key:"nested"`
	Value  int          `key:"value"`
}

type StructWithNestedMap struct {
	Map map[string]any `key:"map"`
}

type StructWithNestedList struct {
	List []SimpleStruct `key:"list"`
}

func TestLookupStruct(t *testing.T) {
	tests := []struct {
		in  any
		key string
		val any
		err error
	}{
		{
			in: SimpleStruct{
				KeyA: "val_a",
				KeyB: "val_b",
			},
			key: ".keyb",
			val: "val_b",
		},
		{
			in: NestedStruct{
				Nested: SimpleStruct{
					KeyA: "val_a",
					KeyB: "val_b",
				},
				Value: 10,
			},
			key: ".nested.keyb",
			val: "val_b",
		},
		{
			in: NestedStruct{
				Nested: SimpleStruct{
					KeyA: "val_a",
					KeyB: "val_b",
				},
				Value: 10,
			},
			key: ".value",
			val: 10,
		},
		{
			in: StructWithNestedMap{
				Map: map[string]any{
					"key1": "val1",
					"key2": "val2",
				},
			},
			key: ".map.key1",
			val: "val1",
		},
		{
			in: StructWithNestedList{
				List: []SimpleStruct{
					{
						KeyA: "val_a[1]",
						KeyB: "val_b[1]",
					},
					{
						KeyA: "val_a[2]",
						KeyB: "val_b[2]",
					},
				},
			},
			key: ".list[1].keyb",
			val: "val_b[2]",
		},
		{
			in: StructWithNestedList{
				List: []SimpleStruct{
					{
						KeyA: "val_a[1]",
						KeyB: "val_b[1]",
					},
				},
			},
			key: ".list.keya",
			val: "val_a[1]",
		},
		{
			in: StructWithNestedList{
				List: []SimpleStruct{
					{
						KeyA: "val_a[1]",
						KeyB: "val_b[1]",
					},
					{
						KeyA: "val_a[2]",
						KeyB: "val_b[2]",
					},
				},
			},
			key: ".list[1]",
			val: SimpleStruct{
				KeyA: "val_a[2]",
				KeyB: "val_b[2]",
			},
		},
	}

	for _, tt := range tests {
		got, err := lookupStruct(tt.key, reflect.ValueOf(tt.in))
		if err != tt.err {
			t.Fatalf("got not expected error: got=%v exp=%v", err, tt.err)
		}
		if tt.err == nil {
			if !reflect.DeepEqual(tt.val, got) {
				t.Fatalf("got not expected result. got=%v exp=%v", got, tt.val)
			}
		}
	}

}
