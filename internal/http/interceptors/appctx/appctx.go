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

// Package appctx creates a context with useful
// components attached to the context like loggers and
// token managers.
package appctx

import (
	"net/http"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/rs/zerolog"
)

// New returns a new HTTP middleware that stores the log
// in the context with request ID information.
func New(log zerolog.Logger) func(http.Handler) http.Handler {
	chain := func(h http.Handler) http.Handler {
		return handler(log, h)
	}
	return chain
}

func handler(log zerolog.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		traceID := trace.Get(ctx)
		log := log.With().Str("traceid", traceID).Logger()
		ctx = appctx.WithLogger(ctx, &log)
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}
