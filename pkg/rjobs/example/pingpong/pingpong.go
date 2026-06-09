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

// Package pingpong is a minimal example of an on-demand rjobs job. It takes a
// "ping" message as a parameter and logs the corresponding "pong", showing how
// a registered job receives its per-run parameters. It is meant as a reference
// for writing real jobs, not as production functionality.
package pingpong

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// JobName is the name the job is registered under and the name callers pass to
// Enqueue.
const JobName = "example.pingpong"

func init() {
	if err := rjobs.RegisterOnDemand(JobName, New); err != nil {
		panic(err)
	}
}

// job is the on-demand ping-pong job.
type job struct{}

// New builds the ping-pong job. The example is self-contained, so it ignores
// its configuration map.
func New(ctx context.Context, m map[string]any) (rjobs.Job, error) {
	return &job{}, nil
}

// params are the per-run parameters supplied at Enqueue time.
type params struct {
	// Ping is the message to pong back. Required.
	Ping string `mapstructure:"ping"`
}

// Run logs a pong for the received ping. A real job would do its work here and
// return an error to have the run retried.
func (j *job) Run(ctx context.Context, p rjobs.Params) error {
	var pp params
	if err := mapstructure.Decode(map[string]any(p), &pp); err != nil {
		return errors.Wrap(err, "pingpong: decoding params failed")
	}
	if pp.Ping == "" {
		return errors.New("pingpong: missing 'ping' parameter")
	}

	appctx.GetLogger(ctx).Info().
		Str("ping", pp.Ping).
		Str("pong", "pong: "+pp.Ping).
		Msg("pingpong: received ping, responding with pong")

	return nil
}

// Enqueue is a convenience wrapper that submits a ping-pong run through the
// process-wide runner. It is what an HTTP handler (or any in-process caller)
// would use to trigger the job on a user request.
func Enqueue(ctx context.Context, ping string) (rjobs.RunID, error) {
	runner := rjobs.Default()
	if runner == nil {
		return "", errors.New("pingpong: jobs service is not enabled")
	}
	return runner.Enqueue(ctx, JobName, rjobs.Params{"ping": ping})
}
