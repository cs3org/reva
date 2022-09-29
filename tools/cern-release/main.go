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
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	author = flag.String("author", "", "the author that creates the release")
	email  = flag.String("email", "", "the email of the authot that creates the release")

	remote = flag.String("remote", "cernbox", "the remote git repository name")
	branch = flag.String("branch", "cernbox", "the branch of the git repository")
)

const (
	specFile = "revad.spec"
)

func init() {
	flag.Parse()

	if *author == "" || *email == "" {
		fmt.Fprintln(os.Stderr, "fill the author and email flags")
		os.Exit(1)
	}
}

func main() {
	err := releaseNewVersion(*author, *email)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func releaseNewVersion(author, email string) error {
	specContent, err := readSpecFile()
	if err != nil {
		return fmt.Errorf("error reading spec content: %w", err)
	}

	version := getVersion(specContent) + 1
	versionStr := fmt.Sprintf("0.0.%d", version)

	// update the version in the spec file
	for i, line := range specContent {
		if strings.HasPrefix(line, "Version:") {
			specContent[i] = "Version: " + versionStr
		}
	}

	// add the changelog
	changelogHeader := -1
	for i, line := range specContent {
		if strings.HasPrefix(line, "%changelog") {
			changelogHeader = i
		}
	}

	if changelogHeader == -1 {
		return errors.New("changelog header not found in spec file")
	}

	var newChangelog []string
	today := time.Now().Format("Mon Jan 02 2006")
	newChangelog = append(newChangelog, fmt.Sprintf("* %s %s <%s> %s", today, author, email, versionStr))
	newChangelog = append(newChangelog, fmt.Sprintf("- v%s", versionStr))

	var newSpec []string
	newSpec = append(newSpec, specContent[:changelogHeader+1]...)
	newSpec = append(newSpec, newChangelog...)
	newSpec = append(newSpec, specContent[changelogHeader+1:]...)

	err = writeSpecFile(newSpec)
	if err != nil {
		return fmt.Errorf("error updating spec file: %w", err)
	}

	tag := "v" + versionStr

	run(exec.Command("git", "add", specFile))
	run(exec.Command("git", "commit", fmt.Sprintf("-m version %s", versionStr)))
	run(exec.Command("git", "tag", "-a", tag, fmt.Sprintf("-m %s", versionStr)))
	run(exec.Command("git", "push", *remote, *branch))
	run(exec.Command("git", "push", *remote, tag))

	return nil
}

func readSpecFile() ([]string, error) {
	f, err := os.Open(specFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scan := bufio.NewScanner(f)

	var spec []string
	for scan.Scan() {
		spec = append(spec, scan.Text())
	}

	return spec, nil
}

func writeSpecFile(spec []string) error {
	f, err := os.OpenFile(specFile, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, line := range spec {
		f.WriteString(line + "\n")
	}

	return nil
}

func getVersion(spec []string) int {
	for _, line := range spec {
		if strings.HasPrefix(line, "Version:") {
			v := strings.TrimPrefix(line, "Version:")
			split := strings.Split(v, ".")

			ver, err := strconv.ParseInt(split[len(split)-1], 10, 64)
			if err != nil {
				return -1
			}
			return int(ver)
		}
	}
	return -1
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
