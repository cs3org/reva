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
	"strconv"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/cbox/utils"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/repository/registry"
	"github.com/go-sql-driver/mysql"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"
)

func init() {
	registry.Register("sql", New)
}

// New creates a Repository with a SQL driver.
func New(ctx context.Context, c map[string]interface{}) (share.Repository, error) {
	conf, err := parseConfig(c)
	if err != nil {
		return nil, err
	}
	return NewFromConfig(ctx, conf)
}

type mgr struct {
	c   *config
	db  *sql.DB
	now func() time.Time
}

// NewFromConfig creates a Repository with a SQL driver using the given config.
func NewFromConfig(ctx context.Context, conf *config) (share.Repository, error) {
	if conf.now == nil {
		conf.now = time.Now
	}

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s", conf.DBUsername, conf.DBPassword, conf.DBAddress, conf.DBName))
	if err != nil {
		return nil, errors.Wrap(err, "sql: error opening connection to mysql database")
	}

	m := &mgr{
		c:   conf,
		db:  db,
		now: conf.now,
	}
	return m, nil
}

type config struct {
	DBUsername string `mapstructure:"db_username"`
	DBPassword string `mapstructure:"db_password"`
	DBAddress  string `mapstructure:"db_address"`
	DBName     string `mapstructure:"db_name"`

	now func() time.Time // set only from tests
}

func parseConfig(conf map[string]interface{}) (*config, error) {
	var c config
	if err := mapstructure.Decode(conf, &c); err != nil {
		return nil, errors.Wrap(err, "error decoding config")
	}
	return &c, nil
}

func formatUserID(u *userpb.UserId) string {
	return fmt.Sprintf("%s@%s", u.OpaqueId, u.Idp)
}

func storeWebDAVAccessMethod(tx *sql.Tx, shareID int64, o *ocm.AccessMethod_WebdavOptions) error {
	amID, err := storeAccessMethod(tx, shareID, WebDAVAccessMethod)
	if err != nil {
		return err
	}

	query := "INSERT INTO ocm_access_method_webdav SET ocm_access_method_id=?, permissions=?"
	params := []any{amID, conversions.RoleFromResourcePermissions(o.WebdavOptions.Permissions).OCSPermissions()}

	_, err = tx.Exec(query, params...)
	return err
}

func storeWebappAccessMethod(tx *sql.Tx, shareID int64, o *ocm.AccessMethod_WebappOptions) error {
	amID, err := storeAccessMethod(tx, shareID, WebappAccessMethod)
	if err != nil {
		return err
	}

	query := "INSERT INTO ocm_access_method_webapp SET ocm_access_method_id=?, view_mode=?"
	params := []any{amID, int(o.WebappOptions.ViewMode)}

	_, err = tx.Exec(query, params...)
	return err
}

func storeTransferAccessMethod(tx *sql.Tx, shareID int64, _ *ocm.AccessMethod_TransferOptions) error {
	_, err := storeAccessMethod(tx, shareID, TransferAccessMethod)
	return err
}

