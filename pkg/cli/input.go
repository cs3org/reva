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

package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// ReadSecret reads a secret from the terminal without echo, after prompt.
// Without a terminal it reads one line of stdin, so that a script can pipe
// the secret. Do not take a secret from a flag: ps and the shell history
// show flags.
func ReadSecret(cmd *cobra.Command, prompt string) (string, error) {
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(cmd.ErrOrStderr(), prompt)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		return string(b), err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || line == "") {
		return "", fmt.Errorf("reading the secret: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// Complete completes the first n arguments with the values that list
// returns. A negative n completes any argument, for a flag. A value can
// have a description after a tab. Completion does not run the hooks of the
// root command, so Complete loads the settings first.
func Complete(rt *Runtime, n int, list func(ctx context.Context, toComplete string) ([]string, error)) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		none := cobra.ShellCompDirectiveNoFileComp
		if n >= 0 && len(args) >= n {
			return nil, none
		}
		if err := rt.Load(cmd.Flags()); err != nil {
			return nil, none
		}
		vals, err := list(cmd.Context(), toComplete)
		if err != nil {
			rt.Debugf("completion: %v", err)
			return nil, none
		}
		return vals, none
	}
}
