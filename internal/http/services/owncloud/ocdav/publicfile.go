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
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"path"
	"path/filepath"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/propfind"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/rhttp/router"
	rtrace "github.com/cs3org/reva/v2/pkg/trace"
	"google.golang.org/grpc/metadata"
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

			switch r.Method {
			case MethodPropfind:
				s.handlePropfindOnToken(w, r, h.namespace, false)
			case http.MethodGet:
				s.handlePathGet(w, r, h.namespace)
			case http.MethodOptions:
				s.handleOptions(w, r)
			case http.MethodHead:
				s.handlePathHead(w, r, h.namespace)
			case http.MethodPut:
				s.handlePathPut(w, r, h.namespace)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		} else {
			// accessing the virtual parent folder
			switch r.Method {
			case MethodPropfind:
				s.handlePropfindOnToken(w, r, h.namespace, true)
			case http.MethodOptions:
				s.handleOptions(w, r)
			case http.MethodHead:
				s.handlePathHead(w, r, h.namespace)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}
	})
}

// TokenInfo contains information about the token
type TokenInfo struct {
	Token             string `xml:"token"`
	LinkURL           string `xml:"linkurl"`
	PasswordProtected bool   `xml:"passwordprotected"`

	StorageID string
	OpaqueID  string
	Path      string
}

// HandleGetToken will return details about the token. NOTE: this endpoint is publicly available.
func (s *svc) HandleGetToken(w http.ResponseWriter, r *http.Request) {
	c, err := pool.GetGatewayServiceClient(s.c.GatewaySvc)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	tkn, _ := router.ShiftPath(r.URL.Path)
	t := TokenInfo{Token: tkn}

	{
		ctx := context.Background()
		// get token details - if possible
		q := r.URL.Query()
		sig := q.Get("signature")
		expiration := q.Get("expiration")
		// We restrict the pre-signed urls to downloads.
		if sig != "" && expiration != "" && r.Method != http.MethodGet {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		res, err := handleSignatureAuth(ctx, c, tkn, sig, expiration)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch res.Status.Code {
		case rpc.Code_CODE_OK:
			// nothing to do
		case rpc.Code_CODE_PERMISSION_DENIED:
			if res.Status.Message == "wrong password" {
				t.PasswordProtected = true
				b, err := xml.Marshal(t)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Write(b)
				w.WriteHeader(http.StatusOK)
				return
			}
			fallthrough
		default:
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		ctx = ctxpkg.ContextSetToken(ctx, res.Token)
		ctx = ctxpkg.ContextSetUser(ctx, res.User)
		ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, res.Token)

		r = r.WithContext(ctx)
		sRes, err := getTokenStatInfo(ctx, c, tkn)
		if err != nil || sRes.Status.Code != rpc.Code_CODE_OK {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		ls := &link.PublicShare{}
		_ = json.Unmarshal(sRes.Info.Opaque.Map["link-share"].Value, ls)

		t.StorageID = ls.ResourceId.GetStorageId()
		t.OpaqueID = ls.ResourceId.GetOpaqueId()

		baseURI, ok := ctx.Value(net.CtxKeyBaseURI).(string)
		if ok {
			ref := path.Join(baseURI, sRes.Info.Path)
			if sRes.Info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
				ref += "/"
			}
			t.LinkURL = ref
		}
	}

	{
		u, ok := ctxpkg.ContextGetUser(r.Context())
		if ok {
			//ref := &provider.Reference{
			//ResourceId: &provider.ResourceId{StorageId: t.StorageID, OpaqueId: t.OpaqueID},
			//Path:       t.Path,
			//}
			ref := &provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: t.StorageID, // "4c510ada-c86b-4815-8820-42cdf82c3d51",
					OpaqueId:  "1fe0481e-7bbb-4fb3-bb8f-a21231cf9e92",
				},
				Path: "",
			}
			ctx := context.Background()
			ctx = ctxpkg.ContextSetUser(ctx, u)
			res, err := c.Stat(r.Context(), &provider.StatRequest{
				Ref: ref})
			fmt.Println(res, err)
		}

	}

	b, err := xml.Marshal(t)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(b)
	w.WriteHeader(http.StatusOK)
	return
}

// ns is the namespace that is prefixed to the path in the cs3 namespace
func (s *svc) handlePropfindOnToken(w http.ResponseWriter, r *http.Request, ns string, onContainer bool) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "token_propfind")
	defer span.End()

	tokenStatInfo := ctx.Value(tokenStatInfoKey{}).(*provider.ResourceInfo)
	sublog := appctx.GetLogger(ctx).With().Interface("tokenStatInfo", tokenStatInfo).Logger()
	sublog.Debug().Msg("handlePropfindOnToken")

	dh := r.Header.Get(net.HeaderDepth)
	depth, err := net.ParseDepth(dh)
	if err != nil {
		sublog.Debug().Msg(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pf, status, err := propfind.ReadPropfind(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	// prefix tokenStatInfo.Path with token
	tokenStatInfo.Path = filepath.Join(r.URL.Path, tokenStatInfo.Path)

	infos := s.getPublicFileInfos(onContainer, depth == net.DepthZero, tokenStatInfo)

	propRes, err := propfind.MultistatusResponse(ctx, &pf, infos, s.c.PublicURL, ns, nil)
	if err != nil {
		sublog.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set(net.HeaderDav, "1, 3, extended-mkcol")
	w.Header().Set(net.HeaderContentType, "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write(propRes); err != nil {
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

	// add the file info
	infos = append(infos, i)

	return infos
}
