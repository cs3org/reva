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
	"text/tabwriter"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/spf13/cobra"
)

func remindersCommand() *cobra.Command {
	run := func(f func(ctx context.Context, client adminpb.AdminAPIClient, w io.Writer, args []string) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Admin(cmd.Context())
			if err != nil {
				return err
			}
			return f(ctx, client, cmd.OutOrStdout(), args)
		}
	}

	cmd := &cobra.Command{
		Use:   "reminders",
		Short: "Show the state of the reminder kinds",
		Long: "Show the state of the reminder kinds: one row per kind, or with a\n" +
			"subcommand the open episodes, the reminders sent, a dry scan of a kind,\n" +
			"or forget a subject.\n\n" +
			"To run a kind now: reva job run reminders.<kind>",
		Example: "  reva reminders\n" +
			"  reva reminders state --kind linkexpiry --recipient alice\n" +
			"  reva reminders history --since 7d\n" +
			"  reva reminders check linkexpiry\n" +
			"  reva reminders forget linkexpiry <subject>",
		Args: cobra.NoArgs,
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, w io.Writer, _ []string) error {
			return remindersKinds(ctx, client, w)
		}),
	}

	type filterOpts struct {
		kind, recipient, subject string
		limit                    int
	}
	filterFlags := func(c *cobra.Command, o *filterOpts) {
		c.Flags().StringVar(&o.kind, "kind", "", "only this kind")
		c.Flags().StringVar(&o.recipient, "recipient", "", "only this recipient")
		c.Flags().StringVar(&o.subject, "subject", "", "only this subject")
		c.Flags().IntVarP(&o.limit, "limit", "n", 100, "max rows")
	}
	filterArgs := func(o *filterOpts) map[string]string {
		return map[string]string{"kind": o.kind, "recipient": o.recipient, "subject": o.subject, "n": strconv.Itoa(o.limit)}
	}

	var stateOpts filterOpts
	state := &cobra.Command{
		Use:   "state",
		Short: "Show the open episodes",
		Args:  cobra.NoArgs,
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, w io.Writer, _ []string) error {
			return remindersState(ctx, client, w, filterArgs(&stateOpts))
		}),
	}
	filterFlags(state, &stateOpts)

	var historyOpts filterOpts
	var historySince string
	history := &cobra.Command{
		Use:   "history",
		Short: "Show the reminders sent, the most recent first",
		Args:  cobra.NoArgs,
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, w io.Writer, _ []string) error {
			filter := filterArgs(&historyOpts)
			if historySince != "" {
				t, err := parseSince(historySince)
				if err != nil {
					return err
				}
				filter["since"] = t.Format(time.RFC3339)
			}
			return remindersHistory(ctx, client, w, filter)
		}),
	}
	filterFlags(history, &historyOpts)
	history.Flags().StringVar(&historySince, "since", "", "only after this, e.g. 7d, 12h or an RFC 3339 time")

	var checkLimit int
	check := &cobra.Command{
		Use:   "check <kind>",
		Short: "Scan a kind now and show what a run would send",
		Args:  cobra.ExactArgs(1),
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, w io.Writer, args []string) error {
			return remindersCheck(ctx, client, w, args[0], checkLimit)
		}),
	}
	check.Flags().IntVarP(&checkLimit, "limit", "n", 100, "max rows")

	forget := &cobra.Command{
		Use:   "forget <kind> <subject>",
		Short: "Tell this subject again from the lowest level",
		Args:  cobra.ExactArgs(2),
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, w io.Writer, args []string) error {
			var res struct {
				Forgotten int `json:"forgotten"`
			}
			if err := remindersInvoke(ctx, client, "reminders", "forget", map[string]string{"kind": args[0], "subject": args[1]}, &res); err != nil {
				return err
			}
			return rt.Print(w, res, func(w io.Writer) { fmt.Fprintf(w, "forgot %d episode(s)\n", res.Forgotten) })
		}),
	}

	cmd.AddCommand(state, history, check, forget)
	return cmd
}

// remindersInvoke decodes the first answer, because every instance reads the same database.
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

func remindersTab(w io.Writer) *tabwriter.Writer { return tabwriter.NewWriter(w, 0, 2, 2, ' ', 0) }

func remindersKinds(ctx context.Context, client adminpb.AdminAPIClient, w io.Writer) error {
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
	return rt.Print(w, res, func(w io.Writer) {
		tw := remindersTab(w)
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
		_ = tw.Flush()
	})
}

func remindersState(ctx context.Context, client adminpb.AdminAPIClient, w io.Writer, filter map[string]string) error {
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
	return rt.Print(w, res, func(w io.Writer) {
		tw := remindersTab(w)
		fmt.Fprintln(tw, "KIND\tSUBJECT\tRECIPIENT\tLEVEL SENT\tOPENED\tSEEN\tCLAIMED")
		for _, e := range res.Episodes {
			to := e.Recipient
			if e.Group {
				to += " (group)"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n", e.Kind, e.Subject, to, e.LevelSent, since(e.OpenedAt), since(e.SeenAt), since(e.ClaimedAt))
		}
		_ = tw.Flush()
	})
}

func remindersHistory(ctx context.Context, client adminpb.AdminAPIClient, w io.Writer, filter map[string]string) error {
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
	return rt.Print(w, res, func(w io.Writer) {
		tw := remindersTab(w)
		fmt.Fprintln(tw, "SENT\tKIND\tSUBJECT\tLEVEL\tRECIPIENT\tEVENT")
		for _, s := range res.Sent {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", s.SentAt, s.Kind, s.Subject, s.Level, s.Recipient, s.Event)
		}
		_ = tw.Flush()
	})
}

// remindersCheck asks one instance only, because the scan reads the source.
func remindersCheck(ctx context.Context, client adminpb.AdminAPIClient, w io.Writer, kind string, n int) error {
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
	return rt.Print(w, res, func(w io.Writer) {
		fmt.Fprintf(w, "%s on %s: %d scanned, %d due for %d recipient(s), %d failure(s)\n", kind, node, res.Report.Scanned, res.Report.Due, res.Report.Recipients, res.Report.Failures)
		if res.Error != "" {
			fmt.Fprintf(w, "errors: %s\n", res.Error)
		}
		tw := remindersTab(w)
		fmt.Fprintln(tw, "SUBJECT\tLEVEL\tLEVEL SENT\tRECIPIENT")
		for _, d := range res.Due {
			to := d.Recipient
			if d.Group {
				to += " (group)"
			}
			fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", d.Subject, d.Level, d.LevelSent, to)
		}
		_ = tw.Flush()
	})
}

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
