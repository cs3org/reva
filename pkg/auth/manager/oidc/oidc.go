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
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

func init() {
	registry.Register("oidc", New)
}

type mgr struct {
	// cached on first request
	provider *oidc.Provider
	c        *config
	metadata *ProviderMetadata
}

// TODO(labkode): add support for multiple clients, like we do in the oidc provider http svc.
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

// ClaimsKey is the key for oidc claims in a context
var ClaimsKey struct{}

// New returns an auth manager implementation that validatet the oidc token to authenticate the user.
func New(m map[string]interface{}) (auth.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	return &mgr{c: c}, nil
}

func (am *mgr) Authenticate(ctx context.Context, clientID, token string) (*user.User, error) {
	log := appctx.GetLogger(ctx)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: am.c.Insecure,
		},
	}
	customHTTPClient := &http.Client{
		Transport: tr,
		Timeout:   time.Second * 10,
	}
	customCtx := context.WithValue(ctx, oauth2.HTTPClient, customHTTPClient)

	if am.provider == nil {
		// Initialize a provider by specifying the issuer URL.
		// provider needs to be cached as when it is created
		// it will fetch the keys from the issuer using the .well-known
		// endpoint
		provider, err := oidc.NewProvider(customCtx, am.c.Provider)
		if err != nil {
			return nil, err
		}
		am.provider = provider
		metadata := &ProviderMetadata{}
		if err := provider.Claims(metadata); err != nil {
			return nil, fmt.Errorf("could not unmarshal provider metadata: %v", err)
		}
		am.metadata = metadata
	}
	provider := am.provider

	// The claims we want to have
	var claims StandardClaims

	if am.metadata.IntrospectionEndpoint == "" {

		log.Debug().Msg("no introspection endpoint, trying to decode access token as jwt")
		//maybe our access token is a jwt token
		c := &oidc.Config{
			ClientID:             am.c.Audience,
			SupportedSigningAlgs: am.c.SigningAlgs,
		}
		if am.c.SkipCheck { // not safe but only way for simplesamlphp to work with an almost compliant oidc (for now)
			c.SkipClientIDCheck = true
			c.SkipIssuerCheck = true
		}
		verifier := provider.Verifier(c)
		idToken, err := verifier.Verify(customCtx, token)

		if err != nil {
			return nil, fmt.Errorf("could not verify jwt: %v", err)
		}
		if err := idToken.Claims(&claims); err != nil {
			return nil, fmt.Errorf("failed to parse claims: %v", err)
		}

		if claims.PreferredUsername == "" {
			// resolve user info here? cache it?
			oauth2Token := &oauth2.Token{
				AccessToken: token,
			}
			userInfo, err := provider.UserInfo(customCtx, oauth2.StaticTokenSource(oauth2Token))
			if err != nil {
				return nil, fmt.Errorf("Failed to get userinfo: %v", err)
			}
			if err := userInfo.Claims(&claims); err != nil {
				return nil, fmt.Errorf("failed to unmarshal userinfo claims: %v", err)
			}
			log.Debug().Interface("claims", claims).Interface("userInfo", userInfo).Msg("unmarshalled userinfo")
		}
	} else {

		// we need to lookup the id token with the access token we got
		// see oidc IDToken.Verifytoken

		data := fmt.Sprintf("token=%s&token_type_hint=access_token", token)
		req, err := http.NewRequest("POST", am.metadata.IntrospectionEndpoint, strings.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("could not create introspection request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		// we follow https://tools.ietf.org/html/rfc7662
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(am.c.ClientID, am.c.ClientSecret)

		res, err := customHTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("could not introspect access token %s: %v", token, err)
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
			log.Warn().Msg("TODO untested verification of jwt encoded introspection response")

			verifier := provider.Verifier(&oidc.Config{ClientID: am.c.Audience})
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
				AccessToken: token,
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

	u := &user.User{
		Id: &user.UserId{
			OpaqueId: claims.Sub, // a stable non reassignable id
			Idp:      claims.Iss, // in the scope of this issuer
		},
		Username: claims.PreferredUsername,
		// TODO groups are not covered by oidc but might be sent as a custom claim.
		// TODO ... use all claims from oidc?
		Groups:       []string{},
		Mail:         claims.Email,
		MailVerified: claims.EmailVerified,
		DisplayName:  claims.Name,
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
