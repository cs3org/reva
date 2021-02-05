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
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/go-ldap/ldap/v3"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("ldap", New)
}

type manager struct {
	c           *config
	userfilter  *template.Template
	groupfilter *template.Template
}

type config struct {
	Hostname        string     `mapstructure:"hostname"`
	Port            int        `mapstructure:"port"`
	BaseDN          string     `mapstructure:"base_dn"`
	UserFilter      string     `mapstructure:"userfilter"`
	AttributeFilter string     `mapstructure:"attributefilter"`
	FindFilter      string     `mapstructure:"findfilter"`
	GroupFilter     string     `mapstructure:"groupfilter"`
	BindUsername    string     `mapstructure:"bind_username"`
	BindPassword    string     `mapstructure:"bind_password"`
	Idp             string     `mapstructure:"idp"`
	Schema          attributes `mapstructure:"schema"`
}

type attributes struct {
	// DN is the distinguished name in ldap, e.g. `cn=einstein,ou=users,dc=example,dc=org`
	DN string `mapstructure:"dn"`
	// UID is an immutable user id, see https://docs.microsoft.com/en-us/azure/active-directory/hybrid/plan-connect-design-concepts
	UID string `mapstructure:"uid"`
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
	DN:          "dn",
	UID:         "ms-DS-ConsistencyGuid", // you can fall back to objectguid or even samaccountname but you will run into trouble when user names change. You have been warned.
	CN:          "cn",
	Mail:        "mail",
	DisplayName: "displayName",
	UIDNumber:   "uidNumber",
	GIDNumber:   "gidNumber",
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
		c: c,
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
		m.getUserFilter(uid),
		[]string{m.c.Schema.DN, m.c.Schema.UID, m.c.Schema.CN, m.c.Schema.Mail, m.c.Schema.DisplayName, m.c.Schema.UIDNumber, m.c.Schema.GIDNumber},
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
		Idp:      m.c.Idp,
		OpaqueId: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.UID),
	}
	groups, err := m.GetUserGroups(ctx, id)
	if err != nil {
		return nil, err
	}
	u := &userpb.User{
		Id:          id,
		Username:    sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.CN),
		Groups:      groups,
		Mail:        sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.Mail),
		DisplayName: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.DisplayName),
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"uid": {
					Decoder: "plain",
					Value:   []byte(sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.UIDNumber)),
				},
				"gid": {
					Decoder: "plain",
					Value:   []byte(sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.GIDNumber)),
				},
			},
		},
	}

	return u, nil
}

func (m *manager) GetUserByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	// TODO align supported claims with rest driver and the others, maybe refactor into common mapping
	switch claim {
	case "mail":
		claim = m.c.Schema.Mail
	case "uid":
		claim = m.c.Schema.UIDNumber
	case "gid":
		claim = m.c.Schema.GIDNumber
	case "username":
		claim = m.c.Schema.CN
	case "userid":
		claim = m.c.Schema.UID
	default:
		return nil, errors.New("ldap: invalid field " + claim)
	}

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
		m.getAttributeFilter(claim, value),
		[]string{m.c.Schema.DN, m.c.Schema.UID, m.c.Schema.CN, m.c.Schema.Mail, m.c.Schema.DisplayName, m.c.Schema.UIDNumber, m.c.Schema.GIDNumber},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	if len(sr.Entries) != 1 {
		return nil, errtypes.NotFound(claim + ":" + value)
	}

	log.Debug().Interface("entries", sr.Entries).Msg("entries")

	id := &userpb.UserId{
		Idp:      m.c.Idp,
		OpaqueId: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.UID),
	}
	groups, err := m.GetUserGroups(ctx, id)
	if err != nil {
		return nil, err
	}
	u := &userpb.User{
		Id:          id,
		Username:    sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.CN),
		Groups:      groups,
		Mail:        sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.Mail),
		DisplayName: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.DisplayName),
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"uid": {
					Decoder: "plain",
					Value:   []byte(sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.UIDNumber)),
				},
				"gid": {
					Decoder: "plain",
					Value:   []byte(sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.GIDNumber)),
				},
			},
		},
	}

	return u, nil

}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {
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
		m.getFindFilter(query),
		[]string{m.c.Schema.DN, m.c.Schema.UID, m.c.Schema.CN, m.c.Schema.Mail, m.c.Schema.DisplayName, m.c.Schema.UIDNumber, m.c.Schema.GIDNumber},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	users := []*userpb.User{}

	for _, entry := range sr.Entries {
		id := &userpb.UserId{
			Idp:      m.c.Idp,
			OpaqueId: entry.GetEqualFoldAttributeValue(m.c.Schema.UID),
		}
		groups, err := m.GetUserGroups(ctx, id)
		if err != nil {
			return nil, err
		}
		user := &userpb.User{
			Id:          id,
			Username:    entry.GetEqualFoldAttributeValue(m.c.Schema.CN),
			Groups:      groups,
			Mail:        entry.GetEqualFoldAttributeValue(m.c.Schema.Mail),
			DisplayName: entry.GetEqualFoldAttributeValue(m.c.Schema.DisplayName),
			Opaque: &types.Opaque{
				Map: map[string]*types.OpaqueEntry{
					"uid": {
						Decoder: "plain",
						Value:   []byte(entry.GetEqualFoldAttributeValue(m.c.Schema.UIDNumber)),
					},
					"gid": {
						Decoder: "plain",
						Value:   []byte(entry.GetEqualFoldAttributeValue(m.c.Schema.GIDNumber)),
					},
				},
			},
		}
		users = append(users, user)
	}

	return users, nil
}

func (m *manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {
	l, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", m.c.Hostname, m.c.Port), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return []string{}, err
	}
	defer l.Close()

	// First bind with a read only user
	err = l.Bind(m.c.BindUsername, m.c.BindPassword)
	if err != nil {
		return []string{}, err
	}

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		m.c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		m.getGroupFilter(uid),
		[]string{m.c.Schema.CN}, // TODO use DN to look up group id
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
		groups = append(groups, entry.GetEqualFoldAttributeValue(m.c.Schema.CN))
	}

	return groups, nil
}

func (m *manager) getUserFilter(uid *userpb.UserId) string {
	b := bytes.Buffer{}
	if err := m.userfilter.Execute(&b, uid); err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error executing user template: userid:%+v", uid))
		panic(err)
	}
	return b.String()
}

func (m *manager) getAttributeFilter(attribute, value string) string {
	attr := strings.ReplaceAll(m.c.AttributeFilter, "{{attr}}", attribute)
	return strings.ReplaceAll(attr, "{{value}}", value)
}

func (m *manager) getFindFilter(query string) string {
	return strings.ReplaceAll(m.c.FindFilter, "{{query}}", query)
}

func (m *manager) getGroupFilter(uid *userpb.UserId) string {
	b := bytes.Buffer{}
	if err := m.groupfilter.Execute(&b, uid); err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error executing group template: userid:%+v", uid))
		panic(err)
	}
	return b.String()
}
