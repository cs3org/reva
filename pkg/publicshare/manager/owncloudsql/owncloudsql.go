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
	"strconv"
	"time"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/publicshare"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/registry"
	"github.com/cs3org/reva/v2/pkg/sharedconf"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"

	// Provides mysql drivers
	_ "github.com/go-sql-driver/mysql"
)

const (
	publicShareType = 3
)

func init() {
	registry.Register("owncloudsql", NewMysql)
}

type Config struct {
	GatewayAddr                string `mapstructure:"gateway_addr"`
	DbUsername                 string `mapstructure:"db_username"`
	DbPassword                 string `mapstructure:"db_password"`
	DbHost                     string `mapstructure:"db_host"`
	DbPort                     int    `mapstructure:"db_port"`
	DbName                     string `mapstructure:"db_name"`
	EnableExpiredSharesCleanup bool   `mapstructure:"enable_expired_shares_cleanup"`
	SharePasswordHashCost      int    `mapstructure:"password_hash_cost"`
}

type mgr struct {
	driver        string
	db            *sql.DB
	c             Config
	userConverter UserConverter
}

// NewMysql returns a new publicshare manager connection to a mysql database
func NewMysql(m map[string]interface{}) (publicshare.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.DbUsername, c.DbPassword, c.DbHost, c.DbPort, c.DbName))
	if err != nil {
		return nil, err
	}

	userConverter := NewGatewayUserConverter(sharedconf.GetGatewaySVC(c.GatewayAddr))

	return New("mysql", db, *c, userConverter)
}

// New returns a new Cache instance connecting to the given sql.DB
func New(driver string, db *sql.DB, c Config, userConverter UserConverter) (publicshare.Manager, error) {
	return &mgr{
		driver:        driver,
		db:            db,
		c:             c,
		userConverter: userConverter,
	}, nil
}

func parseConfig(m map[string]interface{}) (*Config, error) {
	c := &Config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (m *mgr) CreatePublicShare(ctx context.Context, u *user.User, rInfo *provider.ResourceInfo, g *link.Grant) (*link.PublicShare, error) {

	tkn := utils.RandString(15)
	now := time.Now().Unix()

	displayName := tkn
	if rInfo.ArbitraryMetadata != nil && rInfo.ArbitraryMetadata.Metadata["name"] != "" {
		displayName = rInfo.ArbitraryMetadata.Metadata["name"]
	}
	createdAt := &typespb.Timestamp{
		Seconds: uint64(now),
	}

	creator := u.Username
	owner, err := m.userConverter.UserIDToUserName(ctx, rInfo.Owner)
	if err != nil {
		return nil, err
	}
	permissions := sharePermToInt(g.Permissions.Permissions)
	itemType := resourceTypeToItem(rInfo.Type)
	//prefix := rInfo.Id.SpaceId
	itemSource := rInfo.Id.OpaqueId
	fileSource, err := strconv.ParseUint(itemSource, 10, 64)
	if err != nil {
		// it can be the case that the item source may be a character string
		// we leave fileSource blank in that case
		fileSource = 0
	}

	/*
		query := "insert into oc_share set share_type=?,uid_owner=?,uid_initiator=?,item_type=?,fileid_prefix=?,item_source=?,file_source=?,permissions=?,stime=?,token=?,share_name=?"
		params := []interface{}{publicShareType, owner, creator, itemType, prefix, itemSource, fileSource, permissions, now, tkn, displayName}
	*/
	columns := "share_type,uid_owner,uid_initiator,item_type,item_source,file_source,permissions,stime,token,share_name"
	placeholders := "?,?,?,?,?,?,?,?,?,?"
	params := []interface{}{publicShareType, owner, creator, itemType, itemSource, fileSource, permissions, now, tkn, displayName}

	var passwordProtected bool
	password := g.Password
	if password != "" {
		password, err = hashPassword(password, m.c.SharePasswordHashCost)
		if err != nil {
			return nil, errors.Wrap(err, "could not hash share password")
		}
		passwordProtected = true

		columns += ",share_with"
		placeholders += ",?"
		params = append(params, password)
	}

	if g.Expiration != nil && g.Expiration.Seconds != 0 {
		t := time.Unix(int64(g.Expiration.Seconds), 0)
		columns += ",expiration"
		placeholders += ",?"
		params = append(params, t)
	}

	query := "INSERT INTO oc_share (" + columns + ") VALUES (" + placeholders + ")"
	stmt, err := m.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	result, err := stmt.Exec(params...)
	if err != nil {
		return nil, err
	}
	lastID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &link.PublicShare{
		Id: &link.PublicShareId{
			OpaqueId: strconv.FormatInt(lastID, 10),
		},
		Owner:             rInfo.GetOwner(),
		Creator:           u.Id,
		ResourceId:        rInfo.Id,
		Token:             tkn,
		Permissions:       g.Permissions,
		Ctime:             createdAt,
		Mtime:             createdAt,
		PasswordProtected: passwordProtected,
		Expiration:        g.Expiration,
		DisplayName:       displayName,
	}, nil
}

func hashPassword(password string, cost int) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	return "1|" + string(bytes), err
}

