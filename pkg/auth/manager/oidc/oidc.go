// Copyright 2018-2024 CERN
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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/auth/scope"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/httpclient"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/golang-jwt/jwt"
	"github.com/juliangruber/go-intersect"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

func init() {
	registry.Register("oidc", New)
}

type mgr struct {
	providers map[string]*oidc.Provider

	c                *config
	oidcUsersMapping map[string]*oidcUserMapping
}

type config struct {
	Insecure     bool   `docs:"false;Whether to skip certificate checks when sending requests."          mapstructure:"insecure"`
	Issuer       string `docs:";The issuer of the OIDC token."                                           mapstructure:"issuer"`
	IDClaim      string `docs:"sub;The claim containing the ID of the user."                             mapstructure:"id_claim"`
	UIDClaim     string `docs:";The claim containing the UID of the user."                               mapstructure:"uid_claim"`
	GIDClaim     string `docs:";The claim containing the GID of the user."                               mapstructure:"gid_claim"`
	GatewaySvc   string `docs:";The endpoint at which the GRPC gateway is exposed."                      mapstructure:"gatewaysvc"`
	UsersMapping string `docs:"; The optional OIDC users mapping file path"                              mapstructure:"users_mapping"`
	GroupClaim   string `docs:"; The group claim to be looked up to map the user (default to 'groups')." mapstructure:"group_claim"`
}

type oidcUserMapping struct {
	OIDCIssuer string `json:"oidc_issuer" mapstructure:"oidc_issuer"`
	OIDCGroup  string `json:"oidc_group"  mapstructure:"oidc_group"`
	Username   string `json:"username"    mapstructure:"username"`
}

func (c *config) ApplyDefaults() {
	if c.IDClaim == "" {
		// sub is stable and defined as unique. the user manager needs to take care of the sub to user metadata lookup
		c.IDClaim = "sub"
	}
	if c.GroupClaim == "" {
		c.GroupClaim = "groups"
	}
	if c.UIDClaim == "" {
		c.UIDClaim = "uid"
	}
	if c.GIDClaim == "" {
		c.GIDClaim = "gid"
	}

	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

// New returns an auth manager implementation that verifies the oidc token and obtains the user claims.
func New(ctx context.Context, m map[string]interface{}) (auth.Manager, error) {
	manager := &mgr{
		providers: make(map[string]*oidc.Provider),
	}
	err := manager.Configure(m)
	if err != nil {
		return nil, err
	}
	return manager, nil
}

func (am *mgr) Configure(m map[string]interface{}) error {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return errors.Wrap(err, "oidc: error decoding config")
	}
	am.c = &c

	am.oidcUsersMapping = map[string]*oidcUserMapping{}
	if c.UsersMapping == "" {
		// no mapping defined, leave the map empty and move on
		return nil
	}

	f, err := os.ReadFile(c.UsersMapping)
	if err != nil {
		return fmt.Errorf("oidc: error reading the users mapping file: +%v", err)
	}
	oidcUsers := []*oidcUserMapping{}
	err = json.Unmarshal(f, &oidcUsers)
	if err != nil {
		return fmt.Errorf("oidc: error unmarshalling the users mapping file: +%v", err)
	}
	for _, u := range oidcUsers {
		if _, found := am.oidcUsersMapping[u.OIDCGroup]; found {
			return fmt.Errorf("oidc: mapping error, group \"%s\" is mapped to multiple users", u.OIDCGroup)
		}
		am.oidcUsersMapping[u.OIDCGroup] = u
	}

	return nil
}

