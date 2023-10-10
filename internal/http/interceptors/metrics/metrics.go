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

// Package appctx creates a context with useful
// components attached to the context like loggers and
// token managers.
package metrics

import (
	"context"
	"net/http"

	"github.com/cs3org/reva/pkg/prom/registry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var inFlightGauge = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "http_in_flight_requests",
	Help: "A gauge of requests currently being served by the wrapped handler.",
})

var counter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_api_requests_total",
		Help: "A counter for requests to the wrapped handler.",
	},
	[]string{"code", "method"},
)

// duration is partitioned by the HTTP method and handler. It uses custom
// buckets based on the expected request duration.
var duration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "A histogram of latencies for requests.",
		Buckets: []float64{.25, .5, 1, 2.5, 5, 10},
	},
	[]string{"handler", "method"},
)

// responseSize has no labels, making it a zero-dimensional
// ObserverVec.
var responseSize = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_response_size_bytes",
		Help:    "A histogram of response sizes for requests.",
		Buckets: []float64{200, 500, 900, 1500},
	},
	[]string{},
)

// requestSize has no labels, making it a zero-dimensional
// ObserverVec.
var requestSize = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_request_size_bytes",
		Help:    "A histogram of request sizes for requests.",
		Buckets: []float64{200, 500, 900, 1500},
	},
	[]string{},
)

func init() {
	registry.Register("http_metrics", NewPromCollectors)
}

// New returns a prometheus collector.
func NewPromCollectors(_ context.Context, m map[string]interface{}) ([]prometheus.Collector, error) {
	return []prometheus.Collector{inFlightGauge, counter, duration, responseSize}, nil
}

// New returns a new HTTP middleware that stores the log
// in the context with request ID information.
func New() func(h http.Handler) http.Handler {
	chain := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h = promhttp.InstrumentHandlerDuration(duration.MustCurryWith(prometheus.Labels{"handler": r.URL.Path}),
				promhttp.InstrumentHandlerCounter(counter,
					promhttp.InstrumentHandlerResponseSize(responseSize,
						promhttp.InstrumentHandlerRequestSize(requestSize,
							promhttp.InstrumentHandlerInFlight(inFlightGauge, h),
						),
					),
				),
			)
			h.ServeHTTP(w, r)
		})
	}
	return chain
}
