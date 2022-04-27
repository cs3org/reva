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
	"net/http"
	"path"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/prop"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/propfind"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/rhttp/router"
	rtrace "github.com/cs3org/reva/v2/pkg/trace"
	"github.com/cs3org/reva/v2/pkg/utils/resourceid"
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
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		did := resourceid.OwnCloudResourceIDUnwrap(id)
		if did == nil {
			logger := appctx.GetLogger(r.Context())
			logger.Debug().Str("prop", net.PropOcMetaPathForUser).Msg("invalid resource id")
			w.WriteHeader(http.StatusBadRequest)
			m := fmt.Sprintf("Invalid resource id %v", id)
			b, err := errors.Marshal(http.StatusBadRequest, m, "")
			errors.HandleWebdavError(logger, w, b, err)
			return
		}

		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		switch head {
		case "":
			if r.Method != MethodPropfind {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			h.handlePathForUser(w, r, s, did)
		case "v":
			h.VersionsHandler.Handler(s, did).ServeHTTP(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}

	})
}

func (h *MetaHandler) handlePathForUser(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "meta_propfind")
	defer span.End()

	id := resourceid.OwnCloudResourceIDWrap(rid)
	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("resourceid", id).Logger()
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	pf, status, err := propfind.ReadPropfind(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	if ok := hasProp(&pf, net.PropOcMetaPathForUser); !ok {
		sublog.Debug().Str("prop", net.PropOcMetaPathForUser).Msg("error finding prop in request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pathReq := &provider.GetPathRequest{ResourceId: rid}
	pathRes, err := client.GetPath(ctx, pathReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending GetPath grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch pathRes.Status.Code {
	case rpc.Code_CODE_NOT_FOUND:
		sublog.Debug().Str("code", string(pathRes.Status.Code)).Msg("resource not found")
		w.WriteHeader(http.StatusNotFound)
		m := fmt.Sprintf("Resource %s not found", id)
		b, err := errors.Marshal(http.StatusNotFound, m, "")
		errors.HandleWebdavError(&sublog, w, b, err)
		return
	case rpc.Code_CODE_PERMISSION_DENIED:
		// raise StatusNotFound so that resources can't be enumerated
		sublog.Debug().Str("code", string(pathRes.Status.Code)).Msg("resource access denied")
		w.WriteHeader(http.StatusNotFound)
		m := fmt.Sprintf("Resource %s not found", id)
		b, err := errors.Marshal(http.StatusNotFound, m, "")
		errors.HandleWebdavError(&sublog, w, b, err)
		return
	}

	propstatOK := propfind.PropstatXML{
		Status: "HTTP/1.1 200 OK",
		Prop: []prop.PropertyXML{
			prop.Escaped("oc:meta-path-for-user", pathRes.Path),
		},
	}
	baseURI := ctx.Value(net.CtxKeyBaseURI).(string)
	msr := propfind.NewMultiStatusResponseXML()
	msr.Responses = []*propfind.ResponseXML{
		{
			Href: net.EncodePath(path.Join(baseURI, id) + "/"),
			Propstat: []propfind.PropstatXML{
				propstatOK,
			},
		},
	}
	propRes, err := xml.Marshal(msr)
	if err != nil {
		sublog.Error().Err(err).Msg("error marshalling propfind response xml")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set(net.HeaderDav, "1, 3, extended-mkcol")
	w.Header().Set(net.HeaderContentType, "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write(propRes); err != nil {
		sublog.Error().Err(err).Msg("error writing propfind response")
		return
	}
}

func hasProp(pf *propfind.XML, key string) bool {
	for i := range pf.Prop {
		k := fmt.Sprintf("%s/%s", pf.Prop[i].Space, pf.Prop[i].Local)
		if k == key {
			return true
		}
	}
	return false
}
