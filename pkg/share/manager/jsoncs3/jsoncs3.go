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
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/share"
	"github.com/cs3org/reva/v2/pkg/share/manager/jsoncs3/sharecache"
	"github.com/cs3org/reva/v2/pkg/share/manager/registry"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata" // nolint:staticcheck // we need the legacy package to convert V1 to V2 messages
	"github.com/cs3org/reva/v2/pkg/storagespace"
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

type receivedCache map[string]receivedSpaces
type receivedSpaces map[string]*receivedSpace
type receivedSpace struct {
	mtime               int64
	receivedShareStates map[string]receivedShareState
}
type receivedShareState struct {
	state      collaboration.ShareState
	mountPoint *provider.Reference
}

type Manager struct {
	sync.RWMutex

	// Cache holds the all shares, sharded by provider id and space id
	Cache ProviderCache
	// createdCache holds the list of shares a user has created, sharded by user id and space id
	createdCache sharecache.ShareCache
	// groupReceivedCache holds the list of shares a group has access to, sharded by group id and space id
	groupReceivedCache sharecache.ShareCache
	// userReceivedStates holds the state of shares a user has received, sharded by user id and space id
	userReceivedStates receivedCache

	storage     metadata.Storage
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
func New(s metadata.Storage) (*Manager, error) {
	return &Manager{
		Cache:              NewProviderCache(),
		createdCache:       sharecache.New(),
		userReceivedStates: receivedCache{},
		groupReceivedCache: sharecache.New(),
		storage:            s,
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

func (m *Manager) Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error) {

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
	shareID, err := storagespace.FormatReference(shareReference)
	if err != nil {
		return nil, err
	}
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

	m.Cache.Add(md.Id.StorageId, md.Id.SpaceId, shareID, s)

	err = m.setCreatedCache(ctx, s.GetCreator().GetOpaqueId(), shareID)
	if err != nil {
		return nil, err
	}

	spaceID := storagespace.FormatResourceID(provider.ResourceId{
		StorageId: md.Id.StorageId,
		SpaceId:   md.Id.SpaceId,
	})
	// set flag for grantee to have access to share
	switch g.Grantee.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		userid := g.Grantee.GetUserId().GetOpaqueId()
		if m.userReceivedStates[userid] == nil {
			m.userReceivedStates[userid] = receivedSpaces{} // receivedSpaces
		}
		if m.userReceivedStates[userid][spaceID] == nil {
			m.userReceivedStates[userid][spaceID] = &receivedSpace{}
		}
		receivedSpace := m.userReceivedStates[userid][spaceID]

		receivedSpace.mtime = now
		if receivedSpace.receivedShareStates == nil {
			receivedSpace.receivedShareStates = map[string]receivedShareState{}
		}
		receivedSpace.receivedShareStates[shareID] = receivedShareState{
			state: collaboration.ShareState_SHARE_STATE_PENDING,
			// mountpoint stays empty until user accepts the share
		}
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		groupid := g.Grantee.GetGroupId().GetOpaqueId()
		if err := m.groupReceivedCache.Add(groupid, shareID); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// getByID must be called in a lock-controlled block.
func (m *Manager) getByID(id *collaboration.ShareId) (*collaboration.Share, error) {
	shareid, err := storagespace.ParseID(id.OpaqueId)
	if err != nil {
		// invalid share id, does not exist
		return nil, errtypes.NotFound(id.String())
	}
	share := m.Cache.Get(shareid.StorageId, shareid.SpaceId, id.OpaqueId)
	if share == nil {
		return nil, errtypes.NotFound(id.String())
	}
	return share, nil
}

// getByKey must be called in a lock-controlled block.
func (m *Manager) getByKey(key *collaboration.ShareKey) (*collaboration.Share, error) {
	spaceShares := m.Cache.ListSpace(key.ResourceId.StorageId, key.ResourceId.SpaceId)
	for _, share := range spaceShares.Shares {
		if utils.GranteeEqual(key.Grantee, share.Grantee) {
			return share, nil
		}
	}
	return nil, errtypes.NotFound(key.String())
}

// get must be called in a lock-controlled block.
func (m *Manager) get(ref *collaboration.ShareReference) (s *collaboration.Share, err error) {
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

func (m *Manager) GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error) {
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

func (m *Manager) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
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
	m.Cache.Remove(shareid.StorageId, shareid.SpaceId, s.Id.OpaqueId)

	// remove from created cache
	err = m.removeFromCreatedCache(ctx, s.GetCreator().GetOpaqueId(), s.Id.OpaqueId)
	if err != nil {
		return err
	}

	// TODO remove from grantee cache

	return nil
}

func (m *Manager) removeFromCreatedCache(ctx context.Context, creatorid, shareid string) error {
	if err := m.createdCache.Remove(creatorid, shareid); err != nil {
		return err
	}
	createdBytes, err := json.Marshal(m.createdCache.GetShareCache(creatorid))
	if err != nil {
		return err
	}
	// FIXME needs stat & upload if match combo to prevent lost update in redundant deployments
	if err := m.storage.SimpleUpload(ctx, userCreatedPath(creatorid), createdBytes); err != nil {
		return err
	}
	return nil
}

func (m *Manager) UpdateShare(ctx context.Context, ref *collaboration.ShareReference, p *collaboration.SharePermissions) (*collaboration.Share, error) {
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

	err = m.setCreatedCache(ctx, s.GetCreator().GetOpaqueId(), s.Id.OpaqueId)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (m *Manager) setCreatedCache(ctx context.Context, creatorid, shareid string) error {
	if err := m.createdCache.Add(creatorid, shareid); err != nil {
		return err
	}
	createdBytes, err := json.Marshal(m.createdCache.GetShareCache(creatorid))
	if err != nil {
		return err
	}
	// FIXME needs stat & upload if match combo to prevent lost update in redundant deployments
	if err := m.storage.SimpleUpload(ctx, userCreatedPath(creatorid), createdBytes); err != nil {
		return err
	}
	return nil
}

// ListShares
func (m *Manager) ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error) {
	m.Lock()
	defer m.Unlock()

	//log := appctx.GetLogger(ctx)
	user := ctxpkg.ContextMustGetUser(ctx)
	userid := user.Id.OpaqueId
	var ss []*collaboration.Share

	// Q: how do we detect that a created list changed?
	// Option 1: we rely on etag propagation on the storage to bubble up changes in any space to a single created list
	//           - drawback should stop etag propagation at /{userid}/ to prevent further propagation to the root of the share provider space
	//           - we could use the user.ocis.propagation xattr in decomposedfs or the eos equivalent to optimize the storage
	//           - pro: more efficient, more elegant
	//           - con: more complex, does not work on plain posix
	// Option 2: we stat users/{userid}/created.json
	//           - pro: easier to implement, works on plain posix, no folders
	// Can this be hidden behind the metadata storage interface?
	// Decision: use touch for now as it works withe plain posix and is easier to test

	// TODO check if a created or owned filter is set

	var mtime time.Time
	//  - do we have a cached list of created shares for the user in memory?
	if usc := m.createdCache.GetShareCache(userid); usc != nil {
		mtime = usc.Mtime
		//    - y: set If-Modified-Since header to only download if it changed
	} else {
		mtime = time.Time{} // Set zero time so that data from storage always takes precedence
	}

	//  - read /users/{userid}/created.json (with If-Modified-Since header) aka read if changed
	userCreatedPath := userCreatedPath(userid)
	info, err := m.storage.Stat(ctx, userCreatedPath)
	if err != nil {
		// TODO check other cases, we currently only assume it does not exist
		return ss, nil
	}
	// check mtime of /users/{userid}/created.json
	if utils.TSToTime(info.Mtime).After(mtime) {
		//  - update cached list of created shares for the user in memory if changed
		createdBlob, err := m.storage.SimpleDownload(ctx, userCreatedPath)
		if err == nil {
			newShareCache := sharecache.UserShareCache{}
			err := json.Unmarshal(createdBlob, &newShareCache)
			if err != nil {
				// TODO log error but continue?
				// data corrupted, admin needs to take action
				// the service still has data. dump it before ding?
			}
			m.createdCache.SetShareCache(userid, &newShareCache)
		} else {
			// TODO log error but continue with current cached data
		}
	}

	for ssid, spaceShareIDs := range m.createdCache.List(user.Id.OpaqueId) {
		if time.Now().Sub(spaceShareIDs.Mtime) > time.Second*30 {
			// TODO reread from disk
		}
		providerid, spaceid, _, err := storagespace.SplitID(ssid)
		if err != nil {
			continue
		}
		spaceShares := m.Cache.ListSpace(providerid, spaceid)
		for shareid, _ := range spaceShareIDs.IDs {
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

func userCreatedPath(userid string) string {
	userCreatedPath := filepath.Join("/users", userid, "created.json")
	return userCreatedPath
}

// we list the shares that are targeted to the user in context or to the user groups.
func (m *Manager) ListReceivedShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()

	var rss []*collaboration.ReceivedShare
	user := ctxpkg.ContextMustGetUser(ctx)

	// Q: how do we detect that a received list changed?
	// - similar to the created.json we stat and download a received.json
	// con: when adding a received share we MUST have if-match for the initiate-file-upload request
	//      to ensure consistency / prevent lost updates

	ssids := map[string]*receivedSpace{}

	// first collect all spaceids the user has access to as a group member
	for _, group := range user.Groups {
		for ssid, spaceShareIDs := range m.groupReceivedCache.List(group) {
			if time.Now().Sub(spaceShareIDs.Mtime) > time.Second*30 {
				// TODO reread from disk
			}
			// add a pending entry, the state will be updated
			// when reading the received shares below if they have already been accepted or denied
			rs := receivedSpace{
				mtime:               spaceShareIDs.Mtime.UnixNano(),
				receivedShareStates: make(map[string]receivedShareState, len(spaceShareIDs.IDs)),
			}

			for shareid, _ := range spaceShareIDs.IDs {
				rs.receivedShareStates[shareid] = receivedShareState{
					state: collaboration.ShareState_SHARE_STATE_PENDING,
				}
			}
			ssids[ssid] = &rs
		}
	}

	// add all spaces the user has receved shares for, this includes mount points and share state for groups
	for ssid, rspace := range m.userReceivedStates[user.Id.OpaqueId] {
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
		spaceShares := m.Cache.ListSpace(providerid, spaceid)
		for shareId, state := range rspace.receivedShareStates {
			s := spaceShares.Shares[shareId]
			if s == nil {
				continue
			}
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

	return rss, nil
}

// convert must be called in a lock-controlled block.
func (m *Manager) convert(currentUser *userv1beta1.UserId, s *collaboration.Share) *collaboration.ReceivedShare {
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
	if rspace := m.userReceivedStates[currentUser.OpaqueId][spaceId]; rspace != nil {
		if state, ok := rspace.receivedShareStates[s.Id.OpaqueId]; ok {
			rs.State = state.state
			rs.MountPoint = state.mountPoint
		}
	}
	return rs
}

func (m *Manager) GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	return m.getReceived(ctx, ref)
}

func (m *Manager) getReceived(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
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

func (m *Manager) UpdateReceivedShare(ctx context.Context, receivedShare *collaboration.ReceivedShare, fieldMask *field_mask.FieldMask) (*collaboration.ReceivedShare, error) {
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
	rspace := m.userReceivedStates[currentUser.Id.OpaqueId][spaceId]
	if rspace == nil {
		m.userReceivedStates[currentUser.Id.OpaqueId][spaceId] =
			&receivedSpace{
				receivedShareStates: map[string]receivedShareState{},
			}

	}
	// update entry
	rspace.receivedShareStates[rs.Share.Id.OpaqueId] = newState
	rspace.mtime = time.Now().UnixNano()

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
