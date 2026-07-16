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

package admin

import (
	"flag"
	"fmt"
	"io"
)

// command is the admin CLI's own minimal subcommand type (kept internal so the
// package is self-contained and importable, unlike the top-level `main`).
type command struct {
	*flag.FlagSet
	Name        string
	Action      func(w ...io.Writer) error
	Usage       func() string
	Description func() string
	ResetFlags  func()
}

func newCommand(name string) *command {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	c := &command{
		Name:        name,
		Usage:       func() string { return fmt.Sprintf("Usage: %s", name) },
		Action:      func(w ...io.Writer) error { return nil },
		Description: func() string { return "" },
		FlagSet:     fs,
		ResetFlags:  func() {},
	}
	// Make `-h` print the command's own synopsis (not Go's bare "Usage of
	// <name>:") followed by its flags. A command with subcommands can replace
	// this with a richer guide via cmd.FlagSet.Usage.
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), c.Usage())
		fmt.Fprintln(fs.Output(), "\nFlags:")
		fs.PrintDefaults()
	}
	return c
}
