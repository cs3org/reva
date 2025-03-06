// Copyright 2018-2022 CERN
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

package indexer

import (
	"fmt"
	"testing"

	"github.com/owncloud/reva/v2/pkg/storage/utils/indexer/option"
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
		{name: "ByValue", args: args{&someT{}}, want: "github.com.owncloud.reva.v2.pkg.storage.utils.indexer.someT"},
		{name: "ByRef", args: args{someT{}}, want: "github.com.owncloud.reva.v2.pkg.storage.utils.indexer.someT"},
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
