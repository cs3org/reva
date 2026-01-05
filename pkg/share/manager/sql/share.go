// Copyright 2018-2025 CERN
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
	"slices"
	"strconv"
	"strings"
	"time"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	conversions "github.com/cs3org/reva/v3/pkg/cbox/utils"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	revashare "github.com/cs3org/reva/v3/pkg/share"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/genproto/protobuf/field_mask"

	"gorm.io/gorm"

	// Provides mysql drivers.
	_ "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

type ShareMgr struct {
	c  *Config
	db *gorm.DB
}

func NewShareManager(ctx context.Context, m map[string]any) (revashare.Manager, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	c.ApplyDefaults()

	db, err := getDb(c)
	if err != nil {
		return nil, err
	}

	// Migrate schemas
	err = db.AutoMigrate(&model.ShareID{}, &model.Share{}, &model.ShareState{})
	if err != nil {
		return nil, err
	}

	mgr := &ShareMgr{
		c:  &c,
		db: db,
	}
	return mgr, nil
}

func (m *ShareMgr) Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error) {
	user := appctx.ContextMustGetUser(ctx)

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
	_, err := m.getShareByKey(ctx, key, true, true)
	// share already exists
	// TODO stricter error checking
	if err == nil {
		return nil, errors.New(errtypes.AlreadyExists(key.String()).Error())
	}

	var shareWith string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER {
		shareWith = conversions.FormatUserID(g.Grantee.GetUserId())
	} else if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		// ShareWith is a group
		shareWith = g.Grantee.GetGroupId().OpaqueId
	} else {
		return nil, errors.New("Unsuppored grantee type passed to Share()")
	}

	share := &model.Share{
		ShareWith:         shareWith,
		SharedWithIsGroup: g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP,
	}

	// Create Shared ID
	id, err := createID(m.db)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create id for PublicShare")
	}

	share.Id = id
	share.ShareId = model.ShareID{ID: id}
	share.UIDOwner = conversions.FormatUserID(md.Owner)
	share.UIDInitiator = conversions.FormatUserID(user.Id)
	share.InitialPath = md.Path
	share.ItemType = model.ItemType(conversions.ResourceTypeToItem(md.Type))
	share.Inode = md.Id.OpaqueId
	share.Instance = md.Id.StorageId
	share.Permissions = uint8(permissions.OCSFromCS3Permission(g.Permissions.Permissions))
	share.Orphan = false

	if g.Expiration != nil {
		share.Expiration.Scan(time.Unix(int64(g.Expiration.Seconds), 0))
	}

	res := m.db.Save(&share)
	if res.Error != nil {
		return nil, res.Error
	}

	granteeType, _ := m.getUserType(ctx, share.ShareWith)
	return share.AsCS3Share(granteeType), nil
}

func (m *ShareMgr) GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error) {
	share, err := m.getShare(ctx, ref, true, true)
	if err != nil {
		return nil, err
	}

	granteeType, _ := m.getUserType(ctx, share.ShareWith)
	cs3share := share.AsCS3Share(granteeType)

	return cs3share, nil
}

func (m *ShareMgr) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
	share, err := m.getEmptyShareByRef(ctx, ref)
	if err != nil {
		return err
	}
	res := m.db.Where("id = ?", share.Id).Delete(&share)
	return res.Error
}

func (m *ShareMgr) UpdateShare(ctx context.Context, ref *collaboration.ShareReference, req *collaboration.UpdateShareRequest) (*collaboration.Share, error) {
	share, err := m.getEmptyShareByRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	switch req.Field.GetField().(type) {
	case *collaboration.UpdateShareRequest_UpdateField_Permissions:
		perms := uint8(permissions.OCSFromCS3Permission(req.Field.GetPermissions().Permissions))
		res := m.db.Model(&share).Where("id = ?", share.Id).Update("permissions", perms)
		if res.Error != nil {
			return nil, res.Error
		}
	case *collaboration.UpdateShareRequest_UpdateField_DisplayName:
		// Our shares don't support display names at the moment ...
	case *collaboration.UpdateShareRequest_UpdateField_Expiration:
		expiration := req.Field.GetExpiration()
		if expiration == nil {
			res := m.db.Model(&share).Where("id = ?", share.Id).Update("expiration", nil)
			if res.Error != nil {
				return nil, res.Error
			}
		} else {
			res := m.db.Model(&share).Where("id = ?", share.Id).Update("expiration", time.Unix(int64(expiration.Seconds), 0))
			if res.Error != nil {
				return nil, res.Error
			}
		}

	}

	return m.GetShare(ctx, ref)
}

