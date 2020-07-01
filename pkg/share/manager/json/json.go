// Copyright 2018-2020 CERN
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
	"reflect"
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

	mgr := &mgr{
		c:     c,
		model: model,
	}

	return mgr, nil
}

func loadOrCreate(file string) (*shareModel, error) {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
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

	m := &shareModel{}
	if err := json.Unmarshal(data, m); err != nil {
		err = errors.Wrap(err, "error decoding data to json")
		return nil, err
	}

	if m.State == nil {
		m.State = map[string]map[string]collaboration.ShareState{}
	}

	m.file = file
	return m, nil
}

type shareModel struct {
	file   string
	State  map[string]map[string]collaboration.ShareState `json:"state"` // map[username]map[share_id]boolean
	Shares []*collaboration.Share                         `json:"shares"`
}

func (m *shareModel) Save() error {
	data, err := json.Marshal(m)
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

	// do not allow share to myself if share is for a user
	// TODO(labkode): should not this be caught already at the gw level?
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER &&
		g.Grantee.Id.Idp == user.Id.Idp && g.Grantee.Id.OpaqueId == user.Id.OpaqueId {
		return nil, errors.New("json: user and grantee are the same")
	}

	// check if share already exists.
	key := &collaboration.ShareKey{
		Owner:      user.Id,
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
		Owner:       user.Id,
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
		if key.Owner.Idp == s.Owner.Idp && key.Owner.OpaqueId == s.Owner.OpaqueId &&
			key.ResourceId.StorageId == s.ResourceId.StorageId && key.ResourceId.OpaqueId == s.ResourceId.OpaqueId &&
			key.Grantee.Type == s.Grantee.Type && key.Grantee.Id.Idp == s.Grantee.Id.Idp && key.Grantee.Id.OpaqueId == s.Grantee.Id.OpaqueId {
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
	// TODO(labkode): check for creator also.
	user := user.ContextMustGetUser(ctx)
	if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
		return s, nil
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
		if equal(ref, s) {
			if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
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

// TODO(labkode): this is fragile, the check should be done on primitive types.
func equal(ref *collaboration.ShareReference, s *collaboration.Share) bool {
	if ref.GetId() != nil && s.Id != nil {
		if ref.GetId().OpaqueId == s.Id.OpaqueId {
			return true
		}
	} else if ref.GetKey() != nil {
		if reflect.DeepEqual(*ref.GetKey().Owner, *s.Owner) && reflect.DeepEqual(*ref.GetKey().ResourceId, *s.ResourceId) && reflect.DeepEqual(*ref.GetKey().Grantee, *s.Grantee) {
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
		if equal(ref, s) {
			if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
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
		// TODO(labkode): add check for creator.
		if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
			// no filter we return earlier
			if len(filters) == 0 {
				ss = append(ss, s)
			} else {
				// check filters
				// TODO(labkode): add the rest of filters.
				for _, f := range filters {
					if f.Type == collaboration.ListSharesRequest_Filter_TYPE_RESOURCE_ID {
						if s.ResourceId.StorageId == f.GetResourceId().StorageId && s.ResourceId.OpaqueId == f.GetResourceId().OpaqueId {
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
		if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
			// omit shares created by me
			// TODO(labkode): apply check for s.Creator also.
			continue
		}
		if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER {
			if user.Id.Idp == s.Grantee.Id.Idp && user.Id.OpaqueId == s.Grantee.Id.OpaqueId {
				rs := m.convert(ctx, s)
				rss = append(rss, rs)
			}
		} else if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
			// check if all user groups match this share; TODO(labkode): filter shares created by us.
			for _, g := range user.Groups {
				if g == s.Grantee.Id.OpaqueId {
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
		if equal(ref, s) {
			if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER &&
				s.Grantee.Id.Idp == user.Id.Idp && s.Grantee.Id.OpaqueId == user.Id.OpaqueId {
				rs := m.convert(ctx, s)
				return rs, nil
			} else if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
				for _, g := range user.Groups {
					if s.Grantee.Id.OpaqueId == g {
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

	return rs, nil
}
