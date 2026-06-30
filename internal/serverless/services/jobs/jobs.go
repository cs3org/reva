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
	"time"

	revadcfg "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	natsstore "github.com/cs3org/reva/v3/pkg/rjobs/store/nats"
	sqlstatus "github.com/cs3org/reva/v3/pkg/rjobs/store/sql"
	"github.com/cs3org/reva/v3/pkg/rserverless"
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
	AckWaitSeconds int               `mapstructure:"ack_wait_seconds"`
	StatusDB       revadcfg.Database `mapstructure:"status_db"`
	// OnDemand holds the configuration of the on-demand jobs, keyed by job
	// name. Each entry is the job's own config section and is handed to the
	// job's constructor when a run is dispatched. Job names contain dots, so
	// the name must be quoted in the table header, e.g.
	// [serverless.services.jobs.on_demand."example.pingpong"].
	OnDemand map[string]map[string]any `mapstructure:"on_demand"`
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
}

type svc struct {
	conf   *config
	ctx    context.Context
	log    *zerolog.Logger
	runner *rjobs.Runner
}

// New returns a new jobs service.
func New(ctx context.Context, m map[string]any) (rserverless.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	return &svc{
		conf: &c,
		ctx:  ctx,
		log:  appctx.GetLogger(ctx),
	}, nil
}

// Start builds the runner and starts it. A missing NATS address is not an
// error: the runner then runs only all-nodes periodic jobs, which is useful
// for single-node setups that only warm local caches.
func (s *svc) Start() {
	opts := rjobs.Options{
		Workers:        s.conf.WorkerPoolSize,
		OnDemandConfig: s.conf.OnDemand,
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
				Jobs:    rjobs.RegisteredQueueJobNames(),
			})
			if err != nil {
				s.log.Error().Err(err).Msg("jobs: connecting to the queue failed, leader and on-demand jobs disabled")
				_ = status.Close(s.ctx)
			} else {
				opts.Store = store
				opts.Status = status
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

	s.runner = runner
	rjobs.SetDefault(runner)
	runner.Start()
	s.log.Info().Msg("jobs service ready")
}

// Close stops the runner, draining in-flight work within ctx.
func (s *svc) Close(ctx context.Context) error {
	if s.runner == nil {
		return nil
	}
	return s.runner.Stop(ctx)
}
