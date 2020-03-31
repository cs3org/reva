// Copyright 2018-2020 CERN
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
	"github.com/rs/zerolog/log"
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
	Insecure bool   `mapstructure:"insecure"`
	Issuer   string `mapstructure:"issuer"`
	IDClaim  string `mapstructure:"id_claim"`
}

func (c *config) init() {
	if c.IDClaim == "" {
		// sub is stable and defined as unique. the user manager needs to take care of the sub to user metadata lookup
		c.IDClaim = "sub"
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
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
// which contains the access token that we can use to contact the UserInfo endpoint
// and get the user claims.
func (am *mgr) Authenticate(ctx context.Context, clientID, clientSecret string) (*user.User, error) {
	ctx = am.getOAuthCtx(ctx)

	provider, err := am.getOIDCProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating oidc provider: +%v", err)
	}

	oauth2Token := &oauth2.Token{
		AccessToken: clientSecret,
	}
	userInfo, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(oauth2Token))
	if err != nil {
		return nil, fmt.Errorf("oidc: error getting userinfo: +%v", err)
	}

	// claims contains the standard OIDC claims like issuer, iat, aud, ... and any other non-standard one.
	// TODO(labkode): make claims configuration dynamic from the config file so we can add arbitrary mappings from claims to user struct.
	var claims map[string]interface{}
	if err := userInfo.Claims(&claims); err != nil {
		return nil, fmt.Errorf("oidc: error unmarshaling userinfo claims: %v", err)
	}
	log.Debug().Interface("claims", claims).Interface("userInfo", userInfo).Msg("unmarshalled userinfo")

	if claims["issuer"] == nil { //This is not set in simplesamlphp
		claims["issuer"] = am.c.Issuer
	}
	if claims["email_verified"] == nil { //This is not set in simplesamlphp
		claims["email_verified"] = false
	}

	if claims["email"] == nil {
		return nil, fmt.Errorf("no \"email\" attribute found in userinfo: maybe the client did not request the oidc \"email\"-scope")
	}

	if claims["preferred_username"] == nil || claims["name"] == nil {
		return nil, fmt.Errorf("no \"preferred_username\" or \"name\" attribute found in userinfo: maybe the client did not request the oidc \"profile\"-scope")
	}

	u := &user.User{
		Id: &user.UserId{
			OpaqueId: claims[am.c.IDClaim].(string), // a stable non reassignable id
			Idp:      claims["issuer"].(string),     // in the scope of this issuer
		},
		Username: claims["preferred_username"].(string),
		// TODO(labkode) if we can get groups from the claim we need to give the possibility
		// to the admin to chosse what claim provides the groups.
		// TODO(labkode) ... use all claims from oidc?
		// TODO(labkode): do like K8s does it: https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/apiserver/plugin/pkg/authenticator/token/oidc/oidc.go
		Groups:       []string{},
		Mail:         claims["email"].(string),
		MailVerified: claims["email_verified"].(bool),
		DisplayName:  claims["name"].(string),
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
	provider, err := oidc.NewProvider(ctx, am.c.Issuer)
	if err != nil {
		return nil, fmt.Errorf("error creating a new oidc provider: %+v", err)
	}

	am.provider = provider
	return am.provider, nil
}
