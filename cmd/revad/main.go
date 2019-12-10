// Copyright 2018-2019 CERN
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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path"
	"regexp"
	"syscall"

	"github.com/cs3org/reva/cmd/revad/internal/grace"
	"github.com/cs3org/reva/cmd/revad/runtime"
)

var (
	versionFlag = flag.Bool("version", false, "show version and exit")
	testFlag    = flag.Bool("t", false, "test configuration and exit")
	signalFlag  = flag.String("s", "", "send signal to a master process: stop, quit, reload")
	configFlag  = flag.String("c", "/etc/revad/revad.toml", "set configuration file")
	pidFlag     = flag.String("p", "", "pid file. If empty defaults to a random file in the OS temporary directory")
	dirFlag     = flag.String("dev-dir", "", "runs any toml file in the specified directory. Intended for development use only")
	// Compile time variables initialized with gcc flags.
	gitCommit, buildDate, version, goVersion string
)

func main() {
	flag.Parse()

	handleDirFlag()
	handleVersionFlag()
	handleSignalFlag()
	handleTestFlag()

	runtime.Run(*configFlag, *pidFlag)
}

func getVersionString() string {
	msg := "version=%s "
	msg += "commit=%s "
	msg += "go_version=%s "
	msg += "build_date=%s"

	return fmt.Sprintf(msg, version, gitCommit, goVersion, buildDate)
}

func handleDirFlag() {
	var configFiles []string
	if *dirFlag != "" {
		files, err := ioutil.ReadDir(*dirFlag)
		if err != nil {
			log.Fatal(err)
		}

		for _, value := range files {
			if !value.IsDir() {
				expr := regexp.MustCompile(`[\w].toml`)

				if expr.Match([]byte(value.Name())) {
					configFiles = append(configFiles, path.Join(*dirFlag, value.Name()))
				}
			}
		}

		stop := make(chan os.Signal, 1)
		defer close(stop)

		for _, file := range configFiles {
			go runtime.Run(file, file+".pid")
		}

		signal.Notify(stop, os.Interrupt)
		for range stop {
			for i := 0; i < len(configFiles); i++ {
				fname := configFiles[i] + ".pid"
				fmt.Printf("removing pid file: %v\n", fname)
				os.Remove(fname)
			}
			os.Exit(0)
		}
	}
}

func handleVersionFlag() {
	if *versionFlag {
		fmt.Fprintf(os.Stderr, "%s\n", getVersionString())
		os.Exit(1)
	}
}

func handleSignalFlag() {
	if *signalFlag != "" {
		var signal syscall.Signal
		switch *signalFlag {
		case "reload":
			signal = syscall.SIGHUP
		case "quit":
			signal = syscall.SIGQUIT
		case "stop":
			signal = syscall.SIGTERM
		default:
			fmt.Fprintf(os.Stderr, "unknown signal %q\n", *signalFlag)
			os.Exit(1)
		}

		// check that we have a valid pidfile
		if *pidFlag == "" {
			fmt.Fprintf(os.Stderr, "-s flag not set, no clue where the pidfile is stored\n")
			os.Exit(1)
		}
		process, err := grace.GetProcessFromFile(*pidFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error getting process from pidfile: %v\n", err)
			os.Exit(1)
		}

		// kill process with signal
		if err := process.Signal(signal); err != nil {
			fmt.Fprintf(os.Stderr, "error signaling process %d with signal %s\n", process.Pid, signal)
			os.Exit(1)
		}

		os.Exit(0)
	}
}

func handleTestFlag() {
	if *testFlag {
		os.Exit(0)
	}
}
