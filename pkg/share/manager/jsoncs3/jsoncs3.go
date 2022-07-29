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
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/bluele/gcache"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/share"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata" // nolint:staticcheck // we need the legacy package to convert V1 to V2 messages
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"

	"github.com/cs3org/reva/v2/pkg/share/manager/registry"
	"github.com/cs3org/reva/v2/pkg/utils"
)

/*
  The sharded json driver splits the json file per storage space. Similar to fileids shareids are prefixed with the spaceid.

  FAQ
  Q: Why not split shares by user and have a list per user?
  A: While shares are created by users, they are persisted as grants on a file.
     If we persist shares by their creator/owner they would vanish if a user is deprovisioned: shares
	 in project spaces could not be managed collaboratively.
	 By splitting by space, we are in fact not only splitting by user, but more granular, per space.

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

type providerCache map[string]providerSpaces
type providerSpaces map[string]gcache.Cache

type receivedCache map[string]gcache.Cache
type receivedSpace struct {
	mtime               int64
	receivedShareStates map[string]receivedShareState
}
type receivedShareState struct {
	state      collaboration.ShareState
	mountPoint *provider.Reference
}

type manager struct {
	sync.RWMutex

	// cache holds the all shares, sharded by provider id and space id
	cache providerCache
	// createdCache holds the list of shares a user has created, sharded by user id and space id
	createdCache shareCache
	// groupReceivedCache holds the list of shares a group has access to, sharded by group id and space id
	groupReceivedCache shareCache
	// userReceivedStates holds the state of shares a user has received, sharded by user id and space id
	userReceivedStates receivedCache

	storage     metadata.Storage
	spaceETags  gcache.Cache
	serviceUser *userv1beta1.User
	SpaceRoot   *provider.ResourceId
	initialized bool
}

// NewDefault returns a new manager instance with default dependencies
func NewDefault(m map[string]interface{}) (share.Manager, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	s, err := metadata.NewCS3Storage(c.GatewayAddr, c.ProviderAddr, c.ServiceUserID, c.ServiceUserIdp, c.MachineAuthAPIKey)
	if err != nil {
		return nil, err
	}

	return New(s)
}

// New returns a new manager instance.
func New(s metadata.Storage) (share.Manager, error) {
	return &manager{
		cache:              providerCache{},
		createdCache:       NewShareCache(),
		userReceivedStates: receivedCache{},
		groupReceivedCache: NewShareCache(),
		storage:            s,
		spaceETags:         gcache.New(1_000_000).LFU().Build(),
	}, nil
}

// File structure in the jsoncs3 space:
//
// /shares/{shareid.json}                     				// points to {storageid}/{spaceid} for looking up the share information
// /storages/{storageid}/{spaceid.json}                     // contains all share information of all shares in that space
// /users/{userid}/created/{storageid}/{spaceid}			// points to a space the user created shares in
// /users/{userid}/received/{storageid}/{spaceid}.json		// holds the states of received shares of the users in the according space
// /groups/{groupid}/received/{storageid}/{spaceid}			// points to a space the group has received shares in

// We store the shares in the metadata storage under /{storageid}/{spaceid}.json

// To persist the mountpoints of group shares the /{userid}/received/{storageid}/{spaceid}.json file is used.
// - it allows every user to update his own mountpoint without having to update&reread the /{storageid}/{spaceid}.json file

// To persist the accepted / pending state of shares the /{userid}/received/{storageid}/{spaceid}.json file is used.
// - it allows every user to update his own mountpoint without having to update&reread the /{storageid}/{spaceid}.json file

// To determine access to group shares a /{groupid}/received/{storageid}/{spaceid} file is used.

// Whenever a share is created, the share manager has to
// 1. update the /{storageid}/{spaceid}.json file,
// 2. touch /{userid}/created/{storageid}/{spaceid} and
// 3. touch /{userid}/received/{storageid}/{spaceid}.json or /{groupid}/received/{storageid}/{spaceid}
// - The /{userid}/received/{storageid}/{spaceid}.json file persists mountpoints and accepted / rejected state
// - (optional) to wrap these three steps in a transaction the share manager can create a transaction file befor the first step and clean it up when all steps succeded

// To determine the spaces a user has access to we maintain an empty /{userid}/(received|created)/{storageid}/{spaceid} folder
// that we persist when initially traversing all shares in the metadata /{storageid}/{spaceid}.json files
// when a user creates a new share the jsoncs3 manager touches a new /{userid}/(received|created)/{storageid}/{spaceid} folder
//  - the changed mtime can be used to determine when a space needs to be reread for redundant setups

// when initializing we only initialize per user:
//  - we list /{userid}/created/*, for every space we fetch /{storageid}/{spaceid}.json if we
//    have not cached it yet, or if the /{userid}/created/{storageid}${spaceid} etag changed
//  - if it does not exist we query the registry for every storage provider id, then
//    we traverse /{storageid}/ in the metadata storage to
//    1. create /{userid}/created
//    2. touch /{userid}/created/{storageid}${spaceid}
//    TODO 3. split storageid from spaceid touch /{userid}/created/{storageid} && /{userid}/created/{storageid}/{spaceid} (not needed when mtime propagation is enabled)

// we need to split the two lists:
// /{userid}/received/{storageid}/{spaceid}
// /{userid}/created/{storageid}/{spaceid}

func (m *manager) initialize(ctx context.Context) error {
	// if local copy is invalid fetch a new one
	// invalid = not set || etag changed
	if m.initialized {
		return nil
	}

	m.Lock()
	defer m.Unlock()

	if m.initialized { // check if initialization happened while grabbing the lock
		return nil
	}

	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return fmt.Errorf("missing user in context")
	}

	err := m.storage.Init(context.Background(), "jsoncs3-share-manager-metadata")
	if err != nil {
		return err
	}

	infos, err := m.storage.ListDir(ctx, filepath.Join("users", user.Id.OpaqueId, "created"))
	if err != nil {
		return err
	}
	// for every space we fetch /{storageid}/{spaceid}.json if we
	//    have not cached it yet, or if the /{userid}/created/{storageid}${spaceid} etag changed
	for _, storageInfo := range infos {
		// do we have spaces for this storage cached?
		etag := m.getCachedSpaceETag(storageInfo.Name)
		if etag == "" || etag != storageInfo.Etag {

			// TODO update cache
			// reread /{storageid}/{spaceid}.json ?
			// hmm the dir listing for a /einstein-id/created/{storageid}${spaceid} might have a different
			// etag than the one for /marie-id/created/{storageid}${spaceid}
			// do we also need the mtime in addition to the etag? so we can determine which one is younger?
			// currently if einstein creates a share in space a we do a stat for every
			// other user with access to the space because we update the cached space etag AND we touch the
			// /einstein-id/created/{storageid}${spaceid} ... which updates the mtime ... so we don't need
			// the etag, but only the mtime of /einstein-id/created/{storageid}${spaceid} ? which we set to
			// the /{storageid}/{spaceid}.json mtime. since we always do the mtime setting ... this should work
			// well .. if cs3 touch allows setting the mtime
			// client.TouchFile(ctx, &provider.TouchFileRequest{
			// 	Ref:    &provider.Reference{},
			// 	Opaque: &typespb.Opaque{ /*TODO allow setting the mtime with touch*/ },
			// })
			// maybe we need SetArbitraryMetadata to set the mtime
		}
		//
		// TODO use space if etag is same
	}

	return nil
}

