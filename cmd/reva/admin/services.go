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
	"sort"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

// servicesHelp is the `admin services -h` guide: the subcommands, then the flags
// grouped by the subcommand each applies to. Flags must precede the subcommand
// token and the selector (Go's flag parser stops at the first positional).
const servicesHelp = `admin services — inspect and operate the fleet's services

Subcommands (a bare "services" lists them):
  services [service]        list services with their nodes, state and health
  services drain  <sel>     take matching instances out of rotation (no new traffic)
  services enable <sel>     return matching instances to rotation
  services activity <sel>   report in-flight / idle request activity

  <sel> is a service name, a node id (host:port/svc), a host:port, a host, or *.

Flags (place them before the subcommand and selector):
  -admin-host <addr>   admin gRPC endpoint, persisted   (all subcommands)

  list:      -v                 show every node, not a per-service summary
             -o wide|json       output format
             -state <s,...>      only nodes in these states (e.g. degraded,offline)

  drain:     -y                 skip the confirmation prompt

  activity:  -methods           break the counts down per RPC method
             -wait              block until every matched instance is quiescent
             -idle <dur>        with -wait: required idle time on top of 0 in-flight (default 5s)
             -timeout <dur>     with -wait: give up after this long (default 2m)

Examples:
  admin services -v userprovider
  admin services -y drain host1:9158
  admin services -methods activity gateway
  admin services -wait -idle 5s activity host1:9158    # then it is safe to restart
`

