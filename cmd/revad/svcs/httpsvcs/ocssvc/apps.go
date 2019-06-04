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

package ocssvc

import (
	"net/http"

	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
)

// AppsHandler holds references to individual app handlers
type AppsHandler struct {
	SharesHandler *SharesHandler
}

func (h *AppsHandler) init(c *Config) {
	h.SharesHandler = new(SharesHandler)
	h.SharesHandler.init(c)
}

// ServeHTTP routes the known apps
func (h *AppsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var head string
	head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
	switch head {
	case "files_sharing":
		head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		if head == "api" {
			head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
			if head == "v1" {
				h.SharesHandler.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "Not Found", http.StatusNotFound)
	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}
