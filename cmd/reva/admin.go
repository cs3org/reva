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
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/jedib0t/go-pretty/table"
	"github.com/jedib0t/go-pretty/text"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	ins "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// adminCommands is the sub-table dispatched by the single `admin` entry in the
// top-level commands table.
var adminCommands = []*command{
	adminElevateCommand(),
	adminServicesCommand(),
	adminConfigCommand(),
	adminInvocationsCommand(),
	adminInvokeCommand(),
	adminLogsCommand(),
	adminTraceCommand(),
	adminStackCommand(),
	adminImpersonateCommand(),
}

// adminCommand routes its first positional argument through the sub-table,
// handing the rest to the subcommand.
func adminCommand() *command {
	cmd := newCommand("admin")
	cmd.Description = func() string { return "administer the reva fleet (run `admin` for subcommands)" }
	cmd.Action = func(w ...io.Writer) error {
		args := cmd.Args()
		if len(args) == 0 {
			printAdminUsage()
			return nil
		}
		name := args[0]
		for _, sub := range adminCommands {
			if sub.Name == name {
				if err := sub.Parse(args[1:]); err != nil {
					return err
				}
				defer sub.ResetFlags()
				return sub.Action(w...)
			}
		}
		return fmt.Errorf("unknown admin subcommand %q; run `admin` to list them", name)
	}
	return cmd
}

func printAdminUsage() {
	fmt.Println("Usage: admin <subcommand> [flags] [args]")
	fmt.Println("Subcommands:")
	n := 0
	for _, sub := range adminCommands {
		if len(sub.Name) > n {
			n = len(sub.Name)
		}
	}
	for _, sub := range adminCommands {
		fmt.Printf("  %s%s%s\n", sub.Name, strings.Repeat(" ", 2+(n-len(sub.Name))), sub.Description())
	}
}

// ----- shared helpers -----

// resolveAdminHost returns the admin endpoint, persisting a -admin-host flag to
// the config when given so it need not be repeated.
func resolveAdminHost(flagVal string) (string, error) {
	if flagVal != "" {
		persistAdminHost(flagVal)
		return flagVal, nil
	}
	if conf != nil && conf.AdminHost != "" {
		return conf.AdminHost, nil
	}
	return "", errors.New("admin host not set: pass -admin-host <host:port> once (it is persisted for next time)")
}

func persistAdminHost(hostVal string) {
	c, err := readConfig()
	if err != nil || c == nil {
		c = &config{}
		if conf != nil {
			c.Host = conf.Host
		}
	}
	c.AdminHost = hostVal
	_ = writeConfig(c)
	if conf != nil {
		conf.AdminHost = hostVal
	} else {
		conf = c
	}
}

// isSocketHost reports whether the admin host is a local Unix socket
// ("unix:///..."), dialed without TLS and without a token.
func isSocketHost(host string) bool { return strings.HasPrefix(host, "unix:") }

// adminMaxRecvMsgSize lifts the 4 MiB default so a fleet-wide unary fan-out
// (many instances' results in one response) has headroom. Large per-instance
// results (e.g. stack dumps) should stream instead — see `admin stack`.
const adminMaxRecvMsgSize = 64 << 20

func adminConn(host string) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(adminMaxRecvMsgSize))}
	if insecure || isSocketHost(host) {
		opts = append(opts, grpc.WithTransportCredentials(ins.NewCredentials()))
	} else {
		tlsconf := &tls.Config{InsecureSkipVerify: skipverify}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsconf)))
	}
	return grpc.NewClient(host, opts...)
}

func adminClientAt(host string) (adminpb.AdminAPIClient, error) {
	conn, err := adminConn(host)
	if err != nil {
		return nil, err
	}
	return adminpb.NewAdminAPIClient(conn), nil
}

// adminAuthContext attaches the stored short-TTL admin token. Every subcommand
// except `elevate` uses it.
func adminAuthContext() (context.Context, error) {
	t, err := readAdminToken()
	if err != nil || t == "" {
		return nil, errors.New("no admin token found: run `admin elevate` first")
	}
	ctx := appctx.ContextSetToken(context.Background(), t)
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, t)
	return ctx, nil
}

