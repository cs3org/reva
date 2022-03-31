// Copyright 2022 CERN
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
	"fmt"
	"strings"

	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Identity provides methods to query users and groups from an LDAP server
type Identity struct {
	User  userConfig  `mapstructure:",squash"`
	Group groupConfig `mapstructure:",squash"`
}

type userConfig struct {
	BaseDN      string `mapstructure:"user_base_dn"`
	Scope       string `mapstructure:"user_search_scope"`
	scopeVal    int
	Filter      string     `mapstructure:"user_filter"`
	Objectclass string     `mapstructure:"user_objectclass"`
	Schema      userSchema `mapstructure:"user_schema"`
}

type groupConfig struct {
	BaseDN      string `mapstructure:"group_base_dn"`
	Scope       string `mapstructure:"group_search_scope"`
	scopeVal    int
	Filter      string      `mapstructure:"group_filter"`
	Objectclass string      `mapstructure:"group_objectclass"`
	Schema      groupSchema `mapstructure:"group_schema"`
}

type groupSchema struct {
	// GID is an immutable group id, see https://docs.microsoft.com/en-us/azure/active-directory/hybrid/plan-connect-design-concepts
	ID              string `mapstructure:"id"`
	IDIsOctetString bool   `mapstructure:"idIsOctetString"`
	// CN is the group name, typically `cn`, `gid` or `samaccountname`
	Groupname string `mapstructure:"groupName"`
	// Mail is the email address of a group
	Mail string `mapstructure:"mail"`
	// Displayname is the Human readable name, e.g. `Database Admins`
	DisplayName string `mapstructure:"displayName"`
	// GIDNumber is a numeric id that maps to a filesystem gid, eg. 654321
	GIDNumber string `mapstructure:"gidNumber"`
	Member    string `mapstructure:"member"`
}

type userSchema struct {
	// UID is an immutable user id, see https://docs.microsoft.com/en-us/azure/active-directory/hybrid/plan-connect-design-concepts
	ID string `mapstructure:"id"`
	// UIDIsOctetString set this to true i the values of the UID attribute are returned as OCTET STRING values (binary byte sequences)
	// by the Directory Service. This is e.g. the case for the 'objectGUID' and	'ms-DS-ConsistencyGuid' Attributes in AD
	IDIsOctetString bool `mapstructure:"idIsOctetString"`
	// Name is the username, typically `cn`, `uid` or `samaccountname`
	Username string `mapstructure:"userName"`
	// Mail is the email address of a user
	Mail string `mapstructure:"mail"`
	// Displayname is the Human readable name, e.g. `Albert Einstein`
	DisplayName string `mapstructure:"displayName"`
	// UIDNumber is a numeric id that maps to a filesystem uid, eg. 123546
	UIDNumber string `mapstructure:"uidNumber"`
	// GIDNumber is a numeric id that maps to a filesystem gid, eg. 654321
	GIDNumber string `mapstructure:"gidNumber"`
}

// Default userConfig (somewhat inspired by Active Directory)
var userDefaults = userConfig{
	Scope:       "sub",
	Objectclass: "posixAccount",
	Schema: userSchema{
		ID:              "ms-DS-ConsistencyGuid",
		IDIsOctetString: false,
		Username:        "cn",
		Mail:            "mail",
		DisplayName:     "displayName",
		UIDNumber:       "uidNumber",
		GIDNumber:       "gidNumber",
	},
}

// Default userConfig (Active Directory)
var groupDefaults = groupConfig{
	Scope:       "sub",
	Objectclass: "posixGroup",
	Schema: groupSchema{
		ID:              "objectGUID",
		IDIsOctetString: false,
		Groupname:       "cn",
		Mail:            "mail",
		DisplayName:     "cn",
		GIDNumber:       "gidNumber",
		Member:          "memberUid",
	},
}

// New initializes the default config
func New() Identity {
	return Identity{
		User:  userDefaults,
		Group: groupDefaults,
	}
}

