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
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/spf13/cobra"
)

func remindersCommand() *cobra.Command {
	// run wraps the subcommands that need only the client.
	run := func(f func(ctx context.Context, client adminpb.AdminAPIClient, args []string) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			client, ctx, err := adminDial(cmd)
			if err != nil {
				return err
			}
			return f(ctx, client, args)
		}
	}

	var kind, recipient, subject, sinceFlag string
	var n int
	filter := func() map[string]string {
		return map[string]string{"kind": kind, "recipient": recipient, "subject": subject, "n": strconv.Itoa(n)}
	}
	filterFlags := func(c *cobra.Command) {
		c.Flags().StringVar(&kind, "kind", "", "only this kind")
		c.Flags().StringVar(&recipient, "recipient", "", "only this recipient")
		c.Flags().StringVar(&subject, "subject", "", "only this subject")
	}

	cmd := &cobra.Command{
		Use:   "reminders",
		Short: "Show the state of the reminder kinds",
		Long:  "Show one row per reminder kind.\n\nTo run a kind now: reva admin jobs run reminders.<kind>",
		Args:  cobra.NoArgs,
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, _ []string) error {
			return remindersKinds(ctx, client)
		}),
	}
	cmd.PersistentFlags().IntVarP(&n, "limit", "n", 100, "max rows")

	state := &cobra.Command{
		Use:   "state",
		Short: "Show the open episodes",
		Args:  cobra.NoArgs,
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, _ []string) error {
			return remindersState(ctx, client, filter())
		}),
	}
	filterFlags(state)

	history := &cobra.Command{
		Use:   "history",
		Short: "Show the reminders sent, the most recent first",
		Args:  cobra.NoArgs,
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, _ []string) error {
			f := filter()
			if sinceFlag != "" {
				t, err := parseSince(sinceFlag)
				if err != nil {
					return err
				}
				f["since"] = t.Format(time.RFC3339)
			}
			return remindersHistory(ctx, client, f)
		}),
	}
	filterFlags(history)
	history.Flags().StringVar(&sinceFlag, "since", "", "only after this, e.g. 7d, 12h or an RFC 3339 time")

	cmd.AddCommand(
		state,
		history,
		&cobra.Command{
			Use:   "check <kind>",
			Short: "Scan a kind now and show what a run would send",
			Args:  cobra.ExactArgs(1),
			RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, args []string) error {
				return remindersCheck(ctx, client, args[0], n)
			}),
		},
		&cobra.Command{
			Use:   "forget <kind> <subject>",
			Short: "Tell this subject again from the lowest level",
			Args:  cobra.ExactArgs(2),
			RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, args []string) error {
				var res struct {
					Forgotten int `json:"forgotten"`
				}
				if err := remindersInvoke(ctx, client, "reminders", "forget", map[string]string{"kind": args[0], "subject": args[1]}, &res); err != nil {
					return err
				}
				fmt.Printf("forgot %d episode(s)\n", res.Forgotten)
				return nil
			}),
		},
	)
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
