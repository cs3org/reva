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
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bluele/gcache"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/share"
	"github.com/golang/protobuf/proto" // nolint:staticcheck // we need the legacy package to convert V1 to V2 messages
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/prototext"

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
	registry.Register("jsoncs3", New)
}

// New returns a new mgr.
func New(m map[string]interface{}) (share.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	if c.GatewayAddr == "" {
		return nil, errors.New("share manager config is missing gateway address")
	}

	c.init()

	// load or create file
	model, err := loadOrCreate(c.File)
	if err != nil {
		err = errors.Wrap(err, "error loading the file containing the shares")
		return nil, err
	}

	return &mgr{
		c:          c,
		model:      model,
		spaceETags: gcache.New(1000000).LFU().Build(),
	}, nil

}

func loadOrCreate(file string) (*shareModel, error) {
	if info, err := os.Stat(file); errors.Is(err, fs.ErrNotExist) || info.Size() == 0 {
		if err := ioutil.WriteFile(file, []byte("{}"), 0700); err != nil {
			err = errors.Wrap(err, "error opening/creating the file: "+file)
			return nil, err
		}
	}

	fd, err := os.OpenFile(file, os.O_CREATE, 0644)
	if err != nil {
		err = errors.Wrap(err, "error opening/creating the file: "+file)
		return nil, err
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		err = errors.Wrap(err, "error reading the data")
		return nil, err
	}

	j := &jsonEncoding{}
	if err := json.Unmarshal(data, j); err != nil {
		err = errors.Wrap(err, "error decoding data from json")
		return nil, err
	}

	m := &shareModel{State: j.State, MountPoint: j.MountPoint}
	for _, s := range j.Shares {
		var decShare collaboration.Share
		if err = utils.UnmarshalJSONToProtoV1([]byte(s), &decShare); err != nil {
			return nil, errors.Wrap(err, "error decoding share from json")
		}
		m.Shares = append(m.Shares, &decShare)
	}

	if m.State == nil {
		m.State = map[string]map[string]collaboration.ShareState{}
	}
	if m.MountPoint == nil {
		m.MountPoint = map[string]map[string]*provider.Reference{}
	}

	m.file = file
	return m, nil
}

type shareModel struct {
	file       string
	State      map[string]map[string]collaboration.ShareState `json:"state"`       // map[username]map[share_id]ShareState
	MountPoint map[string]map[string]*provider.Reference      `json:"mount_point"` // map[username]map[share_id]MountPoint
	Shares     []*collaboration.Share                         `json:"shares"`
}

type jsonEncoding struct {
	State      map[string]map[string]collaboration.ShareState `json:"state"`       // map[username]map[share_id]ShareState
	MountPoint map[string]map[string]*provider.Reference      `json:"mount_point"` // map[username]map[share_id]MountPoint
	Shares     []string                                       `json:"shares"`
}

func (m *shareModel) Save() error {
	j := &jsonEncoding{State: m.State, MountPoint: m.MountPoint}
	for _, s := range m.Shares {
		encShare, err := utils.MarshalProtoV1ToJSON(s)
		if err != nil {
			return errors.Wrap(err, "error encoding to json")
		}
		j.Shares = append(j.Shares, string(encShare))
	}

	data, err := json.Marshal(j)
	if err != nil {
		err = errors.Wrap(err, "error encoding to json")
		return err
	}

	if err := ioutil.WriteFile(m.file, data, 0644); err != nil {
		err = errors.Wrap(err, "error writing to file: "+m.file)
		return err
	}

	return nil
}

type mgr struct {
	c           *config
	sync.Mutex  // concurrent access to the file
	model       *shareModel
	serviceUser *userv1beta1.User
	SpaceRoot   *provider.ResourceId
	spaceETags  gcache.Cache

	initialized bool
}

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

