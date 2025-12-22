// Copyright 2018-2020 CERN
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

package errors

import (
	"fmt"
	"testing"
)

func TestNewf(t *testing.T) {
	tests := []struct {
		msg  string
		want error
	}{
		{"", fmt.Errorf("errors: ")},
		{"foo", fmt.Errorf("errors: foo")},
	}

	for _, tt := range tests {
		got := Newf(tt.msg)
		if got.Error() != tt.want.Error() {
			t.Errorf("New.Error(): got: %q, want %q", got, tt.want)
		}
	}
}

func TestNewfWithArgs(t *testing.T) {
	tests := []struct {
		msg  string
		args []interface{}
		want error
	}{
		{"foo %d", []interface{}{42}, fmt.Errorf("errors: foo 42")},
		{"foo %s %d", []interface{}{"bar", 42}, fmt.Errorf("errors: foo bar 42")},
	}

	for _, tt := range tests {
		got := Newf(tt.msg, tt.args...)
		if got.Error() != tt.want.Error() {
			t.Errorf("New.Error(): got: %q, want %q", got, tt.want)
		}
	}
}

func TestWrapf(t *testing.T) {
	tests := []struct {
		err  error
		msg  string
		want error
	}{
		{fmt.Errorf("foo"), "", fmt.Errorf("errors: : foo")},
		{fmt.Errorf("foo"), "foo", fmt.Errorf("errors: foo: foo")},
	}

	for _, tt := range tests {
		got := Wrapf(tt.err, tt.msg)
		if got.Error() != tt.want.Error() {
			t.Errorf("New.Error(): got: %q, want %q", got, tt.want)
		}
	}
}

func TestWrapfWithArgs(t *testing.T) {
	tests := []struct {
		err  error
		msg  string
		args []interface{}
		want error
	}{
		{fmt.Errorf("foo"), "foo %d", []interface{}{42}, fmt.Errorf("errors: foo 42: foo")},
		{fmt.Errorf("foo"), "foo %s %d", []interface{}{"bar", 42}, fmt.Errorf("errors: foo bar 42: foo")},
	}

	for _, tt := range tests {
		got := Wrapf(tt.err, tt.msg, tt.args...)
		if got.Error() != tt.want.Error() {
			t.Errorf("New.Error(): got: %q, want %q", got, tt.want)
		}
	}
}
