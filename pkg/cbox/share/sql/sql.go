// Copyright 2018-2022 CERN
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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	conversions "github.com/cs3org/reva/pkg/cbox/utils"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils"

	// Provides mysql drivers.
	_ "github.com/go-sql-driver/mysql"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"
)

const (
	shareTypeUser  = 0
	shareTypeGroup = 1

	projectInstancesPrefix        = "newproject"
	projectSpaceGroupsPrefix      = "cernbox-project-"
	projectSpaceAdminGroupsSuffix = "-admins"
)

func init() {
	registry.Register("sql", New)
}

type config struct {
	DBUsername string `mapstructure:"db_username"`
	DBPassword string `mapstructure:"db_password"`
	DBHost     string `mapstructure:"db_host"`
	DBPort     int    `mapstructure:"db_port"`
	DBName     string `mapstructure:"db_name"`
	GatewaySvc string `mapstructure:"gatewaysvc"`
}

type mgr struct {
	c  *config
	db *sql.DB
}

// New returns a new share manager.
func New(m map[string]interface{}) (share.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName))
	if err != nil {
		return nil, err
	}

	return &mgr{
		c:  c,
		db: db,
	}, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
	return c, nil
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

	shareType, shareWith := conversions.FormatGrantee(g.Grantee)
	itemType := conversions.ResourceTypeToItem(md.Type)
	targetPath := path.Join("/", path.Base(md.Path))
	permissions := conversions.SharePermToInt(g.Permissions.Permissions)
	prefix := md.Id.StorageId
	itemSource := md.Id.OpaqueId
	fileSource, err := strconv.ParseUint(itemSource, 10, 64)
	if err != nil {
		// it can be the case that the item source may be a character string
		// we leave fileSource blank in that case
		fileSource = 0
	}

	stmtString := "insert into oc_share set share_type=?,uid_owner=?,uid_initiator=?,item_type=?,fileid_prefix=?,item_source=?,file_source=?,permissions=?,stime=?,share_with=?,file_target=?"
	stmtValues := []interface{}{shareType, conversions.FormatUserID(md.Owner), conversions.FormatUserID(user.Id), itemType, prefix, itemSource, fileSource, permissions, now, shareWith, targetPath}

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

func (m *mgr) getByID(ctx context.Context, id *collaboration.ShareId) (*collaboration.Share, error) {
	uid := conversions.FormatUserID(ctxpkg.ContextMustGetUser(ctx).Id)
	s := conversions.DBShare{ID: id.OpaqueId}
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, coalesce(item_type, '') as item_type, stime, permissions, share_type FROM oc_share WHERE (orphan = 0 or orphan IS NULL) AND id=? AND (uid_owner=? or uid_initiator=?)"
	if err := m.db.QueryRow(query, id.OpaqueId, uid, uid).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.ItemType, &s.STime, &s.Permissions, &s.ShareType); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(id.OpaqueId)
		}
		return nil, err
	}
	return conversions.ConvertToCS3Share(s), nil
}

