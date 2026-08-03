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

// Package reconciliation hosts the share reconciliation jobs as a serverless
// service. The jobs span the share and the public link manager, which belong to
// two different grpc services, so they are wired here instead of in either of
// them.
package reconciliation

import (
	"context"
	"os"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/reconciliation"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/rserverless"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func init() {
	rserverless.Register("reconciliation", New)
}

type config struct {
	reconciliation.Config `mapstructure:",squash"`
	// Schedule is the interval the orphan job runs on, e.g. "@daily".
	Schedule string `mapstructure:"schedule" validate:"required"`
	// DB is the share database, the same block the sql share driver takes.
	// Unset fields fall back to [shared].
	DB map[string]any `mapstructure:"db"`
}

// ApplyDefaults defaults the embedded reconciliation config, which cfg.Decode
// only does for the struct it is given.
func (c *config) ApplyDefaults() {
	c.Config.ApplyDefaults()
}

type svc struct {
	log *zerolog.Logger
	// jobLog is the job's log file, nil when the job logs to a standard stream.
	jobLog *os.File
}

// New builds the reconciliation service. It registers the jobs right away
// rather than in Start, because every serverless service is constructed before
// any is started and the jobs runner reads the registered jobs when it starts.
//
// The stores are the gorm-backed sql managers: the jobs read and write the
// share tables directly, so unlike the grpc services this one has no driver
// indirection to offer.
func New(ctx context.Context, m map[string]any) (rserverless.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	sm, err := sql.NewShareManager(ctx, c.DB)
	if err != nil {
		return nil, errors.Wrap(err, "reconciliation: opening the share store")
	}
	pm, err := sql.NewPublicShareManager(ctx, c.DB)
	if err != nil {
		return nil, errors.Wrap(err, "reconciliation: opening the public link store")
	}
	gw, err := pool.GetGatewayServiceClient(pool.Endpoint(sharedconf.GetGatewaySVC("")))
	if err != nil {
		return nil, errors.Wrap(err, "reconciliation: getting the gateway client")
	}

	shares, ok := sm.(reconciliation.ShareStore)
	if !ok {
		return nil, errors.Errorf("reconciliation: share manager %T cannot be reconciled", sm)
	}
	links, ok := pm.(reconciliation.PublicLinkStore)
	if !ok {
		return nil, errors.Errorf("reconciliation: public link manager %T cannot be reconciled", pm)
	}

	jobLog, logFile, err := reconciliation.OpenLog(c.LogFile)
	if err != nil {
		return nil, err
	}

	job := &reconciliation.OrphanJob{
		Shares:  shares,
		Links:   links,
		Gateway: gw,
		Log:     jobLog,
		DryRun:  c.DryRun,
	}
	if err := rjobs.RegisterPeriodic(job.Periodic(c.Schedule)); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, errors.Wrap(err, "reconciliation: registering the orphan job")
	}

	log := appctx.GetLogger(ctx)
	log.Info().
		Str("schedule", c.Schedule).
		Str("log_file", c.LogFile).
		Bool("dry_run", c.DryRun).
		Msg("reconciliation: orphan job registered")

	return &svc{log: log, jobLog: logFile}, nil
}

// Start is a no-op: the jobs service owns the runner that fires the job.
func (s *svc) Start() {}

// Close closes the job's log. The runner is stopped by the jobs service, so no
// run is in flight by the time this is called.
func (s *svc) Close(ctx context.Context) error {
	if s.jobLog == nil {
		return nil
	}
	return s.jobLog.Close()
}
