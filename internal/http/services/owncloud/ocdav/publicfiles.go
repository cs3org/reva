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
)

// PublicFilesHandler handles public files requests
type PublicFilesHandler struct{}

// Handler implements http.Handler interface
func (p *PublicFilesHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		// https://tools.ietf.org/html/rfc4918#section-9.1.3
		case "PROPFIND":
			w.WriteHeader(http.StatusNotImplemented)
			// // pf, status, err := readPropfind(r.Body)
			// // if err != nil {
			// // 	log.Error().Err(err).Msg("error reading propfind request")
			// // 	w.WriteHeader(status)
			// // 	return
			// // }

			// c, err := s.getClient()
			// if err != nil {
			// 	log.Error().Err(err).Msg("error getting grpc client")
			// 	w.WriteHeader(http.StatusInternalServerError)
			// 	return
			// }

			// // - query the publicShareProvider looking for such id
			// psRequest := publicshareproviderv0alphapb.GetPublicShareRequest{
			// 	Ref: &publicshareproviderv0alphapb.PublicShareReference{
			// 		Spec: &publicshareproviderv0alphapb.PublicShareReference_Token{
			// 			Token: shareTokenFromURL(r.URL),
			// 		},
			// 	},
			// }

			// // TODO(refs) whitelist requesting a public share on interceptors? either that or don't enable auth interceptor on whatever service exposes public shares (in this case the gateway)
			// publicShareResponse, err := c.GetPublicShare(r.Context(), &psRequest)
			// if err != nil {
			// 	log.Error().Err(err).Msg("error getting grpc client")
			// 	w.WriteHeader(http.StatusInternalServerError)
			// 	return
			// }
			// fmt.Println(publicShareResponse)
			// // - shift path to get the id of the public share in the url
			// // - do DAV response
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		w.WriteHeader(http.StatusNotImplemented)
	})
}

// func shareTokenFromURL(u *url.URL) string {
// 	return strings.Split(u.Path, "/")[1] // remove prefixed "/"
// }
