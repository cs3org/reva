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

package jsoncs3

import (
	"context"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"

	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/share"
	"github.com/cs3org/reva/v2/pkg/share/manager/jsoncs3/providercache"
	"github.com/cs3org/reva/v2/pkg/share/manager/jsoncs3/receivedsharecache"
	"github.com/cs3org/reva/v2/pkg/share/manager/jsoncs3/sharecache"
	"github.com/cs3org/reva/v2/pkg/share/manager/jsoncs3/shareid"
	"github.com/cs3org/reva/v2/pkg/share/manager/registry"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata" // nolint:staticcheck // we need the legacy package to convert V1 to V2 messages
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
)

/*
  The sharded json driver splits the json file per storage space. Similar to fileids shareids are prefixed with the spaceid for easier lookup.
  In addition to the space json the share manager keeps lists for users and groups to cache their lists of created and received shares
  and to hold the state of received shares.

  FAQ
  Q: Why not split shares by user and have a list per user?
  A: While shares are created by users, they are persisted as grants on a file.
     If we persist shares by their creator/owner they would vanish if a user is deprovisioned: shares
	 in project spaces could not be managed collaboratively.
	 By splitting by space, we are in fact not only splitting by user, but more granular, per space.


  File structure in the jsoncs3 space:

  /storages/{storageid}/{spaceid.json} 	// contains the share information of all shares in that space
  /users/{userid}/created.json			// points to the spaces the user created shares in, including the list of shares
  /users/{userid}/received.json			// holds the accepted/pending state and mount point of received shares for users
  /groups/{groupid}/received.json		// points to the spaces the group has received shares in including the list of shares

  Example:
  	├── groups
  	│	└── group1
  	│		└── received.json
  	├── storages
  	│	└── storageid
  	│		└── spaceid.json
  	└── users
   		├── admin
 		│	└── created.json
 		└── einstein
 			└── received.json

  Whenever a share is created, the share manager has to
  1. update the /storages/{storageid}/{spaceid}.json file,
  2. create /users/{userid}/created.json if it doesn't exist yet and add the space/share
  3. create /users/{userid}/received.json or /groups/{groupid}/received.json if it doesn exist yet and add the space/share

  When updating shares /storages/{storageid}/{spaceid}.json is updated accordingly. The mtime is used to invalidate in-memory caches:
  - TODO the upload is tried with an if-unmodified-since header
  - TODO when if fails, the {spaceid}.json file is downloaded, the changes are reapplied and the upload is retried with the new mtime

  When updating received shares the mountpoint and state are updated in /users/{userid}/received.json (for both user and group shares).

  When reading the list of received shares the /users/{userid}/received.json file and the /groups/{groupid}/received.json files are statted.
  - if the mtime changed we download the file to update the local cache

  When reading the list of created shares the /users/{userid}/created.json file is statted
  - if the mtime changed we download the file to update the local cache
*/

func init() {
	registry.Register("jsoncs3", NewDefault)
}

type config struct {
	GatewayAddr       string `mapstructure:"gateway_addr"`
	ProviderAddr      string `mapstructure:"provider_addr"`
	ServiceUserID     string `mapstructure:"service_user_id"`
	ServiceUserIdp    string `mapstructure:"service_user_idp"`
	MachineAuthAPIKey string `mapstructure:"machine_auth_apikey"`
}

// Manager implements a share manager using a cs3 storage backend with local caching
type Manager struct {
	sync.RWMutex

	Cache              providercache.Cache      // holds all shares, sharded by provider id and space id
	CreatedCache       sharecache.Cache         // holds the list of shares a user has created, sharded by user id
	GroupReceivedCache sharecache.Cache         // holds the list of shares a group has access to, sharded by group id
	UserReceivedStates receivedsharecache.Cache // holds the state of shares a user has received, sharded by user id

	storage   metadata.Storage
	SpaceRoot *provider.ResourceId

	initialized bool

	gateway gatewayv1beta1.GatewayAPIClient
}

