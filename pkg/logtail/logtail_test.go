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

package logtail

import (
	"fmt"
	"testing"
	"time"
)

// writeLine feeds one synthetic zerolog-style JSON line to the buffer.
func writeLine(b *Buffer, level, service, msg string) {
	t := time.Now().UTC().Format(time.RFC3339Nano)
	line := fmt.Sprintf(`{"level":%q,"time":%q,"service":%q,"message":%q}`+"\n", level, t, service, msg)
	_, _ = b.Write([]byte(line))
}

func TestRingEvictsOldestNewestFirst(t *testing.T) {
	b := New(3)
	for i := range 5 {
		writeLine(b, "info", "svc", fmt.Sprintf("m%d", i))
	}
	got, truncated := b.Read(Filter{})
	if len(got) != 3 {
		t.Fatalf("want 3 retained, got %d", len(got))
	}
	if truncated {
		t.Fatalf("want not truncated within capacity")
	}
	// Newest first: m4, m3, m2 (m0, m1 evicted).
	want := []string{"m4", "m3", "m2"}
	for i, e := range got {
		if e.Message != want[i] {
			t.Fatalf("pos %d: want %q, got %q", i, want[i], e.Message)
		}
	}
}

func TestReadFiltersAndTruncation(t *testing.T) {
	b := New(100)
	writeLine(b, "info", "userprovider", "hello world")
	writeLine(b, "warn", "userprovider", "careful now")
	writeLine(b, "error", "groupprovider", "boom")

	// Service filter.
	if got, _ := b.Read(Filter{Service: "groupprovider"}); len(got) != 1 || got[0].Message != "boom" {
		t.Fatalf("service filter: got %+v", got)
	}
	// Minimum level: warn keeps warn+error, drops info.
	if got, _ := b.Read(Filter{MinLevel: "warn"}); len(got) != 2 {
		t.Fatalf("level filter: want 2, got %d", len(got))
	}
	// Grep over the raw line.
	if got, _ := b.Read(Filter{Grep: "world"}); len(got) != 1 || got[0].Message != "hello world" {
		t.Fatalf("grep filter: got %+v", got)
	}
	// Limit + truncation.
	got, truncated := b.Read(Filter{Limit: 2})
	if len(got) != 2 || !truncated {
		t.Fatalf("limit: want 2 truncated, got %d truncated=%v", len(got), truncated)
	}
}

func TestReadAndSubscribeBacklogThenLive(t *testing.T) {
	b := New(100)
	writeLine(b, "info", "svc", "old-1")
	writeLine(b, "info", "svc", "old-2")

	backlog, live, cancel := b.ReadAndSubscribe(Filter{Service: "svc"})
	defer cancel()

	// Backlog is oldest-first.
	if len(backlog) != 2 || backlog[0].Message != "old-1" || backlog[1].Message != "old-2" {
		t.Fatalf("backlog: got %+v", backlog)
	}

	writeLine(b, "info", "svc", "live-1")
	writeLine(b, "info", "other", "ignored") // filtered out for this subscriber
	writeLine(b, "info", "svc", "live-2")

	if e := recv(t, live); e.Message != "live-1" {
		t.Fatalf("live 1: got %q", e.Message)
	}
	if e := recv(t, live); e.Message != "live-2" {
		t.Fatalf("live 2 (other should be filtered): got %q", e.Message)
	}
}

func TestCancelClosesChannel(t *testing.T) {
	b := New(10)
	_, live, cancel := b.ReadAndSubscribe(Filter{})
	cancel()
	if _, ok := <-live; ok {
		t.Fatalf("want closed channel after cancel")
	}
	cancel() // idempotent, must not panic
}

func TestDisabledBuffer(t *testing.T) {
	b := New(0)
	if b.Enabled() {
		t.Fatalf("New(0) should be disabled")
	}
	writeLine(b, "info", "svc", "dropped")
	if got, _ := b.Read(Filter{}); len(got) != 0 {
		t.Fatalf("disabled buffer should retain nothing, got %d", len(got))
	}
	backlog, live, cancel := b.ReadAndSubscribe(Filter{})
	defer cancel()
	if len(backlog) != 0 {
		t.Fatalf("disabled backlog should be empty")
	}
	if _, ok := <-live; ok {
		t.Fatalf("disabled subscribe should return a closed channel")
	}
}

func recv(t *testing.T, ch <-chan Entry) Entry {
	t.Helper()
	select {
	case e, ok := <-ch:
		if !ok {
			t.Fatalf("channel closed unexpectedly")
		}
		return e
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for a live entry")
		return Entry{}
	}
}
