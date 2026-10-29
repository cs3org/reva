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
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Plane is where a call goes: the user plane (CS3 with the login token, or
// the impersonated token of --as) or the admin plane (the Admin API).
type Plane int

const (
	UserPlane Plane = iota
	AdminPlane
)

const (
	planeAnnotation   = "reva.plane"
	mutatesAnnotation = "reva.mutates"
	adminValue        = "admin"
)

// OperatorOptions is the title of the help group of the operator options.
const OperatorOptions = "Operator options (need admin rights):"

// AdminCommand marks cmd and its subcommands as operator commands: they
// always use the admin plane.
func AdminCommand(cmd *cobra.Command) *cobra.Command {
	setAnnotation(&cmd.Annotations, planeAnnotation, adminValue)
	cmd.Short += " (admin)"
	return cmd
}

// AdminFlag marks the flag name of flags as an operator option: when it is
// set, the command uses the admin plane.
func AdminFlag(flags *pflag.FlagSet, name string) {
	f := flags.Lookup(name)
	if f == nil {
		panic("cli: no flag " + name)
	}
	if f.Annotations == nil {
		f.Annotations = map[string][]string{}
	}
	f.Annotations[planeAnnotation] = []string{adminValue}
}

// Mutates marks cmd as a command that changes data. Under --as it asks for
// a confirmation first.
func Mutates(cmd *cobra.Command) *cobra.Command {
	setAnnotation(&cmd.Annotations, mutatesAnnotation, "true")
	return cmd
}

// IsOperator reports whether cmd itself is marked with AdminCommand.
func IsOperator(cmd *cobra.Command) bool {
	return cmd.Annotations[planeAnnotation] == adminValue
}

// IsMutating reports whether cmd is marked with Mutates.
func IsMutating(cmd *cobra.Command) bool {
	return cmd.Annotations[mutatesAnnotation] == "true"
}

func setAnnotation(m *map[string]string, k, v string) {
	if *m == nil {
		*m = map[string]string{}
	}
	(*m)[k] = v
}

// IsOperatorFlag reports whether f is marked with AdminFlag.
func IsOperatorFlag(f *pflag.Flag) bool {
	v := f.Annotations[planeAnnotation]
	return len(v) == 1 && v[0] == adminValue
}

// PlaneOf returns the plane of cmd and, for the admin plane, what selects
// it: the operator command or the first operator option that is set.
func PlaneOf(cmd *cobra.Command) (Plane, string) {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations[planeAnnotation] == adminValue {
			return AdminPlane, "`" + cmd.CommandPath() + "`"
		}
	}
	var why string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if why == "" && IsOperatorFlag(f) {
			why = "--" + f.Name
		}
	})
	if why != "" {
		return AdminPlane, why
	}
	return UserPlane, ""
}

// SelectPlane sets the plane of the command that runs, and refuses the
// combinations that make no sense. Its errors are usage errors.
func (r *Runtime) SelectPlane(cmd *cobra.Command) error {
	r.plane, r.need = PlaneOf(cmd)
	if r.as != "" {
		if r.plane == AdminPlane {
			return fmt.Errorf("--as runs a user command as another user: it cannot be used with %s, which needs admin rights", r.need)
		}
		r.need = "--as"
	}
	if strings.ContainsFunc(r.reason, func(c rune) bool { return c < 0x20 || c > 0x7e }) {
		// gRPC headers allow only printable ASCII.
		return errors.New("--reason must be printable ASCII text")
	}
	return nil
}

// Plane returns the plane that SelectPlane chose for the command.
func (r *Runtime) Plane() Plane { return r.plane }

func flagUsages(cmd *cobra.Command, operator bool) string {
	fs := pflag.NewFlagSet(cmd.Name(), pflag.ContinueOnError)
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if IsOperatorFlag(f) == operator {
			fs.AddFlag(f)
		}
	})
	return fs.FlagUsages()
}

// OperatorHelp puts the operator options of every command in their own
// group of the help. It edits the usage template of cmd.
func OperatorHelp(cmd *cobra.Command) {
	cobra.AddTemplateFunc("userFlagUsages", func(c *cobra.Command) string { return flagUsages(c, false) })
	cobra.AddTemplateFunc("operatorFlagUsages", func(c *cobra.Command) string { return flagUsages(c, true) })
	t := strings.Replace(cmd.UsageTemplate(),
		"{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}",
		"{{userFlagUsages . | trimTrailingWhitespaces}}{{end}}{{with operatorFlagUsages .}}\n\n"+OperatorOptions+"\n{{. | trimTrailingWhitespaces}}{{end}}",
		1)
	cmd.SetUsageTemplate(t)
}
