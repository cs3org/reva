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
	"fmt"
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

func (c *collector) refreshLogCounts(ctx context.Context) {
	all, err := c.fleet.InvokeLogCounts(ctx)
	if err != nil {
		appctx.GetLogger(ctx).Warn().Err(err).Msg("prom stats: cannot read the fleet log counts")
		return
	}
	desc := prometheus.NewDesc(c.namespace+"_fleet_log_messages_total",
		"Log messages at warn level or above, per revad process of the fleet.",
		[]string{"service", "host", "level"}, nil)

	type process struct{ host, pid, start string }
	type series struct{ service, host, level string }
	seen := map[process]bool{}
	sums := map[series]uint64{}
	for _, lc := range all {
		host, svc := splitNode(lc.Node)
		p := process{host, fmt.Sprint(lc.PID), fmt.Sprint(lc.Start)}
		if seen[p] {
			continue
		}
		seen[p] = true
		name := processName(lc.Config, svc)
		for lvl, n := range lc.Counts {
			sums[series{name, host, lvl}] += n
		}
	}
	metrics := make([]prometheus.Metric, 0, len(sums))
	for s, n := range sums {
		metrics = append(metrics, prometheus.MustNewConstMetric(desc, prometheus.CounterValue, float64(n), s.service, s.host, s.level))
	}

	c.mu.Lock()
	c.logMetrics = metrics
	c.mu.Unlock()
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
