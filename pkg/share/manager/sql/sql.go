// Copyright 2018-2020 CERN
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
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	// Provides mysql drivers
	_ "github.com/go-sql-driver/mysql"
)

func init() {
	registry.Register("sql", New)
}

type config struct {
	DbUsername string `mapstructure:"db_username"`
	DbPassword string `mapstructure:"db_password"`
	DbHost     string `mapstructure:"db_host"`
	DbPort     int    `mapstructure:"db_port"`
	DbName     string `mapstructure:"db_name"`
}

type mgr struct {
	c  *config
	db *sql.DB
}

type dbShare struct {
	ID           int
	UIDOwner     string
	UIDInitiator string
	Prefix       string
	ItemSource   string
	ShareWith    string
	Permissions  int
	ShareType    int
	STime        int
	FileTarget   string
	State        int
}

// New returns a new mgr.
func New(m map[string]interface{}) (share.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.DbUsername, c.DbPassword, c.DbHost, c.DbPort, c.DbName))
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
	return c, nil
}

func (m *mgr) Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error) {
	user := user.ContextMustGetUser(ctx)

	// do not allow share to myself or the owner if share is for a user
	// TODO(labkode): should not this be caught already at the gw level?
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER &&
		((g.Grantee.Id.Idp == user.Id.Idp && g.Grantee.Id.OpaqueId == user.Id.OpaqueId) ||
			(g.Grantee.Id.Idp == md.Owner.Idp && g.Grantee.Id.OpaqueId == md.Owner.OpaqueId)) {
		return nil, errors.New("json: owner/creator and grantee are the same")
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

	shareType := granteeTypeToInt(g.Grantee.Type)
	itemType := resourceTypeToItem(md.Type)
	targetPath := path.Join("/", path.Base(md.Path))
	permissions := sharePermToInt(g.Permissions.Permissions)
	prefix := md.Id.StorageId
	itemSource := md.Id.OpaqueId
	fileSource, err := strconv.ParseUint(itemSource, 10, 64)
	if err != nil {
		// it can be the case that the item source may be a character string
		// we leave fileSource blank in that case
		fileSource = 0
	}

	stmtString := "insert into oc_share set share_type=?,uid_owner=?,uid_initiator=?,item_type=?,fileid_prefix=?,item_source=?,file_source=?,permissions=?,stime=?,share_with=?,file_target=?"
	stmtValues := []interface{}{shareType, formatUserID(md.Owner), formatUserID(user.Id), itemType, prefix, itemSource, fileSource, permissions, now, formatUserID(g.Grantee.Id), targetPath}

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
	intID, err := strconv.ParseInt(id.OpaqueId, 10, 64)
	if err != nil {
		return nil, err
	}

	s := dbShare{ID: int(intID)}
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, stime, permissions, share_type FROM oc_share WHERE (orphan = 0 or orphan IS NULL) AND id=?"
	if err := m.db.QueryRow(query, id.OpaqueId).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.STime, &s.Permissions, &s.ShareType); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(id.OpaqueId)
		}
		return nil, err
	}
	return convertToCS3Share(s), nil
}

