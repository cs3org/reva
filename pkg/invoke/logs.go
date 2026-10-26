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

package invoke

import (
	"context"
	"errors"
	"time"

	"github.com/cs3org/reva/v3/pkg/logtail"
)

// LogsInvocation is a built-in invocation every service instance exposes: the
// process's recent log lines, filtered to the instance's service. Invoke
// returns a snapshot, InvokeStream follows.
const LogsInvocation = "logs"

type logsArgs struct {
	Level   string    `arg:"level" enum:"trace,debug,info,warn,error" help:"minimum level"`
	Since   time.Time `arg:"since" help:"only lines newer than a duration (e.g. 5m) or an RFC3339 time"`
	Limit   int       `arg:"limit" help:"max lines for a snapshot, or backlog before a follow"`
	Grep    string    `arg:"grep" help:"keep only lines containing this substring"`
	Job     string    `arg:"job" help:"keep only lines of this background job"`
	All     bool      `arg:"all" help:"the whole process, not just this service (true|false)"`
	Service *string   `arg:"service" help:"only this service, not the instance's own"`
}

func init() {
	registerDefault(logsSpec(), logsSnapshot, logsStream)
}

// errLogBufferDisabled makes a disabled buffer ([log] tail = 0) report a clear
// reason rather than an empty result.
var errLogBufferDisabled = errors.New("log buffer disabled on this node (set [log] tail > 0)")

// logsSpec is the catalog entry for the built-in logs invocation.
func logsSpec() InvocationSpec {
	return InvocationSpec{
		Name:        LogsInvocation,
		Description: "Return recent log lines from this process (snapshot), or follow them (stream).",
		Kind:        KindReadonly,
		Streaming:   true,
	}
}

// logsSnapshot serves the unary logs invocation: matching lines, newest first.
func logsSnapshot(_ context.Context, inst instance, args logsArgs) (Result, error) {
	buf := logtail.Default()
	if !buf.Enabled() {
		return nil, errLogBufferDisabled
	}
	entries, truncated := buf.Read(logFilter(inst, args))
	out := make([]map[string]any, len(entries))
	for i, e := range entries {
		out[i] = logEntryMap(e)
	}
	return Result{"entries": out, "truncated": truncated}, nil
}

// logsStream serves the streaming logs invocation: the matching backlog, then
// new lines until the consumer goes away.
func logsStream(ctx context.Context, inst instance, args logsArgs, emit StreamEmit) error {
	buf := logtail.Default()
	if !buf.Enabled() {
		return errLogBufferDisabled
	}
	backlog, live, cancel := buf.ReadAndSubscribe(logFilter(inst, args))
	defer cancel()
	for _, e := range backlog {
		if err := emit(Result(logEntryMap(e))); err != nil {
			return err
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case e, ok := <-live:
			if !ok {
				return nil
			}
			if err := emit(Result(logEntryMap(e))); err != nil {
				return err
			}
		}
	}
}

// logFilter builds a logtail.Filter from the arguments. The service defaults
// to the instance's own unless all=true or an explicit override is given.
func logFilter(inst instance, a logsArgs) logtail.Filter {
	service := inst.service
	if a.All {
		service = ""
	}
	if a.Service != nil {
		service = *a.Service
	}
	return logtail.Filter{
		Service:  service,
		Job:      a.Job,
		MinLevel: a.Level,
		Grep:     a.Grep,
		Limit:    a.Limit,
		Since:    a.Since,
	}
}

// logEntryMap renders a buffered entry as a serializable result.
func logEntryMap(e logtail.Entry) map[string]any {
	return map[string]any{
		"time":    e.Time.Format(time.RFC3339Nano),
		"level":   e.Level,
		"service": e.Service,
		"message": e.Message,
		"raw":     string(e.JSON),
	}
}
