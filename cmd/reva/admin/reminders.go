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
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

const remindersHelp = `admin reminders: the state of the reminder kinds

Subcommands:
  reminders                         one row per kind
  reminders state                   the open episodes
  reminders history                 the reminders sent, the most recent first
  reminders check <kind>            scan a kind now and show what a run would send
  reminders forget <kind> <subject> tell this subject again from the lowest level

Flags (before the subcommand):
  -admin-host <addr>   admin gRPC endpoint, persisted
  -kind <name>         state, history: only this kind
  -recipient <name>    state, history: only this recipient
  -subject <subject>   state, history: only this subject
  -since <d>           history: only after this, e.g. 7d, 12h or an RFC 3339 time
  -n <N>               max rows (default 100)

To run a kind now: admin jobs run reminders.<kind>
`

func adminRemindersCommand() *command {
	cmd := newCommand("reminders")
	cmd.Description = func() string { return "the state of the reminder kinds" }
	cmd.Usage = func() string { return "Usage: admin reminders [state|history|check|forget] ..." }
	cmd.FlagSet.Usage = func() { fmt.Fprint(cmd.Output(), remindersHelp) }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	kind := cmd.String("kind", "", "state, history: only this kind")
	recipient := cmd.String("recipient", "", "state, history: only this recipient")
	subject := cmd.String("subject", "", "state, history: only this subject")
	sinceFlag := cmd.String("since", "", "history: only after this, e.g. 7d, 12h or an RFC 3339 time")
	n := cmd.Int("n", 100, "max rows")
	cmd.ResetFlags = func() {
		*adminHost, *kind, *recipient, *subject, *sinceFlag, *n = "", "", "", "", "", 100
	}
	cmd.Action = func(w ...io.Writer) error {
		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		filter := map[string]string{"kind": *kind, "recipient": *recipient, "subject": *subject, "n": strconv.Itoa(*n)}
		sub, rest := "", []string(nil)
		if cmd.NArg() > 0 {
			sub, rest = cmd.Args()[0], cmd.Args()[1:]
		}
		switch sub {
		case "":
			return remindersKinds(ctx, client)
		case "state":
			return remindersState(ctx, client, filter)
		case "history":
			if *sinceFlag != "" {
				t, err := parseSince(*sinceFlag)
				if err != nil {
					return err
				}
				filter["since"] = t.Format(time.RFC3339)
			}
			return remindersHistory(ctx, client, filter)
		case "check":
			if len(rest) != 1 {
				return errors.New("usage: admin reminders check <kind>")
			}
			return remindersCheck(ctx, client, rest[0], *n)
		case "forget":
			if len(rest) != 2 {
				return errors.New("usage: admin reminders forget <kind> <subject>")
			}
			var res struct {
				Forgotten int `json:"forgotten"`
			}
			if err := remindersInvoke(ctx, client, "reminders", "forget", map[string]string{"kind": rest[0], "subject": rest[1]}, &res); err != nil {
				return err
			}
			fmt.Printf("forgot %d episode(s)\n", res.Forgotten)
			return nil
		default:
			return fmt.Errorf("unknown reminders subcommand %q; run `admin reminders -h` for help", sub)
		}
	}
	return cmd
}

// remindersInvoke runs an invocation and decodes the first answer. Every
// instance reads the same database.
func remindersInvoke(ctx context.Context, client adminpb.AdminAPIClient, selector, invocation string, args map[string]string, out any) error {
	res, err := client.Invoke(ctx, &adminpb.InvokeRequest{Service: selector, Invocation: invocation, Args: args})
	if err != nil {
		return adminErr(err)
	}
	var failures []error
	for _, r := range res.Results {
		if r.Error != "" {
			failures = append(failures, fmt.Errorf("%s: %s", r.Node, r.Error))
			continue
		}
		return json.Unmarshal([]byte(r.ResultJson), out)
	}
	if len(failures) == 0 {
		return errors.New("no reminders service answered")
	}
	return errors.Join(failures...)
}

