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

// Package jobs hosts the rjobs runner as a serverless service. It builds the
// store from configuration, constructs the runner over the registered jobs,
// and exposes it process-wide for in-process enqueueing.
package jobs

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	revadcfg "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	natsstore "github.com/cs3org/reva/v3/pkg/rjobs/store/nats"
	sqlstatus "github.com/cs3org/reva/v3/pkg/rjobs/store/sql"
	"github.com/cs3org/reva/v3/pkg/rserverless"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/rs/zerolog"
)

func init() {
	rserverless.Register("jobs", New)
}

type config struct {
	WorkerPoolSize int    `mapstructure:"worker_pool_size"`
	NatsAddress    string `mapstructure:"nats_address"`
	NatsToken      string `mapstructure:"nats_token"`
	NatsPrefix     string `mapstructure:"nats_prefix"`
	// AckWaitSeconds is the visibility timeout: how long a claimed run may go
	// without a heartbeat before it is redelivered. The runner heartbeats well
	// within this window, so it bounds detection of a dead worker, not the
	// maximum job duration.
	AckWaitSeconds int `mapstructure:"ack_wait_seconds"`
	// ProgressIntervalSeconds is how often the progress of a running run is
	// persisted. Defaults to 10.
	ProgressIntervalSeconds int               `mapstructure:"progress_interval_seconds"`
	StatusDB                revadcfg.Database `mapstructure:"status_db"`
	// OnDemand holds the configuration of the on-demand jobs, keyed by job
	// name. Each entry is the job's own config section and is handed to the
	// job's constructor when a run is dispatched. Job names contain dots, so
	// the name must be quoted in the table header, e.g.
	// [serverless.services.jobs.on_demand."example.pingpong"].
	OnDemand map[string]map[string]any `mapstructure:"on_demand"`
	// MaxConcurrent caps, per job name, how many runs of a job this process
	// executes at once, e.g. max_concurrent = { "transfer.user" = 6 }.
	MaxConcurrent map[string]int `mapstructure:"max_concurrent"`
	// PeriodicReserve is the number of workers kept for leader periodic jobs,
	// which on-demand jobs never take. Defaults to 1.
	PeriodicReserve *int `mapstructure:"periodic_reserve"`
	// RunRetentionDays is how long finished runs are kept in the status DB.
	// Defaults to 30; a negative value keeps them forever.
	RunRetentionDays int `mapstructure:"run_retention_days"`
}

func (c *config) ApplyDefaults() {
	if c.WorkerPoolSize == 0 {
		c.WorkerPoolSize = 4
	}
	if c.NatsPrefix == "" {
		c.NatsPrefix = "reva-jobs"
	}
	if c.AckWaitSeconds == 0 {
		c.AckWaitSeconds = 60
	}
	if c.RunRetentionDays == 0 {
		c.RunRetentionDays = 30
	}
	if c.PeriodicReserve == nil {
		reserve := 1
		c.PeriodicReserve = &reserve
	}
	c.StatusDB = sharedconf.GetDBInfo(c.StatusDB)
}

// validateCaps makes sure the per-job caps and the periodic reserve fit in the
// worker pool.
func (c *config) validateCaps() error {
	if *c.PeriodicReserve < 0 {
		return fmt.Errorf("jobs: periodic_reserve must not be negative")
	}
	sum := *c.PeriodicReserve
	for job, n := range c.MaxConcurrent {
		if n <= 0 {
			return fmt.Errorf("jobs: max_concurrent of %q must be positive", job)
		}
		sum += n
	}
	if sum > c.WorkerPoolSize {
		return fmt.Errorf("jobs: max_concurrent plus periodic_reserve (%d) exceed worker_pool_size (%d)", sum, c.WorkerPoolSize)
	}
	return nil
}

type svc struct {
	conf   *config
	ctx    context.Context
	log    *zerolog.Logger
	runner *rjobs.Runner
	set    *invoke.Set
	// draining is set while the node is out of rotation; the runner then
	// claims no new runs.
	draining atomic.Bool
}

// New returns a new jobs service.
func New(ctx context.Context, m map[string]any) (rserverless.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	if err := c.validateCaps(); err != nil {
		return nil, err
	}

	s := &svc{
		conf: &c,
		ctx:  ctx,
		log:  appctx.GetLogger(ctx),
	}
	// Build the admin-facing invocation set now (its handlers read s.runner
	// lazily, once Start has built it), so the control channel advertises the
	// jobs operations from the first heartbeat.
	s.set = s.buildInvokeSet()
	return s, nil
}

