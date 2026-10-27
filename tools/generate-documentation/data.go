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

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

type release struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	Notes   string `json:"notes,omitempty"` // site URL of the release notes, if published
}

// releases reads the changelog directories, which are named <version>_<date>.
func releases(changelogDir, contentDir string) ([]release, error) {
	entries, err := os.ReadDir(changelogDir)
	if err != nil {
		return nil, err
	}
	var out []release
	for _, e := range entries {
		version, date, ok := strings.Cut(e.Name(), "_")
		if !e.IsDir() || !ok || !semver.IsValid("v"+version) {
			continue
		}
		r := release{Version: version, Date: date}
		notes := filepath.Join(contentDir, "docs", "changelog", version, "_index.md")
		if _, err := os.Stat(notes); err == nil {
			r.Notes = "/docs/changelog/" + version + "/"
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return semver.Compare("v"+out[i].Version, "v"+out[j].Version) < 0 })
	return out, nil
}

type year struct {
	Year    string `json:"year"`
	Commits int    `json:"commits"`
	Authors int    `json:"authors"`
}

type activity struct {
	FirstCommit string `json:"first_commit"`
	Commits     int    `json:"commits"`
	Years       []year `json:"years"`
}

func gitActivity() (*activity, error) {
	out, err := exec.Command("git", "log", "--no-merges", "--format=%as%x09%ae%x09%an").Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	commits := map[string]int{}
	authors := map[string]map[string]bool{}
	a := &activity{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 || strings.Contains(line, "[bot]") {
			continue
		}
		y := parts[0][:4]
		commits[y]++
		if authors[y] == nil {
			authors[y] = map[string]bool{}
		}
		authors[y][strings.ToLower(parts[1])] = true
		a.Commits++
		if a.FirstCommit == "" || parts[0] < a.FirstCommit {
			a.FirstCommit = parts[0]
		}
	}
	for y, n := range commits {
		a.Years = append(a.Years, year{Year: y, Commits: n, Authors: len(authors[y])})
	}
	sort.Slice(a.Years, func(i, j int) bool { return a.Years[i].Year < a.Years[j].Year })
	return a, nil
}

type contributor struct {
	Login         string `json:"login"`
	Avatar        string `json:"avatar"`
	URL           string `json:"url"`
	Contributions int    `json:"contributions"`
}

// fetchContributors uses GITHUB_TOKEN, if set, for a higher rate limit.
func fetchContributors() ([]contributor, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	var out []contributor
	for page := 1; ; page++ {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.github.com/repos/cs3org/reva/contributors?per_page=100&page=%d", page), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		if t := os.Getenv("GITHUB_TOKEN"); t != "" {
			req.Header.Set("Authorization", "Bearer "+t)
		}
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		var batch []struct {
			Login         string `json:"login"`
			AvatarURL     string `json:"avatar_url"`
			HTMLURL       string `json:"html_url"`
			Type          string `json:"type"`
			Contributions int    `json:"contributions"`
		}
		err = json.NewDecoder(res.Body).Decode(&batch)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("github: %s", res.Status)
		}
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			return out, nil
		}
		for _, c := range batch {
			if c.Type == "Bot" {
				continue
			}
			out = append(out, contributor{Login: c.Login, Avatar: c.AvatarURL, URL: c.HTMLURL, Contributions: c.Contributions})
		}
	}
}

func writeJSON(file string, v any) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(b, '\n'), 0o644)
}
