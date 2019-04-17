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

package trace

import (
	"context"
	"fmt"

	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/pkg/errors"

	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/trace"
	"github.com/gofrs/uuid"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var logger = log.New("trace")

func init() {
	grpcserver.RegisterUnaryInterceptor("trace", NewUnary)
	grpcserver.RegisterStreamInterceptor("trace", NewStream)
}

type config struct {
	Priority int    `mapstructure:"priority"`
	Header   string `mapstructure:"header"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		logger.Error(context.Background(), errors.Wrap(err, "error decoding conf"))
		return nil, err
	}
	return c, nil
}

// NewUnary returns a new unary interceptor that adds
// trace information for the request.
func NewUnary(m map[string]interface{}) (grpc.UnaryServerInterceptor, int, error) {
	conf, err := parseConfig(m)
	if err != nil {
		return nil, 0, err
	}

	if conf.Header == "" {
		return nil, 0, fmt.Errorf("trace unary interceptor: header is empty")
	}

	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var t string
		md, ok := metadata.FromIncomingContext(ctx)
		if ok && md != nil {
			if val, ok := md[conf.Header]; ok {
				if len(val) > 0 && val[0] != "" {
					t = val[0]
				}
			}
		}

		if t == "" {
			t = uuid.Must(uuid.NewV4()).String()
		}

		ctx = trace.ContextSetTrace(ctx, t)
		return handler(ctx, req)
	}
	return interceptor, conf.Priority, nil
}

// NewStream returns a new server stream interceptor
// that adds trace information to the request.
func NewStream(m map[string]interface{}) (grpc.StreamServerInterceptor, int, error) {
	conf, err := parseConfig(m)
	if err != nil {
		return nil, 0, err
	}

	if conf.Header == "" {
		return nil, 0, fmt.Errorf("trace stream interceptor: header is empty")
	}

	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		var t string
		md, ok := metadata.FromIncomingContext(ss.Context())
		if ok && md != nil {
			if val, ok := md[conf.Header]; ok {
				if len(val) > 0 && val[0] != "" {
					t = val[0]
				}
			}
		}
		if t == "" {
			t = uuid.Must(uuid.NewV4()).String()
		}

		ctx := trace.ContextSetTrace(ss.Context(), t)
		wrapped := newWrappedServerStream(ctx, ss)
		return handler(srv, wrapped)
	}
	return interceptor, conf.Priority, nil
}

func newWrappedServerStream(ctx context.Context, ss grpc.ServerStream) *wrappedServerStream {
	return &wrappedServerStream{ServerStream: ss, newCtx: ctx}
}

type wrappedServerStream struct {
	grpc.ServerStream
	newCtx context.Context
}

func (ss *wrappedServerStream) Context() context.Context {
	return ss.newCtx
}
