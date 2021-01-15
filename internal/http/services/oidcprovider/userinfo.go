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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ory/fosite"
	"google.golang.org/grpc/metadata"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/token"
)

func (s *svc) doUserinfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	requiredScope := "openid"

	_, ar, err := s.oauth2.IntrospectToken(ctx, fosite.AccessTokenFromRequest(r), fosite.AccessToken, s.getEmptySession(), requiredScope)
	if err != nil {
		fmt.Fprintf(w, "<h1>An error occurred!</h1><p>Could not perform introspection: %v</p>", err)
		return
	}

	log.Debug().Interface("ar", ar).Msg("introspected")

	// sub is uid.OpaqueId and issuer is uid.Provider
	session, ok := ar.GetSession().(*customSession)
	if !ok {
		log.Error().Msg("session is not of type *customSession")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	internalToken := session.internalToken // To include in the context.
	ctx = token.ContextSetToken(ctx, internalToken)
	ctx = metadata.AppendToOutgoingContext(ctx, token.TokenHeader, internalToken) // TODO(labkode): this sucks.
	sub := session.GetSubject()

	fmt.Printf("internal token: %s subject: %s session:%+v", internalToken, sub, session)
	issuer := s.conf.Issuer

	uid := &user.UserId{
		// TODO(labkode): how to get issuer from session? we store it in newSession,
		// so we should be able to get it somehow ... we can use customSession now :)
		// For the time being, the issuer is set in the configuration of the service.
		// TODO(jfd): also fill the idp, if possible.
		// well .. that might be hard, because in this case we are the idp
		// we should put oar hostname into the Iss field
		// - only an oidc provider would be able to provide an iss
		// - maybe for ldap the uidNumber attribute makes more sense as sub?
		//    - this is still a tricky question. ms eg uses sid - security identifiers
		//      but they change when a username changes or he moves to a new node in the tree
		//      to mitigate this they keep track of past ids in the sidHistory attribute
		//      so the filesystem might use an outdated sid in the permissions but the system
		//      can still resolve the user using the sidhistory attribute
		OpaqueId: sub,
		Idp:      issuer,
	}

	// Needs to be an authenticated request, for such reason we need to store the internal reva token
	// in the user session using a custom session.
	c, err := pool.GetGatewayServiceClient(s.conf.GatewayEndpoint)
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway service client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	getUserReq := &user.GetUserRequest{
		UserId: uid,
	}
	getUserRes, err := c.GetUser(ctx, getUserReq)
	if err != nil {
		log.Err(err).Msg("error calling GetUser")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if getUserRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(getUserRes.Status.Code, "oidcprovider")
		log.Err(err).Msg("error getting user information")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user := getUserRes.User
	sc := &StandardClaims{
		Sub: user.Id.OpaqueId,
		// TODO(labkode): Iss is overwritten by config
		Iss:               s.conf.Issuer,
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
