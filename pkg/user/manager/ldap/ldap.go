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
	"fmt"
	"strconv"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/user"
	"github.com/cs3org/reva/v2/pkg/user/manager/registry"
	"github.com/cs3org/reva/v2/pkg/utils"
	ldapIdentity "github.com/cs3org/reva/v2/pkg/utils/ldap"
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
	ldapClient ldap.Client
}

type config struct {
	utils.LDAPConn `mapstructure:",squash"`
	LDAPIdentity   ldapIdentity.Identity `mapstructure:",squash"`
	Idp            string                `mapstructure:"idp"`
	// Nobody specifies the fallback uid number for users that don't have a uidNumber set in LDAP
	Nobody int64 `mapstructure:"nobody"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := config{
		LDAPIdentity: ldapIdentity.New(),
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

	mgr.ldapClient, err = utils.GetLDAPClientWithReconnect(&mgr.c.LDAPConn)
	return mgr, err
}

// Configure initializes the configuration of the user manager from the supplied config map
func (m *manager) Configure(ml map[string]interface{}) error {
	c, err := parseConfig(ml)
	if err != nil {
		return err
	}
	if c.Nobody == 0 {
		c.Nobody = 99
	}

	if err = c.LDAPIdentity.Setup(); err != nil {
		return fmt.Errorf("error setting up Identity config: %w", err)
	}
	m.c = c
	return nil
}

// GetUser implements the user.Manager interface. Looks up a user by Id and return the user
func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {
	log := appctx.GetLogger(ctx)

	log.Debug().Interface("id", uid).Msg("GetUser")
	// If the Idp value in the uid does not match our config, we can't answer this request
	if uid.Idp != "" && uid.Idp != m.c.Idp {
		return nil, errtypes.NotFound("idp mismatch")
	}

	userEntry, err := m.c.LDAPIdentity.GetLDAPUserByID(log, m.ldapClient, uid.OpaqueId)
	if err != nil {
		return nil, err
	}

	log.Debug().Interface("entry", userEntry).Msg("entries")

	u, err := m.ldapEntryToUser(userEntry)
	if err != nil {
		return nil, err
	}

	groups, err := m.c.LDAPIdentity.GetLDAPUserGroups(log, m.ldapClient, userEntry)
	if err != nil {
		return nil, err
	}

	u.Groups = groups
	return u, nil
}

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId, skipFetchingGroups bool) (*userpb.User, error) {
	log := appctx.GetLogger(ctx)

	log.Debug().Str("claim", claim).Str("value", value).Msg("GetUserByClaim")
	userEntry, err := m.c.LDAPIdentity.GetLDAPUserByAttribute(log, m.ldapClient, claim, value)
	if err != nil {
		log.Debug().Err(err).Msg("GetUserByClaim")
		return nil, err
	}

	log.Debug().Interface("entry", userEntry).Msg("entries")

	u, err := m.ldapEntryToUser(userEntry)
	if err != nil {
		return nil, err
	}

	groups, err := m.c.LDAPIdentity.GetLDAPUserGroups(log, m.ldapClient, userEntry)
	if err != nil {
		return nil, err
	}

	u.Groups = groups

	id := &userpb.UserId{
		Idp:      m.c.Idp,
		OpaqueId: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.UID),
		Type:     userpb.UserType_USER_TYPE_PRIMARY,
	}

	groups := []string{}
	if !skipFetchingGroups {
		groups, err = m.GetUserGroups(ctx, id)
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

		groups, err := m.c.LDAPIdentity.GetLDAPUserGroups(log, m.ldapClient, entry)
		if err != nil {
			return nil, err
		}
		u.Groups = groups
		users = append(users, u)
	}

	return users, nil
}

func (m *manager) GetUserByClaim(ctx context.Context, claim, value string, skipFetchingGroups bool) (*userpb.User, error) {
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
	if uid.Idp != "" && uid.Idp != m.c.Idp {
		return nil, errtypes.NotFound("idp mismatch")
	}
	userEntry, err := m.c.LDAPIdentity.GetLDAPUserByID(log, m.ldapClient, uid.OpaqueId)
	if err != nil {
		return []string{}, err
	}
	return m.c.LDAPIdentity.GetLDAPUserGroups(log, m.ldapClient, userEntry)
}

	log.Debug().Interface("entries", sr.Entries).Msg("entries")

	id := &userpb.UserId{
		Idp:      m.c.Idp,
		OpaqueId: sr.Entries[0].GetEqualFoldAttributeValue(m.c.Schema.UID),
		Type:     userpb.UserType_USER_TYPE_PRIMARY,
	}

	groups := []string{}
	if !skipFetchingGroups {
		groups, err = m.GetUserGroups(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	gidNumber := m.c.Nobody
	gidValue := entry.GetEqualFoldAttributeValue(m.c.LDAPIdentity.User.Schema.GIDNumber)
	if gidValue != "" {
		gidNumber, err = strconv.ParseInt(gidValue, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	uidNumber := m.c.Nobody
	uidValue := entry.GetEqualFoldAttributeValue(m.c.LDAPIdentity.User.Schema.UIDNumber)
	if uidValue != "" {
		uidNumber, err = strconv.ParseInt(uidValue, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	u := &userpb.User{
		Id:          id,
		Username:    entry.GetEqualFoldAttributeValue(m.c.LDAPIdentity.User.Schema.Username),
		Mail:        entry.GetEqualFoldAttributeValue(m.c.LDAPIdentity.User.Schema.Mail),
		DisplayName: entry.GetEqualFoldAttributeValue(m.c.LDAPIdentity.User.Schema.DisplayName),
		GidNumber:   gidNumber,
		UidNumber:   uidNumber,
	}
	return u, nil
}

func (m *manager) FindUsers(ctx context.Context, query string, skipFetchingGroups bool) ([]*userpb.User, error) {
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
		id := &userpb.UserId{
			Idp:      m.c.Idp,
			OpaqueId: entry.GetEqualFoldAttributeValue(m.c.Schema.UID),
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
		}

		groups := []string{}
		if !skipFetchingGroups {
			groups, err = m.GetUserGroups(ctx, id)
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

	return &userpb.UserId{
		Idp:      m.c.Idp,
		OpaqueId: uid,
		Type:     userpb.UserType_USER_TYPE_PRIMARY,
	}, nil
}
