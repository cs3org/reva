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
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
)

/*
var (
	version = flag.String("version", "", "version to release: 0.0.1")
	commit  = flag.Bool("commit", false, "creates a commit")
	tag     = flag.Bool("tag", false, "creates a tag")
)
*/

func init() {
	flag.Parse()

}

func main() {
	cmd := exec.Command("git", "diff-index", "--quiet", "HEAD", "--", "chanelog/unreleased")
	run(cmd)
}

func run(cmd *exec.Cmd) {
	var b bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &b)
	cmd.Stdout = mw
	cmd.Stderr = mw
	err := cmd.Run()
	fmt.Println(cmd.Dir, cmd.Args)
	fmt.Println(b.String())
	if err != nil {
		fmt.Println("ERROR: ", err.Error())
		os.Exit(1)
	}
}
