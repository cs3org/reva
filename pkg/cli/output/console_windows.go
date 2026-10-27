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

package output

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnableVirtualTerminal turns on ANSI escape handling for the console behind
// standard output and standard error.
//
// Colour, progress bars and the full-screen browsers are all written as escape
// sequences. Windows Terminal interprets them regardless, but the classic
// console host prints them as literal "←[33m" unless a program opts in, and Go
// does not opt in on a program's behalf. A handle that is not a console, or a
// console too old to understand the mode, is left as it was.
func EnableVirtualTerminal() {
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		h := windows.Handle(f.Fd())
		var mode uint32
		if windows.GetConsoleMode(h, &mode) != nil {
			continue
		}
		_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}
