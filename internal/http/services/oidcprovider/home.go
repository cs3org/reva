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

	"github.com/cs3org/reva/pkg/appctx"
	goauth "golang.org/x/oauth2"
)

// A valid oauth2 client (check the store) that additionally requests an OpenID Connect id token
var clientConf = goauth.Config{
	ClientID:     "my-client",
	ClientSecret: "foobar",
	RedirectURL:  "http://localhost:9998/callback",
	Scopes:       []string{"photos", "openid", "offline"},
	Endpoint: goauth.Endpoint{
		TokenURL: "http://localhost:9998/oauth2/token",
		AuthURL:  "http://localhost:9998/oauth2/auth",
	},
}

func (s *svc) doHome(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())
	_, err := w.Write([]byte(fmt.Sprintf(`
	<p>You can obtain an access token using various methods</p>
	<ul>
		<li>
			<a href="%s">Authorize code grant (with OpenID Connect)</a>
		</li>
		<li>
			<a href="%s">Implicit grant (with OpenID Connect)</a>
		</li>
		<li>
			<a href="/client">Client credentials grant</a>
		</li>
		<li>
			<a href="/owner">Resource owner password credentials grant</a>
		</li>
		<li>
			<a href="%s">Refresh grant</a>. <small>You will first see the login screen which is required to obtain a valid refresh token.</small>
		</li>
		<li>
			<a href="%s">Make an invalid request</a>
		</li>
	</ul>`,
		// TODO(jfd): make sure phoenix uses random state and nonce, see https://tools.ietf.org/html/rfc6819#section-4.4.1.8
		// - nonce vs jti https://security.stackexchange.com/a/188171
		// - state vs nonce https://stackoverflow.com/a/46859861
		clientConf.AuthCodeURL("some-random-state-foobar")+"&nonce=some-random-nonce",
		"http://localhost:9998/oauth2/auth?client_id=my-client&redirect_uri=http%3A%2F%2Flocalhost%3A9998%2Fcallback&response_type=token%20id_token&scope=fosite%20openid&state=some-random-state-foobar&nonce=some-random-nonce",
		clientConf.AuthCodeURL("some-random-state-foobar")+"&nonce=some-random-nonce",
		"/oauth2/auth?client_id=my-client&scope=fosite&response_type=123&redirect_uri=http://localhost:9998/callback",
	)))
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
	}
}