func (m *manager) getCachedSpaceETag(spaceid string) string {
	if e, err := m.spaceETags.Get(spaceid); err != gcache.KeyNotFoundError {
		if etag, ok := e.(string); ok {
			return etag
		}
	}
	return ""
}

func (m *manager) Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error) {

	user := ctxpkg.ContextMustGetUser(ctx)
	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / int64(time.Second)),
		Nanos:   uint32(now % int64(time.Second)),
	}

	// do not allow share to myself or the owner if share is for a user
	// TODO(labkode): should not this be caught already at the gw level?
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
	_, err := m.getByKey(key)
	if err == nil {
		// share already exists
		return nil, errtypes.AlreadyExists(key.String())
	}
	shareReference := &provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: md.GetId().StorageId,
			SpaceId:   md.GetId().SpaceId,
			OpaqueId:  uuid.NewString(),
		},
	}
	id, err := storagespace.FormatReference(shareReference)
	if err != nil {
		return nil, err
	}
	s := &collaboration.Share{
		Id: &collaboration.ShareId{
			OpaqueId: id,
		},
		ResourceId:  md.Id,
		Permissions: g.Permissions,
		Grantee:     g.Grantee,
		Owner:       md.Owner,
		Creator:     user.Id,
		Ctime:       ts,
		Mtime:       ts,
	}

	if m.cache[md.Id.StorageId] == nil {
		m.cache[md.Id.StorageId] = providerSpaces{}
	}
	if m.cache[md.Id.StorageId][md.Id.SpaceId] == nil {
		m.cache[md.Id.StorageId][md.Id.SpaceId] = gcache.New(-1).Simple().Build()
	}
	m.cache[md.Id.StorageId][md.Id.SpaceId].Set(s.Id.OpaqueId, s)

	// set flag for creator to have access to space
	if err := m.createdCache.Add(user.Id.OpaqueId, id); err != nil {
		return nil, err
	}

	spaceId := storagespace.FormatResourceID(provider.ResourceId{
		StorageId: md.Id.StorageId,
		SpaceId:   md.Id.SpaceId,
	})
	// set flag for grantee to have access to space
	switch g.Grantee.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		userid := g.Grantee.GetUserId().GetOpaqueId()
		if m.userReceivedStates[userid] == nil {
			m.userReceivedStates[userid] = gcache.New(-1).Simple().Build() // receivedSpaces
		}
		if !m.userReceivedStates[userid].Has(spaceId) {
			err := m.userReceivedStates[userid].Set(spaceId, receivedSpace{})
			if err != nil {
				return nil, err
			}
		}
		val, err := m.userReceivedStates[userid].GetIFPresent(spaceId)
		if err != nil && err != gcache.KeyNotFoundError {
			return nil, err
		}
		receivedSpace, ok := val.(receivedSpace)
		if !ok {
			return nil, errtypes.InternalError("invalid type in cache")
		}
		receivedSpace.mtime = now
		if receivedSpace.receivedShareStates == nil {
			receivedSpace.receivedShareStates = map[string]receivedShareState{}
		}
		receivedSpace.receivedShareStates[id] = receivedShareState{
			state: collaboration.ShareState_SHARE_STATE_PENDING,
			// mountpoint stays empty until user accepts the share
		}
		m.userReceivedStates[userid].Set(spaceId, receivedSpace)
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		groupid := g.Grantee.GetGroupId().GetOpaqueId()
		if err := m.groupReceivedCache.Add(groupid, id); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// getByID must be called in a lock-controlled block.
