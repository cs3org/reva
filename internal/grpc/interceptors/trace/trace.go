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

	revatrace "github.com/cs3org/reva/v3/pkg/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// getContext returns a context filled with a trace ID.
// If a trace ID is already set, this context is returned as-is.
// Otherwise, if a span is set, the trace id of this span is set.
// Finally, we check for `revad-grpc-trace-id` in the context metadtata.
// If none of these are set, a new trace ID is generated and set.
func getContext(ctx context.Context) context.Context {
	if id := revatrace.Get(ctx); id != "" {
		return ctx
	}
	if span := oteltrace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		return revatrace.Set(ctx, span.SpanContext().TraceID().String())
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if val := md["revad-grpc-trace-id"]; len(val) > 0 && val[0] != "" {
			return revatrace.Set(ctx, val[0])
		}
	}
	return revatrace.Set(ctx, revatrace.Generate())
}

// NewUnary returns a new unary interceptor that adds
// trace information for the request.
func NewUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(getContext(ctx), req)
	}
}

// NewStream returns a new server stream interceptor
// that adds trace information to the request.
func NewStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := getContext(ss.Context())
		return handler(srv, newWrappedServerStream(ctx, ss))
	}
}

// NewUnaryClientInterceptor returns a new unary client interceptor that
// propagates the reva trace ID from the context into outgoing gRPC metadata.
// This ensures trace IDs are carried across gRPC service boundaries (e.g.
// gateway → storage provider) even when OTel is not configured.
func NewUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if traceID := revatrace.Get(ctx); traceID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "revad-grpc-trace-id", traceID)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// NewStreamClientInterceptor returns a new stream client interceptor that
// propagates the reva trace ID from the context into outgoing gRPC metadata.
func NewStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if traceID := revatrace.Get(ctx); traceID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "revad-grpc-trace-id", traceID)
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
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
