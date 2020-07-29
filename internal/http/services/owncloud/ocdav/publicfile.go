// Copyright 2018-2020 CERN
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

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"go.opencensus.io/trace"
)

// PublicFileHandler handles trashbin requests
type PublicFileHandler struct {
	namespace string
}

func (h *PublicFileHandler) init(ns string) error {
	h.namespace = path.Join("/", ns)
	return nil
}

// Handler handles requests
func (h *PublicFileHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		_, relativePath := router.ShiftPath(r.URL.Path)

		log.Debug().Str("relativePath", relativePath).Msg("PublicFileHandler func")

		if relativePath != "" && relativePath != "/" {
			// accessing the file
			// PROPFIND has an implicit call
			if r.Method != "PROPFIND" && !s.adjustResourcePathInURL(w, r) {
				return
			}

			r.URL.Path = path.Base(r.URL.Path)
			switch r.Method {
			case "PROPFIND":
				s.handlePropfindOnToken(w, r, h.namespace, false)
			case http.MethodGet:
				s.handleGet(w, r, h.namespace)
			case http.MethodOptions:
				s.handleOptions(w, r, h.namespace)
			case http.MethodHead:
				s.handleHead(w, r, h.namespace)
			case http.MethodPut:
				s.handlePut(w, r, h.namespace)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		} else {
			// accessing the virtual parent folder
			switch r.Method {
			case "PROPFIND":
				s.handlePropfindOnToken(w, r, h.namespace, true)
			case http.MethodOptions:
				s.handleOptions(w, r, h.namespace)
			case http.MethodHead:
				s.handleHead(w, r, h.namespace)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}
	})
}

func (s *svc) adjustResourcePathInURL(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	// find actual file name
	log := appctx.GetLogger(ctx)
	tokenStatInfo := ctx.Value(tokenStatInfoKey{}).(*provider.ResourceInfo)
	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}
	pathRes, err := client.GetPath(ctx, &provider.GetPathRequest{
		ResourceId: tokenStatInfo.GetId(),
	})
	if err != nil {
		log.Warn().
			Str("tokenStatInfo.Id", tokenStatInfo.GetId().String()).
			Str("tokenStatInfo.Path", tokenStatInfo.Path).
			Msg("Could not get path of resource")
		w.WriteHeader(http.StatusNotFound)
		return false
	}
	if path.Base(r.URL.Path) != path.Base(pathRes.Path) {
		w.WriteHeader(http.StatusNotFound)
		return false
	}

	// adjust path in request URL to point at the parent
	r.URL.Path = path.Dir(r.URL.Path)

	return true
}

// ns is the namespace that is prefixed to the path in the cs3 namespace
func (s *svc) handlePropfindOnToken(w http.ResponseWriter, r *http.Request, ns string, onContainer bool) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "propfind")
	defer span.End()
	log := appctx.GetLogger(ctx)

	tokenStatInfo := ctx.Value(tokenStatInfoKey{}).(*provider.ResourceInfo)
	log.Debug().Interface("tokenStatInfo", tokenStatInfo).Msg("handlePropfindOnToken")

	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "1"
	}

	// see https://tools.ietf.org/html/rfc4918#section-10.2
	if depth != "0" && depth != "1" && depth != "infinity" {
		log.Error().Msgf("invalid Depth header value %s", depth)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pf, status, err := readPropfind(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// find actual file name
	pathRes, err := client.GetPath(ctx, &provider.GetPathRequest{
		ResourceId: tokenStatInfo.GetId(),
	})
	if err != nil {
		log.Warn().
			Str("tokenStatInfo.Id", tokenStatInfo.GetId().String()).
			Str("tokenStatInfo.Path", tokenStatInfo.Path).
			Msg("Could not get path of resource")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	infos := []*provider.ResourceInfo{}

	if onContainer {
		// TODO: filter out metadata like favorite and arbitrary metadata
		if depth != "0" {
			// if the request is to a public link, we need to add yet another value for the file entry.
			infos = append(infos, &provider.ResourceInfo{
				// append the shared as a container. Annex to OC10 standards.
				Id:            tokenStatInfo.Id,
				Path:          tokenStatInfo.Path,
				Type:          provider.ResourceType_RESOURCE_TYPE_CONTAINER,
				Mtime:         tokenStatInfo.Mtime,
				Size:          tokenStatInfo.Size,
				Etag:          tokenStatInfo.Etag,
				PermissionSet: tokenStatInfo.PermissionSet,
			})
		}
	} else if path.Base(r.URL.Path) != path.Base(pathRes.Path) {
		// if queried on the wrong path, return not found
		w.WriteHeader(http.StatusNotFound)
		return
	}

	infos = append(infos, &provider.ResourceInfo{
		Id:            tokenStatInfo.Id,
		Path:          path.Join("/", tokenStatInfo.Path, path.Base(pathRes.Path)),
		Type:          tokenStatInfo.Type,
		Size:          tokenStatInfo.Size,
		MimeType:      tokenStatInfo.MimeType,
		Mtime:         tokenStatInfo.Mtime,
		Etag:          tokenStatInfo.Etag,
		PermissionSet: tokenStatInfo.PermissionSet,
	})

	propRes, err := s.formatPropfind(ctx, &pf, infos, ns)
	if err != nil {
		log.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("DAV", "1, 3, extended-mkcol")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write([]byte(propRes)); err != nil {
		log.Err(err).Msg("error writing response")
	}
}
