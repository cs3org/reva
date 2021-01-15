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

package oidcprovider

import (
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/ory/fosite"
)

func (s *svc) doToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	// This will create an access request object and iterate through the registered TokenEndpointHandlers to validate the request.
	accessRequest, err := s.oauth2.NewAccessRequest(ctx, r, s.getEmptySession())

	// Catch any errors, e.g.:
	// * unknown client
	// * invalid redirect
	// * ...
	if err != nil {
		log.Error().Err(err).Msg("Error occurred in NewAccessRequest")
		s.oauth2.WriteAccessError(w, accessRequest, err)
		return
	}

	// If this is a client_credentials grant, grant all scopes the client is allowed to perform.
	if accessRequest.GetGrantTypes().ExactOne("client_credentials") {
		for _, scope := range accessRequest.GetRequestedScopes() {
			if fosite.HierarchicScopeStrategy(accessRequest.GetClient().GetScopes(), scope) {
				accessRequest.GrantScope(scope)
			}
		}
	}

	// Next we create a response for the access request. Again, we iterate through the TokenEndpointHandlers
	// and aggregate the result in response.
	response, err := s.oauth2.NewAccessResponse(ctx, accessRequest)
	if err != nil {
		log.Error().Err(err).Msg("Error occurred in NewAccessResponse")
		s.oauth2.WriteAccessError(w, accessRequest, err)
		return
	}

	// All done, send the response.
	s.oauth2.WriteAccessResponse(w, accessRequest, response)

	// The client now has a valid access token
}
