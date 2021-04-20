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
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"time"
)

var (
	dev       = flag.Bool("dev", false, "if dev is set to true creates dev builds with commit and build date")
	commit    = flag.String("commit", "", "sets git commit")
	version   = flag.String("version", "", "sets git version")
	goVersion = flag.String("goversion", "", "sets go version")
	buildDate = time.Now().Format("2006-01-02")

	binaries = []string{"reva", "revad"}
	archs    = []string{"386", "amd64"}
	oses     = []string{"linux", "darwin"}
)

func init() {
	flag.Parse()

	if (*commit == "" || *goVersion == "") && (*version == "" && !*dev) {
		fmt.Fprint(os.Stderr, "fill all the flags\n")
		os.Exit(1)
	}

	// if version is not set we use the dev build setting build date and commit number.
	if *version == "" {
		*version = fmt.Sprintf("%s_%s", time.Now().Format("2006_01_02T150405"), *commit)
	}
}

func main() {

	if err := os.RemoveAll("dist"); err != nil {
		fmt.Fprintf(os.Stderr, "error removing dist folder: %s", err)
		os.Exit(1)
	}

	if err := os.MkdirAll("dist", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating dist folder: %s", err)
		os.Exit(1)
	}

	ldFlags := fmt.Sprintf("-s -X main.buildDate=%s -X main.gitCommit=%s -X main.version=%s -X main.goVersion=%s",
		buildDate,
		*commit,
		*version,
		*goVersion,
	)

	for _, bin := range binaries {
		for _, o := range oses {
			for _, arch := range archs {
				if o == "darwin" && arch == "386" { // https://golang.org/doc/go1.14#darwin
					continue
				}
				out := fmt.Sprintf("./dist/%s_%s_%s_%s", bin, *version, o, arch)
				args := []string{"build", "-o", out, "-ldflags", ldFlags, "./cmd/" + bin}
				cmd := exec.Command("go", args...)
				cmd.Env = os.Environ()
				cmd.Env = append(cmd.Env, []string{"GOOS=" + o, "GOARCH=" + arch}...)
				cmd.Dir = "." // root of the repo
				run(cmd)
				hashFile(out)
			}
		}
	}
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

func hashFile(file string) {
	hasher := sha256.New()
	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := io.Copy(hasher, f); err != nil {
		f.Close()
		log.Fatal(err)
	}
	f.Close()
	val := hex.EncodeToString(hasher.Sum(nil))
	if err := ioutil.WriteFile(file+".sha256", []byte(val), 0644); err != nil {
		log.Fatal(err)
	}
}
