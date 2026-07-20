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

package appctx

import (
	"context"
	"strings"

	"github.com/cs3org/reva/v3/pkg/activity"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// NewUnary returns a new unary interceptor that creates the application context.
// counters holds this server's per-service request counters (keyed by reva
// service name), or is nil when activity is not tracked (e.g. the control
// server).
func NewUnary(log zerolog.Logger, counters map[string]*activity.Counter) grpc.UnaryServerInterceptor {
	interceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		traceID := trace.Get(ctx)
		log := requestLogger(log, traceID, info.FullMethod)
		ctx = appctx.WithLogger(ctx, &log)
		defer trackActivity(info.FullMethod, counters)()
		res, err := handler(ctx, req)
		return res, err
	}
	return interceptor
}

// NewStream returns a new server stream interceptor
// that creates the application context.
func NewStream(log zerolog.Logger, counters map[string]*activity.Counter) grpc.StreamServerInterceptor {
	interceptor := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		traceID := trace.Get(ctx)
		log := requestLogger(log, traceID, info.FullMethod)
		ctx = appctx.WithLogger(ctx, &log)

		defer trackActivity(info.FullMethod, counters)()
		wrapped := newWrappedServerStream(ctx, ss)
		err := handler(srv, wrapped)
		return err
	}
	return interceptor
}

// trackActivity records the request against its reva service's counter for the
// duration of the call, so `admin services activity` can tell when an instance
// has quiesced. It returns the matching completion func (a no-op when the method
// is unattributed or belongs to the Admin API / control channel — those
// operational RPCs, including the activity query itself and long-lived admin
// streams, must not count as serving traffic). The Counter is nil-safe, so a
// missing entry (or a nil counters map) simply doesn't count.
func trackActivity(fullMethod string, counters map[string]*activity.Counter) func() {
	if scope.IsAdminMethod(fullMethod) {
		return func() {}
	}
	svc, ok := appctx.GRPCServiceForMethod(fullMethod)
	if !ok {
		return func() {}
	}
	return counters[svc].Enter(rpcName(fullMethod))
}

// rpcName is the method name of a full gRPC method ("/pkg.Service/Method" ->
// "Method"), for the per-method activity breakdown.
func rpcName(fullMethod string) string {
	if i := strings.LastIndex(fullMethod, "/"); i >= 0 {
		return fullMethod[i+1:]
	}
	return fullMethod
}

// requestLogger builds the per-request logger: the trace id plus, when known,
// the reva service handling the method — so request logs are attributable.
func requestLogger(log zerolog.Logger, traceID, fullMethod string) zerolog.Logger {
	c := log.With().Str("traceid", traceID)
	if svc, ok := appctx.GRPCServiceForMethod(fullMethod); ok {
		c = c.Str("service", svc)
	}
	return c.Logger()
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
