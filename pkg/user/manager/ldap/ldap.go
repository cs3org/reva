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

package ldap

import (
	"context"
	"crypto/tls"
	"fmt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"gopkg.in/ldap.v2"
)

func init() {
	registry.Register("ldap", New)
}

type manager struct {
	hostname     string
	port         int
	baseDN       string
	userfilter   string
	groupfilter  string
	bindUsername string
	bindPassword string
	idp          string
	schema       attributes
}

type config struct {
	Hostname     string     `mapstructure:"hostname"`
	Port         int        `mapstructure:"port"`
	BaseDN       string     `mapstructure:"base_dn"`
	UserFilter   string     `mapstructure:"userfilter"`
	GroupFilter  string     `mapstructure:"groupfilter"`
	BindUsername string     `mapstructure:"bind_username"`
	BindPassword string     `mapstructure:"bind_password"`
	Idp          string     `mapstructure:"idp"`
	Schema       attributes `mapstructure:"schema"`
}

type attributes struct {
	Mail        string `mapstructure:"mail"`
	UID         string `mapstructure:"uid"`
	DisplayName string `mapstructure:"displayName"`
	DN          string `mapstructure:"dn"`
	CN          string `mapstructure:"cn"`
}

// Default attributes (Active Directory)
var ldapDefaults = attributes{
	Mail:        "mail",
	UID:         "objectGUID",
	DisplayName: "displayName",
	DN:          "dn",
	CN:          "cn",
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := config{
		Schema: ldapDefaults,
	}
	if err := mapstructure.Decode(m, &c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}

	return &c, nil
}

// New returns a user manager implementation that connects to a LDAP server to provide user metadata.
func New(m map[string]interface{}) (user.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &manager{
		hostname:     c.Hostname,
		port:         c.Port,
		baseDN:       c.BaseDN,
		userfilter:   c.UserFilter,
		groupfilter:  c.GroupFilter,
		bindUsername: c.BindUsername,
		bindPassword: c.BindPassword,
		idp:          c.Idp,
		schema:       c.Schema,
	}, nil
}

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {
	log := appctx.GetLogger(ctx)
	l, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", m.hostname, m.port), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// First bind with a read only user
	err = l.Bind(m.bindUsername, m.bindPassword)
	if err != nil {
		return nil, err
	}

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		m.baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(m.userfilter, uid.OpaqueId), // TODO this is screaming for errors if filter contains >1 %s
		[]string{m.schema.DN, m.schema.UID, m.schema.Mail, m.schema.DisplayName},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	if len(sr.Entries) != 1 {
		return nil, errtypes.NotFound(uid.OpaqueId)
	}

	log.Debug().Interface("entries", sr.Entries).Msg("entries")

	id := &userpb.UserId{
		Idp:      m.idp,
		OpaqueId: sr.Entries[0].GetAttributeValue(m.schema.UID),
	}
	groups, err := m.GetUserGroups(ctx, id)
	if err != nil {
		return nil, err
	}
	u := &userpb.User{
		Id:          id,
		Username:    sr.Entries[0].GetAttributeValue(m.schema.UID),
		Groups:      groups,
		Mail:        sr.Entries[0].GetAttributeValue(m.schema.Mail),
		DisplayName: sr.Entries[0].GetAttributeValue(m.schema.DisplayName),
	}

	return u, nil
}

func (m *manager) GetUserByUID(ctx context.Context, uid string) (*userpb.User, error) {
	return nil, errtypes.NotSupported("ldap: looking up user by UID not supported")
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {
	l, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", m.hostname, m.port), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// First bind with a read only user
	err = l.Bind(m.bindUsername, m.bindPassword)
	if err != nil {
		return nil, err
	}

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		m.baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(m.userfilter, query), // TODO this is screaming for errors if filter contains >1 %s
		[]string{m.schema.DN, m.schema.UID, m.schema.Mail, m.schema.DisplayName},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	users := []*userpb.User{}

	for _, entry := range sr.Entries {
		id := &userpb.UserId{
			Idp:      m.idp,
			OpaqueId: entry.GetAttributeValue(m.schema.UID),
		}
		groups, err := m.GetUserGroups(ctx, id)
		if err != nil {
			return nil, err
		}
		user := &userpb.User{
			Id:          id,
			Username:    entry.GetAttributeValue(m.schema.UID),
			Groups:      groups,
			Mail:        entry.GetAttributeValue(m.schema.Mail),
			DisplayName: entry.GetAttributeValue(m.schema.DisplayName),
		}
		users = append(users, user)
	}

	return users, nil
}

func (m *manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {
	l, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", m.hostname, m.port), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return []string{}, err
	}
	defer l.Close()

	// First bind with a read only user
	err = l.Bind(m.bindUsername, m.bindPassword)
	if err != nil {
		return []string{}, err
	}

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		m.baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(m.groupfilter, uid.OpaqueId), // TODO this is screaming for errors if filter contains >1 %s
		[]string{m.schema.CN},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return []string{}, err
	}

	groups := []string{}

	for _, entry := range sr.Entries {
		groups = append(groups, entry.GetAttributeValue(m.schema.CN))
	}

	return groups, nil
}

func (m *manager) IsInGroup(ctx context.Context, uid *userpb.UserId, group string) (bool, error) {
	groups, err := m.GetUserGroups(ctx, uid)
	if err != nil {
		return false, err
	}

	for _, g := range groups {
		if g == group {
			return true, nil
		}
	}

	return false, nil
}
