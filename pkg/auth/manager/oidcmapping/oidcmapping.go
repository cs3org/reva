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

package oidcmapping

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	oidc "github.com/coreos/go-oidc"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/auth/scope"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/juliangruber/go-intersect"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

func init() {
	registry.Register("oidcmapping", New)
}

type mgr struct {
	provider         *oidc.Provider // cached on first request
	c                *config
	oidcUsersMapping map[string]*oidcUserMapping
}

type config struct {
	Insecure        bool   `mapstructure:"insecure" docs:"false;Whether to skip certificate checks when sending requests."`
	Issuer          string `mapstructure:"issuer" docs:";The issuer of the OIDC token."`
	IDClaim         string `mapstructure:"id_claim" docs:"sub;The claim containing the ID of the user."`
	UIDClaim        string `mapstructure:"uid_claim" docs:";The claim containing the UID of the user."`
	GIDClaim        string `mapstructure:"gid_claim" docs:";The claim containing the GID of the user."`
	UserProviderSvc string `mapstructure:"userprovidersvc" docs:";The endpoint at which the GRPC userprovider is exposed."`
	UsersMapping    string `mapstructure:"usersmapping" docs:"; The OIDC users mapping file path"`
}

type oidcUserMapping struct {
	OIDCIssuer string `mapstructure:"oidc_issuer" json:"oidc_issuer"`
	OIDCGroup  string `mapstructure:"oidc_group" json:"oidc_group"`
	Username   string `mapstructure:"username" json:"username"`
}

