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

package ocdav

import (
	"net/http"

	"github.com/cs3org/reva/pkg/rhttp/router"
)

// MetaHandler handles meta requests
type MetaHandler struct {
	VersionsHandler *VersionsHandler
}

func (h *MetaHandler) init(c *Config) error {
	h.VersionsHandler = new(VersionsHandler)
	return h.VersionsHandler.init(c)
}

// Handler handles requests
func (h *MetaHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var id string
		id, r.URL.Path = router.ShiftPath(r.URL.Path)
		if id == "" {
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}

		did := unwrap(id)

		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		switch head {
		case "v":
			h.VersionsHandler.Handler(s, did).ServeHTTP(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}

	})
}
