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
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
)

var counter uint64

type manager struct {
	lock   *sync.Mutex
	shares []*usershareproviderv0alphapb.Share
}

func (m *manager) add(ctx context.Context, s *usershareproviderv0alphapb.Share) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.shares = append(m.shares, s)
}

func (m *manager) Share(ctx context.Context, u *authv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo, g *usershareproviderv0alphapb.ShareGrant) (*usershareproviderv0alphapb.Share, error) {
	id := atomic.AddUint64(&counter, 1)
	user := user.ContextMustGetUser(ctx)
	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
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
		if reflect.DeepEqual(*s.GetId(), *id) {
			return s, nil
		}
	}
	return nil, errtypes.NotFound(id.String())
}

func (m *manager) getByKey(ctx context.Context, key *usershareproviderv0alphapb.ShareKey) (*usershareproviderv0alphapb.Share, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, s := range m.shares {
		if reflect.DeepEqual(*key.Owner, *s.Owner) && reflect.DeepEqual(*key.ResourceId, *s.ResourceId) && reflect.DeepEqual(*key.Grantee, *s.Grantee) {
			return s, nil
		}
	}
	return nil, errtypes.NotFound(key.String())
}

func (m *manager) get(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference) (s *usershareproviderv0alphapb.Share, err error) {
	if ref.GetId() != nil {
		s, err = m.getByID(ctx, ref.GetId())
	} else if ref.GetKey() != nil {
		s, err = m.getByKey(ctx, ref.GetKey())
	} else {
		err = errtypes.NotFound(ref.String())
	}
	return
}

func (m *manager) GetShare(ctx context.Context, u *authv0alphapb.User, ref *usershareproviderv0alphapb.ShareReference) (*usershareproviderv0alphapb.Share, error) {
	share, err := m.get(ctx, ref)
	if err != nil {
		return nil, err
	}

	return share, nil
}

func (m *manager) Unshare(ctx context.Context, u *authv0alphapb.User, ref *usershareproviderv0alphapb.ShareReference) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	for i, s := range m.shares {
		if equal(ref, s) {
			m.shares[len(m.shares)-1], m.shares[i] = m.shares[i], m.shares[len(m.shares)-1]
			m.shares = m.shares[:len(m.shares)-1]
			return nil
		}
	}
	return errtypes.NotFound(ref.String())
}

func equal(ref *usershareproviderv0alphapb.ShareReference, s *usershareproviderv0alphapb.Share) bool {
	if reflect.DeepEqual(*ref.GetId(), *s.Id) {
		return true
	} else if reflect.DeepEqual(*ref.GetKey().Owner, *s.Owner) && reflect.DeepEqual(*ref.GetKey().ResourceId, *s.ResourceId) && reflect.DeepEqual(*ref.GetKey().Grantee, *s.Grantee) {
		return true
	} else {
		return false
	}
}

func (m *manager) UpdateShare(ctx context.Context, u *authv0alphapb.User, ref *usershareproviderv0alphapb.ShareReference, g *usershareproviderv0alphapb.ShareGrant) (*usershareproviderv0alphapb.Share, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for i, s := range m.shares {
		if equal(ref, s) {
			m.shares[i].Grantee = g.Grantee
			m.shares[i].Permissions = g.Permissions
			return m.shares[i], nil
		}
	}
	return nil, errtypes.NotFound(ref.String())
}

func (m *manager) ListShares(ctx context.Context, u *authv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo) ([]*usershareproviderv0alphapb.Share, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.shares, nil
}

func (m *manager) ListReceivedShares(ctx context.Context, u *authv0alphapb.User) ([]*usershareproviderv0alphapb.ShareGrant, error) {
	return nil, nil
}

func (m *manager) GetReceivedShare(ctx context.Context, u *authv0alphapb.User, ref *usershareproviderv0alphapb.ShareReference) (*usershareproviderv0alphapb.ShareGrant, error) {
	return nil, nil
}

func (m *manager) RejectReceivedShare(ctx context.Context, u *authv0alphapb.User, ref *usershareproviderv0alphapb.ShareReference) error {
	return nil
}
