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
	"fmt"
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

func (s *svc) doAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	// Let's create an AuthorizeRequest object!
	// It will analyze the request and extract important information like scopes, response type and others.
	ar, err := s.oauth2.NewAuthorizeRequest(ctx, r)
	if err != nil {
		log.Error().Err(err).Msg("Error occurred in NewAuthorizeRequest")
		s.oauth2.WriteAuthorizeError(w, ar, err)
		return
	}

	// You have now access to authorizeRequest, Code ResponseTypes, Scopes ...
	var requestedScopes string
	for _, this := range ar.GetRequestedScopes() {
		requestedScopes += fmt.Sprintf(`<li><label><input type="checkbox" name="scopes" value="%s" checked>%s</label></li>`, this, this)
	}

	// Normally, this would be the place where you would check if the user is logged in and gives his consent.
	// We're simplifying things and just checking if the request includes a valid username and password
	if err := r.ParseForm(); err != nil {
		log.Error().Err(err).Msg("Error occurred parsing the form data")
		s.oauth2.WriteAuthorizeError(w, ar, err)
		return
	}

	username := r.PostForm.Get("username")
	password := r.PostForm.Get("password")

	// If no username is set we send to the user an HTML form fill.
	// MVC is dead, long live MCV.
	if username == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, err := w.Write([]byte(fmt.Sprintf(`
			<h1>Login page</h1>
			<p>Howdy! This is the log in page. For this example, it is enough to supply the username.</p>
			<form method="post">
				<p>
					By logging in, you consent to grant these scopes:
					<ul>%s</ul>
				</p>
				<input type="text" name="username" placeholder="Username" autofocus="autofocus"/><br>
				<input type="password" name="password" placeholder="Password"/><br>
				<input type="submit">
			</form>
		`, requestedScopes)))
		if err != nil {
			log.Error().Err(err).Msg("Error writing response")
			s.oauth2.WriteAuthorizeError(w, ar, err)
		}
		return
	}

	c, err := pool.GetGatewayServiceClient(s.conf.GatewayEndpoint)
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway service client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO(labkode): the auth type should be configurable in the config file.
	authReq := &gateway.AuthenticateRequest{
		Type:         "basic", // we are sending username and password -> basic auth
		ClientId:     username,
		ClientSecret: password,
	}
	authRes, err := c.Authenticate(ctx, authReq)
	if err != nil {
		log.Err(err).Msg("error calling Authenticate")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if authRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(authRes.Status.Code, "oidcprovider")
		log.Err(err).Msg("error authenticating client credentials")
		// TODO(labkode): maybe oauth response is better
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	/* TODO(labkode): I think is not needed anymore because
	// the auth returns a full user object already.

	// Once the authentication is successful, we have a user id that has been
	// validated, to fill other fields we need the user information also.
	getUserReq := &userpb.GetUserRequest{
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
	*/

	// let's see what scopes the user gave consent to
	for _, scope := range r.PostForm["scopes"] {
		ar.GrantScope(scope)
	}

	// Now that the user is authorized, we set up a session:
	mySessionData := s.getSession(authRes.Token, authRes.User)

	// When using the HMACSHA strategy you must use something that implements the HMACSessionContainer.
	// It brings you the power of overriding the default values.
	//
	// mySessionData.HMACSession = &strategy.HMACSession{
	//	AccessTokenExpiry: time.Now().Add(time.Day),
	//	AuthorizeCodeExpiry: time.Now().Add(time.Day),
	// }
	//

	// If you're using the JWT strategy, there's currently no distinction between access token and authorize code claims.
	// Therefore, you both access token and authorize code will have the same "exp" claim. If this is something you
	// need let us know on github.
	//
	// mySessionData.JWTClaims.ExpiresAt = time.Now().Add(time.Day)

	// It's also wise to check the requested scopes, e.g.:
	// if authorizeRequest.GetScopes().Has("admin") {
	//     http.Error(rw, "you're not allowed to do that", http.StatusForbidden)
	//     return
	// }

	// Now we need to get a response. This is the place where the AuthorizeEndpointHandlers kick in and start processing the request.
	// NewAuthorizeResponse is capable of running multiple response type handlers which in turn enables this library
	// to support open id connect.
	response, err := s.oauth2.NewAuthorizeResponse(ctx, ar, mySessionData)

	// Catch any errors, e.g.:
	// * unknown client
	// * invalid redirect
	// * ...
	if err != nil {
		log.Error().Msgf("error unstack: %+v", err)
		log.Error().Err(err).Msg("oidcprovider: error occurred in NewAuthorizeResponse")
		s.oauth2.WriteAuthorizeError(w, ar, err)
		return
	}

	// Last but not least, send the response!
	s.oauth2.WriteAuthorizeResponse(w, ar, response)
}