// Setup initialzes some properties that can't be initialized from the
// mapstructure based config. Currently it just converts the LDAP search scope
// strings from the config to the integer constants expected by the ldap API
func (i *Identity) Setup() error {
	var err error
	if i.User.scopeVal, err = stringToScope(i.User.Scope); err != nil {
		return fmt.Errorf("error configuring user scope: %w", err)
	}

	if i.Group.scopeVal, err = stringToScope(i.Group.Scope); err != nil {
		return fmt.Errorf("error configuring group scope: %w", err)
	}
	return nil
}

// GetLDAPUserByID looks up a user by the supplied Id. Returns the corresponding
// ldap.Entry
func (i *Identity) GetLDAPUserByID(log *zerolog.Logger, lc ldap.Client, id string) (*ldap.Entry, error) {
	var filter string
	var err error
	if filter, err = i.getUserFilter(id); err != nil {
		return nil, err
	}
	return i.GetLDAPUserByFilter(log, lc, filter)
}

// GetLDAPUserByAttribute looks up a single user by attribute (can be "mail",
// "uid", "gid", "username" or "userid"). Returns the corresponding ldap.Entry
func (i *Identity) GetLDAPUserByAttribute(log *zerolog.Logger, lc ldap.Client, attribute, value string) (*ldap.Entry, error) {
	var filter string
	var err error
	if filter, err = i.getUserAttributeFilter(attribute, value); err != nil {
		return nil, err
	}
	return i.GetLDAPUserByFilter(log, lc, filter)
}

// GetLDAPUserByFilter looks up a single user by the supplied LDAP filter
// returns the corresponding ldap.Entry
func (i *Identity) GetLDAPUserByFilter(log *zerolog.Logger, lc ldap.Client, filter string) (*ldap.Entry, error) {
	searchRequest := ldap.NewSearchRequest(
		i.User.BaseDN, i.User.scopeVal, ldap.NeverDerefAliases, 1, 0, false,
		filter,
		[]string{
			i.User.Schema.DisplayName,
			i.User.Schema.ID,
			i.User.Schema.Mail,
			i.User.Schema.Username,
		},
		nil,
	)
	log.Debug().Str("backend", "ldap").Str("basedn", i.User.BaseDN).Str("filter", filter).Int("scope", i.User.scopeVal).Msg("LDAP Search")
	res, err := lc.Search(searchRequest)

	if err != nil {
		var errmsg string
		if lerr, ok := err.(*ldap.Error); ok {
			if lerr.ResultCode == ldap.LDAPResultSizeLimitExceeded {
				errmsg = fmt.Sprintf("too many results searching for user '%s'", filter)
				log.Debug().Str("backend", "ldap").Err(lerr).
					Str("userfilter", filter).Msg("too many results searching for user")
			}
		}
		return nil, errtypes.NotFound(errmsg)
	}
	if len(res.Entries) == 0 {
		return nil, errtypes.NotFound(filter)
	}

	return res.Entries[0], nil
}

// GetLDAPUserByDN looks up a single user by the supplied LDAP DN
// returns the corresponding ldap.Entry
func (i *Identity) GetLDAPUserByDN(log *zerolog.Logger, lc ldap.Client, dn string) (*ldap.Entry, error) {
	filter := fmt.Sprintf("(objectclass=%s)", i.User.Objectclass)
	if i.User.Filter != "" {
		filter = fmt.Sprintf("(&%s%s)", i.User.Filter, filter)
	}
	searchRequest := ldap.NewSearchRequest(
		dn, i.User.scopeVal, ldap.NeverDerefAliases, 1, 0, false,
		filter,
		[]string{
			i.User.Schema.DisplayName,
			i.User.Schema.ID,
			i.User.Schema.Mail,
			i.User.Schema.Username,
		},
		nil,
	)
	log.Debug().Str("backend", "ldap").Str("basedn", dn).Str("filter", filter).Int("scope", i.User.scopeVal).Msg("LDAP Search")
	res, err := lc.Search(searchRequest)

	if err != nil {
		return nil, errtypes.NotFound(dn)
	}
	if len(res.Entries) == 0 {
		return nil, errtypes.NotFound(dn)
	}

	return res.Entries[0], nil
}

