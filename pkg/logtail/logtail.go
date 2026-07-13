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

// Package logtail keeps a bounded, in-memory ring of a process's recent log
// lines, with a live tap for followers. It is an io.Writer teed into the
// zerolog output, capturing each event's JSON without touching the real sink.
package logtail

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Entry is one buffered log line: the parsed, filterable fields plus the raw
// JSON.
type Entry struct {
	Time    time.Time
	Level   string
	Service string
	Message string
	JSON    []byte
}

// Filter selects entries by service, minimum level, lower time bound and raw
// substring, capped at Limit (0 = default).
type Filter struct {
	Service  string
	MinLevel string
	Grep     string
	Since    time.Time
	Limit    int
}

// defaultReadLimit caps a snapshot when the caller sets no Limit.
const defaultReadLimit = 200

// subscriber is one live follower; dropped counts entries shed while ch was
// full, surfaced as a synthetic notice.
type subscriber struct {
	ch      chan Entry
	filter  Filter
	dropped int
}

// Buffer is a bounded, concurrency-safe ring of recent log lines with a live
// tap. The zero value is not usable; build one with New.
type Buffer struct {
	mu   sync.Mutex
	cap  int
	ring []Entry // len == cap once filled; a circular buffer
	head int     // index of the next write
	size int     // number of valid entries (<= cap)
	subs map[*subscriber]struct{}
}

// New returns a Buffer holding up to capacity entries. A capacity <= 0 yields
// a disabled buffer (Write is a no-op).
func New(capacity int) *Buffer {
	if capacity <= 0 {
		return &Buffer{}
	}
	return &Buffer{
		cap:  capacity,
		ring: make([]Entry, capacity),
		subs: map[*subscriber]struct{}{},
	}
}

// Enabled reports whether the buffer retains anything (capacity > 0).
func (b *Buffer) Enabled() bool { return b != nil && b.cap > 0 }

// subChanSize bounds a follower's pending queue before entries are dropped.
const subChanSize = 512

// Write records one serialized log event. zerolog may reuse the slice, so the
// bytes are copied. It never errors — logging must not fail on the tap.
func (b *Buffer) Write(p []byte) (int, error) {
	if !b.Enabled() {
		return len(p), nil
	}
	raw := make([]byte, len(p))
	copy(raw, p)
	raw = trimNewline(raw)
	e := parse(raw)

	b.mu.Lock()
	b.ring[b.head] = e
	b.head = (b.head + 1) % b.cap
	if b.size < b.cap {
		b.size++
	}
	b.publish(e)
	b.mu.Unlock()
	return len(p), nil
}

// publish delivers e to matching subscribers, with b.mu held. Sends are
// non-blocking so a stalled follower never blocks Write.
func (b *Buffer) publish(e Entry) {
	for s := range b.subs {
		if !matches(e, s.filter) {
			continue
		}
		if s.dropped > 0 {
			select {
			case s.ch <- droppedNotice(s.dropped, e.Time):
				s.dropped = 0
			default:
			}
		}
		select {
		case s.ch <- e:
		default:
			s.dropped++
		}
	}
}

// Read returns the entries matching f, newest first. The bool reports whether
// more matched than were returned.
func (b *Buffer) Read(f Filter) ([]Entry, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.collectLocked(f)
}

// collectLocked is Read with b.mu held.
func (b *Buffer) collectLocked(f Filter) ([]Entry, bool) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultReadLimit
	}
	out := make([]Entry, 0, limit)
	truncated := false
	// The most recent write is at head-1; walk backwards from there.
	for i := 0; i < b.size; i++ {
		idx := (b.head - 1 - i + b.cap*2) % b.cap
		e := b.ring[idx]
		if !matches(e, f) {
			continue
		}
		if len(out) == limit {
			truncated = true
			break
		}
		out = append(out, e)
	}
	return out, truncated
}

// ReadAndSubscribe snapshots the matching backlog (oldest first) and registers
// a live subscriber under one lock, so no line is missed or duplicated in the
// gap. cancel must be called; it unsubscribes and closes live.
func (b *Buffer) ReadAndSubscribe(f Filter) (backlog []Entry, live <-chan Entry, cancel func()) {
	if !b.Enabled() {
		ch := make(chan Entry)
		close(ch)
		return nil, ch, func() {}
	}
	s := &subscriber{ch: make(chan Entry, subChanSize), filter: f}

	b.mu.Lock()
	newest, _ := b.collectLocked(f)
	b.subs[s] = struct{}{}
	b.mu.Unlock()

	// Reverse the newest-first snapshot to oldest-first for a natural replay.
	backlog = make([]Entry, len(newest))
	for i, e := range newest {
		backlog[len(newest)-1-i] = e
	}

	var once sync.Once
	cancel = func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, s)
			close(s.ch)
			b.mu.Unlock()
		})
	}
	return backlog, s.ch, cancel
}

// matches reports whether e passes the filter.
func matches(e Entry, f Filter) bool {
	if f.Service != "" && e.Service != f.Service {
		return false
	}
	if f.MinLevel != "" && levelRank(e.Level) < levelRank(f.MinLevel) {
		return false
	}
	if !f.Since.IsZero() && e.Time.Before(f.Since) {
		return false
	}
	if f.Grep != "" && !strings.Contains(string(e.JSON), f.Grep) {
		return false
	}
	return true
}

// levelRank orders zerolog levels; an unrecognized level ranks as info.
func levelRank(l string) int {
	switch strings.ToLower(l) {
	case "trace":
		return 0
	case "debug":
		return 1
	case "info":
		return 2
	case "warn":
		return 3
	case "error":
		return 4
	case "fatal":
		return 5
	case "panic":
		return 6
	default:
		return 2
	}
}

// rawLine is the subset of a zerolog JSON event logtail parses for filtering.
type rawLine struct {
	Level   string `json:"level"`
	Time    string `json:"time"`
	Message string `json:"message"`
	Service string `json:"service"`
}

// parse extracts the filterable fields from a JSON log line. Unparseable lines
// are still buffered, stamped with the current time.
func parse(raw []byte) Entry {
	var r rawLine
	_ = json.Unmarshal(raw, &r)
	t, err := time.Parse(time.RFC3339Nano, r.Time)
	if err != nil {
		t = time.Now()
	}
	return Entry{Time: t, Level: r.Level, Service: r.Service, Message: r.Message, JSON: raw}
}

// droppedNotice is the synthetic warn entry a follower receives after n lines
// were shed, so the gap is visible.
func droppedNotice(n int, at time.Time) Entry {
	msg := fmt.Sprintf("logtail: %d log line(s) dropped (follower too slow)", n)
	j, _ := json.Marshal(rawLine{Level: "warn", Time: at.Format(time.RFC3339Nano), Message: msg})
	return Entry{Time: at, Level: "warn", Message: msg, JSON: j}
}

func trimNewline(p []byte) []byte {
	for len(p) > 0 && (p[len(p)-1] == '\n' || p[len(p)-1] == '\r') {
		p = p[:len(p)-1]
	}
	return p
}

// The process-wide default buffer, installed by the logger at startup.
var (
	defMu  sync.RWMutex
	defBuf = New(0) // disabled until the logger installs a real one
)

// SetDefault installs the process-wide buffer.
func SetDefault(b *Buffer) {
	defMu.Lock()
	defBuf = b
	defMu.Unlock()
}

// Default returns the process-wide buffer (a disabled one until SetDefault runs).
func Default() *Buffer {
	defMu.RLock()
	defer defMu.RUnlock()
	return defBuf
}