func (m *mgr) initialize(ctx context.Context) error {
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

	user := ctxpkg.ContextMustGetUser(ctx)

	client, err := m.metadataClient()
	if err != nil {
		return err
	}

	ctx, err = m.getAuthContext(context.Background())
	if err != nil {
		return err
	}
	spaceid := "shares-space"

	// FIXME only create space if ListContainer fails
	cssr, err := client.CreateStorageSpace(ctx, &provider.CreateStorageSpaceRequest{
		Opaque: &typespb.Opaque{
			Map: map[string]*typespb.OpaqueEntry{
				"spaceid": {
					Decoder: "plain",
					Value:   []byte(spaceid),
				},
			},
		},
		Owner: m.serviceUser,
		Name:  "Shares",
		Type:  "metadata",
	})
	switch {
	case err != nil:
		return err
	case cssr.Status.Code == rpcv1beta1.Code_CODE_OK:
		m.SpaceRoot = cssr.StorageSpace.Root
	case cssr.Status.Code == rpcv1beta1.Code_CODE_ALREADY_EXISTS:
		// TODO make CreateStorageSpace return existing space?
		m.SpaceRoot = &provider.ResourceId{SpaceId: spaceid, OpaqueId: spaceid}
	default:
		return errtypes.NewErrtypeFromStatus(cssr.Status)
	}

	// we list /{userid}/* || /{userid}/received ? created?
	res, err := client.ListContainer(ctx, &provider.ListContainerRequest{
		Ref: &provider.Reference{ResourceId: m.SpaceRoot, Path: filepath.Join(user.Id.OpaqueId, "created")},
	})

	switch {
	case err != nil:
		return err
	case cssr.Status.Code == rpcv1beta1.Code_CODE_OK:
		// for every space we fetch /{storageid}/{spaceid}.json if we
		//    have not cached it yet, or if the /{userid}/created/{storageid}${spaceid} etag changed
		for _, storageInfo := range res.Infos {
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
				client.TouchFile(ctx, &provider.TouchFileRequest{
					Ref:    &provider.Reference{},
					Opaque: &typespb.Opaque{ /*TODO allow setting the mtime with touch*/ },
				})
				// maybe we need SetArbitraryMetadata to set the mtime
			}
			//
			// TODO use space if etag is same
		}
	case cssr.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
		// if it does not exist we query the registry for every storage provider id, then
		// we traverse /{storageid}/ in the metadata storage to
		//  1. create /{userid}/
		//  3. touch /{userid}/{storageid}/{spaceid}
		//  2. touch /{userid}/{storageid} (not needed when mtime propagation is enabled)
	default:
		return errtypes.NewErrtypeFromStatus(cssr.Status)
	}

	return nil
}

func (m *mgr) getCachedSpaceETag(spaceid string) string {
	if e, err := m.spaceETags.Get(spaceid); err != gcache.KeyNotFoundError {
		if etag, ok := e.(string); ok {
			return etag
		}
	}
	return ""
}

func (m *mgr) metadataClient() (provider.ProviderAPIClient, error) {
	return pool.GetStorageProviderServiceClient(m.c.ProviderAddr)
}

func (m *mgr) getAuthContext(ctx context.Context) (context.Context, error) {
	client, err := pool.GetGatewayServiceClient(m.c.GatewayAddr)
	if err != nil {
		return nil, err
	}

	authCtx := ctxpkg.ContextSetUser(context.Background(), m.serviceUser)
	authRes, err := client.Authenticate(authCtx, &gateway.AuthenticateRequest{
		Type:         "machine",
		ClientId:     "userid:" + m.serviceUser.Id.OpaqueId,
		ClientSecret: m.c.MachineAuthAPIKey,
	})
	if err != nil {
		return nil, err
	}
	if authRes.GetStatus().GetCode() != rpcv1beta1.Code_CODE_OK {
		return nil, errtypes.NewErrtypeFromStatus(authRes.GetStatus())
	}
	authCtx = metadata.AppendToOutgoingContext(authCtx, ctxpkg.TokenHeader, authRes.Token)
	return authCtx, nil
}

func (m *mgr) getClient() (gateway.GatewayAPIClient, error) {
	return pool.GetGatewayServiceClient(m.c.GatewayAddr)
}

type config struct {
	File        string `mapstructure:"file"`
	GatewayAddr string `mapstructure:"gateway_addr"`
	// ProviderAddr is the address of the metadata storage provider
	ProviderAddr      string `mapstructure:"provider_addr"`
	ServiceUser       string `mapstructure:"service_user"`
	MachineAuthAPIKey string `mapstructure:"machine_auth_api_key"`
}

