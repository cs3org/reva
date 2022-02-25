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
	"fmt"
	"net/url"
	"path"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/cs3org/reva/pkg/storage/utils/indexer"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
	"github.com/cs3org/reva/pkg/storage/utils/metadata"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"
)

//go:generate mockery -name Storage
//go:generate mockery -name Indexer
type Storage interface {
	metadata.Storage
}

type Indexer interface {
	AddIndex(t interface{}, indexBy option.IndexBy, pkName, entityDirName, indexType string, bound *option.Bound, caseInsensitive bool) error
	Add(t interface{}) ([]indexer.IdxAddResult, error)
	FindBy(t interface{}, field string, val string) ([]string, error)
	Delete(t interface{}) error
}

// Manager implements a share manager using a cs3 storage backend
type Manager struct {
	storage Storage
	indexer Indexer

	initialized bool
}

func init() {
	registry.Register("cs3", NewDefault)
}

type config struct {
	GatewayAddr       string `mapstructure:"gateway_addr"`
	ProviderAddr      string `mapstructure:"provider_addr"`
	ServiceUserID     string `mapstructure:"service_user_id"`
	MachineAuthAPIKey string `mapstructure:"machine_auth_apikey"`
}

func indexOwnerFunc(v interface{}) (string, error) {
	share, ok := v.(*collaboration.Share)
	if !ok {
		return "", fmt.Errorf("given entity is not a share")
	}
	return url.QueryEscape(share.Owner.Idp + ":" + share.Owner.OpaqueId), nil
}

func indexGranteeFunc(v interface{}) (string, error) {
	share, ok := v.(*collaboration.Share)
	if !ok {
		return "", fmt.Errorf("given entity is not a share")
	}
	if share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER {
		return "user:" + share.Grantee.GetUserId().Idp + ":" + share.Grantee.GetUserId().OpaqueId, nil
	} else if share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		return "group:" + share.Grantee.GetGroupId().Idp + ":" + share.Grantee.GetGroupId().OpaqueId, nil
	} else {
		return "", fmt.Errorf("unknown grantee type")
	}
}

// NewDefault returns a new manager instance with default dependencies
func NewDefault(m map[string]interface{}) (share.Manager, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	s, err := metadata.NewCS3Storage(c.GatewayAddr, c.ProviderAddr, c.ServiceUserID, c.MachineAuthAPIKey)
	if err != nil {
		return nil, err
	}
	indexer := indexer.CreateIndexer(s)

	return New(s, indexer)
}

// New returns a new manager instance
func New(s Storage, indexer Indexer) (*Manager, error) {
	return &Manager{
		storage:     s,
		indexer:     indexer,
		initialized: false,
	}, nil
}

func (m *Manager) initialize() error {
	if m.initialized {
		return nil
	}

	err := m.storage.Init("cs3-share-manager-metadata", context.Background())
	if err != nil {
		return err
	}
	if err := m.storage.MakeDirIfNotExist(context.Background(), "shares"); err != nil {
		return err
	}
	err = m.indexer.AddIndex(&collaboration.Share{}, option.IndexByFunc{
		Name: "OwnerId",
		Func: indexOwnerFunc,
	}, "Id.OpaqueId", "shares", "non_unique", nil, true)
	if err != nil {
		return err
	}
	err = m.indexer.AddIndex(&collaboration.Share{}, option.IndexByFunc{
		Name: "GranteeId",
		Func: indexGranteeFunc,
	}, "Id.OpaqueId", "shares", "non_unique", nil, true)
	if err != nil {
		return err
	}
	m.initialized = true
	return nil
}

