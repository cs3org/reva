// Copyright 2018-2020 CERN
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
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/metrics"
	"github.com/cs3org/reva/pkg/metrics/config"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.opencensus.io/stats/view"
)

func init() {
	global.Register("prometheus", New)
}

// New returns a new prometheus service
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	conf := &config.Config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	conf.Init()

	metrics, err := metrics.New(conf)
	if err != nil {
		return nil, errors.Wrap(err, "prometheus: error creating metrics")
	}

	// prometheus handler
	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "revad",
	})
	if err != nil {
		return nil, errors.Wrap(err, "prometheus: error creating exporter")
	}
	// metricsHandler wraps the prometheus handler
	metricsHandler := &MetricsHandler{
		pe:      pe,
		metrics: metrics,
	}
	view.RegisterExporter(metricsHandler)

	return &svc{prefix: conf.Prefix, h: metricsHandler}, nil
}

// MetricsHandler struct and methods (ServeHTTP, ExportView) is a wrapper for prometheus Exporter
// so we can override (execute our own logic) before forwarding to the prometheus Exporter: see overriding method MetricsHandler.ServeHTTP()
type MetricsHandler struct {
	pe      *prometheus.Exporter
	metrics *metrics.Metrics
}

// ServeHTTP override and forward to prometheus.Exporter ServeHTTP()
func (h *MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())
	// make sure the latest metrics data are recorded
	if err := h.metrics.RecordMetrics(); err != nil {
		log.Err(err).Msg("Unable to record metrics")
	}
	// proceed with regular flow
	h.pe.ServeHTTP(w, r)
}

// ExportView must only be implemented to adhere to prometheus.Exporter signature; we simply forward to prometheus ExportView
func (h *MetricsHandler) ExportView(vd *view.Data) {
	// just proceed with regular flow
	h.pe.ExportView(vd)
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
