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

package auth

import (
	"fmt"
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/internal/http/interceptors/auth/credential/registry"
	tokenregistry "github.com/cs3org/reva/internal/http/interceptors/auth/token/registry"
	tokenwriterregistry "github.com/cs3org/reva/internal/http/interceptors/auth/tokenwriter/registry"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/token"
	tokenmgr "github.com/cs3org/reva/pkg/token/manager/registry"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

type config struct {
	Priority   int    `mapstructure:"priority"`
	GatewaySvc string `mapstructure:"gatewaysvc"`
	// TODO(jdf): Realm is optional, will be filled with request host if not given?
	Realm                  string                            `mapstructure:"realm"`
	CredentialsByUserAgent map[string]string                 `mapstructure:"credentials_by_user_agent"`
	CredentialChain        []string                          `mapstructure:"credential_chain"`
	CredentialStrategies   map[string]map[string]interface{} `mapstructure:"credential_strategies"`
	TokenStrategy          string                            `mapstructure:"token_strategy"`
	TokenStrategies        map[string]map[string]interface{} `mapstructure:"token_strategies"`
	TokenManager           string                            `mapstructure:"token_manager"`
	TokenManagers          map[string]map[string]interface{} `mapstructure:"token_managers"`
	TokenWriter            string                            `mapstructure:"token_writer"`
	TokenWriters           map[string]map[string]interface{} `mapstructure:"token_writers"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a new middleware with defined priority.
func New(m map[string]interface{}, unprotected []string) (global.Middleware, error) {
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	conf.GatewaySvc = sharedconf.GetGatewaySVC(conf.GatewaySvc)

	// set defaults
	if conf.TokenStrategy == "" {
		conf.TokenStrategy = "header"
	}

	if conf.TokenWriter == "" {
		conf.TokenWriter = "header"
	}

	if conf.TokenManager == "" {
		conf.TokenManager = "jwt"
	}

	if len(conf.CredentialChain) == 0 {
		conf.CredentialChain = []string{"basic", "bearer"}
	}

	if conf.CredentialsByUserAgent == nil {
		conf.CredentialsByUserAgent = map[string]string{}
	}

	credChain := map[string]auth.CredentialStrategy{}
	for i, key := range conf.CredentialChain {
		f, ok := registry.NewCredentialFuncs[conf.CredentialChain[i]]
		if !ok {
			return nil, fmt.Errorf("credential strategy not found: %s", conf.CredentialChain[i])
		}

		credStrategy, err := f(conf.CredentialStrategies[conf.CredentialChain[i]])
		if err != nil {
			return nil, err
		}
		credChain[key] = credStrategy
	}

	g, ok := tokenregistry.NewTokenFuncs[conf.TokenStrategy]
	if !ok {
		return nil, fmt.Errorf("token strategy not found: %s", conf.TokenStrategy)
	}

	tokenStrategy, err := g(conf.TokenStrategies[conf.TokenStrategy])
	if err != nil {
		return nil, err
	}

	h, ok := tokenmgr.NewFuncs[conf.TokenManager]
	if !ok {
		return nil, fmt.Errorf("token manager not found: %s", conf.TokenStrategy)
	}

	tokenManager, err := h(conf.TokenManagers[conf.TokenManager])
	if err != nil {
		return nil, err
	}

	i, ok := tokenwriterregistry.NewTokenFuncs[conf.TokenWriter]
	if !ok {
		return nil, fmt.Errorf("token writer not found: %s", conf.TokenWriter)
	}

	tokenWriter, err := i(conf.TokenWriters[conf.TokenWriter])
	if err != nil {
		return nil, err
	}

	chain := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			// OPTION requests need to pass for preflight requests
			// TODO(labkode): this will break options for auth protected routes.
			// Maybe running the CORS middleware before auth kicks in is enough.
			if r.Method == "OPTIONS" {
				h.ServeHTTP(w, r)
				return
			}

			log := appctx.GetLogger(ctx)

			// skip auth for urls set in the config.
			// TODO(labkode): maybe use method:url to bypass auth.
			if utils.Skip(r.URL.Path, unprotected) {
				log.Info().Msg("skipping auth check for: " + r.URL.Path)
				h.ServeHTTP(w, r)
				return
			}

			tkn := tokenStrategy.GetToken(r)
			if tkn == "" {
				log.Warn().Msg("core access token not set")

				userAgentCredKeys := getCredsForUserAgent(r.UserAgent(), conf.CredentialsByUserAgent, conf.CredentialChain)

				// obtain credentials (basic auth, bearer token, ...) based on user agent
				var creds *auth.Credentials
				for _, k := range userAgentCredKeys {
					creds, err = credChain[k].GetCredentials(w, r)
					if err != nil {
						log.Debug().Err(err).Msg("error retrieving credentials")
					}

					if creds != nil {
						log.Debug().Msgf("credentials obtained from credential strategy: type: %s, client_id: %s", creds.Type, creds.ClientID)
						break
					}
				}

				// if no credentials are found, reply with authentication challenge depending on user agent
				if creds == nil {
					for _, key := range userAgentCredKeys {
						if cred, ok := credChain[key]; ok {
							cred.AddWWWAuthenticate(w, r, conf.Realm)
						} else {
							panic("auth credential strategy: " + key + "must have been loaded in init method")
						}
					}
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				req := &gateway.AuthenticateRequest{
					Type:         creds.Type,
					ClientId:     creds.ClientID,
					ClientSecret: creds.ClientSecret,
				}

				log.Debug().Msgf("AuthenticateRequest: type: %s, client_id: %s against %s", req.Type, req.ClientId, conf.GatewaySvc)

				client, err := pool.GetGatewayServiceClient(conf.GatewaySvc)
				if err != nil {
					log.Error().Err(err).Msg("error getting the authsvc client")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				res, err := client.Authenticate(ctx, req)
				if err != nil {
					log.Error().Err(err).Msg("error calling Authenticate")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				if res.Status.Code != rpc.Code_CODE_OK {
					err := status.NewErrorFromCode(res.Status.Code, "auth")
					log.Err(err).Msg("error generating access token from credentials")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				log.Info().Msg("core access token generated")
				// write token to response
				tkn = res.Token
				tokenWriter.WriteToken(tkn, w)
			} else {
				log.Debug().Msg("access token is already provided")
			}

			// validate token
			claims, err := tokenManager.DismantleToken(r.Context(), tkn)
			if err != nil {
				log.Error().Err(err).Msg("error dismantling token")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			u := &userpb.User{}
			if err := mapstructure.Decode(claims, u); err != nil {
				log.Error().Err(err).Msg("error decoding user claims")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// store user and core access token in context.
			ctx = user.ContextSetUser(ctx, u)
			ctx = token.ContextSetToken(ctx, tkn)
			ctx = metadata.AppendToOutgoingContext(ctx, token.TokenHeader, tkn) // TODO(jfd): hardcoded metadata key. use  PerRPCCredentials?

			r = r.WithContext(ctx)
			h.ServeHTTP(w, r)
		})
	}
	return chain, nil
}

// getCredsForUserAgent returns the WWW Authenticate challenges keys to use given an http request
// and available credentials.
func getCredsForUserAgent(ua string, uam map[string]string, creds []string) []string {
	if ua == "" || len(uam) == 0 {
		return creds
	}

	cred, ok := uam[ua]
	if ok {
		for _, v := range creds {
			if v == cred {
				return []string{cred}
			}
		}
		return creds
	}

	return creds
}