func (m *mgr) getByKey(ctx context.Context, key *collaboration.ShareKey) (*collaboration.Share, error) {
	owner := conversions.FormatUserID(key.Owner)
	uid := conversions.FormatUserID(ctxpkg.ContextMustGetUser(ctx).Id)

	s := conversions.DBShare{}
	shareType, shareWith := conversions.FormatGrantee(key.Grantee)
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, coalesce(item_type, '') as item_type, id, stime, permissions, share_type FROM oc_share WHERE (orphan = 0 or orphan IS NULL) AND uid_owner=? AND fileid_prefix=? AND item_source=? AND share_type=? AND share_with=? AND (uid_owner=? or uid_initiator=?)"
	if err := m.db.QueryRow(query, owner, key.ResourceId.StorageId, key.ResourceId.OpaqueId, shareType, shareWith, uid, uid).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.ItemType, &s.ID, &s.STime, &s.Permissions, &s.ShareType); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(key.String())
		}
		return nil, err
	}
	return conversions.ConvertToCS3Share(s), nil
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
	uid := conversions.FormatUserID(ctxpkg.ContextMustGetUser(ctx).Id)
	var query string
	params := []interface{}{}
	switch {
	case ref.GetId() != nil:
		query = "delete from oc_share where id=? AND (uid_owner=? or uid_initiator=?)"
		params = append(params, ref.GetId().OpaqueId, uid, uid)
	case ref.GetKey() != nil:
		key := ref.GetKey()
		shareType, shareWith := conversions.FormatGrantee(key.Grantee)
		owner := conversions.FormatUserID(key.Owner)
		query = "delete from oc_share where uid_owner=? AND fileid_prefix=? AND item_source=? AND share_type=? AND share_with=? AND (uid_owner=? or uid_initiator=?)"
		params = append(params, owner, key.ResourceId.StorageId, key.ResourceId.OpaqueId, shareType, shareWith, uid, uid)
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
	permissions := conversions.SharePermToInt(p.Permissions)
	uid := conversions.FormatUserID(ctxpkg.ContextMustGetUser(ctx).Id)

	var query string
	params := []interface{}{}
	switch {
	case ref.GetId() != nil:
		query = "update oc_share set permissions=?,stime=? where id=? AND (uid_owner=? or uid_initiator=?)"
		params = append(params, permissions, time.Now().Unix(), ref.GetId().OpaqueId, uid, uid)
	case ref.GetKey() != nil:
		key := ref.GetKey()
		shareType, shareWith := conversions.FormatGrantee(key.Grantee)
		owner := conversions.FormatUserID(key.Owner)
		query = "update oc_share set permissions=?,stime=? where (uid_owner=? or uid_initiator=?) AND fileid_prefix=? AND item_source=? AND share_type=? AND share_with=? AND (uid_owner=? or uid_initiator=?)"
		params = append(params, permissions, time.Now().Unix(), owner, owner, key.ResourceId.StorageId, key.ResourceId.OpaqueId, shareType, shareWith, uid, uid)
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
	query := `select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with,
				coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, coalesce(item_type, '') as item_type,
			  	id, stime, permissions, share_type
			  FROM oc_share WHERE (orphan = 0 or orphan IS NULL) AND (share_type=? OR share_type=?)`
	params := []interface{}{shareTypeUser, shareTypeGroup}

	groupedFilters := share.GroupFiltersByType(filters)
	if len(groupedFilters) > 0 {
		filterQuery, filterParams, err := translateFilters(groupedFilters)
		if err != nil {
			return nil, err
		}
		params = append(params, filterParams...)
		if filterQuery != "" {
			query = fmt.Sprintf("%s AND (%s)", query, filterQuery)
		}
	}

	uidOwnersQuery, uidOwnersParams, err := m.uidOwnerFilters(ctx, groupedFilters)
	if err != nil {
		return nil, err
	}
	params = append(params, uidOwnersParams...)
	if uidOwnersQuery != "" {
		query = fmt.Sprintf("%s AND (%s)", query, uidOwnersQuery)
	}

	rows, err := m.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var s conversions.DBShare
	shares := []*collaboration.Share{}
	for rows.Next() {
		if err := rows.Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.ItemType, &s.ID, &s.STime, &s.Permissions, &s.ShareType); err != nil {
			continue
		}
		shares = append(shares, conversions.ConvertToCS3Share(s))
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return shares, nil
}

// we list the shares that are targeted to the user in context or to the user groups.
func (m *mgr) ListReceivedShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.ReceivedShare, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	uid := conversions.FormatUserID(user.Id)

	params := []interface{}{uid, uid, uid, uid}
	for _, v := range user.Groups {
		params = append(params, v)
	}

	query := `SELECT coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with,
	            coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, coalesce(item_type, '') as item_type,
				ts.id, stime, permissions, share_type, coalesce(tr.state, 0) as state
			  FROM oc_share ts LEFT JOIN oc_share_status tr ON (ts.id = tr.id AND tr.recipient = ?)
			  WHERE (orphan = 0 or orphan IS NULL) AND (uid_owner != ? AND uid_initiator != ?)`
	if len(user.Groups) > 0 {
		query += " AND ((share_with=? AND share_type = 0) OR (share_type = 1 AND share_with in (?" + strings.Repeat(",?", len(user.Groups)-1) + ")))"
	} else {
		query += " AND (share_with=? AND share_type = 0)"
	}

	groupedFilters := share.GroupFiltersByType(filters)
	filterQuery, filterParams, err := translateFilters(groupedFilters)
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

	var s conversions.DBShare
	shares := []*collaboration.ReceivedShare{}
	for rows.Next() {
		if err := rows.Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.ItemType, &s.ID, &s.STime, &s.Permissions, &s.ShareType, &s.State); err != nil {
			continue
		}
		shares = append(shares, conversions.ConvertToCS3ReceivedShare(s))
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return shares, nil
}

