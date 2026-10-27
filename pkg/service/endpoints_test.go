// Copyright 2018-2026 CERN
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

package service

import (
	"testing"

	"github.com/cs3org/reva/v3/pkg/registry"
)

// A peer builds a data provider's or data gateway's URL from the registry when
// the service has no public_url configured. The prefix advertised there is a
// path, so it may or may not be rooted, and either way the URL must not grow a
// doubled or a missing separator.
func TestEndpointURL(t *testing.T) {
	tests := map[string]struct {
		meta     map[string]string
		expected string
	}{
		"rooted prefix":   {map[string]string{registry.MetaPrefix: "/data"}, "http://host:1234/data"},
		"bare prefix":     {map[string]string{registry.MetaPrefix: "data"}, "http://host:1234/data"},
		"nested prefix":   {map[string]string{registry.MetaPrefix: "/a/b"}, "http://host:1234/a/b"},
		"no prefix":       {map[string]string{}, "http://host:1234"},
		"root prefix":     {map[string]string{registry.MetaPrefix: "/"}, "http://host:1234"},
		"https":           {map[string]string{registry.MetaScheme: "https", registry.MetaPrefix: "/data"}, "https://host:1234/data"},
		"public url wins": {map[string]string{registry.MetaPrefix: "/data", registry.MetaPublicURL: "https://edge/x"}, "https://edge/x"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			e := endpoint{name: "dataprovider", node: registry.NewNode("n", "host:1234", tt.meta)}
			if got := e.URL(); got != tt.expected {
				t.Errorf("got %q, expected %q", got, tt.expected)
			}
		})
	}
}
