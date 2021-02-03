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

package cloud

import (
	"net/http"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/cloud/capabilities"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/cloud/user"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/cloud/users"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/rhttp/router"
)

// Handler holds references to UserHandler and CapabilitiesHandler
type Handler struct {
	UserHandler         *user.Handler
	UsersHandler        *users.Handler
	CapabilitiesHandler *capabilities.Handler
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) error {
	h.UserHandler = new(user.Handler)
	h.CapabilitiesHandler = new(capabilities.Handler)
	h.CapabilitiesHandler.Init(c)
	h.UsersHandler = new(users.Handler)
	return h.UsersHandler.Init(c)
}

// Handler routes the cloud endpoints
func (h *Handler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		switch head {
		case "capabilities":
			h.CapabilitiesHandler.Handler().ServeHTTP(w, r)
		case "user":
			h.UserHandler.ServeHTTP(w, r)
		case "users":
			h.UsersHandler.ServeHTTP(w, r)
		default:
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "Not found", nil)
		}
	})
}