// NewDefault returns a new manager instance with default dependencies
func NewDefault(m map[string]interface{}) (share.Manager, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	s, err := metadata.NewCS3Storage(c.ProviderAddr, c.ProviderAddr, c.ServiceUserID, c.ServiceUserIdp, c.MachineAuthAPIKey)
	if err != nil {
		return nil, err
	}

	gc, err := pool.GetGatewayServiceClient(c.GatewayAddr)
	if err != nil {
		return nil, err
	}

	return New(s, gc)
}

// New returns a new manager instance.
func New(s metadata.Storage, gc gatewayv1beta1.GatewayAPIClient) (*Manager, error) {
	return &Manager{
		Cache:              providercache.New(s),
		CreatedCache:       sharecache.New(s, "users", "created.json"),
		UserReceivedStates: receivedsharecache.New(s),
		GroupReceivedCache: sharecache.New(s, "groups", "received.json"),
		storage:            s,
		gateway:            gc,
	}, nil
}

func (m *Manager) initialize() error {
	if m.initialized {
		return nil
	}

	m.Lock()
	defer m.Unlock()

	if m.initialized { // check if initialization happened while grabbing the lock
		return nil
	}

	ctx := context.Background()
	err := m.storage.Init(ctx, "jsoncs3-share-manager-metadata")
	if err != nil {
		return err
	}

	err = m.storage.MakeDirIfNotExist(ctx, "storages")
	if err != nil {
		return err
	}
	err = m.storage.MakeDirIfNotExist(ctx, "users")
	if err != nil {
		return err
	}
	err = m.storage.MakeDirIfNotExist(ctx, "groups")
	if err != nil {
		return err
	}

	m.initialized = true
	return nil
}

// Share creates a new share
func (m *Manager) Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}

	user := ctxpkg.ContextMustGetUser(ctx)
	ts := utils.TSNow()

	// do not allow share to myself or the owner if share is for a user
	// TODO: should this not already be caught at the gw level?
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER &&
		(utils.UserEqual(g.Grantee.GetUserId(), user.Id) || utils.UserEqual(g.Grantee.GetUserId(), md.Owner)) {
		return nil, errors.New("json: owner/creator and grantee are the same")
	}

	// check if share already exists.
	key := &collaboration.ShareKey{
		//Owner:      md.Owner, owner not longer matters as it belongs to the space
		ResourceId: md.Id,
		Grantee:    g.Grantee,
	}

	m.Lock()
	defer m.Unlock()
	_, err := m.getByKey(ctx, key)
	if err == nil {
		// share already exists
		return nil, errtypes.AlreadyExists(key.String())
	}

	shareID := shareid.Encode(md.GetId().GetStorageId(), md.GetId().GetSpaceId(), uuid.NewString())
	s := &collaboration.Share{
		Id: &collaboration.ShareId{
			OpaqueId: shareID,
		},
		ResourceId:  md.Id,
		Permissions: g.Permissions,
		Grantee:     g.Grantee,
		Owner:       md.Owner,
		Creator:     user.Id,
		Ctime:       ts,
		Mtime:       ts,
	}

	err = m.Cache.Add(ctx, md.Id.StorageId, md.Id.SpaceId, shareID, s)
	if _, ok := err.(errtypes.IsPreconditionFailed); ok {
		if err := m.Cache.Sync(ctx, md.Id.StorageId, md.Id.SpaceId); err != nil {
			return nil, err
		}
		err = m.Cache.Add(ctx, md.Id.StorageId, md.Id.SpaceId, shareID, s)
		// TODO try more often?
	}
	if err != nil {
		return nil, err
	}

	err = m.CreatedCache.Add(ctx, s.GetCreator().GetOpaqueId(), shareID)
	if _, ok := err.(errtypes.IsPreconditionFailed); ok {
		if err := m.CreatedCache.Sync(ctx, s.GetCreator().GetOpaqueId()); err != nil {
			return nil, err
		}
		err = m.CreatedCache.Add(ctx, s.GetCreator().GetOpaqueId(), shareID)
		// TODO try more often?
	}
	if err != nil {
		return nil, err
	}

	spaceID := md.Id.StorageId + shareid.IDDelimiter + md.Id.SpaceId
	// set flag for grantee to have access to share
	switch g.Grantee.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		userid := g.Grantee.GetUserId().GetOpaqueId()

		rs := &collaboration.ReceivedShare{
			Share: s,
			State: collaboration.ShareState_SHARE_STATE_PENDING,
		}
		err = m.UserReceivedStates.Add(ctx, userid, spaceID, rs)
		if _, ok := err.(errtypes.IsPreconditionFailed); ok {
			if err := m.UserReceivedStates.Sync(ctx, s.GetCreator().GetOpaqueId()); err != nil {
				return nil, err
			}
			err = m.UserReceivedStates.Add(ctx, userid, spaceID, rs)
			// TODO try more often?
		}
		if err != nil {
			return nil, err
		}
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		groupid := g.Grantee.GetGroupId().GetOpaqueId()
		err := m.GroupReceivedCache.Add(ctx, groupid, shareID)
		if _, ok := err.(errtypes.IsPreconditionFailed); ok {
			if err := m.GroupReceivedCache.Sync(ctx, groupid); err != nil {
				return nil, err
			}
			err = m.GroupReceivedCache.Add(ctx, groupid, shareID)
			// TODO try more often?
		}
		if err != nil {
			return nil, err
		}
	}

	return s, nil
}

