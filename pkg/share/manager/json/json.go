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
	"io/ioutil"
	"os"
	"sync"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/share"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils"
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
	info, err := os.Stat(file)
	if os.IsNotExist(err) || info.Size() == 0 {
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

	m := &shareModel{State: j.State}
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

	m.file = file
	return m, nil
}

type shareModel struct {
	file   string
	State  map[string]map[string]collaboration.ShareState `json:"state"` // map[username]map[share_id]ShareState
	Shares []*collaboration.Share                         `json:"shares"`
}

type jsonEncoding struct {
	State  map[string]map[string]collaboration.ShareState `json:"state"` // map[username]map[share_id]ShareState
	Shares []string                                       `json:"shares"`
}

func (m *shareModel) Save() error {
	j := &jsonEncoding{State: m.State}
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

func genID() string {
	return uuid.New().String()
}

func (m *mgr) Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error) {
	id := genID()
	user := user.ContextMustGetUser(ctx)
	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
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
	_, err := m.getByKey(ctx, key)

	// share already exists
	if err == nil {
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

	m.Lock()
	defer m.Unlock()

	m.model.Shares = append(m.model.Shares, s)
	if err := m.model.Save(); err != nil {
		err = errors.Wrap(err, "error saving model")
		return nil, err
	}

	return s, nil
}

func (m *mgr) getByID(ctx context.Context, id *collaboration.ShareId) (*collaboration.Share, error) {
	m.Lock()
	defer m.Unlock()
	for _, s := range m.model.Shares {
		if s.GetId().OpaqueId == id.OpaqueId {
			return s, nil
		}
	}
	return nil, errtypes.NotFound(id.String())
}

func (m *mgr) getByKey(ctx context.Context, key *collaboration.ShareKey) (*collaboration.Share, error) {
	m.Lock()
	defer m.Unlock()
	for _, s := range m.model.Shares {
		if (utils.UserEqual(key.Owner, s.Owner) || utils.UserEqual(key.Owner, s.Creator)) &&
			utils.ResourceEqual(key.ResourceId, s.ResourceId) && utils.GranteeEqual(key.Grantee, s.Grantee) {
			return s, nil
		}
	}
	return nil, errtypes.NotFound(key.String())
}

func (m *mgr) get(ctx context.Context, ref *collaboration.ShareReference) (s *collaboration.Share, err error) {
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
	if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
		return s, nil
	}

	// or the grantee
	if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user.Id, s.Grantee.GetUserId()) {
		return s, nil
	} else if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		// check if all user groups match this share; TODO(labkode): filter shares created by us.
		for _, g := range user.Groups {
			if g == s.Grantee.GetGroupId().OpaqueId {
				return s, nil
			}
		}
	}
	// we return not found to not disclose information
	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error) {
	share, err := m.get(ctx, ref)
	if err != nil {
		return nil, err
	}

	return share, nil
}

func (m *mgr) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
	m.Lock()
	defer m.Unlock()
	user := user.ContextMustGetUser(ctx)
	for i, s := range m.model.Shares {
		if sharesEqual(ref, s) {
			if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
				m.model.Shares[len(m.model.Shares)-1], m.model.Shares[i] = m.model.Shares[i], m.model.Shares[len(m.model.Shares)-1]
				m.model.Shares = m.model.Shares[:len(m.model.Shares)-1]
				if err := m.model.Save(); err != nil {
					err = errors.Wrap(err, "error saving model")
					return err
				}
				return nil
			}
		}
	}
	return errtypes.NotFound(ref.String())
}

func sharesEqual(ref *collaboration.ShareReference, s *collaboration.Share) bool {
	if ref.GetId() != nil && s.Id != nil {
		if ref.GetId().OpaqueId == s.Id.OpaqueId {
			return true
		}
	} else if ref.GetKey() != nil {
		if (utils.UserEqual(ref.GetKey().Owner, s.Owner) || utils.UserEqual(ref.GetKey().Owner, s.Creator)) &&
			utils.ResourceEqual(ref.GetKey().ResourceId, s.ResourceId) && utils.GranteeEqual(ref.GetKey().Grantee, s.Grantee) {
			return true
		}
	}
	return false
}

