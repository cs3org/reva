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

package metrics

import (
	"context"

	"github.com/cs3org/reva/pkg/prom/registry"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

var collector = grpcprom.NewServerMetrics()

func init() {
	registry.Register("grpc_metrics", NewPromCollectors)
}

// New returns a prometheus collector.
func NewPromCollectors(_ context.Context, m map[string]interface{}) ([]prometheus.Collector, error) {
	return []prometheus.Collector{collector}, nil
}

// NewUnary returns a new unary interceptor that adds
// the useragent to the context.
func NewUnary() grpc.UnaryServerInterceptor {
	return collector.UnaryServerInterceptor()
}

// NewStream returns a new server stream interceptor
// that adds the user agent to the context.
func NewStream() grpc.StreamServerInterceptor {
	return collector.StreamServerInterceptor()
}
