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

package apps

import (
	"net/http"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/notifications"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/rhttp/router"
)

// Handler holds references to individual app handlers
type Handler struct {
	SharingHandler       *sharing.Handler
	NotificationsHandler *notifications.Handler
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) error {
	h.SharingHandler = new(sharing.Handler)
	h.NotificationsHandler = new(notifications.Handler)
	return h.SharingHandler.Init(c)
}

// ServeHTTP routes the known apps
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var head string
	head, r.URL.Path = router.ShiftPath(r.URL.Path)
	switch head {
	case "files_sharing":
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		if head == "api" {
			head, r.URL.Path = router.ShiftPath(r.URL.Path)
			if head == "v1" {
				h.SharingHandler.ServeHTTP(w, r)
				return
			}
		}
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "Not found", nil)
	case "notifications":
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		if head == "api" {
			head, r.URL.Path = router.ShiftPath(r.URL.Path)
			if head == "v1" {
				h.NotificationsHandler.ServeHTTP(w, r)
				return
			}
		}
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "Not found", nil)
	default:
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "Not found", nil)
	}
}
