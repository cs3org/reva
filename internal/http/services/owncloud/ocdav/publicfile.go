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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"go.opencensus.io/trace"
)

// PublicFileHandler handles requests on a shared file. it needs to be wrapped in a collection
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
	ctx, span := trace.StartSpan(ctx, "adjustResourcePathInURL")
	defer span.End()

	// find actual file name
	tokenStatInfo := ctx.Value(tokenStatInfoKey{}).(*provider.ResourceInfo)
	sublog := appctx.GetLogger(ctx).With().Interface("tokenStatInfo", tokenStatInfo).Logger()

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}
	pathRes, err := client.GetPath(ctx, &provider.GetPathRequest{
		ResourceId: tokenStatInfo.GetId(),
	})
	if err != nil {
		sublog.Error().Msg("Could not get path of resource")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}
	if pathRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, pathRes.Status)
		return false
	}
	if path.Base(r.URL.Path) != path.Base(pathRes.Path) {
		sublog.Debug().
			Str("requestbase", path.Base(r.URL.Path)).
			Str("pathbase", path.Base(pathRes.Path)).
			Msg("base paths don't match")
		w.WriteHeader(http.StatusConflict)
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

	tokenStatInfo := ctx.Value(tokenStatInfoKey{}).(*provider.ResourceInfo)
	sublog := appctx.GetLogger(ctx).With().Interface("tokenStatInfo", tokenStatInfo).Logger()
	sublog.Debug().Msg("handlePropfindOnToken")

	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "1"
	}

	// see https://tools.ietf.org/html/rfc4918#section-10.2
	if depth != "0" && depth != "1" && depth != "infinity" {
		sublog.Debug().Msgf("invalid Depth header value %s", depth)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

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

	// find actual file name
	pathRes, err := client.GetPath(ctx, &provider.GetPathRequest{
		ResourceId: tokenStatInfo.GetId(),
	})
	if err != nil {
		sublog.Warn().Msg("Could not get path of resource")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if pathRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, pathRes.Status)
		return
	}

	if !onContainer && path.Base(r.URL.Path) != path.Base(pathRes.Path) {
		// if queried on the wrong path, return not found
		w.WriteHeader(http.StatusNotFound)
		return
	}
	// adjust path
	tokenStatInfo.Path = path.Join("/", tokenStatInfo.Path, path.Base(pathRes.Path))

	infos := s.getPublicFileInfos(onContainer, depth == "0", tokenStatInfo)

	propRes, err := s.formatPropfind(ctx, &pf, infos, ns)
	if err != nil {
		sublog.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("DAV", "1, 3, extended-mkcol")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write([]byte(propRes)); err != nil {
		sublog.Err(err).Msg("error writing response")
	}
}

// there are only two possible entries
// 1. the non existing collection
// 2. the shared file
func (s *svc) getPublicFileInfos(onContainer, onlyRoot bool, i *provider.ResourceInfo) []*provider.ResourceInfo {
	infos := []*provider.ResourceInfo{}
	if onContainer {
		// copy link-share data if present
		// we don't copy everything because the checksum should not be present
		var o *typesv1beta1.Opaque
		if i.Opaque != nil && i.Opaque.Map != nil && i.Opaque.Map["link-share"] != nil {
			o = &typesv1beta1.Opaque{
				Map: map[string]*typesv1beta1.OpaqueEntry{
					"link-share": i.Opaque.Map["link-share"],
				},
			}
		}
		// always add collection
		infos = append(infos, &provider.ResourceInfo{
			// Opaque carries the link-share data we need when rendering the collection root href
			Opaque: o,
			Path:   path.Dir(i.Path),
			Type:   provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		})
		if onlyRoot {
			return infos
		}
	}

	// link share only appears on root collection
	delete(i.Opaque.Map, "link-share")

	// add the file info
	infos = append(infos, i)

	return infos
}
