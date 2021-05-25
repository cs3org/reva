// Copyright 2018-2021 CERN
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
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// Case-insensitive list of PRs for which changelog enforcement needs to be skipped
var skipTags = []string{"[tests-only]", "[build-deps]", "[docs-only]"}

func skipPR(prID int) bool {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_API_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	pr, _, err := client.PullRequests.Get(ctx, "cs3org", "reva", prID)
	if err != nil {
		log.Panic(err)
	}
	prTitle := strings.ToLower(pr.GetTitle())

	for _, tag := range skipTags {
		if strings.HasPrefix(prTitle, tag) {
			log.Print("Skipping changelog check for tag: " + tag)
			return true
		}
	}
	return false
}

func main() {
	repo := flag.String("repo", "", "the remote repo against which diff-index is to be derived")
	prID := flag.Int("pr", 0, "the ID of the PR")
	flag.Parse()

	if *prID > 0 && skipPR(*prID) {
		return
	}

	branch := "master"
	if *repo != "" {
		branch = *repo + "/master"
	}

	cmd := exec.Command("git", "diff-index", branch, "--", ".")
	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	// Return successfully if there are no changes
	if len(out) == 0 {
		return
	}

	cmd = exec.Command("git", "diff-index", branch, "--", "changelog/unreleased")
	out, err = cmd.Output()
	if err != nil {
		log.Fatal(err)
	}

	mods := strings.Split(string(out), "\n")
	for _, m := range mods {
		params := strings.Split(m, " ")
		// The fifth param in the output of diff-index is always the status followed by optional score number
		if len(params) >= 5 && (params[4][0] == 'A' || params[4][0] == 'M') {
			return
		}
	}

	log.Fatal(errors.New("No changelog added. Please create a changelog item based on your changes"))
}
