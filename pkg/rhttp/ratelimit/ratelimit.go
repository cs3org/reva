// Copyright 2018-2025 CERN
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

// Package ratelimit provides a per-client token-bucket HTTP middleware, meant
// for unauthenticated endpoints that trigger outbound requests or other
// expensive work (e.g. OCM provider discovery).
package ratelimit

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"golang.org/x/time/rate"
)

const (
	defaultMaxClients = 10000
	defaultTTL        = 10 * time.Minute
	// overflowFactor scales the shared bucket used once the client table is
	// full, relative to the limit of a single client.
	overflowFactor = 10
)

// Config configures a Limiter.
type Config struct {
	// RequestsPerMinute is the sustained rate allowed per client. Must be > 0.
	RequestsPerMinute int
	// Burst is how many requests a client may send back-to-back. Must be > 0.
	Burst int
	// TrustedProxyCIDRs lists the proxies whose X-Forwarded-For header is
	// believed. Empty means the header is ignored and the TCP peer address is
	// used. Any invalid entry makes New fail.
	TrustedProxyCIDRs []string
	// MaxClients bounds the number of tracked clients (default 10000).
	MaxClients int
	// TTL is how long an idle client is remembered (default 10 minutes).
	TTL time.Duration
}

type entry struct {
	lim  *rate.Limiter
	seen time.Time
}

// Limiter rate-limits HTTP requests per client address.
type Limiter struct {
	limit      rate.Limit
	burst      int
	trusted    []*net.IPNet
	maxClients int
	ttl        time.Duration

	mu        sync.Mutex
	clients   map[string]*entry
	overflow  *rate.Limiter
	lastSweep time.Time

	stop     chan struct{}
	stopOnce sync.Once
}

// New validates the configuration and returns a running Limiter. Call Close
// to stop its background cleanup.
func New(c Config) (*Limiter, error) {
	if c.RequestsPerMinute <= 0 {
		return nil, fmt.Errorf("ratelimit: requests per minute must be positive, got %d", c.RequestsPerMinute)
	}
	if c.Burst <= 0 {
		return nil, fmt.Errorf("ratelimit: burst must be positive, got %d", c.Burst)
	}
	trusted := make([]*net.IPNet, 0, len(c.TrustedProxyCIDRs))
	for _, s := range c.TrustedProxyCIDRs {
		_, n, err := net.ParseCIDR(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("ratelimit: invalid trusted proxy CIDR %q: %w", s, err)
		}
		trusted = append(trusted, n)
	}
	if c.MaxClients <= 0 {
		c.MaxClients = defaultMaxClients
	}
	if c.TTL <= 0 {
		c.TTL = defaultTTL
	}

	limit := rate.Limit(float64(c.RequestsPerMinute) / 60)
	l := &Limiter{
		limit:      limit,
		burst:      c.Burst,
		trusted:    trusted,
		maxClients: c.MaxClients,
		ttl:        c.TTL,
		clients:    make(map[string]*entry),
		overflow:   rate.NewLimiter(limit*overflowFactor, c.Burst*overflowFactor),
		stop:       make(chan struct{}),
	}
	go l.janitor()
	return l, nil
}

// Close stops the background cleanup. It is safe to call on a nil Limiter and
// more than once.
func (l *Limiter) Close() {
	if l == nil {
		return
	}
	l.stopOnce.Do(func() { close(l.stop) })
}

// Middleware rejects requests over the limit with 429 and a Retry-After
// header. On a nil Limiter it returns next unchanged, so a disabled limiter
// needs no special-casing at the call site.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		key := l.clientKey(r)
		res := l.limiterFor(key, now).ReserveN(now, 1)
		if delay := res.DelayFrom(now); !res.OK() || delay > 0 {
			res.CancelAt(now)
			secs := int(math.Ceil(delay.Seconds()))
			if !res.OK() || secs < 1 {
				secs = 1
			}
			// Debug only: under a flood this fires on every request.
			appctx.GetLogger(r.Context()).Debug().Str("client", key).Str("path", r.URL.Path).
				Int("retry_after", secs).Msg("rate limited unauthenticated request")
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"code":"RATE_LIMITED","message":"Too many requests"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) limiterFor(key string, now time.Time) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	if e, ok := l.clients[key]; ok {
		e.seen = now
		return e.lim
	}
	if len(l.clients) >= l.maxClients {
		// Sweep at most once a second so a flood of new keys cannot turn
		// every request into an O(n) scan.
		if now.Sub(l.lastSweep) >= time.Second {
			l.sweep(now)
		}
		if len(l.clients) >= l.maxClients {
			// Table still full: unknown clients share one bucket, which
			// keeps memory bounded without letting the flood evict
			// well-behaved clients that are already tracked.
			return l.overflow
		}
	}
	e := &entry{lim: rate.NewLimiter(l.limit, l.burst), seen: now}
	l.clients[key] = e
	return e.lim
}

// sweep drops idle clients. The caller must hold l.mu.
func (l *Limiter) sweep(now time.Time) {
	l.lastSweep = now
	for k, e := range l.clients {
		if now.Sub(e.seen) > l.ttl {
			delete(l.clients, k)
		}
	}
}

func (l *Limiter) janitor() {
	t := time.NewTicker(l.ttl / 2)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case now := <-t.C:
			l.mu.Lock()
			l.sweep(now)
			l.mu.Unlock()
		}
	}
}

// clientKey returns the bucket key for a request: the client IP, taken from
// X-Forwarded-For only when the TCP peer is a trusted proxy.
func (l *Limiter) clientKey(r *http.Request) string {
	ip := peerIP(r.RemoteAddr)
	if ip == nil {
		return r.RemoteAddr
	}
	if l.isTrusted(ip) {
		// Walk the header right to left, skipping trusted proxies: the first
		// untrusted hop is the client. Anything to its left is attacker
		// controlled and ignored. An unparsable hop stops the walk and the
		// request is keyed on the proxy, which fails towards stricter limiting.
		// Some proxies append the client port, so hops are parsed like
		// RemoteAddr.
		hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			h := peerIP(strings.TrimSpace(hops[i]))
			if h == nil {
				break
			}
			if !l.isTrusted(h) {
				ip = h
				break
			}
		}
	}
	return normalize(ip)
}

func (l *Limiter) isTrusted(ip net.IP) bool {
	for _, n := range l.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func peerIP(addr string) net.IP {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return net.ParseIP(host)
}

// normalize maps IPv4 (including v4-mapped v6) to its address and IPv6 to its
// /64, since a single host typically controls a whole /64 and would otherwise
// get a fresh bucket per address.
func normalize(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String()
}
