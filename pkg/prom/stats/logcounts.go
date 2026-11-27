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

package stats

import (
	"context"
	"encoding/json"
	"maps"
	"path"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/prometheus/client_golang/prometheus"
)

// logCounts is one instance's answer to the logcounts invocation. The counts
// are its process's, so every service of a process reports the same ones.
type logCounts struct {
	Node   string            `json:"-"`
	Counts map[string]uint64 `json:"counts"`
	PID    int               `json:"pid"`
	Start  int64             `json:"start"`
	Config string            `json:"config"`
}

// InvokeLogCounts runs the logcounts invocation on every service of the fleet.
// A node that fails is left out: its counter resumes on the next refresh.
func (adminFleet) InvokeLogCounts(ctx context.Context) ([]logCounts, error) {
	adm, err := service.Admin(ctx)
	if err != nil {
		return nil, err
	}
	ctx, err = admin.AsService(ctx, "stats-collector")
	if err != nil {
		return nil, err
	}
	res, err := adm.ListServices(ctx, &adminpb.ListServicesRequest{Invocation: invoke.LogCountsInvocation})
	if err != nil {
		return nil, err
	}
	log := appctx.GetLogger(ctx)
	var out []logCounts
	for _, sv := range res.Services {
		ictx, cancel := context.WithTimeout(ctx, logCountsTimeout)
		ires, err := adm.Invoke(ictx, &adminpb.InvokeRequest{Service: sv.Name, Invocation: invoke.LogCountsInvocation})
		cancel()
		if err != nil {
			log.Debug().Err(err).Str("service", sv.Name).Msg("prom stats: log counts failed")
			continue
		}
		for _, r := range ires.Results {
			if r.Error != "" {
				continue
			}
			lc := logCounts{Node: r.Node}
			if err := json.Unmarshal([]byte(r.ResultJson), &lc); err != nil {
				continue
			}
			out = append(out, lc)
		}
	}
	return out, nil
}

const logCountsTimeout = 30 * time.Second

func (c *collector) runLogCounts(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		c.refreshLogCounts(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// logProcess is one revad process of the fleet.
type logProcess struct {
	host  string
	pid   int
	start int64
}

// logSeries is one exported series: the processes of a host sharing a config
// file name are summed into it.
type logSeries struct{ service, host, level string }

// logProcessState is what a process last reported.
type logProcessState struct {
	name   string
	counts map[string]uint64
	seen   time.Time
}

// logProcessTTL is how long a process that stopped answering is remembered,
// so that a node back from an outage is not counted twice.
const logProcessTTL = 24 * time.Hour

// refreshLogCounts adds to each series what its processes logged since the
// previous refresh. The series only grow: a process that misses a refresh, or
// restarts, leaves what it already contributed. A process already running
// when the collector started counts from its first answer, so a restart of
// the collector does not count the fleet's history again.
func (c *collector) refreshLogCounts(ctx context.Context) {
	all, err := c.fleet.InvokeLogCounts(ctx)
	if err != nil {
		appctx.GetLogger(ctx).Warn().Err(err).Msg("prom stats: cannot read the fleet log counts")
		return
	}
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.logProcs == nil {
		c.logProcs = map[logProcess]*logProcessState{}
		c.logTotals = map[logSeries]uint64{}
	}
	done := map[logProcess]bool{}
	for _, lc := range all {
		host, svc := splitNode(lc.Node)
		p := logProcess{host, lc.PID, lc.Start}
		if done[p] { // one answer per service of the process
			continue
		}
		done[p] = true
		st, known := c.logProcs[p]
		if !known {
			st = &logProcessState{name: processName(lc.Config, svc), counts: map[string]uint64{}}
			if lc.Start < c.logsSince {
				maps.Copy(st.counts, lc.Counts)
			}
			c.logProcs[p] = st
		}
		st.seen = now
		for lvl, n := range lc.Counts {
			k := logSeries{st.name, host, lvl}
			if prev := st.counts[lvl]; n > prev {
				c.logTotals[k] += n - prev
			} else if _, ok := c.logTotals[k]; !ok {
				c.logTotals[k] = 0
			}
			st.counts[lvl] = max(st.counts[lvl], n)
		}
	}
	for p, st := range c.logProcs {
		if now.Sub(st.seen) > logProcessTTL {
			delete(c.logProcs, p)
		}
	}

	desc := prometheus.NewDesc(c.namespace+"_fleet_log_messages_total",
		"Log messages at warn level or above of the fleet's revad processes, summed per host and config file name.",
		[]string{"service", "host", "level"}, nil)
	metrics := make([]prometheus.Metric, 0, len(c.logTotals))
	for s, n := range c.logTotals {
		metrics = append(metrics, prometheus.MustNewConstMetric(desc, prometheus.CounterValue, float64(n), s.service, s.host, s.level))
	}
	c.logMetrics = metrics
}

// splitNode splits a node id "host:port/service".
func splitNode(node string) (host, svc string) {
	addr, svc, _ := strings.Cut(node, "/")
	host = addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host = addr[:i]
	}
	return host, svc
}

// processName names a process the way its log file is named:
// /etc/revad/revad-gateway.toml is "gateway". Without a config file it is the
// service's name.
func processName(config, svc string) string {
	if config == "" {
		return svc
	}
	name := strings.TrimSuffix(path.Base(config), ".toml")
	return strings.TrimPrefix(name, "revad-")
}
