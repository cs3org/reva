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

// Package rjobs provides a framework to run background work in reva, both
// periodically and once on demand. Jobs are registered into a Runner which
// owns scheduling, the worker pool and the lifecycle; callers only declare
// what to run and how often.
package rjobs

import (
	"context"
	"time"
)

// Scope declares where a periodic job runs in a multi-replica deployment.
// It is a required field of Periodic: the zero value is invalid so that the
// author has to think about the semantics explicitly.
type Scope int

const (
	// ScopeAllNodes runs the job on every replica with a local ticker. It
	// never goes through the durable queue, so it keeps working even when the
	// store is unavailable. Use it for replica-local state, e.g. warming an
	// in-memory cache.
	ScopeAllNodes Scope = iota + 1
	// ScopeLeader runs the job on exactly one replica through the durable
	// queue and gated by the elector. Use it for work that mutates state
	// shared by all replicas, e.g. a cleanup that must fire once.
	ScopeLeader
)

// String returns a human readable name for the scope.
func (s Scope) String() string {
	switch s {
	case ScopeAllNodes:
		return "all-nodes"
	case ScopeLeader:
		return "leader"
	default:
		return "invalid"
	}
}

// OverlapPolicy decides what happens when a job is due again while a previous
// run of the same job is still in flight.
type OverlapPolicy int

const (
	// Skip drops the new run if the previous one has not finished yet. This is
	// the default and the right choice for most refresh/cleanup work.
	Skip OverlapPolicy = iota
	// Allow lets runs overlap.
	Allow
)

// Params is the payload of an on-demand job. It must be JSON serialisable so
// that it survives a round trip through the durable store. Periodic jobs
// receive an empty Params.
type Params map[string]any

// Job is a unit of on-demand background work. Implementations must be
// idempotent and re-entrant: the framework guarantees at-least-once delivery,
// so the same job may run more than once (e.g. after a crash) or concurrently.
type Job interface {
	// Run executes the job. The returned Params are recorded as the run's
	// result (e.g. a download URL) and surfaced through the run status; return
	// nil if the job has no result. Returning an error marks the run as failed;
	// the store decides retry and backoff. ctx is cancelled on shutdown.
	Run(ctx context.Context, p Params) (Params, error)
}

// NewJob constructs an on-demand job from its configuration. It is the
// function registered with RegisterOnDemand. The map is the job's own section
// of the service configuration
// ([serverless.services.jobs.on_demand."<name>"]), or nil when the job has no
// such section, in which case it should fall back to its defaults.
type NewJob func(ctx context.Context, m map[string]any) (Job, error)

// Periodic describes a job that runs on a schedule. The Run closure is
// expected to capture whatever dependencies the job needs (a cache handle, a
// client, ...), so periodic jobs usually live next to the component that owns
// the work.
type Periodic struct {
	// Name is the stable identity of the job, used for logs, metrics,
	// deduplication and as the config key. It must be unique across all
	// registered jobs.
	Name string
	// Schedule is the interval spec, e.g. "@every 5m", "@hourly", "@daily" or
	// "@weekly". See schedule.go for the supported grammar.
	Schedule string
	// Scope is required and selects the execution path. See Scope.
	Scope Scope
	// Run is the work to perform. The same idempotency contract as Job.Run
	// applies.
	Run func(ctx context.Context) error
	// RunOnStart fires the job once immediately when the runner starts,
	// instead of waiting for the first interval to elapse.
	RunOnStart bool
	// Jitter randomises each fire by up to the given duration, so that
	// replicas do not all run the job at the same instant.
	Jitter time.Duration
	// Overlap selects the behaviour when a run is due while a previous run is
	// still in flight. Defaults to Skip.
	Overlap OverlapPolicy
}
