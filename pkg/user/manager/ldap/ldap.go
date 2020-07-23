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
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
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
	userfilter   *template.Template
	findfilter   string
	groupfilter  *template.Template
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
	FindFilter   string     `mapstructure:"findfilter"`
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
	Mail: "mail",
	// An immutable user id, see https://docs.microsoft.com/en-us/azure/active-directory/hybrid/plan-connect-design-concepts
	UID:         "ms-DS-ConsistencyGuid", // you can fall back to objectguid or even samaccountname but you will run into trouble when user names change. You have been warned.
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

	// backwards compatibility
	c.UserFilter = strings.ReplaceAll(c.UserFilter, "%s", "{{.OpaqueId}}")
	if c.FindFilter == "" {
		c.FindFilter = c.UserFilter
	}
	c.GroupFilter = strings.ReplaceAll(c.GroupFilter, "%s", "{{.OpaqueId}}")

	mgr := &manager{
		hostname:     c.Hostname,
		port:         c.Port,
		baseDN:       c.BaseDN,
		findfilter:   c.FindFilter,
		bindUsername: c.BindUsername,
		bindPassword: c.BindPassword,
		idp:          c.Idp,
		schema:       c.Schema,
	}

	mgr.userfilter, err = template.New("uf").Funcs(sprig.TxtFuncMap()).Parse(c.UserFilter)
	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error parsing userfilter tpl:%s", c.UserFilter))
		panic(err)
	}
	mgr.groupfilter, err = template.New("gf").Funcs(sprig.TxtFuncMap()).Parse(c.GroupFilter)
	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error parsing groupfilter tpl:%s", c.GroupFilter))
		panic(err)
	}

	return mgr, nil
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
		m.getUserFilter(uid),
		[]string{m.schema.DN, m.schema.CN, m.schema.UID, m.schema.Mail, m.schema.DisplayName},
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
		Username:    sr.Entries[0].GetAttributeValue(m.schema.CN),
		Groups:      groups,
		Mail:        sr.Entries[0].GetAttributeValue(m.schema.Mail),
		DisplayName: sr.Entries[0].GetAttributeValue(m.schema.DisplayName),
	}

	return u, nil
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
		m.getFindFilter(query),
		[]string{m.schema.DN, m.schema.CN, m.schema.UID, m.schema.Mail, m.schema.DisplayName},
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
			Username:    entry.GetAttributeValue(m.schema.CN),
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
		m.getGroupFilter(uid),
		[]string{m.schema.CN}, // TODO use DN to look up group id
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return []string{}, err
	}

	groups := []string{}

	for _, entry := range sr.Entries {
		// FIXME this makes the users groups use the cn, not an immutable id
		// FIXME 1. use the memberof or members attribute of a user to get the groups
		// FIXME 2. ook up the id for each group
		groups = append(groups, entry.GetAttributeValue(m.schema.CN))
	}

	return groups, nil
}

func (m *manager) IsInGroup(ctx context.Context, uid *userpb.UserId, group string) (bool, error) {
	// TODO implement with dedicated ldap query
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

func (m *manager) getUserFilter(uid *userpb.UserId) string {
	b := bytes.Buffer{}
	if err := m.userfilter.Execute(&b, uid); err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error executing user template: userid:%+v", uid))
		panic(err)
	}
	return b.String()
}

func (m *manager) getFindFilter(query string) string {
	return strings.ReplaceAll(m.findfilter, "{{query}}", query)
}

func (m *manager) getGroupFilter(uid *userpb.UserId) string {
	b := bytes.Buffer{}
	if err := m.groupfilter.Execute(&b, uid); err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error executing group template: userid:%+v", uid))
		panic(err)
	}
	return b.String()
}