// getByID must be called in a lock-controlled block.
func (m *Manager) getByID(ctx context.Context, id *collaboration.ShareId) (*collaboration.Share, error) {
	storageID, spaceID, _ := shareid.Decode(id.OpaqueId)
	// sync cache, maybe our data is outdated
	err := m.Cache.Sync(ctx, storageID, spaceID)
	if err != nil {
		return nil, err
	}

	share := m.Cache.Get(storageID, spaceID, id.OpaqueId)
	if share == nil {
		return nil, errtypes.NotFound(id.String())
	}
	return share, nil
}

// getByKey must be called in a lock-controlled block.
func (m *Manager) getByKey(ctx context.Context, key *collaboration.ShareKey) (*collaboration.Share, error) {
	err := m.Cache.Sync(ctx, key.ResourceId.StorageId, key.ResourceId.SpaceId)
	if err != nil {
		return nil, err
	}

	spaceShares := m.Cache.ListSpace(key.ResourceId.StorageId, key.ResourceId.SpaceId)
	for _, share := range spaceShares.Shares {
		if utils.GranteeEqual(key.Grantee, share.Grantee) && utils.ResourceIDEqual(share.ResourceId, key.ResourceId) {
			return share, nil
		}
	}
	return nil, errtypes.NotFound(key.String())
}

// get must be called in a lock-controlled block.
func (m *Manager) get(ctx context.Context, ref *collaboration.ShareReference) (s *collaboration.Share, err error) {
	switch {
	case ref.GetId() != nil:
		s, err = m.getByID(ctx, ref.GetId())
	case ref.GetKey() != nil:
		s, err = m.getByKey(ctx, ref.GetKey())
	default:
		err = errtypes.NotFound(ref.String())
	}
	return
}

// GetShare gets the information for a share by the given ref.
func (m *Manager) GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}

	m.Lock()
	defer m.Unlock()
	s, err := m.get(ctx, ref)
	if err != nil {
		return nil, err
	}
	// check if we are the creator or the grantee
	// TODO allow manager to get shares in a space created by other users
	user := ctxpkg.ContextMustGetUser(ctx)
	if share.IsCreatedByUser(s, user) || share.IsGrantedToUser(s, user) {
		return s, nil
	}

	req := &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: s.ResourceId},
	}
	res, err := m.gateway.Stat(ctx, req)
	if err == nil &&
		res.Status.Code == rpcv1beta1.Code_CODE_OK &&
		res.Info.PermissionSet.ListGrants {
		return s, nil
	}

	// we return not found to not disclose information
	return nil, errtypes.NotFound(ref.String())
}