func (m *ShareMgr) ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error) {
	shares, err := m.ListModelShares(nil, filters, false)
	if err != nil {
		return nil, err
	}

	var cs3shares []*collaboration.Share
	for _, s := range shares {
		granteeType, _ := m.getUserType(ctx, s.ShareWith)
		cs3share := s.AsCS3Share(granteeType)
		cs3shares = append(cs3shares, cs3share)
	}

	return cs3shares, nil
}

// we list the shares that are targeted to the user in context or to the user groups.
func (m *ShareMgr) ListReceivedShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.ReceivedShare, error) {
	user := appctx.ContextMustGetUser(ctx)

	// We need to do this to parse the result
	// Normally, GORM would be able to fill in the Share that is referenced in ShareState
	// However, in GORM's docs: "Join Preload will loads association data using left join"
	// Because we do a RIGHT JOIN, GORM cannot load the data into shareState.Share (in case that ShareState is empty)
	// So we load them both separately, and then set ShareState.Share = Share ourselves
	var results []struct {
		model.ShareState
		model.Share
	}

	query := m.db.Model(&model.ShareState{}).
		Select("share_states.*, shares.*").
		Joins("RIGHT OUTER JOIN shares ON shares.id = share_states.share_id and share_states.user = ?", user.Username).
		Where("shares.orphan = ?", false).
		Where("shares.deleted_at IS NULL")

	// Also search by all the groups the user is a member of
	innerQuery := m.db.Where("shares.share_with = ? and shares.shared_with_is_group = ?", user.Username, false)
	for _, group := range user.Groups {
		innerQuery = innerQuery.Or("shares.share_with = ? and shares.shared_with_is_group = ?", group, true)
	}
	query = query.Where(innerQuery)

	// Append filters
	m.appendShareFiltersToQuery(query, filters)

	// Get the shares + states
	res := query.Find(&results)
	if res.Error != nil {
		return nil, res.Error
	}

	var receivedShares []*collaboration.ReceivedShare

	// Now we parse everything into the CS3 definition of a CS3ReceivedShare
	for _, res := range results {
		shareState := res.ShareState
		shareState.Share = res.Share
		granteeType, _ := m.getUserType(ctx, res.Share.ShareWith)

		receivedShares = append(receivedShares, res.Share.AsCS3ReceivedShare(&shareState, granteeType))
	}

	return receivedShares, nil
}

func (m *ShareMgr) GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	var s *collaboration.ReceivedShare
	var err error
	switch {
	case ref.GetId() != nil:
		s, err = m.getReceivedByID(ctx, ref.GetId(), userpb.UserType_USER_TYPE_INVALID)
	case ref.GetKey() != nil:
		s, err = m.getReceivedByKey(ctx, ref.GetKey(), userpb.UserType_USER_TYPE_INVALID)
	default:
		err = errtypes.NotFound(ref.String())
	}

	if err != nil {
		return nil, err
	}

	// resolve grantee's user type if applicable
	if s.Share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER {
		s.Share.Grantee.GetUserId().Type, _ = m.getUserType(ctx, s.Share.Grantee.GetUserId().OpaqueId)
	}

	return s, nil
}

func (m *ShareMgr) UpdateReceivedShare(ctx context.Context, recvShare *collaboration.ReceivedShare, fieldMask *field_mask.FieldMask) (*collaboration.ReceivedShare, error) {

	user := appctx.ContextMustGetUser(ctx)

	rs, err := m.getReceivedByID(ctx, recvShare.Share.Id, user.Id.Type)
	if err != nil {
		return nil, err
	}

	share, err := emptyShareWithId(recvShare.Share.Id.OpaqueId)
	if err != nil {
		return nil, err
	}

	shareState, err := m.getShareState(ctx, share, user)
	if err != nil {
		return nil, err
	}

	// FieldMask determines which parts of the share we actually update
	for _, path := range fieldMask.Paths {
		switch path {
		case "state":
			rs.State = recvShare.State
			switch rs.State {
			case collaboration.ShareState_SHARE_STATE_ACCEPTED:
				shareState.Hidden = false
			case collaboration.ShareState_SHARE_STATE_REJECTED:
				shareState.Hidden = true
			}
		case "hidden":
			rs.Hidden = recvShare.Hidden
		default:
			return nil, errtypes.NotSupported("updating " + path + " is not supported")
		}
	}

	// Now we do the actual update to the db model

	res := m.db.Save(&shareState)
	if res.Error != nil {
		return nil, res.Error
	}

	return rs, nil
}