func extractClaims(token string) (jwt.MapClaims, error) {
	var claims jwt.MapClaims
	_, _, err := new(jwt.Parser).ParseUnverified(token, &claims)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func extractIssuer(m jwt.MapClaims) (string, bool) {
	issIface, ok := m["iss"]
	if !ok {
		return "", false
	}
	iss, _ := issIface.(string)
	return iss, iss != ""
}

func (am *mgr) getOIDCProviderForIssuer(ctx context.Context, issuer string) (*oidc.Provider, error) {
	// FIXME: op not atomic TODO: fix message and make it more clear
	if am.providers[issuer] == nil {
		// TODO (gdelmont): the provider should be periodically recreated
		// as the public key can change over time
		provider, err := oidc.NewProvider(ctx, issuer)
		if err != nil {
			return nil, errors.Wrapf(err, "oidc: error creating a new oidc provider")
		}
		am.providers[issuer] = provider
	}
	return am.providers[issuer], nil
}

func (am *mgr) isIssuerAllowed(issuer string) bool {
	if am.c.Issuer == issuer {
		return true
	}
	for _, m := range am.oidcUsersMapping {
		if m.OIDCIssuer == issuer {
			return true
		}
	}
	return false
}

func (am *mgr) doUserMapping(tkn *oidc.IDToken, claims jwt.MapClaims) (string, error) {
	var sub = tkn.Subject
	if am.c.IDClaim != "sub" && claims[am.c.IDClaim] != nil {
		sub, _ = claims[am.c.IDClaim].(string)
	}
	if len(am.oidcUsersMapping) == 0 {
		return sub, nil
	}
	// we need the custom claims for the mapping
	if claims[am.c.GroupClaim] == nil {
		// we are required to perform a user mapping but the group claim is not available
		return sub, nil
	}

	mappings := make([]string, 0, len(am.oidcUsersMapping))
	for _, m := range am.oidcUsersMapping {
		if m.OIDCIssuer == tkn.Issuer {
			mappings = append(mappings, m.OIDCGroup)
		}
	}

	intersection := intersect.Simple(claims[am.c.GroupClaim], mappings)
	if len(intersection) > 1 {
		// multiple mappings are not implemented as we cannot decide which one to choose
		return "", errtypes.PermissionDenied("more than one user mapping entry exists for the given group claims")
	}
	if len(intersection) == 0 {
		return "", errtypes.PermissionDenied("no user mapping found for the given group claim(s)")
	}
	m := intersection[0].(string)
	return am.oidcUsersMapping[m].Username, nil
}

// The clientID would be empty as we only need to validate the clientSecret variable
// which contains the access token that we can use to contact the UserInfo endpoint
// and get the user claims.
func (am *mgr) Authenticate(ctx context.Context, _, clientSecret string) (*user.User, map[string]*authpb.Scope, error) {
	log := appctx.GetLogger(ctx)
	ctx = am.getOAuthCtx(ctx)

	claims, err := extractClaims(clientSecret)
	if err != nil {
		return nil, nil, errtypes.PermissionDenied(fmt.Sprintf("error extracting claims from oidc token: %+v", err))
	}

	issuer, ok := extractIssuer(claims)
	if !ok {
		return nil, nil, errtypes.PermissionDenied("issuer not contained in the token")
	}
	log.Debug().Str("issuer", issuer).Msg("extracted issuer from token")

	if !am.isIssuerAllowed(issuer) {
		log.Debug().Str("issuer", issuer).Msg("issuer is not in the whitelist")
		return nil, nil, errtypes.PermissionDenied("issuer not recognised")
	}
	log.Debug().Str("issuer", issuer).Msg("issuer is whitelisted")

	provider, err := am.getOIDCProviderForIssuer(ctx, issuer)
	if err != nil {
		return nil, nil, errors.Wrap(err, "oidc: error creating oidc provider")
	}

	config := &oidc.Config{
		SkipClientIDCheck: true,
	}

	tkn, err := provider.Verifier(config).Verify(ctx, clientSecret)
	if err != nil {
		return nil, nil, errtypes.PermissionDenied(fmt.Sprintf("oidc token failed verification: %+v", err))
	}

	sub, err := am.doUserMapping(tkn, claims)
	if err != nil {
		return nil, nil, err
	}
	log.Debug().Str("sub", sub).Msg("mapped user from token")

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(am.c.GatewaySvc))
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting user provider grpc client")
	}
	userRes, err := client.GetUserByClaim(ctx, &user.GetUserByClaimRequest{
		Claim: "username",
		Value: sub,
	})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error getting user by username '%v'", sub)
	}
	if userRes.Status.Code != rpc.Code_CODE_OK {
		return nil, nil, status.NewErrorFromCode(userRes.Status.Code, "oidc")
	}

	u := userRes.GetUser()

	var scopes map[string]*authpb.Scope
	if u.Id.Type == user.UserType_USER_TYPE_LIGHTWEIGHT {
		scopes, err = scope.AddLightweightAccountScope(authpb.Role_ROLE_OWNER, nil)
		if err != nil {
			return nil, nil, err
		}
		// TODO (gdelmont): we may want to define a template to prettify the user info for lw account?
		// strip the `guest:` prefix if present in the email claim (appears to come from LDAP at CERN?)
		u.Mail = strings.Replace(u.Mail, "guest: ", "", 1)
		// and decorate the display name with the email domain to make it different from a primary account
		u.DisplayName = u.DisplayName + " (" + strings.Split(u.Mail, "@")[1] + ")"
	} else {
		scopes, err = scope.AddOwnerScope(nil)
		if err != nil {
			return nil, nil, err
		}
	}

	return u, scopes, nil
}

func (am *mgr) getOAuthCtx(ctx context.Context) context.Context {
	tr := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: am.c.Insecure},
	}

	// Sometimes for testing we need to skip the TLS check, that's why we need a
	// custom HTTP client.
	customHTTPClient := httpclient.New(
		httpclient.Timeout(time.Second*10),
		httpclient.RoundTripper(tr),
	)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, customHTTPClient)
	return ctx
}
