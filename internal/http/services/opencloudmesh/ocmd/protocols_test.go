// Copyright 2018-2024 CERN
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
			raw: `{"unsupported":{}}`,
			err: "protocol unsupported not recognised",
		},
		{
			raw: `{"name":"foo","options":{"unsupported":"value"}}`,
			err: `missing sharedSecret from options {"unsupported":"value"}`,
		},
		{
			raw: `{"name":"ocm10format","options":{"sharedSecret":"secret"}}`,
			expected: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read", "write", "share"},
					URI:          "",
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"webdav":{"sharedSecret":"secret","permissions":["read","write"],"requirements":["req"],"uri":"http://example.org"}}`,
			expected: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read", "write"},
					Requirements: []string{"req"},
					URI:          "http://example.org",
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"webdav":{"accessType":["datatx"],"sharedSecret":"secret","permissions":["read"],"uri":"http://example.org"}}`,
			expected: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					AccessTypes:  []ocm.AccessType{ocm.AccessType_ACCESS_TYPE_DATATX},
					Permissions:  []string{"read"},
					Requirements: []string{},
					URI:          "http://example.org",
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"webapp":{"uri":"http://example.org/test"}}`,
			expected: []Protocol{
				&Webapp{
					URI: "http://example.org/test",
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"webdav":{"sharedSecret":"secret","permissions":["read","write"],"uri":"http://example.org"},"webapp":{"uri":"http://example.org/test"}}`,
			expected: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read", "write"},
					URI:          "http://example.org",
				},
				&Webapp{
					URI: "http://example.org/test",
				},
			},
		},
	}

	for _, tt := range tests {
		var got Protocols
		err := json.Unmarshal([]byte(tt.raw), &got)
		if err != nil && err.Error() != tt.err {
			t.Fatalf("unexpected error. got=%+v expected=%+v", err, tt.err)
		}

		if tt.err == "" {
			if !protocolsEqual(got, tt.expected) {
				t.Fatalf("result does not match with expected. got=%#v expected=%#v for test=%+v", got, tt.expected, tt)
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
					Requirements: []string{},
					URI:          "http://example.org",
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webdav": map[string]any{
					"sharedSecret": "secret",
					"permissions":  []any{"read"},
					"uri":          "http://example.org",
				},
			},
		},
		{
			in: []Protocol{
				&WebDAV{
					AccessTypes:  []string{"datatx"},
					SharedSecret: "secret",
					Permissions:  []string{"read"},
					Requirements: []string{},
					URI:          "http://example.org",
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webdav": map[string]any{
					"accessTypes":  []string{"datatx"},
					"sharedSecret": "secret",
					"permissions":  []any{"read"},
					"uri":          "http://example.org",
				},
			},
		},
		{
			in: []Protocol{
				&Webapp{
					URI:      "http://example.org",
					ViewMode: "read",
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webapp": map[string]any{
					"uri":          "http://example.org",
					"viewMode":     "read",
					"sharedSecret": "",
				},
			},
		},
		{
			in: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read"},
					Requirements: []string{"req"},
					URI:          "http://example.org",
				},
				&Webapp{
					URI:      "http://example.org",
					ViewMode: "read",
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webdav": map[string]any{
					"sharedSecret": "secret",
					"permissions":  []any{"read"},
					"requirements": []any{"req"},
					"uri":          "http://example.org",
				},
				"webapp": map[string]any{
					"uri":          "http://example.org",
					"viewMode":     "read",
					"sharedSecret": "",
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
				t.Fatalf("result does not match with expected. got=%#v expected=%#v", got, tt.expected)
			}
		}
	}
}
