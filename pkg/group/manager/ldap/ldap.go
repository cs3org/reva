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

package ldap

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/group"
	"github.com/cs3org/reva/v3/pkg/group/manager/registry"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/go-ldap/ldap/v3"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("ldap", New)
}

type manager struct {
	c            *config
	groupfilter  *template.Template
	memberfilter *template.Template
}

type config struct {
	utils.LDAPConn  `mapstructure:",squash"`
	BaseDN          string     `mapstructure:"base_dn"`
	GroupFilter     string     `mapstructure:"groupfilter"`
	MemberFilter    string     `mapstructure:"memberfilter"`
	AttributeFilter string     `mapstructure:"attributefilter"`
	FindFilter      string     `mapstructure:"findfilter"`
	Idp             string     `mapstructure:"idp"`
	Schema          attributes `mapstructure:"schema"`
	Nobody          int64      `mapstructure:"nobody"`
}

type attributes struct {
	// DN is the distinguished name in ldap, e.g. `cn=admins,ou=groups,dc=example,dc=org`
	DN string `mapstructure:"dn"`
	// GID is an immutable group id, see https://docs.microsoft.com/en-us/azure/active-directory/hybrid/plan-connect-design-concepts
	GID string `mapstructure:"gid"`
	// CN is the group name, typically `cn`, `gid` or `samaccountname`
	CN string `mapstructure:"cn"`
	// Mail is the email address of a group
	Mail string `mapstructure:"mail"`
	// Displayname is the Human readable name, e.g. `Database Admins`
	DisplayName string `mapstructure:"displayName"`
	// GIDNumber is a numeric id that maps to a filesystem gid, eg. 654321
	GIDNumber string `mapstructure:"gidNumber"`
}

// Default attributes (Active Directory).
var ldapDefaults = attributes{
	DN:          "dn",
	GID:         "objectGUID", // you can fall back to samaccountname but you will run into trouble when group names change. You have been warned.
	CN:          "cn",
	Mail:        "mail",
	DisplayName: "displayName",
	GIDNumber:   "gidNumber",
}

// New returns a group manager implementation that connects to a LDAP server to provide group metadata.
func New(ctx context.Context, m map[string]interface{}) (group.Manager, error) {
	var c config
	c.Schema = ldapDefaults
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	c.GroupFilter = strings.ReplaceAll(c.GroupFilter, "%s", "{{.OpaqueId}}")
	if c.FindFilter == "" {
		c.FindFilter = c.GroupFilter
	}
	c.MemberFilter = strings.ReplaceAll(c.MemberFilter, "%s", "{{.OpaqueId}}")

	mgr := &manager{
		c: &c,
	}

	var err error
	mgr.groupfilter, err = template.New("gf").Funcs(sprig.TxtFuncMap()).Parse(c.GroupFilter)
	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error parsing groupfilter tpl:%s", c.GroupFilter))
		panic(err)
	}
	mgr.memberfilter, err = template.New("uf").Funcs(sprig.TxtFuncMap()).Parse(c.MemberFilter)
	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error parsing memberfilter tpl:%s", c.MemberFilter))
		panic(err)
	}

	return mgr, nil
}

func (m *manager) GetGroup(ctx context.Context, gid *grouppb.GroupId, skipFetchingMembers bool) (*grouppb.Group, error) {
	log := appctx.GetLogger(ctx)
	l, err := utils.GetLDAPConnection(&m.c.LDAPConn)
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		m.c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		m.getGroupFilter(gid),
		[]string{m.c.Schema.DN, m.c.Schema.GID, m.c.Schema.CN, m.c.Schema.Mail, m.c.Schema.DisplayName, m.c.Schema.GIDNumber},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	if len(sr.Entries) != 1 {
		return nil, errtypes.NotFound(gid.OpaqueId)
	}

	log.Debug().Interface("entries", sr.Entries).Msg("entries")

	id := &grouppb.GroupId{
		Idp:      m.c.Idp,
		OpaqueId: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.GID),
	}

	var members []*userpb.UserId
	if !skipFetchingMembers {
		members, err = m.GetMembers(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	gidNumber := m.c.Nobody
	gidValue := sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.GIDNumber)
	if gidValue != "" {
		gidNumber, err = strconv.ParseInt(gidValue, 10, 64)
		if err != nil {
			return nil, err
		}
	}

	g := &grouppb.Group{
		Id:          id,
		GroupName:   sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.CN),
		Members:     members,
		Mail:        sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.Mail),
		DisplayName: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.DisplayName),
		GidNumber:   gidNumber,
	}

	return g, nil
}