func (m *manager) getByID(id *collaboration.ShareId) (*collaboration.Share, error) {
	shareid, err := storagespace.ParseID(id.OpaqueId)
	if err != nil {
		// invalid share id, does not exist
		return nil, errtypes.NotFound(id.String())
	}
	if providerSpaces, ok := m.cache[shareid.StorageId]; ok {
		if spaceShares, ok := providerSpaces[shareid.SpaceId]; ok {
			for _, value := range spaceShares.GetALL(false) {
				if share, ok := value.(*collaboration.Share); ok {
					if share.GetId().OpaqueId == id.OpaqueId {
						return share, nil
					}
				}
			}
		}
	}
	return nil, errtypes.NotFound(id.String())
}

// getByKey must be called in a lock-controlled block.
func (m *manager) getByKey(key *collaboration.ShareKey) (*collaboration.Share, error) {
	if providerSpaces, ok := m.cache[key.ResourceId.StorageId]; ok {
		if spaceShares, ok := providerSpaces[key.ResourceId.SpaceId]; ok {
			for _, value := range spaceShares.GetALL(false) {
				if share, ok := value.(*collaboration.Share); ok {
					if utils.GranteeEqual(key.Grantee, share.Grantee) {
						return share, nil
					}
				}
			}
		}
	}
	return nil, errtypes.NotFound(key.String())
}

