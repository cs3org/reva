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

package ocdav

import (
	"net/http"
	"path"
)

// WebDavHandler implements a dav endpoint
type WebDavHandler struct {
	namespace string
}

func (h *WebDavHandler) init(ns string) error {
	h.namespace = path.Join("/", ns)
	return nil
}

// Handler handles requests
func (h *WebDavHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			s.handlePropfind(w, r, h.namespace)
		case "LOCK":
			s.handleLock(w, r, h.namespace)
		case "UNLOCK":
			s.handleUnlock(w, r, h.namespace)
		case "PROPPATCH":
			s.handleProppatch(w, r, h.namespace)
		case "MKCOL":
			s.handleMkcol(w, r, h.namespace)
		case "MOVE":
			s.handleMove(w, r, h.namespace)
		case "COPY":
			s.handleCopy(w, r, h.namespace)
		case "REPORT":
			s.handleReport(w, r, h.namespace)
		case http.MethodGet:
			s.handleGet(w, r, h.namespace)
		case http.MethodPut:
			s.handlePut(w, r, h.namespace)
		case http.MethodOptions:
			s.handleOptions(w, r, h.namespace)
		case http.MethodHead:
			s.handleHead(w, r, h.namespace)
		case http.MethodDelete:
			s.handleDelete(w, r, h.namespace)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
