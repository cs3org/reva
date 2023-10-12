// Copyright 2018-2023 CERN
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

package prometheus

import (
	"context"
	"net/http"

	"github.com/cs3org/reva/pkg/prom/registry"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func init() {
	global.Register("prometheus", New)
}

// New returns a new prometheus service.
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	// instantiate and register all collectors
	collectors := []prometheus.Collector{}
	for _, f := range registry.NewFuncs {
		cols, err := f(ctx, m)
		if err != nil {
			return nil, err
		}
		collectors = append(collectors, cols...)
	}

	// custom registry to avoid global prometheus registry that can be
	// modified at global package level
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors...)

	handler := promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			Registry:          reg,
			EnableOpenMetrics: true,
		})
	return &svc{prefix: c.Prefix, h: handler}, nil
}

type config struct {
	Prefix string `mapstructure:"prefix"`
}

func (c *config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "metrics"
	}
}

type svc struct {
	prefix string
	h      http.Handler
}

func (s *svc) Prefix() string {
	return s.prefix
}

func (s *svc) Handler() http.Handler {
	return s.h
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Unprotected() []string {
	// TODO(labkode): all prometheus endpoints are public?
	return []string{"/"}
}
