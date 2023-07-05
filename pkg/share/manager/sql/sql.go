// Copyright 2018-2023 CERN
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
	"database/sql"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"

	// Provides mysql drivers.
	_ "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"
)

const (
	shareTypeUser  = 0
	shareTypeGroup = 1
)

func init() {
	registry.Register("oc10-sql", NewMysql)
}

type config struct {
	GatewayAddr    string `mapstructure:"gatewaysvc"`
	StorageMountID string `mapstructure:"storage_mount_id"`
	DBUsername     string `mapstructure:"db_username"`
	DBPassword     string `mapstructure:"db_password"`
	DBHost         string `mapstructure:"db_host"`
	DBPort         int    `mapstructure:"db_port"`
	DBName         string `mapstructure:"db_name"`
}

func (c *config) ApplyDefaults() {
	c.GatewayAddr = sharedconf.GetGatewaySVC(c.GatewayAddr)
}

type mgr struct {
	driver         string
	db             *sql.DB
	storageMountID string
	userConverter  UserConverter
}

// NewMysql returns a new share manager connection to a mysql database.
func NewMysql(ctx context.Context, m map[string]interface{}) (share.Manager, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName))
	if err != nil {
		return nil, err
	}

	userConverter := NewGatewayUserConverter(c.GatewayAddr)

	return New("mysql", db, c.StorageMountID, userConverter)
}

// New returns a new Cache instance connecting to the given sql.DB.
func New(driver string, db *sql.DB, storageMountID string, userConverter UserConverter) (share.Manager, error) {
	return &mgr{
		driver:         driver,
		db:             db,
		storageMountID: storageMountID,
		userConverter:  userConverter,
	}, nil
}

func (m *mgr) Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error) {
	user := ctxpkg.ContextMustGetUser(ctx)

	// do not allow share to myself or the owner if share is for a user
	// TODO(labkode): should not this be caught already at the gw level?
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER &&
		(utils.UserEqual(g.Grantee.GetUserId(), user.Id) || utils.UserEqual(g.Grantee.GetUserId(), md.Owner)) {
		return nil, errors.New("sql: owner/creator and grantee are the same")
	}

	// check if share already exists.
	key := &collaboration.ShareKey{
		Owner:      md.Owner,
		ResourceId: md.Id,
		Grantee:    g.Grantee,
	}
	_, err := m.getByKey(ctx, key)

	// share already exists
	if err == nil {
		return nil, errtypes.AlreadyExists(key.String())
	}

	now := time.Now().Unix()
	ts := &typespb.Timestamp{
		Seconds: uint64(now),
	}

	owner, err := m.userConverter.UserIDToUserName(ctx, md.Owner)
	if err != nil {
		return nil, err
	}
	shareType, shareWith, err := m.formatGrantee(ctx, g.Grantee)
	if err != nil {
		return nil, err
	}
	itemType := resourceTypeToItem(md.Type)
	targetPath := path.Join("/", path.Base(md.Path))
	permissions := sharePermToInt(g.Permissions.Permissions)
	itemSource := md.Id.OpaqueId
	fileSource, err := strconv.ParseUint(itemSource, 10, 64)
	if err != nil {
		// it can be the case that the item source may be a character string
		// we leave fileSource blank in that case
		fileSource = 0
	}

	stmtString := "INSERT INTO oc_share (share_type,uid_owner,uid_initiator,item_type,item_source,file_source,permissions,stime,share_with,file_target) VALUES (?,?,?,?,?,?,?,?,?,?)"
	stmtValues := []interface{}{shareType, owner, user.Username, itemType, itemSource, fileSource, permissions, now, shareWith, targetPath}

	stmt, err := m.db.Prepare(stmtString)
	if err != nil {
		return nil, err
	}
	result, err := stmt.Exec(stmtValues...)
	if err != nil {
		return nil, err
	}
	lastID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &collaboration.Share{
		Id: &collaboration.ShareId{
			OpaqueId: strconv.FormatInt(lastID, 10),
		},
		ResourceId:  md.Id,
		Permissions: g.Permissions,
		Grantee:     g.Grantee,
		Owner:       md.Owner,
		Creator:     user.Id,
		Ctime:       ts,
		Mtime:       ts,
	}, nil
}