// GetLDAPUsers searches for users using a prefix-substring match on the user
// attributes. Returns a slice of matching ldap.Entries
func (i *Identity) GetLDAPUsers(log *zerolog.Logger, lc ldap.Client, query string) ([]*ldap.Entry, error) {
	filter := i.getUserFindFilter(query)
	searchRequest := ldap.NewSearchRequest(
		i.User.BaseDN,
		i.User.scopeVal, ldap.NeverDerefAliases, 0, 0, false,
		filter,
		[]string{i.User.Schema.ID, i.User.Schema.Username, i.User.Schema.Mail, i.User.Schema.DisplayName, i.User.Schema.UIDNumber, i.User.Schema.GIDNumber},
		nil,
	)

	log.Debug().Str("backend", "ldap").Str("basedn", i.User.BaseDN).Str("filter", filter).Int("scope", i.User.scopeVal).Msg("LDAP Search")
	sr, err := lc.Search(searchRequest)
	if err != nil {
		return nil, err
	}
	return sr.Entries, nil
}

// GetLDAPUserGroups looks up the group member ship of the supplied LDAP user entry.
// Returns a slice of strings with groupids
func (i *Identity) GetLDAPUserGroups(log *zerolog.Logger, lc ldap.Client, userEntry *ldap.Entry) ([]string, error) {
	var memberValue string

	if strings.ToLower(i.Group.Objectclass) == "posixgroup" {
		// posixGroup usually means that the member attribute just contains the username
		memberValue = userEntry.GetEqualFoldAttributeValue(i.User.Schema.Username)
	} else {
		// In all other case we assume the member Attribute to contain full LDAP DNs
		memberValue = userEntry.DN
	}

	searchRequest := ldap.NewSearchRequest(
		i.Group.BaseDN, i.Group.scopeVal,
		ldap.NeverDerefAliases, 0, 0, false,
		i.getGroupMemberFilter(memberValue),
		[]string{i.Group.Schema.ID},
		nil,
	)

	sr, err := lc.Search(searchRequest)
	if err != nil {
		return []string{}, err
	}

	groups := make([]string, 0, len(sr.Entries))
	for _, entry := range sr.Entries {
		// FIXME this makes the users groups use the cn, not an immutable id
		// FIXME 1. use the memberof or members attribute of a user to get the groups
		// FIXME 2. ook up the id for each group
		groups = append(groups, entry.GetEqualFoldAttributeValue(i.Group.Schema.ID))
	}
	return groups, nil
}

// GetLDAPGroupByID looks up a group by the supplied Id. Returns the corresponding
// ldap.Entry
func (i *Identity) GetLDAPGroupByID(log *zerolog.Logger, lc ldap.Client, id string) (*ldap.Entry, error) {
	var filter string
	var err error
	if filter, err = i.getGroupFilter(id); err != nil {
		return nil, err
	}
	return i.GetLDAPGroupByFilter(log, lc, filter)
}

// GetLDAPGroupByAttribute looks up a single group by attribute (can be "mail", "gid_number",
// "display_name", "group_name", "group_id"). Returns the corresponding ldap.Entry
func (i *Identity) GetLDAPGroupByAttribute(log *zerolog.Logger, lc ldap.Client, attribute, value string) (*ldap.Entry, error) {
	var filter string
	var err error
	if filter, err = i.getGroupAttributeFilter(attribute, value); err != nil {
		return nil, err
	}
	return i.GetLDAPGroupByFilter(log, lc, filter)
}

// GetLDAPGroupByFilter looks up a single group by the supplied LDAP filter
// returns the corresponding ldap.Entry
func (i *Identity) GetLDAPGroupByFilter(log *zerolog.Logger, lc ldap.Client, filter string) (*ldap.Entry, error) {
	searchRequest := ldap.NewSearchRequest(
		i.Group.BaseDN, i.Group.scopeVal, ldap.NeverDerefAliases, 1, 0, false,
		filter,
		[]string{
			i.Group.Schema.DisplayName,
			i.Group.Schema.ID,
			i.Group.Schema.Mail,
			i.Group.Schema.Groupname,
			i.Group.Schema.Member,
		},
		nil,
	)
	log.Debug().Str("backend", "ldap").Msgf("Search %s", i.Group.BaseDN)
	res, err := lc.Search(searchRequest)

	if err != nil {
		var errmsg string
		if lerr, ok := err.(*ldap.Error); ok {
			if lerr.ResultCode == ldap.LDAPResultSizeLimitExceeded {
				errmsg = fmt.Sprintf("too many results searching for group '%s'", filter)
				log.Debug().Str("backend", "ldap").Err(lerr).
					Str("groupfilter", filter).Msg("too many results searching for group")
			}
		}
		return nil, errtypes.NotFound(errmsg)
	}
	if len(res.Entries) == 0 {
		return nil, errtypes.NotFound(filter)
	}

	return res.Entries[0], nil
}

