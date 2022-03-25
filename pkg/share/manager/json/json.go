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

package json

import (
	"context"
	"encoding/json"
	"io/fs"
	"io/ioutil"
	"os"
	"sync"
	"time"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/share"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"

	"github.com/cs3org/reva/v2/pkg/share/manager/registry"
	"github.com/cs3org/reva/v2/pkg/utils"
)

func init() {
	registry.Register("json", New)
}

// New returns a new mgr.
func New(m map[string]interface{}) (share.Manager, error) {
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

	return &mgr{
		c:     c,
		model: model,
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
	c          *config
	sync.Mutex // concurrent access to the file
	model      *shareModel
}

type config struct {
	File string `mapstructure:"file"`
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
	var ss []*collaboration.Share
	m.Lock()
	defer m.Unlock()
	user := ctxpkg.ContextMustGetUser(ctx)
	for _, s := range m.model.Shares {
		if share.IsCreatedByUser(s, user) && share.MatchesFilters(s, filters) {
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
	var rss []*collaboration.ReceivedShare
	for _, s := range m.model.Shares {
		if !share.IsCreatedByUser(s, user) &&
			share.IsGrantedToUser(s, user) &&
			share.MatchesFilters(s, filters) {
			rss = append(rss, m.convert(user, s))
		}
	}

	// if there is a mix-up of shares of type group and shares of type user we need to deduplicate them, since it points
	// to the same resource. Leave the more explicit and hide the less explicit. In this case we hide the group shares
	// and return the user share to the user.
	filtered := make([]*collaboration.ReceivedShare, 0)
	filtered = append(filtered, rss...)

	for i := range rss {
		for j := range rss {
			if rss[i].Share.ResourceId.GetOpaqueId() == rss[j].Share.ResourceId.GetOpaqueId() {
				if rss[i].Share.GetGrantee().GetType() == provider.GranteeType_GRANTEE_TYPE_GROUP && rss[j].Share.GetGrantee().GetType() == provider.GranteeType_GRANTEE_TYPE_USER {
					if rss[i].State == rss[j].State {
						// remove the group share from the results
						filtered[i] = filtered[len(filtered)-1]
						filtered[len(filtered)-1] = nil
						filtered = filtered[:len(filtered)-1]
					}
				}
			}
		}
	}

	return filtered, nil
}

// convert must be called in a lock-controlled block.
func (m *mgr) convert(currentUser *userv1beta1.User, s *collaboration.Share) *collaboration.ReceivedShare {
	rs := &collaboration.ReceivedShare{
		Share: s,
		State: collaboration.ShareState_SHARE_STATE_PENDING,
	}
	if v, ok := m.model.State[currentUser.Id.String()]; ok {
		if state, ok := v[s.Id.String()]; ok {
			rs.State = state
		}
	}
	if v, ok := m.model.MountPoint[currentUser.Id.String()]; ok {
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
	return m.convert(user, s), nil
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