// Exported functions below are not part of the CS3-defined API, but are used by cernboxcop

// Used by cernboxcop, to include listings with orphans (which cannot be represented in the CS3 Shares)
func (m *ShareMgr) ListModelShares(u *user.User, filters []*collaboration.Filter, remove_orphan bool) ([]model.Share, error) {
	query := m.db.Model(&model.Share{})
	if remove_orphan {
		query = query.Where("orphan = ?", false)
	}

	if u != nil {
		uid := conversions.FormatUserID(u.Id)
		query = query.Where("uid_owner = ? or uid_initiator = ?", uid, uid)
	}

	// Append filters
	m.appendShareFiltersToQuery(query, filters)

	var shares []model.Share
	res := query.Find(&shares)
	if res.Error != nil {
		return nil, res.Error
	}
	return shares, nil
}

func (m *ShareMgr) GetShareUnfiltered(ctx context.Context, ref *collaboration.ShareReference) (*model.Share, error) {
	share, err := m.getShare(ctx, ref, false, false)
	if err != nil {
		return nil, err
	}

	return share, nil
}

func (m *ShareMgr) GetSharesByShareWith(ctx context.Context, shareWith string) ([]model.Share, error) {
	query := m.db.Model(&model.Share{}).
		Where("orphan = ?", false).
		Where("share_with = ?", shareWith)

	var shares []model.Share
	res := query.Find(&shares)
	if res.Error != nil {
		return nil, res.Error
	}

	return shares, nil
}

// TransferShare transfers a share to a new initiator. Only to be used for shares in projects.
func (m *ShareMgr) TransferShare(ctx context.Context, ref *collaboration.ShareReference, newInitiator string) error {
	if newInitiator == "" {
		return errors.New("Must pass a non-nil initiator")
	}

	share, err := m.getEmptyShareByRef(ctx, ref)
	if err != nil {
		return err
	}

	res := m.db.Model(&share).Update("uid_initiator", newInitiator)
	if res.Error != nil {
		return res.Error
	}
	return nil
}

func (m *ShareMgr) MarkAsOrphaned(ctx context.Context, ref *collaboration.ShareReference) error {
	share, err := m.getEmptyShareByRef(ctx, ref)
	if err != nil {
		return err
	}

	res := m.db.Model(&share).Where("id = ?", share.Id).Update("orphan", true)
	return res.Error
}

// Move share moves a share to a new location, also updating its owner. It is the reponsibility of the caller to ensure that `newOwner`
// corresponds to the owner of `newLocation`

func (m *ShareMgr) MoveShare(ctx context.Context, ref *collaboration.ShareReference, newLocation *provider.ResourceId, newOwner string) error {
	if newOwner == "" {
		return errors.New("Must pass a non-nil owner")
	}

	if newLocation.OpaqueId == "" || newLocation.StorageId == "" {
		return errors.New("Must pass a non-nil location")
	}

	share, err := m.getEmptyShareByRef(ctx, ref)
	if err != nil {
		return err
	}

	res := m.db.Model(&share).Update("uid_owner", newOwner).Update("inode", newLocation.OpaqueId).Update("instance", newLocation.StorageId)
	if res.Error != nil {
		return res.Error
	}

	return nil
}

func (m *ShareMgr) getPath(ctx context.Context, resID *provider.ResourceId) (string, error) {
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(m.c.GatewaySvc))
	if err != nil {
		return "", err
	}

	res, err := client.GetPath(ctx, &provider.GetPathRequest{
		ResourceId: resID,
	})

	if err != nil {
		return "", err
	}

	if res.Status.Code == rpc.Code_CODE_OK {
		return res.GetPath(), nil
	} else if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
		return "", errtypes.NotFound(resID.OpaqueId)
	}
	return "", errors.New(res.Status.Code.String() + ": " + res.Status.Message)
}