func adminServicesCommand() *command {
	cmd := newCommand("services")
	cmd.Description = func() string {
		return "list services, drain/enable instances, or report request activity (one <service>; -v for every node)"
	}
	cmd.Usage = func() string {
		return "Usage: admin services [-v] [-o wide|json] [-state ...] [service]\n" +
			"       admin services drain|enable [-admin-host h] [-y] <selector>\n" +
			"       admin services [-methods] [-wait [-timeout D] [-idle D]] activity <selector>"
	}
	// `services` multiplexes several subcommands over one flag set, so `-h`
	// needs a hand-written guide grouping the flags under the subcommand each
	// belongs to (Go's default flag dump can't).
	cmd.FlagSet.Usage = func() { fmt.Fprint(cmd.Output(), servicesHelp) }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	verbose := cmd.Bool("v", false, "show every node instead of a per-service summary")
	output := cmd.String("o", "", "output format: wide | json")
	stateFilter := cmd.String("state", "", "only show nodes in these states (comma-separated, e.g. degraded,offline)")
	yes := cmd.Bool("y", false, "skip the confirmation prompt when draining")
	wait := cmd.Bool("wait", false, "activity: block until every matched instance is quiescent")
	timeout := cmd.Duration("timeout", 2*time.Minute, "activity -wait: give up after this long")
	idle := cmd.Duration("idle", 5*time.Second, "activity -wait: require this much idle time on top of zero in-flight")
	methods := cmd.Bool("methods", false, "activity: break the counts down per RPC method")
	cmd.ResetFlags = func() {
		*adminHost, *verbose, *output, *stateFilter, *yes = "", false, "", "", false
		*wait, *timeout, *idle, *methods = false, 2*time.Minute, 5*time.Second, false
	}
	cmd.Action = func(w ...io.Writer) error {
		// `services drain|enable|activity <selector>` are subcommands; no reva
		// service is named "drain", "enable" or "activity".
		if cmd.NArg() > 0 {
			switch cmd.Args()[0] {
			case "drain":
				return adminServicesRotation(*adminHost, cmd.Args()[1:], true, *yes)
			case "enable":
				return adminServicesRotation(*adminHost, cmd.Args()[1:], false, *yes)
			case "activity":
				return adminServicesActivity(*adminHost, cmd.Args()[1:], *wait, *timeout, *idle, *methods)
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

// activityStat mirrors the `activity` invocation's per-instance result.
type activityStat struct {
	InFlight    int64            `json:"in_flight"`
	Total       int64            `json:"total"`
	IdleSeconds float64          `json:"idle_seconds"` // <0 means no request ever served
	LastRequest string           `json:"last_request"`
	Methods     []activityMethod `json:"methods"`
}

// activityMethod is one RPC method's slice of an instance's activity.
type activityMethod struct {
	Method      string  `json:"method"`
	InFlight    int64   `json:"in_flight"`
	Total       int64   `json:"total"`
	IdleSeconds float64 `json:"idle_seconds"`
}

// adminServicesActivity reports each matched instance's request activity. With
// -wait it polls until every instance is quiescent — zero in-flight and idle for
// at least idle — or timeout elapses, so a drain can be followed by a scripted
// "wait until safe to restart". With methods it also prints the per-RPC-method
// breakdown.
func adminServicesActivity(adminHost string, args []string, wait bool, timeout, idle time.Duration, methods bool) error {
	if len(args) < 1 {
		return errors.New("Usage: admin services activity [-admin-host h] [-methods] [-wait [-timeout D] [-idle D]] <selector>")
	}
	selector := args[0]
	client, ctx, err := adminDial(adminHost)
	if err != nil {
		return err
	}
	poll := func() ([]*adminpb.NodeResult, error) {
		res, err := client.Invoke(ctx, &adminpb.InvokeRequest{Service: selector, Invocation: "activity"})
		if err != nil {
			return nil, adminErr(err)
		}
		return res.Results, nil
	}

	if !wait {
		results, err := poll()
		if err != nil {
			return err
		}
		printActivity(results, methods)
		return nil
	}

	deadline := time.Now().Add(timeout)
	for {
		results, err := poll()
		if err != nil {
			return err
		}
		if busy := busyNodes(results, idle); len(busy) == 0 {
			printActivity(results, methods)
			fmt.Println("all matched instances are quiescent")
			return nil
		} else if time.Now().After(deadline) {
			printActivity(results, methods)
			return fmt.Errorf("timed out after %s; still active: %s", timeout, strings.Join(busy, ", "))
		}
		time.Sleep(time.Second)
	}
}

// busyNodes returns the nodes that are not yet quiescent: still serving a
// request, not idle long enough, or unreachable (an error we cannot clear).
func busyNodes(results []*adminpb.NodeResult, idle time.Duration) []string {
	var busy []string
	for _, r := range results {
		if r.Error != "" {
			busy = append(busy, r.Node)
			continue
		}
		var s activityStat
		if err := json.Unmarshal([]byte(r.ResultJson), &s); err != nil {
			busy = append(busy, r.Node)
			continue
		}
		quiescent := s.InFlight == 0 && (s.IdleSeconds < 0 || s.IdleSeconds >= idle.Seconds())
		if !quiescent {
			busy = append(busy, r.Node)
		}
	}
	return busy
}

// printActivity renders one row per instance: in-flight, total served, idle.
// With methods, each instance's per-RPC-method breakdown follows, busiest first.
func printActivity(results []*adminpb.NodeResult, methods bool) {
	for _, r := range results {
		if r.Error != "" {
			fmt.Printf("  %s: error: %s\n", r.Node, r.Error)
			continue
		}
		var s activityStat
		if err := json.Unmarshal([]byte(r.ResultJson), &s); err != nil {
			fmt.Printf("  %s: %s\n", r.Node, r.ResultJson)
			continue
		}
		fmt.Printf("  %s: in-flight=%d total=%d idle=%s\n", r.Node, s.InFlight, s.Total, idleLabel(s.IdleSeconds))
		if !methods {
			continue
		}
		sort.Slice(s.Methods, func(i, j int) bool {
			if s.Methods[i].Total != s.Methods[j].Total {
				return s.Methods[i].Total > s.Methods[j].Total
			}
			return s.Methods[i].Method < s.Methods[j].Method
		})
		for _, m := range s.Methods {
			fmt.Printf("      %-28s in-flight=%d total=%d idle=%s\n", m.Method, m.InFlight, m.Total, idleLabel(m.IdleSeconds))
		}
	}
}

// idleLabel renders idle seconds as a short duration, or "-" when the instance
// has served no request at all.
func idleLabel(sec float64) string {
	if sec < 0 {
		return "-"
	}
	return time.Duration(sec * float64(time.Second)).Round(time.Second).String()
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