func storeAccessMethod(tx *sql.Tx, shareID int64, t AccessMethod) (int64, error) {
	query := "INSERT INTO ocm_shares_access_methods SET ocm_share_id=?, type=?"
	params := []any{shareID, int(t)}

	res, err := tx.Exec(query, params...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// StoreShare stores a share.
func (m *mgr) StoreShare(ctx context.Context, s *ocm.Share) (*ocm.Share, error) {
	if err := transaction(ctx, m.db, func(tx *sql.Tx) error {
		// store the share
		query := "INSERT INTO ocm_shares SET token=?,fileid_prefix=?,item_source=?,name=?,share_with=?,owner=?,initiator=?,ctime=?,mtime=?,type=?"
		params := []any{s.Token, s.ResourceId.StorageId, s.ResourceId.OpaqueId, s.Name, formatUserID(s.Grantee.GetUserId()), s.Owner.OpaqueId, s.Creator.OpaqueId, s.Ctime.Seconds, s.Mtime.Seconds, convertFromCS3OCMShareType(s.ShareType)}

		if s.Expiration != nil {
			query += ",expiration=?"
			params = append(params, s.Expiration.Seconds)
		}

		res, err := tx.Exec(query, params...)
		if err != nil {
			return err
		}

		id, err := res.LastInsertId()
		if err != nil {
			return err
		}

		// store the access methods of the share
		for _, m := range s.AccessMethods {
			switch r := m.Term.(type) {
			case *ocm.AccessMethod_WebdavOptions:
				if err := storeWebDAVAccessMethod(tx, id, r); err != nil {
					return err
				}
			case *ocm.AccessMethod_WebappOptions:
				if err := storeWebappAccessMethod(tx, id, r); err != nil {
					return err
				}
			case *ocm.AccessMethod_TransferOptions:
				if err := storeTransferAccessMethod(tx, id, r); err != nil {
					return err
				}
			}
		}

		s.Id = &ocm.ShareId{OpaqueId: strconv.FormatInt(id, 10)}
		return nil
	}); err != nil {
		// check if the share already exists in the db
		// https://dev.mysql.com/doc/mysql-errors/8.0/en/server-error-reference.html#error_er_dup_unique
		// https://dev.mysql.com/doc/mysql-errors/8.0/en/server-error-reference.html#error_er_dup_entry
		var e *mysql.MySQLError
		if errors.As(err, &e) && (e.Number == 1169 || e.Number == 1062) {
			return nil, share.ErrShareAlreadyExisting
		}
		return nil, err
	}
	return s, nil
}

// this func will run f in a transaction, committing if no errors
// rolling back if there were error running f.
func transaction(ctx context.Context, db *sql.DB, f func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	var txErr error
	defer func() {
		if txErr == nil {
			_ = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}()

	txErr = f(tx)
	return txErr
}

// GetShare gets the information for a share by the given ref.
func (m *mgr) GetShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*ocm.Share, error) {
	var (
		s   *ocm.Share
		err error
	)
	switch {
	case ref.GetId() != nil:
		s, err = m.getByID(ctx, user, ref.GetId())
	case ref.GetKey() != nil:
		s, err = m.getByKey(ctx, user, ref.GetKey())
	case ref.GetToken() != "":
		s, err = m.getByToken(ctx, ref.GetToken())
	default:
		err = errtypes.NotFound(ref.String())
	}

	return s, err
}

func (m *mgr) getByID(ctx context.Context, user *userpb.User, id *ocm.ShareId) (*ocm.Share, error) {
	query := "SELECT id, token, fileid_prefix, item_source, name, share_with, owner, initiator, ctime, mtime, expiration, type FROM ocm_shares WHERE id=? AND (initiator=? OR owner=?)"

	var s dbShare
	if err := m.db.QueryRowContext(ctx, query, id.OpaqueId, user.Id.OpaqueId, user.Id.OpaqueId).Scan(&s.ID, &s.Token, &s.Prefix, &s.ItemSource, &s.Name, &s.ShareWith, &s.Owner, &s.Initiator, &s.Ctime, &s.Mtime, &s.Expiration, &s.ShareType); err != nil {
		if err == sql.ErrNoRows {
			return nil, share.ErrShareNotFound
		}
		return nil, err
	}

	am, err := m.getAccessMethods(ctx, s.ID)
	if err != nil {
		return nil, err
	}

	return convertToCS3OCMShare(&s, am), nil
}

func (m *mgr) getByKey(ctx context.Context, user *userpb.User, key *ocm.ShareKey) (*ocm.Share, error) {
	query := "SELECT id, token, fileid_prefix, item_source, name, share_with, owner, initiator, ctime, mtime, expiration, type FROM ocm_shares WHERE owner=? AND fileid_prefix=? AND item_source=? AND share_with=? AND (initiator=? OR owner=?)"

	var s dbShare
	if err := m.db.QueryRowContext(ctx, query, key.Owner.OpaqueId, key.ResourceId.StorageId, key.ResourceId.OpaqueId, formatUserID(key.Grantee.GetUserId()), user.Id.OpaqueId, user.Id.OpaqueId).Scan(&s.ID, &s.Token, &s.Prefix, &s.ItemSource, &s.Name, &s.ShareWith, &s.Owner, &s.Initiator, &s.Ctime, &s.Mtime, &s.Expiration, &s.ShareType); err != nil {
		if err == sql.ErrNoRows {
			return nil, share.ErrShareNotFound
		}
	}

	am, err := m.getAccessMethods(ctx, s.ID)
	if err != nil {
		return nil, err
	}

	return convertToCS3OCMShare(&s, am), nil
}

func (m *mgr) getByToken(ctx context.Context, token string) (*ocm.Share, error) {
	query := "SELECT id, token, fileid_prefix, item_source, name, share_with, owner, initiator, ctime, mtime, expiration, type FROM ocm_shares WHERE token=?"

	var s dbShare
	if err := m.db.QueryRowContext(ctx, query, token).Scan(&s.ID, &s.Token, &s.Prefix, &s.ItemSource, &s.Name, &s.ShareWith, &s.Owner, &s.Initiator, &s.Ctime, &s.Mtime, &s.Expiration, &s.ShareType); err != nil {
		if err == sql.ErrNoRows {
			return nil, share.ErrShareNotFound
		}
	}

	am, err := m.getAccessMethods(ctx, s.ID)
	if err != nil {
		return nil, err
	}

	return convertToCS3OCMShare(&s, am), nil
}

func (m *mgr) getAccessMethods(ctx context.Context, id int) ([]*ocm.AccessMethod, error) {
	query := "SELECT m.type, dav.permissions, app.view_mode FROM ocm_shares_access_methods as m LEFT JOIN ocm_access_method_webdav as dav ON m.id=dav.ocm_access_method_id LEFT JOIN ocm_access_method_webapp as app ON m.id=app.ocm_access_method_id WHERE m.ocm_share_id=?"

	var methods []*ocm.AccessMethod
	rows, err := m.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}

	var a dbAccessMethod
	for rows.Next() {
		if err := rows.Scan(&a.Type, &a.WebDAVPermissions, &a.WebAppViewMode); err != nil {
			continue
		}
		methods = append(methods, convertToCS3AccessMethod(&a))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return methods, nil
}

// DeleteShare deletes the share pointed by ref.
func (m *mgr) DeleteShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) error {
	switch {
	case ref.GetId() != nil:
		return m.deleteByID(ctx, user, ref.GetId())
	case ref.GetKey() != nil:
		return m.deleteByKey(ctx, user, ref.GetKey())
	default:
		return errtypes.NotFound(ref.String())
	}
}

func (m *mgr) deleteByID(ctx context.Context, user *userpb.User, id *ocm.ShareId) error {
	query := "DELETE FROM ocm_shares WHERE id=? AND (owner=? OR initiator=?)"
	_, err := m.db.ExecContext(ctx, query, id.OpaqueId, user.Id.OpaqueId, user.Id.OpaqueId)
	return err
}

func (m *mgr) deleteByKey(ctx context.Context, user *userpb.User, key *ocm.ShareKey) error {
	query := "DELETE FROM ocm_shares WHERE owner=? AND fileid_prefix=? AND item_source=? AND share_with=? AND (initiator=? OR owner=?)"
	_, err := m.db.ExecContext(ctx, query, key.Owner.OpaqueId, key.ResourceId.StorageId, key.ResourceId.OpaqueId, formatUserID(key.Grantee.GetUserId()), user.Id.OpaqueId, user.Id.OpaqueId)
	return err
}

// UpdateShare updates the mode of the given share.
func (m *mgr) UpdateShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*ocm.Share, error) {
	switch {
	case ref.GetId() != nil:
		return m.updateShareByID(ctx, user, ref.GetId(), f...)
	case ref.GetKey() != nil:
		return m.updateShareByKey(ctx, user, ref.GetKey(), f...)
	default:
		return nil, errtypes.NotFound(ref.String())
	}
}

