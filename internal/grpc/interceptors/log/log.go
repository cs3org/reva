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

package log

import (
	"context"
	"time"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// NewUnary returns a new unary interceptor
// that logs grpc calls.
func NewUnary() grpc.UnaryServerInterceptor {
	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		res, err := handler(ctx, req)
		code := status.Code(err)
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

		log := appctx.GetLogger(ctx)
		var event *zerolog.Event
		if code != codes.OK {
			event = log.Error()
		} else {
			event = log.Debug()
		}

		event.Str("user-agent", userAgent).
			Str("from", fromAddress).
			Str("uri", info.FullMethod).
			Str("start", start.Format("02/Jan/2006:15:04:05 -0700")).
			Str("end", end.Format("02/Jan/2006:15:04:05 -0700")).Int("time_ns", int(diff)).
			Str("code", code.String()).
			Msg("unary")

		return res, err
	}
	return interceptor
}

// NewStream returns a new server stream interceptor
// that adds trace information to the request.
func NewStream() grpc.StreamServerInterceptor {
	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		end := time.Now()
		code := status.Code(err)
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

		log := appctx.GetLogger(ss.Context())
		var event *zerolog.Event
		if code != codes.OK {
			event = log.Error()
		} else {
			event = log.Info()
		}

		event.Str("user-agent", userAgent).
			Str("from", fromAddress).
			Str("uri", info.FullMethod).
			Str("start", start.Format("02/Jan/2006:15:04:05 -0700")).
			Str("end", end.Format("02/Jan/2006:15:04:05 -0700")).Int("time_ns", int(diff)).
			Str("code", code.String()).
			Msg("stream")

		return err
	}
	return interceptor
}
