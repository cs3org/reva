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

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	manager "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	"github.com/cs3org/reva/pkg/token"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/metadata"
)

// PublicFilesHandler handles public files requests
type PublicFilesHandler struct{}

// Handler implements http.Handler interface
func (p *PublicFilesHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		// https://tools.ietf.org/html/rfc4918#section-9.1.3
		case "PROPFIND":
			// - get a gateway client
			// - validate against a regexp the url is valid, since we do no routing
			// - get the URL path (i.e: /#/s/token) -> "token"
			// - use "token" to query the public share provider by token
			// - get public share
			// - prepare response
			// 	- get the file info (stat -> storageprovider)
			// - set response headers
			// - send response to phoenix
			// - add validations
			// 	- is the public share protected by password?
			// 	- is the public share still valid in time?
			// 	- if none of the above tests pass -> what do we return, not found? invalid?

			pf, status, err := readPropfind(r.Body)
			if err != nil {
				log.Error().Err(err).Msg("error reading propfind request")
				w.WriteHeader(status)
				return
			}

			gwClient, err := s.getClient()
			if err != nil {
				log.Error().Err(err).Msg("error getting grpc client")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// TODO(refs) authenticate request. use plain text user credentials temporarily, later on use resource owner credentials, and ideally not basic auth
			authRequest := gateway.AuthenticateRequest{
				ClientId:     "einstein",
				ClientSecret: "relativity",
				Type:         "basic",
			}

			authResponse, err := gwClient.Authenticate(r.Context(), &authRequest)
			if err != nil {
				log.Error().Err(err).Msg("error authenticating resource owner")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			psRequestByToken := manager.GetPublicShareByTokenRequest{
				Token: getRequestToken(r.URL.Path),
			}

			publicShareResponse, err := gwClient.GetPublicShareByToken(r.Context(), &psRequestByToken)
			if err != nil {
				log.Error().Err(err).Msg("error requesting public share")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// now that we got the share we need to get the resource info
			statReq := provider.StatRequest{
				Ref: &provider.Reference{
					Spec: &provider.Reference_Id{
						Id: publicShareResponse.GetShare().ResourceId,
					},
				},
			}

			ctx := token.ContextSetToken(r.Context(), authResponse.GetToken())
			ctx = metadata.AppendToOutgoingContext(ctx, "x-access-token", authResponse.GetToken())

			statResponse, err := gwClient.Stat(ctx, &statReq)
			if err != nil {
				log.Error().Err(err).Msg("error during stat call")
				return
			}

			sInfo := []*provider.ResourceInfo{statResponse.GetInfo()}
			// now prepare the dav response with the resource info
			propRes, err := s.formatPropfind(ctx, &pf, sInfo, "")
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
