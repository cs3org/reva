// Copyright 2018-2024 CERN
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

	"github.com/cs3org/reva/v3/pkg/prom/registry"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// mount is where the scrape endpoint is served.
const mount = "/metrics"

func init() {
	global.Register("prometheus", New)
}

// New returns a new prometheus service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
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
	return &svc{h: handler}, nil
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
	h http.Handler
}

func (s *svc) Prefix() string {
	return mount
}

// Routes mounts the prometheus handler, which serves the scrape endpoint
// regardless of the path it is reached at.
func (s *svc) Routes(r *router.Router) {
	// TODO(labkode): all prometheus endpoints are public?
	r.Mount(mount, s.h, router.Unprotected())
}

func (s *svc) Close() error {
	return nil
}