func (c *config) init() {
	if c.IDClaim == "" {
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

// New returns an auth manager.
func New(m map[string]interface{}) (auth.Manager, error) {
	manager := &mgr{}
	err := manager.Configure(m)
	if err != nil {
		return nil, err
	}
	return manager, nil
}

func (am *mgr) Configure(m map[string]interface{}) error {
	c, err := parseConfig(m)
	if err != nil {
		return err
	}
	c.init()
	am.c = c

	am.oidcUsersMapping = map[string]*oidcUserMapping{}
	f, err := ioutil.ReadFile(c.UsersMapping)
	if err != nil {
		return fmt.Errorf("oidcmapping: error reading oidc users mapping file: +%v", err)
	}

	oidcUsers := []*oidcUserMapping{}

	err = json.Unmarshal(f, &oidcUsers)
	if err != nil {
		return fmt.Errorf("oidcmapping: error unmarshalling oidc users mapping file: +%v", err)
	}

	for _, u := range oidcUsers {
		am.oidcUsersMapping[u.OIDCGroup] = u
	}

	return nil
}

// Authenticate clientID would be empty as we only need to validate the clientSecret variable
// which contains the access token that we can use to contact the UserInfo endpoint
// and get the user claims.
func (am *mgr) Authenticate(ctx context.Context, clientID, clientSecret string) (*user.User, map[string]*authpb.Scope, error) {
	ctx = am.getOAuthCtx(ctx)
	log := appctx.GetLogger(ctx)

	oidcProvider, err := am.getOIDCProvider(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("oidcmapping: error creating oidc provider: +%v", err)
	}

	oauth2Token := &oauth2.Token{
		AccessToken: clientSecret,
	}

	// query the oidc provider for user info
	userInfo, err := oidcProvider.UserInfo(ctx, oauth2.StaticTokenSource(oauth2Token))
	if err != nil {
		return nil, nil, fmt.Errorf("oidcmapping: error getting userinfo: +%v", err)
	}

	// claims contains the standard OIDC claims like issuer, iat, aud, ... and any other non-standard one.
	// TODO(labkode): make claims configuration dynamic from the config file so we can add arbitrary mappings from claims to user struct.
	var claims map[string]interface{}
	if err := userInfo.Claims(&claims); err != nil {
		return nil, nil, fmt.Errorf("oidcmapping: error unmarshaling userinfo claims: %v", err)
	}

	log.Debug().Interface("claims", claims).Interface("userInfo", userInfo).Msg("unmarshalled userinfo")

	if claims["issuer"] == nil { // This is not set in simplesamlphp
		claims["issuer"] = am.c.Issuer
	}
	if claims["email_verified"] == nil { // This is not set in simplesamlphp
		claims["email_verified"] = false
	}
	if claims["email"] == nil {
		return nil, nil, fmt.Errorf("oidcmapping: no \"email\" attribute found in userinfo: maybe the client did not request the oidc \"email\"-scope")
	}
	if claims["preferred_username"] == nil || claims["name"] == nil {
		return nil, nil, fmt.Errorf("oidcmapping: no \"preferred_username\" or \"name\" attribute found in userinfo: maybe the client did not request the oidc \"profile\"-scope")
	}
	if claims["groups"] == nil {
		return nil, nil, fmt.Errorf("oidcmapping: no \"groups\" attribute found in userinfo")
	}

	// discover the user username
	var username string
	mappings := make([]string, 0, len(am.oidcUsersMapping))
	for _, v := range am.oidcUsersMapping {
		if v.OIDCIssuer == claims["issuer"] {
			mappings = append(mappings, v.OIDCGroup)
		}
	}
	intersection := intersect.Simple(claims["groups"], mappings)
	if len(intersection) > 1 {
		// multiple mappings is not implemented, we don't know which one to choose
		return nil, nil, errors.New("oidcmapping: mapping failed, more than one mapping found")
	}
	if len(intersection) == 1 {
		for _, m := range intersection {
			username = am.oidcUsersMapping[m.(string)].Username
		}
	}
	if username == "" {
		return nil, nil, errors.New("oidcmapping: unable to retrieve username from mappings")
	}

	var uid, gid float64
	if am.c.UIDClaim != "" {
		uid, _ = claims[am.c.UIDClaim].(float64)
	}
	if am.c.GIDClaim != "" {
		gid, _ = claims[am.c.GIDClaim].(float64)
	}

	userID := &user.UserId{
		OpaqueId: username,
		Idp:      "",
		Type:     user.UserType_USER_TYPE_PRIMARY,
	}
	gwc, err := pool.GetUserProviderServiceClient(am.c.UserProviderSvc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "oidcmapping: error getting gateway grpc client")
	}

	getUserByClaimResp, err := gwc.GetUserByClaim(ctx, &user.GetUserByClaimRequest{
		Claim: "username",
		Value: username,
	})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "oidcmapping: error getting user by claim username (\"%v\")", username)
	}
	if getUserByClaimResp.Status.Code != rpc.Code_CODE_OK {
		return nil, nil, status.NewErrorFromCode(getUserByClaimResp.Status.Code, "oidcmapping")
	}

	userID.Idp = getUserByClaimResp.GetUser().GetId().Idp
	userID.Type = getUserByClaimResp.GetUser().GetId().Type

	getGroupsResp, err := gwc.GetUserGroups(ctx, &user.GetUserGroupsRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "oidcmapping: error getting user groups")
	}
	if getGroupsResp.Status.Code != rpc.Code_CODE_OK {
		return nil, nil, status.NewErrorFromCode(getGroupsResp.Status.Code, "oidcmapping")
	}

	u := &user.User{
		Id:           userID,
		Username:     getUserByClaimResp.GetUser().GetUsername(),
		Groups:       getUserByClaimResp.GetUser().GetGroups(),
		Mail:         claims["email"].(string),
		MailVerified: claims["email_verified"].(bool),
		DisplayName:  claims["name"].(string),
		UidNumber:    int64(uid),
		GidNumber:    int64(gid),
	}

	var scopes map[string]*authpb.Scope
	scopes, err = scope.AddOwnerScope(nil)
	if err != nil {
		return nil, nil, err
	}

	return u, scopes, nil
}

func (am *mgr) getOAuthCtx(ctx context.Context) context.Context {
	// Sometimes for testing we need to skip the TLS check, that's why we need a
	// custom HTTP client.
	customHTTPClient := rhttp.GetHTTPClient(
		rhttp.Context(ctx),
		rhttp.Timeout(time.Second*10),
		rhttp.Insecure(am.c.Insecure),
		// Fixes connection fd leak which might be caused by provider-caching
		rhttp.DisableKeepAlive(true),
	)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, customHTTPClient)
	return ctx
}

// getOIDCProvider returns a singleton OIDC provider
func (am *mgr) getOIDCProvider(ctx context.Context) (*oidc.Provider, error) {
	ctx = am.getOAuthCtx(ctx)
	log := appctx.GetLogger(ctx)

	if am.provider != nil {
		return am.provider, nil
	}

	// Initialize a provider by specifying the issuer URL.
	// Once initialized is a singleton that is reused if further requests.
	// The provider is responsible to verify the token sent by the client
	// against the security keys oftentimes available in the .well-known endpoint.
	provider, err := oidc.NewProvider(ctx, am.c.Issuer)

	if err != nil {
		log.Error().Err(err).Msg("oidcmapping: error creating a new oidc provider")
		return nil, fmt.Errorf("oidcmapping: error creating a new oidc provider: %+v", err)
	}

	am.provider = provider
	return am.provider, nil
}
