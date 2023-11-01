// Package utils contains utilities for storage registries
package utils

import (
	"regexp"
	"strings"
)

// GenerateRegexCombinations expands bracket regexes
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