// Unshare deletes a share
func (m *Manager) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
	if err := m.initialize(); err != nil {
		return err
	}

	m.Lock()
	defer m.Unlock()
	user := ctxpkg.ContextMustGetUser(ctx)

	s, err := m.get(ctx, ref)
	if err != nil {
		return err
	}
	// TODO allow manager to unshare shares in a space created by other users
	if !share.IsCreatedByUser(s, user) {
		// TODO why not permission denied?
		return errtypes.NotFound(ref.String())
	}

	storageID, spaceID, _ := shareid.Decode(s.Id.OpaqueId)
	err = m.Cache.Remove(ctx, storageID, spaceID, s.Id.OpaqueId)
	if _, ok := err.(errtypes.IsPreconditionFailed); ok {
		if err := m.Cache.Sync(ctx, storageID, spaceID); err != nil {
			return err
		}
		err = m.Cache.Remove(ctx, storageID, spaceID, s.Id.OpaqueId)
		// TODO try more often?
	}
	if err != nil {
		return err
	}

	// remove from created cache
	err = m.CreatedCache.Remove(ctx, s.GetCreator().GetOpaqueId(), s.Id.OpaqueId)
	if _, ok := err.(errtypes.IsPreconditionFailed); ok {
		if err := m.CreatedCache.Sync(ctx, s.GetCreator().GetOpaqueId()); err != nil {
			return err
		}
		err = m.CreatedCache.Remove(ctx, s.GetCreator().GetOpaqueId(), s.Id.OpaqueId)
		// TODO try more often?
	}
	if err != nil {
		return err
	}

	// TODO remove from grantee cache

	return nil
}

// UpdateShare updates the mode of the given share.
func (m *Manager) UpdateShare(ctx context.Context, ref *collaboration.ShareReference, p *collaboration.SharePermissions) (*collaboration.Share, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}

	m.Lock()
	defer m.Unlock()
	s, err := m.get(ctx, ref)
	if err != nil {
		return nil, err
	}

	user := ctxpkg.ContextMustGetUser(ctx)
	if !share.IsCreatedByUser(s, user) {
		req := &provider.StatRequest{
			Ref: &provider.Reference{ResourceId: s.ResourceId},
		}
		res, err := m.gateway.Stat(ctx, req)
		if err != nil ||
			res.Status.Code != rpcv1beta1.Code_CODE_OK ||
			!res.Info.PermissionSet.UpdateGrant {
			return nil, errtypes.NotFound(ref.String())
		}
	}

	s.Permissions = p
	s.Mtime = utils.TSNow()

	// Update provider cache
	err = m.Cache.Persist(ctx, s.ResourceId.StorageId, s.ResourceId.SpaceId)
	// when persisting fails
	if _, ok := err.(errtypes.IsPreconditionFailed); ok {
		// reupdate
		s, err = m.get(ctx, ref) // does an implicit sync
		if err != nil {
			return nil, err
		}
		s.Permissions = p
		s.Mtime = utils.TSNow()

		// persist again
		err = m.Cache.Persist(ctx, s.ResourceId.StorageId, s.ResourceId.SpaceId)
		// TODO try more often?
	}
	if err != nil {
		return nil, err
	}

	return s, nil
}

// ListShares returns the shares created by the user
func (m *Manager) ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}

	m.Lock()
	defer m.Unlock()

	user := ctxpkg.ContextMustGetUser(ctx)

	if len(share.FilterFiltersByType(filters, collaboration.Filter_TYPE_RESOURCE_ID)) > 0 {
		return m.listSharesByIDs(ctx, user, filters)
	}

	return m.listCreatedShares(ctx, user, filters)
}

