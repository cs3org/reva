// Copyright 2018-2020 CERN
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
	"net/http"

	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
)

// CloudHandler holds references to UserHandler and CapabilitiesHandler
type CloudHandler struct {
	UserHandler         *UserHandler
	UsersHandler        *UsersHandler
	CapabilitiesHandler *CapabilitiesHandler
}

func (h *CloudHandler) init(c *config.Config) {
	h.UserHandler = new(UserHandler)
	h.UsersHandler = new(UsersHandler)
	h.CapabilitiesHandler = new(CapabilitiesHandler)
	h.CapabilitiesHandler.init(c)
}

// Handler routes the cloud endpoints
func (h *CloudHandler) Handler() http.Handler {
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
