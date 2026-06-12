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
			raw: `{"name":"multi","options":{},"webdav":{"sharedSecret":"secret","permissions":["read","write"],"requirements":["req"],"uri":"https://example.org"}}`,
			expected: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read", "write"},
					Requirements: []string{"req"},
					URI:          "https://example.org",
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"webdav":{"accessTypes":["datatx"],"sharedSecret":"secrettx","permissions":["read"],"uri":"https://example.org"}}`,
			expected: []Protocol{
				&WebDAV{
					SharedSecret: "secrettx",
					AccessTypes:  []string{"datatx"},
					Permissions:  []string{"read"},
					URI:          "https://example.org",
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"webapp":{"uri":"https://example.org/test","sharedSecret":"secret","permissions":["read"],"requirements":["must-exchange-token"],"targets":["blank"]}}`,
			expected: []Protocol{
				&Webapp{
					URI:          "https://example.org/test",
					SharedSecret: "secret",
					Permissions:  []string{"read"},
					Requirements: []string{"must-exchange-token"},
					Targets:      []string{"blank"},
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"embedded":{"payload":{"a":"b"}}}`,
			expected: []Protocol{
				&Embedded{
					Payload: json.RawMessage(`{"a":"b"}`),
				},
			},
		},
		{
			raw: `{"name":"multi","options":{},"webdav":{"sharedSecret":"secret","permissions":["read","write"],"uri":"https://example.org"},"webapp":{"uri":"https://example.org/test","sharedSecret":"secret","permissions":["read","write"],"requirements":["must-exchange-token"],"targets":["blank"]}}`,
			expected: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read", "write"},
					URI:          "https://example.org",
				},
				&Webapp{
					URI:          "https://example.org/test",
					SharedSecret: "secret",
					Permissions:  []string{"read", "write"},
					Requirements: []string{"must-exchange-token"},
					Targets:      []string{"blank"},
				},
			},
		},
	}

	for _, tt := range tests {
		var got Protocols
		err := json.Unmarshal([]byte(tt.raw), &got)
		if err != nil && err.Error() != tt.err {
			t.Fatalf("unexpected error. Got=%+v expected=%+v", err, tt.err)
		}

		if tt.err == "" {
			if !protocolsEqual(got, tt.expected) {
				got_json, _ := got.MarshalJSON()
				expected_json, _ := tt.expected.MarshalJSON()
				t.Fatalf("result does not match with expected.\n     Got: %s\nExpected: %s", got_json, expected_json)
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

func TestRequirementsRoundTripThroughToOCMProtocol(t *testing.T) {
	w := &WebDAV{
		SharedSecret: "secret",
		Permissions:  []string{"read"},
		Requirements: []string{"must-exchange-token"},
		URI:          "https://example.org/dav",
	}

	proto := w.ToOCMProtocol()
	wdav := proto.GetWebdavOptions()
	if wdav == nil {
		t.Fatal("expected WebDAV protocol options")
	}
	if len(wdav.Requirements) != 1 || wdav.Requirements[0] != "must-exchange-token" {
		t.Errorf("requirements lost in ToOCMProtocol: got %v", wdav.Requirements)
	}
	if len(wdav.AccessTypes) == 0 {
		t.Error("expected at least one access type (default remote)")
	}
}

func TestProtocolsValidateRejectsUnsupportedWebDAVRequirement(t *testing.T) {
	protocols := Protocols{
		&WebDAV{
			SharedSecret: "secret",
			Permissions:  []string{"read"},
			Requirements: []string{"unsupported-requirement"},
			URI:          "https://example.org/dav",
		},
	}

	err := protocols.Validate()
	if err == nil {
		t.Fatal("expected validation error for unsupported requirement")
	}
	if got, want := err.Error(), `protocol webdav has unsupported requirement "unsupported-requirement"`; got != want {
		t.Fatalf("Validate() error = %q, want %q", got, want)
	}
}

func TestProtocolsValidateAllowsSupportedWebDAVRequirement(t *testing.T) {
	protocols := Protocols{
		&WebDAV{
			SharedSecret: "secret",
			Permissions:  []string{"read"},
			Requirements: []string{"must-exchange-token"},
			URI:          "https://example.org/dav",
		},
	}

	if err := protocols.Validate(); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
}

func TestProtocolsValidateWebapp(t *testing.T) {
	tests := []struct {
		name   string
		webapp *Webapp
		err    string
	}{
		{
			name: "valid",
			webapp: &Webapp{
				SharedSecret: "secret",
				Permissions:  []string{"read", "write"},
				Requirements: []string{"must-exchange-token"},
				Targets:      []string{"blank"},
				URI:          "https://example.org/app",
			},
		},
		{
			name: "legacy payload without permissions is rejected",
			webapp: &Webapp{
				SharedSecret: "secret",
				URI:          "https://example.org/app",
			},
			err: "protocol webapp missing permissions",
		},
		{
			name: "missing sharedSecret",
			webapp: &Webapp{
				Permissions:  []string{"read"},
				Requirements: []string{"must-exchange-token"},
				Targets:      []string{"blank"},
				URI:          "https://example.org/app",
			},
			err: "protocol webapp missing sharedSecret",
		},
		{
			name: "missing targets",
			webapp: &Webapp{
				SharedSecret: "secret",
				Permissions:  []string{"read"},
				Requirements: []string{"must-exchange-token"},
				URI:          "https://example.org/app",
			},
			err: "protocol webapp missing targets",
		},
		{
			name: "requirements without must-exchange-token",
			webapp: &Webapp{
				SharedSecret: "secret",
				Permissions:  []string{"read"},
				Requirements: []string{"must-use-mfa"},
				Targets:      []string{"blank"},
				URI:          "https://example.org/app",
			},
			err: "protocol webapp requirements must include must-exchange-token",
		},
		{
			name: "unsupported permission",
			webapp: &Webapp{
				SharedSecret: "secret",
				Permissions:  []string{"admin"},
				Requirements: []string{"must-exchange-token"},
				Targets:      []string{"blank"},
				URI:          "https://example.org/app",
			},
			err: `protocol webapp has unsupported permission "admin"`,
		},
		{
			name: "unsupported target",
			webapp: &Webapp{
				SharedSecret: "secret",
				Permissions:  []string{"read"},
				Requirements: []string{"must-exchange-token"},
				Targets:      []string{"popup"},
				URI:          "https://example.org/app",
			},
			err: `protocol webapp has unsupported target "popup"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Protocols{tt.webapp}.Validate()
			if tt.err == "" {
				if err != nil {
					t.Fatalf("Validate() returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if got := err.Error(); got != tt.err {
				t.Fatalf("Validate() error = %q, want %q", got, tt.err)
			}
		})
	}
}