func (m *Manager) listSharesByIDs(ctx context.Context, user *userv1beta1.User, filters []*collaboration.Filter) ([]*collaboration.Share, error) {
	providerSpaces := make(map[string]map[string]struct{})
	for _, f := range share.FilterFiltersByType(filters, collaboration.Filter_TYPE_RESOURCE_ID) {
		storageID := f.GetResourceId().GetStorageId()
		spaceID := f.GetResourceId().GetSpaceId()
		if providerSpaces[storageID] == nil {
			providerSpaces[storageID] = make(map[string]struct{})
		}
		providerSpaces[storageID][spaceID] = struct{}{}
	}

	statCache := make(map[string]struct{})
	var ss []*collaboration.Share
	for providerID, spaces := range providerSpaces {
		for spaceID := range spaces {
			err := m.Cache.Sync(ctx, providerID, spaceID)
			if err != nil {
				return nil, err
			}

			shares := m.Cache.ListSpace(providerID, spaceID)

			for _, s := range shares.Shares {
				if !share.MatchesFilters(s, filters) {
					continue
				}

				if !(share.IsCreatedByUser(s, user) || share.IsGrantedToUser(s, user)) {
					key := storagespace.FormatResourceID(*s.ResourceId)
					if _, hit := statCache[key]; !hit {
						req := &provider.StatRequest{
							Ref: &provider.Reference{ResourceId: s.ResourceId},
						}
						res, err := m.gateway.Stat(ctx, req)
						if err != nil ||
							res.Status.Code != rpcv1beta1.Code_CODE_OK ||
							!res.Info.PermissionSet.ListGrants {
							continue
						}
						statCache[key] = struct{}{}
					}
				}

				ss = append(ss, s)
			}
		}
	}
	return ss, nil
}

func (m *Manager) listCreatedShares(ctx context.Context, user *userv1beta1.User, filters []*collaboration.Filter) ([]*collaboration.Share, error) {
	var ss []*collaboration.Share

	if err := m.CreatedCache.Sync(ctx, user.Id.OpaqueId); err != nil {
		return ss, err
	}
	for ssid, spaceShareIDs := range m.CreatedCache.List(user.Id.OpaqueId) {
		storageID, spaceID, _ := shareid.Decode(ssid)
		err := m.Cache.Sync(ctx, storageID, spaceID)
		if err != nil {
			continue
		}
		spaceShares := m.Cache.ListSpace(storageID, spaceID)
		for shareid := range spaceShareIDs.IDs {
			s := spaceShares.Shares[shareid]
			if s == nil {
				continue
			}
			if utils.UserEqual(user.GetId(), s.GetCreator()) {
				if share.MatchesFilters(s, filters) {
					ss = append(ss, s)
				}
			}
		}
	}

	return ss, nil
}

