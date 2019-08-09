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

package ocdavsvc

import (
	"net/http"

	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
)

// DavHandler routes to the different sub handlers
type DavHandler struct {
	FilesHandler   *FilesHandler
	AvatarsHandler *AvatarsHandler
	MetaHandler    *MetaHandler
}

func (h *DavHandler) init(c *Config) error {
	h.FilesHandler = new(FilesHandler)
	h.FilesHandler.init(c)
	h.AvatarsHandler = new(AvatarsHandler)
	h.AvatarsHandler.init(c)
	h.MetaHandler = new(MetaHandler)
	h.MetaHandler.init(c)
	return nil
}

// Handler handles requests
func (h *DavHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var head string
		head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		switch head {
		case "files":
			h.FilesHandler.Handler(s).ServeHTTP(w, r)
		case "avatars":
			h.AvatarsHandler.Handler(s).ServeHTTP(w, r)
		case "meta":
			h.MetaHandler.Handler(s).ServeHTTP(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
