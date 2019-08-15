// Copyright 2018-2019 CERN
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
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cs3org/reva/pkg/share"

	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/cs3org/reva/pkg/user"
)

var counter uint64

func init() {
	registry.Register("memory", New)
}

// New returns a new manager.
func New(c map[string]interface{}) (share.Manager, error) {
	state := map[string]map[*usershareproviderv0alphapb.ShareId]usershareproviderv0alphapb.ShareState{}
	return &manager{
		shareState: state,
		lock:       &sync.Mutex{},
	}, nil
}

type manager struct {
	lock   *sync.Mutex
	shares []*usershareproviderv0alphapb.Share
	// shareState contains the share state for a user.
	// map["alice"]["share-id"]state.
	shareState map[string]map[*usershareproviderv0alphapb.ShareId]usershareproviderv0alphapb.ShareState
}

func (m *manager) add(ctx context.Context, s *usershareproviderv0alphapb.Share) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.shares = append(m.shares, s)
}

func (m *manager) Share(ctx context.Context, md *storageproviderv0alphapb.ResourceInfo, g *usershareproviderv0alphapb.ShareGrant) (*usershareproviderv0alphapb.Share, error) {
	id := atomic.AddUint64(&counter, 1)
	user := user.ContextMustGetUser(ctx)
	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	// do not allow share to myself if share is for a user
	if g.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER &&
		g.Grantee.Id.Idp == user.Id.Idp && g.Grantee.Id.OpaqueId == user.Id.OpaqueId {
		return nil, errors.New("memory: user and grantee are the same")
	}

	// check if share already exists.
	key := &usershareproviderv0alphapb.ShareKey{
		Owner:      user.Id,
		ResourceId: md.Id,
		Grantee:    g.Grantee,
	}
	_, err := m.getByKey(ctx, key)
	// share already exists
	if err == nil {
		return nil, errtypes.AlreadyExists(key.String())
	}

	s := &usershareproviderv0alphapb.Share{
		Id: &usershareproviderv0alphapb.ShareId{
			OpaqueId: fmt.Sprintf("%d", id),
		},
		ResourceId:  md.Id,
		Permissions: g.Permissions,
		Grantee:     g.Grantee,
		Owner:       user.Id,
		Creator:     user.Id,
		Ctime:       ts,
		Mtime:       ts,
	}

	m.add(ctx, s)
	return s, nil
}

func (m *manager) getByID(ctx context.Context, id *usershareproviderv0alphapb.ShareId) (*usershareproviderv0alphapb.Share, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, s := range m.shares {
		if s.GetId().OpaqueId == id.OpaqueId {
			return s, nil
		}
	}
	return nil, errtypes.NotFound(id.String())
}

func (m *manager) getByKey(ctx context.Context, key *usershareproviderv0alphapb.ShareKey) (*usershareproviderv0alphapb.Share, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, s := range m.shares {
		if key.Owner.Idp == s.Owner.Idp && key.Owner.OpaqueId == s.Owner.OpaqueId &&
			key.ResourceId.StorageId == s.ResourceId.StorageId && key.ResourceId.OpaqueId == s.ResourceId.OpaqueId &&
			key.Grantee.Type == s.Grantee.Type && key.Grantee.Id.Idp == s.Grantee.Id.Idp && key.Grantee.Id.OpaqueId == s.Grantee.Id.OpaqueId {
			return s, nil
		}
	}
	return nil, errtypes.NotFound(key.String())
}