// ListReceivedShares returns the list of shares the user has access to.
func (m *Manager) ListReceivedShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.ReceivedShare, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}

	m.Lock()
	defer m.Unlock()

	var rss []*collaboration.ReceivedShare
	user := ctxpkg.ContextMustGetUser(ctx)

	ssids := map[string]*receivedsharecache.Space{}

	// first collect all spaceids the user has access to as a group member
	for _, group := range user.Groups {
		if err := m.GroupReceivedCache.Sync(ctx, group); err != nil {
			continue // ignore error, cache will be updated on next read
		}
		for ssid, spaceShareIDs := range m.GroupReceivedCache.List(group) {
			// add a pending entry, the state will be updated
			// when reading the received shares below if they have already been accepted or denied
			rs := receivedsharecache.Space{
				Mtime:  spaceShareIDs.Mtime,
				States: make(map[string]*receivedsharecache.State, len(spaceShareIDs.IDs)),
			}

			for shareid := range spaceShareIDs.IDs {
				rs.States[shareid] = &receivedsharecache.State{
					State: collaboration.ShareState_SHARE_STATE_PENDING,
				}
			}
			ssids[ssid] = &rs
		}
	}

	// add all spaces the user has receved shares for, this includes mount points and share state for groups
	_ = m.UserReceivedStates.Sync(ctx, user.Id.OpaqueId) // ignore error, cache will be updated on next read

	if m.UserReceivedStates.ReceivedSpaces[user.Id.OpaqueId] != nil {
		for ssid, rspace := range m.UserReceivedStates.ReceivedSpaces[user.Id.OpaqueId].Spaces {
			if rs, ok := ssids[ssid]; ok {
				for shareid, state := range rspace.States {
					// overwrite state
					rs.States[shareid] = state
				}
			} else {
				ssids[ssid] = rspace
			}
		}
	}

	for ssid, rspace := range ssids {
		storageID, spaceID, _ := shareid.Decode(ssid)
		err := m.Cache.Sync(ctx, storageID, spaceID)
		if err != nil {
			continue
		}
		for shareID, state := range rspace.States {
			s := m.Cache.Get(storageID, spaceID, shareID)
			if s == nil {
				continue
			}

			if share.IsGrantedToUser(s, user) {
				if share.MatchesFilters(s, filters) {
					rs := &collaboration.ReceivedShare{
						Share:      s,
						State:      state.State,
						MountPoint: state.MountPoint,
					}
					rss = append(rss, rs)
				}
			}
		}
	}

	return rss, nil
}

// convert must be called in a lock-controlled block.
func (m *Manager) convert(ctx context.Context, userID string, s *collaboration.Share) *collaboration.ReceivedShare {
	rs := &collaboration.ReceivedShare{
		Share: s,
		State: collaboration.ShareState_SHARE_STATE_PENDING,
	}

	storageID, spaceID, _ := shareid.Decode(s.Id.OpaqueId)

	_ = m.UserReceivedStates.Sync(ctx, userID) // ignore error, cache will be updated on next read
	state := m.UserReceivedStates.Get(userID, storageID+shareid.IDDelimiter+spaceID, s.Id.GetOpaqueId())
	if state != nil {
		rs.State = state.State
		rs.MountPoint = state.MountPoint
	}
	return rs
}

// GetReceivedShare returns the information for a received share.
func (m *Manager) GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}

	return m.getReceived(ctx, ref)
}

func (m *Manager) getReceived(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()
	s, err := m.get(ctx, ref)
	if err != nil {
		return nil, err
	}
	user := ctxpkg.ContextMustGetUser(ctx)
	if !share.IsGrantedToUser(s, user) {
		return nil, errtypes.NotFound(ref.String())
	}
	return m.convert(ctx, user.Id.GetOpaqueId(), s), nil
}

// UpdateReceivedShare updates the received share with share state.
func (m *Manager) UpdateReceivedShare(ctx context.Context, receivedShare *collaboration.ReceivedShare, fieldMask *field_mask.FieldMask) (*collaboration.ReceivedShare, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}

	rs, err := m.getReceived(ctx, &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: receivedShare.Share.Id}})
	if err != nil {
		return nil, err
	}

	m.Lock()
	defer m.Unlock()

	for i := range fieldMask.Paths {
		switch fieldMask.Paths[i] {
		case "state":
			rs.State = receivedShare.State
		case "mount_point":
			rs.MountPoint = receivedShare.MountPoint
		default:
			return nil, errtypes.NotSupported("updating " + fieldMask.Paths[i] + " is not supported")
		}
	}

	// write back

	userID := ctxpkg.ContextMustGetUser(ctx)

	err = m.UserReceivedStates.Add(ctx, userID.GetId().GetOpaqueId(), rs.Share.ResourceId.StorageId+shareid.IDDelimiter+rs.Share.ResourceId.SpaceId, rs)
	if _, ok := err.(errtypes.IsPreconditionFailed); ok {
		// when persisting fails, download, readd and persist again
		if err := m.UserReceivedStates.Sync(ctx, userID.GetId().GetOpaqueId()); err != nil {
			return nil, err
		}
		err = m.UserReceivedStates.Add(ctx, userID.GetId().GetOpaqueId(), rs.Share.ResourceId.StorageId+shareid.IDDelimiter+rs.Share.ResourceId.SpaceId, rs)
		// TODO try more often?
	}
	if err != nil {
		return nil, err
	}

	return rs, nil
}

