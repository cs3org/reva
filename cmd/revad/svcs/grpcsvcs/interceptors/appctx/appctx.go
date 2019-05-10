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

package appctx

import (
	"context"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/reqid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// NewUnary returns a new unary interceptor that creates the application context.
func NewUnary(log zerolog.Logger) grpc.UnaryServerInterceptor {
	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var t string
		md, ok := metadata.FromIncomingContext(ctx)
		if ok && md != nil {
			if val, ok := md[reqid.ReqIDHeaderName]; ok {
				if len(val) > 0 && val[0] != "" {
					t = val[0]
				}
			}
		}

		if t == "" {
			t = reqid.MintReqID()
		}

		ctx = reqid.ContextSetReqID(ctx, t)

		sub := log.With().Str("reqid", t).Logger()
		ctx = appctx.WithLogger(ctx, &sub)

		return handler(ctx, req)
	}
	return interceptor
}

// NewStream returns a new server stream interceptor
// that creates the application context.
func NewStream(log zerolog.Logger) grpc.StreamServerInterceptor {
	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		var t string
		md, ok := metadata.FromIncomingContext(ss.Context())
		if ok && md != nil {
			if val, ok := md[reqid.ReqIDHeaderName]; ok {
				if len(val) > 0 && val[0] != "" {
					t = val[0]
				}
			}
		}
		if t == "" {
			t = reqid.MintReqID()
		}

		ctx := reqid.ContextSetReqID(ss.Context(), t)
		sub := log.With().Str("reqid", t).Logger()
		ctx = appctx.WithLogger(ctx, &sub)

		wrapped := newWrappedServerStream(ctx, ss)
		return handler(srv, wrapped)
	}
	return interceptor
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
