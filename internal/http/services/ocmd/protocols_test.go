// Copyright 2018-2023 CERN
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

package ocmd

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gdexlab/go-render/render"
)

func TestUnmarshalProtocol(t *testing.T) {
	tests := []struct {
		raw      string
		expected Protocols
		err      string
	}{
		{
			raw:      "{}",
			expected: []Protocol{},
		},
		{
			raw:      `{"name":"foo","options":{ }}`,
			expected: []Protocol{},
		},
		{
			raw: `{"name":"foo","options":{"unsupported":"value"}}`,
			err: `protocol options not supported: {"unsupported":"value"}`,
		},
		{
			raw: `{"unsupported":{}}`,
			err: "protocol unsupported not recognised",
		},
		{
			raw: `{"name":"multi","options":{},"webdav":{"sharedSecret":"secret","permissions":["read","write"],"url":"http://example.org"}}`,
			expected: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read", "write"},
					URL:          "http://example.org",
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"webapp":{"uriTemplate":"http://example.org/{test}"}}`,
			expected: []Protocol{
				&Webapp{
					URITemplate: "http://example.org/{test}",
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"datatx":{"sharedSecret":"secret","srcUri":"http://example.org","size":10}}`,
			expected: []Protocol{
				&Datatx{
					SharedSecret: "secret",
					SourceURI:    "http://example.org",
					Size:         10,
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"webdav":{"sharedSecret":"secret","permissions":["read","write"],"url":"http://example.org"},"webapp":{"uriTemplate":"http://example.org/{test}"},"datatx":{"sharedSecret":"secret","srcUri":"http://example.org","size":10}}`,
			expected: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read", "write"},
					URL:          "http://example.org",
				},
				&Webapp{
					URITemplate: "http://example.org/{test}",
				},
				&Datatx{
					SharedSecret: "secret",
					SourceURI:    "http://example.org",
					Size:         10,
				},
			},
		},
	}

	for _, tt := range tests {
		var got Protocols
		err := json.Unmarshal([]byte(tt.raw), &got)
		if err != nil && err.Error() != tt.err {
			t.Fatalf("not expected error. got=%+v expected=%+v", err, tt.err)
		}

		if tt.err == "" {
			if !protocolsEqual(got, tt.expected) {
				t.Fatalf("result does not match with expected. got=%+v expected=%+v", render.AsCode(got), render.AsCode(tt.expected))
			}
		}
	}
}

func protocolsToMap(p Protocols) map[string]Protocol {
	m := make(map[string]Protocol)
	for _, prot := range p {
		switch prot.(type) {
		case *WebDAV:
			m["webdav"] = prot
		case *Webapp:
			m["webapp"] = prot
		case *Datatx:
			m["datatx"] = prot
		}
	}
	return m
}

func protocolsEqual(p1, p2 Protocols) bool {
	return reflect.DeepEqual(protocolsToMap(p1), protocolsToMap(p2))
}

func TestMarshalProtocol(t *testing.T) {
	tests := []struct {
		in       Protocols
		expected map[string]any
		err      string
	}{
		{
			in:  []Protocol{},
			err: "json: error calling MarshalJSON for type ocmd.Protocols: no protocol defined",
		},
		{
			in: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read"},
					URL:          "http://example.org",
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webdav": map[string]any{
					"sharedSecret": "secret",
					"permissions":  []any{"read"},
					"url":          "http://example.org",
				},
			},
		},
		{
			in: []Protocol{
				&Webapp{
					URITemplate: "http://example.org",
					ViewMode:    "read",
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webapp": map[string]any{
					"uriTemplate": "http://example.org",
					"viewMode":    "read",
				},
			},
		},
		{
			in: []Protocol{
				&Datatx{
					SharedSecret: "secret",
					SourceURI:    "http://example.org/source",
					Size:         10,
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"datatx": map[string]any{
					"sharedSecret": "secret",
					"srcUri":       "http://example.org/source",
					"size":         float64(10),
				},
			},
		},
		{
			in: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read"},
					URL:          "http://example.org",
				},
				&Webapp{
					URITemplate: "http://example.org",
					ViewMode:    "read",
				},
				&Datatx{
					SharedSecret: "secret",
					SourceURI:    "http://example.org/source",
					Size:         10,
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webdav": map[string]any{
					"sharedSecret": "secret",
					"permissions":  []any{"read"},
					"url":          "http://example.org",
				},
				"webapp": map[string]any{
					"uriTemplate": "http://example.org",
					"viewMode":    "read",
				},
				"datatx": map[string]any{
					"sharedSecret": "secret",
					"srcUri":       "http://example.org/source",
					"size":         float64(10),
				},
			},
		},
	}

	for _, tt := range tests {
		d, err := json.Marshal(tt.in)
		if err != nil && err.Error() != tt.err {
			t.Fatalf("not expected error. got=%+v expected=%+v", err, tt.err)
		}
		if err == nil {
			var got map[string]any
			if err := json.Unmarshal(d, &got); err != nil {
				t.Fatalf("not expected error %+v with input %+v", err, tt.in)
			}

			if !reflect.DeepEqual(tt.expected, got) {
				t.Fatalf("result does not match with expected. got=%+v expected=%+v", render.AsCode(got), render.AsCode(tt.expected))
			}
		}
	}
}
