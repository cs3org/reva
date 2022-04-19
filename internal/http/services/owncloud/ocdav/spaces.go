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

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/propfind"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/rhttp/router"
	"github.com/cs3org/reva/v2/pkg/utils"
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
func (h *SpacesHandler) Handler(s *svc, trashbinHandler *TrashbinHandler) http.Handler {
	config := s.Config()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ctx := r.Context()
		// log := appctx.GetLogger(ctx)

		if r.Method == http.MethodOptions {
			s.handleOptions(w, r)
			return
		}

		var segment string
		segment, r.URL.Path = router.ShiftPath(r.URL.Path)
		if segment == "" {
			// listing is disabled, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if segment == _trashbinPath {
			h.handleSpacesTrashbin(w, r, s, trashbinHandler)
			return
		}

		spaceID := segment

		switch r.Method {
		case MethodPropfind:
			p := propfind.NewHandler(config.PublicURL, func() (gateway.GatewayAPIClient, error) {
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
			if err != nil {
				log.Error().Err(err).Str("space", spaceID).Msg(err.Error())
			}
			if status != 0 { // 0 would mean handleLock already sent the response
				w.WriteHeader(status)
				if status != http.StatusNoContent {
					var b []byte
					if b, err = errors.Marshal(status, err.Error(), ""); err == nil {
						_, err = w.Write(b)
					}
					if err != nil {
						log.Error().Err(err).Str("space", spaceID).Msg(err.Error())
					}
				}
			}
		case MethodUnlock:
			log := appctx.GetLogger(r.Context())
			status, err := s.handleUnlock(w, r, spaceID)
			if err != nil {
				log.Error().Err(err).Str("space", spaceID).Msg(err.Error())
			}
			if status != 0 { // 0 would mean handleUnlock already sent the response
				w.WriteHeader(status)
				if status != http.StatusNoContent {
					var b []byte
					if b, err = errors.Marshal(status, err.Error(), ""); err == nil {
						_, err = w.Write(b)
					}
					if err != nil {
						log.Error().Err(err).Str("space", spaceID).Msg(err.Error())
					}
				}
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

func (h *SpacesHandler) handleSpacesTrashbin(w http.ResponseWriter, r *http.Request, s *svc, trashbinHandler *TrashbinHandler) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	var spaceID string
	spaceID, r.URL.Path = router.ShiftPath(r.URL.Path)
	if spaceID == "" {
		// listing is disabled, no auth will change that
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: spaceID,
			OpaqueId:  spaceID,
		},
	}

	var key string
	key, r.URL.Path = router.ShiftPath(r.URL.Path)

	switch r.Method {
	case MethodPropfind:
		trashbinHandler.listTrashbin(w, r, s, ref, path.Join(_trashbinPath, spaceID), key, r.URL.Path)
	case MethodMove:
		if key == "" {
			http.Error(w, "501 Not implemented", http.StatusNotImplemented)
			break
		}
		// find path in url relative to trash base
		baseURI := ctx.Value(net.CtxKeyBaseURI).(string)
		baseURI = path.Join(baseURI, spaceID)

		dh := r.Header.Get(net.HeaderDestination)
		dst, err := net.ParseDestination(baseURI, dh)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		log.Debug().Str("key", key).Str("path", r.URL.Path).Str("dst", dst).Msg("spaces restore")

		dstRef := ref
		dstRef.Path = utils.MakeRelativePath(dst)

		trashbinHandler.restore(w, r, s, ref, dstRef, key, r.URL.Path)
	case http.MethodDelete:
		trashbinHandler.delete(w, r, s, ref, key, r.URL.Path)
	default:
		http.Error(w, "501 Not implemented", http.StatusNotImplemented)
	}
}
