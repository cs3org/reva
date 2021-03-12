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
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/mitchellh/mapstructure"
	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/storage"
	"github.com/ory/fosite/token/jwt"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("oidcprovider", New)
}

type config struct {
	Prefix          string                            `mapstructure:"prefix"`
	GatewayEndpoint string                            `mapstructure:"gatewaysvc"`
	Clients         map[string]map[string]interface{} `mapstructure:"clients"`
	Issuer          string                            `mapstructure:"issuer"`
}

func (c *config) init() {

	if c.Prefix == "" {
		c.Prefix = "oauth2"
	}

	c.GatewayEndpoint = sharedconf.GetGatewaySVC(c.GatewayEndpoint)
}

type client struct {
	ID            string   `mapstructure:"id"`
	Secret        string   `mapstructure:"client_secret,"`
	RedirectURIs  []string `mapstructure:"redirect_uris"`
	GrantTypes    []string `mapstructure:"grant_types"`
	ResponseTypes []string `mapstructure:"response_types"`
	Scopes        []string `mapstructure:"scopes"`
	Audience      []string `mapstructure:"audience"`
	Public        bool     `mapstructure:"public"`
}

type svc struct {
	prefix  string
	conf    *config
	handler http.Handler
	store   *storage.MemoryStore
	oauth2  fosite.OAuth2Provider
	clients map[string]fosite.Client
}

// New returns a new oidcprovidersvc
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}

	c.init()

	clients, err := getClients(c.Clients)
	if err != nil {
		return nil, errors.Wrap(err, "oidcprovider: error parsing oidc clients")
	}

	store := getStore(clients)
	conf := getConfig(c.Issuer)
	provider := getOAuth2Provider(store, conf)

	s := &svc{
		conf:    c,
		prefix:  c.Prefix,
		clients: clients,
		store:   store,
		oauth2:  provider,
	}

	s.setHandler()
	return s, nil
}

func getOAuth2Provider(st fosite.Storage, conf *compose.Config) fosite.OAuth2Provider {
	return compose.ComposeAllEnabled(
		conf,
		st,
		[]byte("my super secret signing password"), // Caution: it MUST always be 32 bytes long.
		mustRSAKey(),
	)
}

func getClients(confClients map[string]map[string]interface{}) (map[string]fosite.Client, error) {
	clients := map[string]fosite.Client{}
	for id, val := range confClients {
		client := &client{}
		if err := mapstructure.Decode(val, client); err != nil {
			err = errors.Wrap(err, "oidcprovider: error decoding client configuration")
			return nil, err
		}

		fosClient := &fosite.DefaultClient{
			ID:            client.ID,
			Secret:        []byte(client.Secret),
			RedirectURIs:  client.RedirectURIs,
			GrantTypes:    client.GrantTypes,
			ResponseTypes: client.ResponseTypes,
			Scopes:        client.Scopes,
			Audience:      client.Audience,
			Public:        client.Public,
		}

		clients[id] = fosClient
	}
	return clients, nil
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

func (s *svc) Unprotected() []string {
	// TODO(jfd): the introspect endpoint should be protected? That is why we have client id and client secret?
	return []string{
		"/",
	}
}

func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())

		if r.Method == "OPTIONS" {
			// TODO use CORS allow access from everywhere
			w.Header().Set("Access-Control-Allow-Origin", "*")
			return
		}

		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
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

func getStore(clients map[string]fosite.Client) *storage.MemoryStore {
	return &storage.MemoryStore{
		IDSessions:             make(map[string]fosite.Requester),
		Clients:                clients,
		AuthorizeCodes:         map[string]storage.StoreAuthorizeCode{},
		AccessTokens:           map[string]fosite.Requester{},
		RefreshTokens:          map[string]storage.StoreRefreshToken{},
		PKCES:                  map[string]fosite.Requester{},
		AccessTokenRequestIDs:  map[string]string{},
		RefreshTokenRequestIDs: map[string]string{},
	}
}

func getConfig(issuer string) *compose.Config {
	return &compose.Config{
		IDTokenIssuer: issuer,
	}
}

// customSession keeps track of the session between the auth and token and userinfo endpoints.
// We need our custom session to store the internal token.
type customSession struct {
	*openid.DefaultSession
	internalToken string
}

// A session is passed from the `/auth` to the `/token` endpoint. You probably want to store data like: "Who made the request",
// "What organization does that person belong to" and so on.
// For our use case, the session will meet the requirements imposed by JWT access tokens, HMAC access tokens and OpenID Connect
// ID Tokens plus a custom field

// newSession is a helper function for creating a new session. This may look like a lot of code but since we are
// setting up multiple strategies it is a bit longer.
// Usually, you could do:
//
//  session = new(fosite.DefaultSession)
func (s *svc) getSession(token string, user *userpb.User) *customSession {
	return &customSession{
		DefaultSession: &openid.DefaultSession{
			Claims: &jwt.IDTokenClaims{
				// TODO(labkode): we override the issuer here as we are the OIDC provider.
				// Does it make sense? The auth backend can be on another domain, but this service
				// is the one responsible for oidc logic.
				// The issuer needs to map the subject in the configuration.
				Issuer:  s.conf.Issuer,
				Subject: user.Id.OpaqueId,
				// TODO(labkode): check what audience means and set it correctly.
				//Audience:    []string{"https://my-client.my-application.com"},
				// TODO(labkode): make times configurable to align to internal token lifetime.
				ExpiresAt: time.Now().Add(time.Hour * 6),
				IssuedAt:  time.Now(),
				//RequestedAt: time.Now(),
				//AuthTime:    time.Now(),
			},
			Headers: &jwt.Headers{
				Extra: make(map[string]interface{}),
			},
			Username: user.Username,
			Subject:  user.Id.OpaqueId,
		},
		internalToken: token,
	}
}

// emptySession creates a session object and fills it with safe defaults
func (s *svc) getEmptySession() *customSession {
	return &customSession{
		DefaultSession: &openid.DefaultSession{
			Claims: &jwt.IDTokenClaims{
				// TODO(labkode): we override the issuer here as we are the OIDC provider.
				// Does it make sense? The auth backend can be on another domain, but this service
				// is the one responsible for oidc logic.
				// The issuer needs to map the in the configuration.
				Issuer: s.conf.Issuer,
				// TODO(labkode): check what audience means and set it correctly.
				//Audience:    []string{"https://my-client.my-application.com"},
				// TODO(labkode): make times configurable to align to internal token lifetime.
				ExpiresAt: time.Now().Add(time.Hour * 6),
				IssuedAt:  time.Now(),
				//RequestedAt: time.Now(),
				//AuthTime:    time.Now(),
			},
			Headers: &jwt.Headers{
				Extra: make(map[string]interface{}),
			},
		},
	}
}

func mustRSAKey() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		// TODO(jfd): don't panic!
		panic(err)
	}
	return key
}
