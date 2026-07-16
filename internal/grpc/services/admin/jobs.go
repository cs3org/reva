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
	"strconv"
	"strings"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// jobsService is the registered name of the rjobs serverless runner.
const jobsService = "jobs"

// The jobs RPCs are the direct/fan-out planes over the rjobs runner. Their
// bodies use the invoke channel: InspectJobs fans out to every runner (live
// per-process state); the ledger reads and the mutations target one runner
// (shared store), since the store — not the ingress runner — decides execution.

// jobsEndpoints resolves every live jobs runner.
func (s *svc) jobsEndpoints() ([]endpoint, error) {
	reg, err := s.registryHandle()
	if err != nil {
		return nil, err
	}
	_, eps, err := resolveSelector(reg, jobsService)
	if err != nil || len(eps) == 0 {
		return nil, status.Error(codes.NotFound, "admin: no jobs runner is live in the fleet")
	}
	return eps, nil
}

// invokeJobsOne runs a single-target jobs invocation on one ready runner.
func (s *svc) invokeJobsOne(ctx context.Context, invocation string, args map[string]string) (*adminpb.NodeResult, error) {
	eps, err := s.jobsEndpoints()
	if err != nil {
		return nil, err
	}
	res := invokeOne(ctx, eps[0], invocation, args)
	if res.Error != "" {
		return nil, status.Errorf(codes.Internal, "admin: jobs %s: %s", invocation, res.Error)
	}
	return res, nil
}

func (s *svc) InspectJobs(ctx context.Context, _ *adminpb.InspectJobsRequest) (*adminpb.InspectJobsResponse, error) {
	eps, err := s.jobsEndpoints()
	if err != nil {
		return nil, err
	}
	resp := &adminpb.InspectJobsResponse{}
	for _, r := range fanOutInvoke(ctx, eps, "inspect", nil) {
		ri := &adminpb.JobRunnerInfo{Node: r.Node}
		if r.Error != "" {
			ri.Error = r.Error
			resp.Runners = append(resp.Runners, ri)
			continue
		}
		var info runnerInfoJSON
		if err := json.Unmarshal([]byte(r.ResultJson), &info); err != nil {
			ri.Error = "unparseable result: " + err.Error()
			resp.Runners = append(resp.Runners, ri)
			continue
		}
		ri.Workers, ri.Busy, ri.StoreWired = int32(info.Workers), int32(info.Busy), info.StoreWired
		ri.InFlightPeriodic = info.InFlightPeriodic
		for _, j := range info.Jobs {
			ri.Jobs = append(ri.Jobs, &adminpb.JobDefinition{
				Name: j.Name, Kind: j.Kind, Schedule: j.Schedule, Scope: j.Scope, Overlap: j.Overlap,
			})
		}
		for _, a := range info.Active {
			ri.Active = append(ri.Active, &adminpb.ActiveJobRun{RunId: a.RunID, Job: a.Job, Started: a.Started})
		}
		resp.Runners = append(resp.Runners, ri)
	}
	return resp, nil
}

func (s *svc) ListJobRuns(ctx context.Context, req *adminpb.ListJobRunsRequest) (*adminpb.ListJobRunsResponse, error) {
	args := map[string]string{}
	if req.Job != "" {
		args["job"] = req.Job
	}
	if req.Owner != "" {
		args["owner"] = req.Owner
	}
	if len(req.States) > 0 {
		args["states"] = strings.Join(req.States, ",")
	}
	if req.Internal {
		args["internal"] = "true"
	}
	if req.Limit > 0 {
		args["limit"] = strconv.Itoa(int(req.Limit))
	}
	if req.Offset > 0 {
		args["offset"] = strconv.Itoa(int(req.Offset))
	}
	res, err := s.invokeJobsOne(ctx, "runs", args)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Runs []jobRunJSON `json:"runs"`
	}
	if err := json.Unmarshal([]byte(res.ResultJson), &payload); err != nil {
		return nil, status.Errorf(codes.Internal, "admin: parsing runs: %v", err)
	}
	resp := &adminpb.ListJobRunsResponse{}
	for _, r := range payload.Runs {
		resp.Runs = append(resp.Runs, r.toProto())
	}
	return resp, nil
}

func (s *svc) GetJobRun(ctx context.Context, req *adminpb.GetJobRunRequest) (*adminpb.JobRun, error) {
	if req.RunId == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: run_id is required")
	}
	res, err := s.invokeJobsOne(ctx, "status", map[string]string{"run_id": req.RunId})
	if err != nil {
		return nil, err
	}
	return parseJobRun(res.ResultJson)
}

