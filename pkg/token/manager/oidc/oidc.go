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

package oidc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	oidc "github.com/coreos/go-oidc"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	userproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/userprovider/v0alpha"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/token/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

func init() {
	registry.Register("oidc", New)
}

type config struct {
	// the endpoint of the oidc provider
	Insecure     bool     `mapstructure:"insecure"`
	SkipCheck    bool     `mapstructure:"skipcheck"`
	Provider     string   `mapstructure:"provider"`
	Audience     string   `mapstructure:"audience"`
	SigningAlgs  []string `mapstructure:"signing_algorithms"`
	ClientID     string   `mapstructure:"client_id"`
	ClientSecret string   `mapstructure:"client_secret"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}
func (c *config) init() {
	// TODO set defaults for dev env
	if len(c.SigningAlgs) < 1 {
		c.SigningAlgs = []string{"RS256", "PS256"}
	}
}

// New returns a token manager that dismantles oidc access tokens
// 1. using an introspection endpoint
// 2. falls back to parsing the token as a jwt
func New(m map[string]interface{}) (token.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	mgr := &manager{c: c}
	return mgr, nil
}

type manager struct {
	c *config
	// cached on first request
	provider *oidc.Provider
	metadata *ProviderMetadata
}

func (m *manager) MintToken(ctx context.Context, u *userproviderv0alphapb.User) (string, error) {
	return "", errtypes.NotSupported("oidc only reads tokens")
}

func (m *manager) DismantleToken(ctx context.Context, accessToken string) (*userproviderv0alphapb.User, error) {
	log := appctx.GetLogger(ctx)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: m.c.Insecure,
		},
	}
	customHTTPClient := &http.Client{
		Transport: tr,
		Timeout:   time.Second * 10,
	}
	customCtx := context.WithValue(ctx, oauth2.HTTPClient, customHTTPClient)

	if m.provider == nil {
		// Initialize a provider by specifying the issuer URL.
		// provider needs to be cached as when it is created
		// it will fetch the keys from the issuer using the .well-known
		// endpoint
		provider, err := oidc.NewProvider(customCtx, m.c.Provider)
		if err != nil {
			return nil, err
		}
		m.provider = provider
		metadata := &ProviderMetadata{}
		if err := provider.Claims(metadata); err != nil {
			return nil, fmt.Errorf("could not unmarshal provider metadata: %v", err)
		}
		m.metadata = metadata
	}
	provider := m.provider

	// The claims we want to have
	var claims StandardClaims

	if m.metadata.IntrospectionEndpoint == "" {

		log.Debug().Msg("no introspection endpoint, trying to decode auth token as jwt")
		//maybe our access token is a jwt token
		c := &oidc.Config{
			ClientID:             m.c.Audience,
			SupportedSigningAlgs: m.c.SigningAlgs,
		}
		if m.c.SkipCheck { // not safe but only way for simplesamlphp to work with an almost compliant oidc (for now)
			c.SkipClientIDCheck = true
			c.SkipIssuerCheck = true
		}
		verifier := provider.Verifier(c)
		idToken, err := verifier.Verify(customCtx, accessToken)
		if err != nil {
			return nil, fmt.Errorf("could not verify jwt: %v", err)
		}
		if err := idToken.Claims(&claims); err != nil {
			return nil, fmt.Errorf("failed to parse claims: %v", err)
		}

	} else {

		// we need to lookup the id token with the access token we got
		// see oidc IDToken.VerifyAccessToken

		data := fmt.Sprintf("token=%s&token_type_hint=access_token", accessToken)
		req, err := http.NewRequest("POST", m.metadata.IntrospectionEndpoint, strings.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("could not create introspection request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		// we follow https://tools.ietf.org/html/rfc7662
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(m.c.ClientID, m.c.ClientSecret)

		res, err := customHTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("could not introspect auth token %s: %v", accessToken, err)
		}
		defer res.Body.Close()

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return nil, fmt.Errorf("could not read introspection response body: %v", err)
		}

		log.Debug().Str("body", string(body)).Msg("body")
		switch strings.Split(res.Header.Get("Content-Type"), ";")[0] {
		// application/jwt is in draft https://tools.ietf.org/html/draft-ietf-oauth-jwt-introspection-response-03
		case "application/jwt":
			// verify the jwt
			log.Warn().Msg("TODO untested verification of jwt encoded auth token")

			verifier := provider.Verifier(&oidc.Config{ClientID: m.c.Audience})
			idToken, err := verifier.Verify(customCtx, string(body))
			if err != nil {
				return nil, fmt.Errorf("could not verify jwt: %v", err)
			}

			if err := idToken.Claims(&claims); err != nil {
				return nil, fmt.Errorf("failed to parse claims: %v", err)
			}
		case "application/json":
			var ir IntrospectionResponse
			// parse json
			if err := json.Unmarshal(body, &ir); err != nil {
				return nil, fmt.Errorf("failed to parse claims: %v", err)
			}
			// verify the auth token is still active
			if !ir.Active {
				log.Debug().Interface("ir", ir).Str("body", string(body)).Msg("token no longer active")
				return nil, fmt.Errorf("token no longer active")
			}
			// resolve user info here? cache it?
			oauth2Token := &oauth2.Token{
				AccessToken: accessToken,
			}
			userInfo, err := provider.UserInfo(customCtx, oauth2.StaticTokenSource(oauth2Token))
			if err != nil {
				return nil, fmt.Errorf("Failed to get userinfo: %v", err)
			}
			if err := userInfo.Claims(&claims); err != nil {
				return nil, fmt.Errorf("failed to unmarshal userinfo claims: %v", err)
			}
			claims.Iss = ir.Iss
			log.Debug().Interface("claims", claims).Interface("userInfo", userInfo).Msg("unmarshalled userinfo")

		default:
			return nil, fmt.Errorf("unknown content type: %s", res.Header.Get("Content-Type"))
		}
	}

	u := &userproviderv0alphapb.User{
		// TODO(jfd) clean up idp = iss, sub = opaque ... is redundant
		Id: &typespb.UserId{
			OpaqueId: claims.Sub, // a stable non reassignable id
			Idp:      claims.Iss, // in the scope of this issuer
		},
		// Subject:     claims.Sub, // TODO(labkode) remove from CS3, is in Id
		// Issuer:      claims.Iss, // TODO(labkode) remove from CS3, is in Id
		Username: claims.PreferredUsername,
		// TODO groups
		// TODO ... use all claims from oidc?
		Groups:      []string{},
		Mail:        claims.Email,
		DisplayName: claims.Name,
	}

	// try kopano konnect specific claims
	if u.Username == "" {
		u.Username = claims.KCIdentity["kc.i.un"]
	}
	if u.DisplayName == "" {
		u.DisplayName = claims.KCIdentity["kc.i.dn"]
	}

	return u, nil
}
