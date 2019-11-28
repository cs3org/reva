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

	types "github.com/cs3org/go-cs3apis/cs3/types"
	userproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/userprovider/v1beta1"
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
	filter       string
	bindUsername string
	bindPassword string
	schema       attributes
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
	Mail        string `mapstructure:"mail"`
	UID         string `mapstructure:"uid"`
	DisplayName string `mapstructure:"displayName"`
	DN          string `mapstructure:"dn"`
}

// Default attributes (Active Directory)
var ldapDefaults = attributes{
	Mail:        "mail",
	UID:         "objectGUID",
	DisplayName: "displayName",
	DN:          "dn",
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
		filter:       c.Filter,
		bindUsername: c.BindUsername,
		bindPassword: c.BindPassword,
		schema:       c.Schema,
	}, nil
}

func (m *manager) GetUser(ctx context.Context, uid *types.UserId) (*userproviderv1beta1pb.User, error) {
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
		fmt.Sprintf(m.filter, uid.OpaqueId), // TODO this is screaming for errors if filter contains >1 %s
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

	return &userproviderv1beta1pb.User{
		Username:    sr.Entries[0].GetAttributeValue(m.schema.UID),
		Groups:      []string{},
		Mail:        sr.Entries[0].GetAttributeValue(m.schema.Mail),
		DisplayName: sr.Entries[0].GetAttributeValue(m.schema.DisplayName),
	}, nil
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*userproviderv1beta1pb.User, error) {
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
		fmt.Sprintf(m.filter, query), // TODO this is screaming for errors if filter contains >1 %s
		[]string{m.schema.DN, m.schema.UID, m.schema.Mail, m.schema.DisplayName},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	users := []*userproviderv1beta1pb.User{}

	for _, entry := range sr.Entries {
		user := &userproviderv1beta1pb.User{
			Username:    entry.GetAttributeValue(m.schema.UID),
			Groups:      []string{},
			Mail:        sr.Entries[0].GetAttributeValue(m.schema.Mail),
			DisplayName: sr.Entries[0].GetAttributeValue(m.schema.DisplayName),
		}
		users = append(users, user)
	}

	return users, nil
}

func (m *manager) GetUserGroups(ctx context.Context, uid *types.UserId) ([]string, error) {
	return []string{}, nil // FIXME implement GetUserGroups for ldap user manager
}

func (m *manager) IsInGroup(ctx context.Context, uid *types.UserId, group string) (bool, error) {
	return false, nil // FIXME implement IsInGroup for ldap user manager
}