func (m *manager) GetGroupByClaim(ctx context.Context, claim, value string, skipFetchingMembers bool) (*grouppb.Group, error) {
	// TODO align supported claims with rest driver and the others, maybe refactor into common mapping
	switch claim {
	case "mail":
		claim = m.c.Schema.Mail
	case "gid_number":
		claim = m.c.Schema.GIDNumber
	case "group_name":
		claim = m.c.Schema.CN
	case "groupid":
		claim = m.c.Schema.GID
	default:
		return nil, errors.New("ldap: invalid field " + claim)
	}

	log := appctx.GetLogger(ctx)
	l, err := utils.GetLDAPConnection(&m.c.LDAPConn)
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		m.c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		m.getAttributeFilter(claim, value),
		[]string{m.c.Schema.DN, m.c.Schema.GID, m.c.Schema.CN, m.c.Schema.Mail, m.c.Schema.DisplayName, m.c.Schema.GIDNumber},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	if len(sr.Entries) != 1 {
		return nil, errtypes.NotFound(claim + ": " + value)
	}

	log.Debug().Interface("entries", sr.Entries).Msg("entries")

	id := &grouppb.GroupId{
		Idp:      m.c.Idp,
		OpaqueId: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.GID),
	}

	var members []*userpb.UserId
	if !skipFetchingMembers {
		members, err = m.GetMembers(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	gidNumber, err := strconv.ParseInt(sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.GIDNumber), 10, 64)
	if err != nil {
		return nil, err
	}

	g := &grouppb.Group{
		Id:          id,
		GroupName:   sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.CN),
		Members:     members,
		Mail:        sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.Mail),
		DisplayName: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.DisplayName),
		GidNumber:   gidNumber,
	}

	return g, nil
}

func (m *manager) FindGroups(ctx context.Context, query string, skipFetchingMembers bool) ([]*grouppb.Group, error) {
	l, err := utils.GetLDAPConnection(&m.c.LDAPConn)
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		m.c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		m.getFindFilter(query),
		[]string{m.c.Schema.DN, m.c.Schema.GID, m.c.Schema.CN, m.c.Schema.Mail, m.c.Schema.DisplayName, m.c.Schema.GIDNumber},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	groups := []*grouppb.Group{}

	for _, entry := range sr.Entries {
		id := &grouppb.GroupId{
			Idp:      m.c.Idp,
			OpaqueId: entry.GetEqualFoldAttributeValue(m.c.Schema.GID),
		}

		var members []*userpb.UserId
		if !skipFetchingMembers {
			members, err = m.GetMembers(ctx, id)
			if err != nil {
				return nil, err
			}
		}

		gidNumber, err := strconv.ParseInt(entry.GetEqualFoldAttributeValue(m.c.Schema.GIDNumber), 10, 64)
		if err != nil {
			return nil, err
		}

		g := &grouppb.Group{
			Id:          id,
			GroupName:   entry.GetEqualFoldAttributeValue(m.c.Schema.CN),
			Members:     members,
			Mail:        entry.GetEqualFoldAttributeValue(m.c.Schema.Mail),
			DisplayName: entry.GetEqualFoldAttributeValue(m.c.Schema.DisplayName),
			GidNumber:   gidNumber,
		}
		groups = append(groups, g)
	}

	return groups, nil
}

func (m *manager) GetMembers(ctx context.Context, gid *grouppb.GroupId) ([]*userpb.UserId, error) {
	l, err := utils.GetLDAPConnection(&m.c.LDAPConn)
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		m.c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		m.getMemberFilter(gid),
		[]string{m.c.Schema.CN}, // TODO use DN to look up user id
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	users := []*userpb.UserId{}
	for _, entry := range sr.Entries {
		// FIXME this makes the group members use the cn, not an immutable id
		users = append(users, &userpb.UserId{
			OpaqueId: entry.GetEqualFoldAttributeValue(m.c.Schema.CN),
			Idp:      m.c.Idp,
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
		})
	}

	return users, nil
}

func (m *manager) HasMember(ctx context.Context, gid *grouppb.GroupId, uid *userpb.UserId) (bool, error) {
	members, err := m.GetMembers(ctx, gid)
	if err != nil {
		return false, err
	}

	for _, u := range members {
		if u.OpaqueId == uid.OpaqueId && u.Idp == uid.Idp {
			return true, nil
		}
	}
	return false, nil
}

func (m *manager) getGroupFilter(gid *grouppb.GroupId) string {
	b := bytes.Buffer{}
	if err := m.groupfilter.Execute(&b, gid); err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error executing group template: groupid: %+v", gid))
		panic(err)
	}
	return b.String()
}

func (m *manager) getMemberFilter(gid *grouppb.GroupId) string {
	b := bytes.Buffer{}
	if err := m.memberfilter.Execute(&b, gid); err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error executing member template: groupid: %+v", gid))
		panic(err)
	}
	return b.String()
}

func (m *manager) getAttributeFilter(attribute, value string) string {
	attr := strings.ReplaceAll(m.c.AttributeFilter, "{{attr}}", ldap.EscapeFilter(attribute))
	return strings.ReplaceAll(attr, "{{value}}", ldap.EscapeFilter(value))
}

func (m *manager) getFindFilter(query string) string {
	return strings.ReplaceAll(m.c.FindFilter, "{{query}}", ldap.EscapeFilter(query))
}