// adminDial builds a client and its auth context. With no -admin-host it
// prefers a working local-root socket, then falls back to the stored network
// host + admin token. An explicit -admin-host always wins.
func adminDial(adminHostFlag string) (adminpb.AdminAPIClient, context.Context, error) {
	if adminHostFlag != "" {
		host, err := resolveAdminHost(adminHostFlag)
		if err != nil {
			return nil, nil, err
		}
		return adminDialHost(host)
	}

	// No flag: try the local-root socket(s) first.
	for _, p := range defaultSocketPaths() {
		if !isSocketFile(p) {
			continue
		}
		client, err := adminClientAt("unix://" + p)
		if err != nil {
			continue
		}
		if adminSocketWorks(client) {
			return client, context.Background(), nil
		}
	}

	// Fall back to the stored network admin host + token.
	host, err := resolveAdminHost("")
	if err != nil {
		return nil, nil, err
	}
	return adminDialHost(host)
}

// adminDialHost dials one resolved host: a socket needs no token, a network
// host uses the stored one.
func adminDialHost(host string) (adminpb.AdminAPIClient, context.Context, error) {
	client, err := adminClientAt(host)
	if err != nil {
		return nil, nil, err
	}
	if isSocketHost(host) {
		return client, context.Background(), nil
	}
	ctx, err := adminAuthContext()
	if err != nil {
		return nil, nil, err
	}
	return client, ctx, nil
}

// defaultSocketPaths mirrors the server's well-known socket locations (the CLI
// cannot import the service package).
func defaultSocketPaths() []string {
	paths := []string{"/run/reva/admin.sock"}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "reva", "admin.sock"))
	}
	return paths
}

func isSocketFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode()&os.ModeSocket != 0
}

// adminSocketWorks probes the socket with a cheap call; a denied or stale
// socket returns false so the caller falls back.
func adminSocketWorks(client adminpb.AdminAPIClient) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.GetServerInfo(ctx, &adminpb.GetServerInfoRequest{})
	return err == nil
}

// adminErr annotates authn/authz failures with the sudo-timeout hint.
func adminErr(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.Unauthenticated, codes.PermissionDenied:
		return fmt.Errorf("%w (admin token may be expired or missing; re-run `admin elevate`)", err)
	default:
		return err
	}
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

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

func adminElevateCommand() *command {
	cmd := newCommand("elevate")
	cmd.Description = func() string { return "step up: exchange the login token for a short-TTL admin token" }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	cmd.ResetFlags = func() { *adminHost = "" }
	cmd.Action = func(w ...io.Writer) error {
		host, err := resolveAdminHost(*adminHost)
		if err != nil {
			return err
		}
		client, err := adminClientAt(host)
		if err != nil {
			return err
		}
		userTok, err := readToken()
		if err != nil {
			return errors.New("no login token: run `login` first")
		}
		ctx := appctx.ContextSetToken(context.Background(), userTok)
		ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, userTok)

		res, err := client.RequestAdmin(ctx, &adminpb.RequestAdminRequest{})
		if err != nil {
			return err
		}
		writeAdminToken(res.Token)
		fmt.Printf("elevated; admin token valid until %s\n", time.Unix(res.ExpiresAt, 0).Format(time.RFC3339))
		return nil
	}
	return cmd
}

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

func adminConfigCommand() *command {
	cmd := newCommand("config")
	cmd.Description = func() string { return "show a service's effective (redacted) config (TOML by default)" }
	cmd.Usage = func() string { return "Usage: admin config [-admin-host h] [-o toml|json] <service|node-id>" }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	output := cmd.String("o", "toml", "output format: toml | json")
	cmd.ResetFlags = func() { *adminHost, *output = "", "toml" }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New(cmd.Usage())
		}
		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		res, err := client.GetServiceConfig(ctx, &adminpb.GetServiceConfigRequest{Service: cmd.Args()[0]})
		if err != nil {
			return adminErr(err)
		}
		if len(res.Results) == 0 {
			fmt.Println("(no instances)")
			return nil
		}
		// Header each instance only when there is more than one.
		for i, r := range res.Results {
			if len(res.Results) > 1 {
				if i > 0 {
					fmt.Println()
				}
				fmt.Printf("# %s\n", r.Node)
			}
			if r.Error != "" {
				fmt.Printf("# error: %s\n", r.Error)
				continue
			}
			if err := renderConfig(r.ResultJson, *output); err != nil {
				return err
			}
		}
		return nil
	}
	return cmd
}

