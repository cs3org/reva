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
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

const jobsHelp = `admin jobs — inspect and drive the background jobs runner

Subcommands:
  jobs list                     registered jobs and their live status (fan-out)
  jobs active                   the runs executing right now, per node (fan-out)
  jobs runs                     durable run history from the store
  jobs status <run-id>          one run's full status
  jobs run <job> [k=v ...]      enqueue an on-demand run
  jobs trigger <job>            run a periodic job now
  jobs cancel <run-id>          cancel a run
  jobs stop <job>               cancel a periodic job's in-flight run

Flags (before the subcommand):
  -admin-host <addr>   admin gRPC endpoint, persisted
  -owner <user>        runs: filter by owner; run: attribute the run to a user
  -job <name>          runs: filter by job
  -state <s,...>       runs: filter by states (queued,running,succeeded,failed,cancelled)
  -internal            runs: only internal (ownerless) runs
  -n <N>               runs: max rows (default 50)
`

func adminJobsCommand() *command {
	cmd := newCommand("jobs")
	cmd.Description = func() string { return "inspect and drive the background jobs runner" }
	cmd.Usage = func() string { return "Usage: admin jobs <list|active|runs|status|run|trigger|cancel|stop> ..." }
	cmd.FlagSet.Usage = func() { fmt.Fprint(cmd.Output(), jobsHelp) }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	owner := cmd.String("owner", "", "runs: filter by owner; run: attribute to a user")
	jobFilter := cmd.String("job", "", "runs: filter by job")
	stateFilter := cmd.String("state", "", "runs: filter by states (comma-separated)")
	internal := cmd.Bool("internal", false, "runs: only internal (ownerless) runs")
	n := cmd.Int("n", 50, "runs: max rows")
	cmd.ResetFlags = func() {
		*adminHost, *owner, *jobFilter, *stateFilter, *internal, *n = "", "", "", "", false, 50
	}
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			fmt.Print(jobsHelp)
			return nil
		}
		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		sub, rest := cmd.Args()[0], cmd.Args()[1:]
		switch sub {
		case "list":
			return jobsList(ctx, client)
		case "active":
			return jobsActive(ctx, client)
		case "runs":
			return jobsRuns(ctx, client, *jobFilter, *owner, *stateFilter, *internal, int32(*n))
		case "status":
			return jobsStatus(ctx, client, rest)
		case "run":
			return jobsRun(ctx, client, rest, *owner)
		case "trigger":
			return jobsSimple(ctx, client, "trigger", rest)
		case "cancel":
			return jobsCancel(ctx, client, rest)
		case "stop":
			return jobsSimple(ctx, client, "stop", rest)
		default:
			return fmt.Errorf("unknown jobs subcommand %q; run `admin jobs` for help", sub)
		}
	}
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
	fmt.Fprintln(tw, "NODE\tRUN ID\tJOB\tRUNNING FOR\tWORKERS\tSTORE")
	for _, r := range resp.Runners {
		if r.Error != "" {
			fmt.Fprintf(tw, "%s\terror: %s\t\t\t\t\n", r.Node, r.Error)
			continue
		}
		store := boolLabel(r.StoreWired, "yes", "no")
		workers := fmt.Sprintf("%d/%d", r.Busy, r.Workers)
		if len(r.ActiveRuns) == 0 {
			fmt.Fprintf(tw, "%s\t-\t-\t-\t%s\t%s\n", r.Node, workers, store)
			continue
		}
		for _, a := range r.ActiveRuns {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Node, a.RunId, a.Job, since(a.Started), workers, store)
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
	if len(args) < 1 {
		return errors.New("Usage: admin jobs status <run-id>")
	}
	r, err := client.GetJobRun(ctx, &adminpb.GetJobRunRequest{RunId: args[0]})
	if err != nil {
		return adminErr(err)
	}
	fmt.Printf("run:      %s\njob:      %s\nstate:    %s\nattempt:  %d\nowner:    %s\nenqueued: %s\nstarted:  %s\nfinished: %s\n",
		r.RunId, r.Job, r.State, r.Attempt, dash(r.Owner), dash(r.EnqueuedAt), dash(r.StartedAt), dash(r.FinishedAt))
	if r.CancelRequested {
		fmt.Println("cancel:   requested")
	}
	if r.LastError != "" {
		fmt.Printf("error:    %s\n", r.LastError)
	}
	if r.ResultJson != "" {
		fmt.Printf("result:   %s\n", r.ResultJson)
	}
	return nil
}

func jobsRun(ctx context.Context, client adminpb.AdminAPIClient, args []string, owner string) error {
	if len(args) < 1 {
		return errors.New("Usage: admin jobs run [-owner U] <job> [key=val ...]")
	}
	resp, err := client.EnqueueJob(ctx, &adminpb.EnqueueJobRequest{
		Job:    args[0],
		Owner:  owner,
		Params: parseKeyVals(args[1:]),
	})
	if err != nil {
		return adminErr(err)
	}
	fmt.Printf("enqueued run %s\n", resp.RunId)
	return nil
}

func jobsCancel(ctx context.Context, client adminpb.AdminAPIClient, args []string) error {
	if len(args) < 1 {
		return errors.New("Usage: admin jobs cancel <run-id>")
	}
	r, err := client.CancelJobRun(ctx, &adminpb.CancelJobRunRequest{RunId: args[0]})
	if err != nil {
		return adminErr(err)
	}
	fmt.Printf("%s: %s\n", r.RunId, r.State)
	return nil
}

// jobsSimple handles the job-name mutations with no interesting result: trigger
// and stop.
func jobsSimple(ctx context.Context, client adminpb.AdminAPIClient, op string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: admin jobs %s <job>", op)
	}
	var err error
	if op == "trigger" {
		_, err = client.TriggerJob(ctx, &adminpb.TriggerJobRequest{Job: args[0]})
	} else {
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
