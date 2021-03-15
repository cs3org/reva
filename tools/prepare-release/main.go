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
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

var (
	version = flag.String("version", "", "version to release: 0.0.1")
	commit  = flag.Bool("commit", false, "creates a commit")
	tag     = flag.Bool("tag", false, "creates a tag")
)

func init() {
	flag.Parse()

	if *version == "" {
		fmt.Fprintf(os.Stderr, "missing version: use -version flag\n")
		os.Exit(1)
	}
}

func main() {
	// check if repo is dirty
	if isRepoDirty() {
		fmt.Fprintf(os.Stderr, "the repo is dirty, to generate a new release all changes need to be committed\n")
		os.Exit(1)
		return
	}

	// also the build is okay
	cmd := exec.Command("make", "release")
	run(cmd)

	fmt.Printf("Generating new release: version=%s\n", *version)

	dt := time.Now()
	date := dt.Format("2006-01-02")
	newChangelog := fmt.Sprintf("changelog/%s_%s", *version, date)

	if info, _ := os.Stat("changelog/unreleased"); info == nil {
		fmt.Fprintf(os.Stderr, "no changelog/unreleased folder, to create a new version you need to fill it")
		os.Exit(1)
	}

	cmd = exec.Command("mv", "changelog/unreleased", newChangelog)
	run(cmd)

	// install release-deps: calens
	cmd = exec.Command("make", "release-deps")
	run(cmd)

	// create new changelog
	cmd = exec.Command(getGoBin("calens"), "-o", "CHANGELOG.md")
	run(cmd)

	// add new VERSION and BUILD_DATE
	if err := ioutil.WriteFile("VERSION", []byte(*version), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing to VERSION file: %s", err)
		os.Exit(1)
	}

	// add new VERSION and RELEASE_DATE
	if err := ioutil.WriteFile("RELEASE_DATE", []byte(date), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing to RELEASE_DATE file: %s", err)
		os.Exit(1)
	}

	tmp, err := ioutil.TempDir("", "reva-changelog")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating tmp directory to store changelog: %s", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(path.Join(tmp, "changelog"), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating changelog in temporary directory: %s", tmp)
		os.RemoveAll(tmp)
		os.Exit(1)
	}

	dir := path.Join(tmp, fmt.Sprintf("changelog/%s_%s", *version, date))
	cmd = exec.Command("cp", "-a", fmt.Sprintf("changelog/%s_%s", *version, date), dir)
	run(cmd)

	// create new changelog
	cmd = exec.Command(getGoBin("calens"), "-o", "changelog/NOTE.md", "-i", path.Join(tmp, "changelog"))
	run(cmd)

	// Generate changelog also in the documentation
	if err := os.MkdirAll(fmt.Sprintf("docs/content/en/docs/changelog/%s", *version), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating docs/content/en/docs/changelog/%s: %s", *version, err)
		os.RemoveAll(tmp)
		os.Exit(1)
	}
	os.RemoveAll(tmp)

	data, err := ioutil.ReadFile("changelog/NOTE.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading NOTE.md: %s", err)
		os.Exit(1)
	}

	releaseDocs := fmt.Sprintf(`
---
title: "v%s"
linkTitle: "v%s"
weight: 40
description: >
  Changelog for Reva v%s (%s)
---

`, *version, *version, *version, date)

	releaseDocs += string(data)
	if err := ioutil.WriteFile(fmt.Sprintf("docs/content/en/docs/changelog/%s/_index.md", *version), []byte(releaseDocs), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing docs release file _index.md: %s", err)
		os.Exit(1)
	}

	add(fmt.Sprintf("v%s", *version),
		"changelog",
		"CHANGELOG.md",
		"VERSION",
		"RELEASE_DATE",
		"docs/content/en/docs/changelog",
	)

	if *commit {
		createCommit(fmt.Sprintf("v%s", *version))
		fmt.Println("Commit created, check with git log")
	}

	if *tag {
		createTag(*version)
		fmt.Println("Tag created, check with git tag")
	}

	if *tag && *commit {
		fmt.Println("RELEASE READY: you only need to\n$ git push --follow-tags")
		os.Exit(0)
	} else {
		fmt.Println("Was a dry run, run with -commit and -tag to create release")
		os.Exit(1)
	}
}

func isRepoDirty() bool {
	repo := "."
	cmd := exec.Command("git", "status", "-s")
	cmd.Dir = repo
	changes := runAndGet(cmd)
	if changes != "" {
		fmt.Println("repo is dirty")
		fmt.Println(changes)
	}
	return changes != ""
}

func add(msg string, files ...string) {
	for _, f := range files {
		cmd := exec.Command("git", "add", "--all", f)
		cmd.Dir = "."
		run(cmd)
	}

}

func createCommit(msg string) {
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = "." // always run from the root of the repo
	run(cmd)
}

func createTag(version string) {
	// check if repo is dirty
	if isRepoDirty() {
		fmt.Fprintf(os.Stderr, "repo is dirty when creating a new tag for version %s", version)
		os.Exit(1)
	}

	cmd := exec.Command("git", "tag", "-a", "v"+version, "-m", "v"+version)
	run(cmd)
}

func getGoBin(tool string) string {
	cmd := exec.Command("go", "env", "GOPATH")
	gopath := runAndGet(cmd)
	gobin := fmt.Sprintf("%s/bin", gopath)
	return path.Join(gobin, tool)
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

func runAndGet(cmd *exec.Cmd) string {
	var b bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &b)
	cmd.Stderr = mw
	out, err := cmd.Output()
	fmt.Println(cmd.Dir, cmd.Args)
	fmt.Println(b.String())
	if err != nil {
		fmt.Println("ERROR: ", err.Error())
		os.Exit(1)
	}
	return strings.TrimSpace(string(out))
}
