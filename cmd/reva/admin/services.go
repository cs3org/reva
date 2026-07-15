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
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

func adminServicesCommand() *command {
	cmd := newCommand("services")
	cmd.Description = func() string {
		return "list services, or drain/enable instances (optionally one <service>; -v for every node)"
	}
	cmd.Usage = func() string {
		return "Usage: admin services [-v] [-o wide|json] [-state ...] [service]\n" +
			"       admin services drain|enable [-admin-host h] [-y] <selector>"
	}
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	verbose := cmd.Bool("v", false, "show every node instead of a per-service summary")
	output := cmd.String("o", "", "output format: wide | json")
	stateFilter := cmd.String("state", "", "only show nodes in these states (comma-separated, e.g. degraded,offline)")
	yes := cmd.Bool("y", false, "skip the confirmation prompt when draining")
	cmd.ResetFlags = func() { *adminHost, *verbose, *output, *stateFilter, *yes = "", false, "", "", false }
	cmd.Action = func(w ...io.Writer) error {
		// `services drain|enable <selector>` takes instances out of / into
		// rotation; no reva service is named "drain" or "enable".
		if cmd.NArg() > 0 {
			switch cmd.Args()[0] {
			case "drain":
				return adminServicesRotation(*adminHost, cmd.Args()[1:], true, *yes)
			case "enable":
				return adminServicesRotation(*adminHost, cmd.Args()[1:], false, *yes)
			}
		}
		if err := validateOutput(*output); err != nil {
			return err
		}
		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		var svcFilter string
		if cmd.NArg() > 0 {
			svcFilter = cmd.Args()[0]
		}
		res, err := client.ListServices(ctx, &adminpb.ListServicesRequest{Service: svcFilter})
		if err != nil {
			return adminErr(err)
		}
		var rows []nodeRow
		for _, sv := range res.Services {
			for _, n := range sv.Nodes {
				rows = append(rows, nodeRowFrom(sv.Name, n))
			}
		}
		rows = filterByState(rows, *stateFilter)
		switch {
		case *output == "json":
			return renderJSON(rows)
		case *verbose:
			renderNodeTable(rows, *output == "wide")
		default:
			renderServiceSummary(rows, *output == "wide")
		}
		return nil
	}
	return cmd
}

// adminServicesRotation drains (out of rotation) or enables (back into
// rotation) the selected instances by running the `rotation` invocation. Drain
// confirms first unless yes, since it stops new traffic to the matched nodes.
func adminServicesRotation(adminHost string, args []string, drain, yes bool) error {
	if len(args) < 1 {
		return errors.New("Usage: admin services drain|enable [-admin-host h] [-y] <selector>")
	}
	selector := args[0]
	state := "ready"
	if drain {
		state = "drain"
		if !yes && !confirm(fmt.Sprintf("take %q out of rotation? new traffic to it will stop", selector)) {
			return errors.New("aborted")
		}
	}
	client, ctx, err := adminDial(adminHost)
	if err != nil {
		return err
	}
	res, err := client.Invoke(ctx, &adminpb.InvokeRequest{
		Service:    selector,
		Invocation: "rotation",
		Args:       map[string]string{"state": state},
	})
	if err != nil {
		return adminErr(err)
	}
	for _, r := range res.Results {
		if r.Error != "" {
			fmt.Printf("  %s: error: %s\n", r.Node, r.Error)
			continue
		}
		var d struct{ Node, Previous, State string }
		if err := json.Unmarshal([]byte(r.ResultJson), &d); err != nil {
			fmt.Printf("  %s: %s\n", r.Node, r.ResultJson)
			continue
		}
		if d.Previous != d.State {
			fmt.Printf("  %s: %s -> %s\n", r.Node, d.Previous, d.State)
		} else {
			fmt.Printf("  %s: %s\n", r.Node, d.State)
		}
	}
	return nil
}

// validateOutput rejects an unknown -o value.
func validateOutput(o string) error {
	switch o {
	case "", "wide", "json":
		return nil
	default:
		return fmt.Errorf("unknown -o %q (use wide or json)", o)
	}
}
