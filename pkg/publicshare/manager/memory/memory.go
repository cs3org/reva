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
	"math/rand"
	"sync"
	"time"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/publicshare/manager/registry"
)

func init() {
	registry.Register("memory", New)
}

// New returns a new memory manager.
func New(c map[string]interface{}) (publicshare.Manager, error) {
	return &manager{
		shares: sync.Map{},
	}, nil
}

type manager struct {
	shares sync.Map
}

// CreatePublicShare safely adds a new entry to manager.shares
func (m *manager) CreatePublicShare(ctx context.Context, u *user.User, rInfo *provider.ResourceInfo, g *link.Grant) (*link.PublicShare, error) {
	// where could this initialization go wrong and early return?
	id := &link.PublicShareId{
		OpaqueId: randString(12),
	}
	tkn := randString(12)
	now := uint64(time.Now().Unix())

	newShare := link.PublicShare{
		Id:          id,
		Owner:       rInfo.GetOwner(),
		Creator:     u.Id,
		ResourceId:  rInfo.Id,
		Token:       tkn,
		Permissions: g.Permissions,
		Ctime: &typespb.Timestamp{
			Seconds: now,
			Nanos:   uint32(now % 1000000000),
		},
		Mtime: &typespb.Timestamp{
			Seconds: now,
			Nanos:   uint32(now % 1000000000),
		},
		PasswordProtected: false,
		Expiration:        g.Expiration,
		DisplayName:       tkn,
	}

	m.shares.Store(newShare.Token, &newShare)
	return &newShare, nil
}

// UpdatePublicShare updates the expiration date, permissions and Mtime
func (m *manager) UpdatePublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference, g *link.Grant) (*link.PublicShare, error) {
	share, err := m.GetPublicShare(ctx, u, ref)
	if err != nil {
		return nil, errors.New("ref does not exist")
	}

	token := share.GetToken()

	// thread unsafe. 2 goroutines can access to the same resource?
	share.Permissions = g.Permissions
	share.Expiration = g.Expiration
	share.Mtime = &typespb.Timestamp{
		Seconds: uint64(time.Now().Unix()),
		Nanos:   uint32(time.Now().Unix() % 1000000000),
	}

	m.shares.Store(token, share)

	return &link.PublicShare{}, nil
}

func (m *manager) GetPublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference) (share *link.PublicShare, err error) {
	// Attempt to fetch public share by token
	if ref.GetToken() != "" {
		share, err = m.GetPublicShareByToken(ctx, ref.GetToken())
		if err != nil {
			return nil, errors.New("there are no shares for the given reference")
		}
	}

	// Attempt to fetch public share by Id
	if ref.GetId() != nil {
		share, err = m.getPublicShareByTokenID(ctx, *ref.GetId())
		if err != nil {
			return nil, errors.New("there are no shares for the given reference")
		}
	}

	return share, nil
}

func (m *manager) ListPublicShares(ctx context.Context, u *user.User, md *provider.ResourceInfo) ([]*link.PublicShare, error) {
	shares := []*link.PublicShare{}
	m.shares.Range(func(k, v interface{}) bool {
		shares = append(shares, v.(*link.PublicShare))
		return true
	})

	return shares, nil
}

func (m *manager) RevokePublicShare(ctx context.Context, u *user.User, id string) (err error) {
	// check whether the referente exists
	if _, err := m.GetPublicShareByToken(ctx, id); err != nil {
		return errors.New("reference does not exist")
	}
	m.shares.Delete(id)
	return
}

func (m *manager) GetPublicShareByToken(ctx context.Context, token string) (*link.PublicShare, error) {
	if ps, ok := m.shares.Load(token); ok {
		return ps.(*link.PublicShare), nil
	}
	return nil, errors.New("invalid token")
}

// Helpers
func randString(n int) string {
	var l = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = l[rand.Intn(len(l))]
	}
	return string(b)
}

func (m *manager) getPublicShareByTokenID(ctx context.Context, targetID link.PublicShareId) (*link.PublicShare, error) {
	// iterate over existing shares and return the first one matching the id
	var found *link.PublicShare
	m.shares.Range(func(k, v interface{}) bool {
		id := v.(*link.PublicShare).GetId()
		if targetID.String() == id.String() {
			found = v.(*link.PublicShare)
			return true
		}
		return false
	})

	if found != nil {
		return found, nil
	}
	return nil, errors.New("invalid token")
}