func remindersKinds(ctx context.Context, client adminpb.AdminAPIClient) error {
	var res struct {
		Kinds []struct {
			Name     string `json:"name"`
			Event    string `json:"event"`
			Schedule string `json:"schedule"`
			DryRun   bool   `json:"dry_run"`
			Summary  struct {
				Open    int64 `json:"open"`
				Pending int64 `json:"pending"`
				Sent24h int64 `json:"sent_24h"`
				Sent7d  int64 `json:"sent_7d"`
			} `json:"summary"`
			LastRun *struct {
				FinishedAt string `json:"finished_at"`
				Error      string `json:"error"`
				Report     struct {
					Due       int  `json:"due"`
					Published int  `json:"published"`
					Failures  int  `json:"failures"`
					DryRun    bool `json:"dry_run"`
				} `json:"report"`
			} `json:"last_run"`
		} `json:"kinds"`
	}
	if err := remindersInvoke(ctx, client, "reminders", "kinds", nil, &res); err != nil {
		return err
	}
	tw := newTab()
	fmt.Fprintln(tw, "KIND\tEVENT\tSCHEDULE\tDRY RUN\tOPEN\tPENDING\tSENT 24H\tSENT 7D\tLAST RUN")
	for _, k := range res.Kinds {
		last := "never"
		if r := k.LastRun; r != nil {
			switch {
			case r.Error != "":
				last = fmt.Sprintf("failed %s: %s", since(r.FinishedAt), r.Error)
			case r.Report.DryRun:
				last = fmt.Sprintf("ok %s (dry run, %d due)", since(r.FinishedAt), r.Report.Due)
			default:
				last = fmt.Sprintf("ok %s (%d published)", since(r.FinishedAt), r.Report.Published)
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%d\t%d\t%d\t%d\t%s\n", k.Name, k.Event, dash(k.Schedule), k.DryRun,
			k.Summary.Open, k.Summary.Pending, k.Summary.Sent24h, k.Summary.Sent7d, last)
	}
	return tw.Flush()
}

func remindersState(ctx context.Context, client adminpb.AdminAPIClient, filter map[string]string) error {
	var res struct {
		Episodes []struct {
			Kind      string `json:"kind"`
			Subject   string `json:"subject"`
			Recipient string `json:"recipient"`
			Group     bool   `json:"recipient_group"`
			LevelSent int    `json:"level_sent"`
			ClaimedAt string `json:"claimed_at"`
			OpenedAt  string `json:"opened_at"`
			SeenAt    string `json:"seen_at"`
		} `json:"episodes"`
	}
	if err := remindersInvoke(ctx, client, "reminders", "state", filter, &res); err != nil {
		return err
	}
	tw := newTab()
	fmt.Fprintln(tw, "KIND\tSUBJECT\tRECIPIENT\tLEVEL SENT\tOPENED\tSEEN\tCLAIMED")
	for _, e := range res.Episodes {
		to := e.Recipient
		if e.Group {
			to += " (group)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n", e.Kind, e.Subject, to, e.LevelSent, since(e.OpenedAt), since(e.SeenAt), since(e.ClaimedAt))
	}
	return tw.Flush()
}

func remindersHistory(ctx context.Context, client adminpb.AdminAPIClient, filter map[string]string) error {
	var res struct {
		Sent []struct {
			Kind      string `json:"kind"`
			Subject   string `json:"subject"`
			Level     int    `json:"level"`
			Recipient string `json:"recipient"`
			Event     string `json:"event"`
			SentAt    string `json:"sent_at"`
		} `json:"sent"`
	}
	if err := remindersInvoke(ctx, client, "reminders", "history", filter, &res); err != nil {
		return err
	}
	tw := newTab()
	fmt.Fprintln(tw, "SENT\tKIND\tSUBJECT\tLEVEL\tRECIPIENT\tEVENT")
	for _, s := range res.Sent {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", s.SentAt, s.Kind, s.Subject, s.Level, s.Recipient, s.Event)
	}
	return tw.Flush()
}

// remindersCheck runs check on one instance only: it scans the source.
func remindersCheck(ctx context.Context, client adminpb.AdminAPIClient, kind string, n int) error {
	svcs, err := client.ListServices(ctx, &adminpb.ListServicesRequest{Service: "reminders"})
	if err != nil {
		return adminErr(err)
	}
	node := ""
	for _, s := range svcs.Services {
		if s.Name == "reminders" && len(s.Nodes) > 0 {
			node = s.Nodes[0].Id
			break
		}
	}
	if node == "" {
		return errors.New("no reminders service is running")
	}
	var res struct {
		Report struct {
			Scanned    int `json:"scanned"`
			Due        int `json:"due"`
			Recipients int `json:"recipients"`
			Failures   int `json:"failures"`
		} `json:"report"`
		Due []struct {
			Subject   string `json:"subject"`
			Level     int    `json:"level"`
			LevelSent int    `json:"level_sent"`
			Recipient string `json:"recipient"`
			Group     bool   `json:"recipient_group"`
		} `json:"due"`
		Error string `json:"error"`
	}
	if err := remindersInvoke(ctx, client, node, "check", map[string]string{"kind": kind, "n": strconv.Itoa(n)}, &res); err != nil {
		return err
	}
	fmt.Printf("%s on %s: %d scanned, %d due for %d recipient(s), %d failure(s)\n", kind, node, res.Report.Scanned, res.Report.Due, res.Report.Recipients, res.Report.Failures)
	if res.Error != "" {
		fmt.Printf("errors: %s\n", res.Error)
	}
	tw := newTab()
	fmt.Fprintln(tw, "SUBJECT\tLEVEL\tLEVEL SENT\tRECIPIENT")
	for _, d := range res.Due {
		to := d.Recipient
		if d.Group {
			to += " (group)"
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", d.Subject, d.Level, d.LevelSent, to)
	}
	return tw.Flush()
}

// parseSince reads a number of days ("7d"), a Go duration ("12h") or an
// RFC 3339 time.
func parseSince(s string) (time.Time, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		if n, err := strconv.Atoi(days); err == nil {
			return time.Now().AddDate(0, 0, -n), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d), nil
	}
	return time.Parse(time.RFC3339, s)
}
