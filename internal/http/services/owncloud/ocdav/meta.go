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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"go.opencensus.io/trace"
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
		switch {
		case head == "" && r.Method == "PROPFIND":
			h.handlePropfind(w, r, s, did)
		case head == "v":
			h.VersionsHandler.Handler(s, did).ServeHTTP(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (h *MetaHandler) handlePropfind(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "handlePropfind")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Logger()

	pf, status, err := readPropfind(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO parse additional metdadata?

	ref := &provider.Reference{
		Spec: &provider.Reference_Id{Id: rid},
	}
	req := &provider.StatRequest{
		Ref: ref,
	}
	res, err := client.Stat(ctx, req)
	if err != nil {
		sublog.Error().Err(err).Interface("req", req).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, res.Status)
		return
	}

	info := res.Info
	// TODO add /v entry
	infos := []*provider.ResourceInfo{info}

	propRes, err := s.formatPropfind(ctx, &pf, infos, "meta")
	if err != nil {
		sublog.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("DAV", "1, 3, extended-mkcol")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	var disableTus bool
	// let clients know this collection supports tus.io POST requests to start uploads
	if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		if info.Opaque != nil {
			_, disableTus = info.Opaque.Map["disable_tus"]
		}
		if !disableTus {
			w.Header().Add("Access-Control-Expose-Headers", "Tus-Resumable, Tus-Version, Tus-Extension")
			w.Header().Set("Tus-Resumable", "1.0.0")
			w.Header().Set("Tus-Version", "1.0.0")
			w.Header().Set("Tus-Extension", "creation,creation-with-upload")
		}
	}
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write([]byte(propRes)); err != nil {
		sublog.Err(err).Msg("error writing response")
	}
}