// Share creates a new share
func (m *Manager) Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}
	user := ctxpkg.ContextMustGetUser(ctx)
	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	//// do not allow share to myself or the owner if share is for a user
	//// TODO(labkode): should not this be caught already at the gw level?
	//if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER &&
	//	(utils.UserEqual(g.Grantee.GetUserId(), user.Id) || utils.UserEqual(g.Grantee.GetUserId(), md.Owner)) {
	//	return nil, errors.New("json: owner/creator and grantee are the same")
	//}
	//
	//// check if share already exists.
	//key := &collaboration.ShareKey{
	//	Owner:      md.Owner,
	//	ResourceId: md.Id,
	//	Grantee:    g.Grantee,
	//}
	//_, err := m.getByKey(ctx, key)
	//
	//// share already exists
	//if err == nil {
	//	return nil, errtypes.AlreadyExists(key.String())
	//}

	share := &collaboration.Share{
		Id: &collaboration.ShareId{
			OpaqueId: uuid.NewString(),
		},
		ResourceId:  md.Id,
		Permissions: g.Permissions,
		Grantee:     g.Grantee,
		Owner:       md.Owner,
		Creator:     user.Id,
		Ctime:       ts,
		Mtime:       ts,
	}

	data, err := json.Marshal(share)
	if err != nil {
		return nil, err
	}

	fn := path.Join("shares", share.Id.OpaqueId)
	err = m.storage.SimpleUpload(ctx, fn, data)
	if err != nil {
		return nil, err
	}

	_, err = m.indexer.Add(share)
	if err != nil {
		return nil, err
	}

	return share, nil
}

// GetShare gets the information for a share by the given ref.
func (m *Manager) GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}
	fn := path.Join("shares", ref.GetId().OpaqueId)
	data, err := m.storage.SimpleDownload(ctx, fn)
	if err != nil {
		return nil, err
	}

	return unmarshalShareData(data)
}

// Unshare deletes the share pointed by ref.
func (m *Manager) Unshare(ctx context.Context, ref *collaboration.ShareReference) error {
	if err := m.initialize(); err != nil {
		return err
	}
	share, err := m.GetShare(ctx, ref)
	if err != nil {
		return err
	}

	fn := path.Join("shares", ref.GetId().OpaqueId)
	err = m.storage.Delete(ctx, fn)
	if err != nil {
		return err
	}

	return m.indexer.Delete(share)
}

// ListShares returns the shares created by the user. If md is provided is not nil,
// it returns only shares attached to the given resource.
func (m *Manager) ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}

	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, errtypes.UserRequired("error getting user from context")
	}

	allShareIds, err := m.indexer.FindBy(&collaboration.Share{}, "OwnerId", url.QueryEscape(user.GetId().Idp+":"+user.GetId().OpaqueId))
	if err != nil {
		return nil, err
	}
	result := []*collaboration.Share{}
	for _, id := range allShareIds {
		data, err := m.storage.SimpleDownload(ctx, path.Join("shares", id))
		if err != nil {
			return nil, err
		}

		s, err := unmarshalShareData(data)
		if err != nil {
			return nil, err
		}
		if share.MatchesFilters(s, filters) {
			result = append(result, s)
		}
	}
	return result, nil
}

// UpdateShare updates the mode of the given share.
func (m *Manager) UpdateShare(ctx context.Context, ref *collaboration.ShareReference, p *collaboration.SharePermissions) (*collaboration.Share, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}
	return nil, nil
}

// ListReceivedShares returns the list of shares the user has access to.
func (m *Manager) ListReceivedShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.ReceivedShare, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}
	return nil, nil
}

// GetReceivedShare returns the information for a received share.
func (m *Manager) GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}
	return nil, nil
}

// UpdateReceivedShare updates the received share with share state.
func (m *Manager) UpdateReceivedShare(ctx context.Context, share *collaboration.ReceivedShare, fieldMask *field_mask.FieldMask) (*collaboration.ReceivedShare, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}
	return nil, nil
}

func unmarshalShareData(data []byte) (*collaboration.Share, error) {
	userShare := &collaboration.Share{
		Grantee: &provider.Grantee{Id: &provider.Grantee_UserId{}},
	}
	groupShare := &collaboration.Share{
		Grantee: &provider.Grantee{Id: &provider.Grantee_GroupId{}},
	}
	err := json.Unmarshal(data, userShare)
	if err == nil && userShare.Grantee.GetUserId() != nil {
		return userShare, nil
	}
	err = json.Unmarshal(data, groupShare) // try to unmarshal to a group share if the user share unmarshalling failed
	if err == nil && groupShare.Grantee.GetGroupId() != nil {
		return groupShare, nil
	}
	return nil, errtypes.InternalError("failed to unmarshal share data")
}
