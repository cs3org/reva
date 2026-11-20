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

package logger

import (
	"context"
	"sync/atomic"

	"github.com/cs3org/reva/v3/pkg/prom/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

// The hook counts before the metric name is known from the config, so the
// counts are plain atomics that the collector reads at scrape time.
var logMessages = map[zerolog.Level]*atomic.Uint64{
	zerolog.WarnLevel:  {},
	zerolog.ErrorLevel: {},
	zerolog.FatalLevel: {},
	zerolog.PanicLevel: {},
}

func init() {
	registry.Register("logs", NewPromCollectors)
}

type promConfig struct {
	Logs struct {
		// Namespace prefixes the metric name (default "reva").
		Namespace string `mapstructure:"namespace"`
	} `mapstructure:"logs"`
}

// NewPromCollectors returns the counter of log messages at warn level or above.
func NewPromCollectors(_ context.Context, m map[string]any) ([]prometheus.Collector, error) {
	var c promConfig
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	namespace := c.Logs.Namespace
	if namespace == "" {
		namespace = "reva"
	}
	cols := make([]prometheus.Collector, 0, len(logMessages))
	for lvl, n := range logMessages {
		cols = append(cols, prometheus.NewCounterFunc(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "log_messages_total",
			Help:        "Number of log messages emitted at warn level or above.",
			ConstLabels: prometheus.Labels{"level": lvl.String()},
		}, func() float64 { return float64(n.Load()) }))
	}
	return cols, nil
}

// countHook runs only for events that pass the level filters, so the counter matches the log output.
type countHook struct{}

func (countHook) Run(_ *zerolog.Event, lvl zerolog.Level, _ string) {
	if n, ok := logMessages[lvl]; ok {
		n.Add(1)
	}
}
