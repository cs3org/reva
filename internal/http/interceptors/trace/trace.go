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

package trace

import (
	"context"
	"net/http"

	revatrace "github.com/cs3org/reva/v3/pkg/trace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

// New returns a new HTTP middleware that stores the log
// in the context with request ID information.
func New() func(http.Handler) http.Handler {
	return handler
}

func handler(next http.Handler) http.Handler {
	// otelhttp extracts W3C traceparent from the request, creates a span,
	// and injects it into the request context before calling inner.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID, ctx := getTraceID(r)
		ctx = metadata.AppendToOutgoingContext(ctx, "revad-grpc-trace-id", traceID)
		w.Header().Set("x-request-id", traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
	return otelhttp.NewHandler(inner, "reva.http")
}

func getTraceID(r *http.Request) (string, context.Context) {
	ctx := r.Context()
	// Preserve a trace ID already set in the context by a parent middleware.
	if id := revatrace.Get(ctx); id != "" {
		return id, ctx
	}
	// Prefer the OTel span's trace ID (set by otelhttp above). This ensures
	// the trace ID in logs matches the one visible in Jaeger.
	if span := oteltrace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		id := span.SpanContext().TraceID().String()
		return id, revatrace.Set(ctx, id)
	}
	// Accept X-Trace-ID / X-Request-ID headers from clients that don't
	// send a W3C traceparent.
	if traceID := r.Header.Get("X-Trace-ID"); traceID != "" {
		return traceID, revatrace.Set(ctx, traceID)
	}
	if traceID := r.Header.Get("X-Request-ID"); traceID != "" {
		return traceID, revatrace.Set(ctx, traceID)
	}
	// Otherwise, we generate one ourselves
	traceID := revatrace.Generate()
	return traceID, revatrace.Set(ctx, traceID)
}