func (s *svc) EnqueueJob(ctx context.Context, req *adminpb.EnqueueJobRequest) (*adminpb.EnqueueJobResponse, error) {
	if req.Job == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: job is required")
	}
	args := map[string]string{"job": req.Job}
	if req.Owner != "" {
		args["owner"] = req.Owner
	}
	if len(req.Params) > 0 {
		b, _ := json.Marshal(req.Params)
		args["params"] = string(b)
	}
	res, err := s.invokeJobsOne(ctx, "enqueue", args)
	if err != nil {
		return nil, err
	}
	s.auditJob(ctx, "enqueue_job", req.Job, map[string]string{"owner": req.Owner})
	var p struct {
		RunID string `json:"run_id"`
	}
	_ = json.Unmarshal([]byte(res.ResultJson), &p)
	return &adminpb.EnqueueJobResponse{RunId: p.RunID}, nil
}

func (s *svc) TriggerJob(ctx context.Context, req *adminpb.TriggerJobRequest) (*adminpb.TriggerJobResponse, error) {
	if req.Job == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: job is required")
	}
	if _, err := s.invokeJobsOne(ctx, "trigger", map[string]string{"job": req.Job}); err != nil {
		return nil, err
	}
	s.auditJob(ctx, "trigger_job", req.Job, nil)
	return &adminpb.TriggerJobResponse{}, nil
}

func (s *svc) CancelJobRun(ctx context.Context, req *adminpb.CancelJobRunRequest) (*adminpb.JobRun, error) {
	if req.RunId == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: run_id is required")
	}
	res, err := s.invokeJobsOne(ctx, "cancel", map[string]string{"run_id": req.RunId})
	if err != nil {
		return nil, err
	}
	s.auditJob(ctx, "cancel_job_run", req.RunId, nil)
	return parseJobRun(res.ResultJson)
}

func (s *svc) CancelPeriodicJob(ctx context.Context, req *adminpb.CancelPeriodicJobRequest) (*adminpb.CancelPeriodicJobResponse, error) {
	if req.Job == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: job is required")
	}
	if _, err := s.invokeJobsOne(ctx, "stop", map[string]string{"job": req.Job}); err != nil {
		return nil, err
	}
	s.auditJob(ctx, "cancel_periodic_job", req.Job, nil)
	return &adminpb.CancelPeriodicJobResponse{}, nil
}

// auditJob records a jobs mutation.
func (s *svc) auditJob(ctx context.Context, action, target string, fields map[string]string) {
	admin.Audit(ctx, admin.AuditEvent{Action: action, Actor: actorName(ctx), Target: target, Fields: fields})
}

// ----- JSON mirrors of the invocation results (the control-channel contract) -----

type runnerInfoJSON struct {
	Workers          int             `json:"workers"`
	Busy             int             `json:"busy"`
	StoreWired       bool            `json:"store_wired"`
	Jobs             []jobDefJSON    `json:"jobs"`
	InFlightPeriodic []string        `json:"in_flight_periodic"`
	Active           []activeRunJSON `json:"active"`
}

type jobDefJSON struct {
	Name, Kind, Schedule, Scope, Overlap string
}

type activeRunJSON struct {
	RunID   string `json:"run_id"`
	Job     string `json:"job"`
	Started string `json:"started"`
}

type jobRunJSON struct {
	RunID           string         `json:"run_id"`
	Job             string         `json:"job"`
	State           string         `json:"state"`
	Attempt         int            `json:"attempt"`
	Owner           string         `json:"owner"`
	EnqueuedAt      string         `json:"enqueued_at"`
	StartedAt       string         `json:"started_at"`
	FinishedAt      string         `json:"finished_at"`
	LastError       string         `json:"last_error"`
	CancelRequested bool           `json:"cancel_requested"`
	Result          map[string]any `json:"result"`
}

func (r jobRunJSON) toProto() *adminpb.JobRun {
	pb := &adminpb.JobRun{
		RunId: r.RunID, Job: r.Job, State: r.State, Attempt: int32(r.Attempt), Owner: r.Owner,
		EnqueuedAt: r.EnqueuedAt, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
		LastError: r.LastError, CancelRequested: r.CancelRequested,
	}
	if len(r.Result) > 0 {
		b, _ := json.Marshal(r.Result)
		pb.ResultJson = string(b)
	}
	return pb
}

func parseJobRun(resultJSON string) (*adminpb.JobRun, error) {
	var r jobRunJSON
	if err := json.Unmarshal([]byte(resultJSON), &r); err != nil {
		return nil, status.Errorf(codes.Internal, "admin: parsing run: %v", err)
	}
	return r.toProto(), nil
}
