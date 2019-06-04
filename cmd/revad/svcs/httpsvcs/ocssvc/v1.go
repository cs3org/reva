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

// V1Handler routes to the different sub handlers
type V1Handler struct {
	AppsHandler   *AppsHandler
	CloudHandler  *CloudHandler
	ConfigHandler *ConfigHandler
}

func (h *V1Handler) init(c *Config) {
	h.AppsHandler = new(AppsHandler)
	h.AppsHandler.init(c)
	h.CloudHandler = new(CloudHandler)
	h.CloudHandler.init(c)
	h.ConfigHandler = new(ConfigHandler)
	h.ConfigHandler.init(c)
}

// Handler handles requests
func (h *V1Handler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var head string
		head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		switch head {
		case "apps":
			h.AppsHandler.ServeHTTP(w, r)
		case "cloud":
			h.CloudHandler.Handler().ServeHTTP(w, r)
		case "config":
			h.ConfigHandler.Handler().ServeHTTP(w, r)
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	})
}