func (m *manager) get(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference) (s *usershareproviderv0alphapb.Share, err error) {
	switch {
	case ref.GetId() != nil:
		s, err = m.getByID(ctx, ref.GetId())
	case ref.GetKey() != nil:
		s, err = m.getByKey(ctx, ref.GetKey())
	default:
		err = errtypes.NotFound(ref.String())
	}

	if err != nil {
		return s, err
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

func (m *manager) GetShare(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference) (*usershareproviderv0alphapb.Share, error) {
	share, err := m.get(ctx, ref)
	if err != nil {
		return nil, err
	}

	return share, nil
}

func (m *manager) Unshare(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	user := user.ContextMustGetUser(ctx)
	for i, s := range m.shares {
		if equal(ref, s) {
			if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
				m.shares[len(m.shares)-1], m.shares[i] = m.shares[i], m.shares[len(m.shares)-1]
				m.shares = m.shares[:len(m.shares)-1]
				return nil
			}
		}
	}
	return errtypes.NotFound(ref.String())
}

// TODO(labkode): this is fragile, the check should be done on primitve types.
func equal(ref *usershareproviderv0alphapb.ShareReference, s *usershareproviderv0alphapb.Share) bool {
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

func (m *manager) UpdateShare(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference, p *usershareproviderv0alphapb.SharePermissions) (*usershareproviderv0alphapb.Share, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	user := user.ContextMustGetUser(ctx)
	for i, s := range m.shares {
		if equal(ref, s) {
			if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
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

func (m *manager) ListShares(ctx context.Context, filters []*usershareproviderv0alphapb.ListSharesRequest_Filter) ([]*usershareproviderv0alphapb.Share, error) {
	var ss []*usershareproviderv0alphapb.Share
	m.lock.Lock()
	defer m.lock.Unlock()
	user := user.ContextMustGetUser(ctx)
	for _, s := range m.shares {
		// TODO(labkode): add check for creator.
		if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
			// no filter we return earlier
			if len(filters) == 0 {
				ss = append(ss, s)
			} else {
				// check filters
				// TODO(labkode): add the rest of filters.
				for _, f := range filters {
					if f.Type == usershareproviderv0alphapb.ListSharesRequest_Filter_LIST_SHARES_REQUEST_FILTER_TYPE_RESOURCE_ID {
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
func (m *manager) ListReceivedShares(ctx context.Context) ([]*usershareproviderv0alphapb.ReceivedShare, error) {
	var rss []*usershareproviderv0alphapb.ReceivedShare
	m.lock.Lock()
	defer m.lock.Unlock()
	user := user.ContextMustGetUser(ctx)
	for _, s := range m.shares {
		if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
			// omit shares created by me
			// TODO(labkode): apply check for s.Creator also.
			continue
		}
		if s.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER {
			if user.Id.Idp == s.Grantee.Id.Idp && user.Id.OpaqueId == s.Grantee.Id.OpaqueId {
				rs := m.convert(ctx, s)
				rss = append(rss, rs)
			}
		} else if s.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP {
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
func (m *manager) convert(ctx context.Context, s *usershareproviderv0alphapb.Share) *usershareproviderv0alphapb.ReceivedShare {
	rs := &usershareproviderv0alphapb.ReceivedShare{
		Share: s,
	}
	user := user.ContextMustGetUser(ctx)
	if v, ok := m.shareState[user.Id.String()]; ok {
		if state, ok := v[s.Id]; ok {
			rs.State = state
		}
	}
	return rs
}

func (m *manager) GetReceivedShare(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference) (*usershareproviderv0alphapb.ReceivedShare, error) {
	return m.getReceived(ctx, ref)
}

func (m *manager) getReceived(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference) (*usershareproviderv0alphapb.ReceivedShare, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	user := user.ContextMustGetUser(ctx)
	for _, s := range m.shares {
		if equal(ref, s) {
			if s.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_USER &&
				s.Grantee.Id.Idp == user.Id.Idp && s.Grantee.Id.OpaqueId == user.Id.OpaqueId {
				rs := m.convert(ctx, s)
				return rs, nil
			} else if s.Grantee.Type == storageproviderv0alphapb.GranteeType_GRANTEE_TYPE_GROUP {
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

func (m *manager) UpdateReceivedShare(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference, f *usershareproviderv0alphapb.UpdateReceivedShareRequest_UpdateField) (*usershareproviderv0alphapb.ReceivedShare, error) {
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
		a := map[*usershareproviderv0alphapb.ShareId]usershareproviderv0alphapb.ShareState{
			rs.Share.Id: f.GetState(),
		}
		m.shareState[user.Id.String()] = a
	}
	return rs, nil
}
