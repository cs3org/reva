// Copyright 2018-2026 CERN
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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	_ "github.com/go-sql-driver/mysql"
)

type mgr struct {
	c  *Config
	db *gorm.DB
}

func NewOCMShareManager(ctx context.Context, m map[string]any) (share.Repository, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Interface("config", m).Msg("creating OCM share manager")
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	db, err := getDb(c)
	if err != nil {
		log.Debug().Err(err).Msg("error getting db client")
		return nil, err
	}

	err = db.AutoMigrate(&model.OcmShare{}, &model.OcmShareProtocol{},
		&model.OcmReceivedShare{}, &model.OcmReceivedShareProtocol{})
	if err != nil {
		log.Debug().Err(err).Msg("error migrating database")
		return nil, err
	}

	mgr := &mgr{
		c:  &c,
		db: db,
	}
	return mgr, nil
}

func formatUserID(u *userpb.UserId) string {
	return fmt.Sprintf("%s@%s", u.OpaqueId, u.Idp)
}

// GenerateID reserves a unique ID for a share that is yet to be stored,
// so that the share can be referenced before being persisted with StoreShare.
func (m *mgr) GenerateID(ctx context.Context) (*ocm.ShareId, error) {
	id, err := createID(m.db)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create id for OCM share")
	}
	return &ocm.ShareId{OpaqueId: strconv.FormatUint(uint64(id), 10)}, nil
}

