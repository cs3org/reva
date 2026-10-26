// Copyright 2018-2026 CERN
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

// Command brandcheck checks a brand file of the CLI: it parses it strictly,
// and checks it against the build it runs in (revacmd.CheckBrand).
//
//	go run github.com/cs3org/reva/v3/cmd/reva/brandcheck brand.toml
//
// It sees only the sign-in methods of the core. A build with cli.auth.*
// plugins checks its own brand with `go run . __brandcheck`, or with
// revacmd.CheckBrand in a test that imports the plugins.
package main

import (
	"fmt"
	"io"
	"os"

	revacmd "github.com/cs3org/reva/v3/cmd/reva/app"
	"github.com/cs3org/reva/v3/pkg/cli/brand"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, w io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(w, "usage: brandcheck FILE...")
		return 2
	}
	code := 0
	for _, p := range args {
		if err := check(p); err != nil {
			fmt.Fprintf(w, "%s: %v\n", p, err)
			code = 1
		}
	}
	return code
}

func check(p string) error {
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	b, err := brand.Parse(data)
	if err != nil {
		return err
	}
	return revacmd.CheckBrand(b)
}