// GetLDAPGroups searches for groups using a prefix-substring match on the group
// attributes. Returns a slice of matching ldap.Entries
func (i *Identity) GetLDAPGroups(log *zerolog.Logger, lc ldap.Client, query string) ([]*ldap.Entry, error) {
	searchRequest := ldap.NewSearchRequest(
		i.Group.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		i.getGroupFindFilter(query),
		[]string{i.Group.Schema.ID, i.Group.Schema.Groupname, i.Group.Schema.Mail, i.Group.Schema.DisplayName, i.Group.Schema.GIDNumber},
		nil,
	)

	sr, err := lc.Search(searchRequest)
	if err != nil {
		return nil, err
	}
	return sr.Entries, nil
}

// GetLDAPGroupMembers looks up all members of the supplied LDAP group entry and returns the
// corresponding LDAP user entries
func (i *Identity) GetLDAPGroupMembers(log *zerolog.Logger, lc ldap.Client, group *ldap.Entry) ([]*ldap.Entry, error) {
	members := group.GetEqualFoldAttributeValues(i.Group.Schema.Member)
	log.Debug().Str("dn", group.DN).Interface("member", members).Msg("Get Group members")
	memberEntries := make([]*ldap.Entry, 0, len(members))
	for _, member := range members {
		var e *ldap.Entry
		var err error
		if strings.ToLower(i.Group.Objectclass) == "posixgroup" {
			e, err = i.GetLDAPUserByAttribute(log, lc, "username", member)
		} else {
			e, err = i.GetLDAPUserByDN(log, lc, member)
		}
		if err != nil {
			log.Warn().Err(err).Interface("member", member).Msg("Failed read user entry for member")
			continue
		}
		memberEntries = append(memberEntries, e)
	}

	return memberEntries, nil
}

func filterEscapeBinaryUUID(value uuid.UUID) string {
	filtered := ""
	for _, b := range value {
		filtered = fmt.Sprintf("%s\\%02x", filtered, b)
	}
	return filtered
}

func (i *Identity) getUserFilter(uid string) (string, error) {
	var escapedUUID string
	if i.User.Schema.IDIsOctetString {
		id, err := uuid.Parse(uid)
		if err != nil {
			err := errors.Wrap(err, fmt.Sprintf("error parsing OpaqueID '%s' as UUID", uid))
			return "", err
		}
		escapedUUID = filterEscapeBinaryUUID(id)
	} else {
		escapedUUID = ldap.EscapeFilter(uid)
	}

	return fmt.Sprintf("(&%s(objectclass=%s)(%s=%s))",
		i.User.Filter,
		i.User.Objectclass,
		i.User.Schema.ID,
		escapedUUID,
	), nil
}

func (i *Identity) getUserAttributeFilter(attribute, value string) (string, error) {
	switch attribute {
	case "mail":
		attribute = i.User.Schema.Mail
	case "uid":
		attribute = i.User.Schema.UIDNumber
	case "gid":
		attribute = i.User.Schema.GIDNumber
	case "username":
		attribute = i.User.Schema.Username
	case "userid":
		attribute = i.User.Schema.ID
	default:
		return "", errors.New("ldap: invalid field " + attribute)
	}
	if attribute == "userid" && i.User.Schema.IDIsOctetString {
		id, err := uuid.Parse(value)
		if err != nil {
			err := errors.Wrap(err, fmt.Sprintf("error parsing OpaqueID '%s' as UUID", value))
			return "", err
		}
		value = filterEscapeBinaryUUID(id)
	} else {
		value = ldap.EscapeFilter(value)
	}
	return fmt.Sprintf("(&%s(objectclass=%s)(%s=%s))",
		i.User.Filter,
		i.User.Objectclass,
		attribute,
		value,
	), nil
}

