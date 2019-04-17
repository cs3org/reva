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

package log

import (
	"context"
	"time"

	"google.golang.org/grpc/metadata"

	"google.golang.org/grpc/peer"

	"google.golang.org/grpc/codes"

	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/cernbox/reva/pkg/log"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

var logger = log.New("grpc-interceptor-log")

func init() {
	grpcserver.RegisterUnaryInterceptor("log", NewUnary)
	grpcserver.RegisterStreamInterceptor("log", NewStream)
}

type config struct {
	Priority int `mapstructure:"priority"`
}

// NewUnary returns a new unary interceptor
// that logs grpc calls.
func NewUnary(m map[string]interface{}) (grpc.UnaryServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}

	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		res, err := handler(ctx, req)
		code := grpc.Code(err)
		end := time.Now()
		diff := end.Sub(start).Nanoseconds()
		var fromAddress, userAgent string
		if p, ok := peer.FromContext(ctx); ok {
			fromAddress = p.Addr.Network() + "://" + p.Addr.String()
		}
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals, ok := md["user-agent"]; ok {
				if len(vals) > 0 && vals[0] != "" {
					userAgent = vals[0]
				}
			}
		}

		var b *log.Builder
		if code != codes.OK {
			b = logger.BuildError()
		} else {
			b = logger.Build()
		}

		b = b.Str("user-agent", userAgent)
		b = b.Str("from", fromAddress)
		b = b.Str("uri", info.FullMethod)
		b = b.Str("start", start.Format("02/Jan/2006:15:04:05 -0700"))
		b = b.Str("end", end.Format("02/Jan/2006:15:04:05 -0700")).Int("time_ns", int(diff))
		b = b.Str("code", code.String())
		b.Msg(ctx, "GRPC unary call")

		return res, err
	}
	return interceptor, conf.Priority, nil
}

// NewStream returns a new server stream interceptor
// that adds trace information to the request.
func NewStream(m map[string]interface{}) (grpc.StreamServerInterceptor, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}

	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		end := time.Now()
		code := grpc.Code(err)
		diff := end.Sub(start).Nanoseconds()
		var fromAddress, userAgent string
		if p, ok := peer.FromContext(ss.Context()); ok {
			fromAddress = p.Addr.Network() + "://" + p.Addr.String()
		}
		if md, ok := metadata.FromIncomingContext(ss.Context()); ok {
			if vals, ok := md["user-agent"]; ok {
				if len(vals) > 0 && vals[0] != "" {
					userAgent = vals[0]
				}
			}
		}

		var b *log.Builder
		if code != codes.OK {
			b = logger.BuildError()
		} else {
			b = logger.Build()
		}

		b = b.Str("user-agent", userAgent)
		b = b.Str("from", fromAddress)
		b = b.Str("uri", info.FullMethod)
		b = b.Str("start", start.Format("02/Jan/2006:15:04:05 -0700"))
		b = b.Str("end", end.Format("02/Jan/2006:15:04:05 -0700")).Int("time_ns", int(diff))
		b = b.Str("code", code.String())
		b.Msg(ss.Context(), "GRPC stream call")

		return err
	}
	return interceptor, conf.Priority, nil
}
