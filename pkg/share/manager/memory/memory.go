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

package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cs3org/reva/pkg/share"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils"
)

var counter uint64

func init() {
	registry.Register("memory", New)
}

// New returns a new manager.
func New(c map[string]interface{}) (share.Manager, error) {
	state := map[string]map[*collaboration.ShareId]collaboration.ShareState{}
	return &manager{
		shareState: state,
		lock:       &sync.Mutex{},
	}, nil
}

type manager struct {
	lock   *sync.Mutex
	shares []*collaboration.Share
	// shareState contains the share state for a user.
	// map["alice"]["share-id"]state.
	shareState map[string]map[*collaboration.ShareId]collaboration.ShareState
}

func (m *manager) add(ctx context.Context, s *collaboration.Share) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.shares = append(m.shares, s)
}

func (m *manager) Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error) {
	id := atomic.AddUint64(&counter, 1)
	user := user.ContextMustGetUser(ctx)
	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER &&
		(utils.UserEqual(g.Grantee.GetUserId(), user.Id) || utils.UserEqual(g.Grantee.GetUserId(), md.Owner)) {
		return nil, errors.New("memory: owner/creator and grantee are the same")
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
			OpaqueId: fmt.Sprintf("%d", id),
		},
		ResourceId:  md.Id,
		Permissions: g.Permissions,
		Grantee:     g.Grantee,
		Owner:       md.Owner,
		Creator:     user.Id,
		Ctime:       ts,
		Mtime:       ts,
	}

	m.add(ctx, s)
	return s, nil
}

func (m *manager) getByID(ctx context.Context, id *collaboration.ShareId) (*collaboration.Share, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, s := range m.shares {
		if s.GetId().OpaqueId == id.OpaqueId {
			return s, nil
		}
	}
	return nil, errtypes.NotFound(id.String())
}

func (m *manager) getByKey(ctx context.Context, key *collaboration.ShareKey) (*collaboration.Share, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, s := range m.shares {
		if (utils.UserEqual(key.Owner, s.Owner) || utils.UserEqual(key.Owner, s.Creator)) &&
			utils.ResourceEqual(key.ResourceId, s.ResourceId) && utils.GranteeEqual(key.Grantee, s.Grantee) {
			return s, nil
		}
	}
	return nil, errtypes.NotFound(key.String())
}

func (m *manager) get(ctx context.Context, ref *collaboration.ShareReference) (s *collaboration.Share, err error) {
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

	// we return not found to not disclose information
	return nil, errtypes.NotFound(ref.String())
}

func (m *manager) GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error) {
	share, err := m.get(ctx, ref)
	if err != nil {
		return nil, err
	}

	return share, nil
}

func (m *manager) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	user := user.ContextMustGetUser(ctx)
	for i, s := range m.shares {
		if sharesEqual(ref, s) {
			if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
				m.shares[len(m.shares)-1], m.shares[i] = m.shares[i], m.shares[len(m.shares)-1]
				m.shares = m.shares[:len(m.shares)-1]
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

func (m *manager) UpdateShare(ctx context.Context, ref *collaboration.ShareReference, p *collaboration.SharePermissions) (*collaboration.Share, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	user := user.ContextMustGetUser(ctx)
	for i, s := range m.shares {
		if sharesEqual(ref, s) {
			if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
				now := time.Now().UnixNano()
				m.shares[i].Permissions = p
				m.shares[i].Mtime = &typespb.Timestamp{
					Seconds: uint64(now / 1000000000),
					Nanos:   uint32(now % 1000000000),
				}
				return m.shares[i], nil
			}
		}
	}
	return nil, errtypes.NotFound(ref.String())
}

func (m *manager) ListShares(ctx context.Context, filters []*collaboration.ListSharesRequest_Filter) ([]*collaboration.Share, error) {
	var ss []*collaboration.Share
	m.lock.Lock()
	defer m.lock.Unlock()
	user := user.ContextMustGetUser(ctx)
	for _, s := range m.shares {
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
func (m *manager) ListReceivedShares(ctx context.Context) ([]*collaboration.ReceivedShare, error) {
	var rss []*collaboration.ReceivedShare
	m.lock.Lock()
	defer m.lock.Unlock()
	user := user.ContextMustGetUser(ctx)
	for _, s := range m.shares {
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
func (m *manager) convert(ctx context.Context, s *collaboration.Share) *collaboration.ReceivedShare {
	rs := &collaboration.ReceivedShare{
		Share: s,
		State: collaboration.ShareState_SHARE_STATE_PENDING,
	}
	user := user.ContextMustGetUser(ctx)
	if v, ok := m.shareState[user.Id.String()]; ok {
		if state, ok := v[s.Id]; ok {
			rs.State = state
		}
	}
	return rs
}

func (m *manager) GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	return m.getReceived(ctx, ref)
}

func (m *manager) getReceived(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	user := user.ContextMustGetUser(ctx)
	for _, s := range m.shares {
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

func (m *manager) UpdateReceivedShare(ctx context.Context, ref *collaboration.ShareReference, f *collaboration.UpdateReceivedShareRequest_UpdateField) (*collaboration.ReceivedShare, error) {
	rs, err := m.getReceived(ctx, ref)
	if err != nil {
		return nil, err
	}

	user := user.ContextMustGetUser(ctx)
	m.lock.Lock()
	defer m.lock.Unlock()

	if v, ok := m.shareState[user.Id.String()]; ok {
		v[rs.Share.Id] = f.GetState()
		m.shareState[user.Id.String()] = v
	} else {
		a := map[*collaboration.ShareId]collaboration.ShareState{
			rs.Share.Id: f.GetState(),
		}
		m.shareState[user.Id.String()] = a
	}
	return rs, nil
}
