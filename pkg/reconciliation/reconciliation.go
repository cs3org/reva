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

// Package reconciliation reconciles the share database against the state of the
// storage. For now it holds a single job, orphan detection, which marks shares
// and public links whose resource or recipient no longer exists. It is
// storage-driver agnostic: it reads from the database and resolves resources
// and identities through the gateway (CS3).
//
// Every line a job logs carries an "event" naming what happened, so a run can
// be replayed or reverted by filtering on it rather than by parsing free-form
// messages. An event is the job's own name followed by the step: every job has
// start, skip, fail and end, plus one event for the change it makes. Each line
// also carries "job" and "run", the uuid of the run it belongs to, so jobs stay
// apart even when they log to the same file.
package reconciliation

import (
	"context"
	"os"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// ShareStore is what the jobs need from a share manager: the CS3 listing every
// manager implements, plus marking, which the CS3 API has no call for. A
// manager that cannot mark cannot be reconciled. The listing is expected to
// cover every owner and to leave out the already-orphaned.
type ShareStore interface {
	ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error)
	MarkAsOrphaned(ctx context.Context, ref *collaboration.ShareReference) error
	// Unshare removes the referenced share. The row is soft deleted, so a
	// removal can be undone in the database.
	Unshare(ctx context.Context, ref *collaboration.ShareReference) error
}

// PublicLinkStore is the same for a public share manager.
type PublicLinkStore interface {
	ListPublicShares(ctx context.Context, u *userpb.User, filters []*link.ListPublicSharesRequest_Filter, md *provider.ResourceInfo, sign bool) ([]*link.PublicShare, error)
	MarkAsOrphaned(ctx context.Context, ref *link.PublicShareReference) error
}

// Config configures one reconciliation job. Every job takes the same knobs and
// each is decoded from its own configuration section, so one can be scheduled,
// dry-run and logged without touching the others.
type Config struct {
	// Schedule is the interval the job runs on, e.g. "@daily".
	Schedule string `mapstructure:"schedule"`
	// DryRun, when set, makes the job log and report what it would do without
	// touching the share database.
	DryRun bool `mapstructure:"dry_run"`
	// RunOnStart fires the job once as soon as the runner starts, instead of
	// waiting a full interval for the first run.
	RunOnStart bool `mapstructure:"run_on_start"`
	// LogFile is the path the job writes its own log to. It takes "stdout" or
	// "stderr" to write to the standard streams instead.
	LogFile string `mapstructure:"log_file"`
}

// OpenLog opens a job's own log. A job keeps its own log rather than writing to
// revad's because the log is the record of what a run changed: it is always
// JSON, whatever [log] mode the rest of the process runs in, and it is not
// interleaved with unrelated output.
//
// The returned file is nil when the log goes to a standard stream, and is the
// caller's to close otherwise.
func OpenLog(path string) (*zerolog.Logger, *os.File, error) {
	switch path {
	case "stdout":
		log := zerolog.New(os.Stdout).With().Timestamp().Logger()
		return &log, nil, nil
	case "stderr":
		log := zerolog.New(os.Stderr).With().Timestamp().Logger()
		return &log, nil, nil
	}

	// the log is opened at startup so that a bad path is a startup error
	// rather than a run that silently keeps no record.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "reconciliation: opening the log at %s", path)
	}
	log := zerolog.New(f).With().Timestamp().Logger()
	return &log, f, nil
}
