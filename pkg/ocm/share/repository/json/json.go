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

package json

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/repository/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"
)

func init() {
	registry.Register("json", New)
}

// New returns a new authorizer object.
func New(m map[string]interface{}) (share.Repository, error) {
	c, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}
	c.init()

	// load or create file
	model, err := loadOrCreate(c.File)
	if err != nil {
		err = errors.Wrap(err, "error loading the file containing the shares")
		return nil, err
	}

	mgr := &mgr{
		c:     c,
		model: model,
	}

	return mgr, nil
}

func loadOrCreate(file string) (*shareModel, error) {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		if err := os.WriteFile(file, []byte("{}"), 0700); err != nil {
			return nil, errors.Wrap(err, "error creating the file: "+file)
		}
	}

	f, err := os.OpenFile(file, os.O_RDONLY, 0644)
	if err != nil {
		err = errors.Wrap(err, "error opening the file: "+file)
		return nil, err
	}
	defer f.Close()

	var m shareModel
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return nil, errors.Wrap(err, "error decoding data to json")
	}

	if m.Shares == nil {
		m.Shares = map[string]*ocm.Share{}
	}
	if m.ReceivedShares == nil {
		m.ReceivedShares = map[string]*ocm.ReceivedShare{}
	}
	m.file = file

	return &m, nil
}

type shareModel struct {
	file           string                        `json:"-"`
	Shares         map[string]*ocm.Share         `json:"shares"`          // share_id -> share
	ReceivedShares map[string]*ocm.ReceivedShare `json:"received_shares"` // share_id -> share
}

type config struct {
	File                string `mapstructure:"file"`
	InsecureConnections bool   `mapstructure:"insecure_connections"`
}

func (c *config) init() {
	if c.File == "" {
		c.File = "/var/tmp/reva/ocm-shares.json"
	}
}

type mgr struct {
	c          *config
	sync.Mutex // concurrent access to the file
	model      *shareModel
}

