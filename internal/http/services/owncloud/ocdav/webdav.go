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
	"fmt"
	"net/http"
	"path"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/propfind"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
)

// Common Webdav methods.
//
// Unless otherwise noted, these are defined in RFC 4918 section 9.
const (
	MethodPropfind  = "PROPFIND"
	MethodLock      = "LOCK"
	MethodUnlock    = "UNLOCK"
	MethodProppatch = "PROPPATCH"
	MethodMkcol     = "MKCOL"
	MethodMove      = "MOVE"
	MethodCopy      = "COPY"
	MethodReport    = "REPORT"
)

// WebDavHandler implements a dav endpoint
type WebDavHandler struct {
	namespace         string
	useLoggedInUserNS bool
}

func (h *WebDavHandler) init(ns string, useLoggedInUserNS bool) error {
	h.namespace = path.Join("/", ns)
	h.useLoggedInUserNS = useLoggedInUserNS
	return nil
}

// Handler handles requests
func (h *WebDavHandler) Handler(s *svc) http.Handler {
	config := s.Config()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ns, newPath, err := s.ApplyLayout(r.Context(), h.namespace, h.useLoggedInUserNS, r.URL.Path)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			b, err := errors.Marshal(http.StatusNotFound, fmt.Sprintf("could not get storage for %s", r.URL.Path), "")
			errors.HandleWebdavError(appctx.GetLogger(r.Context()), w, b, err)
		}
		r.URL.Path = newPath

		switch r.Method {
		case MethodPropfind:
			p := propfind.NewHandler(config.PublicURL, func() (gateway.GatewayAPIClient, error) {
				return pool.GetGatewayServiceClient(config.GatewaySvc)
			})
			p.HandlePathPropfind(w, r, ns)
		case MethodLock:
			log := appctx.GetLogger(r.Context())
			// TODO initialize status with http.StatusBadRequest
			// TODO initialize err with errors.ErrUnsupportedMethod
			status, err := s.handleLock(w, r, ns)
			if status != 0 { // 0 would mean handleLock already sent the response
				w.WriteHeader(status)
				if status != http.StatusNoContent {
					var b []byte
					if b, err = errors.Marshal(status, err.Error(), ""); err == nil {
						_, err = w.Write(b)
					}
				}
			}
			if err != nil {
				log.Error().Err(err).Msg(err.Error())
			}
		case MethodUnlock:
			log := appctx.GetLogger(r.Context())
			status, err := s.handleUnlock(w, r, ns)
			if status != 0 { // 0 would mean handleUnlock already sent the response
				w.WriteHeader(status)
				if status != http.StatusNoContent {
					var b []byte
					if b, err = errors.Marshal(status, err.Error(), ""); err == nil {
						_, err = w.Write(b)
					}
				}
			}
			if err != nil {
				log.Error().Err(err).Msg(err.Error())
			}
		case MethodProppatch:
			s.handlePathProppatch(w, r, ns)
		case MethodMkcol:
			s.handlePathMkcol(w, r, ns)
		case MethodMove:
			s.handlePathMove(w, r, ns)
		case MethodCopy:
			s.handlePathCopy(w, r, ns)
		case MethodReport:
			s.handleReport(w, r, ns)
		case http.MethodGet:
			s.handlePathGet(w, r, ns)
		case http.MethodPut:
			s.handlePathPut(w, r, ns)
		case http.MethodPost:
			s.handlePathTusPost(w, r, ns)
		case http.MethodOptions:
			s.handleOptions(w, r)
		case http.MethodHead:
			s.handlePathHead(w, r, ns)
		case http.MethodDelete:
			s.handlePathDelete(w, r, ns)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
