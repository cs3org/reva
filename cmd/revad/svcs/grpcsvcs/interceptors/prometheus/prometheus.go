// Copyright 2018-2019 CERN
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
	"github.com/cernbox/reva/cmd/revad/grpcserver"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

func init() {
	grpcserver.RegisterUnaryInterceptor("prometheus", NewUnary)
	grpcserver.RegisterStreamInterceptor("prometheus", NewStream)
}

type config struct {
	Priority int `mapstructure:"priority"`
}

// NewUnary returns a server interceptor that adds telemetry to
// grpc calls.
func NewUnary(m map[string]interface{}) (grpc.UnaryServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}
	return grpc_prometheus.UnaryServerInterceptor, conf.Priority, nil
}

// NewStream returns a streaming server inteceptor that adds telemetry to
// streaming grpc calls.
func NewStream(m map[string]interface{}) (grpc.StreamServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}
	return grpc_prometheus.StreamServerInterceptor, conf.Priority, nil
}
