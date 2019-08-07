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
	"context"
	"net/http"
	"net/url"
	"path"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/user"
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
		log := appctx.GetLogger(r.Context())

		if r.Method == "OPTIONS" {
			// no need for the user, and we need to be able
			// to answer preflight checks, which have no auth headers
			s.doOptions(w, r)
			return
		}

		// inject username in path
		ctx := r.Context()
		u, ok := user.ContextGetUser(ctx)
		if !ok {
			log.Error().Msg("error getting user from context")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// TODO(labkode): this assumes too much, basically using ocdavsvc you can't access a global namespace.
		// This must be changed.
		// r.URL.Path = path.Join("/", u.Username, tail2)
		r.URL.Path = path.Join("/", u.Username, r.URL.Path)

		// webdav should be death: baseURI is encoded as part of the
		// reponse payload in href field
		baseURI := path.Join("/", s.Prefix(), "remote.php/webdav")
		ctx = context.WithValue(r.Context(), ctxKeyBaseURI, baseURI)

		// inject username into Destination header if present
		dstHeader := r.Header.Get("Destination")
		if dstHeader != "" {
			dstURL, err := url.ParseRequestURI(dstHeader)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if dstURL.Path[:18] != "/remote.php/webdav" {
				log.Warn().Str("path", dstURL.Path).Msg("dst needs to start with /remote.php/webdav/")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			r.Header.Set("Destination", path.Join(baseURI, u.Username, dstURL.Path[18:])) // 18 = len ("/remote.php/webdav")
		}

		r = r.WithContext(ctx)

		switch r.Method {
		case "PROPFIND":
			s.doPropfind(w, r)
		case "HEAD":
			s.doHead(w, r)
		case "GET":
			s.doGet(w, r)
		case "LOCK":
			s.doLock(w, r)
		case "UNLOCK":
			s.doUnlock(w, r)
		case "PROPPATCH":
			s.doProppatch(w, r)
		case "MKCOL":
			s.doMkcol(w, r)
		case "MOVE":
			s.doMove(w, r)
		case "COPY":
			s.doCopy(w, r)
		case "PUT":
			s.doPut(w, r)
		case "DELETE":
			s.doDelete(w, r)
		case "REPORT":
			s.doReport(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
