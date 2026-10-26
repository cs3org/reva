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
	"os"
	"regexp"
	"strings"

	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/jedib0t/go-pretty/text"
	"github.com/spf13/cobra"
)

var (
	headingColor = text.Colors{text.Bold, text.FgCyan}
	commandColor = text.Colors{text.Bold, text.FgGreen}
	flagColor    = text.Colors{text.FgYellow}
	typeColor    = text.Colors{text.Italic}
	errorColor   = text.Colors{text.Bold, text.FgRed}
)

// flagLine matches a line of pflag's FlagUsages: the flag names, the value
// type if any, and the description after a gap of two or more spaces.
var flagLine = regexp.MustCompile(`^(\s+)((?:-\w, )?--[\w-]+)( \w+)?(\s{2,}.*)$`)

// colorHelp colors the help of cmd and of all its children: the headings,
// the command names and the flags. It edits the default cobra template
// instead of a copy, so a template change in cobra only loses some colors.
func colorHelp(cmd *cobra.Command) {
	if !cli.UseColor(os.Stdout) {
		return
	}
	cobra.AddTemplateFunc("heading", headingColor.Sprint)
	cobra.AddTemplateFunc("command", commandColor.Sprint)
	cobra.AddTemplateFunc("bold", text.Bold.Sprint)
	cobra.AddTemplateFunc("flags", colorFlags)
	r := strings.NewReplacer(
		"Usage:{{", `{{heading "Usage:"}}{{`,
		"\nAliases:\n", "\n{{heading \"Aliases:\"}}\n",
		"\nExamples:\n", "\n{{heading \"Examples:\"}}\n",
		"\nAvailable Commands:", "\n{{heading \"Available Commands:\"}}",
		"\n{{.Title}}", "\n{{heading .Title}}",
		"\nAdditional Commands:", "\n{{heading \"Additional Commands:\"}}",
		"\nFlags:\n", "\n{{heading \"Flags:\"}}\n",
		"\nGlobal Flags:\n", "\n{{heading \"Global Flags:\"}}\n",
		"\n"+cli.OperatorOptions+"\n", "\n{{heading \""+cli.OperatorOptions+"\"}}\n",
		"\nAdditional help topics:", "\n{{heading \"Additional help topics:\"}}",
		"{{.UseLine}}", "{{bold .UseLine}}",
		"  {{.CommandPath}} [command]{{end}}", `  {{bold .CommandPath " [command]"}}{{end}}`,
		"{{rpad .Name .NamePadding }}", "{{command (rpad .Name .NamePadding)}}",
		"| trimTrailingWhitespaces}}", "| trimTrailingWhitespaces | flags}}",
	)
	cmd.SetUsageTemplate(r.Replace(cmd.UsageTemplate()))
}

func colorFlags(usages string) string {
	lines := strings.Split(usages, "\n")
	for i, l := range lines {
		m := flagLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		typ := m[3]
		if typ != "" {
			typ = typeColor.Sprint(typ)
		}
		lines[i] = m[1] + flagColor.Sprint(m[2]) + typ + m[4]
	}
	return strings.Join(lines, "\n")
}