func (m *shareModel) save() error {
	f, err := os.OpenFile(m.file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return errors.Wrap(err, "error opening file "+m.file)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(m); err != nil {
		return errors.Wrap(err, "error encoding to json")
	}

	return nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func genID() string {
	return uuid.New().String()
}

func (m *mgr) StoreShare(ctx context.Context, share *ocm.Share) (*ocm.Share, error) {
	m.Lock()
	defer m.Unlock()

	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	share.Id = &ocm.ShareId{
		OpaqueId: genID(),
	}
	share.Mtime = ts
	share.Ctime = ts

	m.model.Shares[share.Id.OpaqueId] = cloneShare(share)

	if err := m.model.save(); err != nil {
		return nil, errors.Wrap(err, "error saving share")
	}

	return share, nil
}

func cloneShare(s *ocm.Share) *ocm.Share {
	d, err := utils.MarshalProtoV1ToJSON(s)
	if err != nil {
		panic(err)
	}
	var cloned ocm.Share
	if err := utils.UnmarshalJSONToProtoV1(d, &cloned); err != nil {
		panic(err)
	}
	return &cloned
}

func cloneReceivedShare(s *ocm.ReceivedShare) *ocm.ReceivedShare {
	d, err := utils.MarshalProtoV1ToJSON(s)
	if err != nil {
		panic(err)
	}
	var cloned ocm.ReceivedShare
	if err := utils.UnmarshalJSONToProtoV1(d, &cloned); err != nil {
		panic(err)
	}
	return &cloned
}

func (m *mgr) GetShare(ctx context.Context, user *userpb.UserId, ref *ocm.ShareReference) (*ocm.Share, error) {
	var (
		s   *ocm.Share
		err error
	)

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
	if utils.UserEqual(user, s.Owner) || utils.UserEqual(user, s.Creator) {
		return s, nil
	}

	return nil, errtypes.NotFound(ref.String())
}

// // Called from both grpc CreateOCMShare for outgoing
// // and http /ocm/shares for incoming
// // pi is provider info
// // pm is permissions.
// func (m *mgr) Share(ctx context.Context, md *provider.ResourceId, g *ocm.ShareGrant, name string,
// 	pi *ocmprovider.ProviderInfo, pm string, owner *userpb.UserId, token string, st ocm.Share_ShareType) (*ocm.Share, error) {
// 	id := genID()
// 	now := time.Now().UnixNano()
// 	ts := &typespb.Timestamp{
// 		Seconds: uint64(now / 1000000000),
// 		Nanos:   uint32(now % 1000000000),
// 	}

// 	// Since both OCMCore and OCMShareProvider use the same package, we distinguish
// 	// between calls received from them on the basis of whether they provide info
// 	// about the remote provider on which the share is to be created.
// 	// If this info is provided, this call is on the owner's mesh provider and so
// 	// we call the CreateOCMCoreShare method on the remote provider as well,
// 	// else this is received from another provider and we only create a local share.
// 	var isOwnersMeshProvider bool
// 	if pi != nil {
// 		isOwnersMeshProvider = true
// 	}

// 	var userID *userpb.UserId
// 	if !isOwnersMeshProvider {
// 		// Since this call is on the remote provider, the owner of the resource is expected to be specified.
// 		if owner == nil {
// 			return nil, errors.New("json: owner of resource not provided")
// 		}
// 		userID = owner
// 		g.Grantee.Opaque = &typespb.Opaque{
// 			Map: map[string]*typespb.OpaqueEntry{
// 				"token": {
// 					Decoder: "plain",
// 					Value:   []byte(token),
// 				},
// 			},
// 		}
// 	} else {
// 		userID = ctxpkg.ContextMustGetUser(ctx).GetId()
// 	}

// 	// do not allow share to myself if share is for a user
// 	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(g.Grantee.GetUserId(), userID) {
// 		return nil, errors.New("json: user and grantee are the same")
// 	}

// 	// check if share already exists.
// 	key := &ocm.ShareKey{
// 		Owner:      userID,
// 		ResourceId: md,
// 		Grantee:    g.Grantee,
// 	}
// 	_, err := m.getByKey(ctx, key)

// 	// share already exists
// 	if isOwnersMeshProvider && err == nil {
// 		return nil, errtypes.AlreadyExists(key.String())
// 	}

// 	s := &ocm.Share{
// 		Id: &ocm.ShareId{
// 			OpaqueId: id,
// 		},
// 		Name:        name,
// 		ResourceId:  md,
// 		Permissions: g.Permissions,
// 		Grantee:     g.Grantee,
// 		Owner:       userID,
// 		Creator:     userID,
// 		Ctime:       ts,
// 		Mtime:       ts,
// 		ShareType:   st,
// 	}

// 	if isOwnersMeshProvider {
// 		protocol := map[string]interface{}{
// 			"name": "webdav",
// 			"options": map[string]string{
// 				"permissions": pm,
// 				"token":       ctxpkg.ContextMustGetToken(ctx),
// 			},
// 		}
// 		if st == ocm.Share_SHARE_TYPE_TRANSFER {
// 			protocol["name"] = "datatx"
// 		}

// 		requestBodyMap := map[string]interface{}{
// 			"shareWith":    g.Grantee.GetUserId().OpaqueId,
// 			"name":         name,
// 			"providerId":   fmt.Sprintf("%s:%s", md.StorageId, md.OpaqueId),
// 			"owner":        fmt.Sprintf("%s@%s", userID.OpaqueId, userID.Idp),
// 			"protocol":     protocol,
// 			"meshProvider": userID.Idp,
// 		}
// 		err = sender.Send(ctx, requestBodyMap, pi)
// 		if err != nil {
// 			err = errors.Wrap(err, "error sending OCM POST")
// 			return nil, err
// 		}
// 	}

// 	m.Lock()
// 	if err := m.model.ReadFile(); err != nil {
// 		err = errors.Wrap(err, "error reading model")
// 		return nil, err
// 	}

// 	if isOwnersMeshProvider {
// 		encShare, err := utils.MarshalProtoV1ToJSON(s)
// 		if err != nil {
// 			return nil, err
// 		}
// 		m.model.Shares[s.Id.OpaqueId] = string(encShare)
// 	} else {
// 		encShare, err := utils.MarshalProtoV1ToJSON(&ocm.ReceivedShare{
// 			Share: s,
// 			State: ocm.ShareState_SHARE_STATE_PENDING,
// 		})
// 		if err != nil {
// 			return nil, err
// 		}
// 		m.model.ReceivedShares[s.Id.OpaqueId] = string(encShare)
// 	}

// 	if err := m.model.Save(); err != nil {
// 		err = errors.Wrap(err, "error saving model")
// 		return nil, err
// 	}
// 	m.Unlock()

// 	return s, nil
// }

func (m *mgr) getByID(ctx context.Context, id *ocm.ShareId) (*ocm.Share, error) {
	m.Lock()
	defer m.Unlock()

	if share, ok := m.model.Shares[id.OpaqueId]; ok {
		return share, nil
	}
	return nil, errtypes.NotFound(id.String())
}

func (m *mgr) getByKey(ctx context.Context, key *ocm.ShareKey) (*ocm.Share, error) {
	m.Lock()
	defer m.Unlock()

	for _, share := range m.model.Shares {
		if (utils.UserEqual(key.Owner, share.Owner) || utils.UserEqual(key.Owner, share.Creator)) &&
			utils.ResourceIDEqual(key.ResourceId, share.ResourceId) && utils.GranteeEqual(key.Grantee, share.Grantee) {
			return share, nil
		}
	}
	return nil, errtypes.NotFound(key.String())
}

func (m *mgr) DeleteShare(ctx context.Context, user *userpb.UserId, ref *ocm.ShareReference) error {
	m.Lock()
	defer m.Unlock()

	for id, share := range m.model.Shares {
		if sharesEqual(ref, share) {
			if utils.UserEqual(user, share.Owner) || utils.UserEqual(user, share.Creator) {
				delete(m.model.Shares, id)
				if err := m.model.save(); err != nil {
					return err
				}
				return nil
			}
		}
	}
	return errtypes.NotFound(ref.String())
}

func sharesEqual(ref *ocm.ShareReference, s *ocm.Share) bool {
	if ref.GetId() != nil && s.Id != nil {
		if ref.GetId().OpaqueId == s.Id.OpaqueId {
			return true
		}
	} else if ref.GetKey() != nil {
		if (utils.UserEqual(ref.GetKey().Owner, s.Owner) || utils.UserEqual(ref.GetKey().Owner, s.Creator)) &&
			utils.ResourceIDEqual(ref.GetKey().ResourceId, s.ResourceId) && utils.GranteeEqual(ref.GetKey().Grantee, s.Grantee) {
			return true
		}
	}
	return false
}

func receivedShareEqual(ref *ocm.ShareReference, s *ocm.ReceivedShare) bool {
	if ref.GetId() != nil && s.Id != nil {
		if ref.GetId().OpaqueId == s.Id.OpaqueId {
			return true
		}
	} else if ref.GetKey() != nil {

		if (utils.UserEqual(ref.GetKey().Owner, s.Owner) || utils.UserEqual(ref.GetKey().Owner, s.Creator)) &&
			utils.ResourceIDEqual(ref.GetKey().ResourceId, s.ResourceId) && utils.GranteeEqual(ref.GetKey().Grantee, s.Grantee) {
			return true
		}
	}
	return false
}

func (m *mgr) UpdateShare(ctx context.Context, user *userpb.UserId, ref *ocm.ShareReference, p *ocm.SharePermissions) (*ocm.Share, error) {
	return nil, errtypes.NotSupported("not yet implemented")
}

func (m *mgr) ListShares(ctx context.Context, user *userpb.UserId, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {
	var ss []*ocm.Share

	m.Lock()
	defer m.Unlock()

	for _, share := range m.model.Shares {
		if utils.UserEqual(user, share.Owner) || utils.UserEqual(user, share.Creator) {
			// no filter we return earlier
			if len(filters) == 0 {
				ss = append(ss, share)
			} else {
				// check filters
				// TODO(labkode): add the rest of filters.
				for _, f := range filters {
					if f.Type == ocm.ListOCMSharesRequest_Filter_TYPE_RESOURCE_ID {
						if utils.ResourceIDEqual(share.ResourceId, f.GetResourceId()) {
							ss = append(ss, share)
						}
					}
				}
			}
		}
	}
	return ss, nil
}

func (m *mgr) StoreReceivedShare(ctx context.Context, share *ocm.ReceivedShare) (*ocm.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()

	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	share.Id = &ocm.ShareId{
		OpaqueId: genID(),
	}
	share.Ctime = ts
	share.Mtime = ts

	m.model.ReceivedShares[share.Id.OpaqueId] = cloneReceivedShare(share)

	return share, nil
}

func (m *mgr) ListReceivedShares(ctx context.Context, user *userpb.UserId) ([]*ocm.ReceivedShare, error) {
	var rss []*ocm.ReceivedShare
	m.Lock()
	defer m.Unlock()

	for _, share := range m.model.ReceivedShares {
		if utils.UserEqual(user, share.Owner) || utils.UserEqual(user, share.Creator) {
			// omit shares created by me
			continue
		}
		if share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user, share.Grantee.GetUserId()) {
			rss = append(rss, share)
		}
	}
	return rss, nil
}

func (m *mgr) GetReceivedShare(ctx context.Context, user *userpb.UserId, ref *ocm.ShareReference) (*ocm.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()

	for _, share := range m.model.ReceivedShares {
		if receivedShareEqual(ref, share) {
			if share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user, share.Grantee.GetUserId()) {
				return share, nil
			}
		}
	}
	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) UpdateReceivedShare(ctx context.Context, user *userpb.UserId, share *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*ocm.ReceivedShare, error) {
	rs, err := m.GetReceivedShare(ctx, user, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: share.Id}})
	if err != nil {
		return nil, err
	}

	m.Lock()
	defer m.Unlock()

	for _, mask := range fieldMask.Paths {
		switch mask {
		case "state":
			rs.State = share.State
		// TODO case "mount_point":
		default:
			return nil, errtypes.NotSupported("updating " + mask + " is not supported")
		}
	}

	if err := m.model.save(); err != nil {
		return nil, errors.Wrap(err, "error saving model")
	}

	return rs, nil
}
