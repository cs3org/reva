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
	"context"
	"net/http"
	"path"
)

// WebDavHandler routes to the legacy dav endpoint
type WebDavHandler struct {
}

func (h *WebDavHandler) init(c *Config) error {
	return nil
}

// Handler handles requests
func (h *WebDavHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// webdav should be death: baseURI is encoded as part of the
		// response payload in href field
		baseURI := path.Join("/", s.Prefix(), "remote.php/webdav")

		ctx := context.WithValue(r.Context(), ctxKeyBaseURI, baseURI)

		// always send requests to the home namespace in CS3
		// the /home namespace expects paths relative to the users home dir without a username in it
		namespace := "/home"
		r = r.WithContext(ctx)

		switch r.Method {
		case "PROPFIND":
			s.doPropfind(w, r, namespace)
		case http.MethodOptions:
			s.doOptions(w, r, namespace)
		case http.MethodHead:
			s.doHead(w, r, namespace)
		case http.MethodGet:
			s.doGet(w, r, namespace)
		case "LOCK":
			s.doLock(w, r, namespace)
		case "UNLOCK":
			s.doUnlock(w, r, namespace)
		case "PROPPATCH":
			s.doProppatch(w, r, namespace)
		case "MKCOL":
			s.doMkcol(w, r, namespace)
		case "MOVE":
			s.doMove(w, r, namespace)
		case "COPY":
			s.doCopy(w, r, namespace)
		case http.MethodPut:
			s.doPut(w, r, namespace)
		case http.MethodDelete:
			s.doDelete(w, r, namespace)
		case "REPORT":
			s.doReport(w, r, namespace)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
