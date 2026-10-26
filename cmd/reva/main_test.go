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

package main

import (
	"testing"

	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/spf13/cobra"
)

func TestAsWithAnOperatorCommandIsAUsageError(t *testing.T) {
	if code := run([]string{"--as", "bob", "service", "list"}); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestAsWithAnOperatorOptionIsAUsageError(t *testing.T) {
	rt := &cli.Runtime{}
	started := false
	root := rootCommand(rt, &started)
	probe := &cobra.Command{Use: "probe", RunE: func(*cobra.Command, []string) error { return nil }}
	probe.Flags().String("owner", "", "")
	cli.AdminFlag(probe.Flags(), "owner")
	root.AddCommand(probe)

	root.SetArgs([]string{"--as", "bob", "probe", "--owner", "alice"})
	if err := root.Execute(); err == nil || started {
		t.Fatalf("err = %v, started = %v: want a usage error (exit 2)", err, started)
	}
}

func TestNoAdminNamespace(t *testing.T) {
	root := rootCommand(&cli.Runtime{}, new(bool))
	if c, _, _ := root.Find([]string{"admin"}); c != root {
		t.Errorf("`reva admin` still exists: %s", c.CommandPath())
	}
	for _, p := range [][]string{{"service", "config"}, {"invocation", "get"}, {"job", "list"}, {"elevate"}, {"logs"}, {"trace"}, {"stack"}, {"invoke"}, {"impersonate"}} {
		c, _, err := root.Find(p)
		if err != nil || c == root {
			t.Errorf("no command %v", p)
			continue
		}
		if p[0] != "elevate" {
			if pl, _ := cli.PlaneOf(c); pl != cli.AdminPlane {
				t.Errorf("%s is not an operator command", c.CommandPath())
			}
		}
	}
}