func (c *config) init() {
	if c.File == "" {
		c.File = "/var/tmp/reva/shares.json"
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (m *mgr) Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error) {
	id := uuid.NewString()
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
		Owner:      md.Owner,
		ResourceId: md.Id,
		Grantee:    g.Grantee,
	}

	m.Lock()
	defer m.Unlock()
	_, _, err := m.getByKey(key)
	if err == nil {
		// share already exists
		return nil, errtypes.AlreadyExists(key.String())
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

	m.model.Shares = append(m.model.Shares, s)
	if err := m.model.Save(); err != nil {
		err = errors.Wrap(err, "error saving model")
		return nil, err
	}

	return s, nil
}

// getByID must be called in a lock-controlled block.
func (m *mgr) getByID(id *collaboration.ShareId) (int, *collaboration.Share, error) {
	for i, s := range m.model.Shares {
		if s.GetId().OpaqueId == id.OpaqueId {
			return i, s, nil
		}
	}
	return -1, nil, errtypes.NotFound(id.String())
}

// getByKey must be called in a lock-controlled block.
func (m *mgr) getByKey(key *collaboration.ShareKey) (int, *collaboration.Share, error) {
	for i, s := range m.model.Shares {
		if (utils.UserEqual(key.Owner, s.Owner) || utils.UserEqual(key.Owner, s.Creator)) &&
			utils.ResourceIDEqual(key.ResourceId, s.ResourceId) && utils.GranteeEqual(key.Grantee, s.Grantee) {
			return i, s, nil
		}
	}
	return -1, nil, errtypes.NotFound(key.String())
}

// get must be called in a lock-controlled block.
func (m *mgr) get(ref *collaboration.ShareReference) (idx int, s *collaboration.Share, err error) {
	switch {
	case ref.GetId() != nil:
		idx, s, err = m.getByID(ref.GetId())
	case ref.GetKey() != nil:
		idx, s, err = m.getByKey(ref.GetKey())
	default:
		err = errtypes.NotFound(ref.String())
	}
	return
}

func (m *mgr) GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error) {
	m.Lock()
	defer m.Unlock()
	_, s, err := m.get(ref)
	if err != nil {
		return nil, err
	}
	// check if we are the owner or the grantee
	user := ctxpkg.ContextMustGetUser(ctx)
	if share.IsCreatedByUser(s, user) || share.IsGrantedToUser(s, user) {
		return s, nil
	}
	// we return not found to not disclose information
	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
	m.Lock()
	defer m.Unlock()
	user := ctxpkg.ContextMustGetUser(ctx)

	idx, s, err := m.get(ref)
	if err != nil {
		return err
	}
	if !share.IsCreatedByUser(s, user) {
		return errtypes.NotFound(ref.String())
	}

	last := len(m.model.Shares) - 1
	m.model.Shares[idx] = m.model.Shares[last]
	// explicitly nil the reference to prevent memory leaks
	// https://github.com/golang/go/wiki/SliceTricks#delete-without-preserving-order
	m.model.Shares[last] = nil
	m.model.Shares = m.model.Shares[:last]
	if err := m.model.Save(); err != nil {
		err = errors.Wrap(err, "error saving model")
		return err
	}
	return nil
}

func (m *mgr) UpdateShare(ctx context.Context, ref *collaboration.ShareReference, p *collaboration.SharePermissions) (*collaboration.Share, error) {
	m.Lock()
	defer m.Unlock()
	idx, s, err := m.get(ref)
	if err != nil {
		return nil, err
	}

	user := ctxpkg.ContextMustGetUser(ctx)
	if !share.IsCreatedByUser(s, user) {
		return nil, errtypes.NotFound(ref.String())
	}

	now := time.Now().UnixNano()
	m.model.Shares[idx].Permissions = p
	m.model.Shares[idx].Mtime = &typespb.Timestamp{
		Seconds: uint64(now / int64(time.Second)),
		Nanos:   uint32(now % int64(time.Second)),
	}

	if err := m.model.Save(); err != nil {
		err = errors.Wrap(err, "error saving model")
		return nil, err
	}
	return m.model.Shares[idx], nil
}

func (m *mgr) ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error) {
	if err := m.initialize(ctx); err != nil {
		return nil, err
	}

	m.Lock()
	defer m.Unlock()
	log := appctx.GetLogger(ctx)
	user := ctxpkg.ContextMustGetUser(ctx)

	client, err := pool.GetGatewayServiceClient(m.c.GatewayAddr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list shares")
	}
	cache := make(map[string]struct{})
	var ss []*collaboration.Share
	for _, s := range m.model.Shares {
		if share.MatchesFilters(s, filters) {
			// Only add the share if the share was created by the user or if
			// the user has ListGrants permissions on the shared resource.
			// The ListGrants check is necessary when a space member wants
			// to list shares in a space.
			// We are using a cache here so that we don't have to stat a
			// resource multiple times.
			key := strings.Join([]string{s.ResourceId.StorageId, s.ResourceId.OpaqueId}, "!")
			if _, hit := cache[key]; !hit && !share.IsCreatedByUser(s, user) {
				sRes, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: s.ResourceId}})
				if err != nil || sRes.Status.Code != rpcv1beta1.Code_CODE_OK {
					log.Error().
						Err(err).
						Interface("status", sRes.Status).
						Interface("resource_id", s.ResourceId).
						Msg("ListShares: could not stat resource")
					continue
				}
				if !sRes.Info.PermissionSet.ListGrants {
					continue
				}
				cache[key] = struct{}{}
			}
			ss = append(ss, s)
		}
	}
	return ss, nil
}

