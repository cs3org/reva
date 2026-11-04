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
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/spf13/cobra"
)

func jobCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "job", Short: "Inspect and drive the background jobs runner"}
	// run wraps the subcommands that need only the client.
	run := func(f func(ctx context.Context, client adminpb.AdminAPIClient, args []string) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			client, ctx, err := rt.Admin(cmd.Context())
			if err != nil {
				return err
			}
			return f(ctx, client, args)
		}
	}

	var runsJob, runsOwner, runsState string
	var runsInternal bool
	var runsLimit int32
	runs := &cobra.Command{
		Use:   "runs",
		Short: "Show the durable run history from the store",
		Args:  cobra.NoArgs,
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, _ []string) error {
			return jobsRuns(ctx, client, runsJob, runsOwner, runsState, runsInternal, runsLimit)
		}),
	}
	runs.Flags().StringVar(&runsJob, "job", "", "only the runs of this job")
	runs.Flags().StringVar(&runsOwner, "owner", "", "only the runs of this owner")
	runs.Flags().StringVar(&runsState, "state", "", "only these states, comma-separated (queued,running,succeeded,failed,cancelled,aborted)")
	runs.Flags().BoolVar(&runsInternal, "internal", false, "only internal (ownerless) runs")
	runs.Flags().Int32VarP(&runsLimit, "limit", "n", 50, "max rows")

	var logsJob string
	var logsLines int
	var logsFollow bool
	logs := &cobra.Command{
		Use:   "logs [<run-id>]",
		Short: "Show the recent log lines of a run, or of every run of a job with --job",
		Long: "Show the recent log lines of a run, from every service it called.\n" +
			"With --job, show the lines of every run of that job.",
		Args: cobra.MaximumNArgs(1),
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, args []string) error {
			return jobsLogs(ctx, client, args, logsJob, logsLines, logsFollow, rt.Output())
		}),
	}
	logs.Flags().StringVar(&logsJob, "job", "", "show the runs of this job")
	logs.Flags().IntVarP(&logsLines, "lines", "n", 50, "lines per instance")
	logs.Flags().BoolVarP(&logsFollow, "follow", "f", false, "follow (a run until it succeeds or is cancelled, a job until interrupted)")

	var runOwner string
	runJob := &cobra.Command{
		Use:   "run <job> [key=val ...]",
		Short: "Enqueue an on-demand run",
		Args:  cobra.MinimumNArgs(1),
		RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, args []string) error {
			return jobsRun(ctx, client, args, runOwner)
		}),
	}
	runJob.Flags().StringVar(&runOwner, "owner", "", "attribute the run to this user")

	simple := func(op, use, short string) *cobra.Command {
		return &cobra.Command{
			Use:   op + " " + use,
			Short: short,
			Args:  cobra.ExactArgs(1),
			RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, args []string) error {
				return jobsSimple(ctx, client, op, args)
			}),
		}
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List the registered jobs and their live status",
			Args:  cobra.NoArgs,
			RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, _ []string) error {
				return jobsList(ctx, client)
			}),
		},
		&cobra.Command{
			Use:   "active",
			Short: "Show the runs executing right now, per node",
			Args:  cobra.NoArgs,
			RunE: run(func(ctx context.Context, client adminpb.AdminAPIClient, _ []string) error {
				return jobsActive(ctx, client)
			}),
		},
		runs,
		&cobra.Command{
			Use:   "status <run-id>",
			Short: "Show the full status of one run",
			Args:  cobra.ExactArgs(1),
			RunE:  run(jobsStatus),
		},
		logs,
		runJob,
		simple("trigger", "<job>", "Run a periodic job now"),
		&cobra.Command{
			Use:   "cancel <run-id>",
			Short: "Cancel a run",
			Args:  cobra.ExactArgs(1),
			RunE:  run(jobsCancel),
		},
		simple("stop", "<job>", "Cancel the in-flight run of a periodic job"),
		simple("forget", "<job>", "Drop the queue state of a removed job"),
	)
	return cmd
}