func (m *mgr) UpdateShare(ctx context.Context, ref *collaboration.ShareReference, p *collaboration.SharePermissions) (*collaboration.Share, error) {
	m.Lock()
	defer m.Unlock()
	user := user.ContextMustGetUser(ctx)
	for i, s := range m.model.Shares {
		if sharesEqual(ref, s) {
			if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
				now := time.Now().UnixNano()
				m.model.Shares[i].Permissions = p
				m.model.Shares[i].Mtime = &typespb.Timestamp{
					Seconds: uint64(now / 1000000000),
					Nanos:   uint32(now % 1000000000),
				}
				if err := m.model.Save(); err != nil {
					err = errors.Wrap(err, "error saving model")
					return nil, err
				}
				return m.model.Shares[i], nil
			}
		}
	}
	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) ListShares(ctx context.Context, filters []*collaboration.ListSharesRequest_Filter) ([]*collaboration.Share, error) {
	var ss []*collaboration.Share
	m.Lock()
	defer m.Unlock()
	user := user.ContextMustGetUser(ctx)
	for _, s := range m.model.Shares {
		if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
			// no filter we return earlier
			if len(filters) == 0 {
				ss = append(ss, s)
			} else {
				// check filters
				// TODO(labkode): add the rest of filters.
				for _, f := range filters {
					if f.Type == collaboration.ListSharesRequest_Filter_TYPE_RESOURCE_ID {
						if utils.ResourceEqual(s.ResourceId, f.GetResourceId()) {
							ss = append(ss, s)
						}
					}
				}
			}
		}
	}
	return ss, nil
}

// we list the shares that are targeted to the user in context or to the user groups.
func (m *mgr) ListReceivedShares(ctx context.Context) ([]*collaboration.ReceivedShare, error) {
	var rss []*collaboration.ReceivedShare
	m.Lock()
	defer m.Unlock()
	user := user.ContextMustGetUser(ctx)
	for _, s := range m.model.Shares {
		if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
			// omit shares created by me
			continue
		}
		if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user.Id, s.Grantee.GetUserId()) {
			rs := m.convert(ctx, s)
			rss = append(rss, rs)
		} else if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
			// check if all user groups match this share; TODO(labkode): filter shares created by us.
			for _, g := range user.Groups {
				if g == s.Grantee.GetGroupId().OpaqueId {
					rs := m.convert(ctx, s)
					rss = append(rss, rs)
				}
			}
		}
	}
	return rss, nil
}

// convert must be called in a lock-controlled block.
func (m *mgr) convert(ctx context.Context, s *collaboration.Share) *collaboration.ReceivedShare {
	rs := &collaboration.ReceivedShare{
		Share: s,
		State: collaboration.ShareState_SHARE_STATE_PENDING,
	}
	user := user.ContextMustGetUser(ctx)
	if v, ok := m.model.State[user.Id.String()]; ok {
		if state, ok := v[s.Id.String()]; ok {
			rs.State = state
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
	user := user.ContextMustGetUser(ctx)
	for _, s := range m.model.Shares {
		if sharesEqual(ref, s) {
			if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user.Id, s.Grantee.GetUserId()) {
				rs := m.convert(ctx, s)
				return rs, nil
			} else if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
				for _, g := range user.Groups {
					if s.Grantee.GetGroupId().OpaqueId == g {
						rs := m.convert(ctx, s)
						return rs, nil
					}
				}
			}
		}
	}
	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) UpdateReceivedShare(ctx context.Context, ref *collaboration.ShareReference, f *collaboration.UpdateReceivedShareRequest_UpdateField) (*collaboration.ReceivedShare, error) {
	rs, err := m.getReceived(ctx, ref)
	if err != nil {
		return nil, err
	}

	user := user.ContextMustGetUser(ctx)
	m.Lock()
	defer m.Unlock()

	if v, ok := m.model.State[user.Id.String()]; ok {
		v[rs.Share.Id.String()] = f.GetState()
		m.model.State[user.Id.String()] = v
	} else {
		a := map[string]collaboration.ShareState{
			rs.Share.Id.String(): f.GetState(),
		}
		m.model.State[user.Id.String()] = a
	}

	if err := m.model.Save(); err != nil {
		err = errors.Wrap(err, "error saving model")
		return nil, err
	}

	rs.State = f.GetState()
	return rs, nil
}