func (m *ShareMgr) getShare(ctx context.Context, ref *collaboration.ShareReference, filter bool, verifyOwner bool) (*model.Share, error) {
	var s *model.Share
	var err error
	switch {
	case ref.GetId() != nil:
		s, err = m.getShareByID(ctx, ref.GetId(), filter)
	case ref.GetKey() != nil:
		s, err = m.getShareByKey(ctx, ref.GetKey(), false, filter)
	default:
		return nil, errtypes.NotFound(ref.String())
	}
	if err != nil {
		return nil, err
	}

	if !verifyOwner {
		return s, nil
	}

	user := appctx.ContextMustGetUser(ctx)
	if s.UIDOwner == user.Id.OpaqueId && s.UIDInitiator == user.Id.OpaqueId {
		return s, nil
	}

	path, err := m.getPath(ctx, &provider.ResourceId{
		StorageId: s.Instance,
		OpaqueId:  s.Inode,
	})
	if err != nil {
		return nil, err
	}

	if m.isProjectAdmin(user, path) {
		return s, nil
	}

	return nil, errtypes.NotFound(ref.String())
}

// Get Share by ID. Does not return orphans if filter is set to true.
func (m *ShareMgr) getShareByID(_ context.Context, id *collaboration.ShareId, filter bool) (*model.Share, error) {
	var share model.Share
	res := m.db.First(&share, id.OpaqueId)

	if res.RowsAffected == 0 {
		return nil, errtypes.NotFound(id.OpaqueId)
	}

	if filter && share.Orphan {
		return nil, errtypes.NotFound(id.OpaqueId)
	}

	return &share, nil
}

// Get Share by Key. Does not return orphans if filter is set to true.
func (m *ShareMgr) getShareByKey(ctx context.Context, key *collaboration.ShareKey, checkOwner bool, filter bool) (*model.Share, error) {
	owner := conversions.FormatUserID(key.Owner)

	var share model.Share
	_, shareWith := conversions.FormatGrantee(key.Grantee)

	query := m.db.Model(&share).
		Where("uid_owner = ?", owner).
		Where("instance = ?", key.ResourceId.StorageId).
		Where("inode = ?", key.ResourceId.OpaqueId).
		Where("shared_with_is_group = ?", key.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP).
		Where("share_with = ?", strings.ToLower(shareWith))

	uid := conversions.FormatUserID(appctx.ContextMustGetUser(ctx).Id)
	// In case the user is not the owner (i.e. in the case of projects)
	if checkOwner && owner != uid {
		query = query.Where("uid_initiator = ?", uid)
	}

	res := query.First(&share)

	if res.RowsAffected == 0 {
		return nil, errtypes.NotFound(key.String())
	}

	if filter && share.Orphan {
		return nil, errtypes.NotFound(key.String())
	}

	return &share, nil
}

func (m *ShareMgr) isProjectAdmin(u *userpb.User, path string) bool {
	if strings.HasPrefix(path, projectPathPrefix) {
		// The path will look like /eos/project/c/cernbox, we need to extract the project name
		parts := strings.SplitN(path, "/", 6)
		if len(parts) < 5 {
			return false
		}

		adminGroup := projectSpaceGroupsPrefix + parts[4] + projectSpaceAdminGroupsSuffix
		return slices.Contains(u.Groups, adminGroup)
	}
	return false
}

func (m *ShareMgr) getShareState(ctx context.Context, share *model.Share, user *userpb.User) (*model.ShareState, error) {
	var shareState model.ShareState
	query := m.db.Model(&shareState).
		Where("share_id = ?", share.Id).
		Where("user = ?", user.Username)

	res := query.First(&shareState)

	if res.RowsAffected == 0 {
		// If no share state has been created yet, we create it now using these defaults
		shareState = model.ShareState{
			Share:  *share,
			Hidden: false,
			Synced: false,
			User:   user.Username,
		}
	}

	return &shareState, nil
}