func (m *mgr) StoreShare(ctx context.Context, s *ocm.Share) (*ocm.Share, error) {
	parsed, err := strconv.ParseUint(s.Id.OpaqueId, 10, 64)
	if err != nil {
		return nil, errtypes.BadRequest("invalid share ID")
	}
	id := uint(parsed)
	err = m.db.Transaction(func(tx *gorm.DB) error {

		share := &model.OcmShare{
			Token:         s.Token,
			Instance:      s.ResourceId.StorageId,
			Inode:         s.ResourceId.OpaqueId,
			Name:          s.Name,
			ShareWith:     formatUserID(s.Grantee.GetUserId()),
			Owner:         s.Owner.OpaqueId,
			Initiator:     s.Creator.OpaqueId,
			Ctime:         s.Ctime.Seconds,
			Mtime:         s.Mtime.Seconds,
			RecipientType: convertFromCS3OCMShareType(s.RecipientType),
		}
		if s.Expiration != nil {
			share.Expiration = datatypes.NullTime{
				V:     time.Unix(int64(s.Expiration.Seconds), 0),
				Valid: true,
			}
		}
		share.Id = id
		share.ShareId = model.ShareID{ID: id}
		if err := tx.Create(share).Error; err != nil {
			return errors.Wrap(err, "failed to create OCM share")
		}
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
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func storeWebDAVAccessMethod(tx *gorm.DB, shareID uint, o *ocm.AccessMethod_WebdavOptions) error {
	accessMethod := &model.OcmShareProtocol{
		OcmShareID:   uint(shareID),
		Type:         model.WebDAVProtocol,
		Permissions:  int(permissions.OCSFromCS3Permission(o.WebdavOptions.Permissions)),
		AccessTypes:  accessTypesToInt(o.WebdavOptions.AccessTypes),
		Requirements: stringsToJSON(o.WebdavOptions.Requirements),
	}

	err := tx.Create(accessMethod).Error
	if err != nil {
		return errors.Wrap(err, "failed to store webdav access method")
	}
	return nil
}

func storeWebappAccessMethod(tx *gorm.DB, shareID uint, o *ocm.AccessMethod_WebappOptions) error {
	accessMethod := &model.OcmShareProtocol{
		OcmShareID:   uint(shareID),
		Type:         model.WebappProtocol,
		Permissions:  int(permissions.OCSFromCS3Permission(o.WebappOptions.GetPermissions())),
		Requirements: stringsToJSON(o.WebappOptions.Requirements),
		AppName:      o.WebappOptions.AppName,
	}

	err := tx.Create(accessMethod).Error
	if err != nil {
		return errors.Wrap(err, "failed to store webapp access method")
	}
	return nil
}

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

func (m *mgr) ListShares(ctx context.Context, user *userpb.User, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {
	query := m.db.WithContext(ctx).Where("initiator = ? OR owner = ?", user.Id.OpaqueId, user.Id.OpaqueId)

	if len(filters) > 0 {
		filterQuery, filterParams, err := translateShareFilters(filters)
		if err != nil {
			return nil, err
		}
		if filterQuery != "" {
			query = query.Where(filterQuery, filterParams...)
		}
	}

	var shareModels []model.OcmShare
	if err := query.Find(&shareModels).Error; err != nil {
		return nil, err
	}

	shares := []*ocm.Share{}
	var ids []any
	for _, s := range shareModels {
		if s.DeletedAt.Valid {
			continue
		}
		share := convertToCS3OCMShare(&s, nil)
		shares = append(shares, share)
		ids = append(ids, s.Id)
	}

	am, err := m.getAccessMethodsByIds(ctx, ids)
	if err != nil {
		return nil, err
	}

	for _, share := range shares {
		if methods, ok := am[share.Id.OpaqueId]; ok {
			share.AccessMethods = methods
		}
	}

	return shares, nil
}

// GetEmbeddedPayload returns the RO-Crate JSON payload carried by the embedded
// protocol of the received share, or an empty string if the share has no
// embedded protocol.
func (m *mgr) GetEmbeddedPayload(ctx context.Context, user *userpb.User, share *ocm.ReceivedShare) (string, error) {
	protocols, err := m.getProtocolsByIds(ctx, []any{share.Id.OpaqueId})
	if err != nil {
		return "", err
	}

	pList, ok := protocols[share.Id.OpaqueId]
	if !ok {
		return "", errtypes.NotFound("share not found")
	}

	for _, protocol := range pList {
		if embedded, ok := protocol.Term.(*ocm.Protocol_EmbeddedOptions); ok {
			return embedded.EmbeddedOptions.Payload, nil
		}
	}
	return "", nil
}

func (m *mgr) StoreReceivedShare(ctx context.Context, s *ocm.ReceivedShare) (*ocm.ReceivedShare, error) {
	if err := m.db.Transaction(func(tx *gorm.DB) error {

		receivedShare := &model.OcmReceivedShare{
			Name:          s.Name,
			RemoteShareID: s.RemoteShareId,
			ItemType:      convertFromCS3ResourceType(s.SharedResourceType),
			ShareWith:     s.Grantee.GetUserId().OpaqueId,
			Owner:         formatUserID(s.Owner),
			Initiator:     formatUserID(s.Creator),
			Ctime:         s.Ctime.Seconds,
			Mtime:         s.Mtime.Seconds,
			RecipientType: convertFromCS3OCMShareType(s.RecipientType),
			State:         convertFromCS3OCMShareState(s.State),
		}
		if s.Expiration != nil {
			receivedShare.Expiration = datatypes.NullTime{
				V:     time.Unix(int64(s.Expiration.Seconds), 0),
				Valid: true,
			}
		}

		id := tx.Create(receivedShare)
		err := id.Error
		if err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") {
				return share.ErrShareAlreadyExisting
			}
			return err
		}

		for _, p := range s.Protocols {
			switch r := p.Term.(type) {
			case *ocm.Protocol_WebdavOptions:
				if err := storeWebDAVProtocol(tx, int64(receivedShare.ID), r); err != nil {
					return err
				}
			case *ocm.Protocol_WebappOptions:
				if err := storeWebappProtocol(tx, int64(receivedShare.ID), r); err != nil {
					return err
				}
			case *ocm.Protocol_EmbeddedOptions:
				if err := storeEmbeddedProtocol(tx, int64(receivedShare.ID), r); err != nil {
					return err
				}
			}
		}

		s.Id = &ocm.ShareId{OpaqueId: fmt.Sprintf("%d", receivedShare.ID)}
		return nil
	}); err != nil {
		return nil, err
	}
	return s, nil
}

func storeWebDAVProtocol(tx *gorm.DB, shareID int64, o *ocm.Protocol_WebdavOptions) error {
	protocol := &model.OcmReceivedShareProtocol{
		OcmReceivedShareID: uint(shareID),
		Type:               model.WebDAVProtocol,
		Uri:                o.WebdavOptions.Uri,
		SharedSecret:       o.WebdavOptions.SharedSecret,
		Permissions:        int(permissions.OCSFromCS3Permission(o.WebdavOptions.Permissions.Permissions)),
		AccessTypes:        accessTypesToInt(o.WebdavOptions.AccessTypes),
		Requirements:       stringsToJSON(o.WebdavOptions.Requirements),
	}

	if err := tx.Create(protocol).Error; err != nil {
		return err
	}
	return nil
}

func storeWebappProtocol(tx *gorm.DB, shareID int64, o *ocm.Protocol_WebappOptions) error {
	protocol := &model.OcmReceivedShareProtocol{
		OcmReceivedShareID: uint(shareID),
		Type:               model.WebappProtocol,
		Uri:                o.WebappOptions.Uri,
		SharedSecret:       o.WebappOptions.SharedSecret,
		Permissions:        int(permissions.OCSFromCS3Permission(o.WebappOptions.GetSharePermissions())),
		Requirements:       stringsToJSON(o.WebappOptions.Requirements),
		Targets:            stringsToJSON(o.WebappOptions.Targets),
		AppName:            o.WebappOptions.AppName,
		AppIconHint:        o.WebappOptions.AppIconHint,
		MediaTypes:         stringsToJSON(o.WebappOptions.MediaTypes),
	}

	if err := tx.Create(protocol).Error; err != nil {
		return err
	}
	return nil
}
func storeEmbeddedProtocol(tx *gorm.DB, shareID int64, o *ocm.Protocol_EmbeddedOptions) error {
	protocol := &model.OcmReceivedShareProtocol{
		OcmReceivedShareID: uint(shareID),
		Type:               model.EmbeddedProtocol,
		Payload:            datatypes.JSON([]byte(o.EmbeddedOptions.Payload)),
	}
	if err := tx.Create(protocol).Error; err != nil {
		return err
	}
	return nil
}

func stringsToJSON(reqs []string) datatypes.JSON {
	if len(reqs) == 0 {
		return nil
	}
	b, err := json.Marshal(reqs)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}

func accessTypesToInt(at []ocm.AccessType) model.OcmAccessType {
	var bitmask model.OcmAccessType
	for _, t := range at {
		bitmask |= model.OcmAccessType(t)
	}
	return bitmask
}

func (m *mgr) ListReceivedShares(ctx context.Context, user *userpb.User, filters []*ocm.ListReceivedOCMSharesRequest_Filter) ([]*ocm.ReceivedShare, error) {
	query := m.db.WithContext(ctx).Where("share_with = ?", user.Id.OpaqueId)

	if len(filters) > 0 {
		filterQuery, filterParams, err := translateReceivedShareFilters(filters)
		if err != nil {
			return nil, err
		}
		if filterQuery != "" {
			query = query.Where(filterQuery, filterParams...)
		}
	}

	var receivedShareModels []model.OcmReceivedShare
	if err := query.Find(&receivedShareModels).Error; err != nil {
		return nil, err
	}
	shares := []*ocm.ReceivedShare{}
	var ids []any
	for _, s := range receivedShareModels {
		share := convertToCS3OCMReceivedShare(&s, nil)
		shares = append(shares, share)
		ids = append(ids, s.ID)
	}
	p, err := m.getProtocolsByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, share := range shares {
		if protocols, ok := p[share.Id.OpaqueId]; ok {
			share.Protocols = protocols
		}
	}

	return shares, nil
}

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

func (m *mgr) UpdateReceivedShare(ctx context.Context, user *userpb.User, s *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*ocm.ReceivedShare, error) {
	shareID, err := strconv.Atoi(s.Id.OpaqueId)
	if err != nil {
		return nil, errtypes.BadRequest("invalid share ID")
	}

	updates, updatedShare, err := m.translateUpdateFieldMask(s, fieldMask)
	if err != nil {
		return nil, err
	}

	result := m.db.WithContext(ctx).
		Model(&model.OcmReceivedShare{}).
		Where("id = ? AND share_with = ?", shareID, user.Id.OpaqueId).
		Updates(updates)

	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, share.ErrShareNotFound
	}

	return updatedShare, nil
}

func (m *mgr) translateUpdateFieldMask(share *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (map[string]any, *ocm.ReceivedShare, error) {
	updates := make(map[string]any)
	newShare := proto.Clone(share).(*ocm.ReceivedShare)

	for _, mask := range fieldMask.Paths {
		switch mask {
		case "state":
			updates["state"] = convertFromCS3OCMShareState(share.State)
			newShare.State = share.State
		case "hidden":
			// The share state tracks the embedded transfer lifecycle, so the
			// hidden flag is carried by the dedicated Hidden field instead.
			updates["hidden"] = share.Hidden
			newShare.Hidden = share.Hidden
		default:
			return nil, nil, errtypes.NotSupported("updating " + mask + " is not supported")
		}
	}

	now := time.Now().Unix()
	updates["mtime"] = now
	newShare.Mtime = &typesv1beta1.Timestamp{
		Seconds: uint64(now),
	}

	return updates, newShare, nil
}

func (m *mgr) getByID(ctx context.Context, user *userpb.User, id *ocm.ShareId) (*ocm.Share, error) {
	shareID, err := strconv.Atoi(id.OpaqueId)
	if err != nil {
		return nil, errtypes.BadRequest("invalid share ID")
	}

	shareWith := formatUserID(user.Id)

	var shareModel model.OcmShare
	if err := m.db.WithContext(ctx).
		Where("id = ? AND (initiator = ? OR owner = ? OR share_with = ?)", shareID, user.Id.OpaqueId, user.Id.OpaqueId, shareWith).
		First(&shareModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, share.ErrShareNotFound
		}
		return nil, err
	}

	am, err := m.getAccessMethods(ctx, int(shareModel.Id))
	if err != nil {
		return nil, err
	}

	if shareModel.DeletedAt.Valid {
		return nil, share.ErrShareNotFound
	}

	return convertToCS3OCMShare(&shareModel, am), nil

}

func (m *mgr) getByKey(ctx context.Context, user *userpb.User, key *ocm.ShareKey) (*ocm.Share, error) {
	var shareModel model.OcmShare
	if err := m.db.WithContext(ctx).
		Where("owner = ? AND instance = ? AND inode = ? AND share_with = ? AND (initiator = ? OR owner = ?)",
			key.Owner.OpaqueId, key.ResourceId.StorageId, key.ResourceId.OpaqueId, formatUserID(key.Grantee.GetUserId()), user.Id.OpaqueId, user.Id.OpaqueId).
		First(&shareModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, share.ErrShareNotFound
		}
		return nil, err
	}

	am, err := m.getAccessMethods(ctx, int(shareModel.Id))
	if err != nil {
		return nil, err
	}

	if shareModel.DeletedAt.Valid {
		return nil, share.ErrShareNotFound
	}

	return convertToCS3OCMShare(&shareModel, am), nil
}