// UpdatePublicShare updates the expiration date, permissions and Mtime
func (m *mgr) UpdatePublicShare(ctx context.Context, u *user.User, req *link.UpdatePublicShareRequest) (*link.PublicShare, error) {
	return nil, errtypes.NotSupported("not implemented")
}

func (m *mgr) GetPublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference, sign bool) (share *link.PublicShare, err error) {
	return nil, errtypes.NotSupported("not implemented")
}

func (m *mgr) ListPublicShares(ctx context.Context, u *user.User, filters []*link.ListPublicSharesRequest_Filter, sign bool) ([]*link.PublicShare, error) {
	uid := u.Username
	query := `SELECT
				coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, 
				coalesce(share_with, '') as share_with, coalesce(file_source, '') as file_source,
				coalesce(item_type, '') as item_type, coalesce(token,'') as token,
				coalesce(expiration, '') as expiration, coalesce(share_name, '') as share_name,
				s.id, s.stime, s.permissions, fc.storage as storage
			FROM oc_share s
			LEFT JOIN oc_filecache fc ON fc.fileid = file_source
			WHERE (uid_owner=? or uid_initiator=?)
			AND (share_type=?)`
	var resourceFilters, ownerFilters, creatorFilters string
	var resourceParams, ownerParams, creatorParams []interface{}
	params := []interface{}{uid, uid, publicShareType}

	for _, f := range filters {
		switch f.Type {
		case link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID:
			if len(resourceFilters) != 0 {
				resourceFilters += " OR "
			}
			resourceFilters += "item_source=?"
			resourceParams = append(resourceParams, f.GetResourceId().OpaqueId)
		case link.ListPublicSharesRequest_Filter_TYPE_OWNER:
			if len(ownerFilters) != 0 {
				ownerFilters += " OR "
			}
			ownerFilters += "(uid_owner=?)"
			ownerParams = append(ownerParams, formatUserID(f.GetOwner()))
		case link.ListPublicSharesRequest_Filter_TYPE_CREATOR:
			if len(creatorFilters) != 0 {
				creatorFilters += " OR "
			}
			creatorFilters += "(uid_initiator=?)"
			creatorParams = append(creatorParams, formatUserID(f.GetCreator()))
		}
	}
	if resourceFilters != "" {
		query = fmt.Sprintf("%s AND (%s)", query, resourceFilters)
		params = append(params, resourceParams...)
	}
	if ownerFilters != "" {
		query = fmt.Sprintf("%s AND (%s)", query, ownerFilters)
		params = append(params, ownerParams...)
	}
	if creatorFilters != "" {
		query = fmt.Sprintf("%s AND (%s)", query, creatorFilters)
		params = append(params, creatorParams...)
	}

	rows, err := m.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var s DBShare
	shares := []*link.PublicShare{}
	for rows.Next() {
		if err := rows.Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.FileSource, &s.ItemType, &s.Token, &s.Expiration, &s.ShareName, &s.ID, &s.STime, &s.Permissions, &s.ItemStorage); err != nil {
			continue
		}
		var cs3Share *link.PublicShare
		if cs3Share, err = m.ConvertToCS3PublicShare(ctx, s); err != nil {
			return nil, err
		}
		if expired(cs3Share) {
			_ = m.cleanupExpiredShares()
		} else {
			if cs3Share.PasswordProtected && sign {
				if err := publicshare.AddSignature(cs3Share, s.ShareWith); err != nil {
					return nil, err
				}
			}
			shares = append(shares, cs3Share)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return shares, nil
}

func (m *mgr) RevokePublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference) error {
	return errtypes.NotSupported("not implemented")
}

func (m *mgr) GetPublicShareByToken(ctx context.Context, token string, auth *link.PublicShareAuthentication, sign bool) (*link.PublicShare, error) {
	return nil, errtypes.NotSupported("not implemented")
}

func expired(s *link.PublicShare) bool {
	if s.Expiration != nil {
		if t := time.Unix(int64(s.Expiration.GetSeconds()), int64(s.Expiration.GetNanos())); t.Before(time.Now()) {
			return true
		}
	}
	return false
}

func (m *mgr) cleanupExpiredShares() error {
	/*
		if !m.c.EnableExpiredSharesCleanup {
			return nil
		}

		query := "update oc_share set orphan = 1 where expiration IS NOT NULL AND expiration < ?"
		params := []interface{}{time.Now().Format("2006-01-02 03:04:05")}

		stmt, err := m.db.Prepare(query)
		if err != nil {
			return err
		}
		if _, err = stmt.Exec(params...); err != nil {
			return err
		}
	*/
	return nil
}