func (m *mgr) queriesUpdatesOnShare(ctx context.Context, id *ocm.ShareId, f ...*ocm.UpdateOCMShareRequest_UpdateField) (string, []string, []any, [][]any, error) {
	var qi strings.Builder
	params := []any{}

	qe := []string{}
	eparams := [][]any{}

	for _, field := range f {
		switch u := field.Field.(type) {
		case *ocm.UpdateOCMShareRequest_UpdateField_Expiration:
			qi.WriteString("expiration=?")
			params = append(params, u.Expiration.Seconds)
		case *ocm.UpdateOCMShareRequest_UpdateField_AccessMethods:
			// TODO: access method can be added or removed as well
			// now they can only be updated
			switch t := u.AccessMethods.Term.(type) {
			case *ocm.AccessMethod_WebdavOptions:
				q := "UPDATE ocm_access_method_webdav SET permissions=? WHERE ocm_access_method_id=(SELECT id FROM ocm_shares_access_methods WHERE ocm_share_id=? AND type=?)"
				qe = append(qe, q)
				eparams = append(eparams, []any{utils.SharePermToInt(t.WebdavOptions.Permissions), id.OpaqueId, WebDAVAccessMethod})
			case *ocm.AccessMethod_WebappOptions:
				q := "UPDATE ocm_access_method_webapp SET view_mode=? WHERE ocm_access_method_id=(SELECT id FROM ocm_shares_access_methods WHERE ocm_share_id=? AND type=?)"
				qe = append(qe, q)
				eparams = append(eparams, []any{t.WebappOptions.ViewMode, id.OpaqueId, WebappAccessMethod})
			}
		}
	}
	return qi.String(), qe, params, eparams, nil
}

