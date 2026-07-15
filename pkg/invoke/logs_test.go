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
	"fmt"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/logtail"
)

func writeLogLine(b *logtail.Buffer, service, msg string) {
	t := time.Now().UTC().Format(time.RFC3339Nano)
	b.Write(fmt.Appendf(nil, `{"level":"info","time":%q,"service":%q,"message":%q}`+"\n", t, service, msg))
}

// TestLogsInvocationSnapshot checks the built-in logs invocation defaults to the
// instance's own service, and that all=true widens to the whole process.
func TestLogsInvocationSnapshot(t *testing.T) {
	buf := logtail.New(100)
	logtail.SetDefault(buf)
	defer logtail.SetDefault(logtail.New(0))

	writeLogLine(buf, "userprovider", "hello from up")
	writeLogLine(buf, "groupprovider", "hello from gp")

	id := "127.0.0.1:9500/userprovider"
	RegisterInstance(id, "userprovider", nil, nil)

	// The catalog advertises logs as a streaming-capable default.
	specs, _ := Invocations(id)
	if !hasSpec(specs, LogsInvocation) {
		t.Fatalf("expected %q in catalog: %+v", LogsInvocation, specs)
	}

	// Default: only this instance's service.
	res, err := Invoke(context.Background(), id, LogsInvocation, nil)
	if err != nil {
		t.Fatalf("Invoke(logs): %v", err)
	}
	entries := res["entries"].([]map[string]any)
	if len(entries) != 1 || entries[0]["service"] != "userprovider" {
		t.Fatalf("service-scoped snapshot wrong: %+v", entries)
	}

	// all=true: the whole process.
	res, err = Invoke(context.Background(), id, LogsInvocation, map[string]any{"all": "true"})
	if err != nil {
		t.Fatalf("Invoke(logs all): %v", err)
	}
	if got := len(res["entries"].([]map[string]any)); got != 2 {
		t.Fatalf("all=true should see both services, got %d", got)
	}
}

// TestLogsInvocationStream checks the streaming logs invocation replays the
// backlog then delivers live lines, filtered to the service.
func TestLogsInvocationStream(t *testing.T) {
	buf := logtail.New(100)
	logtail.SetDefault(buf)
	defer logtail.SetDefault(logtail.New(0))

	writeLogLine(buf, "authprovider", "backlog-1")

	id := "127.0.0.1:9600/authprovider"
	RegisterInstance(id, "authprovider", nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan string, 8)
	done := make(chan error, 1)
	go func() {
		done <- InvokeStream(ctx, id, LogsInvocation, nil, func(r Result) error {
			got <- r["message"].(string)
			return nil
		})
	}()

	if msg := <-got; msg != "backlog-1" {
		t.Fatalf("expected backlog first, got %q", msg)
	}
	writeLogLine(buf, "authprovider", "live-1")
	if msg := <-got; msg != "live-1" {
		t.Fatalf("expected live line, got %q", msg)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("InvokeStream did not return after cancel")
	}
}

// TestDefaultsRegistry checks the built-in defaults: catalog order follows
// registration order, and a non-streaming default can still be reached over
// InvokeStream (run once, one result).
func TestDefaultsRegistry(t *testing.T) {
	id := "127.0.0.1:9800/svc-y"
	RegisterInstance(id, "svc-y", nil, nil)

	specs, ok := Invocations(id)
	if !ok || len(specs) < 3 || specs[0].Name != ConfigInvocation || specs[1].Name != LogsInvocation || specs[2].Name != VersionInvocation {
		t.Fatalf("expected [config, logs, version] leading the catalog, got %+v", specs)
	}

	res, err := Invoke(context.Background(), id, VersionInvocation, nil)
	if err != nil {
		t.Fatalf("Invoke(version): %v", err)
	}
	if res["go"] == "" || res["uptime"] == "" {
		t.Fatalf("version result incomplete: %+v", res)
	}

	// A non-streaming default streams a single result.
	got := 0
	if serr := InvokeStream(context.Background(), id, ConfigInvocation, nil, func(Result) error { got++; return nil }); serr != nil || got != 1 {
		t.Fatalf("stream-invoking config: err=%v results=%d, want one result", serr, got)
	}
}

// TestLogsDisabledBuffer checks logs reports a clear error when no buffer is kept.
func TestLogsDisabledBuffer(t *testing.T) {
	logtail.SetDefault(logtail.New(0))
	id := "127.0.0.1:9700/svc-x"
	RegisterInstance(id, "svc-x", nil, nil)
	if _, err := Invoke(context.Background(), id, LogsInvocation, nil); err == nil {
		t.Fatal("expected an error when the log buffer is disabled")
	}
}
