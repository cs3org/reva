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

package wellknown

import (
	"encoding/json"
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth/manager/oidc"
)

func (s *svc) doOpenidConfiguration(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())
	pm := &oidc.ProviderMetadata{
		Issuer:                s.conf.Issuer,
		AuthorizationEndpoint: s.conf.AuthorizationEndpoint,
		JwksURI:               s.conf.JwksURI,
		TokenEndpoint:         s.conf.TokenEndpoint,
		RevocationEndpoint:    s.conf.RevocationEndpoint,
		IntrospectionEndpoint: s.conf.IntrospectionEndpoint,
		UserinfoEndpoint:      s.conf.UserinfoEndpoint,
		EndSessionEndpoint:    s.conf.EndSessionEndpoint,
	}

	b, err := json.Marshal(pm)
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(b)
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
		return
	}
}
