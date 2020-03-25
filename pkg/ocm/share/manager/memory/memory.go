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

package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/manager/registry"
)

func init() {
	registry.Register("memory", New)
}

// New returns a new memory manager.
func New(c map[string]interface{}) (share.Manager, error) {
	return &mgr{
		shares: sync.Map{},
	}, nil
}

type mgr struct {
	shares sync.Map
}

func (m *mgr) Share(ctx context.Context, md *provider.ResourceId, g *ocm.ShareGrant) (*ocm.Share, error) {
	s := new(ocm.Share)
	m.shares.Store(s.Id, &s)

	fmt.Printf("%+v\n", m.shares)

	return s, nil
}

func (m *mgr) GetShare(ctx context.Context, ref *ocm.ShareReference) (*ocm.Share, error) {

	fmt.Printf("%+v\n", m.shares)

	if s, ok := m.shares.Load(ref.GetId()); ok {
		return s.(*ocm.Share), nil
	}

	return nil, errors.New("invalid share ID")
}

func (m *mgr) Unshare(ctx context.Context, ref *ocm.ShareReference) error {

	m.shares.Delete(ref.GetId())
	return nil
}

func (m *mgr) UpdateShare(ctx context.Context, ref *ocm.ShareReference, p *ocm.SharePermissions) (*ocm.Share, error) {
	s := new(ocm.Share)
	m.shares.Store(s.Id, &s)
	return s, nil
}

func (m *mgr) ListShares(ctx context.Context, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {

	shares := []*ocm.Share{}
	m.shares.Range(func(k, v interface{}) bool {
		shares = append(shares, v.(*ocm.Share))
		return true
	})
	return shares, nil
}

func (m *mgr) ListReceivedShares(ctx context.Context) ([]*ocm.ReceivedShare, error) {
	s := []*ocm.ReceivedShare{new(ocm.ReceivedShare)}
	return s, nil
}

func (m *mgr) GetReceivedShare(ctx context.Context, ref *ocm.ShareReference) (*ocm.ReceivedShare, error) {
	s := new(ocm.ReceivedShare)
	return s, nil
}

func (m *mgr) UpdateReceivedShare(ctx context.Context, ref *ocm.ShareReference, f *ocm.UpdateReceivedOCMShareRequest_UpdateField) (*ocm.ReceivedShare, error) {
	s := new(ocm.ReceivedShare)
	return s, nil
}