func TestWebappToOCMProtocol(t *testing.T) {
	w := &Webapp{
		SharedSecret: "secret",
		Permissions:  []string{"read", "write", "share"},
		Requirements: []string{"must-exchange-token"},
		Targets:      []string{"blank"},
		URI:          "https://example.org/app",
		AppName:      "Some App",
		AppIconHint:  "text/plain",
		MediaTypes:   []string{"text/plain"},
	}

	proto := w.ToOCMProtocol()
	wapp := proto.GetWebappOptions()
	if wapp == nil {
		t.Fatal("expected Webapp protocol options")
	}
	if wapp.Uri != w.URI || wapp.SharedSecret != w.SharedSecret || wapp.AppName != w.AppName || wapp.AppIconHint != w.AppIconHint {
		t.Errorf("scalar fields lost in ToOCMProtocol: got %+v", wapp)
	}
	if len(wapp.Requirements) != 1 || wapp.Requirements[0] != "must-exchange-token" {
		t.Errorf("requirements lost in ToOCMProtocol: got %v", wapp.Requirements)
	}
	if len(wapp.Targets) != 1 || wapp.Targets[0] != "blank" {
		t.Errorf("targets lost in ToOCMProtocol: got %v", wapp.Targets)
	}
	if len(wapp.MediaTypes) != 1 || wapp.MediaTypes[0] != "text/plain" {
		t.Errorf("media types lost in ToOCMProtocol: got %v", wapp.MediaTypes)
	}
	p := wapp.GetPermissions().GetPermissions()
	if p == nil || !p.InitiateFileDownload || !p.InitiateFileUpload {
		t.Errorf("permissions lost in ToOCMProtocol: got %v", p)
	}
	if !wapp.GetPermissions().GetReshare() {
		t.Error("reshare permission lost in ToOCMProtocol")
	}
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
					AccessTypes:  []string{},
					Permissions:  []string{"read"},
					Requirements: []string{},
					URI:          "https://example.org",
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webdav": map[string]any{
					"sharedSecret": "secret",
					"permissions":  []any{"read"},
					"uri":          "https://example.org",
				},
			},
		},
		{
			in: []Protocol{
				&WebDAV{
					SharedSecret: "secrettx",
					AccessTypes:  []string{"datatx"},
					Permissions:  []string{"read"},
					Requirements: []string{},
					URI:          "https://example.org",
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webdav": map[string]any{
					"sharedSecret": "secrettx",
					"accessTypes":  []any{"datatx"},
					"permissions":  []any{"read"},
					"uri":          "https://example.org",
				},
			},
		},
		{
			in: []Protocol{
				&Webapp{
					URI:          "https://example.org",
					SharedSecret: "secret",
					Permissions:  []string{"read"},
					Requirements: []string{"must-exchange-token"},
					Targets:      []string{"blank"},
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webapp": map[string]any{
					"uri":          "https://example.org",
					"sharedSecret": "secret",
					"permissions":  []any{"read"},
					"requirements": []any{"must-exchange-token"},
					"targets":      []any{"blank"},
				},
			},
		},
		{
			in: []Protocol{
				&Embedded{
					Payload: json.RawMessage(`{"a":"b"}`),
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"embedded": map[string]any{
					"payload": map[string]any{"a": "b"},
				},
			},
		},
		{
			in: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					AccessTypes:  []string{},
					Permissions:  []string{"read"},
					Requirements: []string{"req"},
					URI:          "https://example.org",
				},
				&Webapp{
					URI:          "https://example.org",
					SharedSecret: "secret",
					Permissions:  []string{"read"},
					Requirements: []string{"must-exchange-token"},
					Targets:      []string{"blank"},
				},
			},
			expected: map[string]any{
				"name":    "multi",
				"options": map[string]any{},
				"webdav": map[string]any{
					"sharedSecret": "secret",
					"permissions":  []any{"read"},
					"requirements": []any{"req"},
					"uri":          "https://example.org",
				},
				"webapp": map[string]any{
					"uri":          "https://example.org",
					"sharedSecret": "secret",
					"permissions":  []any{"read"},
					"requirements": []any{"must-exchange-token"},
					"targets":      []any{"blank"},
				},
			},
		},
	}

	for _, tt := range tests {
		d, err := json.Marshal(tt.in)
		if err != nil && err.Error() != tt.err {
			t.Fatalf("unexpected error. Got=%+v expected=%+v", err, tt.err)
		}
		if err == nil {
			var got map[string]any
			if err := json.Unmarshal(d, &got); err != nil {
				t.Fatalf("unexpected error %+v with input %+v", err, tt.in)
			}

			if !reflect.DeepEqual(tt.expected, got) {
				t.Fatalf("result does not match with expected.\n     Got: %#v\nExpected: %#v", got, tt.expected)
			}
		}
	}
}
