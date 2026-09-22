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

// Package stats is the prometheus collector turning the fleet's service
// statistics into metrics. It needs no configuration: it discovers the
// services advertising a "stats" invocation through the service registry,
// invokes them over the control channel on a background refresh loop, and
// serves the cached results — a scrape never triggers an invocation.
//
// If an owner-attributes file is present (default
// /etc/revad/owner-attributes.json, a JSON object mapping owner ids to
// attribute key/value pairs), per-owner samples are folded by those
// attributes and the attribute keys become metric labels; without the
// file they collapse into plain totals. The attribute vocabulary is
// entirely the site's: reva only forwards it.
package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"github.com/cs3org/reva/v3/pkg/prom/registry"
	svcregistry "github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/stats"
	"github.com/cs3org/reva/v3/pkg/token"
	jwtmgr "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/metadata"
)

func init() {
	registry.Register("stats", New)
}

type config struct {
	Stats struct {
		// RefreshInterval is how often the fleet is queried (default 5m).
		RefreshInterval string `mapstructure:"refresh_interval"`
		// Namespace prefixes every metric name (default "reva").
		Namespace string `mapstructure:"namespace"`
		// OwnerAttributesFile overrides the conventional attributes path.
		OwnerAttributesFile string `mapstructure:"owner_attributes_file"`
	} `mapstructure:"stats"`
}

// DefaultOwnerAttributesFile is the conventional owner-attributes path.
const DefaultOwnerAttributesFile = "/etc/revad/owner-attributes.json"

// fleet abstracts discovery and invocation for testability.
type fleet interface {
	// AuthContext returns a context authorized to call the control
	// channel (the collector's own admin-scoped identity).
	AuthContext(ctx context.Context) (context.Context, error)
	// StatsTargets returns, per service advertising the stats invocation,
	// its live control endpoints.
	StatsTargets() map[string][]client.Endpoint
	// InvokeStats runs the stats invocation on one endpoint.
	InvokeStats(ctx context.Context, ep client.Endpoint) (*stats.Payload, error)
}

// New builds the collector and starts its refresh loop.
func New(ctx context.Context, m map[string]any) ([]prometheus.Collector, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	interval := 5 * time.Minute
	if c.Stats.RefreshInterval != "" {
		d, err := time.ParseDuration(c.Stats.RefreshInterval)
		if err != nil {
			return nil, fmt.Errorf("prom stats: refresh_interval: %w", err)
		}
		interval = d
	}
	namespace := c.Stats.Namespace
	if namespace == "" {
		namespace = "reva"
	}
	attrFile := c.Stats.OwnerAttributesFile
	if attrFile == "" {
		attrFile = DefaultOwnerAttributesFile
	}

	tm, err := jwtmgr.New(nil)
	if err != nil {
		return nil, fmt.Errorf("prom stats: token manager: %w", err)
	}
	col := &collector{
		namespace: namespace,
		attrFile:  attrFile,
		fleet:     registryFleet{tokenManager: tm},
		errors:    map[string]float64{},
	}
	go col.run(ctx, interval)
	return []prometheus.Collector{col}, nil
}

// collector caches the fleet's statistics as const metrics.
type collector struct {
	namespace string
	attrFile  string
	fleet     fleet

	mu      sync.Mutex
	cached  []prometheus.Metric
	errors  map[string]float64 // per service, cumulative refresh errors
	refresh map[string]refreshInfo
}

type refreshInfo struct {
	when     time.Time
	duration time.Duration
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.cached {
		ch <- m
	}
	for _, m := range c.selfMetricsLocked() {
		ch <- m
	}
}

// run is the refresh loop; the first refresh happens immediately.
func (c *collector) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		c.refreshOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// refreshOnce queries every discovered stats target and swaps the cache.
func (c *collector) refreshOnce(ctx context.Context) {
	log := appctx.GetLogger(ctx)
	targets := c.fleet.StatsTargets()
	if len(targets) == 0 {
		return
	}
	ctx, err := c.fleet.AuthContext(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("prom stats: cannot authenticate to the control channel")
		return
	}
	attrs, attrKeys := c.loadOwnerAttributes()

	agg := newAggregator(c.namespace)
	c.mu.Lock()
	refresh := map[string]refreshInfo{}
	maps.Copy(refresh, c.refresh)
	errors := c.errors
	c.mu.Unlock()

	for svcName, eps := range targets {
		start := time.Now()
		payloads, err := c.collectService(ctx, eps)
		if err != nil {
			errors[svcName]++
			log.Warn().Err(err).Str("service", svcName).Msg("prom stats: refresh failed")
			continue
		}
		for _, p := range payloads {
			agg.add(svcName, p, attrs, attrKeys)
		}
		refresh[svcName] = refreshInfo{when: time.Now(), duration: time.Since(start)}
	}

	metrics, errs := agg.build()
	for _, err := range errs {
		log.Warn().Err(err).Msg("prom stats: dropping metric")
	}

	c.mu.Lock()
	c.cached = metrics
	c.refresh = refresh
	c.errors = errors
	c.mu.Unlock()
}

