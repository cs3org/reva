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
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/jedib0t/go-pretty/table"
	"github.com/jedib0t/go-pretty/text"
	"golang.org/x/term"
)

// nodeRow is one row of the services/nodes table.
type nodeRow struct {
	service, id, address, state, transport, pid, host, lastSeen, lastSeenRaw string
	meta                                                                     map[string]string
}

func nodeRowFrom(service string, n *adminpb.Node) nodeRow {
	m := n.Metadata
	return nodeRow{
		service:     service,
		id:          n.Id,
		address:     n.Address,
		state:       n.State,
		transport:   m["transport"],
		pid:         m["pid"],
		host:        m["host"],
		lastSeen:    humanizeSince(m["last_seen"]),
		lastSeenRaw: m["last_seen"],
		meta:        m,
	}
}

// frameworkMeta are the metadata keys already surfaced as their own columns;
// the META column shows the rest.
var frameworkMeta = map[string]bool{
	"transport": true, "host": true, "state": true, "last_seen": true, "pid": true,
}

// extraMeta renders a node's service-specific metadata as "k=v k=v", or "-".
func extraMeta(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if !frameworkMeta[k] {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return "-"
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, " ")
}

func sortNodeRows(rows []nodeRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].service != rows[j].service {
			return rows[i].service < rows[j].service
		}
		return rows[i].address < rows[j].address
	})
}

// renderNodeTable prints the nodes grouped by service; wide adds HOST and the
// full node id.
func renderNodeTable(rows []nodeRow, wide bool) {
	sortNodeRows(rows)

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	if wide {
		t.AppendHeader(table.Row{"SERVICE", "ADDRESS", "STATE", "TRANSPORT", "HOST", "LAST SEEN", "NODE ID", "META"})
	} else {
		t.AppendHeader(table.Row{"SERVICE", "ADDRESS", "STATE", "TRANSPORT", "LAST SEEN"})
	}

	colorize := stdoutIsTTY()
	for _, r := range rows {
		state := stateCell(r.state, colorize)
		if wide {
			t.AppendRow(table.Row{r.service, r.address, state, r.transport, r.host, r.lastSeen, r.id, extraMeta(r.meta)})
		} else {
			t.AppendRow(table.Row{r.service, r.address, state, r.transport, r.lastSeen})
		}
	}
	t.Render()
	printFleetSummary(rows)
}

