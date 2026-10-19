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

var (
	logsLevel   = OptArg[string]("level", "minimum level: trace|debug|info|warn|error")
	logsSince   = OptArg[time.Time]("since", "only lines newer than a duration (e.g. 5m) or an RFC3339 time")
	logsLimit   = OptArg[int]("limit", "max lines for a snapshot, or backlog before a follow")
	logsGrep    = OptArg[string]("grep", "keep only lines containing this substring")
	logsAll     = OptArg[bool]("all", "the whole process, not just this service (true|false)")
	logsService = OptArg[string]("service", "only this service, not the instance's own")
)

func init() {
	registerDefault(defaultInvocation{
		spec:   logsSpec(),
		args:   []argDecl{logsLevel, logsSince, logsLimit, logsGrep, logsAll, logsService},
		fn:     logsSnapshot,
		stream: logsStream,
	})
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
func logsSnapshot(_ context.Context, inst instance, args Args) (Result, error) {
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
func logsStream(ctx context.Context, inst instance, args Args, emit StreamEmit) error {
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
func logFilter(inst instance, a Args) logtail.Filter {
	service := inst.service
	if logsAll.Get(a) {
		service = ""
	}
	if logsService.Has(a) {
		service = logsService.Get(a)
	}
	return logtail.Filter{
		Service:  service,
		MinLevel: logsLevel.Get(a),
		Grep:     logsGrep.Get(a),
		Limit:    logsLimit.Get(a),
		Since:    logsSince.Get(a),
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
