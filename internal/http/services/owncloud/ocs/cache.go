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

package ocs

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/cs3org/reva/v3/pkg/appctx"

	"google.golang.org/grpc/metadata"
)

func (s *svc) cacheWarmup(w http.ResponseWriter, r *http.Request) {
	if s.warmupCacheTracker != nil {
		u, ok1 := appctx.ContextGetUser(r.Context())
		tkn, ok2 := appctx.ContextGetToken(r.Context())
		if !ok1 || !ok2 {
			return
		}

		log := appctx.GetLogger(r.Context())

		// We make a copy of the context because the original one comes with its cancel channel,
		// so once the initial request is finished, this ctx gets cancelled as well.
		// And in most of the cases, the warmup takes a longer amount of time to complete than the original request.
		// TODO: Check if we can come up with a better solution, eg, https://stackoverflow.com/a/54132324
		ctx := context.Background()
		ctx = appctx.WithLogger(ctx, log)
		ctx = appctx.ContextSetUser(ctx, u)
		ctx = appctx.ContextSetToken(ctx, tkn)
		ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, tkn)

		req, _ := http.NewRequest(http.MethodGet, "", nil)
		req = req.WithContext(ctx)
		req.URL = r.URL

		id := u.Id.OpaqueId
		if _, err := s.warmupCacheTracker.Get(id); err != nil {
			p := httptest.NewRecorder()
			_ = s.warmupCacheTracker.Set(id, true)

			log.Info().Msgf("cache warmup getting created shares for user %s", id)
			req.URL.Path = "/v1.php/apps/files_sharing/api/v1/shares"
			s.router.ServeHTTP(p, req)

			log.Info().Msgf("cache warmup getting received shares for user %s", id)
			req.URL.Path = "/v1.php/apps/files_sharing/api/v1/shares"
			q := req.URL.Query()
			q.Set("shared_with_me", "true")
			q.Set("state", "all")
			req.URL.RawQuery = q.Encode()
			s.router.ServeHTTP(p, req)
		}
	}
}
