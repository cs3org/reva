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

package sql

import (
	"context"
	"fmt"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	conversions "github.com/cs3org/reva/v3/pkg/cbox/utils"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
	"github.com/cs3org/reva/v3/pkg/ocm/invite/repository/registry"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This module implements the invite.Repository interface using gorm.
//
// The OCM Invitation tokens are saved in the table:
//     ocm_tokens(*token*, initiator, expiration, description)
//
// The OCM remote users are saved in the table:
//     ocm_remote_users(*initiator*, *opaque_user_id*, *idp*, email, display_name)

func init() {
	registry.Register("sql", New)
}

type mgr struct {
	c  *Config
	db *gorm.DB
}

type Config struct {
	config.Database `mapstructure:",squash"`
	GatewaySvc      string `mapstructure:"gatewaysvc"`
}

func (c *Config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
	c.Database = sharedconf.GetDBInfo(c.Database)
}

// OcmToken is the gorm model for the ocm_tokens table.
type OcmToken struct {
	gorm.Model
	Token       string `gorm:"size:255;uniqueIndex:i_token"`
	Initiator   string `gorm:"size:255;index:i_initiator"`
	Expiration  time.Time
	Description string `gorm:"size:255"`
}

// OcmRemoteUser is the gorm model for the ocm_remote_users table.
type OcmRemoteUser struct {
	gorm.Model
	Initiator    string `gorm:"size:255;index:i_initiator;uniqueIndex:i_unique"`
	OpaqueUserID string `gorm:"size:255;uniqueIndex:i_unique"`
	Idp          string `gorm:"size:255;uniqueIndex:i_unique"`
	Email        string `gorm:"size:255"`
	DisplayName  string `gorm:"size:255"`
}

// New creates a sql repository for ocm tokens and users.
func New(ctx context.Context, m map[string]any) (invite.Repository, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	c.ApplyDefaults()

	var db *gorm.DB
	var err error
	switch c.Engine {
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(c.DBName), &gorm.Config{})
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	default: // default is mysql
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	}
	if err != nil {
		return nil, errors.Wrap(err, "sql: error opening connection to database using engine "+c.Engine)
	}

	// Migrate schemas
	if err := db.AutoMigrate(&OcmToken{}, &OcmRemoteUser{}); err != nil {
		return nil, errors.Wrap(err, "sql: failed to migrate ocm invite schemas")
	}

	return &mgr{
		c:  &c,
		db: db,
	}, nil
}

func timestampToTime(t *types.Timestamp) time.Time {
	return time.Unix(int64(t.Seconds), int64(t.Nanos))
}

// AddToken stores the token in the repository.
func (m *mgr) AddToken(ctx context.Context, token *invitepb.InviteToken) error {
	t := &OcmToken{
		Token:       token.Token,
		Initiator:   conversions.FormatUserID(token.UserId),
		Expiration:  timestampToTime(token.Expiration),
		Description: token.Description,
	}
	return m.db.WithContext(ctx).Create(t).Error
}

func convertToInviteToken(t *OcmToken) *invitepb.InviteToken {
	return &invitepb.InviteToken{
		Token:  t.Token,
		UserId: conversions.MakeUserID(t.Initiator),
		Expiration: &types.Timestamp{
			Seconds: uint64(t.Expiration.Unix()),
		},
		Description: t.Description,
	}
}

// GetToken gets the token from the repository.
func (m *mgr) GetToken(ctx context.Context, token string) (*invitepb.InviteToken, error) {
	var t OcmToken
	res := m.db.WithContext(ctx).Where("token = ?", token).First(&t)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return nil, invite.ErrTokenNotFound
		}
		return nil, res.Error
	}
	return convertToInviteToken(&t), nil
}

// ListTokens gets the valid tokens from the repository (i.e. not expired).
func (m *mgr) ListTokens(ctx context.Context, initiator *userpb.UserId) ([]*invitepb.InviteToken, error) {
	var ts []OcmToken
	res := m.db.WithContext(ctx).
		Where("initiator = ?", conversions.FormatUserID(initiator)).
		Where("expiration > ?", time.Now()).
		Find(&ts)
	if res.Error != nil {
		return nil, res.Error
	}

	tokens := make([]*invitepb.InviteToken, 0, len(ts))
	for i := range ts {
		tokens = append(tokens, convertToInviteToken(&ts[i]))
	}
	return tokens, nil
}

// AddRemoteUser stores the remote user.
func (m *mgr) AddRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUser *userpb.User) error {
	u := &OcmRemoteUser{
		Initiator:    conversions.FormatUserID(initiator),
		OpaqueUserID: conversions.FormatUserID(remoteUser.Id),
		Idp:          remoteUser.Id.Idp,
		Email:        remoteUser.Mail,
		DisplayName:  remoteUser.DisplayName,
	}
	res := m.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(u)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return invite.ErrUserAlreadyAccepted
	}
	return nil
}

func (u *OcmRemoteUser) toCS3User() *userpb.User {
	return &userpb.User{
		Id: &userpb.UserId{
			Idp:      u.Idp,
			OpaqueId: u.OpaqueUserID,
			Type:     userpb.UserType_USER_TYPE_FEDERATED,
		},
		Mail:        u.Email,
		DisplayName: u.DisplayName,
	}
}

// GetRemoteUser retrieves details about a remote user who has accepted an invite to share.
func (m *mgr) GetRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUserID *userpb.UserId) (*userpb.User, error) {
	var u OcmRemoteUser
	res := m.db.WithContext(ctx).
		Where("initiator = ?", conversions.FormatUserID(initiator)).
		Where("opaque_user_id = ?", conversions.FormatUserID(remoteUserID)).
		Where("idp = ?", remoteUserID.Idp).
		First(&u)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return nil, errtypes.NotFound(remoteUserID.OpaqueId)
		}
		return nil, res.Error
	}
	return u.toCS3User(), nil
}

// FindRemoteUsers finds remote users who have accepted invites based on their attributes.
func (m *mgr) FindRemoteUsers(ctx context.Context, initiator *userpb.UserId, attr string) ([]*userpb.User, error) {
	// TODO: (gdelmont) this query can get really slow in case the number of rows is too high.
	// For the time being this is not expected, but if in future this happens, consider to add
	// a fulltext index.
	s := "%" + attr + "%"
	var us []OcmRemoteUser
	res := m.db.WithContext(ctx).
		Where("initiator = ?", conversions.FormatUserID(initiator)).
		Where("opaque_user_id LIKE ? OR idp LIKE ? OR email LIKE ? OR display_name LIKE ?", s, s, s, s).
		Find(&us)
	if res.Error != nil {
		return nil, res.Error
	}

	users := make([]*userpb.User, 0, len(us))
	for i := range us {
		users = append(users, us[i].toCS3User())
	}
	return users, nil
}

// DeleteRemoteUser removes the remote user from the initiator's list.
func (m *mgr) DeleteRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUser *userpb.UserId) error {
	return m.db.WithContext(ctx).
		Where("initiator = ?", conversions.FormatUserID(initiator)).
		Where("opaque_user_id = ?", conversions.FormatUserID(remoteUser)).
		Where("idp = ?", remoteUser.Idp).
		Delete(&OcmRemoteUser{}).Error
}
