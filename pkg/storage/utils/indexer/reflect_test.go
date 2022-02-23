package indexer

import (
	"fmt"
	"testing"

	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
)

func Test_getTypeFQN(t *testing.T) {
	type someT struct{}

	type args struct {
		t interface{}
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "ByValue", args: args{&someT{}}, want: "github.com.cs3org.reva.pkg.storage.utils.indexer.someT"},
		{name: "ByRef", args: args{someT{}}, want: "github.com.cs3org.reva.pkg.storage.utils.indexer.someT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getTypeFQN(tt.args.t); got != tt.want {
				t.Errorf("getTypeFQN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_valueOf(t *testing.T) {
	type nestedDeeplyT struct {
		Val string
	}
	type nestedT struct {
		Deeply nestedDeeplyT
	}
	type someT struct {
		val    string
		Nested nestedT
	}
	type args struct {
		v       interface{}
		indexBy option.IndexBy
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "ByValue", args: args{v: someT{val: "hello"}, indexBy: option.IndexByField("val")}, want: "hello"},
		{name: "ByRef", args: args{v: &someT{val: "hello"}, indexBy: option.IndexByField("val")}, want: "hello"},
		{name: "nested", args: args{v: &someT{Nested: nestedT{Deeply: nestedDeeplyT{Val: "nestedHello"}}}, indexBy: option.IndexByField("Nested.Deeply.Val")}, want: "nestedHello"},
		{name: "using a indexFunc", args: args{v: &someT{Nested: nestedT{Deeply: nestedDeeplyT{Val: "nestedHello"}}}, indexBy: option.IndexByFunc{
			Name: "neestedDeeplyVal",
			Func: func(i interface{}) (string, error) {
				t, ok := i.(*someT)
				if !ok {
					return "", fmt.Errorf("booo")
				}
				return t.Nested.Deeply.Val, nil
			},
		}}, want: "nestedHello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := valueOf(tt.args.v, tt.args.indexBy); got != tt.want || err != nil {
				t.Errorf("valueOf() = %v, want %v", got, tt.want)
			}
		})
	}
}
