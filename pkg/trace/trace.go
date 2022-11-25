// Copyright 2018-2021 CERN
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
	"net/url"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"

	"go.opentelemetry.io/otel/trace"
)

var (
	// Propagator is the default Reva propagator.
	Propagator      = propagation.NewCompositeTextMapPropagator(propagation.Baggage{}, propagation.TraceContext{})
	defaultProvider = revaDefaultTracerProvider{
		provider: trace.NewNoopTracerProvider(),
	}
)

type revaDefaultTracerProvider struct {
	mutex       sync.RWMutex
	initialized bool
	provider    trace.TracerProvider
}

type ctxKey struct{}

// ContextSetTracerProvider returns a copy of ctx with p associated.
func ContextSetTracerProvider(ctx context.Context, p trace.TracerProvider) context.Context {
	if tp, ok := ctx.Value(ctxKey{}).(trace.TracerProvider); ok {
		if tp == p {
			return ctx
		}
	}
	return context.WithValue(ctx, ctxKey{}, p)
}

// ContextGetTracerProvider returns the TracerProvider associated with the ctx.
// If no TracerProvider is associated is associated, the global default TracerProvider
// is returned
func ContextGetTracerProvider(ctx context.Context) trace.TracerProvider {
	if p, ok := ctx.Value(ctxKey{}).(trace.TracerProvider); ok {
		return p
	}
	return DefaultProvider()
}

// InitDefaultTracerProvider initializes a global default TracerProvider at a package level.
func InitDefaultTracerProvider(exporter, collector, endpoint string) {
	defaultProvider.mutex.Lock()
	defer defaultProvider.mutex.Unlock()
	if !defaultProvider.initialized {
		switch exporter {
		case "otlp":
			defaultProvider.provider = getOtlpTracerProvider(true, endpoint, "reva default otlp provider")
		default:
			defaultProvider.provider = getJaegerTracerProvider(true, collector, endpoint, "reva default jaeger provider")
		}
	}
	defaultProvider.initialized = true
}

// GetTracerProvider returns a new TracerProvider, configure for the specified service
func GetTracerProvider(enabled bool, exporter, collector, endpoint, serviceName string) trace.TracerProvider {
	switch exporter {
	case "otlp":
		return getOtlpTracerProvider(enabled, endpoint, serviceName)
	default:
		return getJaegerTracerProvider(enabled, collector, endpoint, serviceName)
	}
}

// DefaultProvider returns the "global" default TracerProvider
func DefaultProvider() trace.TracerProvider {
	defaultProvider.mutex.RLock()
	defer defaultProvider.mutex.RUnlock()
	return defaultProvider.provider
}

// getJaegerTracerProvider returns a new TracerProvider, configure for the specified service
func getJaegerTracerProvider(enabled bool, collector, endpoint, serviceName string) trace.TracerProvider {
	if !enabled {
		return trace.NewNoopTracerProvider()
	}

	// default to 'reva' as service name if not set
	if serviceName == "" {
		serviceName = "reva"
	}

	var exp *jaeger.Exporter
	var err error

	if endpoint != "" {
		var agentHost string
		var agentPort string

		agentHost, agentPort, err = parseAgentConfig(endpoint)
		if err != nil {
			panic(err)
		}

		exp, err = jaeger.New(
			jaeger.WithAgentEndpoint(
				jaeger.WithAgentHost(agentHost),
				jaeger.WithAgentPort(agentPort),
			),
		)
		if err != nil {
			panic(err)
		}
	}

	if collector != "" {
		exp, err = jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(collector)))
		if err != nil {
			panic(err)
		}
	}

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			semconv.HostNameKey.String(hostname),
		)),
	)
}

func parseAgentConfig(ae string) (string, string, error) {
	u, err := url.Parse(ae)
	// as per url.go:
	// [...] Trying to parse a hostname and path
	// without a scheme is invalid but may not necessarily return an
	// error, due to parsing ambiguities.
	if err == nil && u.Hostname() != "" && u.Port() != "" {
		return u.Hostname(), u.Port(), nil
	}

	p := strings.Split(ae, ":")
	if len(p) != 2 {
		return "", "", fmt.Errorf(fmt.Sprintf("invalid agent endpoint `%s`. expected format: `hostname:port`", ae))
	}

	switch {
	case p[0] == "" && p[1] == "": // case ae = ":"
		return "", "", fmt.Errorf(fmt.Sprintf("invalid agent endpoint `%s`. expected format: `hostname:port`", ae))
	case p[0] == "":
		return "", "", fmt.Errorf(fmt.Sprintf("invalid agent endpoint `%s`. expected format: `hostname:port`", ae))
	}
	return p[0], p[1], nil
}

// getOtelTracerProvider returns a new TracerProvider, configure for the specified service
func getOtlpTracerProvider(enabled bool, endpoint string, serviceName string) trace.TracerProvider {
	if !enabled {
		return trace.NewNoopTracerProvider()
	}

	// default to 'reva' as service name if not set
	if serviceName == "" {
		serviceName = "reva"
	}

	//secureOption := otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, ""))
	//if len(insecure) > 0 {
	secureOption := otlptracegrpc.WithInsecure()
	//}

	exporter, err := otlptrace.New(
		context.Background(),
		otlptracegrpc.NewClient(
			secureOption,
			otlptracegrpc.WithEndpoint(endpoint),
		),
	)

	if err != nil {
		panic(err)
	}
	resources, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("library.language", "go"),
		),
	)
	if err != nil {
		panic(err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resources),
	)
}
