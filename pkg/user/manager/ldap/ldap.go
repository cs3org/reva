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
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("ldap", New)
}

type manager struct {
	c          *config
	userfilter *template.Template
}

type config struct {
	utils.LDAPConn  `mapstructure:",squash"`
	BaseDN          string     `mapstructure:"base_dn"`
	UserFilter      string     `mapstructure:"userfilter"`
	AttributeFilter string     `mapstructure:"attributefilter"`
	FindFilter      string     `mapstructure:"findfilter"`
	GroupFilter     string     `mapstructure:"groupfilter"`
	Idp             string     `mapstructure:"idp"`
	Schema          attributes `mapstructure:"schema"`
	Nobody          int64      `mapstructure:"nobody"`
}

type attributes struct {
	// DN is the distinguished name in ldap, e.g. `cn=einstein,ou=users,dc=example,dc=org`
	DN string `mapstructure:"dn"`
	// UID is an immutable user id, see https://docs.microsoft.com/en-us/azure/active-directory/hybrid/plan-connect-design-concepts
	UID string `mapstructure:"uid"`
	// UIDIsOctetString set this to true if the values of the UID attribute are returned as OCTET STRING values (binary byte sequences)
	// by the Directory Service. This is e.g. the case for the 'objectGUID' and	'ms-DS-ConsistencyGuid' Attributes in AD
	UIDIsOctetString bool `mapstructure:"uidIsOctetString"`
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
	// GID is an immutable group id
	GID string `mapstructure:"gid"`
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
	GID:              "cn",
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
	mgr := &manager{}
	err := mgr.Configure(m)
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

func (m *manager) Configure(ml map[string]interface{}) error {
	c, err := parseConfig(ml)
	if err != nil {
		return err
	}

	// backwards compatibility
	c.UserFilter = strings.ReplaceAll(c.UserFilter, "%s", "{{.OpaqueId}}")
	if c.FindFilter == "" {
		c.FindFilter = c.UserFilter
	}

	if c.Nobody == 0 {
		c.Nobody = 99
	}

	m.c = c
	m.userfilter, err = template.New("uf").Funcs(sprig.TxtFuncMap()).Parse(c.UserFilter)
	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error parsing userfilter tpl:%s", c.UserFilter))
		panic(err)
	}
	return nil
}

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {
	log := appctx.GetLogger(ctx)

	if uid.Idp != "" && uid.Idp != m.c.Idp {
		return nil, errtypes.NotFound("idp mismatch")
	}

	l, err := utils.GetLDAPConnection(&m.c.LDAPConn)
	if err != nil {
		return nil, err
	}
	defer l.Close()

	userEntry, err := m.getLDAPUserByID(ctx, l, uid)
	if err != nil {
		return nil, err
	}

	log.Debug().Interface("entry", userEntry).Msg("entries")

	id, err := m.ldapEntryToUserID(userEntry)
	if err != nil {
		return nil, err
	}

	groups, err := m.getLDAPUserGroups(ctx, l, userEntry)
	if err != nil {
		return nil, err
	}

	gidNumber := m.c.Nobody
	gidValue := userEntry.GetEqualFoldAttributeValue(m.c.Schema.GIDNumber)
	if gidValue != "" {
		gidNumber, err = strconv.ParseInt(gidValue, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	uidNumber := m.c.Nobody
	uidValue := userEntry.GetEqualFoldAttributeValue(m.c.Schema.UIDNumber)
	if uidValue != "" {
		uidNumber, err = strconv.ParseInt(uidValue, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	u := &userpb.User{
		Id:          id,
		Username:    userEntry.GetEqualFoldAttributeValue(m.c.Schema.CN),
		Groups:      groups,
		Mail:        userEntry.GetEqualFoldAttributeValue(m.c.Schema.Mail),
		DisplayName: userEntry.GetEqualFoldAttributeValue(m.c.Schema.DisplayName),
		GidNumber:   gidNumber,
		UidNumber:   uidNumber,
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

	id, err := m.ldapEntryToUserID(sr.Entries[0])
	if err != nil {
		return nil, err
	}
	groups, err := m.getLDAPUserGroups(ctx, l, sr.Entries[0])
	if err != nil {
		return nil, err
	}
	gidNumber := m.c.Nobody
	gidValue := sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.GIDNumber)
	if gidValue != "" {
		gidNumber, err = strconv.ParseInt(gidValue, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	uidNumber := m.c.Nobody
	uidValue := sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.UIDNumber)
	if uidValue != "" {
		uidNumber, err = strconv.ParseInt(uidValue, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	u := &userpb.User{
		Id:          id,
		Username:    sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.CN),
		Groups:      groups,
		Mail:        sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.Mail),
		DisplayName: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.DisplayName),
		GidNumber:   gidNumber,
		UidNumber:   uidNumber,
	}

	return u, nil

}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {
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
		[]string{m.c.Schema.DN, m.c.Schema.UID, m.c.Schema.CN, m.c.Schema.Mail, m.c.Schema.DisplayName, m.c.Schema.UIDNumber, m.c.Schema.GIDNumber},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	users := []*userpb.User{}

	for _, entry := range sr.Entries {
		id, err := m.ldapEntryToUserID(entry)
		if err != nil {
			return nil, err
		}
		groups, err := m.getLDAPUserGroups(ctx, l, entry)
		if err != nil {
			return nil, err
		}
		gidNumber := m.c.Nobody
		gidValue := sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.GIDNumber)
		if gidValue != "" {
			gidNumber, err = strconv.ParseInt(gidValue, 10, 64)
			if err != nil {
				return nil, err
			}
		}
		uidNumber := m.c.Nobody
		uidValue := sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.UIDNumber)
		if uidValue != "" {
			uidNumber, err = strconv.ParseInt(uidValue, 10, 64)
			if err != nil {
				return nil, err
			}
		}
		user := &userpb.User{
			Id:          id,
			Username:    entry.GetEqualFoldAttributeValue(m.c.Schema.CN),
			Groups:      groups,
			Mail:        entry.GetEqualFoldAttributeValue(m.c.Schema.Mail),
			DisplayName: entry.GetEqualFoldAttributeValue(m.c.Schema.DisplayName),
			GidNumber:   gidNumber,
			UidNumber:   uidNumber,
		}
		users = append(users, user)
	}

	return users, nil
}

func (m *manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {
	l, err := utils.GetLDAPConnection(&m.c.LDAPConn)
	if err != nil {
		return []string{}, err
	}
	defer l.Close()

	userEntry, err := m.getLDAPUserByID(ctx, l, uid)
	if err != nil {
		return []string{}, err
	}
	return m.getLDAPUserGroups(ctx, l, userEntry)
}

func (m *manager) ldapEntryToUserID(entry *ldap.Entry) (*userpb.UserId, error) {
	var uid string
	if m.c.Schema.UIDIsOctetString {
		rawValue := entry.GetEqualFoldRawAttributeValue(m.c.Schema.UID)
		if value, err := uuid.FromBytes(rawValue); err == nil {
			uid = value.String()
		} else {
			return nil, err
		}
	} else {
		uid = entry.GetEqualFoldAttributeValue(m.c.Schema.UID)
	}

	return &userpb.UserId{
		Idp:      m.c.Idp,
		OpaqueId: uid,
		Type:     userpb.UserType_USER_TYPE_PRIMARY,
	}, nil
}

func (m *manager) getLDAPUserByID(ctx context.Context, conn *ldap.Conn, uid *userpb.UserId) (*ldap.Entry, error) {
	log := appctx.GetLogger(ctx)
	// Search for the given clientID, use a sizeLimit of 1 to be able
	// to error out early when the userid is not unique
	searchRequest := ldap.NewSearchRequest(
		m.c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
		m.getUserFilter(uid),
		[]string{m.c.Schema.DN, m.c.Schema.UID, m.c.Schema.CN, m.c.Schema.Mail, m.c.Schema.DisplayName, m.c.Schema.UIDNumber, m.c.Schema.GIDNumber},
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		if lerr, ok := err.(*ldap.Error); ok {
			if lerr.ResultCode == ldap.LDAPResultSizeLimitExceeded {
				log.Error().Err(lerr).Msg(fmt.Sprintf("userid '%s' is not unique", uid))
			}
		}
		return nil, errtypes.NotFound(uid.OpaqueId)
	}

	if len(sr.Entries) == 0 {
		return nil, errtypes.NotFound(uid.OpaqueId)
	}
	return sr.Entries[0], nil

}

func (m *manager) getLDAPUserGroups(ctx context.Context, conn *ldap.Conn, userEntry *ldap.Entry) ([]string, error) {
	username := userEntry.GetEqualFoldAttributeValue(m.c.Schema.CN)
	searchRequest := ldap.NewSearchRequest(
		m.c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		m.getGroupFilter(username),
		[]string{m.c.Schema.GID}, // TODO use DN to look up group id
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return []string{}, err
	}

	groups := []string{}

	for _, entry := range sr.Entries {
		// FIXME this makes the users groups use the cn, not an immutable id
		// FIXME 1. use the memberof or members attribute of a user to get the groups
		// FIXME 2. ook up the id for each group
		groups = append(groups, entry.GetEqualFoldAttributeValue(m.c.Schema.GID))
	}
	return groups, nil
}

func (m *manager) getUserFilter(uid *userpb.UserId) string {
	uidTmp := uid
	if m.c.Schema.UIDIsOctetString {
		uuid, err := uuid.Parse(uid.OpaqueId)
		if err != nil {
			err := errors.Wrap(err, fmt.Sprintf("error parsing OpaqueID '%s' as UUID", uid.OpaqueId))
			panic(err)
		}
		escapedUID := *uid
		escapedUID.OpaqueId = filterEscapeBinaryUUID(uuid)
		uidTmp = &escapedUID
	}

	b := bytes.Buffer{}
	if err := m.userfilter.Execute(&b, uidTmp); err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error executing user template: userid:%+v", uid))
		panic(err)
	}
	return b.String()
}

func filterEscapeBinaryUUID(value uuid.UUID) string {
	filtered := ""
	for _, b := range value {
		filtered = fmt.Sprintf("%s\\%02x", filtered, b)
	}
	return filtered
}

func (m *manager) getAttributeFilter(attribute, value string) string {
	attr := strings.ReplaceAll(m.c.AttributeFilter, "{{attr}}", ldap.EscapeFilter(attribute))
	return strings.ReplaceAll(attr, "{{value}}", ldap.EscapeFilter(value))
}

func (m *manager) getFindFilter(query string) string {
	return strings.ReplaceAll(m.c.FindFilter, "{{query}}", ldap.EscapeFilter(query))
}

func (m *manager) getGroupFilter(memberName string) string {
	return strings.ReplaceAll(m.c.GroupFilter, "{{query}}", ldap.EscapeFilter(memberName))
}