func (m *mgr) updateShareByID(ctx context.Context, user *userpb.User, id *ocm.ShareId, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*ocm.Share, error) {
	var query strings.Builder

	now := m.now().Unix()
	query.WriteString("UPDATE ocm_shares SET ")
	params := []any{}

	squery, am, sparams, paramsAm, err := m.queriesUpdatesOnShare(ctx, id, f...)
	if err != nil {
		return nil, err
	}

	if squery != "" {
		query.WriteString(squery)
		query.WriteString(", ")
	}

	query.WriteString("mtime=? WHERE id=? AND (initiator=? OR owner=?)")
	params = append(params, sparams...)
	params = append(params, now, id.OpaqueId, user.Id.OpaqueId, user.Id.OpaqueId)

	if err := transaction(ctx, m.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, query.String(), params...); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return share.ErrShareNotFound
			}
		}

		for i, q := range am {
			if _, err := tx.ExecContext(ctx, q, paramsAm[i]...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return m.getByID(ctx, user, id)
}

func (m *mgr) updateShareByKey(ctx context.Context, user *userpb.User, key *ocm.ShareKey, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*ocm.Share, error) {
	share, err := m.getByKey(ctx, user, key)
	if err != nil {
		return nil, err
	}
	return m.updateShareByID(ctx, user, share.Id, f...)
}

func translateFilters(filters []*ocm.ListOCMSharesRequest_Filter) (string, []any, error) {
	var (
		filterQuery strings.Builder
		params      []any
	)

	grouped := groupFiltersByType(filters)

	var count int
	for _, lst := range grouped {
		for n, f := range lst {
			switch filter := f.Term.(type) {
			case *ocm.ListOCMSharesRequest_Filter_ResourceId:
				filterQuery.WriteString("fileid_prefix=? AND item_source=?")
				params = append(params, filter.ResourceId.StorageId, filter.ResourceId.OpaqueId)
			case *ocm.ListOCMSharesRequest_Filter_Creator:
				filterQuery.WriteString("initiator=?")
				params = append(params, filter.Creator.OpaqueId)
			case *ocm.ListOCMSharesRequest_Filter_Owner:
				filterQuery.WriteString("owner=?")
				params = append(params, filter.Owner.OpaqueId)
			default:
				return "", nil, errtypes.BadRequest("unknown filter")
			}

			if n != len(lst)-1 {
				filterQuery.WriteString(" OR ")
			}
		}
		if count != len(grouped)-1 {
			filterQuery.WriteString(" AND ")
		}
		count++
	}

	return filterQuery.String(), params, nil
}

func groupFiltersByType(filters []*ocm.ListOCMSharesRequest_Filter) map[ocm.ListOCMSharesRequest_Filter_Type][]*ocm.ListOCMSharesRequest_Filter {
	m := make(map[ocm.ListOCMSharesRequest_Filter_Type][]*ocm.ListOCMSharesRequest_Filter)
	for _, f := range filters {
		m[f.Type] = append(m[f.Type], f)
	}
	return m
}

// ListShares returns the shares created by the user. If md is provided is not nil,
// it returns only shares attached to the given resource.
func (m *mgr) ListShares(ctx context.Context, user *userpb.User, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {
	query := "SELECT id, token, fileid_prefix, item_source, name, share_with, owner, initiator, ctime, mtime, expiration, type FROM ocm_shares WHERE (initiator=? OR owner=?)"
	params := []any{user.Id.OpaqueId, user.Id.OpaqueId}

	filterQuery, filterParams, err := translateFilters(filters)
	if err != nil {
		return nil, err
	}

	if filterQuery != "" {
		query = fmt.Sprintf("%s AND (%s)", query, filterQuery)
		params = append(params, filterParams...)
	}

	rows, err := m.db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, err
	}

	var s dbShare
	shares := []*ocm.Share{}
	var ids []any
	for rows.Next() {
		if err := rows.Scan(&s.ID, &s.Token, &s.Prefix, &s.ItemSource, &s.Name, &s.ShareWith, &s.Owner, &s.Initiator, &s.Ctime, &s.Mtime, &s.Expiration, &s.ShareType); err != nil {
			continue
		}
		shares = append(shares, convertToCS3OCMShare(&s, nil))
		ids = append(ids, s.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// resolve the access method of all the shares
	am, err := m.getAccessMethodsIds(ctx, ids)
	if err != nil {
		return nil, err
	}

	// join the results to get the shares with access methods
	for _, share := range shares {
		if methods, ok := am[share.Id.OpaqueId]; ok {
			share.AccessMethods = methods
		}
	}

	return shares, nil
}

func (m *mgr) getAccessMethodsIds(ctx context.Context, ids []any) (map[string][]*ocm.AccessMethod, error) {
	methods := make(map[string][]*ocm.AccessMethod)
	if len(ids) == 0 {
		return methods, nil
	}

	query := "SELECT m.ocm_share_id, m.type, dav.permissions, app.view_mode FROM ocm_shares_access_methods as m LEFT JOIN ocm_access_method_webdav as dav ON m.id=dav.ocm_access_method_id LEFT JOIN ocm_access_method_webapp as app ON m.id=app.ocm_access_method_id WHERE m.ocm_share_id IN "
	in := strings.Repeat("?,", len(ids))
	query += "(" + in[:len(in)-1] + ")"

	rows, err := m.db.QueryContext(ctx, query, ids...)
	if err != nil {
		return nil, err
	}

	var am dbAccessMethod
	for rows.Next() {
		if err := rows.Scan(&am.ShareID, &am.Type, &am.WebDAVPermissions, &am.WebAppViewMode); err != nil {
			continue
		}
		m := convertToCS3AccessMethod(&am)
		methods[am.ShareID] = append(methods[am.ShareID], m)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return methods, nil
}

func storeWebDAVProtocol(tx *sql.Tx, shareID int64, o *ocm.Protocol_WebdavOptions) error {
	pID, err := storeProtocol(tx, shareID, WebDAVProtocol)
	if err != nil {
		return err
	}

	query := "INSERT INTO ocm_protocol_webdav SET ocm_protocol_id=?, uri=?, shared_secret=?, permissions=?"
	params := []any{pID, o.WebdavOptions.Uri, o.WebdavOptions.SharedSecret, utils.SharePermToInt(o.WebdavOptions.Permissions.Permissions)}

	_, err = tx.Exec(query, params...)
	return err
}

func storeWebappProtocol(tx *sql.Tx, shareID int64, o *ocm.Protocol_WebappOptions) error {
	pID, err := storeProtocol(tx, shareID, WebappProtocol)
	if err != nil {
		return err
	}

	query := "INSERT INTO ocm_protocol_webapp SET ocm_protocol_id=?, uri_template=?, view_mode=?"
	params := []any{pID, o.WebappOptions.UriTemplate, o.WebappOptions.ViewMode}

	_, err = tx.Exec(query, params...)
	return err
}

func storeTransferProtocol(tx *sql.Tx, shareID int64, o *ocm.Protocol_TransferOptions) error {
	pID, err := storeProtocol(tx, shareID, TransferProtocol)
	if err != nil {
		return err
	}

	query := "INSERT INTO ocm_protocol_transfer SET ocm_protocol_id=?, source_uri=?, shared_secret=?, size=?"
	params := []any{pID, o.TransferOptions.SourceUri, o.TransferOptions.SharedSecret, o.TransferOptions.Size}

	_, err = tx.Exec(query, params...)
	return err
}

func storeProtocol(tx *sql.Tx, shareID int64, p Protocol) (int64, error) {
	query := "INSERT INTO ocm_received_share_protocols SET ocm_received_share_id=?, type=?"
	params := []any{shareID, int(p)}

	res, err := tx.Exec(query, params...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// StoreReceivedShare stores a received share.
func (m *mgr) StoreReceivedShare(ctx context.Context, s *ocm.ReceivedShare) (*ocm.ReceivedShare, error) {
	if err := transaction(ctx, m.db, func(tx *sql.Tx) error {
		query := "INSERT INTO ocm_received_shares SET name=?,remote_share_id=?,item_type=?,share_with=?,owner=?,initiator=?,ctime=?,mtime=?,type=?,state=?"
		params := []any{s.Name, s.RemoteShareId, convertFromCS3ResourceType(s.ResourceType), s.Grantee.GetUserId().OpaqueId, formatUserID(s.Owner), formatUserID(s.Creator), s.Ctime.Seconds, s.Mtime.Seconds, convertFromCS3OCMShareType(s.ShareType), convertFromCS3OCMShareState(s.State)}

		if s.Expiration != nil {
			query += ",expiration=?"
			params = append(params, s.Expiration.Seconds)
		}

		res, err := tx.Exec(query, params...)
		if err != nil {
			return err
		}

		id, err := res.LastInsertId()
		if err != nil {
			return err
		}

		for _, p := range s.Protocols {
			switch r := p.Term.(type) {
			case *ocm.Protocol_WebdavOptions:
				if err := storeWebDAVProtocol(tx, id, r); err != nil {
					return err
				}
			case *ocm.Protocol_WebappOptions:
				if err := storeWebappProtocol(tx, id, r); err != nil {
					return err
				}
			case *ocm.Protocol_TransferOptions:
				if err := storeTransferProtocol(tx, id, r); err != nil {
					return err
				}
			}
		}

		s.Id = &ocm.ShareId{OpaqueId: strconv.FormatInt(id, 10)}
		return nil
	}); err != nil {
		// check if the share already exists in the db
		// https://dev.mysql.com/doc/mysql-errors/8.0/en/server-error-reference.html#error_er_dup_unique
		var e *mysql.MySQLError
		if errors.As(err, &e) && e.Number == 1169 {
			return nil, share.ErrShareAlreadyExisting
		}
		return nil, err
	}

	return s, nil
}

// ListReceivedShares returns the list of shares the user has access.
func (m *mgr) ListReceivedShares(ctx context.Context, user *userpb.User) ([]*ocm.ReceivedShare, error) {
	query := "SELECT id, name, remote_share_id, item_type, share_with, owner, initiator, ctime, mtime, expiration, type, state FROM ocm_received_shares WHERE share_with=?"

	rows, err := m.db.QueryContext(ctx, query, user.Id.OpaqueId)
	if err != nil {
		return nil, err
	}

	var s dbReceivedShare
	shares := []*ocm.ReceivedShare{}
	var ids []any
	for rows.Next() {
		if err := rows.Scan(&s.ID, &s.Name, &s.RemoteShareID, &s.ItemType, &s.ShareWith, &s.Owner, &s.Initiator, &s.Ctime, &s.Mtime, &s.Expiration, &s.Type, &s.State); err != nil {
			continue
		}
		shares = append(shares, convertToCS3OCMReceivedShare(&s, nil))
		ids = append(ids, s.ID)
	}

	// resolve the protocols of all the received shares
	p, err := m.getProtocolsIds(ctx, ids)
	if err != nil {
		return nil, err
	}

	// join the result to get the shares with protocols
	for _, share := range shares {
		if protocols, ok := p[share.Id.OpaqueId]; ok {
			share.Protocols = protocols
		}
	}

	return shares, nil
}

func (m *mgr) getProtocolsIds(ctx context.Context, ids []any) (map[string][]*ocm.Protocol, error) {
	protocols := make(map[string][]*ocm.Protocol)
	if len(ids) == 0 {
		return protocols, nil
	}
	query := "SELECT p.ocm_received_share_id, p.type, dav.uri, dav.shared_secret, dav.permissions, app.uri_template, app.view_mode, tx.source_uri, tx.shared_secret, tx.size FROM ocm_received_share_protocols as p LEFT JOIN ocm_protocol_webdav as dav ON p.id=dav.ocm_protocol_id LEFT JOIN ocm_protocol_webapp as app ON p.id=app.ocm_protocol_id LEFT JOIN ocm_protocol_transfer as tx ON p.id=tx.ocm_protocol_id WHERE p.ocm_received_share_id IN "
	in := strings.Repeat("?,", len(ids))
	query += "(" + in[:len(in)-1] + ")"

	rows, err := m.db.QueryContext(ctx, query, ids...)
	if err != nil {
		return nil, err
	}

	var p dbProtocol
	for rows.Next() {
		if err := rows.Scan(&p.ShareID, &p.Type, &p.WebDAVURI, &p.WebDAVSharedSecret, &p.WebDavPermissions, &p.WebappURITemplate, &p.WebappViewMode, &p.TransferSourceURI, &p.TransferSharedSecret, &p.TransferSize); err != nil {
			continue
		}
		protocols[p.ShareID] = append(protocols[p.ShareID], convertToCS3Protocol(&p))
	}

	return protocols, nil
}

// GetReceivedShare returns the information for a received share the user has access.
func (m *mgr) GetReceivedShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*ocm.ReceivedShare, error) {
	var (
		s   *ocm.ReceivedShare
		err error
	)
	switch {
	case ref.GetId() != nil:
		s, err = m.getReceivedByID(ctx, user, ref.GetId())
	default:
		err = errtypes.NotFound(ref.String())
	}

	return s, err
}

func (m *mgr) getReceivedByID(ctx context.Context, user *userpb.User, id *ocm.ShareId) (*ocm.ReceivedShare, error) {
	query := "SELECT id, name, remote_share_id, item_type, share_with, owner, initiator, ctime, mtime, expiration, type, state FROM ocm_received_shares WHERE id=? AND share_with=?"
	params := []any{id.OpaqueId, user.Id.OpaqueId}

	var s dbReceivedShare
	if err := m.db.QueryRowContext(ctx, query, params...).Scan(&s.ID, &s.Name, &s.RemoteShareID, &s.ItemType, &s.ShareWith, &s.Owner, &s.Initiator, &s.Ctime, &s.Mtime, &s.Expiration, &s.Type, &s.State); err != nil {
		if err == sql.ErrNoRows {
			return nil, share.ErrShareNotFound
		}
		return nil, err
	}

	p, err := m.getProtocols(ctx, s.ID)
	if err != nil {
		return nil, err
	}

	return convertToCS3OCMReceivedShare(&s, p), nil
}

func (m *mgr) getProtocols(ctx context.Context, id int) ([]*ocm.Protocol, error) {
	query := "SELECT p.type, dav.uri, dav.shared_secret, dav.permissions, app.uri_template, app.view_mode, tx.source_uri, tx.shared_secret, tx.size FROM ocm_received_share_protocols as p LEFT JOIN ocm_protocol_webdav as dav ON p.id=dav.ocm_protocol_id LEFT JOIN ocm_protocol_webapp as app ON p.id=app.ocm_protocol_id LEFT JOIN ocm_protocol_transfer as tx ON p.id=tx.ocm_protocol_id WHERE p.ocm_received_share_id=?"

	var protocols []*ocm.Protocol
	rows, err := m.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}

	var p dbProtocol
	for rows.Next() {
		if err := rows.Scan(&p.Type, &p.WebDAVURI, &p.WebDAVSharedSecret, &p.WebDavPermissions, &p.WebappURITemplate, &p.WebappViewMode, &p.TransferSourceURI, &p.TransferSharedSecret, &p.TransferSize); err != nil {
			continue
		}
		protocols = append(protocols, convertToCS3Protocol(&p))
	}
	return protocols, nil
}

// UpdateReceivedShare updates the received share with share state.
func (m *mgr) UpdateReceivedShare(ctx context.Context, user *userpb.User, s *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*ocm.ReceivedShare, error) {
	query := "UPDATE ocm_received_shares SET"
	params := []any{}

	fquery, fparams, updatedShare, err := m.translateUpdateFieldMask(s, fieldMask)
	if err != nil {
		return nil, err
	}

	query = fmt.Sprintf("%s %s WHERE id=?", query, fquery)
	params = append(params, fparams...)
	params = append(params, s.Id.OpaqueId)

	res, err := m.db.ExecContext(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, share.ErrShareNotFound
	}
	return updatedShare, nil
}

func (m *mgr) translateUpdateFieldMask(share *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (string, []any, *ocm.ReceivedShare, error) {
	var (
		query  strings.Builder
		params []any
	)

	newShare := *share

	for _, mask := range fieldMask.Paths {
		switch mask {
		case "state":
			query.WriteString("state=?")
			params = append(params, convertFromCS3OCMShareState(share.State))
			newShare.State = share.State
		default:
			return "", nil, nil, errtypes.NotSupported("updating " + mask + " is not supported")
		}
		query.WriteString(",")
	}

	now := m.now().Unix()
	query.WriteString("mtime=?")
	params = append(params, now)
	newShare.Mtime = &typesv1beta1.Timestamp{
		Seconds: uint64(now),
	}

	return query.String(), params, &newShare, nil
}
