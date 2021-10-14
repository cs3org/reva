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

package shareswithme

import (
	"net/http"
	"net/http/httptest"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/cache"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
)

type strategy struct {
	handler *shares.Handler
}

// New creates a SharesWithMe cache warmup
func New(h *shares.Handler) cache.Warmuper {
	return &strategy{
		handler: h,
	}
}

// Warmup returns a function that will fill the cache for the SharesWithMe
// if the key string is in the cache
func (s *strategy) Warmup(r *http.Request) (string, cache.ActionFunc) {
	user := ctxpkg.ContextMustGetUser(r.Context())

	return user.Id.OpaqueId, func() {
		w := httptest.NewRecorder()
		s.handler.ListSharesWithMe(w, r)
	}
}
