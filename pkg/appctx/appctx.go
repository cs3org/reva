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

package appctx

import (
	"context"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

const traceIDKey = "traceid"

// WithLogger returns a context with an associated logger.
func WithLogger(ctx context.Context, l *zerolog.Logger) context.Context {
	traceID := GetTraceID(ctx)
	sublog := l.With().Str(traceIDKey, traceID.String()).Logger()
	return sublog.WithContext(ctx)
}

// GetLogger returns the logger associated with the given context
// or a disabled logger in case no logger is stored inside the context.
func GetLogger(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}

func GetTraceID(ctx context.Context) trace.TraceID {
	traceID := trace.SpanContextFromContext(ctx).TraceID()
	return traceID
}
