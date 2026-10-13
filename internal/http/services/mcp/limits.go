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

package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/ratelimit"
)

// limits holds the per app password and per user budgets. Every limit applies
// with the same value to both, so a user does not get more by creating more
// app passwords.
type limits struct {
	buckets *ratelimit.Limiter

	mu    sync.Mutex
	reads map[string]int
}

func newLimits() *limits {
	return &limits{buckets: ratelimit.New(), reads: make(map[string]int)}
}

func limitKeys(kind string, ap *scope.AppAuthScope, userID string) []string {
	return []string{kind + ":password:" + ap.PasswordID, kind + ":user:" + userID}
}

// acquireRead takes a read_file slot for every key, or none.
func (l *limits) acquireRead(keys []string, max int) bool {
	if max <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		if l.reads[k] >= max {
			return false
		}
	}
	for _, k := range keys {
		l.reads[k]++
	}
	return true
}

func (l *limits) releaseRead(keys []string, max int) {
	if max <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		if l.reads[k]--; l.reads[k] <= 0 {
			delete(l.reads, k)
		}
	}
}

// serveLimited applies the call budgets to the tool calls of a request before
// the SDK sees it, so that a refusal is a 429 with Retry-After.
func (s *svc) serveLimited(w http.ResponseWriter, r *http.Request, ap *scope.AppAuthScope, next http.Handler) {
	if r.Method != http.MethodPost {
		next.ServeHTTP(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxBodyBytes()))
	if err != nil {
		code := http.StatusBadRequest
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			code = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), code)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	calls := toolCalls(body)
	if len(calls) == 0 {
		next.ServeHTTP(w, r)
		return
	}
	userID := appctx.ContextMustGetUser(r.Context()).GetId().GetOpaqueId()
	rl := s.c.RateLimit

	var writes, reads int
	for _, name := range calls {
		if t, ok := s.tools[name]; ok && t.write {
			writes++
		}
		if name == toolReadFile {
			reads++
		}
	}
	if ok, wait := s.limits.buckets.AllowN(limitKeys("calls", ap, userID), rl.CallsPerTokenPerMin, len(calls)); !ok {
		tooManyRequests(w, wait, "too many tool calls")
		return
	}
	if writes > 0 {
		if ok, wait := s.limits.buckets.AllowN(limitKeys("writes", ap, userID), rl.WriteCallsPerTokenPerMin, writes); !ok {
			tooManyRequests(w, wait, "too many changes")
			return
		}
	}
	if reads > 0 {
		keys := limitKeys("reads", ap, userID)
		acquired := 0
		for range reads {
			if !s.limits.acquireRead(keys, rl.ReadConcurrencyPerToken) {
				break
			}
			acquired++
		}
		defer func() {
			for range acquired {
				s.limits.releaseRead(keys, rl.ReadConcurrencyPerToken)
			}
		}()
		if acquired < reads {
			tooManyRequests(w, time.Second, "too many files read at once")
			return
		}
	}
	next.ServeHTTP(w, r)
}

func tooManyRequests(w http.ResponseWriter, wait time.Duration, msg string) {
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(max(wait, time.Second).Seconds()))))
	http.Error(w, msg, http.StatusTooManyRequests)
}

// toolCalls returns the tools a JSON-RPC message, or batch, calls. A body that
// does not parse is left for the SDK to refuse.
func toolCalls(body []byte) []string {
	type message struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	var msgs []message
	body = bytes.TrimSpace(body)
	if len(body) > 0 && body[0] == '[' {
		if json.Unmarshal(body, &msgs) != nil {
			return nil
		}
	} else {
		var m message
		if json.Unmarshal(body, &m) != nil {
			return nil
		}
		msgs = append(msgs, m)
	}
	var names []string
	for _, m := range msgs {
		if m.Method == "tools/call" {
			names = append(names, m.Params.Name)
		}
	}
	return names
}

// takeReadBytes charges n bytes to the read budget of the caller.
func (s *svc) takeReadBytes(c *call, n int64) error {
	perMin := s.c.RateLimit.ReadBytesPerTokenPerMin
	if n <= 0 || perMin <= 0 {
		return nil
	}
	// A bucket never holds more than a minute's worth.
	n = min(n, int64(perMin))
	ok, wait := s.limits.buckets.AllowN(limitKeys("bytes", c.ap, c.user.GetId().GetOpaqueId()), perMin, int(n))
	if !ok {
		return newError(codeRateLimited, fmt.Sprintf("the read budget is spent; retry in %d seconds, or read less", int(math.Ceil(wait.Seconds()))))
	}
	return nil
}
