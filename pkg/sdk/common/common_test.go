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

package common_test

import (
	"fmt"
	"testing"
	"time"

	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/pkg/sdk/common"
	testintl "github.com/cs3org/reva/pkg/sdk/common/testing"
)

func TestDataDescriptor(t *testing.T) {
	const name = "DATA_DESC"
	const size = 42

	dataDesc := common.CreateDataDescriptor(name, size)
	now := time.Now().Round(time.Millisecond)
	if v := dataDesc.Name(); v != name {
		t.Errorf(testintl.FormatTestResult("DataDescriptor.Name", name, v))
	}
	if v := dataDesc.Size(); v != size {
		t.Errorf(testintl.FormatTestResult("DataDescriptor.Size", size, v))
	}
	if v := dataDesc.Mode(); v != 0700 {
		t.Errorf(testintl.FormatTestResult("DataDescriptor.Mode", 0700, v))
	}
	if v := dataDesc.IsDir(); v != false {
		t.Errorf(testintl.FormatTestResult("DataDescriptor.IsDir", false, v))
	}
	if v := dataDesc.ModTime(); !v.Round(time.Millisecond).Equal(now) {
		// Since there's always a slight chance that the rounded times won't match, just log this mismatch
		t.Logf(testintl.FormatTestResult("DataDescriptor.ModTime", now, v))
	}
	if v := dataDesc.Sys(); v != nil {
		t.Errorf(testintl.FormatTestResult("DataDescriptor.Sys", nil, v))
	}
}

func TestFindString(t *testing.T) {
	tests := []struct {
		input  []string
		needle string
		wants  int
	}{
		{[]string{}, "so empty", -1},
		{[]string{"12345", "hello", "goodbye"}, "hello", 1},
		{[]string{"Rudimentär", "Ich bin du", "Wüste", "SANDIGER GRUND"}, "Wüste", 2},
		{[]string{"Rudimentär", "Ich bin du", "Wüste", "SANDIGER GRUND", "Sandiger Grund"}, "Sandiger Grund", 4},
		{[]string{"Nachahmer", "Roger", "k thx bye"}, "thx", -1},
		{[]string{"Six Feet Under", "Rock&Roll", "k thx bye"}, "Six Feet Under", 0},
		{[]string{"Six Feet Under", "Rock&Roll", "k thx bye"}, "Six Feet UNDER", -1},
	}

	for _, test := range tests {
		found := common.FindString(test.input, test.needle)
		if found != test.wants {
			t.Errorf(testintl.FormatTestResult("FindString", test.wants, found, test.input, test.needle))
		}
	}
}

func TestFindStringNoCase(t *testing.T) {
	tests := []struct {
		input  []string
		needle string
		wants  int
	}{
		{[]string{}, "so empty", -1},
		{[]string{"12345", "hello", "goodbye"}, "hello", 1},
		{[]string{"Rudimentär", "Ich bin du", "Wüste", "SANDIGER GRUND"}, "Wüste", 2},
		{[]string{"Rudimentär", "Ich bin du", "Wüste", "SANDIGER GRUND", "Sandiger Grund"}, "Sandiger Grund", 3},
		{[]string{"Nachahmer", "Roger", "k thx bye"}, "thx", -1},
		{[]string{"Six Feet Under", "Rock&Roll", "k thx bye"}, "Six Feet Under", 0},
		{[]string{"Six Feet Under", "Rock&Roll", "k thx bye"}, "Six Feet UNDER", 0},
	}

	for _, test := range tests {
		found := common.FindStringNoCase(test.input, test.needle)
		if found != test.wants {
			t.Errorf(testintl.FormatTestResult("FindString", test.wants, found, test.input, test.needle))
		}
	}
}

func TestDecodeOpaqueMap(t *testing.T) {
	opaque := types.Opaque{
		Map: map[string]*types.OpaqueEntry{
			"magic": {
				Decoder: "plain",
				Value:   []byte("42"),
			},
			"json": {
				Decoder: "json",
				Value:   []byte("[]"),
			},
		},
	}

	tests := []struct {
		key           string
		wants         string
		shouldSucceed bool
	}{
		{"magic", "42", true},
		{"json", "[]", false},
		{"somekey", "", false},
	}

	decodedMap := common.DecodeOpaqueMap(&opaque)
	for _, test := range tests {
		value, ok := decodedMap[test.key]
		if ok == test.shouldSucceed {
			if ok {
				if value != test.wants {
					t.Errorf(testintl.FormatTestResult("DecodeOpaqueMap", test.wants, value, opaque))
				}
			}
		} else {
			t.Errorf(testintl.FormatTestResult("DecodeOpaqueMap", test.shouldSucceed, ok, opaque))
		}
	}
}

func TestGetValuesFromOpaque(t *testing.T) {
	opaque := types.Opaque{
		Map: map[string]*types.OpaqueEntry{
			"magic": {
				Decoder: "plain",
				Value:   []byte("42"),
			},
			"stuff": {
				Decoder: "plain",
				Value:   []byte("Some stuff"),
			},
			"json": {
				Decoder: "json",
				Value:   []byte("[]"),
			},
		},
	}

	tests := []struct {
		keys          []string
		mandatory     bool
		shouldSucceed bool
	}{
		{[]string{"magic", "stuff"}, true, true},
		{[]string{"magic", "stuff", "json"}, false, true},
		{[]string{"magic", "stuff", "json"}, true, false},
		{[]string{"notfound"}, false, true},
		{[]string{"notfound"}, true, false},
	}

	for _, test := range tests {
		_, err := common.GetValuesFromOpaque(&opaque, test.keys, test.mandatory)
		if err != nil && test.shouldSucceed {
			t.Errorf(testintl.FormatTestError("GetValuesFromOpaque", err, opaque, test.keys, test.mandatory))
		} else if err == nil && !test.shouldSucceed {
			t.Errorf(testintl.FormatTestError("GetValuesFromOpaque", fmt.Errorf("getting values from an invalid opaque succeeded"), opaque, test.keys, test.mandatory))
		}
	}
}
