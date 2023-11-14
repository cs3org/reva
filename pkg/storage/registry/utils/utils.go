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

// Package utils contains utilities for storage registries
package utils

import (
	"regexp"
	"strings"
)

// GenerateRegexCombinations expands bracket regexes.
func GenerateRegexCombinations(rex string) []string {
	var bracketRegex = regexp.MustCompile(`\[(.*?)\]`)
	m := bracketRegex.FindString(rex)
	r := strings.Trim(strings.Trim(m, "["), "]")
	if r == "" {
		return []string{rex}
	}
	var combinations []string
	for i := 0; i < len(r); i++ {
		if i < len(r)-2 && r[i+1] == '-' {
			for j := r[i]; j <= r[i+2]; j++ {
				p := strings.Replace(rex, m, string(j), 1)
				combinations = append(combinations, GenerateRegexCombinations(p)...)
			}
			i += 2
		} else {
			p := strings.Replace(rex, m, string(r[i]), 1)
			combinations = append(combinations, GenerateRegexCombinations(p)...)
		}
	}
	return combinations
}