func (m *mgr) GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error) {
	var s *collaboration.Share
	var err error
	switch {
	case ref.GetId() != nil:
		s, err = m.getByID(ctx, ref.GetId())
	case ref.GetKey() != nil:
		s, err = m.getByKey(ctx, ref.GetKey())
	default:
		err = errtypes.NotFound(ref.String())
	}

	if err != nil {
		return nil, err
	}

	return s, nil
}

func (m *mgr) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
	uid := ctxpkg.ContextMustGetUser(ctx).Username
	var query string
	params := []interface{}{}
	switch {
	case ref.GetId() != nil:
		query = "DELETE FROM oc_share where id=? AND (uid_owner=? or uid_initiator=?)"
		params = append(params, ref.GetId().OpaqueId, uid, uid)
	case ref.GetKey() != nil:
		key := ref.GetKey()
		shareType, shareWith, err := m.formatGrantee(ctx, key.Grantee)
		if err != nil {
			return err
		}
		owner := formatUserID(key.Owner)
		query = "DELETE FROM oc_share WHERE uid_owner=? AND item_source=? AND share_type=? AND share_with=? AND (uid_owner=? or uid_initiator=?)"
		params = append(params, owner, key.ResourceId.StorageId, shareType, shareWith, uid, uid)
	default:
		return errtypes.NotFound(ref.String())
	}

	stmt, err := m.db.Prepare(query)
	if err != nil {
		return err
	}
	res, err := stmt.Exec(params...)
	if err != nil {
		return err
	}

	rowCnt, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowCnt == 0 {
		return errtypes.NotFound(ref.String())
	}
	return nil
}

func (m *mgr) UpdateShare(ctx context.Context, ref *collaboration.ShareReference, p *collaboration.SharePermissions) (*collaboration.Share, error) {
	permissions := sharePermToInt(p.Permissions)
	uid := ctxpkg.ContextMustGetUser(ctx).Username

	var query string
	params := []interface{}{}
	switch {
	case ref.GetId() != nil:
		query = "update oc_share set permissions=?,stime=? where id=? AND (uid_owner=? or uid_initiator=?)"
		params = append(params, permissions, time.Now().Unix(), ref.GetId().OpaqueId, uid, uid)
	case ref.GetKey() != nil:
		key := ref.GetKey()
		shareType, shareWith, err := m.formatGrantee(ctx, key.Grantee)
		if err != nil {
			return nil, err
		}
		owner := formatUserID(key.Owner)
		query = "update oc_share set permissions=?,stime=? where (uid_owner=? or uid_initiator=?) AND item_source=? AND share_type=? AND share_with=? AND (uid_owner=? or uid_initiator=?)"
		params = append(params, permissions, time.Now().Unix(), owner, owner, key.ResourceId.StorageId, shareType, shareWith, uid, uid)
	default:
		return nil, errtypes.NotFound(ref.String())
	}

	stmt, err := m.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	if _, err = stmt.Exec(params...); err != nil {
		return nil, err
	}

	return m.GetShare(ctx, ref)
}

func (m *mgr) ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error) {
	uid := ctxpkg.ContextMustGetUser(ctx).Username
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(item_source, '') as item_source, id, stime, permissions, share_type FROM oc_share WHERE (uid_owner=? or uid_initiator=?)"
	params := []interface{}{uid, uid}

	var (
		filterQuery  string
		filterParams []interface{}
		err          error
	)
	if len(filters) == 0 {
		filterQuery += "(share_type=? OR share_type=?)"
		params = append(params, shareTypeUser)
		params = append(params, shareTypeGroup)
	} else {
		filterQuery, filterParams, err = translateFilters(filters)
		if err != nil {
			return nil, err
		}
		params = append(params, filterParams...)
	}

	if filterQuery != "" {
		query = fmt.Sprintf("%s AND (%s)", query, filterQuery)
	}

	rows, err := m.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var s DBShare
	shares := []*collaboration.Share{}
	for rows.Next() {
		if err := rows.Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.ItemSource, &s.ID, &s.STime, &s.Permissions, &s.ShareType); err != nil {
			continue
		}
		share, err := m.convertToCS3Share(ctx, s, m.storageMountID)
		if err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return shares, nil
}