// get must be called in a lock-controlled block.
func (m *manager) get(ref *collaboration.ShareReference) (s *collaboration.Share, err error) {
	switch {
	case ref.GetId() != nil:
		s, err = m.getByID(ref.GetId())
	case ref.GetKey() != nil:
		s, err = m.getByKey(ref.GetKey())
	default:
		err = errtypes.NotFound(ref.String())
	}
	return
}

func (m *manager) GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error) {
	m.Lock()
	defer m.Unlock()
	s, err := m.get(ref)
	if err != nil {
		return nil, err
	}
	// check if we are the creator or the grantee
	// TODO allow manager to get shares in a space created by other users
	user := ctxpkg.ContextMustGetUser(ctx)
	if share.IsCreatedByUser(s, user) || share.IsGrantedToUser(s, user) {
		return s, nil
	}
	// we return not found to not disclose information
	return nil, errtypes.NotFound(ref.String())
}

func (m *manager) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
	m.Lock()
	defer m.Unlock()
	user := ctxpkg.ContextMustGetUser(ctx)

	s, err := m.get(ref)
	if err != nil {
		return err
	}
	// TODO allow manager to unshare shares in a space created by other users
	if !share.IsCreatedByUser(s, user) {
		// TODO why not permission denied?
		return errtypes.NotFound(ref.String())
	}

	shareid, err := storagespace.ParseID(s.Id.OpaqueId)
	m.cache[shareid.StorageId][shareid.SpaceId].Remove(s.Id.OpaqueId)

	return nil
}

func (m *manager) UpdateShare(ctx context.Context, ref *collaboration.ShareReference, p *collaboration.SharePermissions) (*collaboration.Share, error) {
	m.Lock()
	defer m.Unlock()
	s, err := m.get(ref)
	if err != nil {
		return nil, err
	}

	user := ctxpkg.ContextMustGetUser(ctx)
	if !share.IsCreatedByUser(s, user) {
		return nil, errtypes.NotFound(ref.String())
	}

	now := time.Now().UnixNano()
	s.Permissions = p
	s.Mtime = &typespb.Timestamp{
		Seconds: uint64(now / int64(time.Second)),
		Nanos:   uint32(now % int64(time.Second)),
	}

	// FIXME actually persist
	// if err := m.model.Save(); err != nil {
	// 	err = errors.Wrap(err, "error saving model")
	// 	return nil, err
	// }
	return s, nil
}

// ListShares
func (m *manager) ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error) {
	/*if err := m.initialize(ctx); err != nil {
		return nil, err
	}*/

	m.Lock()
	defer m.Unlock()
	//log := appctx.GetLogger(ctx)
	user := ctxpkg.ContextMustGetUser(ctx)
	var ss []*collaboration.Share

	for ssid, spaceShareIDs := range m.createdCache.List(user.Id.OpaqueId) {
		if time.Now().Sub(spaceShareIDs.mtime) > time.Second*30 {
			// TODO reread from disk
		}
		providerid, spaceid, _, err := storagespace.SplitID(ssid)
		if err != nil {
			continue
		}
		if providerSpaces, ok := m.cache[providerid]; ok {
			if spaceShares, ok := providerSpaces[spaceid]; ok {
				for shareid, _ := range spaceShareIDs.IDs {
					v, err := spaceShares.Get(shareid)
					if err != nil {
						continue
					}
					if s, ok := v.(*collaboration.Share); ok {
						if utils.UserEqual(user.GetId(), s.GetCreator()) {
							if share.MatchesFilters(s, filters) {
								ss = append(ss, s)
							}
						}
					}
				}
			}
		}
	}

	return ss, nil
}

