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

	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth/manager/oidc"
)

func (s *svc) doUserinfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	requiredScope := "openid"

	_, ar, err := s.oauth2.IntrospectToken(ctx, fosite.AccessTokenFromRequest(r), fosite.AccessToken, emptySession(), requiredScope)
	if err != nil {
		fmt.Fprintf(w, "<h1>An error occurred!</h1><p>Could not perform introspection: %v</p>", err)
		return
	}

	log.Debug().Interface("ar", ar).Msg("introspected")

	sub := ar.GetSession().GetSubject()

	uid := &typespb.UserId{
		// TODO(jfd): also fill the idp, if possible.
		// well .. that might be hard, because in this case we are the idp
		// we should put oar hostname into the Iss field
		// - only an oidc provider would be able to provide an iss
		// - maybe for ldap the uidNumber attribute makes more sense as sub?
		//    - this is still a tricky question. ms eg uses sid - security identifiers
		//      but they change when a usename changes or he moves to a new node in the tree
		//      to mitigate this they keep track of past ids in the sidHistory attribute
		//      so the filesystem might use an outdated sid in the permissions but the system
		//      can still resolve the user using the sidhistory attribute
		OpaqueId: sub,
	}
	user, err := s.usermgr.GetUser(ctx, uid)
	if err != nil {
		log.Error().Err(err).Str("sub", sub).Msg("unknown user")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	sc := &oidc.StandardClaims{
		Sub:               user.Id.OpaqueId,
		Iss:               user.Id.Idp,
		PreferredUsername: user.Username,
		Name:              user.DisplayName,
		Email:             user.Mail,
	}

	b, err := json.Marshal(sc)
	if err != nil {
		log.Error().Err(err).Msg("error marshaling standard claims")
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
