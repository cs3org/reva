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
	"os"
	"os/signal"
	"slices"
	"strings"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/spf13/cobra"
)

func invocationCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "invocation", Short: "Show the invocations that the services expose"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list <service>",
			Short: "List the invocations of a service",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				client, ctx, err := rt.Admin(cmd.Context())
				if err != nil {
					return err
				}
				res, err := client.ListInvocations(ctx, &adminpb.ListInvocationsRequest{Service: args[0]})
				if err != nil {
					return adminErr(err)
				}
				if rt.Output() == "json" {
					return rt.Print(cmd.OutOrStdout(), res.Invocations, nil)
				}
				for _, inv := range res.Invocations {
					fmt.Printf("%-20s [%s] %s\n", inv.Name, invocationKind(inv), inv.Description)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "get <service> <invocation>",
			Short: "Show one invocation of a service, with its arguments",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				client, ctx, err := rt.Admin(cmd.Context())
				if err != nil {
					return err
				}
				res, err := client.ListInvocations(ctx, &adminpb.ListInvocationsRequest{Service: args[0]})
				if err != nil {
					return adminErr(err)
				}
				i := slices.IndexFunc(res.Invocations, func(inv *adminpb.InvocationSpec) bool { return inv.Name == args[1] })
				if i < 0 {
					return fmt.Errorf("service %s has no invocation %q", args[0], args[1])
				}
				inv := res.Invocations[i]
				return rt.Print(cmd.OutOrStdout(), inv, func(w io.Writer) {
					fmt.Fprintf(w, "%s [%s]\n%s\n", inv.Name, invocationKind(inv), inv.Description)
					for _, a := range inv.Args {
						req := ""
						if a.Required {
							req = " (required)"
						}
						fmt.Fprintf(w, "  %-16s %s%s\n", a.Name, a.Description, req)
					}
				})
			},
		},
	)
	return cmd
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
		Use:   "invoke <selector> <invocation> [key=val ...]",
		Short: "Run a service invocation on one or all instances",
		Long:  "Run a service invocation. The selector is a service name (every instance) or a node id (one instance).",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, posArgs []string) error {
			selector, invocation := posArgs[0], posArgs[1]
			args := parseKeyVals(posArgs[2:])

			client, ctx, err := rt.Admin(cmd.Context())
			if err != nil {
				return err
			}

			// Confirm dangerous invocations unless --yes. The kind comes from the
			// service's own catalog.
			spec := invocationSpec(ctx, client, selector, invocation)
			if spec.GetKind() == "dangerous" {
				if err := rt.Confirm(fmt.Sprintf("invocation %q on %q is DANGEROUS; proceed?", invocation, selector)); err != nil {
					return err
				}
			}

			if stream || spec.GetStreamOnly() {
				return runInvokeStream(ctx, client, selector, invocation, args)
			}

			res, err := client.Invoke(ctx, &adminpb.InvokeRequest{
				Service:    selector,
				Invocation: invocation,
				Args:       args,
			})
			if err != nil {
				return adminErr(err)
			}
			printNodeResults(res.Results)
			return nil
		},
	}
	cmd.Flags().BoolVar(&stream, "stream", false, "run a streaming invocation, printing results as they arrive (Ctrl-C stops); implied for a stream-only invocation")
	return cmd
}

// runInvokeStream drives a streaming invocation, printing node-labelled results
// as they arrive until every instance's stream ends or the user interrupts.
func runInvokeStream(ctx context.Context, client adminpb.AdminAPIClient, selector, invocation string, args map[string]string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	stream, err := client.InvokeStream(ctx, &adminpb.InvokeRequest{Service: selector, Invocation: invocation, Args: args})
	if err != nil {
		return adminErr(err)
	}
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			if ctx.Err() != nil { // interrupted: a clean stop
				return nil
			}
			return adminErr(err)
		}
		if msg.Error != "" {
			fmt.Printf("  %s: ERROR %s\n", msg.Node, msg.Error)
			continue
		}
		fmt.Printf("  %s: %s\n", msg.Node, msg.ResultJson)
	}
}

// invocationSpec best-effort resolves an invocation's catalog entry, for the
// confirmation gate and the choice of a stream; on any error it returns nil
// (no prompt, no stream).
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

// parseKeyVals turns ["k=v", "a=b"] into a map for invocation arguments.
func parseKeyVals(pairs []string) map[string]string {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		if k, v, ok := strings.Cut(p, "="); ok {
			out[k] = v
		}
	}
	return out
}
