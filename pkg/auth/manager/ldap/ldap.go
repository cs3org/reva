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

package ldap

import (
	"context"
	"strconv"
	"strings"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/auth"
	"github.com/cs3org/reva/v2/pkg/auth/manager/registry"
	"github.com/cs3org/reva/v2/pkg/auth/scope"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/logger"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/sharedconf"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("ldap", New)
}

type mgr struct {
	c          *config
	ldapClient ldap.Client
}

type config struct {
	utils.LDAPConn `mapstructure:",squash"`
	BaseDN         string     `mapstructure:"base_dn"`
	UserFilter     string     `mapstructure:"userfilter"`
	LoginFilter    string     `mapstructure:"loginfilter"`
	Idp            string     `mapstructure:"idp"`
	GatewaySvc     string     `mapstructure:"gatewaysvc"`
	Schema         attributes `mapstructure:"schema"`
	Nobody         int64      `mapstructure:"nobody"`
}

type attributes struct {
	// DN is the distinguished name in ldap, e.g. `cn=einstein,ou=users,dc=example,dc=org`
	DN string `mapstructure:"dn"`
	// UID is an immutable user id, see https://docs.microsoft.com/en-us/azure/active-directory/hybrid/plan-connect-design-concepts
	UID              string `mapstructure:"uid"`
	UIDIsOctetString bool   `mapstructure:"uidIsOctetString"`
	// CN is the username, typically `cn`, `uid` or `samaccountname`
	CN string `mapstructure:"cn"`
	// Mail is the email address of a user
	Mail string `mapstructure:"mail"`
	// Displayname is the Human readable name, e.g. `Albert Einstein`
	DisplayName string `mapstructure:"displayName"`
	// UIDNumber is a numeric id that maps to a filesystem uid, eg. 123546
	UIDNumber string `mapstructure:"uidNumber"`
	// GIDNumber is a numeric id that maps to a filesystem gid, eg. 654321
	GIDNumber string `mapstructure:"gidNumber"`
}

// Default attributes (Active Directory)
var ldapDefaults = attributes{
	DN:               "dn",
	UID:              "ms-DS-ConsistencyGuid", // you can fall back to objectguid or even samaccountname but you will run into trouble when user names change. You have been warned.
	UIDIsOctetString: false,
	CN:               "cn",
	Mail:             "mail",
	DisplayName:      "displayName",
	UIDNumber:        "uidNumber",
	GIDNumber:        "gidNumber",
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{
		Schema: ldapDefaults,
	}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns an auth manager implementation that connects to a LDAP server to validate the user.
func New(m map[string]interface{}) (auth.Manager, error) {
	manager := &mgr{}
	err := manager.Configure(m)
	if err != nil {
		return nil, err
	}
	manager.ldapClient, err = utils.GetLDAPClientWithReconnect(&manager.c.LDAPConn)
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

	// backwards compatibility
	if c.UserFilter != "" {
		logger.New().Warn().Msg("userfilter is deprecated, use a loginfilter like `(&(objectclass=posixAccount)(|(cn={{login}}))(mail={{login}}))`")
	}
	if c.LoginFilter == "" {
		c.LoginFilter = c.UserFilter
		c.LoginFilter = strings.ReplaceAll(c.LoginFilter, "%s", "{{login}}")
	}
	if c.Nobody == 0 {
		c.Nobody = 99
	}

	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
	am.c = c
	return nil
}

func (am *mgr) Authenticate(ctx context.Context, clientID, clientSecret string) (*user.User, map[string]*authpb.Scope, error) {
	log := appctx.GetLogger(ctx)

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		am.c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		am.getLoginFilter(clientID),
		[]string{am.c.Schema.DN, am.c.Schema.UID, am.c.Schema.CN, am.c.Schema.Mail, am.c.Schema.DisplayName, am.c.Schema.UIDNumber, am.c.Schema.GIDNumber},
		nil,
	)

	sr, err := am.ldapClient.Search(searchRequest)
	if err != nil {
		return nil, nil, err
	}

	if len(sr.Entries) != 1 {
		return nil, nil, errtypes.NotFound(clientID)
	}

	userdn := sr.Entries[0].DN

	// Bind as the user to verify their password
	la, err := utils.GetLDAPClientForAuth(&am.c.LDAPConn)
	if err != nil {
		return nil, nil, err
	}
	defer la.Close()
	err = la.Bind(userdn, clientSecret)
	if err != nil {
		log.Debug().Err(err).Interface("userdn", userdn).Msg("bind with user credentials failed")
		return nil, nil, err
	}

	var uid string
	if am.c.Schema.UIDIsOctetString {
		rawValue := sr.Entries[0].GetEqualFoldRawAttributeValue(am.c.Schema.UID)
		if value, err := uuid.FromBytes(rawValue); err == nil {
			uid = value.String()
		}
	} else {
		uid = sr.Entries[0].GetEqualFoldAttributeValue(am.c.Schema.UID)
	}

	userID := &user.UserId{
		Idp:      am.c.Idp,
		OpaqueId: uid,
		Type:     user.UserType_USER_TYPE_PRIMARY, // TODO: assign the appropriate user type
	}
	gwc, err := pool.GetGatewayServiceClient(am.c.GatewaySvc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "ldap: error getting gateway grpc client")
	}
	getGroupsResp, err := gwc.GetUserGroups(ctx, &user.GetUserGroupsRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "ldap: error getting user groups")
	}
	if getGroupsResp.Status.Code != rpc.Code_CODE_OK {
		return nil, nil, errors.Wrap(err, "ldap: grpc getting user groups failed")
	}
	gidNumber := am.c.Nobody
	gidValue := sr.Entries[0].GetEqualFoldAttributeValue(am.c.Schema.GIDNumber)
	if gidValue != "" {
		gidNumber, err = strconv.ParseInt(gidValue, 10, 64)
		if err != nil {
			return nil, nil, err
		}
	}
	uidNumber := am.c.Nobody
	uidValue := sr.Entries[0].GetEqualFoldAttributeValue(am.c.Schema.UIDNumber)
	if uidValue != "" {
		uidNumber, err = strconv.ParseInt(uidValue, 10, 64)
		if err != nil {
			return nil, nil, err
		}
	}
	u := &user.User{
		Id: userID,
		// TODO add more claims from the StandardClaims, eg EmailVerified
		Username: sr.Entries[0].GetEqualFoldAttributeValue(am.c.Schema.CN),
		// TODO groups
		Groups:      getGroupsResp.Groups,
		Mail:        sr.Entries[0].GetEqualFoldAttributeValue(am.c.Schema.Mail),
		DisplayName: sr.Entries[0].GetEqualFoldAttributeValue(am.c.Schema.DisplayName),
		UidNumber:   uidNumber,
		GidNumber:   gidNumber,
	}

	var scopes map[string]*authpb.Scope
	if userID != nil && userID.Type == user.UserType_USER_TYPE_LIGHTWEIGHT {
		scopes, err = scope.AddLightweightAccountScope(authpb.Role_ROLE_OWNER, nil)
		if err != nil {
			return nil, nil, err
		}
	} else {
		scopes, err = scope.AddOwnerScope(nil)
		if err != nil {
			return nil, nil, err
		}
	}

	log.Debug().Interface("entry", sr.Entries[0]).Interface("user", u).Msg("authenticated user")

	return u, scopes, nil

}

func (am *mgr) getLoginFilter(login string) string {
	return strings.ReplaceAll(am.c.LoginFilter, "{{login}}", ldap.EscapeFilter(login))
}
