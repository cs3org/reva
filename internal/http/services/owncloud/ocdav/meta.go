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
	"encoding/xml"
	"fmt"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/router"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"net/http"
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
		switch head {
		case "v":
			h.VersionsHandler.Handler(s, did).ServeHTTP(w, r)
		default:
			h.doGetPath(w, r, s, did)
		}
	})
}

func (h *MetaHandler) doGetPath(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "getPath")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Interface("resourceid", rid).Logger()

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	pathRes, err := client.GetPath(ctx, &provider.GetPathRequest{ResourceId: rid})
	if err != nil {
		sublog.Error().Err(err).Msg("error sending GetPath grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch pathRes.Status.Code {
	case rpc.Code_CODE_NOT_FOUND:
		w.WriteHeader(http.StatusNotFound)
		b, err := Marshal(exception{
			code: SabredavNotFound,
		})
		HandleWebdavError(&sublog, w, b, err)
		return
	case rpc.Code_CODE_PERMISSION_DENIED:
		w.WriteHeader(http.StatusForbidden)
		b, err := Marshal(exception{
			code: SabredavPermissionDenied,
		})
		HandleWebdavError(&sublog, w, b, err)
		return
	}

	response := responseXML{
		// static... umgh... is there a method to get the path?
		Href: fmt.Sprintf("/remote.php/dav/meta/%s/", wrapResourceID(rid)),
		Propstat: []propstatXML{
			propstatXML{
				Status: "HTTP/1.1 200 OK",
				Prop: []*propertyXML{
					// pathRes.Path contains /users/..id../.. in response
					s.newProp("oc:meta-path-for-user", pathRes.Path),
				},
			},
		},
	}

	responseXML, err := xml.Marshal(&response)
	if err != nil {
		sublog.Error().Err(err).Msg("error marshaling GetPath responseXML")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg := `<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:" `
	msg += `xmlns:s="http://sabredav.org/ns" xmlns:oc="http://owncloud.org/ns">`
	msg += string(responseXML) + `</d:multistatus>`

	w.Header().Set(HeaderDav, "1, 3, extended-mkcol")
	w.Header().Set(HeaderContentType, "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, err = w.Write([]byte(msg))
	if err != nil {
		sublog.Error().Err(err).Msg("error writing body")
		return
	}
}