func (m *mgr) getByToken(ctx context.Context, token string) (*ocm.Share, error) {
	var shareModel model.OcmShare
	if err := m.db.WithContext(ctx).
		Where("token = ?", token).
		First(&shareModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, share.ErrShareNotFound
		}
		return nil, err
	}

	am, err := m.getAccessMethods(ctx, int(shareModel.Id))
	if err != nil {
		return nil, err
	}

	if shareModel.DeletedAt.Valid {
		return nil, share.ErrShareNotFound
	}

	return convertToCS3OCMShare(&shareModel, am), nil
}

func (m *mgr) getAccessMethods(ctx context.Context, id int) ([]*ocm.AccessMethod, error) {
	var modelAMs []model.OcmShareProtocol
	if err := m.db.WithContext(ctx).
		Where("ocm_share_id = ?", id).
		Find(&modelAMs).Error; err != nil {
		return nil, err
	}

	var methods []*ocm.AccessMethod
	for _, am := range modelAMs {
		methods = append(methods, convertToCS3AccessMethod(&am))
	}

	return methods, nil
}

func (m *mgr) deleteByID(ctx context.Context, user *userpb.User, id *ocm.ShareId) error {
	shareID, err := strconv.Atoi(id.OpaqueId)
	if err != nil {
		return errtypes.BadRequest("invalid share ID")
	}

	result := m.db.WithContext(ctx).
		Where("id = ? AND (owner = ? OR initiator = ?)", shareID, user.Id.OpaqueId, user.Id.OpaqueId).
		Delete(&model.OcmShare{})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return share.ErrShareNotFound
	}

	return nil
}

