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

// Package activity tracks one service instance's request activity: how many
// requests are in flight right now, how many have been served, and when the last
// one started. A Counter is created per service per server in the runtime and
// shared by reference between the request-handling choke points that feed it
// (the gRPC appctx interceptor, the HTTP router) and the built-in `activity`
// invocation that reports it — no process-global state, and one counter per
// listener means the granularity matches a node id (per instance). The signal
// backs the "is this instance still serving requests?" question during a drain
// and restart: in-flight zero, plus an idle window, means quiesced.
//
// This is a leaf with no reva dependencies, so every layer that touches a
// Counter can do so without an import cycle. All methods are nil-safe: a nil
// *Counter (a service with no counter wired, e.g. serverless) reads as idle and
// silently drops writes.
package activity

import (
	"sync"
	"sync/atomic"
	"time"
)

// Stat is a point-in-time view of one service's request activity. Methods holds
// the per-RPC-method breakdown on the aggregate snapshot (nil on a per-method
// entry).
type Stat struct {
	InFlight    int64     // requests currently being served
	Total       int64     // requests served since the process started
	LastRequest time.Time // start of the most recent request (zero if none)
	Methods     map[string]Stat
}

// hit accumulates one stream of requests (a whole service, or one method).
type hit struct {
	inFlight    atomic.Int64
	total       atomic.Int64
	lastRequest atomic.Int64 // unix nanos of the last request start, 0 if none
}

func (h *hit) enter() {
	h.inFlight.Add(1)
	h.total.Add(1)
	h.lastRequest.Store(time.Now().UnixNano())
}

func (h *hit) exit() { h.inFlight.Add(-1) }

func (h *hit) stat() Stat {
	s := Stat{InFlight: h.inFlight.Load(), Total: h.total.Load()}
	if ns := h.lastRequest.Load(); ns > 0 {
		s.LastRequest = time.Unix(0, ns)
	}
	return s
}

// Counter accumulates one service instance's request activity: an aggregate
// (lock-free atomics, so the quiescence check stays cheap) plus a lazily-built
// per-method breakdown.
type Counter struct {
	agg      hit
	mu       sync.RWMutex
	byMethod map[string]*hit
}

// New returns a fresh Counter.
func New() *Counter { return &Counter{} }

// Enter records the start of a request and returns the function to call when it
// finishes. method is the RPC method name for the per-method breakdown; an empty
// method (e.g. HTTP) counts toward the aggregate only. Typical use: `defer
// counter.Enter(m)()`.
func (c *Counter) Enter(method string) func() {
	if c == nil {
		return func() {}
	}
	c.agg.enter()
	if method == "" {
		return func() { c.agg.exit() }
	}
	m := c.methodHit(method)
	m.enter()
	return func() {
		c.agg.exit()
		m.exit()
	}
}

// methodHit returns the counter for one method, creating it on first use. The
// method set of a service is small and bounded, so the map saturates quickly and
// is read-path thereafter.
func (c *Counter) methodHit(method string) *hit {
	c.mu.RLock()
	h, ok := c.byMethod[method]
	c.mu.RUnlock()
	if ok {
		return h
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if h, ok = c.byMethod[method]; ok {
		return h
	}
	if c.byMethod == nil {
		c.byMethod = map[string]*hit{}
	}
	h = &hit{}
	c.byMethod[method] = h
	return h
}

// Snapshot returns the counter's current stats — aggregate plus per-method
// breakdown (a zero Stat for a nil counter or one that has served nothing).
func (c *Counter) Snapshot() Stat {
	if c == nil {
		return Stat{}
	}
	s := c.agg.stat()
	c.mu.RLock()
	if len(c.byMethod) > 0 {
		s.Methods = make(map[string]Stat, len(c.byMethod))
		for name, h := range c.byMethod {
			s.Methods[name] = h.stat()
		}
	}
	c.mu.RUnlock()
	return s
}