func (m *mgr) getByKey(ctx context.Context, key *collaboration.ShareKey) (*collaboration.Share, error) {
	s := dbShare{}
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, id, stime, permissions, share_type FROM oc_share WHERE (orphan = 0 or orphan IS NULL) AND (uid_owner=? or uid_initiator=?) AND fileid_prefix=? AND item_source=? AND share_type=? AND share_with=?"
	if err := m.db.QueryRow(query, formatUserID(key.Owner), formatUserID(key.Owner), key.ResourceId.StorageId, key.ResourceId.OpaqueId, granteeTypeToInt(key.Grantee.Type), formatUserID(key.Grantee.Id)).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.ID, &s.STime, &s.Permissions, &s.ShareType); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(key.String())
		}
		return nil, err
	}
	return convertToCS3Share(s), nil
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

	// check if we are the owner
	user := user.ContextMustGetUser(ctx)
	if (user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId) ||
		(user.Id.Idp == s.Creator.Idp && user.Id.OpaqueId == s.Creator.OpaqueId) {
		return s, nil
	}

	// we return not found to not disclose information
	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
	user := user.ContextMustGetUser(ctx)

	var query string
	params := []interface{}{}
	switch {
	case ref.GetId() != nil:
		query = "delete from oc_share where id=? AND (uid_owner=? or uid_initiator=?)"
		params = append(params, ref.GetId().OpaqueId, formatUserID(user.Id), formatUserID(user.Id))
	case ref.GetKey() != nil:
		key := ref.GetKey()
		if key.Owner != user.Id {
			return errtypes.NotFound(ref.String())
		}
		query = "delete from oc_share where (uid_owner=? or uid_initiator=?) AND fileid_prefix=? AND item_source=? AND share_type=? AND share_with=?"
		params = append(params, formatUserID(key.Owner), formatUserID(key.Owner), key.ResourceId.StorageId, key.ResourceId.OpaqueId, granteeTypeToInt(key.Grantee.Type), formatUserID(key.Grantee.Id))
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
	user := user.ContextMustGetUser(ctx)
	permissions := sharePermToInt(p.Permissions)

	var query string
	params := []interface{}{}
	switch {
	case ref.GetId() != nil:
		query = "update oc_share set permissions=?,stime=? where id=? AND (uid_owner=? or uid_initiator=?)"
		params = append(params, permissions, time.Now().Unix(), ref.GetId().OpaqueId, formatUserID(user.Id), formatUserID(user.Id))
	case ref.GetKey() != nil:
		key := ref.GetKey()
		if key.Owner != user.Id {
			return nil, errtypes.NotFound(ref.String())
		}
		query = "update oc_share set permissions=?,stime=? where (uid_owner=? or uid_initiator=?) AND fileid_prefix=? AND item_source=? AND share_type=? AND share_with=?"
		params = append(params, permissions, time.Now().Unix(), formatUserID(key.Owner), formatUserID(key.Owner), key.ResourceId.StorageId, key.ResourceId.OpaqueId, granteeTypeToInt(key.Grantee.Type), formatUserID(key.Grantee.Id))
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

func (m *mgr) ListShares(ctx context.Context, filters []*collaboration.ListSharesRequest_Filter) ([]*collaboration.Share, error) {
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, id, stime, permissions, share_type FROM oc_share WHERE (orphan = 0 or orphan IS NULL) AND (uid_owner=? or uid_initiator=?) AND (share_type=? OR share_type=?)"
	var filterQuery string
	params := []interface{}{formatUserID(user.ContextMustGetUser(ctx).Id), formatUserID(user.ContextMustGetUser(ctx).Id), 0, 1}
	for i, f := range filters {
		if f.Type == collaboration.ListSharesRequest_Filter_TYPE_RESOURCE_ID {
			filterQuery += "(fileid_prefix=? AND item_source=?)"
			if i != len(filters)-1 {
				filterQuery += " OR "
			}
			params = append(params, f.GetResourceId().StorageId, f.GetResourceId().OpaqueId)
		}
	}
	if filterQuery != "" {
		query = fmt.Sprintf("%s AND (%s)", query, filterQuery)
	}

	rows, err := m.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var s dbShare
	shares := []*collaboration.Share{}
	for rows.Next() {
		if err := rows.Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.ID, &s.STime, &s.Permissions, &s.ShareType); err != nil {
			continue
		}
		shares = append(shares, convertToCS3Share(s))
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return shares, nil
}

// we list the shares that are targeted to the user in context or to the user groups.
func (m *mgr) ListReceivedShares(ctx context.Context) ([]*collaboration.ReceivedShare, error) {
	user := user.ContextMustGetUser(ctx)

	params := []interface{}{formatUserID(user.Id), formatUserID(user.Id)}
	for _, v := range user.Groups {
		params = append(params, v)
	}

	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, id, stime, permissions, share_type, accepted FROM oc_share WHERE (orphan = 0 or orphan IS NULL) AND id not in (SELECT distinct(id) FROM oc_share_acl WHERE rejected_by=?)"
	if len(user.Groups) > 0 {
		query += "AND (share_with=? OR share_with in (?" + strings.Repeat(",?", len(user.Groups)-1) + "))"
	} else {
		query += "AND (share_with=?)"
	}

	rows, err := m.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var s dbShare
	shares := []*collaboration.ReceivedShare{}
	for rows.Next() {
		if err := rows.Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.ID, &s.STime, &s.Permissions, &s.ShareType, &s.State); err != nil {
			continue
		}
		shares = append(shares, convertToCS3ReceivedShare(s))
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return shares, nil
}

func (m *mgr) getReceivedByID(ctx context.Context, id *collaboration.ShareId) (*collaboration.ReceivedShare, error) {
	user := user.ContextMustGetUser(ctx)

	intID, err := strconv.ParseInt(id.OpaqueId, 10, 64)
	if err != nil {
		return nil, err
	}

	params := []interface{}{id.OpaqueId, formatUserID(user.Id), formatUserID(user.Id)}
	for _, v := range user.Groups {
		params = append(params, v)
	}

	s := dbShare{ID: int(intID)}
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, stime, permissions, share_type, accepted FROM oc_share WHERE (orphan = 0 or orphan IS NULL) AND id=? AND id not in (SELECT distinct(id) FROM oc_share_acl WHERE rejected_by=?)"
	if len(user.Groups) > 0 {
		query += "AND (share_with=? OR share_with in (?" + strings.Repeat(",?", len(user.Groups)-1) + "))"
	} else {
		query += "AND (share_with=?)"
	}
	if err := m.db.QueryRow(query, params...).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.STime, &s.Permissions, &s.ShareType, &s.State); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(id.OpaqueId)
		}
		return nil, err
	}
	return convertToCS3ReceivedShare(s), nil
}

