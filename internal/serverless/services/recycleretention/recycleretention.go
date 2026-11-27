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

// Package recycleretention removes the old items from the CephFS recycle bins
// of the cephmount driver and the Samba VFS module recycleplus. Both move a
// deleted item to <bin>/<space>/YYYY/MM/DD/<path>, with the date in UTC. Nothing
// else removes the items, thus this service removes the day folders that are
// older than the retention period.
//
// The service registers a periodic job in the jobs service of the same process.
// The job runs on each node that has the service, so enable it on one node only:
//
//	[serverless.services.jobs]
//
//	[serverless.services.recycleretention]
//	bins = ["/mnt/doylegen01/project/*/.recycle"]
//	retention_days = 92
package recycleretention

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/rserverless"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

const jobName = "recycleretention.purge"

func init() {
	rserverless.Register("recycleretention", New)
}

type config struct {
	// Bins are glob patterns of the recycle bins, for example
	// /mnt/doylegen01/project/*/.recycle.
	Bins []string `mapstructure:"bins" validate:"required,min=1"`
	// RetentionDays is how many days a deleted item is kept at least. There is
	// no default, because the job removes data.
	RetentionDays int `mapstructure:"retention_days" validate:"required,gt=0"`
	// Schedule is how often the job runs, for example @daily or @every 12h.
	Schedule string `mapstructure:"schedule"`
	// DryRun only logs the day folders that the job would remove.
	DryRun bool `mapstructure:"dry_run"`
}

func (c *config) ApplyDefaults() {
	if c.Schedule == "" {
		c.Schedule = "@daily"
	}
}

type svc struct {
	conf *config
}

// New returns a new recycleretention service.
func New(ctx context.Context, m map[string]any) (rserverless.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	for _, p := range c.Bins {
		if _, err := filepath.Match(p, ""); err != nil {
			return nil, fmt.Errorf("recycleretention: bin pattern %q: %w", p, err)
		}
	}

	s := &svc{conf: &c}
	if err := rjobs.RegisterPeriodic(rjobs.Periodic{
		Name:     jobName,
		Schedule: c.Schedule,
		Scope:    rjobs.ScopeAllNodes,
		// The ticker starts again with the process: without a run at start, a
		// daily job never runs if revad restarts more often.
		RunOnStart: true,
		Run: func(ctx context.Context) error {
			return s.purge(ctx, time.Now())
		},
	}); err != nil {
		return nil, err
	}
	return s, nil
}

// Start does nothing, the jobs service runs the job.
func (s *svc) Start() {}

// Close does nothing, the jobs service stops the job.
func (s *svc) Close(ctx context.Context) error {
	return nil
}

// purge removes the day folders before the cutoff, the start of the day (UTC)
// retention_days before now, in every bin.
func (s *svc) purge(ctx context.Context, now time.Time) error {
	log := appctx.GetLogger(ctx)
	now = now.UTC()
	cutoff := time.Date(now.Year(), now.Month(), now.Day()-s.conf.RetentionDays, 0, 0, 0, 0, time.UTC)

	var errs []error
	for _, pattern := range s.conf.Bins {
		// New checked the pattern, thus Glob cannot fail.
		bins, _ := filepath.Glob(pattern)
		if len(bins) == 0 {
			log.Warn().Str("pattern", pattern).Msg("recycleretention: no bin matches")
		}
		for _, bin := range bins {
			errs = append(errs, s.purgeBin(ctx, bin, cutoff))
		}
	}
	return errors.Join(errs...)
}

// purgeBin removes the day folders of bin before cutoff, then the month and
// year folders that are empty and over. Users put symlinks in the bin when they
// delete them, and the job runs as root, thus it follows no symlink: os.Root
// keeps all calls in the bin, fs.WalkDir does not follow symlinks, and
// RemoveAll removes a symlink, not its target.
func (s *svc) purgeBin(ctx context.Context, bin string, cutoff time.Time) error {
	log := appctx.GetLogger(ctx)
	// os.OpenRoot follows a symlink, thus check the bin first.
	info, err := os.Lstat(bin)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("recycleretention: %s is not a folder", bin)
	}
	root, err := os.OpenRoot(bin)
	if err != nil {
		return err
	}
	defer root.Close()

	var errs []error
	var months, years []string
	err = fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if name == "." || !d.IsDir() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		_, date, _ := strings.Cut(name, "/")
		switch strings.Count(name, "/") {
		case 0: // the bin of a space
			return nil
		case 1:
			year, err := time.Parse("2006", date)
			if err != nil {
				return fs.SkipDir
			}
			if !year.AddDate(1, 0, 0).After(cutoff) {
				years = append(years, name)
			}
			return nil
		case 2:
			month, err := time.Parse("2006/01", date)
			if err != nil {
				return fs.SkipDir
			}
			if !month.AddDate(0, 1, 0).After(cutoff) {
				months = append(months, name)
			}
			return nil
		}

		// The content of a day folder is never read.
		day, err := time.Parse("2006/01/02", date)
		if err != nil || !day.Before(cutoff) {
			return fs.SkipDir
		}
		path := filepath.Join(bin, name)
		if s.conf.DryRun {
			log.Info().Str("folder", path).Msg("recycleretention: dry run, not removed")
			return fs.SkipDir
		}
		if err := root.RemoveAll(name); err != nil {
			errs = append(errs, fmt.Errorf("recycleretention: removing %s: %w", path, err))
			return fs.SkipDir
		}
		log.Info().Str("folder", path).Msg("recycleretention: removed")
		return fs.SkipDir
	})
	if err != nil {
		errs = append(errs, err)
	}

	// Samba and Reva can still create today's folder in a month or a year that
	// is not over, thus only the ones that are over are removed.
	if !s.conf.DryRun {
		for _, name := range append(months, years...) {
			// fails if the folder is not empty
			_ = root.Remove(name)
		}
	}
	return errors.Join(errs...)
}