// we list the shares that are targeted to the user in context or to the user groups.
func (m *manager) ListReceivedShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()

	var rss []*collaboration.ReceivedShare
	user := ctxpkg.ContextMustGetUser(ctx)

	ssids := map[string]receivedSpace{}

	// first collect all spaceids the user has access to as a group member
	for _, group := range user.Groups {
		for ssid, spaceShareIDs := range m.groupReceivedCache.List(group) {
			if time.Now().Sub(spaceShareIDs.mtime) > time.Second*30 {
				// TODO reread from disk
			}
			// add a pending entry, the state will be updated
			// when reading the received shares below if they have already been accepted or denied
			rs := receivedSpace{
				mtime:               spaceShareIDs.mtime.UnixNano(),
				receivedShareStates: make(map[string]receivedShareState, len(spaceShareIDs.IDs)),
			}

			for shareid, _ := range spaceShareIDs.IDs {
				rs.receivedShareStates[shareid] = receivedShareState{
					state: collaboration.ShareState_SHARE_STATE_PENDING,
				}
			}
			ssids[ssid] = rs
		}
	}

	// add all spaces the user has receved shares for, this includes mount points and share state for groups
	for key, value := range m.userReceivedStates[user.Id.OpaqueId].GetALL(false) {
		var ssid string
		var rspace receivedSpace
		var ok bool
		if ssid, ok = key.(string); !ok {
			continue
		}
		if rspace, ok = value.(receivedSpace); !ok {
			continue
		}

		if rspace.mtime < time.Now().Add(-30*time.Second).UnixNano() {
			// TODO reread from disk
		}
		// TODO use younger mtime to determine if
		if rs, ok := ssids[ssid]; ok {
			for shareid, state := range rspace.receivedShareStates {
				// overwrite state
				rs.receivedShareStates[shareid] = state
			}
		} else {
			ssids[ssid] = rspace
		}
	}

	for ssid, rspace := range ssids {
		providerid, spaceid, _, err := storagespace.SplitID(ssid)
		if err != nil {
			continue
		}
		if providerSpaces, ok := m.cache[providerid]; ok {
			if spaceShares, ok := providerSpaces[spaceid]; ok {
				for shareId, state := range rspace.receivedShareStates {
					value, err := spaceShares.Get(shareId)
					if err != nil {
						continue
					}
					if s, ok := value.(*collaboration.Share); ok {
						if utils.UserEqual(user.GetId(), s.GetGrantee().GetUserId()) {
							if share.MatchesFilters(s, filters) {
								rs := &collaboration.ReceivedShare{
									Share:      s,
									State:      state.state,
									MountPoint: state.mountPoint,
								}
								rss = append(rss, rs)
							}
						}
					}
				}
			}
		}
	}

	return rss, nil
}

// convert must be called in a lock-controlled block.
func (m *manager) convert(currentUser *userv1beta1.UserId, s *collaboration.Share) *collaboration.ReceivedShare {
	rs := &collaboration.ReceivedShare{
		Share: s,
		State: collaboration.ShareState_SHARE_STATE_PENDING,
	}

	providerid, spaceid, _, err := storagespace.SplitID(s.Id.OpaqueId)
	if err != nil {
		return rs
	}
	spaceId := storagespace.FormatResourceID(provider.ResourceId{
		StorageId: providerid,
		SpaceId:   spaceid,
	})
	v, err := m.userReceivedStates[currentUser.OpaqueId].Get(spaceId)
	if err != nil {
		return rs
	}
	if rspace, ok := v.(receivedSpace); ok {
		if state, ok := rspace.receivedShareStates[s.Id.OpaqueId]; ok {
			rs.State = state.state
			rs.MountPoint = state.mountPoint
		}
	}
	return rs
}