func (m *mgr) getReceivedByID(ctx context.Context, id *collaboration.ShareId) (*collaboration.ReceivedShare, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	uid := conversions.FormatUserID(user.Id)

	params := []interface{}{uid, id.OpaqueId, uid}
	for _, v := range user.Groups {
		params = append(params, v)
	}

	s := conversions.DBShare{ID: id.OpaqueId}
	query := `select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with,
			    coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, coalesce(item_type, '') as item_type,
				stime, permissions, share_type, coalesce(tr.state, 0) as state
			  FROM oc_share ts LEFT JOIN oc_share_status tr ON (ts.id = tr.id AND tr.recipient = ?)
			  WHERE (orphan = 0 or orphan IS NULL) AND ts.id=?`
	if len(user.Groups) > 0 {
		query += " AND ((share_with=? AND share_type = 0) OR (share_type = 1 AND share_with in (?" + strings.Repeat(",?", len(user.Groups)-1) + ")))"
	} else {
		query += " AND (share_with=?  AND share_type = 0)"
	}
	if err := m.db.QueryRow(query, params...).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.ItemType, &s.STime, &s.Permissions, &s.ShareType, &s.State); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(id.OpaqueId)
		}
		return nil, err
	}
	return conversions.ConvertToCS3ReceivedShare(s), nil
}

func (m *mgr) getReceivedByKey(ctx context.Context, key *collaboration.ShareKey) (*collaboration.ReceivedShare, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	uid := conversions.FormatUserID(user.Id)

	shareType, shareWith := conversions.FormatGrantee(key.Grantee)
	params := []interface{}{uid, conversions.FormatUserID(key.Owner), key.ResourceId.StorageId, key.ResourceId.OpaqueId, shareType, shareWith, shareWith}
	for _, v := range user.Groups {
		params = append(params, v)
	}

	s := conversions.DBShare{}
	query := `select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with,
	            coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, coalesce(item_type, '') as item_type,
				ts.id, stime, permissions, share_type, coalesce(tr.state, 0) as state
			  FROM oc_share ts LEFT JOIN oc_share_status tr ON (ts.id = tr.id AND tr.recipient = ?)
			  WHERE (orphan = 0 or orphan IS NULL) AND uid_owner=? AND fileid_prefix=? AND item_source=? AND share_type=? AND share_with=?`
	if len(user.Groups) > 0 {
		query += " AND ((share_with=? AND share_type = 0) OR (share_type = 1 AND share_with in (?" + strings.Repeat(",?", len(user.Groups)-1) + ")))"
	} else {
		query += " AND (share_with=? AND share_type = 0)"
	}

	if err := m.db.QueryRow(query, params...).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.ItemType, &s.ID, &s.STime, &s.Permissions, &s.ShareType, &s.State); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(key.String())
		}
		return nil, err
	}
	return conversions.ConvertToCS3ReceivedShare(s), nil
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
	user := ctxpkg.ContextMustGetUser(ctx)

	rs, err := m.GetReceivedShare(ctx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: share.Share.Id}})
	if err != nil {
		return nil, err
	}

	for i := range fieldMask.Paths {
		switch fieldMask.Paths[i] {
		case "state":
			rs.State = share.State
		default:
			return nil, errtypes.NotSupported("updating " + fieldMask.Paths[i] + " is not supported")
		}
	}

	state := 0
	switch rs.GetState() {
	case collaboration.ShareState_SHARE_STATE_REJECTED:
		state = -1
	case collaboration.ShareState_SHARE_STATE_ACCEPTED:
		state = 1
	}

	params := []interface{}{rs.Share.Id.OpaqueId, conversions.FormatUserID(user.Id), state, state}
	query := "insert into oc_share_status(id, recipient, state) values(?, ?, ?) ON DUPLICATE KEY UPDATE state = ?"

	stmt, err := m.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	_, err = stmt.Exec(params...)
	if err != nil {
		return nil, err
	}

	return rs, nil
}

