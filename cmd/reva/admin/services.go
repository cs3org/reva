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
	"fmt"
	"io"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

func adminServicesCommand() *command {
	cmd := newCommand("services")
	cmd.Description = func() string {
		return "list the fleet's services (optionally one <service>; -v for every node)"
	}
	cmd.Usage = func() string { return "Usage: admin services [-v] [-o wide|json] [-state ...] [service]" }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	verbose := cmd.Bool("v", false, "show every node instead of a per-service summary")
	output := cmd.String("o", "", "output format: wide | json")
	stateFilter := cmd.String("state", "", "only show nodes in these states (comma-separated, e.g. degraded,offline)")
	cmd.ResetFlags = func() { *adminHost, *verbose, *output, *stateFilter = "", false, "", "" }
	cmd.Action = func(w ...io.Writer) error {
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

// validateOutput rejects an unknown -o value.
func validateOutput(o string) error {
	switch o {
	case "", "wide", "json":
		return nil
	default:
		return fmt.Errorf("unknown -o %q (use wide or json)", o)
	}
}