// renderConfig prints one instance's config JSON as TOML (default) or indented
// JSON.
func renderConfig(configJSON, output string) error {
	switch output {
	case "json":
		var buf bytes.Buffer
		if json.Indent(&buf, []byte(configJSON), "", "  ") == nil {
			fmt.Println(buf.String())
		} else {
			fmt.Println(configJSON)
		}
	case "toml", "":
		out, err := configToTOML(configJSON)
		if err != nil {
			return err
		}
		fmt.Print(out)
	default:
		return fmt.Errorf("unknown -o %q (use toml or json)", output)
	}
	return nil
}

// configToTOML renders a JSON config object as TOML — the format reva admins
// read — keeping integers as integers (not 15.0).
func configToTOML(jsonStr string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(jsonStr))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return "", fmt.Errorf("decoding config: %w", err)
	}
	normalizeNumbers(m)
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(m); err != nil {
		return "", fmt.Errorf("encoding toml: %w", err)
	}
	return buf.String(), nil
}

// normalizeNumbers turns json.Number into int64 (when integral) or float64 so
// TOML shows 15 rather than 15.0; it mutates maps/slices in place.
func normalizeNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = normalizeNumbers(val)
		}
	case []any:
		for i, e := range t {
			t[i] = normalizeNumbers(e)
		}
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	}
	return v
}

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

func adminLogsCommand() *command {
	cmd := newCommand("logs")
	cmd.Description = func() string { return "read (or follow) a service's recent logs across the fleet" }
	cmd.Usage = func() string {
		return "Usage: admin logs [-admin-host h] [-f] [-n N] [-level L] [-since D] [-grep P] [-o text|json] <selector>\n" +
			"       admin logs level [-admin-host h] <selector> [trace|debug|info|warn|error]"
	}
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	follow := cmd.Bool("f", false, "follow: stream new lines until interrupted")
	tail := cmd.Int("n", 200, "number of recent lines (snapshot), or backlog before -f")
	level := cmd.String("level", "", "minimum level: trace|debug|info|warn|error")
	since := cmd.String("since", "", "only lines newer than a duration (e.g. 5m) or an RFC3339 time")
	grep := cmd.String("grep", "", "keep only lines containing this substring")
	output := cmd.String("o", "text", "output format: text | json")
	cmd.ResetFlags = func() {
		*adminHost, *follow, *tail, *level, *since, *grep, *output = "", false, 200, "", "", "", "text"
	}
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New(cmd.Usage())
		}
		// `logs level <selector> [newlevel]` reports or sets the runtime log level.
		if cmd.Args()[0] == "level" {
			return adminLogsLevel(*adminHost, cmd.Args()[1:])
		}
		if *output != "text" && *output != "json" {
			return fmt.Errorf("unknown -o %q (use text or json)", *output)
		}
		selector := cmd.Args()[0]
		args := map[string]string{"limit": strconv.Itoa(*tail)}
		if *level != "" {
			args["level"] = *level
		}
		if *since != "" {
			args["since"] = *since
		}
		if *grep != "" {
			args["grep"] = *grep
		}

		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		// A node id addresses one instance; a plain service name may span several,
		// so label lines with their node in that case.
		multi := !strings.Contains(selector, "/")
		if *follow {
			return followLogs(ctx, client, selector, args, *output, multi)
		}
		return snapshotLogs(ctx, client, selector, args, *output, multi)
	}
	return cmd
}

