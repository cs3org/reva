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
			raw: `{"webdav":{"sharedSecret":"secret","permissions":["read","write"],"url":"http://example.org"}}`,
			expected: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read", "write"},
					URL:          "http://example.org",
				},
			},
		},
		{
			raw: `{"webapp":{"uriTemplate":"http://example.org/{test}"}}`,
			expected: []Protocol{
				&Webapp{
					URITemplate: "http://example.org/{test}",
				},
			},
		},
		{
			raw: `{"datatx":{"sharedSecret":"secret","srcUri":"http://example.org","size":10}}`,
			expected: []Protocol{
				&Datatx{
					SharedSecret: "secret",
					SourceURI:    "http://example.org",
					Size:         10,
				},
			},
		},
		{
			raw: `{"webdav":{"sharedSecret":"secret","permissions":["read","write"],"url":"http://example.org"},"webapp":{"uriTemplate":"http://example.org/{test}"},"datatx":{"sharedSecret":"secret","srcUri":"http://example.org","size":10}}`,
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
		{
			raw: `{"not_existing":{}}`,
			err: "protocol not_existing not recognised",
		},
	}

	for _, tt := range tests {
		var got Protocols
		err := json.Unmarshal([]byte(tt.raw), &got)
		if err != nil && err.Error() != tt.err {
			t.Fatalf("not expected error. got=%+v expected=%+v", err, tt.err)
		}

		if tt.err == "" {
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("result does not match with expected. got=%+v expected=%+v", render.AsCode(got), render.AsCode(tt.expected))
			}
		}
	}
}

func TestMarshalProtocol(t *testing.T) {
	tests := []struct {
		in       Protocols
		expected map[string]map[string]any
	}{
		{
			in:       []Protocol{},
			expected: map[string]map[string]any{},
		},
		{
			in: []Protocol{
				&WebDAV{
					SharedSecret: "secret",
					Permissions:  []string{"read"},
					URL:          "http://example.org",
				},
			},
			expected: map[string]map[string]any{
				"webdav": {
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
				},
			},
			expected: map[string]map[string]any{
				"webapp": {
					"uriTemplate": "http://example.org",
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
			expected: map[string]map[string]any{
				"datatx": {
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
				},
				&Datatx{
					SharedSecret: "secret",
					SourceURI:    "http://example.org/source",
					Size:         10,
				},
			},
			expected: map[string]map[string]any{
				"webdav": {
					"sharedSecret": "secret",
					"permissions":  []any{"read"},
					"url":          "http://example.org",
				},
				"webapp": {
					"uriTemplate": "http://example.org",
				},
				"datatx": {
					"sharedSecret": "secret",
					"srcUri":       "http://example.org/source",
					"size":         float64(10),
				},
			},
		},
	}

	for _, tt := range tests {
		d, err := json.Marshal(tt.in)
		if err != nil {
			t.Fatal("not expected error", err)
		}

		var got map[string]map[string]any
		if err := json.Unmarshal(d, &got); err != nil {
			t.Fatal("not expected error", err)
		}

		if !reflect.DeepEqual(tt.expected, got) {
			t.Fatalf("result does not match with expected. got=%+v expected=%+v", render.AsCode(got), render.AsCode(tt.expected))
		}
	}
}
