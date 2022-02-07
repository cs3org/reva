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
	"path"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/propfind"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
)

// SpacesHandler handles trashbin requests
type SpacesHandler struct {
	gatewaySvc        string
	namespace         string
	useLoggedInUserNS bool
}

func (h *SpacesHandler) init(c *Config) error {
	h.gatewaySvc = c.GatewaySvc
	h.namespace = path.Join("/", c.WebdavNamespace)
	h.useLoggedInUserNS = true
	return nil
}

// Handler handles requests
func (h *SpacesHandler) Handler(s *svc) http.Handler {
	config := s.Config()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ctx := r.Context()
		// log := appctx.GetLogger(ctx)

		if r.Method == http.MethodOptions {
			s.handleOptions(w, r)
			return
		}

		var spaceID string
		spaceID, r.URL.Path = router.ShiftPath(r.URL.Path)

		if spaceID == "" {
			// listing is disabled, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		switch r.Method {
		case MethodPropfind:
			p := propfind.NewHandler(config.PublicURL, func() (propfind.GatewayClient, error) {
				return pool.GetGatewayServiceClient(config.GatewaySvc)
			})
			p.HandleSpacesPropfind(w, r, spaceID)
		case MethodProppatch:
			s.handleSpacesProppatch(w, r, spaceID)
		case MethodLock:
			log := appctx.GetLogger(r.Context())
			// TODO initialize status with http.StatusBadRequest
			// TODO initialize err with errors.ErrUnsupportedMethod
			status, err := s.handleSpacesLock(w, r, spaceID)
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
				log.Error().Err(err).Str("space", spaceID).Msg(err.Error())
			}
		case MethodUnlock:
			log := appctx.GetLogger(r.Context())
			status, err := s.handleUnlock(w, r, spaceID)
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
				log.Error().Err(err).Str("space", spaceID).Msg(err.Error())
			}
		case MethodMkcol:
			s.handleSpacesMkCol(w, r, spaceID)
		case MethodMove:
			s.handleSpacesMove(w, r, spaceID)
		case MethodCopy:
			s.handleSpacesCopy(w, r, spaceID)
		case MethodReport:
			s.handleReport(w, r, spaceID)
		case http.MethodGet:
			s.handleSpacesGet(w, r, spaceID)
		case http.MethodPut:
			s.handleSpacesPut(w, r, spaceID)
		case http.MethodPost:
			s.handleSpacesTusPost(w, r, spaceID)
		case http.MethodOptions:
			s.handleOptions(w, r)
		case http.MethodHead:
			s.handleSpacesHead(w, r, spaceID)
		case http.MethodDelete:
			s.handleSpacesDelete(w, r, spaceID)
		default:
			http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		}
	})
}
