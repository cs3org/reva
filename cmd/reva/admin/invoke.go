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
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/spf13/cobra"
)

func invocationCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "invocation", Short: "Show the invocations that the services expose"}
	cmd.AddCommand(
		&cobra.Command{
			Use:               "list <service> [invocation]",
			Short:             "List the invocations of a service, or show one with its arguments",
			Args:              cobra.RangeArgs(1, 2),
			ValidArgsFunction: completeCatalogue(false),
			RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) == 2 {
					return showInvocation(cmd, args[0], args[1])
				}
				client, ctx, err := rt.Admin(cmd.Context())
				if err != nil {
					return err
				}
				res, err := client.ListInvocations(ctx, &adminpb.ListInvocationsRequest{Service: args[0]})
				if err != nil {
					return adminErr(err)
				}
				t := cli.Table{Headers: []string{"Name", "Kind", "Arguments", "Description"}, Items: res.Invocations}
				for _, inv := range res.Invocations {
					t.Rows = append(t.Rows, []string{inv.Name, invocationKind(inv), compactArgs(inv), inv.Description})
				}
				return rt.Print(cmd.OutOrStdout(), t, nil)
			},
		},
		&cobra.Command{
			Use:               "get <service> <invocation>",
			Short:             "Show one invocation of a service, with its arguments",
			Args:              cobra.ExactArgs(2),
			ValidArgsFunction: completeCatalogue(false),
			RunE: func(cmd *cobra.Command, args []string) error {
				return showInvocation(cmd, args[0], args[1])
			},
		},
	)
	return cmd
}

func showInvocation(cmd *cobra.Command, service, name string) error {
	client, ctx, err := rt.Admin(cmd.Context())
	if err != nil {
		return err
	}
	res, err := client.ListInvocations(ctx, &adminpb.ListInvocationsRequest{Service: service})
	if err != nil {
		return adminErr(err)
	}
	i := slices.IndexFunc(res.Invocations, func(inv *adminpb.InvocationSpec) bool { return inv.Name == name })
	if i < 0 {
		return fmt.Errorf("service %s has no invocation %q", service, name)
	}
	inv := res.Invocations[i]
	return rt.Print(cmd.OutOrStdout(), inv, func(w io.Writer) {
		if inv.Description != "" {
			fmt.Fprintf(w, "%s: %s\n", inv.Name, inv.Description)
		} else {
			fmt.Fprintln(w, inv.Name)
		}
		if inv.Kind == "" {
			fmt.Fprintln(w, "No instance answered: the kind and the arguments are not known.")
			return
		}
		scope := "every instance"
		if inv.Scope == string(invoke.ScopeShared) {
			scope = "one instance (shared)"
		}
		streaming := "no"
		switch {
		case inv.StreamOnly:
			streaming = "only"
		case inv.Streaming:
			streaming = "yes, with --stream"
		}
		fmt.Fprintf(w, "kind: %s\nruns on: %s\nstreaming: %s\n", inv.Kind, scope, streaming)
		if len(inv.Args) == 0 {
			fmt.Fprintln(w, "no arguments")
			return
		}
		t := cli.Table{Headers: []string{"Argument", "Type", "Required", "Default", "Values", "Description"}}
		for _, a := range inv.Args {
			typ := a.Type
			if a.Repeated {
				typ = "list of " + typ
			}
			req := ""
			if a.Required {
				req = "yes"
			}
			t.Rows = append(t.Rows, []string{a.Name, typ, req, a.Default, strings.Join(a.Enum, ", "), a.Description})
		}
		t.Write(w)
	})
}

func compactArgs(inv *adminpb.InvocationSpec) string {
	if inv.Kind == "" {
		return "?"
	}
	parts := make([]string, 0, len(inv.Args))
	for _, a := range inv.Args {
		if a.Required {
			parts = append(parts, a.Name)
		} else {
			parts = append(parts, "["+a.Name+"]")
		}
	}
	return strings.Join(parts, " ")
}

func invocationKind(inv *adminpb.InvocationSpec) string {
	kind := inv.Kind
	if kind == "" {
		kind = "?"
	}
	switch {
	case inv.StreamOnly:
		kind += ",stream-only"
	case inv.Streaming:
		kind += ",stream"
	}
	if inv.Scope != "" {
		kind += "," + inv.Scope
	}
	return kind
}

func invokeCommand() *cobra.Command {
	var stream bool
	cmd := &cobra.Command{
		Use:   "invoke <selector> <invocation> [key=value ...]",
		Short: "Run a service invocation on one or all instances",
		Long: "Run a service invocation. The selector is a service name (every instance) or a node id (one instance).\n\n" +
			"The arguments are key=value pairs, and a list is comma-separated. `reva invocation get <service> <invocation>`\n" +
			"shows the arguments of an invocation. They are checked against the catalogue before the call.\n\n" +
			"A list result prints as a table, anything else as key: value lines. -o json prints one document,\n" +
			"[{node, result, error}], or one per line for a stream. The exit code is 1 if an instance failed.",
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: completeCatalogue(true),
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			selector, invocation := posArgs[0], posArgs[1]
			args, err := parseKeyVals(posArgs[2:])
			if err != nil {
				return err
			}

			client, ctx, err := rt.Admin(cmd.Context())
			if err != nil {
				return err
			}

			spec := invocationSpec(ctx, client, selector, invocation)
			if err := checkArgs(spec, args); err != nil {
				return &cli.UsageError{Err: err}
			}
			if stream && spec.GetKind() != "" && !spec.Streaming {
				return fmt.Errorf("invocation %s does not stream", invocation)
			}
			if spec.GetKind() == "dangerous" {
				if err := rt.Confirm(fmt.Sprintf("invocation %q on %q is DANGEROUS; proceed?", invocation, selector)); err != nil {
					return err
				}
			}

			if stream || spec.GetStreamOnly() {
				// A shared invocation runs on one instance, and so does a node id.
				s := &streamWriter{w: cmd.OutOrStdout(), errW: cmd.ErrOrStderr(),
					node: !strings.Contains(selector, "/") && spec.GetScope() != string(invoke.ScopeShared)}
				return runInvokeStream(ctx, client, &adminpb.InvokeRequest{Service: selector, Invocation: invocation, Args: args}, s)
			}

			res, err := client.Invoke(ctx, &adminpb.InvokeRequest{
				Service:    selector,
				Invocation: invocation,
				Args:       args,
			})
			if err != nil {
				return adminErr(err)
			}
			return writeResults(cmd.OutOrStdout(), cmd.ErrOrStderr(), res.Results)
		},
	}
	cmd.Flags().BoolVar(&stream, "stream", false, "use the streaming form of an invocation that also has a single result, for example to follow logs (Ctrl-C stops); a stream-only invocation always streams")
	return cmd
}

