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

package net

import (
	"fmt"
	"regexp"
	"strings"
)

type ctxKey int

const (
	// CtxKeyBaseURI is the key of the base URI context field
	CtxKeyBaseURI ctxKey = iota

	// NsDav is the Dav ns
	NsDav = "DAV:"
	// NsOwncloud is the owncloud ns
	NsOwncloud = "http://owncloud.org/ns"
	// NsOCS is the OCS ns
	NsOCS = "http://open-collaboration-services.org/ns"

	// RFC1123 time that mimics oc10. time.RFC1123 would end in "UTC", see https://github.com/golang/go/issues/13781
	RFC1123 = "Mon, 02 Jan 2006 15:04:05 GMT"

	// PropQuotaUnknown is the quota unknown property
	PropQuotaUnknown = "-2"
	// PropOcFavorite is the favorite ns property
	PropOcFavorite = "http://owncloud.org/ns/favorite"
)

// replaceAllStringSubmatchFunc is taken from 'Go: Replace String with Regular Expression Callback'
// see: https://elliotchance.medium.com/go-replace-string-with-regular-expression-callback-f89948bad0bb
func replaceAllStringSubmatchFunc(re *regexp.Regexp, str string, repl func([]string) string) string {
	result := ""
	lastIndex := 0
	for _, v := range re.FindAllSubmatchIndex([]byte(str), -1) {
		groups := []string{}
		for i := 0; i < len(v); i += 2 {
			groups = append(groups, str[v[i]:v[i+1]])
		}
		result += str[lastIndex:v[0]] + repl(groups)
		lastIndex = v[1]
	}
	return result + str[lastIndex:]
}

var hrefre = regexp.MustCompile(`([^A-Za-z0-9_\-.~()/:@!$])`)

// EncodePath encodes the path of a url.
//
// slashes (/) are treated as path-separators.
// ported from https://github.com/sabre-io/http/blob/bb27d1a8c92217b34e778ee09dcf79d9a2936e84/lib/functions.php#L369-L379
func EncodePath(path string) string {
	return replaceAllStringSubmatchFunc(hrefre, path, func(groups []string) string {
		b := groups[1]
		var sb strings.Builder
		for i := 0; i < len(b); i++ {
			sb.WriteString(fmt.Sprintf("%%%x", b[i]))
		}
		return sb.String()
	})
}
