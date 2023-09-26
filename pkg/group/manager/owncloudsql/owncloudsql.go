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

package owncloudsql

import (
	"context"
	"database/sql"
	"fmt"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/group"
	"github.com/cs3org/reva/v2/pkg/group/manager/owncloudsql/groups"
	"github.com/cs3org/reva/v2/pkg/group/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	// Provides mysql drivers
	_ "github.com/go-sql-driver/mysql"
)

func init() {
	registry.Register("owncloudsql", NewMysql)
}

type manager struct {
	c  *config
	db *groups.Groups
}

type config struct {
	DbUsername         string `mapstructure:"dbusername"`
	DbPassword         string `mapstructure:"dbpassword"`
	DbHost             string `mapstructure:"dbhost"`
	DbPort             int    `mapstructure:"dbport"`
	DbName             string `mapstructure:"dbname"`
	Idp                string `mapstructure:"idp"`
	Nobody             int64  `mapstructure:"nobody"`
	JoinOwnCloudUUID   bool   `mapstructure:"join_ownclouduuid"`
	EnableMedialSearch bool   `mapstructure:"enable_medial_search"`
}

// NewMysql returns a group manager implementation that connects to an owncloud mysql database
func NewMysql(m map[string]interface{}) (group.Manager, error) {
	mgr := &manager{}
	err := mgr.Configure(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	mgr.db, err = groups.NewMysql(
		fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", mgr.c.DbUsername, mgr.c.DbPassword, mgr.c.DbHost, mgr.c.DbPort, mgr.c.DbName),
		mgr.c.JoinUsername,
		mgr.c.JoinOwnCloudUUID,
		mgr.c.EnableMedialSearch,
	)
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

	if c.Nobody == 0 {
		c.Nobody = 99
	}

	m.c = c
	return nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, &c); err != nil {
		return nil, err
	}
	return c, nil
}

func (m *manager) GetGroup(ctx context.Context, gid *grouppb.GroupId, skipFetchingMembers bool) (*grouppb.Group, error) {
	// search via the group_id
	g, err := m.db.GetGroupByClaim(ctx, "group_id", gid.OpaqueId)
	if err == sql.ErrNoRows {
		return nil, errtypes.NotFound(gid.OpaqueId)
	}
	return m.convertToCS3Group(ctx, g, skipFetchingMembers)
}

func (m *manager) GetGroupByClaim(ctx context.Context, claim, value string, skipFetchingMembers bool) (*grouppb.Group, error) {
	g, err := m.db.GetGroupByClaim(ctx, claim, value)
	if err == sql.ErrNoRows {
		return nil, errtypes.NotFound(claim + "=" + value)
	} else if err != nil {
		return nil, err
	}
	return m.convertToCS3Group(ctx, g, skipFetchingMembers)
}

func (m *manager) FindGroups(ctx context.Context, query string, skipFetchingMembers bool) ([]*grouppb.Group, error) {
	ocgroups, err := m.db.FindGroups(ctx, query)
	if err == sql.ErrNoRows {
		return nil, errtypes.NotFound("no groups found for " + query)
	} else if err != nil {
		return nil, err
	}

	groups := make([]*grouppb.Group, 0, len(ocgroups))
	for i := range ocgroups {
		g, err := m.convertToCS3Group(ctx, &ocgroups[i], skipFetchingMembers)
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("group", ocgroups[i]).Msg("could not convert group, skipping")
			continue
		}
		groups = append(groups, g)
	}

	return groups, nil
}

func (m *manager) GetMembers(ctx context.Context, gid *grouppb.GroupId) ([]*userpb.UserId, error) {
	members, err := m.db.GetMembers(ctx, gid.OpaqueId)
	if err == sql.ErrNoRows {
		return nil, errtypes.NotFound("no members found for gid " + gid.OpaqueId)
	} else if err != nil {
		return nil, err
	}

	users := make([]*userpb.UserId, 0, len(members))
	for i := range members {
		u := m.convertToCS3UserId(ctx, members[i])
		users = append(users, u)
	}
	return users, nil
}

func (m *manager) HasMember(ctx context.Context, gid *grouppb.GroupId, uid *userpb.UserId) (bool, error) {
	if gid == nil || uid == nil || gid.Idp != m.c.Idp || uid.Idp != m.c.Idp {
		return false, nil
	}
	hasMember, err := m.db.HasMember(ctx, gid.OpaqueId, uid.OpaqueId)
	if err == sql.ErrNoRows {
		return false, errtypes.NotFound("no count for has member query gid:" + gid.OpaqueId + " uid:" + uid.OpaqueId)
	} else if err != nil {
		return false, err
	}
	return hasMember, nil
}

func (m *manager) convertToCS3Group(ctx context.Context, g *groups.Group, skipFetchingMembers bool) (*grouppb.Group, error) {
	group := &grouppb.Group{
		Id: &grouppb.GroupId{
			Idp:      m.c.Idp,
			OpaqueId: g.GID,
		},
		GroupName:   g.GID,
		DisplayName: g.GID,
	}
	if !skipFetchingMembers {
		var err error
		if group.Members, err = m.GetMembers(ctx, group.Id); err != nil {
			return nil, err
		}
	}
	return group, nil
}

func (m *manager) convertToCS3UserId(ctx context.Context, userID string) *userpb.UserId {
	return &userpb.UserId{
		Idp:      m.c.Idp,
		OpaqueId: userID,
	}
}
