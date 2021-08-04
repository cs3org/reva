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

	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/user"
	"google.golang.org/grpc/metadata"
)

func (h *V1Handler) cacheWarmup(w http.ResponseWriter, r *http.Request) {
	if h.WarmupCache != nil {
		u := user.ContextMustGetUser(r.Context())
		tkn := token.ContextMustGetToken(r.Context())

		ctx := context.Background()
		ctx = user.ContextSetUser(ctx, u)
		ctx = token.ContextSetToken(ctx, tkn)
		ctx = metadata.AppendToOutgoingContext(ctx, token.TokenHeader, tkn)
		req := r.Clone(ctx)

		id := u.Id.OpaqueId
		if _, err := h.WarmupCache.Get(id); err != nil {
			p := httptest.NewRecorder()
			_ = h.WarmupCache.Set(id, true)
			go h.AppsHandler.SharingHandler.SharesHandler.ListSharesWithOthers(p, req)
			go h.AppsHandler.SharingHandler.SharesHandler.ListSharesWithMe(p, req)
		}
	}
}