func (m *manager) GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	return m.getReceived(ctx, ref)
}

func (m *manager) getReceived(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()
	s, err := m.get(ref)
	if err != nil {
		return nil, err
	}
	user := ctxpkg.ContextMustGetUser(ctx)
	if !share.IsGrantedToUser(s, user) {
		return nil, errtypes.NotFound(ref.String())
	}
	return m.convert(user.Id, s), nil
}

func (m *manager) UpdateReceivedShare(ctx context.Context, receivedShare *collaboration.ReceivedShare, fieldMask *field_mask.FieldMask) (*collaboration.ReceivedShare, error) {
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

	newState := receivedShareState{
		state:      rs.State,
		mountPoint: rs.MountPoint,
	}

	// write back
	currentUser := ctxpkg.ContextMustGetUser(ctx)
	spaceId := storagespace.FormatResourceID(provider.ResourceId{
		StorageId: rs.Share.ResourceId.StorageId,
		SpaceId:   rs.Share.ResourceId.SpaceId,
	})
	v, err := m.userReceivedStates[currentUser.Id.OpaqueId].Get(spaceId)
	switch {
	case err == gcache.KeyNotFoundError:
		// add new entry
		m.userReceivedStates[currentUser.Id.OpaqueId].Set(spaceId,
			receivedSpace{
				mtime: time.Now().UnixNano(),
				receivedShareStates: map[string]receivedShareState{
					rs.Share.Id.OpaqueId: newState,
				},
			})
	case err != nil:
		// something went horribly wrong
		return nil, err

	default:
		// update entry
		if rspace, ok := v.(receivedSpace); ok {
			rspace.receivedShareStates[rs.Share.Id.OpaqueId] = newState
			rspace.mtime = time.Now().UnixNano()
			m.userReceivedStates[currentUser.Id.OpaqueId].Set(spaceId, rspace)
		}
	}

	return rs, nil
}

// // Dump exports shares and received shares to channels (e.g. during migration)
// func (m *manager) Dump(ctx context.Context, shareChan chan<- *collaboration.Share, receivedShareChan chan<- share.ReceivedShareWithUser) error {
// 	log := appctx.GetLogger(ctx)
// 	for _, s := range m.model.Shares {
// 		shareChan <- s
// 	}

// 	for userIDString, states := range m.model.State {
// 		userMountPoints := m.model.MountPoint[userIDString]
// 		id := &userv1beta1.UserId{}
// 		mV2 := proto.MessageV2(id)
// 		if err := prototext.Unmarshal([]byte(userIDString), mV2); err != nil {
// 			log.Error().Err(err).Msg("error unmarshalling the user id")
// 			continue
// 		}

// 		for shareIDString, state := range states {
// 			sid := &collaboration.ShareId{}
// 			mV2 := proto.MessageV2(sid)
// 			if err := prototext.Unmarshal([]byte(shareIDString), mV2); err != nil {
// 				log.Error().Err(err).Msg("error unmarshalling the user id")
// 				continue
// 			}

// 			var s *collaboration.Share
// 			for _, is := range m.model.Shares {
// 				if is.Id.OpaqueId == sid.OpaqueId {
// 					s = is
// 					break
// 				}
// 			}
// 			if s == nil {
// 				log.Warn().Str("share id", sid.OpaqueId).Msg("Share not found")
// 				continue
// 			}

// 			var mp *provider.Reference
// 			if userMountPoints != nil {
// 				mp = userMountPoints[shareIDString]
// 			}

// 			receivedShareChan <- share.ReceivedShareWithUser{
// 				UserID: id,
// 				ReceivedShare: &collaboration.ReceivedShare{
// 					Share:      s,
// 					State:      state,
// 					MountPoint: mp,
// 				},
// 			}
// 		}
// 	}

// 	return nil
// }
