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

package sharing

import (
	"net/http"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing/sharees"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/router"
)

// Handler implements the ownCloud sharing API
type Handler struct {
	gatewayAddr    string
	SharesHandler  *shares.Handler
	ShareesHandler *sharees.Handler
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) error {
	h.gatewayAddr = c.GatewaySvc
	h.SharesHandler = new(shares.Handler)
	h.ShareesHandler = new(sharees.Handler)
	if err := h.SharesHandler.Init(c); err != nil {
		return err
	}

	return h.ShareesHandler.Init(c)

}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	var head string
	head, r.URL.Path = router.ShiftPath(r.URL.Path)

	log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")

	switch head {
	case "shares":
		h.SharesHandler.ServeHTTP(w, r)
	case "sharees":
		h.ShareesHandler.ServeHTTP(w, r)
	default:
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "Not found", nil)
	}
}
