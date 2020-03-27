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

package ocdav

import (
	"net/http"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	manager "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/token"
	tokenmngr "github.com/cs3org/reva/pkg/token/manager/jwt"
	"github.com/cs3org/reva/pkg/user"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/metadata"
)

// PublicFilesHandler handles public files requests
type PublicFilesHandler struct{}

// Handler implements http.Handler interface
func (p *PublicFilesHandler) Handler(s *svc) http.Handler {
	mgr, err := tokenmngr.New(map[string]interface{}{})
	if err != nil {
		log.Fatal().Err(err).Msg("PublicFiles token manager setup")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		// https://tools.ietf.org/html/rfc4918#section-9.1.3
		case "PROPFIND":
			propFindRequestBody, status, err := readPropfind(r.Body)
			if err != nil {
				log.Error().Err(err).Msg("error reading propfind request")
				w.WriteHeader(status)
				return
			}

			conn, err := s.getClient()
			if err != nil {
				log.Error().Err(err).Msg("error getting grpc client")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			psRequestByToken := manager.GetPublicShareByTokenRequest{
				Token: getRequestToken(r.URL.Path),
			}

			publicShareResponse, err := conn.GetPublicShareByToken(r.Context(), &psRequestByToken)
			if err != nil {
				log.Error().Err(err).Msg("error requesting public share")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			gur, err := conn.GetUser(r.Context(), &userpb.GetUserRequest{
				UserId: publicShareResponse.Share.Owner,
			})
			if err != nil {
				log.Error().Err(err).Msg("get user")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			tkn, err := mgr.MintToken(r.Context(), gur.User)
			if err != nil {
				log.Error().Err(err).Msg("mint token")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			ctx := user.ContextSetUser(r.Context(), gur.User)
			ctx = token.ContextSetToken(ctx, tkn)
			ctx = metadata.AppendToOutgoingContext(ctx, "x-access-token", tkn)

			ref := &provider.Reference{
				Spec: &provider.Reference_Id{
					Id: publicShareResponse.GetShare().ResourceId,
				},
			}

			statReq := provider.StatRequest{
				Ref: ref,
			}

			statResponse, err := conn.Stat(ctx, &statReq)
			if err != nil {
				log.Error().Err(err).Msg("error during stat call")
				return
			}

			info := statResponse.GetInfo()
			infos := []*provider.ResourceInfo{info}

			if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
				req := &provider.ListContainerRequest{
					Ref: ref,
				}
				res, err := conn.ListContainer(ctx, req)
				if err != nil {
					log.Error().Err(err).Msg("error sending list container grpc request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if res.Status.Code != rpc.Code_CODE_OK {
					log.Err(err).Msg("error calling grpc list container")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				infos = append(infos, res.Infos...)
			}

			propRes, err := s.formatPropfind(ctx, &propFindRequestBody, infos, "")
			if err != nil {
				log.Error().Err(err).Msg("error formatting propfind")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("DAV", "1, 3, extended-mkcol")
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			w.WriteHeader(http.StatusMultiStatus)
			_, err = w.Write([]byte(propRes))
			if err != nil {
				log.Error().Err(err).Msg("error writing body")
				return
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		w.WriteHeader(http.StatusNotImplemented)
	})
}

// extracts the share token from the url. /#/{token} -> // {token}
// TODO(refs) unit test this
func getRequestToken(path string) string {
	return strings.Split(path, "/")[1]
}
