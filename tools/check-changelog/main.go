// Copyright 2018-2020 CERN
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
	"errors"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func main() {
	repo := flag.String("repo", "", "the remote repo against which diff-index is to be derived")
	branch := "master"
	if *repo != "" {
		branch += "/master"
	}

	cmd := exec.Command("git", "diff-index", branch, "--", "changelog/unreleased")
	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}

	var changelog bool
	mods := strings.Split(string(out), "\n")
	fmt.Printf("%+q\n", mods)

	for _, m := range mods {
		params := strings.Split(m, " ")
		// The fifth param in the output of diff-index is always the status followed by optional score number
		if len(params) >= 5 && params[4][0] == 'A' {
			changelog = true
		}
	}

	if !changelog {
		log.Fatal(errors.New("No changelog added. Please create a changelog item based on your changes"))
	}
}