func (m *mgr) deleteByKey(ctx context.Context, user *userpb.User, key *ocm.ShareKey) error {
	result := m.db.WithContext(ctx).
		Where("owner = ? AND instance = ? AND inode = ? AND share_with = ? AND (initiator = ? OR owner = ?)",
			key.Owner.OpaqueId, key.ResourceId.StorageId, key.ResourceId.OpaqueId, formatUserID(key.Grantee.GetUserId()), user.Id.OpaqueId, user.Id.OpaqueId).
		Delete(&model.OcmShare{})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return share.ErrShareNotFound
	}

	return nil
}

func (m *mgr) queriesUpdatesOnShare(ctx context.Context, id *ocm.ShareId, f ...*ocm.UpdateOCMShareRequest_UpdateField) (map[string]any, []func(*gorm.DB) error, error) {
	var updates map[string]any
	var accessMethodUpdates []func(*gorm.DB) error

	for _, field := range f {
		switch u := field.Field.(type) {
		case *ocm.UpdateOCMShareRequest_UpdateField_Expiration:
			if updates == nil {
				updates = make(map[string]any)
			}
			updates["expiration"] = u.Expiration.Seconds
		case *ocm.UpdateOCMShareRequest_UpdateField_AccessMethods:
			switch t := u.AccessMethods.Term.(type) {
			case *ocm.AccessMethod_WebdavOptions:
				accessMethodUpdates = append(accessMethodUpdates, func(tx *gorm.DB) error {
					return tx.Model(&model.OcmShareProtocol{}).
						Where("ocm_share_id = ? AND type = ?", id.OpaqueId, model.WebDAVProtocol).
						Update("permissions", int(permissions.RoleFromResourcePermissions(t.WebdavOptions.Permissions).OCSPermissions())).Error
				})
			case *ocm.AccessMethod_WebappOptions:
				accessMethodUpdates = append(accessMethodUpdates, func(tx *gorm.DB) error {
					// omitted fields mean "preserve stored value", as for webdav
					u := map[string]any{
						"permissions": int(permissions.RoleFromResourcePermissions(t.WebappOptions.GetPermissions()).OCSPermissions()),
					}
					if t.WebappOptions.AppName != "" {
						u["app_name"] = t.WebappOptions.AppName
					}
					return tx.Model(&model.OcmShareProtocol{}).
						Where("ocm_share_id = ? AND type = ?", id.OpaqueId, model.WebappProtocol).
						Updates(u).Error
				})
			}
		}
	}

	return updates, accessMethodUpdates, nil
}

