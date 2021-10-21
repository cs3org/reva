// Copyright 2018-2021 CERN
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

	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"google.golang.org/grpc/metadata"
)

func (s *svc) cacheWarmup(w http.ResponseWriter, r *http.Request) {
	if s.warmupCacheTracker != nil {
		u := ctxpkg.ContextMustGetUser(r.Context())
		tkn := ctxpkg.ContextMustGetToken(r.Context())

		ctx := context.Background()
		ctx = ctxpkg.ContextSetUser(ctx, u)
		ctx = ctxpkg.ContextSetToken(ctx, tkn)
		ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, tkn)
		req := r.Clone(ctx)
		req.Method = http.MethodGet

		id := u.Id.OpaqueId
		if _, err := s.warmupCacheTracker.Get(id); err != nil {
			p := httptest.NewRecorder()
			_ = s.warmupCacheTracker.Set(id, true)
			req.URL.Path = "/v1.php/apps/files_sharing/api/v1/shares"
			s.router.ServeHTTP(p, req)
			req.URL.Path = "/v1.php/apps/files_sharing/api/v1/shares?shared_with_me=true"
			s.router.ServeHTTP(p, req)
		}
	}
}
