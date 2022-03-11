// Copyright 2018-2022 CERN
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

package cs3

import (
	"context"
	"encoding/json"
	"path"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/publicshare"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/registry"
	"github.com/cs3org/reva/v2/pkg/storage/utils/indexer"
	"github.com/cs3org/reva/v2/pkg/storage/utils/indexer/option"
	"github.com/cs3org/reva/v2/pkg/storage/utils/metadata"
	"github.com/cs3org/reva/v2/pkg/utils"
)

func init() {
	registry.Register("cs3", NewDefault)
}

type Manager struct {
	storage          metadata.Storage
	indexer          indexer.Indexer
	passwordHashCost int

	initialized bool
}

type PublicShareWithPassword struct {
	link.PublicShare

	HashedPassword string
}

// New returns a new filesystem public shares manager.
func NewDefault(c map[string]interface{}) (publicshare.Manager, error) {
	return nil, nil
}

func New(storage metadata.Storage, indexer indexer.Indexer, passwordHashCost int) (publicshare.Manager, error) {
	return &Manager{
		storage:          storage,
		indexer:          indexer,
		passwordHashCost: passwordHashCost,
		initialized:      false,
	}, nil
}

func (m *Manager) initialize() error {
	if m.initialized {
		return nil
	}

	err := m.storage.Init(context.Background(), "public-share-manager-metadata")
	if err != nil {
		return err
	}
	if err := m.storage.MakeDirIfNotExist(context.Background(), "publicshares"); err != nil {
		return err
	}
	err = m.indexer.AddIndex(&link.PublicShare{}, option.IndexByField("Id.OpaqueId"), "Token", "publicshares", "unique", nil, true)
	if err != nil {
		return err
	}
	m.initialized = true
	return nil
}

func (m *Manager) CreatePublicShare(ctx context.Context, u *user.User, ri *provider.ResourceInfo, g *link.Grant) (*link.PublicShare, error) {
	if !m.initialized {
		m.initialize()
	}

	id := &link.PublicShareId{
		OpaqueId: utils.RandString(15),
	}

	tkn := utils.RandString(15)
	now := time.Now().UnixNano()

	displayName := tkn
	if ri.ArbitraryMetadata != nil {
		metadataName, ok := ri.ArbitraryMetadata.Metadata["name"]
		if !ok {
			displayName = metadataName
		}
	}

	var passwordProtected bool
	password := g.Password
	if len(password) > 0 {
		h, err := bcrypt.GenerateFromPassword([]byte(password), m.passwordHashCost)
		if err != nil {
			return nil, errors.Wrap(err, "could not hash share password")
		}
		password = string(h)
		passwordProtected = true
	}

	createdAt := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	s := &PublicShareWithPassword{
		PublicShare: link.PublicShare{
			Id:                id,
			Owner:             ri.GetOwner(),
			Creator:           u.Id,
			ResourceId:        ri.Id,
			Token:             tkn,
			Permissions:       g.Permissions,
			Ctime:             createdAt,
			Mtime:             createdAt,
			PasswordProtected: passwordProtected,
			Expiration:        g.Expiration,
			DisplayName:       displayName,
		},
		HashedPassword: password,
	}

	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	fn := path.Join("publicshares", tkn)
	err = m.storage.SimpleUpload(ctx, fn, data)
	if err != nil {
		return nil, err
	}

	m.indexer.Add(s.PublicShare)

	return &s.PublicShare, nil
}

func (m *Manager) UpdatePublicShare(ctx context.Context, u *user.User, req *link.UpdatePublicShareRequest, g *link.Grant) (*link.PublicShare, error) {
	return nil, nil
}

func (m *Manager) GetPublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference, sign bool) (*link.PublicShare, error) {
	switch {
	case ref.GetToken() != "":
		return m.getByToken(ctx, ref.GetToken())
	case ref.GetId().GetOpaqueId() != "":
		return m.getById(ctx, ref.GetId().GetOpaqueId())
	default:
		return nil, errtypes.BadRequest("neither id nor token given")
	}
}

func (m *Manager) getById(ctx context.Context, id string) (*link.PublicShare, error) {
	tokens, err := m.indexer.FindBy(&link.PublicShare{}, "Id.OpaqueId", id)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, errtypes.NotFound("publicshare with the given id not found")
	}
	return m.getByToken(ctx, tokens[0])
}

func (m *Manager) getByToken(ctx context.Context, token string) (*link.PublicShare, error) {
	fn := path.Join("publicshares", token)
	data, err := m.storage.SimpleDownload(ctx, fn)
	if err != nil {
		return nil, err
	}

	ps := PublicShareWithPassword{}
	err = json.Unmarshal(data, &ps)
	if err != nil {
		return nil, err
	}
	return &ps.PublicShare, nil
}

func (m *Manager) ListPublicShares(ctx context.Context, u *user.User, filters []*link.ListPublicSharesRequest_Filter, md *provider.ResourceInfo, sign bool) ([]*link.PublicShare, error) {
	return nil, nil
}

func (m *Manager) RevokePublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference) error {
	return nil
}

func (m *Manager) GetPublicShareByToken(ctx context.Context, token string, auth *link.PublicShareAuthentication, sign bool) (*link.PublicShare, error) {
	return nil, nil
}