func (m *mgr) updateShareByID(ctx context.Context, user *userpb.User, id *ocm.ShareId, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*ocm.Share, error) {
	shareID, err := strconv.Atoi(id.OpaqueId)
	if err != nil {
		return nil, errtypes.BadRequest("invalid share ID")
	}

	currentMethods, err := m.getAccessMethods(ctx, shareID)
	if err != nil {
		return nil, err
	}

	if err := validateImmutableFields(currentMethods, f...); err != nil {
		return nil, err
	}

	updates, accessMethodUpdates, err := m.queriesUpdatesOnShare(ctx, id, f...)
	if err != nil {
		return nil, err
	}

	if updates == nil {
		updates = make(map[string]any)
	}

	now := time.Now().Unix()
	updates["mtime"] = now

	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.OcmShare{}).
			Where("id = ? AND (initiator = ? OR owner = ?)", shareID, user.Id.OpaqueId, user.Id.OpaqueId).
			Updates(updates)

		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return share.ErrShareNotFound
		}

		for _, updateFunc := range accessMethodUpdates {
			if err := updateFunc(tx); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return m.getByID(ctx, user, id)
}

// validateImmutableFields checks that a partial update does not attempt to
// change Requirements or AccessTypes, which are immutable after creation.
// Omitted or empty slices mean "preserve stored value" and are allowed.
// Identical non-empty slices are a no-op. Different non-empty slices are rejected.
func validateImmutableFields(currentMethods []*ocm.AccessMethod, f ...*ocm.UpdateOCMShareRequest_UpdateField) error {
	for _, field := range f {
		u, ok := field.Field.(*ocm.UpdateOCMShareRequest_UpdateField_AccessMethods)
		if !ok {
			continue
		}
		switch t := u.AccessMethods.Term.(type) {
		case *ocm.AccessMethod_WebdavOptions:
			var storedReqs []string
			var storedATs []ocm.AccessType
			for _, m := range currentMethods {
				if existing, ok := m.Term.(*ocm.AccessMethod_WebdavOptions); ok {
					storedReqs = existing.WebdavOptions.Requirements
					storedATs = existing.WebdavOptions.AccessTypes
					break
				}
			}

			if err := checkImmutableStringSlice(storedReqs, t.WebdavOptions.Requirements, "requirements"); err != nil {
				return err
			}
			if err := checkImmutableAccessTypes(storedATs, t.WebdavOptions.AccessTypes, "access_types"); err != nil {
				return err
			}
		case *ocm.AccessMethod_WebappOptions:
			var storedReqs []string
			for _, m := range currentMethods {
				if existing, ok := m.Term.(*ocm.AccessMethod_WebappOptions); ok {
					storedReqs = existing.WebappOptions.Requirements
					break
				}
			}

			if err := checkImmutableStringSlice(storedReqs, t.WebappOptions.Requirements, "requirements"); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkImmutableStringSlice(stored, incoming []string, field string) error {
	if len(incoming) == 0 {
		return nil
	}
	if len(stored) == 0 {
		return errtypes.BadRequest(field + " cannot be set on update; they are immutable after creation")
	}
	if len(stored) != len(incoming) {
		return errtypes.BadRequest(field + " are immutable after creation")
	}
	for i := range stored {
		if stored[i] != incoming[i] {
			return errtypes.BadRequest(field + " are immutable after creation")
		}
	}
	return nil
}

func checkImmutableAccessTypes(stored, incoming []ocm.AccessType, field string) error {
	if len(incoming) == 0 {
		return nil
	}
	if len(stored) == 0 {
		return errtypes.BadRequest(field + " cannot be set on update; they are immutable after creation")
	}
	if len(stored) != len(incoming) {
		return errtypes.BadRequest(field + " are immutable after creation")
	}
	for i := range stored {
		if stored[i] != incoming[i] {
			return errtypes.BadRequest(field + " are immutable after creation")
		}
	}
	return nil
}

func (m *mgr) updateShareByKey(ctx context.Context, user *userpb.User, key *ocm.ShareKey, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*ocm.Share, error) {
	share, err := m.getByKey(ctx, user, key)
	if err != nil {
		return nil, err
	}
	return m.updateShareByID(ctx, user, share.Id, f...)
}

func translateShareFilters(filters []*ocm.ListOCMSharesRequest_Filter) (string, []any, error) {
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
				filterQuery.WriteString("instance = ? AND inode = ?")
				params = append(params, filter.ResourceId.StorageId, filter.ResourceId.OpaqueId)
			case *ocm.ListOCMSharesRequest_Filter_Creator:
				filterQuery.WriteString("initiator = ?")
				params = append(params, filter.Creator.OpaqueId)
			case *ocm.ListOCMSharesRequest_Filter_Owner:
				filterQuery.WriteString("owner= ? ")
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

func translateReceivedShareFilters(filters []*ocm.ListReceivedOCMSharesRequest_Filter) (string, []any, error) {
	var (
		filterQuery strings.Builder
		params      []any
	)

	grouped := groupReceivedFiltersByType(filters)

	var count int
	for _, lst := range grouped {
		for n, f := range lst {
			switch filter := f.Term.(type) {
			case *ocm.ListReceivedOCMSharesRequest_Filter_SharedResourceType:
				filterQuery.WriteString("item_type = ?")
				params = append(params, translateSharedResourceTypeToItemType(filter.SharedResourceType))
			case *ocm.ListReceivedOCMSharesRequest_Filter_Creator:
				filterQuery.WriteString("initiator = ?")
				params = append(params, filter.Creator.OpaqueId)
			case *ocm.ListReceivedOCMSharesRequest_Filter_Owner:
				filterQuery.WriteString("owner = ?")
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

func translateSharedResourceTypeToItemType(t ocm.SharedResourceType) model.ItemType {
	switch t {
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE:
		return model.ItemTypeFile
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER:
		return model.ItemTypeFolder
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED:
		return model.ItemTypeEmbedded
	default:
		return model.ItemTypeFile
	}
}

func groupFiltersByType(filters []*ocm.ListOCMSharesRequest_Filter) map[ocm.ListOCMSharesRequest_Filter_Type][]*ocm.ListOCMSharesRequest_Filter {
	m := make(map[ocm.ListOCMSharesRequest_Filter_Type][]*ocm.ListOCMSharesRequest_Filter)
	for _, f := range filters {
		m[f.Type] = append(m[f.Type], f)
	}
	return m
}

func groupReceivedFiltersByType(filters []*ocm.ListReceivedOCMSharesRequest_Filter) map[ocm.ListReceivedOCMSharesRequest_Filter_Type][]*ocm.ListReceivedOCMSharesRequest_Filter {
	m := make(map[ocm.ListReceivedOCMSharesRequest_Filter_Type][]*ocm.ListReceivedOCMSharesRequest_Filter)
	for _, f := range filters {
		m[f.Type] = append(m[f.Type], f)
	}
	return m
}
func (m *mgr) getAccessMethodsByIds(ctx context.Context, ids []any) (map[string][]*ocm.AccessMethod, error) {
	methods := make(map[string][]*ocm.AccessMethod)
	if len(ids) == 0 {
		return methods, nil
	}

	var mProtos []model.OcmShareProtocol
	if err := m.db.WithContext(ctx).
		Where("ocm_share_id IN ?", ids).
		Find(&mProtos).Error; err != nil {
		return nil, err
	}

	for _, p := range mProtos {
		method := convertToCS3AccessMethod(&p)
		shareID := strconv.FormatUint(uint64(p.OcmShareID), 10)
		methods[shareID] = append(methods[shareID], method)
	}

	return methods, nil
}

func (m *mgr) getProtocolsByIds(ctx context.Context, ids []any) (map[string][]*ocm.Protocol, error) {
	protocols := make(map[string][]*ocm.Protocol)
	if len(ids) == 0 {
		return protocols, nil
	}

	var mrProtos []model.OcmReceivedShareProtocol
	if err := m.db.WithContext(ctx).
		Where("ocm_received_share_id IN ?", ids).
		Find(&mrProtos).Error; err != nil {
		return nil, err
	}

	for _, p := range mrProtos {
		protocol := convertToCS3Protocol(&p)
		shareID := strconv.FormatUint(uint64(p.OcmReceivedShareID), 10)
		protocols[shareID] = append(protocols[shareID], protocol)
	}

	return protocols, nil
}

func (m *mgr) getReceivedByID(ctx context.Context, user *userpb.User, id *ocm.ShareId) (*ocm.ReceivedShare, error) {
	shareID, err := strconv.Atoi(id.OpaqueId)

	if err != nil {
		return nil, errtypes.BadRequest("invalid share ID")
	}

	var receivedShareModel model.OcmReceivedShare
	if err := m.db.WithContext(ctx).
		Where("id = ? AND share_with = ?", shareID, user.Id.OpaqueId).
		First(&receivedShareModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, share.ErrShareNotFound
		}
		return nil, err
	}
	p, err := m.getProtocols(ctx, int(receivedShareModel.ID))
	if err != nil {
		return nil, err
	}

	return convertToCS3OCMReceivedShare(&receivedShareModel, p), nil
}

func (m *mgr) getProtocols(ctx context.Context, id int) ([]*ocm.Protocol, error) {
	var protocolModels []model.OcmReceivedShareProtocol
	if err := m.db.WithContext(ctx).
		Where("ocm_received_share_id = ?", id).
		Find(&protocolModels).Error; err != nil {
		return nil, err
	}

	var protocols []*ocm.Protocol
	for _, p := range protocolModels {
		protocols = append(protocols, convertToCS3Protocol(&p))
	}

	return protocols, nil
}