// we list the shares that are targeted to the user in context or to the user groups.
func (m *mgr) ListReceivedShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.ReceivedShare, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	uid := user.Username

	params := []interface{}{uid, uid, uid}
	for _, v := range user.Groups {
		params = append(params, v)
	}

	homeConcat := ""
	if m.driver == "mysql" { // mysql upsert
		homeConcat = "storages.id = CONCAT('home::', ts.uid_owner)"
	} else { // sqlite3 upsert
		homeConcat = "storages.id = 'home::' || ts.uid_owner"
	}
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(item_source, '') as item_source, ts.id, stime, permissions, share_type, accepted, storages.numeric_id FROM oc_share ts LEFT JOIN oc_storages storages ON " + homeConcat + " WHERE (uid_owner != ? AND uid_initiator != ?) "
	if len(user.Groups) > 0 {
		query += "AND (share_with=? OR share_with in (?" + strings.Repeat(",?", len(user.Groups)-1) + "))"
	} else {
		query += "AND (share_with=?)"
	}

	filterQuery, filterParams, err := translateFilters(filters)
	if err != nil {
		return nil, err
	}
	params = append(params, filterParams...)

	if filterQuery != "" {
		query = fmt.Sprintf("%s AND (%s)", query, filterQuery)
	}

	rows, err := m.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var s DBShare
	shares := []*collaboration.ReceivedShare{}
	for rows.Next() {
		if err := rows.Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.ItemSource, &s.ID, &s.STime, &s.Permissions, &s.ShareType, &s.State, &s.ItemStorage); err != nil {
			continue
		}
		share, err := m.convertToCS3ReceivedShare(ctx, s, m.storageMountID)
		if err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return shares, nil
}

func (m *mgr) GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	var s *collaboration.ReceivedShare
	var err error
	switch {
	case ref.GetId() != nil:
		s, err = m.getReceivedByID(ctx, ref.GetId())
	case ref.GetKey() != nil:
		s, err = m.getReceivedByKey(ctx, ref.GetKey())
	default:
		err = errtypes.NotFound(ref.String())
	}

	if err != nil {
		return nil, err
	}

	return s, nil
}

func (m *mgr) UpdateReceivedShare(ctx context.Context, share *collaboration.ReceivedShare, fieldMask *field_mask.FieldMask) (*collaboration.ReceivedShare, error) {
	rs, err := m.GetReceivedShare(ctx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: share.Share.Id}})
	if err != nil {
		return nil, err
	}

	for i := range fieldMask.Paths {
		switch fieldMask.Paths[i] {
		case "state":
			rs.State = share.State
		// TODO case "mount_point":
		default:
			return nil, errtypes.NotSupported("updating " + fieldMask.Paths[i] + " is not supported")
		}
	}

	var queryAccept string
	switch rs.GetState() {
	case collaboration.ShareState_SHARE_STATE_REJECTED:
		queryAccept = "update oc_share set accepted=2 where id=?"
	case collaboration.ShareState_SHARE_STATE_ACCEPTED:
		queryAccept = "update oc_share set accepted=0 where id=?"
	}

	if queryAccept != "" {
		stmt, err := m.db.Prepare(queryAccept)
		if err != nil {
			return nil, err
		}
		_, err = stmt.Exec(rs.Share.Id.OpaqueId)
		if err != nil {
			return nil, err
		}
	}

	return rs, nil
}

func (m *mgr) getByID(ctx context.Context, id *collaboration.ShareId) (*collaboration.Share, error) {
	uid := ctxpkg.ContextMustGetUser(ctx).Username
	s := DBShare{ID: id.OpaqueId}
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(item_source, '') as item_source, stime, permissions, share_type FROM oc_share WHERE id=? AND (uid_owner=? or uid_initiator=?)"
	if err := m.db.QueryRow(query, id.OpaqueId, uid, uid).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.ItemSource, &s.STime, &s.Permissions, &s.ShareType); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(id.OpaqueId)
		}
		return nil, err
	}
	return m.convertToCS3Share(ctx, s, m.storageMountID)
}

func (m *mgr) getByKey(ctx context.Context, key *collaboration.ShareKey) (*collaboration.Share, error) {
	owner, err := m.userConverter.UserIDToUserName(ctx, key.Owner)
	if err != nil {
		return nil, err
	}
	uid := ctxpkg.ContextMustGetUser(ctx).Username

	s := DBShare{}
	shareType, shareWith, err := m.formatGrantee(ctx, key.Grantee)
	if err != nil {
		return nil, err
	}
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(item_source, '') as item_source, id, stime, permissions, share_type FROM oc_share WHERE uid_owner=? AND item_source=? AND share_type=? AND share_with=? AND (uid_owner=? or uid_initiator=?)"
	if err = m.db.QueryRow(query, owner, key.ResourceId.StorageId, shareType, shareWith, uid, uid).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.ItemSource, &s.ID, &s.STime, &s.Permissions, &s.ShareType); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(key.String())
		}
		return nil, err
	}
	return m.convertToCS3Share(ctx, s, m.storageMountID)
}