// jobsList merges every runner's introspection into a job-centric view.
func jobsList(ctx context.Context, client adminpb.AdminAPIClient) error {
	resp, err := client.InspectJobs(ctx, &adminpb.InspectJobsRequest{})
	if err != nil {
		return adminErr(err)
	}
	type agg struct {
		def     *adminpb.JobDefinition
		running []string // nodes where it is executing
	}
	jobs := map[string]*agg{}
	var order []string
	for _, r := range resp.Runners {
		if r.Error != "" {
			fmt.Printf("# %s: error: %s\n", r.Node, r.Error)
			continue
		}
		for _, d := range r.Jobs {
			if _, ok := jobs[d.Name]; !ok {
				jobs[d.Name] = &agg{def: d}
				order = append(order, d.Name)
			}
		}
		for _, a := range r.ActiveRuns {
			if j := jobs[a.Job]; j != nil {
				j.running = append(j.running, shortNode(r.Node))
			}
		}
		for _, name := range r.InFlightPeriodics {
			if j := jobs[name]; j != nil {
				j.running = append(j.running, shortNode(r.Node))
			}
		}
	}
	sort.Strings(order)
	tw := newTab()
	fmt.Fprintln(tw, "JOB\tKIND\tSCHEDULE\tSCOPE\tSTATUS")
	for _, name := range order {
		j := jobs[name]
		status := "idle"
		if len(j.running) > 0 {
			status = "running on " + strings.Join(dedup(j.running), ",")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", j.def.Name, j.def.Kind, dash(j.def.Schedule), dash(j.def.Scope), status)
	}
	return tw.Flush()
}

// jobsActive shows what each runner is executing right now.
func jobsActive(ctx context.Context, client adminpb.AdminAPIClient) error {
	resp, err := client.InspectJobs(ctx, &adminpb.InspectJobsRequest{})
	if err != nil {
		return adminErr(err)
	}
	tw := newTab()
	fmt.Fprintln(tw, "NODE\tRUN ID\tJOB\tRUNNING FOR\tPROGRESS\tWORKERS\tSTORE")
	for _, r := range resp.Runners {
		if r.Error != "" {
			fmt.Fprintf(tw, "%s\terror: %s\t\t\t\t\t\n", r.Node, r.Error)
			continue
		}
		store := boolLabel(r.StoreWired, "yes", "no")
		workers := fmt.Sprintf("%d/%d", r.Busy, r.Workers)
		if len(r.ActiveRuns) == 0 {
			fmt.Fprintf(tw, "%s\t-\t-\t-\t-\t%s\t%s\n", r.Node, workers, store)
			continue
		}
		for _, a := range r.ActiveRuns {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Node, a.RunId, a.Job, since(a.Started), progressLabel(a.Progress), workers, store)
		}
	}
	return tw.Flush()
}

func jobsRuns(ctx context.Context, client adminpb.AdminAPIClient, job, owner, states string, internal bool, limit int32) error {
	req := &adminpb.ListJobRunsRequest{Job: job, Owner: owner, Internal: internal, Limit: limit}
	if states != "" {
		req.States = strings.Split(states, ",")
	}
	resp, err := client.ListJobRuns(ctx, req)
	if err != nil {
		return adminErr(err)
	}
	tw := newTab()
	fmt.Fprintln(tw, "RUN ID\tJOB\tSTATE\tOWNER\tATTEMPT\tENQUEUED\tDURATION")
	for _, r := range resp.Runs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			r.RunId, r.Job, r.State, dash(r.Owner), r.Attempt, since(r.EnqueuedAt), duration(r.StartedAt, r.FinishedAt))
	}
	return tw.Flush()
}

func jobsStatus(ctx context.Context, client adminpb.AdminAPIClient, args []string) error {
	r, err := client.GetJobRun(ctx, &adminpb.GetJobRunRequest{RunId: args[0]})
	if err != nil {
		return adminErr(err)
	}
	fmt.Printf("run:      %s\njob:      %s\nstate:    %s\nattempt:  %d\nowner:    %s\nenqueued: %s\nstarted:  %s\nfinished: %s\n",
		r.RunId, r.Job, r.State, r.Attempt, dash(r.Owner), dash(r.EnqueuedAt), dash(r.StartedAt), dash(r.FinishedAt))
	if r.CancelRequested {
		fmt.Println("cancel:   requested")
	}
	if r.Progress != nil {
		fmt.Printf("progress: %s (%s)\n", progressLabel(r.Progress), since(r.Progress.At))
		if r.Progress.DetailsJson != "" {
			fmt.Printf("details:  %s\n", r.Progress.DetailsJson)
		}
	}
	if r.LastError != "" {
		fmt.Printf("error:    %s\n", r.LastError)
	}
	if r.ResultJson != "" {
		fmt.Printf("result:   %s\n", r.ResultJson)
	}
	return nil
}

