package rhttp

import "testing"

func TestURLHasPrefix(t *testing.T) {
	tests := map[string]struct {
		url      string
		prefix   string
		expected bool
	}{
		"root": {
			url:      "/",
			prefix:   "/",
			expected: true,
		},
		"suburl_root": {
			url:      "/api/v0",
			prefix:   "/",
			expected: true,
		},
		"suburl_root_slash_end": {
			url:      "/api/v0/",
			prefix:   "/",
			expected: true,
		},
		"suburl_root_no_slash": {
			url:      "/api/v0",
			prefix:   "",
			expected: true,
		},
		"no_common_prefix": {
			url:      "/api/v0/project",
			prefix:   "/api/v0/p",
			expected: false,
		},
		"long_url_prefix": {
			url:      "/api/v0/project/test",
			prefix:   "/api/v0",
			expected: true,
		},
		"prefix_end_slash": {
			url:      "/api/v0/project/test",
			prefix:   "/api/v0/",
			expected: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			res := urlHasPrefix(test.url, test.prefix)
			if res != test.expected {
				t.Fatalf("%s got an unexpected result: %+v instead of %+v", t.Name(), res, test.expected)
			}
		})
	}
}