func shareIsRoutable(share *collaboration.Share) bool {
	return strings.Contains(share.Id.OpaqueId, shareid.IDDelimiter)
}
func updateShareID(share *collaboration.Share) {
	share.Id.OpaqueId = shareid.Encode(share.ResourceId.StorageId, share.ResourceId.SpaceId, share.Id.OpaqueId)
}

// Load imports shares and received shares from channels (e.g. during migration)
func (m *Manager) Load(ctx context.Context, shareChan <-chan *collaboration.Share, receivedShareChan <-chan share.ReceivedShareWithUser) error {
	log := appctx.GetLogger(ctx)
	if err := m.initialize(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		for s := range shareChan {
			if s == nil {
				continue
			}
			if !shareIsRoutable(s) {
				updateShareID(s)
			}
			if err := m.Cache.Add(context.Background(), s.GetResourceId().GetStorageId(), s.GetResourceId().GetSpaceId(), s.Id.OpaqueId, s); err != nil {
				log.Error().Err(err).Interface("share", s).Msg("error persisting share")
			} else {
				log.Debug().Str("storageid", s.GetResourceId().GetStorageId()).Str("spaceid", s.GetResourceId().GetSpaceId()).Str("shareid", s.Id.OpaqueId).Msg("imported share")
			}
			if err := m.CreatedCache.Add(ctx, s.GetCreator().GetOpaqueId(), s.Id.OpaqueId); err != nil {
				log.Error().Err(err).Interface("share", s).Msg("error persisting created cache")
			} else {
				log.Debug().Str("creatorid", s.GetCreator().GetOpaqueId()).Str("shareid", s.Id.OpaqueId).Msg("updated created cache")
			}
		}
		wg.Done()
	}()
	go func() {
		for s := range receivedShareChan {
			if s.ReceivedShare != nil {
				if !shareIsRoutable(s.ReceivedShare.GetShare()) {
					updateShareID(s.ReceivedShare.GetShare())
				}
				switch s.ReceivedShare.Share.Grantee.Type {
				case provider.GranteeType_GRANTEE_TYPE_USER:
					if err := m.UserReceivedStates.Add(context.Background(), s.ReceivedShare.GetShare().GetGrantee().GetUserId().GetOpaqueId(), s.ReceivedShare.GetShare().GetResourceId().GetSpaceId(), s.ReceivedShare); err != nil {
						log.Error().Err(err).Interface("received share", s).Msg("error persisting received share for user")
					} else {
						log.Debug().Str("userid", s.ReceivedShare.GetShare().GetGrantee().GetUserId().GetOpaqueId()).Str("spaceid", s.ReceivedShare.GetShare().GetResourceId().GetSpaceId()).Str("shareid", s.ReceivedShare.GetShare().Id.OpaqueId).Msg("updated received share userdata")
					}
				case provider.GranteeType_GRANTEE_TYPE_GROUP:
					if err := m.GroupReceivedCache.Add(context.Background(), s.ReceivedShare.GetShare().GetGrantee().GetGroupId().GetOpaqueId(), s.ReceivedShare.GetShare().GetId().GetOpaqueId()); err != nil {
						log.Error().Err(err).Interface("received share", s).Msg("error persisting received share to group cache")
					} else {
						log.Debug().Str("groupid", s.ReceivedShare.GetShare().GetGrantee().GetGroupId().GetOpaqueId()).Str("shareid", s.ReceivedShare.GetShare().Id.OpaqueId).Msg("updated received share group cache")
					}
				}
			}
		}
		wg.Done()
	}()
	wg.Wait()

	return nil
}
