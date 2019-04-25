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

// Package appctx creates a context with useful
// components attached to the context like loggers and
// token managers.
package appctx

import (
	"net/http"

	"github.com/cernbox/reva/pkg/appctx"
	"github.com/cernbox/reva/pkg/reqid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/metadata"
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
		reqID := getReqID(r)

		// set reqID into context.
		ctx = reqid.ContextSetReqID(ctx, reqID)
		ctx = metadata.AppendToOutgoingContext(ctx, reqid.ReqIDHeaderName, reqID) // for grpc

		sub := log.With().Str("reqid", reqID).Logger()
		ctx = appctx.WithLogger(ctx, &sub)

		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}

func getReqID(r *http.Request) string {
	var reqID string
	val, ok := reqid.ContextGetReqID(r.Context())
	if ok && val != "" {
		reqID = val
	} else {
		// try to get it from header
		reqID = r.Header.Get(reqid.ReqIDHeaderName)
		if reqID == "" {
			reqID = reqid.MintReqID()
		}
	}
	return reqID
}
