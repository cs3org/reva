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

package trace

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	oteltrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// ProviderConfig holds the tracing configuration.
type ProviderConfig struct {
	Enabled     bool
	Endpoint    string // OTLP/gRPC endpoint, e.g. "localhost:4317"
	Collector   string // OTLP/HTTP collector URL, e.g. "http://localhost:4318"
	ServiceName string
	Log         *zerolog.Logger
}

// InitProvider sets up the global OTel TracerProvider and W3C TraceContext
// propagator.
//
// If tracing is disabled or no endpoint is configured,
// the provider returned is just a nop.
//
// The returned shutdown function must be called (e.g. via defer) to flush and stop the exporter.
func InitProvider(ctx context.Context, cfg ProviderConfig) (func(context.Context) error, error) {
	if !cfg.Enabled || (cfg.Endpoint == "" && cfg.Collector == "") {
		return func(context.Context) error { return nil }, nil
	}

	if cfg.Log != nil {
		endpoint := cfg.Endpoint
		if endpoint == "" {
			endpoint = cfg.Collector
		}
		cfg.Log.Info().Str("endpoint", endpoint).Msg("tracing enabled")
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
			cfg.Log.Warn().Err(err).Msg("tracing export error")
		}))
	}

	name := cfg.ServiceName
	if name == "" {
		name = "reva"
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(name)),
		resource.WithProcess(),
		resource.WithOS(),
	)
	if err != nil {
		return nil, err
	}

	var exporter oteltrace.SpanExporter
	switch {
	case cfg.Endpoint != "":
		exporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
			otlptracegrpc.WithTimeout(5*time.Second),
		)
	case cfg.Collector != "":
		exporter, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(cfg.Collector),
			otlptracehttp.WithTimeout(5*time.Second),
		)
	}
	if err != nil {
		return nil, err
	}

	tp := oteltrace.NewTracerProvider(
		oteltrace.WithBatcher(exporter),
		oteltrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if cfg.Log != nil {
		go pingEndpoint(cfg.Endpoint, cfg.Collector, cfg.Log)
	}

	return tp.Shutdown, nil
}

// pingEndpoint checks whether the tracing endpoint is reachable at startup,
// logging a warning if not. It runs in a goroutine so it does not block startup.
// For gRPC endpoints a TCP dial is used; for HTTP collectors a real HTTP request
// is made so that application-level failures (e.g. no pods behind a route) are
// also detected.
func pingEndpoint(grpcEndpoint, httpCollector string, log *zerolog.Logger) {
	if grpcEndpoint != "" {
		conn, err := net.DialTimeout("tcp", grpcEndpoint, 5*time.Second)
		if err != nil {
			log.Warn().Err(err).Str("endpoint", grpcEndpoint).Msg("tracing endpoint unreachable on startup")
			return
		}
		conn.Close()
		return
	}

	// For HTTP collectors the hostname may be given without a scheme; default to https.
	target := httpCollector
	if len(target) > 0 && target[0] != 'h' {
		target = "https://" + target
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(target + "/v1/traces")
	if err != nil {
		log.Warn().Err(err).Str("endpoint", httpCollector).Msg("tracing endpoint unreachable on startup")
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		log.Warn().Err(fmt.Errorf("HTTP %d", resp.StatusCode)).Str("endpoint", httpCollector).Msg("tracing endpoint unreachable on startup")
	}
}
