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
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

func adminInvocationsCommand() *command {
	cmd := newCommand("invocations")
	cmd.Description = func() string { return "list the invocations a service exposes" }
	cmd.Usage = func() string { return "Usage: admin invocations [-admin-host h] <service>" }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	cmd.ResetFlags = func() { *adminHost = "" }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New(cmd.Usage())
		}
		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		res, err := client.ListInvocations(ctx, &adminpb.ListInvocationsRequest{Service: cmd.Args()[0]})
		if err != nil {
			return adminErr(err)
		}
		for _, inv := range res.Invocations {
			kind := inv.Kind
			if kind == "" {
				kind = "?"
			}
			if inv.Streaming {
				kind += ",stream"
			}
			fmt.Printf("%-20s [%s] %s\n", inv.Name, kind, inv.Description)
		}
		return nil
	}
	return cmd
}

func adminInvokeCommand() *command {
	cmd := newCommand("invoke")
	cmd.Description = func() string { return "run a service invocation on one or all instances" }
	cmd.Usage = func() string {
		return "Usage: admin invoke [-admin-host h] [-y] [-stream] <selector> <invocation> [key=val ...]"
	}
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	yes := cmd.Bool("y", false, "skip the confirmation prompt for dangerous invocations")
	stream := cmd.Bool("stream", false, "run a streaming invocation, printing results as they arrive (Ctrl-C stops)")
	cmd.ResetFlags = func() { *adminHost, *yes, *stream = "", false, false }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 2 {
			return errors.New(cmd.Usage())
		}
		// selector is a service name (every instance) or a node id (one instance).
		selector, invocation := cmd.Args()[0], cmd.Args()[1]
		args := parseKeyVals(cmd.Args()[2:])

		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}

		// Confirm dangerous invocations unless -y. The kind comes from the
		// service's own catalog.
		if !*yes && invocationKind(ctx, client, selector, invocation) == "dangerous" {
			if !confirm(fmt.Sprintf("invocation %q on %q is DANGEROUS; proceed?", invocation, selector)) {
				return errors.New("aborted")
			}
		}

		if *stream {
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
	}
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

// invocationKind best-effort resolves an invocation's kind for the confirmation
// gate; on any error it returns "" (no prompt).
func invocationKind(ctx context.Context, client adminpb.AdminAPIClient, svcName, invocation string) string {
	res, err := client.ListInvocations(ctx, &adminpb.ListInvocationsRequest{Service: svcName})
	if err != nil {
		return ""
	}
	for _, inv := range res.Invocations {
		if inv.Name == invocation {
			return inv.Kind
		}
	}
	return ""
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