// adminLogsLevel reports or sets the runtime log level of the selected
// instances: with no level it prints each instance's current one, with a level
// it changes them (in-memory; a restart reverts to the configured level).
func adminLogsLevel(adminHost string, args []string) error {
	if len(args) < 1 {
		return errors.New("Usage: admin logs level [-admin-host h] <selector> [trace|debug|info|warn|error]")
	}
	invArgs := map[string]string{}
	if len(args) >= 2 {
		invArgs["level"] = args[1]
	}
	client, ctx, err := adminDial(adminHost)
	if err != nil {
		return err
	}
	res, err := client.Invoke(ctx, &adminpb.InvokeRequest{Service: args[0], Invocation: "loglevel", Args: invArgs})
	if err != nil {
		return adminErr(err)
	}
	for _, r := range res.Results {
		if r.Error != "" {
			fmt.Printf("  %s: error: %s\n", r.Node, r.Error)
			continue
		}
		var d struct{ Previous, Level string }
		if err := json.Unmarshal([]byte(r.ResultJson), &d); err != nil {
			fmt.Printf("  %s: %s\n", r.Node, r.ResultJson)
			continue
		}
		if invArgs["level"] != "" && d.Previous != d.Level {
			fmt.Printf("  %s: %s -> %s\n", r.Node, d.Previous, d.Level)
		} else {
			fmt.Printf("  %s: %s\n", r.Node, d.Level)
		}
	}
	return nil
}

// logLine is one streamed/returned log entry from the `logs` invocation.
type logLine struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Service string `json:"service"`
	Message string `json:"message"`
	Raw     string `json:"raw"`
}

// logSnapshot is the unary `logs` result: recent lines plus a truncation flag.
type logSnapshot struct {
	Entries   []logLine `json:"entries"`
	Truncated bool      `json:"truncated"`
}

// snapshotLogs runs the unary logs invocation and prints the merged lines in
// chronological order.
func snapshotLogs(ctx context.Context, client adminpb.AdminAPIClient, selector string, args map[string]string, output string, multi bool) error {
	res, err := client.Invoke(ctx, &adminpb.InvokeRequest{Service: selector, Invocation: "logs", Args: args})
	if err != nil {
		return adminErr(err)
	}
	type entry struct {
		node string
		line logLine
		t    time.Time
	}
	var merged []entry
	anyTruncated := false
	for _, r := range res.Results {
		if r.Error != "" {
			fmt.Printf("# %s: error: %s\n", r.Node, r.Error)
			continue
		}
		var snap logSnapshot
		if err := json.Unmarshal([]byte(r.ResultJson), &snap); err != nil {
			continue
		}
		anyTruncated = anyTruncated || snap.Truncated
		for _, l := range snap.Entries {
			merged = append(merged, entry{node: r.Node, line: l, t: parseLogTime(l.Time)})
		}
	}
	// The buffer returns newest-first; sort ascending so it reads like a log file.
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].t.Before(merged[j].t) })
	nodeWidth := 0
	for _, e := range merged {
		nodeWidth = max(nodeWidth, len(e.node))
	}
	color := stdoutIsTTY()
	for _, e := range merged {
		fmt.Println(formatLogLine(e.node, nodeWidth, e.line, multi, color, output))
	}
	if anyTruncated && output == "text" {
		fmt.Printf("# more lines matched than shown; raise -n to see more\n")
	}
	return nil
}

// followLogs streams the logs invocation until interrupted.
func followLogs(ctx context.Context, client adminpb.AdminAPIClient, selector string, args map[string]string, output string, multi bool) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	stream, err := client.InvokeStream(ctx, &adminpb.InvokeRequest{Service: selector, Invocation: "logs", Args: args})
	if err != nil {
		return adminErr(err)
	}
	color := stdoutIsTTY()
	nodeWidth := 0
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
			fmt.Printf("# %s: error: %s\n", msg.Node, msg.Error)
			continue
		}
		var l logLine
		if err := json.Unmarshal([]byte(msg.ResultJson), &l); err != nil {
			continue
		}
		nodeWidth = max(nodeWidth, len(msg.Node))
		fmt.Println(formatLogLine(msg.Node, nodeWidth, l, multi, color, output))
	}
}

