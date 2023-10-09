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

// Package appctx creates a context with useful
// components attached to the context like loggers and
// token managers.
package trace

import (
	"net/http"

	"github.com/cs3org/reva/pkg/trace"
	"google.golang.org/grpc/metadata"
)

// New returns a new HTTP middleware that stores the log
// in the context with request ID information.
func New() func(http.Handler) http.Handler {
	return handler
}

func handler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// try to get trace from context
		traceID := trace.Get(ctx)
		if traceID == "" {
			// check if traceID is coming from header
			traceID = r.Header.Get("X-Trace-ID")
			if traceID == "" {
				traceID = trace.Generate()
			}
			ctx = trace.Set(ctx, traceID)
		}

		// in case the http service will call a grpc service,
		// we set the outgoing context so the trace information is
		// passed through the two protocols.
		ctx = metadata.AppendToOutgoingContext(ctx, "revad-grpc-trace-id", traceID)
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}