func (m *mgr) getReceivedByKey(ctx context.Context, key *collaboration.ShareKey) (*collaboration.ReceivedShare, error) {
	s := dbShare{}
	query := "select coalesce(uid_owner, '') as uid_owner, coalesce(uid_initiator, '') as uid_initiator, coalesce(share_with, '') as share_with, coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source, id, stime, permissions, share_type, accepted FROM oc_share WHERE (orphan = 0 or orphan IS NULL) AND (uid_owner=? or uid_initiator=?) AND fileid_prefix=? AND item_source=? AND share_type=? AND share_with=? AND id not in (SELECT distinct(id) FROM oc_share_acl WHERE rejected_by=?)"
	if err := m.db.QueryRow(query, formatUserID(key.Owner), formatUserID(key.Owner), key.ResourceId.StorageId, key.ResourceId.OpaqueId, granteeTypeToInt(key.Grantee.Type), formatUserID(key.Grantee.Id), formatUserID(key.Grantee.Id)).Scan(&s.UIDOwner, &s.UIDInitiator, &s.ShareWith, &s.Prefix, &s.ItemSource, &s.ID, &s.STime, &s.Permissions, &s.ShareType, &s.State); err != nil {
		if err == sql.ErrNoRows {
			return nil, errtypes.NotFound(key.String())
		}
		return nil, err
	}

	return convertToCS3ReceivedShare(s), nil
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

	user := user.ContextMustGetUser(ctx)
	if s.Share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && s.Share.Grantee.Id.OpaqueId == user.Id.OpaqueId && s.Share.Grantee.Id.Idp == user.Id.Idp {
		return s, nil
	}

	if s.Share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		for _, v := range user.Groups {
			if s.Share.Grantee.Id.OpaqueId == v {
				return s, nil
			}
		}
	}

	// we return not found to not disclose information
	return nil, errtypes.NotFound(ref.String())

}

func (m *mgr) UpdateReceivedShare(ctx context.Context, ref *collaboration.ShareReference, f *collaboration.UpdateReceivedShareRequest_UpdateField) (*collaboration.ReceivedShare, error) {
	user := user.ContextMustGetUser(ctx)

	rs, err := m.GetReceivedShare(ctx, ref)
	if err != nil {
		return nil, err
	}

	var query string
	params := []interface{}{rs.Share.Id.OpaqueId}
	switch f.GetState() {
	case collaboration.ShareState_SHARE_STATE_REJECTED:
		query = "insert into oc_share_acl(id, rejected_by) values(?, ?)"
		params = append(params, formatUserID(user.Id))
	case collaboration.ShareState_SHARE_STATE_ACCEPTED:
		query = "update oc_share set accepted=1 where id=?"
	}

	stmt, err := m.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	_, err = stmt.Exec(params...)
	if err != nil {
		return nil, err
	}

	rs.State = f.GetState()
	return rs, nil
}
