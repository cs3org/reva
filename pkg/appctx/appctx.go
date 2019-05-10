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

	"github.com/cs3org/reva/pkg/reqid"
	"github.com/rs/zerolog"
)

// WithLogger returns a context with an associated logger.
func WithLogger(ctx context.Context, l *zerolog.Logger) context.Context {
	return l.WithContext(ctx)
}

// GetLogger returns the logger associated with the given context
// or a disabled logger in case no logger is stored inside the context.
func GetLogger(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}

// WithTrace returns a context with an associated reqid.
func WithTrace(ctx context.Context, t string) context.Context {
	return reqid.ContextSetReqID(ctx, t)
}

// GetTrace returns the trace stored in the context.
func GetTrace(ctx context.Context) string {
	t, ok := reqid.ContextGetReqID(ctx)
	if ok {
		return t
	}
	return "unknown"
}