func jobsLogs(ctx context.Context, client adminpb.AdminAPIClient, args []string, job string, limit int, follow bool, output string) error {
	if (len(args) < 1) == (job == "") {
		return errors.New("pass a run id or --job, not both")
	}
	if job != "" {
		logArgs := map[string]string{"limit": strconv.Itoa(limit), "job": job}
		if follow {
			return followLogs(ctx, client, "*", logArgs, output, true)
		}
		return snapshotLogs(ctx, client, "*", logArgs, output, true)
	}
	// The runner uses the run id as the trace id of the job, so a grep for it
	// also finds the lines of the services that the job called.
	id := args[0]
	logArgs := map[string]string{"limit": strconv.Itoa(limit), "grep": id}
	if !follow {
		return snapshotLogs(ctx, client, "*", logArgs, output, true)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go stopWhenRunEnds(ctx, cancel, client, id)
	return followLogs(ctx, client, "*", logArgs, output, true)
}

// stopWhenRunEnds cancels a follow when the run succeeds or is cancelled. A
// failed run is retried, so the follow continues. Periodic runs have no
// status, so for them the follow continues until interrupted.
func stopWhenRunEnds(ctx context.Context, cancel context.CancelFunc, client adminpb.AdminAPIClient, id string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		r, err := client.GetJobRun(ctx, &adminpb.GetJobRunRequest{RunId: id})
		if err != nil {
			continue
		}
		if r.State == "succeeded" || r.State == "cancelled" {
			// Give the last lines of the other instances time to arrive.
			time.Sleep(time.Second)
			cancel()
			return
		}
	}
}

func jobsRun(ctx context.Context, client adminpb.AdminAPIClient, args []string, owner string) error {
	params, err := parseKeyVals(args[1:])
	if err != nil {
		return err
	}
	resp, err := client.EnqueueJob(ctx, &adminpb.EnqueueJobRequest{
		Job:    args[0],
		Owner:  owner,
		Params: params,
	})
	if err != nil {
		return adminErr(err)
	}
	fmt.Printf("enqueued run %s\n", resp.RunId)
	return nil
}

func jobsCancel(ctx context.Context, client adminpb.AdminAPIClient, args []string) error {
	r, err := client.CancelJobRun(ctx, &adminpb.CancelJobRunRequest{RunId: args[0]})
	if err != nil {
		return adminErr(err)
	}
	fmt.Printf("%s: %s\n", r.RunId, r.State)
	return nil
}

// jobsSimple handles the job-name mutations with no interesting result: trigger,
// stop and forget.
func jobsSimple(ctx context.Context, client adminpb.AdminAPIClient, op string, args []string) error {
	var err error
	switch op {
	case "trigger":
		_, err = client.TriggerJob(ctx, &adminpb.TriggerJobRequest{Job: args[0]})
	case "forget":
		_, err = client.ForgetJob(ctx, &adminpb.ForgetJobRequest{Job: args[0]})
	default:
		_, err = client.CancelPeriodicJob(ctx, &adminpb.CancelPeriodicJobRequest{Job: args[0]})
	}
	if err != nil {
		return adminErr(err)
	}
	fmt.Printf("%s: %s ok\n", args[0], op)
	return nil
}

// ----- small rendering helpers -----

func newTab() *tabwriter.Writer { return tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0) }

// since renders how long ago an RFC3339 instant was.
func since(rfc string) string {
	if rfc == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return rfc
	}
	return time.Since(t).Round(time.Second).String() + " ago"
}

// duration renders how long a run took/has taken from its start and finish.
func duration(startRFC, finishRFC string) string {
	if startRFC == "" {
		return "-"
	}
	start, err := time.Parse(time.RFC3339, startRFC)
	if err != nil {
		return "-"
	}
	end := time.Now()
	if finishRFC != "" {
		if t, err := time.Parse(time.RFC3339, finishRFC); err == nil {
			end = t
		}
	}
	return end.Sub(start).Round(time.Second).String()
}

// progressLabel renders a progress snapshot as "phase done/total unit (pct)".
func progressLabel(p *adminpb.JobProgress) string {
	if p == nil {
		return "-"
	}
	var b strings.Builder
	if p.Phase != "" {
		b.WriteString(p.Phase + " ")
	}
	fmt.Fprintf(&b, "%d", p.Done)
	if p.Total > 0 {
		fmt.Fprintf(&b, "/%d", p.Total)
	}
	if p.Unit != "" {
		b.WriteString(" " + p.Unit)
	}
	if p.Total > 0 {
		fmt.Fprintf(&b, " (%d%%)", p.Done*100/p.Total)
	}
	return b.String()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func boolLabel(b bool, y, n string) string {
	if b {
		return y
	}
	return n
}

// shortNode trims the "/service" tail from a node id for compact display.
func shortNode(node string) string {
	if i := strings.LastIndex(node, "/"); i >= 0 {
		return node[:i]
	}
	return node
}

func dedup(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