// collectService invokes stats on one instance, fanning out to the rest
// only when the payload declares per-instance scope.
func (c *collector) collectService(ctx context.Context, eps []client.Endpoint) ([]*stats.Payload, error) {
	if len(eps) == 0 {
		return nil, fmt.Errorf("no live instances")
	}
	first, err := c.fleet.InvokeStats(ctx, eps[0])
	if err != nil {
		return nil, err
	}
	payloads := []*stats.Payload{first}
	if first.Scope != stats.ScopeInstance {
		return payloads, nil
	}
	for _, ep := range eps[1:] {
		p, err := c.fleet.InvokeStats(ctx, ep)
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, p)
	}
	return payloads, nil
}

// loadOwnerAttributes reads the attributes file; a missing file means no
// enrichment. Returns the owner map and the sorted union of attribute keys.
func (c *collector) loadOwnerAttributes() (map[string]map[string]string, []string) {
	data, err := os.ReadFile(c.attrFile)
	if err != nil {
		return nil, nil
	}
	var attrs map[string]map[string]string
	if err := json.Unmarshal(data, &attrs); err != nil {
		return nil, nil
	}
	keySet := map[string]struct{}{}
	for _, kv := range attrs {
		for k := range kv {
			keySet[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return attrs, keys
}

// selfMetricsLocked renders the collector's own health metrics.
func (c *collector) selfMetricsLocked() []prometheus.Metric {
	var out []prometheus.Metric
	tsDesc := prometheus.NewDesc(c.namespace+"_stats_refresh_timestamp_seconds",
		"Unix time of the last successful stats refresh, per service.", []string{"service"}, nil)
	durDesc := prometheus.NewDesc(c.namespace+"_stats_refresh_duration_seconds",
		"Duration of the last successful stats refresh, per service.", []string{"service"}, nil)
	errDesc := prometheus.NewDesc(c.namespace+"_stats_refresh_errors_total",
		"Failed stats refreshes, per service.", []string{"service"}, nil)
	for svcName, info := range c.refresh {
		out = append(out,
			prometheus.MustNewConstMetric(tsDesc, prometheus.GaugeValue, float64(info.when.Unix()), svcName),
			prometheus.MustNewConstMetric(durDesc, prometheus.GaugeValue, info.duration.Seconds(), svcName))
	}
	for svcName, n := range c.errors {
		out = append(out, prometheus.MustNewConstMetric(errDesc, prometheus.CounterValue, n, svcName))
	}
	return out
}

// registryFleet is the production fleet: the process-wide service
// registry plus the control channel.
type registryFleet struct {
	tokenManager token.Manager
}

// AuthContext mints a short-lived admin-scoped token for the collector's
// own identity — the control channel only accepts admin scope — and puts
// it on the outgoing metadata, exactly like an admin fan-out does. The
// token is signed with the deployment's shared JWT secret, so every
// process in the fleet validates it.
func (f registryFleet) AuthContext(ctx context.Context) (context.Context, error) {
	u := &userpb.User{
		Id:       &userpb.UserId{OpaqueId: "reva:stats-collector", Type: userpb.UserType_USER_TYPE_APPLICATION},
		Username: "stats-collector",
	}
	scopes, err := scope.AddAdminScope(nil)
	if err != nil {
		return nil, err
	}
	tkn, err := f.tokenManager.MintToken(ctx, u, scopes)
	if err != nil {
		return nil, err
	}
	return metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, tkn), nil
}

// StatsTargets scans registry metadata for services advertising the stats
// invocation — no dialing involved.
func (registryFleet) StatsTargets() map[string][]client.Endpoint {
	reg := service.GlobalRegistry()
	if reg == nil {
		return nil
	}
	svcs, err := reg.ListServices()
	if err != nil {
		return nil
	}
	targets := map[string][]client.Endpoint{}
	for _, sv := range svcs {
		if !advertisesStats(sv.Nodes()) {
			continue
		}
		if _, eps, err := client.Resolve(reg, sv.Name()); err == nil && len(eps) > 0 {
			targets[sv.Name()] = eps
		}
	}
	return targets
}

func advertisesStats(nodes []svcregistry.Node) bool {
	for _, n := range nodes {
		for name := range strings.SplitSeq(n.Metadata()[invoke.MetaInvocations], ",") {
			if strings.TrimSpace(name) == stats.Invocation {
				return true
			}
		}
	}
	return false
}

// InvokeStats runs the stats invocation on one endpoint and parses the
// payload.
func (registryFleet) InvokeStats(ctx context.Context, ep client.Endpoint) (*stats.Payload, error) {
	res := client.InvokeOne(ctx, ep, stats.Invocation, nil)
	if res.Error != "" {
		return nil, fmt.Errorf("%s: %s", res.Node, res.Error)
	}
	return stats.FromJSON([]byte(res.ResultJSON))
}