// getUserFindFilter construct a LDAP filter to perform a prefix-substring
// search for users.
func (i *Identity) getUserFindFilter(query string) string {
	searchAttrs := []string{
		i.User.Schema.Mail,
		i.User.Schema.DisplayName,
		i.User.Schema.Username,
	}
	var filter string
	for _, attr := range searchAttrs {
		filter = fmt.Sprintf("%s(%s=%s*)", filter, attr, ldap.EscapeFilter(query))
	}
	// substring search for UUID is not possible
	filter = fmt.Sprintf("%s(%s=%s)", filter, i.User.Schema.ID, ldap.EscapeFilter(query))

	return fmt.Sprintf("(&%s(objectclass=%s)(|%s))",
		i.User.Filter,
		i.User.Objectclass,
		filter,
	)
}

// getGroupFindFilter construct a LDAP filter to perform a prefix-substring
// search for groups.
func (i *Identity) getGroupFindFilter(query string) string {
	searchAttrs := []string{
		i.Group.Schema.Mail,
		i.Group.Schema.DisplayName,
		i.Group.Schema.Groupname,
	}
	var filter string
	for _, attr := range searchAttrs {
		filter = fmt.Sprintf("%s(%s=%s*)", filter, attr, ldap.EscapeFilter(query))
	}
	// substring search for UUID is not possible
	filter = fmt.Sprintf("%s(%s=%s)", filter, i.Group.Schema.ID, ldap.EscapeFilter(query))

	return fmt.Sprintf("(&%s(objectclass=%s)(|%s))",
		i.Group.Filter,
		i.Group.Objectclass,
		filter,
	)
}

func stringToScope(scope string) (int, error) {
	var s int
	switch scope {
	case "sub":
		s = ldap.ScopeWholeSubtree
	case "one":
		s = ldap.ScopeSingleLevel
	case "base":
		s = ldap.ScopeBaseObject
	default:
		return 0, fmt.Errorf("invalid Scope '%s'", scope)
	}
	return s, nil
}

func (i *Identity) getGroupMemberFilter(memberName string) string {
	return fmt.Sprintf("(&%s(objectclass=%s)(%s=%s))",
		i.Group.Filter,
		i.Group.Objectclass,
		i.Group.Schema.Member,
		ldap.EscapeFilter(memberName),
	)
}

func (i *Identity) getGroupFilter(id string) (string, error) {
	var escapedUUID string
	if i.Group.Schema.IDIsOctetString {
		id, err := uuid.Parse(id)
		if err != nil {
			err := errors.Wrap(err, fmt.Sprintf("error parsing OpaqueID '%s' as UUID", id))
			return "", err
		}
		escapedUUID = filterEscapeBinaryUUID(id)
	} else {
		escapedUUID = ldap.EscapeFilter(id)
	}

	return fmt.Sprintf("(&%s(objectclass=%s)(%s=%s))",
		i.Group.Filter,
		i.Group.Objectclass,
		i.Group.Schema.ID,
		escapedUUID,
	), nil
}

func (i *Identity) getGroupAttributeFilter(attribute, value string) (string, error) {
	switch attribute {
	case "mail":
		attribute = i.Group.Schema.Mail
	case "gid_number":
		attribute = i.Group.Schema.GIDNumber
	case "display_name":
		attribute = i.Group.Schema.DisplayName
	case "group_name":
		attribute = i.Group.Schema.Groupname
	case "group_id":
		attribute = i.Group.Schema.ID
	default:
		return "", errors.New("ldap: invalid field " + attribute)
	}
	if attribute == "group_id" && i.Group.Schema.IDIsOctetString {
		id, err := uuid.Parse(value)
		if err != nil {
			err := errors.Wrap(err, fmt.Sprintf("error parsing OpaqueID '%s' as UUID", value))
			return "", err
		}
		value = filterEscapeBinaryUUID(id)
	} else {
		value = ldap.EscapeFilter(value)
	}
	return fmt.Sprintf("(&%s(objectclass=%s)(%s=%s))",
		i.Group.Filter,
		i.Group.Objectclass,
		attribute,
		value,
	), nil
}