// Returns a Share containing at least the id field, but not necessarily more
func (m *ShareMgr) getEmptyShareByRef(ctx context.Context, ref *collaboration.ShareReference) (*model.Share, error) {
	var share *model.Share
	var err error
	if id := ref.GetId(); id != nil {
		share, err = emptyShareWithId(id.OpaqueId)
	} else {
		share, err = m.getShare(ctx, ref, true, true)
	}
	return share, err
}

func emptyShareWithId(id string) (*model.Share, error) {
	intId, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}
	share := &model.Share{
		ProtoShare: model.ProtoShare{
			Id: uint(intId),
		},
	}
	return share, nil
}

func (m *ShareMgr) getReceivedByID(ctx context.Context, id *collaboration.ShareId, gtype userpb.UserType) (*collaboration.ReceivedShare, error) {
	user := appctx.ContextMustGetUser(ctx)
	share, err := m.getShareByID(ctx, id, true)
	if err != nil {
		return nil, err
	}

	shareState, err := m.getShareState(ctx, share, user)
	if err != nil {
		return nil, err
	}

	receivedShare := share.AsCS3ReceivedShare(shareState, gtype)
	return receivedShare, nil
}

func (m *ShareMgr) getReceivedByKey(ctx context.Context, key *collaboration.ShareKey, gtype userpb.UserType) (*collaboration.ReceivedShare, error) {
	user := appctx.ContextMustGetUser(ctx)
	share, err := m.getShareByKey(ctx, key, false, true)
	if err != nil {
		return nil, err
	}

	shareState, err := m.getShareState(ctx, share, user)
	if err != nil {
		return nil, err
	}

	receivedShare := share.AsCS3ReceivedShare(shareState, gtype)
	return receivedShare, nil
}

func (m *ShareMgr) getUserType(ctx context.Context, username string) (userpb.UserType, error) {
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(m.c.GatewaySvc))
	if err != nil {
		return userpb.UserType_USER_TYPE_PRIMARY, err
	}
	userRes, err := client.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim: "username",
		Value: username,
	})
	if err != nil {
		return userpb.UserType_USER_TYPE_PRIMARY, errors.Wrapf(err, "error getting user by username '%v'", username)
	}
	if userRes.Status.Code != rpc.Code_CODE_OK {
		return userpb.UserType_USER_TYPE_PRIMARY, status.NewErrorFromCode(userRes.Status.Code, "oidc")
	}

	return userRes.GetUser().Id.Type, nil
}

func (m *ShareMgr) appendShareFiltersToQuery(query *gorm.DB, filters []*collaboration.Filter) {
	// We want to chain filters of different types with AND
	// and filters of the same type with OR
	// Therefore, we group them by type
	groupedFilters := revashare.GroupFiltersByType(filters)

	for filtertype, filters := range groupedFilters {
		switch filtertype {
		case collaboration.Filter_TYPE_RESOURCE_ID:
			innerQuery := m.db
			for i, filter := range filters {
				if i == 0 {
					innerQuery = innerQuery.Where("instance = ? and inode = ?", filter.GetResourceId().StorageId, filter.GetResourceId().OpaqueId)
				} else {
					innerQuery = innerQuery.Or("instance = ? and inode = ?", filter.GetResourceId().StorageId, filter.GetResourceId().OpaqueId)
				}
			}
			query = query.Where(innerQuery)
		case collaboration.Filter_TYPE_EXCLUDE_DENIALS:
			query = query.Where("permissions > ?", 0)
		case collaboration.Filter_TYPE_GRANTEE_TYPE:
			innerQuery := m.db
			for i, filter := range filters {
				isGroup := filter.GetGranteeType() == provider.GranteeType_GRANTEE_TYPE_GROUP
				if i == 0 {
					innerQuery = innerQuery.Where("shared_with_is_group = ?", isGroup)
				} else {
					innerQuery = innerQuery.Or("shared_with_is_group = ? ", isGroup)
				}
			}
			query = query.Where(innerQuery)
		case collaboration.Filter_TYPE_CREATOR:
			innerQuery := m.db
			for i, filter := range filters {
				if i == 0 {
					innerQuery = innerQuery.Where("uid_initiator = ?", filter.GetCreator().OpaqueId)
				} else {
					innerQuery = innerQuery.Or("uid_initiator = ?", filter.GetCreator().OpaqueId)
				}
			}
			query = query.Where(innerQuery)
		}
	}
}