// we list the shares that are targeted to the user in context or to the user groups.
func (m *mgr) ListReceivedShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()

	user := ctxpkg.ContextMustGetUser(ctx)
	mem := make(map[string]int)
	var rss []*collaboration.ReceivedShare
	for _, s := range m.model.Shares {
		if !share.IsCreatedByUser(s, user) &&
			share.IsGrantedToUser(s, user) &&
			share.MatchesFilters(s, filters) {

			rs := m.convert(user.Id, s)
			idx, seen := mem[s.ResourceId.OpaqueId]
			if !seen {
				rss = append(rss, rs)
				mem[s.ResourceId.OpaqueId] = len(rss) - 1
				continue
			}

			// When we arrive here there was already a share for this resource.
			// if there is a mix-up of shares of type group and shares of type user we need to deduplicate them, since it points
			// to the same resource. Leave the more explicit and hide the less explicit. In this case we hide the group shares
			// and return the user share to the user.
			other := rss[idx]
			if other.Share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP && s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER {
				if other.State == rs.State {
					rss[idx] = rs
				} else {
					rss = append(rss, rs)
				}
			}
		}
	}

	return rss, nil
}

// convert must be called in a lock-controlled block.
func (m *mgr) convert(currentUser *userv1beta1.UserId, s *collaboration.Share) *collaboration.ReceivedShare {
	rs := &collaboration.ReceivedShare{
		Share: s,
		State: collaboration.ShareState_SHARE_STATE_PENDING,
	}
	if v, ok := m.model.State[currentUser.String()]; ok {
		if state, ok := v[s.Id.String()]; ok {
			rs.State = state
		}
	}
	if v, ok := m.model.MountPoint[currentUser.String()]; ok {
		if mp, ok := v[s.Id.String()]; ok {
			rs.MountPoint = mp
		}
	}
	return rs
}

func (m *mgr) GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	return m.getReceived(ctx, ref)
}

func (m *mgr) getReceived(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()
	_, s, err := m.get(ref)
	if err != nil {
		return nil, err
	}
	user := ctxpkg.ContextMustGetUser(ctx)
	if !share.IsGrantedToUser(s, user) {
		return nil, errtypes.NotFound(ref.String())
	}
	return m.convert(user.Id, s), nil
}

func (m *mgr) UpdateReceivedShare(ctx context.Context, receivedShare *collaboration.ReceivedShare, fieldMask *field_mask.FieldMask) (*collaboration.ReceivedShare, error) {
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

	user := ctxpkg.ContextMustGetUser(ctx)
	// Persist state
	if v, ok := m.model.State[user.Id.String()]; ok {
		v[rs.Share.Id.String()] = rs.State
		m.model.State[user.Id.String()] = v
	} else {
		a := map[string]collaboration.ShareState{
			rs.Share.Id.String(): rs.State,
		}
		m.model.State[user.Id.String()] = a
	}

	// Persist mount point
	if v, ok := m.model.MountPoint[user.Id.String()]; ok {
		v[rs.Share.Id.String()] = rs.MountPoint
		m.model.MountPoint[user.Id.String()] = v
	} else {
		a := map[string]*provider.Reference{
			rs.Share.Id.String(): rs.MountPoint,
		}
		m.model.MountPoint[user.Id.String()] = a
	}

	if err := m.model.Save(); err != nil {
		err = errors.Wrap(err, "error saving model")
		return nil, err
	}

	return rs, nil
}

// Dump exports shares and received shares to channels (e.g. during migration)
func (m *mgr) Dump(ctx context.Context, shareChan chan<- *collaboration.Share, receivedShareChan chan<- share.ReceivedShareWithUser) error {
	log := appctx.GetLogger(ctx)
	for _, s := range m.model.Shares {
		shareChan <- s
	}

	for userIDString, states := range m.model.State {
		userMountPoints := m.model.MountPoint[userIDString]
		id := &userv1beta1.UserId{}
		mV2 := proto.MessageV2(id)
		if err := prototext.Unmarshal([]byte(userIDString), mV2); err != nil {
			log.Error().Err(err).Msg("error unmarshalling the user id")
			continue
		}

		for shareIDString, state := range states {
			sid := &collaboration.ShareId{}
			mV2 := proto.MessageV2(sid)
			if err := prototext.Unmarshal([]byte(shareIDString), mV2); err != nil {
				log.Error().Err(err).Msg("error unmarshalling the user id")
				continue
			}

			var s *collaboration.Share
			for _, is := range m.model.Shares {
				if is.Id.OpaqueId == sid.OpaqueId {
					s = is
					break
				}
			}
			if s == nil {
				log.Warn().Str("share id", sid.OpaqueId).Msg("Share not found")
				continue
			}

			var mp *provider.Reference
			if userMountPoints != nil {
				mp = userMountPoints[shareIDString]
			}

			receivedShareChan <- share.ReceivedShareWithUser{
				UserID: id,
				ReceivedShare: &collaboration.ReceivedShare{
					Share:      s,
					State:      state,
					MountPoint: mp,
				},
			}
		}
	}

	return nil
}