// formatLogLine renders one entry: the raw JSON for -o json, else a console
// line in the familiar zerolog format, node-prefixed (padded to nodeWidth so
// the columns align) when the result spans instances and colored with
// zerolog's palette when stdout is a TTY.
func formatLogLine(node string, nodeWidth int, l logLine, multi, color bool, output string) string {
	if output == "json" {
		if l.Raw != "" {
			return l.Raw
		}
		b, _ := json.Marshal(l)
		return string(b)
	}
	ts := l.Time
	if t := parseLogTime(l.Time); !t.IsZero() {
		ts = t.Format("2006-01-02 15:04:05.000")
	}
	lvl := strings.ToUpper(l.Level)
	if len(lvl) > 3 {
		lvl = lvl[:3]
	}
	caller, fields := logLineDetails(l.Raw)

	var b strings.Builder
	if multi {
		fmt.Fprintf(&b, "%-*s  ", nodeWidth, node)
	}
	b.WriteString(logColor(ts, "90", color)) // dark gray, like zerolog's console time
	b.WriteString(" " + logColor(fmt.Sprintf("%-3s", lvl), levelColor(l.Level), color) + " ")
	if caller != "" {
		b.WriteString(logColor(caller, "1", color) + logColor(" >", "36", color) + " ")
	}
	if l.Message != "" || len(fields) > 0 || caller != "" {
		b.WriteString(l.Message)
	} else {
		// A line the buffer could not parse: show it verbatim over losing it.
		b.WriteString(l.Raw)
	}
	for _, f := range fields {
		if f[0] == "err" || f[0] == "error" {
			b.WriteString(" " + logColor(f[0]+"=", "31", color) + logColor(f[1], "1;31", color))
		} else {
			b.WriteString(" " + logColor(f[0]+"=", "36", color) + f[1])
		}
	}
	return b.String()
}

// logColor wraps s in an ANSI SGR sequence when enabled.
func logColor(s, code string, enabled bool) string {
	if !enabled || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// levelColor is zerolog's console level palette.
func levelColor(level string) string {
	switch strings.ToLower(level) {
	case "trace":
		return "35" // magenta
	case "debug":
		return "33" // yellow
	case "info":
		return "32" // green
	case "warn":
		return "31" // red
	case "error", "fatal", "panic":
		return "1;31" // bold red
	default:
		return "1" // bold
	}
}

// logLineDetails parses a raw zerolog JSON line into its caller and [key,
// value] fields in writer order; time/level/message are rendered separately.
func logLineDetails(raw string) (caller string, fields [][2]string) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	t, err := dec.Token()
	if err != nil || t != json.Delim('{') {
		return "", nil
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return caller, fields
		}
		key, ok := kt.(string)
		if !ok {
			return caller, fields
		}
		var v any
		if err := dec.Decode(&v); err != nil {
			return caller, fields
		}
		switch key {
		case "time", "level", "message":
		case "caller":
			caller = logFieldValue(v)
		default:
			fields = append(fields, [2]string{key, logFieldValue(v)})
		}
	}
	return caller, fields
}

