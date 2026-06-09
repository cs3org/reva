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
// store and elector from configuration, constructs the runner over the
// registered jobs, and exposes it process-wide for in-process enqueueing.
package jobs

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	natsstore "github.com/cs3org/reva/v3/pkg/rjobs/store/nats"
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
}

func (c *config) ApplyDefaults() {
	if c.WorkerPoolSize == 0 {
		c.WorkerPoolSize = 4
	}
	if c.NatsPrefix == "" {
		c.NatsPrefix = "reva-jobs"
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
		Workers: s.conf.WorkerPoolSize,
	}

	if s.conf.NatsAddress != "" {
		store, err := natsstore.New(s.ctx, natsstore.Options{
			Address: s.conf.NatsAddress,
			Token:   s.conf.NatsToken,
			Prefix:  s.conf.NatsPrefix,
		})
		if err != nil {
			s.log.Error().Err(err).Msg("jobs: connecting to the store failed, leader and on-demand jobs disabled")
		} else {
			opts.Store = store
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