// renderJSON emits the node list as JSON, for scripting.
func renderJSON(rows []nodeRow) error {
	sortNodeRows(rows)
	type out struct {
		Service   string            `json:"service"`
		ID        string            `json:"id"`
		Address   string            `json:"address"`
		State     string            `json:"state"`
		Transport string            `json:"transport"`
		PID       string            `json:"pid"`
		Host      string            `json:"host"`
		LastSeen  string            `json:"last_seen"`
		Metadata  map[string]string `json:"metadata"`
	}
	list := make([]out, 0, len(rows))
	for _, r := range rows {
		list = append(list, out{r.service, r.id, r.address, r.state, r.transport, r.pid, r.host, r.lastSeenRaw, r.meta})
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// renderServiceSummary prints one row per service with its node counts by
// state; wide adds the hosts.
func renderServiceSummary(rows []nodeRow, wide bool) {
	type agg struct {
		nodes, ready, degraded, offline, draining int
		hosts                                     map[string]bool
	}
	byName := map[string]*agg{}
	var names []string
	for _, r := range rows {
		a := byName[r.service]
		if a == nil {
			a = &agg{hosts: map[string]bool{}}
			byName[r.service] = a
			names = append(names, r.service)
		}
		a.nodes++
		if r.host != "" {
			a.hosts[r.host] = true
		}
		switch r.state {
		case "ready":
			a.ready++
		case "degraded":
			a.degraded++
		case "offline":
			a.offline++
		case "draining":
			a.draining++
		}
	}
	sort.Strings(names)

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	header := table.Row{"SERVICE", "NODES", "READY", "DEGRADED", "OFFLINE", "DRAINING"}
	if wide {
		header = append(header, "HOSTS")
	}
	t.AppendHeader(header)
	colorize := stdoutIsTTY()
	for _, name := range names {
		a := byName[name]
		row := table.Row{name, a.nodes, a.ready,
			countCell(a.degraded, stateColors["degraded"], colorize),
			countCell(a.offline, stateColors["offline"], colorize),
			countCell(a.draining, stateColors["draining"], colorize)}
		if wide {
			hosts := make([]string, 0, len(a.hosts))
			for h := range a.hosts {
				hosts = append(hosts, h)
			}
			sort.Strings(hosts)
			row = append(row, strings.Join(hosts, ","))
		}
		t.AppendRow(row)
	}
	t.Render()
	printFleetSummary(rows)
}

// stdoutIsTTY reports whether stdout is an interactive terminal, so coloring
// never reaches pipes or files.
func stdoutIsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// stateColors maps a non-ready state to its highlight color.
var stateColors = map[string]text.Colors{
	"degraded": {text.FgYellow},
	"offline":  {text.FgRed},
	"draining": {text.FgCyan},
}

// stateCell renders a node's state, highlighting non-ready ones.
func stateCell(state string, colorize bool) string {
	if state == "ready" {
		return state
	}
	label := strings.ToUpper(state)
	if colorize {
		if c, ok := stateColors[state]; ok {
			return c.Sprint(label)
		}
	}
	return label
}

// countCell renders a per-state count: 0 as "-", non-zero optionally colored.
func countCell(n int, c text.Colors, colorize bool) string {
	if n == 0 {
		return "-"
	}
	s := fmt.Sprintf("%d", n)
	if colorize {
		return c.Sprint(s)
	}
	return s
}

// filterByState keeps rows whose state is in the comma-separated filter.
func filterByState(rows []nodeRow, filter string) []nodeRow {
	if strings.TrimSpace(filter) == "" {
		return rows
	}
	want := map[string]bool{}
	for s := range strings.SplitSeq(filter, ",") {
		if s = strings.TrimSpace(s); s != "" {
			want[s] = true
		}
	}
	out := make([]nodeRow, 0, len(rows))
	for _, r := range rows {
		if want[r.state] {
			out = append(out, r)
		}
	}
	return out
}

// printFleetSummary prints the "N services · M nodes · ready:X …" footer.
func printFleetSummary(rows []nodeRow) {
	states := map[string]int{}
	seen := map[string]bool{}
	svcCount := 0
	for _, r := range rows {
		states[r.state]++
		if !seen[r.service] {
			seen[r.service] = true
			svcCount++
		}
	}
	parts := []string{fmt.Sprintf("%d services", svcCount), fmt.Sprintf("%d nodes", len(rows))}
	for _, st := range []string{"ready", "degraded", "offline", "draining"} {
		if states[st] > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", st, states[st]))
			delete(states, st)
		}
	}
	for st, n := range states { // any unexpected states
		parts = append(parts, fmt.Sprintf("%s:%d", st, n))
	}
	fmt.Println(strings.Join(parts, " · "))
}

// humanizeSince renders an RFC3339 timestamp as a compact age (e.g. "3s ago").
func humanizeSince(ts string) string {
	if ts == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	d := max(time.Since(t), 0)
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%ds ago", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%dm ago", int(d.Hours()), int(d.Minutes())%60)
	}
}

// printNodeResults renders the per-node results of a fan-out operation.
func printNodeResults(results []*adminpb.NodeResult) {
	for _, r := range results {
		if r.Error != "" {
			fmt.Printf("  %s: ERROR %s\n", r.Node, r.Error)
			continue
		}
		if r.ResultJson != "" {
			fmt.Printf("  %s: %s\n", r.Node, r.ResultJson)
		} else {
			fmt.Printf("  %s: ok\n", r.Node)
		}
	}
}

// ----- subcommands -----
