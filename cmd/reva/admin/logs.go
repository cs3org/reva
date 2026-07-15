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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

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