func (m *mgr) uidOwnerFilters(ctx context.Context, filters map[collaboration.Filter_Type][]*collaboration.Filter) (string, []interface{}, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	uid := conversions.FormatUserID(user.Id)

	query := "uid_owner=? or uid_initiator=?"
	params := []interface{}{uid, uid}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(m.c.GatewaySvc))
	if err != nil {
		return "", nil, err
	}

	if resourceFilters, ok := filters[collaboration.Filter_TYPE_RESOURCE_ID]; ok {
		for _, f := range resourceFilters {
			// For shares inside project spaces, if the user is an admin, we try to list all shares created by other admins
			if strings.HasPrefix(f.GetResourceId().GetStorageId(), projectInstancesPrefix) {
				res, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: f.GetResourceId()}})
				if err != nil || res.Status.Code != rpc.Code_CODE_OK {
					continue
				}

				// The path will look like /eos/project/c/cernbox, we need to extract the project name
				parts := strings.SplitN(res.Info.Path, "/", 6)
				if len(parts) < 5 {
					continue
				}

				adminGroup := projectSpaceGroupsPrefix + parts[4] + projectSpaceAdminGroupsSuffix
				for _, g := range user.Groups {
					if g == adminGroup {
						// User belongs to the admin group, list all shares for the resource

						// TODO: this only works if shares for a single project are requested.
						// If shares for multiple projects are requested, then we're not checking if the
						// user is an admin for all of those. We can append the query ` or uid_owner=?`
						// for all the project owners, which works fine for new reva
						// but won't work for revaold since there, we store the uid of the share creator as uid_owner.
						// For this to work across the two versions, this change would have to be made in revaold
						// but it won't be straightforward as there, the storage provider doesn't return the
						// resource owners.
						return "", []interface{}{}, nil
					}
				}
			}
		}
	}

	return query, params, nil
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
func translateFilters(filters map[collaboration.Filter_Type][]*collaboration.Filter) (string, []interface{}, error) {
	var (
		filterQuery string
		params      []interface{}
	)

	// If multiple filters of the same type are passed to this function, they need to be combined with the `OR` operator.
	// That is why the filters got grouped by type.
	// For every given filter type, iterate over the filters and if there are more than one combine them.
	// Combine the different filter types using `AND`
	var filterCounter = 0
	for filterType, currFilters := range filters {
		switch filterType {
		case collaboration.Filter_TYPE_RESOURCE_ID:
			filterQuery += "("
			for i, f := range currFilters {
				filterQuery += "(fileid_prefix =? AND item_source=?)"
				params = append(params, f.GetResourceId().StorageId, f.GetResourceId().OpaqueId)

				if i != len(currFilters)-1 {
					filterQuery += " OR "
				}
			}
			filterQuery += ")"
		case collaboration.Filter_TYPE_GRANTEE_TYPE:
			filterQuery += "("
			for i, f := range currFilters {
				filterQuery += "share_type=?"
				params = append(params, granteeTypeToShareType(f.GetGranteeType()))

				if i != len(currFilters)-1 {
					filterQuery += " OR "
				}
			}
			filterQuery += ")"
		case collaboration.Filter_TYPE_EXCLUDE_DENIALS:
			// TODO this may change once the mapping of permission to share types is completed (cf. pkg/cbox/utils/conversions.go)
			filterQuery += "(permissions > 0)"
		default:
			return "", nil, fmt.Errorf("filter type is not supported")
		}
		if filterCounter != len(filters)-1 {
			filterQuery += " AND "
		}
		filterCounter++
	}
	return filterQuery, params, nil
}
