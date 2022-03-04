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
	"fmt"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	// Propagator is the default Reva propagator.
	Propagator = propagation.NewCompositeTextMapPropagator(propagation.Baggage{}, propagation.TraceContext{})

	// Provider is the default Reva tracer provider.
	Provider = trace.NewNoopTracerProvider()
)

// SetTraceProvider sets the TracerProvider at a package level.
func SetTraceProvider(collectorEndpoint string, agentEndpoint, serviceName string) {
	// default to 'reva' as service name if not set
	if serviceName == "" {
		serviceName = "reva"
	}

	var exp *jaeger.Exporter
	var err error

	if agentEndpoint != "" {
		var agentHost string
		var agentPort string

		agentHost, agentPort, err = parseAgentConfig(agentEndpoint)
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

	if collectorEndpoint != "" {
		exp, err = jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(collectorEndpoint)))
		if err != nil {
			panic(err)
		}
	}

	Provider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
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