// logFieldValue renders one field value console-style: strings bare (quoted
// when they contain whitespace), nested values as compact JSON.
func logFieldValue(v any) string {
	switch t := v.(type) {
	case string:
		if strings.ContainsAny(t, " \t\n") {
			return strconv.Quote(t)
		}
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func parseLogTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func adminStackCommand() *command {
	cmd := newCommand("stack")
	cmd.Description = func() string { return "dump the goroutine stacks of the selected instances" }
	cmd.Usage = func() string { return "Usage: admin stack [-admin-host h] [-grep P] <selector>" }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	grep := cmd.String("grep", "", "keep only goroutines whose stack contains this substring")
	cmd.ResetFlags = func() { *adminHost, *grep = "", "" }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New(cmd.Usage())
		}
		args := map[string]string{}
		if *grep != "" {
			args["grep"] = *grep
		}
		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		// Stream per instance: a dump is large, so one aggregate response could
		// exceed the message limit on a wide fan-out.
		stream, err := client.InvokeStream(ctx, &adminpb.InvokeRequest{Service: cmd.Args()[0], Invocation: "stack", Args: args})
		if err != nil {
			return adminErr(err)
		}
		first := true
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return adminErr(err)
			}
			if !first {
				fmt.Println()
			}
			first = false
			if msg.Error != "" {
				fmt.Printf("# %s: error: %s\n", msg.Node, msg.Error)
				continue
			}
			var d struct {
				Goroutines int    `json:"goroutines"`
				HeapAlloc  uint64 `json:"heap_alloc_bytes"`
				NumGC      uint32 `json:"num_gc"`
				Stacks     string `json:"stacks"`
				Truncated  bool   `json:"truncated"`
			}
			if err := json.Unmarshal([]byte(msg.ResultJson), &d); err != nil {
				fmt.Printf("# %s: %s\n", msg.Node, msg.ResultJson)
				continue
			}
			fmt.Printf("# %s  goroutines=%d heap=%.1fMiB gc=%d\n", msg.Node, d.Goroutines, float64(d.HeapAlloc)/(1<<20), d.NumGC)
			fmt.Println(d.Stacks)
			if d.Truncated {
				fmt.Println("# (dump truncated)")
			}
		}
	}
	return cmd
}

func adminTraceCommand() *command {
	cmd := newCommand("trace")
	cmd.Description = func() string { return "follow one request (by trace id) or one user across the whole fleet" }
	cmd.Usage = func() string {
		return "Usage: admin trace [-admin-host h] [-user u] [-f] [-n N] [-since D] [-o text|json] [<traceid>]"
	}
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	user := cmd.String("user", "", "trace by username instead of trace id")
	follow := cmd.Bool("f", false, "follow: stream new matching lines until interrupted")
	tail := cmd.Int("n", 200, "number of recent lines per instance")
	since := cmd.String("since", "", "only lines newer than a duration (e.g. 5m) or an RFC3339 time")
	output := cmd.String("o", "text", "output format: text | json")
	cmd.ResetFlags = func() { *adminHost, *user, *follow, *tail, *since, *output = "", "", false, 200, "", "text" }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 && *user == "" {
			return errors.New(cmd.Usage())
		}
		if *output != "text" && *output != "json" {
			return fmt.Errorf("unknown -o %q (use text or json)", *output)
		}
		// A trace id is grepped as a bare substring; a user by its structured
		// log field, which the auth layer stamps on every authenticated request.
		grep := ""
		if cmd.NArg() > 0 {
			grep = cmd.Args()[0]
		}
		if *user != "" {
			if grep != "" {
				return errors.New("pass a trace id or -user, not both")
			}
			grep = fmt.Sprintf("%q:%q", "user", *user)
		}
		args := map[string]string{"limit": strconv.Itoa(*tail), "grep": grep}
		if *since != "" {
			args["since"] = *since
		}

		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		// Fan out to every instance in the fleet; each returns its own service's
		// matching lines, so the merged result is the request's full path.
		if *follow {
			return followLogs(ctx, client, "*", args, *output, true)
		}
		return snapshotLogs(ctx, client, "*", args, *output, true)
	}
	return cmd
}

func adminImpersonateCommand() *command {
	cmd := newCommand("impersonate")
	cmd.Description = func() string { return "mint a user-scoped token for a target user" }
	cmd.Usage = func() string { return "Usage: admin impersonate [-admin-host h] [-reason r] <user>" }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	reason := cmd.String("reason", "", "reason recorded in the audit log")
	cmd.ResetFlags = func() { *adminHost, *reason = "", "" }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New(cmd.Usage())
		}
		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		res, err := client.Impersonate(ctx, &adminpb.ImpersonateRequest{User: cmd.Args()[0], Reason: *reason})
		if err != nil {
			return adminErr(err)
		}
		fmt.Println(res.Token)
		return nil
	}
	return cmd
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
