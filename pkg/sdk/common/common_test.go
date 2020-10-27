/*
 * MIT License
 *
 * Copyright (c) 2020 Daniel Mueller
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

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
