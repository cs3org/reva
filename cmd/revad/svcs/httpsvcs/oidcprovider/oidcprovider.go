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
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/storage"
	"github.com/ory/fosite/token/jwt"

	"github.com/cs3org/reva/cmd/revad/httpserver"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/mitchellh/mapstructure"
)

func init() {
	httpserver.Register("oidcprovider", New)
}

type config struct {
	Prefix string `mapstructure:"prefix"`
}

type svc struct {
	prefix  string
	handler http.Handler
}

// This is an exemplary storage instance. We will add a client and a user to it so we can use these later on.
var store = newExampleStore()

func newExampleStore() *storage.MemoryStore {
	return &storage.MemoryStore{
		IDSessions: make(map[string]fosite.Requester),
		Clients: map[string]fosite.Client{
			"phoenix": &fosite.DefaultClient{
				ID:            "phoenix",
				Secret:        []byte(`$2a$10$IxMdI6d.LIRZPpSfEwNoeu4rY3FhDREsxFJXikcgdRRAStxUlsuEO`), // = "foobar"
				RedirectURIs:  []string{"http://localhost:8300/oidc-callback.html"},
				ResponseTypes: []string{"id_token", "code", "token"},
				GrantTypes:    []string{"implicit", "refresh_token", "authorization_code", "password", "client_credentials"},
				Scopes:        []string{"openid", "profile", "email", "offline"},
			},
			"reva": &fosite.DefaultClient{
				ID:            "reva",
				Secret:        []byte(`$2a$10$IxMdI6d.LIRZPpSfEwNoeu4rY3FhDREsxFJXikcgdRRAStxUlsuEO`), // = "foobar"
				ResponseTypes: []string{"id_token", "code", "token"},
				GrantTypes:    []string{"client_credentials"},
				Scopes:        []string{"openid", "profile", "email", "offline"},
			},
		},
		// TODO(jfd): implement reva specific user store that uses existing user managers
		Users: map[string]storage.MemoryUserRelation{
			"aaliyah_abernathy": {
				Username: "aaliyah_abernathy",
				Password: "secret",
			},
			"aaliyah_adams": {
				Username: "aaliyah_adams",
				Password: "secret",
			},
			"aaliyah_anderson": {
				Username: "aaliyah_anderson",
				Password: "secret",
			},
		},
		AuthorizeCodes:         map[string]storage.StoreAuthorizeCode{},
		Implicit:               map[string]fosite.Requester{},
		AccessTokens:           map[string]fosite.Requester{},
		RefreshTokens:          map[string]fosite.Requester{},
		PKCES:                  map[string]fosite.Requester{},
		AccessTokenRequestIDs:  map[string]string{},
		RefreshTokenRequestIDs: map[string]string{},
	}
}

var fconfig = new(compose.Config)

// Because we are using oauth2 and open connect id, we use this little helper to combine the two in one
// variable.
var start = compose.CommonStrategy{
	// alternatively you could use:
	//  OAuth2Strategy: compose.NewOAuth2JWTStrategy(mustRSAKey())
	// TODO(jfd): generate / read proper secret from config
	CoreStrategy: compose.NewOAuth2HMACStrategy(fconfig, []byte("some-super-cool-secret-that-nobody-knows"), nil),

	// open id connect strategy
	OpenIDConnectTokenStrategy: compose.NewOpenIDConnectStrategy(fconfig, mustRSAKey()),
}

var oauth2 = compose.Compose(
	fconfig,
	store,
	start,
	nil,

	// enabled handlers
	compose.OAuth2AuthorizeExplicitFactory,
	compose.OAuth2AuthorizeImplicitFactory,
	compose.OAuth2ClientCredentialsGrantFactory,
	compose.OAuth2RefreshTokenGrantFactory,
	compose.OAuth2ResourceOwnerPasswordCredentialsFactory,

	compose.OAuth2TokenRevocationFactory,
	compose.OAuth2TokenIntrospectionFactory,

	// be aware that open id connect factories need to be added after oauth2 factories to work properly.
	compose.OpenIDConnectExplicitFactory,
	compose.OpenIDConnectImplicitFactory,
	compose.OpenIDConnectHybridFactory,
	compose.OpenIDConnectRefreshFactory,
)

// A session is passed from the `/auth` to the `/token` endpoint. You probably want to store data like: "Who made the request",
// "What organization does that person belong to" and so on.
// For our use case, the session will meet the requirements imposed by JWT access tokens, HMAC access tokens and OpenID Connect
// ID Tokens plus a custom field

// newSession is a helper function for creating a new session. This may look like a lot of code but since we are
// setting up multiple strategies it is a bit longer.
// Usually, you could do:
//
//  session = new(fosite.DefaultSession)
func newSession(username string, sub string) *openid.DefaultSession {
	return &openid.DefaultSession{
		Claims: &jwt.IDTokenClaims{
			Issuer:  "http://localhost:9998",
			Subject: sub,
			//Audience:    []string{"https://my-client.my-application.com"},
			ExpiresAt:   time.Now().Add(time.Hour * 6),
			IssuedAt:    time.Now(),
			RequestedAt: time.Now(),
			AuthTime:    time.Now(),
		},
		Headers: &jwt.Headers{
			Extra: make(map[string]interface{}),
		},
		Subject:  sub,
		Username: username,
	}
}

// emptySession creates a session object and fills it with safe defaults
func emptySession() *openid.DefaultSession {
	return newSession("", "")
}

func mustRSAKey() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		// TODO(jfd): don't panic!
		panic(err)
	}
	return key
}

// TODO(jfd): do not fake the sub like tkis. it would change when the username changes ...
func getSub(ctx context.Context, username string) string {
	hasher := md5.New()
	_, err := hasher.Write([]byte(username))
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("Error occurred in getSub")
		return ""
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

// New returns a new oidcprovidersvc
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	s := &svc{
		prefix: conf.Prefix,
	}
	s.setHandler()
	return s, nil
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	return s.prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())

		// TODO use CORS allow access from everywhere
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var head string
		head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		log.Info().Msgf("oidcprovider routing: head=%s tail=%s", head, r.URL.Path)
		switch head {
		case "":
			s.doHome(w, r)
		case "auth":
			s.doAuth(w, r)
		case "token":
			s.doToken(w, r)
		case "revoke":
			s.doRevoke(w, r)
		case "introspect":
			s.doIntrospect(w, r)
		case "userinfo":
			s.doUserinfo(w, r)
		case "sessions":
			// TODO(jfd) make session lookup configurable? only for development?
			s.doSessions(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