// checkArgs makes a typo fail before the call goes to every instance. The
// server checks the arguments again.
func checkArgs(spec *adminpb.InvocationSpec, args map[string]string) error {
	// A spec without a kind has only the names: there is nothing to check.
	if spec.GetKind() == "" {
		return nil
	}
	names := make([]string, 0, len(spec.Args))
	for _, a := range spec.Args {
		names = append(names, a.Name)
	}
	for _, k := range slices.Sorted(maps.Keys(args)) {
		if !slices.Contains(names, k) {
			if len(names) == 0 {
				return fmt.Errorf("invocation %s takes no arguments, got %q", spec.Name, k)
			}
			return fmt.Errorf("invocation %s has no argument %q: use %s", spec.Name, k, strings.Join(names, ", "))
		}
	}
	for _, a := range spec.Args {
		v, ok := args[a.Name]
		if !ok {
			if a.Required {
				return fmt.Errorf("invocation %s needs the argument %s=<%s>", spec.Name, a.Name, a.Type)
			}
			continue
		}
		if len(a.Enum) == 0 {
			continue
		}
		values := []string{v}
		if a.Repeated {
			values = strings.Split(v, ",")
		}
		for _, e := range values {
			if e = strings.TrimSpace(e); (e != "" || !a.Repeated) && !slices.Contains(a.Enum, e) {
				return fmt.Errorf("argument %s: %q is not one of %s", a.Name, e, strings.Join(a.Enum, ", "))
			}
		}
	}
	return nil
}

func completeCatalogue(keyVals bool) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		none := cobra.ShellCompDirectiveNoFileComp
		if len(args) >= 2 && !keyVals {
			return nil, none
		}
		// Completion does not run the PersistentPreRunE, so load the settings here.
		if err := rt.Load(cmd.Flags()); err != nil {
			return nil, none
		}
		client, ctx, err := rt.Admin(cmd.Context())
		if err != nil {
			return nil, none
		}
		if len(args) == 0 {
			res, err := client.ListServices(ctx, &adminpb.ListServicesRequest{})
			if err != nil {
				return nil, none
			}
			var out []string
			for _, s := range res.Services {
				if !slices.Contains(out, s.Name) {
					out = append(out, s.Name)
				}
			}
			return out, none
		}
		if len(args) == 1 {
			res, err := client.ListInvocations(ctx, &adminpb.ListInvocationsRequest{Service: args[0]})
			if err != nil {
				return nil, none
			}
			out := make([]string, 0, len(res.Invocations))
			for _, inv := range res.Invocations {
				out = append(out, inv.Name+"\t"+inv.Description)
			}
			return out, none
		}
		spec := invocationSpec(ctx, client, args[0], args[1])
		if spec == nil {
			return nil, none
		}
		if k, _, ok := strings.Cut(toComplete, "="); ok {
			i := slices.IndexFunc(spec.Args, func(a *adminpb.ArgSpec) bool { return a.Name == k })
			if i < 0 {
				return nil, none
			}
			values := spec.Args[i].Enum
			if len(values) == 0 && spec.Args[i].Type == "bool" {
				values = []string{"true", "false"}
			}
			out := make([]string, 0, len(values))
			for _, v := range values {
				out = append(out, k+"="+v)
			}
			return out, none
		}
		var out []string
		for _, a := range spec.Args {
			if !slices.ContainsFunc(args[2:], func(kv string) bool { return strings.HasPrefix(kv, a.Name+"=") }) {
				out = append(out, a.Name+"=\t"+a.Description)
			}
		}
		return out, none | cobra.ShellCompDirectiveNoSpace
	}
}

func runInvokeStream(ctx context.Context, client adminpb.AdminAPIClient, req *adminpb.InvokeRequest, s *streamWriter) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	stream, err := client.InvokeStream(ctx, req)
	if err != nil {
		return adminErr(err)
	}
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return s.err()
		}
		if err != nil {
			if ctx.Err() != nil { // interrupted: a clean stop
				return s.err()
			}
			return adminErr(err)
		}
		if err := s.write(msg); err != nil {
			return err
		}
	}
}

// invocationSpec returns nil on any error, so that the call goes on without a
// confirmation and without a stream.
func invocationSpec(ctx context.Context, client adminpb.AdminAPIClient, svcName, invocation string) *adminpb.InvocationSpec {
	res, err := client.ListInvocations(ctx, &adminpb.ListInvocationsRequest{Service: svcName})
	if err != nil {
		return nil
	}
	for _, inv := range res.Invocations {
		if inv.Name == invocation {
			return inv
		}
	}
	return nil
}

func parseKeyVals(pairs []string) (map[string]string, error) {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("argument %q is not key=value", p)
		}
		out[k] = v
	}
	return out, nil
}
