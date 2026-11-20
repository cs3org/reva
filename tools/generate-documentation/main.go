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

// Command generate-documentation writes the parts of the website that come
// from the code: a page per component, and the data the site's history,
// community, and home pages show. Run it from the repository root.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	contributors := flag.Bool("contributors", false, "refresh the contributor list from the GitHub API")
	flag.Parse()

	if err := run(*contributors); err != nil {
		fmt.Fprintln(os.Stderr, "generate-documentation:", err)
		os.Exit(1)
	}
}

func run(withContributors bool) error {
	const (
		contentDir = "docs/content/en"
		dataDir    = "docs/data/generated"
	)

	pkgs, err := loadPackages("pkg", "internal")
	if err != nil {
		return err
	}
	c := buildCatalog(pkgs)
	if err := writePages(contentDir, c); err != nil {
		return err
	}

	rels, err := releases("changelog", contentDir)
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dataDir, "releases.json"), rels); err != nil {
		return err
	}

	act, err := gitActivity()
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dataDir, "activity.json"), act); err != nil {
		return err
	}

	if withContributors {
		list, err := fetchContributors()
		if err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(dataDir, "contributors.json"), list); err != nil {
			return err
		}
	}

	byKind := map[string]int{}
	for _, comp := range c.components {
		byKind[comp.Kind]++
	}
	version, _ := os.ReadFile("VERSION")
	date, _ := os.ReadFile("RELEASE_DATE")
	stats := map[string]any{
		"version":      strings.TrimSpace(string(version)),
		"release_date": strings.TrimSpace(string(date)),
		"components":   len(c.components),
		"by_kind":      byKind,
		"families":     len(c.families),
		"releases":     len(rels),
	}
	return writeJSON(filepath.Join(dataDir, "stats.json"), stats)
}
