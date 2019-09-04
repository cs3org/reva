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

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	publicshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	v0alpha "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	pbtypes "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/publicshare/manager/registry"
)

func init() {
	registry.Register("memory", New)
}

// New returns a new memory manager
func New(c map[string]interface{}) (publicshare.Manager, error) {
	return &manager{}, nil
}

type manager struct{}

// TODO(refs) implement application logic.
func (m *manager) CreatePublicShare(ctx context.Context, u *authv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo, g *publicshareproviderv0alphapb.Grant) (*publicshareproviderv0alphapb.PublicShare, error) {
	return &publicshareproviderv0alphapb.PublicShare{}, nil
}

func (m *manager) UpdatePublicShare(ctx context.Context, u *authv0alphapb.User, ref *publicshareproviderv0alphapb.PublicShareReference, g *publicshareproviderv0alphapb.Grant) (*publicshareproviderv0alphapb.PublicShare, error) {
	return &publicshareproviderv0alphapb.PublicShare{}, nil
}

func (m *manager) GetPublicShare(ctx context.Context, u *authv0alphapb.User, ref *publicshareproviderv0alphapb.PublicShareReference) (*publicshareproviderv0alphapb.PublicShare, error) {
	return &publicshareproviderv0alphapb.PublicShare{}, nil
}

func (m *manager) ListPublicShares(ctx context.Context, u *authv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo) ([]*publicshareproviderv0alphapb.PublicShare, error) {
	shares := []*publicshareproviderv0alphapb.PublicShare{
		&publicshareproviderv0alphapb.PublicShare{
			Id: &publicshareproviderv0alphapb.PublicShareId{
				OpaqueId: "some_publicly_shared_id",
			},
			Token:       "my_token",
			ResourceId:  &v0alpha.ResourceId{},
			Permissions: &publicshareproviderv0alphapb.PublicSharePermissions{},
			Owner:       &pbtypes.UserId{},
			Creator:     &pbtypes.UserId{},
			Ctime:       &pbtypes.Timestamp{},
			Expiration:  &pbtypes.Timestamp{},
			Mtime:       &pbtypes.Timestamp{},
			DisplayName: "some_public_share",
		},
	}
	return shares, nil
}

func (m *manager) RevokePublicShare(ctx context.Context, u *authv0alphapb.User, id string) error {
	return nil
}

func (m *manager) GetPublicShareByToken(ctx context.Context, token string) (*publicshareproviderv0alphapb.PublicShare, error) {
	return &publicshareproviderv0alphapb.PublicShare{}, nil
}
