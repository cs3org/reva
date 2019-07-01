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

package oidcprovider

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ory/fosite"

	"golang.org/x/oauth2/clientcredentials"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth/manager/oidc"
)

// The same thing (valid oauth2 client) but for using the client credentials grant
var appClientConf = clientcredentials.Config{
	ClientID:     "reva",
	ClientSecret: "foobar",
	Scopes:       []string{"openid profile email"},
	TokenURL:     "http://localhost:9998/oauth2/token",
}

func (s *svc) doUserinfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	requiredScope := "openid"

	_, ar, err := oauth2.IntrospectToken(ctx, fosite.AccessTokenFromRequest(r), fosite.AccessToken, emptySession(), requiredScope)
	if err != nil {
		fmt.Fprintf(w, "<h1>An error occurred!</h1><p>Could not perform introspection: %v</p>", err)
		return
	}

	log.Debug().Interface("ar", ar).Msg("introspected")

	var sc *oidc.StandardClaims
	switch ar.GetSession().GetUsername() {
	//TODO use reva specific implementation that uses existing user managers
	case "aaliyah_abernathy":
		sc = &oidc.StandardClaims{
			Name: "Aaliyah Abernathy",
		}
	case "aaliyah_adams":
		sc = &oidc.StandardClaims{
			Name: "Aaliyah Adams",
		}
	case "aaliyah_anderson":
		sc = &oidc.StandardClaims{
			Name: "Aaliyah Anderson",
		}
	default:
		log.Error().Err(err).Msg("unknown user")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sc.Sub = ar.GetSession().GetSubject()
	sc.PreferredUsername = ar.GetSession().GetUsername()
	sc.EmailVerified = true
	sc.Email = sc.PreferredUsername + "@owncloudqa.com"

	b, err := json.Marshal(sc)
	if err != nil {
		log.Error().Err(err).Msg("error marshaling standard claims")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