func (m *mgr) getReceivedByID(ctx context.Context, id *collaboration.ShareId) (*collaboration.ReceivedShare, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	uid := user.Username

	params := []interface{}{id.OpaqueId, uid}
	for _, v := range user.Groups {
		params = append(params, v)
	}

	s := DBShare{ID: id.OpaqueId}
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(item_source, '') as item_source, stime, permissions, share_type, accepted FROM oc_share ts WHERE ts.id=? "
	if len(user.Groups) > 0 {
		query += "AND (share_with=? OR share_with in (?" + strings.Repeat(",?", len(user.Groups)-1) + "))"
	} else {
		query += "AND (share_with=?)"
	}
	if err := m.db.QueryRow(query, params...).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.ItemSource, &s.STime, &s.Permissions, &s.ShareType, &s.State); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(id.OpaqueId)
		}
		return nil, err
	}
	return m.convertToCS3ReceivedShare(ctx, s, m.storageMountID)
}

func (m *mgr) getReceivedByKey(ctx context.Context, key *collaboration.ShareKey) (*collaboration.ReceivedShare, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	uid := user.Username

	shareType, shareWith, err := m.formatGrantee(ctx, key.Grantee)
	if err != nil {
		return nil, err
	}
	params := []interface{}{uid, formatUserID(key.Owner), key.ResourceId.StorageId, key.ResourceId.OpaqueId, shareType, shareWith, shareWith}
	for _, v := range user.Groups {
		params = append(params, v)
	}

	s := DBShare{}
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(item_source, '') as item_source, ts.id, stime, permissions, share_type, accepted FROM oc_share ts WHERE uid_owner=? AND item_source=? AND share_type=? AND share_with=? "
	if len(user.Groups) > 0 {
		query += "AND (share_with=? OR share_with in (?" + strings.Repeat(",?", len(user.Groups)-1) + "))"
	} else {
		query += "AND (share_with=?)"
	}

	if err := m.db.QueryRow(query, params...).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.ItemSource, &s.ID, &s.STime, &s.Permissions, &s.ShareType, &s.State); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(key.String())
		}
		return nil, err
	}
	return m.convertToCS3ReceivedShare(ctx, s, m.storageMountID)
}

func granteeTypeToShareType(granteeType provider.GranteeType) int {
	switch granteeType {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		return shareTypeUser
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		return shareTypeGroup
	}
	return -1
}

// translateFilters translates the filters to sql queries.
func translateFilters(filters []*collaboration.Filter) (string, []interface{}, error) {
	var (
		filterQuery string
		params      []interface{}
	)

	groupedFilters := share.GroupFiltersByType(filters)
	// If multiple filters of the same type are passed to this function, they need to be combined with the `OR` operator.
	// That is why the filters got grouped by type.
	// For every given filter type, iterate over the filters and if there are more than one combine them.
	// Combine the different filter types using `AND`
	var filterCounter = 0
	for filterType, filters := range groupedFilters {
		switch filterType {
		case collaboration.Filter_TYPE_RESOURCE_ID:
			filterQuery += "("
			for i, f := range filters {
				filterQuery += "item_source=?"
				params = append(params, f.GetResourceId().OpaqueId)

				if i != len(filters)-1 {
					filterQuery += " OR "
				}
			}
			filterQuery += ")"
		case collaboration.Filter_TYPE_GRANTEE_TYPE:
			filterQuery += "("
			for i, f := range filters {
				filterQuery += "share_type=?"
				params = append(params, granteeTypeToShareType(f.GetGranteeType()))

				if i != len(filters)-1 {
					filterQuery += " OR "
				}
			}
			filterQuery += ")"
		case collaboration.Filter_TYPE_EXCLUDE_DENIALS:
			// TODO this may change once the mapping of permission to share types is completed (cf. pkg/cbox/utils/conversions.go)
			filterQuery += "permissions > 0"
		default:
			return "", nil, fmt.Errorf("filter type is not supported")
		}
		if filterCounter != len(groupedFilters)-1 {
			filterQuery += " AND "
		}
		filterCounter++
	}
	return filterQuery, params, nil
}
