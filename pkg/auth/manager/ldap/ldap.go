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

package ldap

import (
	"context"
	"crypto/tls"
	"fmt"

	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	userproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/userprovider/v0alpha"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"gopkg.in/ldap.v2"
)

func init() {
	registry.Register("ldap", New)
}

type mgr struct {
	c *config
}

type config struct {
	Hostname     string     `mapstructure:"hostname"`
	Port         int        `mapstructure:"port"`
	BaseDN       string     `mapstructure:"base_dn"`
	Filter       string     `mapstructure:"filter"`
	BindUsername string     `mapstructure:"bind_username"`
	BindPassword string     `mapstructure:"bind_password"`
	Schema       attributes `mapstructure:"schema"`
}

type attributes struct {
	DN          string `mapstructure:"dn"`
	UID         string `mapstructure:"uid"`
	UserName    string `mapstructure:"userName"`
	Mail        string `mapstructure:"mail"`
	DisplayName string `mapstructure:"displayName"`
}

// Default attributes (Active Directory)
var ldapDefaults = attributes{
	DN:          "dn",
	UID:         "objectGUID",
	UserName:    "sAMAccountName",
	Mail:        "mail",
	DisplayName: "displayName",
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := config{
		Schema: ldapDefaults,
	}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return &c, nil
}

// New returns an auth manager implementation that connects to a LDAP server to validate the user.
func New(m map[string]interface{}) (auth.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &mgr{c: c}, nil
}

func (m *mgr) Authenticate(ctx context.Context, clientID, clientSecret string) (*userproviderv0alphapb.User, error) {
	log := appctx.GetLogger(ctx)

	l, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", m.c.Hostname, m.c.Port), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// First bind with a read only user
	err = l.Bind(m.c.BindUsername, m.c.BindPassword)
	if err != nil {
		return nil, err
	}

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		m.c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(m.c.Filter, clientID),
		// TODO(jfd): objectguid, entryuuid etc ... make configurable
		[]string{m.c.Schema.DN, m.c.Schema.UID, m.c.Schema.Mail, m.c.Schema.DisplayName},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	if len(sr.Entries) != 1 {
		return nil, errtypes.NotFound(clientID)
	}

	log.Debug().Interface("entries", sr.Entries).Msg("entries")

	userdn := sr.Entries[0].DN

	// Bind as the user to verify their password
	err = l.Bind(userdn, clientSecret)
	if err != nil {
		return nil, err
	}

	u := &userproviderv0alphapb.User{
		// TODO(jfd) clean up idp = iss, sub = opaque ... is redundant
		Id: &typespb.UserId{
			// TODO(jfd): how do we determine the issuer for ldap? ... make configurable
			Idp: fmt.Sprintf("%s:%d", m.c.Hostname, m.c.Port),
			// TODO(jfd): objectguid, entryuuid etc ... make configurable
			OpaqueId: sr.Entries[0].GetAttributeValue(m.c.Schema.UID),
		},
		// Subject:     claims.Sub, // TODO(labkode) remove from CS3, is in Id
		// Issuer:      claims.Iss, // TODO(labkode) remove from CS3, is in Id
		// TODO add more claims from the StandardClaims, eg EmailVerified
		Username: sr.Entries[0].GetAttributeValue(m.c.Schema.UserName),
		// TODO groups
		Groups:      []string{},
		Mail:        sr.Entries[0].GetAttributeValue(m.c.Schema.Mail),
		DisplayName: sr.Entries[0].GetAttributeValue(m.c.Schema.DisplayName),
	}
	return u, nil

}
