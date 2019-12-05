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

// Package oidc  verifies an OIDC token against the configured OIDC provider
// and obtains the necessary claims to obtain user information.
package oidc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	oidc "github.com/coreos/go-oidc"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
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
	provider *oidc.Provider // cached on first request
	c        *config
}

type config struct {
	Insecure     bool     `mapstructure:"insecure"`
	SkipCheck    bool     `mapstructure:"skipcheck"`
	Provider     string   `mapstructure:"provider"` // the endpoint of the oidc provider, a.k.a the issuer OIDC term.
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
	// set default signing algorithms to verifiy against.
	if len(c.SigningAlgs) < 1 {
		c.SigningAlgs = []string{"RS256", "PS256"}
	}
}

// New returns an auth manager implementation that verifies the oidc token and obtains the user claims.
func New(m map[string]interface{}) (auth.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	return &mgr{c: c}, nil
}

// the clientID it would be empty as we only need to validate the clientSecret variable
// that contains the OIDC token to be verified against the OIDC provider.
func (am *mgr) Authenticate(ctx context.Context, clientID, clientSecret string) (*user.User, error) {
	//log := appctx.GetLogger(ctx)

	ctx = am.getOAuthCtx(ctx)

	provider, err := am.getOIDCProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating oidc provider: +%v", err)
	}

	// verifier is used to verify the oidc token againsy the oidc provider.
	verifier := am.getVerifier(provider)

	idToken, err := verifier.Verify(ctx, clientSecret)
	if err != nil {
		return nil, errors.Wrap(err, "oidc: error verifying oidc token")
	}

	// claims contains the standard OIDC claims like issuer, iat, aud, ...
	var claims StandardClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, errors.Wrap(err, "oidc: error parsing claims")
	}

	/*
		if am.metadata.IntrospectionEndpoint == "" {

			log.Debug().Msg("no introspection endpoint, trying to decode access token as jwt")
			// maybe our access token is a jwt token
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

		} else {

			// we need to lookup the id token with the access token we got
			// see oidc IDToken.Verifytoken

			data := fmt.Sprintf("token=%s&token_type_hint=access_token", token)
			req, err := http.NewRequest("POST", am.metadata.UserinfoEndpoint, strings.NewReader(data))
			if err != nil {
				return nil, fmt.Errorf("could not create introspection request: %v", err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			// we follow https://tools.ietf.org/html/rfc7662
			req.Header.Set("Accept", "application/json")
			//req.SetBasicAuth(am.c.ClientID, am.c.ClientSecret)
			//req.SetBasicAuth(am.c.ClientID, token)
			req.SetBasicAuth(am.c.ClientID, "")

			res, err := customHTTPClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("could not introspect access token %s: %v", token, err)
			}
			defer res.Body.Close()

			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				return nil, fmt.Errorf("could not read introspection response body: %v", err)
			}

			log.Debug().Str("body", string(body)).Str("client", am.c.ClientID).Str("req", fmt.Sprintf("%+v", req)).Msg("oidc introspect response")
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

	*/

	u := &user.User{
		Id: &user.UserId{
			OpaqueId: claims.Sub, // a stable non reassignable id
			Idp:      claims.Iss, // in the scope of this issuer
		},
		Username: claims.PreferredUsername,
		// TODO(labkode) if we can get groups from the claim we need to give the possibility
		// to the admin to chosse what claim provides the groups.
		// TODO(labkode) ... use all claims from oidc?
		Groups:       []string{},
		Mail:         claims.Email,
		MailVerified: claims.EmailVerified,
		DisplayName:  claims.Name,
	}

	return u, nil
}

func (am *mgr) getOAuthCtx(ctx context.Context) context.Context {
	// Sometimes for testing we need to skip the TLS check, that's why we need a
	// custom HTTP client.
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: am.c.Insecure,
		},
	}
	customHTTPClient := &http.Client{
		Transport: tr,
		Timeout:   time.Second * 10,
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, customHTTPClient)
	return ctx
}

func (am *mgr) getOIDCProvider(ctx context.Context) (*oidc.Provider, error) {
	if am.provider != nil {
		return am.provider, nil
	}

	// Initialize a provider by specifying the issuer URL.
	// Once initialized is a singleton that is reuser if further requests.
	// The provider is responsible to verify the token sent by the client
	// against the security keys oftentimes available in the .well-known endpoint.
	provider, err := oidc.NewProvider(ctx, am.c.Provider)
	if err != nil {
		return nil, fmt.Errorf("error creating a new oidc provider: %+v", err)
	}

	// Once the provider is configured we obtain the metadata claims like the
	// authorization, token and instrospection endpoints if they are available.

	/*metadata := &ProviderMetadata{}
	if err := provider.Claims(metadata); err != nil {
		return nil, fmt.Errorf("error unmarshaling oidc provider metadata: %v", err)
	}
	*/

	am.provider = provider
	//am.metadata = metadata
	return am.provider, nil
}

func (am *mgr) getVerifier(provider *oidc.Provider) *oidc.IDTokenVerifier {
	c := &oidc.Config{
		ClientID:             am.c.ClientID,
		SupportedSigningAlgs: am.c.SigningAlgs,
	}

	// it is unsage to skip client id and issuer check but it comes useful when
	// testing against dev oidc providers that are not properly setup.
	if am.c.SkipCheck {
		c.SkipClientIDCheck = true
		c.SkipIssuerCheck = true
		c.SkipExpiryCheck = true
	}

	verifier := provider.Verifier(c)
	return verifier
}