// Start builds the runner and starts it. A missing NATS address is not an
// error: the runner then runs only all-nodes periodic jobs, which is useful
// for single-node setups that only warm local caches.
func (s *svc) Start() {
	opts := rjobs.Options{
		Workers:          s.conf.WorkerPoolSize,
		OnDemandConfig:   s.conf.OnDemand,
		ProgressInterval: time.Duration(s.conf.ProgressIntervalSeconds) * time.Second,
		MaxConcurrent:    s.conf.MaxConcurrent,
		PeriodicReserve:  *s.conf.PeriodicReserve,
	}

	// the durable queue and the status store go together: on-demand and
	// leader-scoped jobs need both. If either is unavailable we run only the
	// all-nodes jobs, which need neither.
	if s.conf.NatsAddress != "" {
		status, err := sqlstatus.New(s.ctx, s.conf.StatusDB)
		if err != nil {
			s.log.Error().Err(err).Msg("jobs: opening the status store failed, leader and on-demand jobs disabled")
		} else {
			store, err := natsstore.New(s.ctx, natsstore.Options{
				Address: s.conf.NatsAddress,
				Token:   s.conf.NatsToken,
				Prefix:  s.conf.NatsPrefix,
				AckWait: time.Duration(s.conf.AckWaitSeconds) * time.Second,
				Jobs:    s.queueJobs(),
			})
			if err != nil {
				s.log.Error().Err(err).Msg("jobs: connecting to the queue failed, leader and on-demand jobs disabled")
				_ = status.Close(s.ctx)
			} else {
				opts.Store = store
				opts.Status = status
				// only with a queue: a leader job without one fails the runner.
				s.registerRetention()
			}
		}
	} else {
		s.log.Warn().Msg("jobs: no nats_address configured, only all-nodes jobs will run")
	}

	runner, err := rjobs.NewRunner(s.ctx, opts)
	if err != nil {
		s.log.Error().Err(err).Msg("jobs: building the runner failed")
		return
	}

	if s.draining.Load() {
		runner.Pause()
	}
	s.runner = runner
	rjobs.SetDefault(runner)
	runner.Start()
	s.log.Info().Msg("jobs service ready")
}

// queueJobs returns the jobs the queue store consumes: the registered ones,
// plus the retention job, which is registered only once the queue is up.
func (s *svc) queueJobs() []rjobs.QueueJob {
	jobs := rjobs.RegisteredQueueJobs()
	if s.conf.RunRetentionDays < 0 {
		return jobs
	}
	for _, j := range jobs {
		if j.Name == retentionJob {
			return jobs
		}
	}
	return append(jobs, rjobs.QueueJob{Name: retentionJob, Leader: true})
}

// retentionJob is the leader job that prunes old runs from the status store.
const retentionJob = "rjobs.retention"

var retentionOnce sync.Once

// registerRetention registers the job that deletes finished runs older than
// run_retention_days. It registers at most once per process.
func (s *svc) registerRetention() {
	if s.conf.RunRetentionDays < 0 {
		return
	}
	retention := time.Duration(s.conf.RunRetentionDays) * 24 * time.Hour
	retentionOnce.Do(func() {
		err := rjobs.RegisterPeriodic(rjobs.Periodic{
			Name:     retentionJob,
			Schedule: "@daily",
			Scope:    rjobs.ScopeLeader,
			Run: func(ctx context.Context) error {
				if s.runner == nil {
					return nil
				}
				n, err := s.runner.PruneRuns(ctx, time.Now().Add(-retention))
				appctx.GetLogger(ctx).Info().Int("deleted", n).Msg("jobs: pruned finished runs")
				return err
			},
		})
		if err != nil {
			s.log.Error().Err(err).Msg("jobs: registering the retention job failed")
		}
	})
}

// SetDraining pauses the runner while the node is drained, so it stops claiming
// runs and hands off the running jobs that can move, and resumes it after.
func (s *svc) SetDraining(draining bool) {
	s.draining.Store(draining)
	if s.runner == nil {
		return
	}
	if draining {
		s.runner.Pause()
	} else {
		s.runner.Resume()
	}
}

// Close stops the runner, draining in-flight work within ctx.
func (s *svc) Close(ctx context.Context) error {
	if s.runner == nil {
		return nil
	}
	return s.runner.Stop(ctx)
}
