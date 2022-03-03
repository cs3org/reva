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

	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
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
	"github.com/cs3org/reva/pkg/utils"
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

type ReceivedShareMetadata struct {
	State      collaboration.ShareState `json:"state"`
	MountPoint *provider.Reference      `json:"mountpoint"`
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

	err := m.storage.Init(context.Background(), "cs3-share-manager-metadata")
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

	switch {
	case ref.GetId() != nil:
		return m.getShareById(ctx, ref.GetId().OpaqueId)
	case ref.GetKey() != nil:
		return m.getShareByKey(ctx, ref.GetKey())
	default:
		return nil, errtypes.BadRequest("neither share id nor key was given")
	}

}

func (m *Manager) getShareById(ctx context.Context, id string) (*collaboration.Share, error) {
	data, err := m.storage.SimpleDownload(ctx, shareFilename(id))
	if err != nil {
		return nil, err
	}

	userShare := &collaboration.Share{
		Grantee: &provider.Grantee{Id: &provider.Grantee_UserId{}},
	}
	err = json.Unmarshal(data, userShare)
	if err == nil && userShare.Grantee.GetUserId() != nil {
		return userShare, nil
	}

	groupShare := &collaboration.Share{
		Grantee: &provider.Grantee{Id: &provider.Grantee_GroupId{}},
	}
	err = json.Unmarshal(data, groupShare) // try to unmarshal to a group share if the user share unmarshalling failed
	if err == nil && groupShare.Grantee.GetGroupId() != nil {
		return groupShare, nil
	}

	return nil, errtypes.InternalError("failed to unmarshal share data")
}

func (m *Manager) getShareByKey(ctx context.Context, key *collaboration.ShareKey) (*collaboration.Share, error) {
	ownerIds, err := m.indexer.FindBy(&collaboration.Share{}, "OwnerId", userIdToIndex(key.Owner))
	if err != nil {
		return nil, err
	}
	granteeIndex, err := granteeToIndex(key.Grantee)
	if err != nil {
		return nil, err
	}
	granteeIds, err := m.indexer.FindBy(&collaboration.Share{}, "GranteeId", granteeIndex)
	if err != nil {
		return nil, err
	}

	ids := intersectSlices(ownerIds, granteeIds)
	for _, id := range ids {
		share, err := m.getShareById(ctx, id)
		if err != nil {
			return nil, err
		}
		if utils.ResourceIDEqual(share.ResourceId, key.ResourceId) {
			return share, nil
		}
	}
	return nil, errtypes.NotFound("share not found")
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

	allShareIds, err := m.indexer.FindBy(&collaboration.Share{}, "OwnerId", userIdToIndex(user.GetId()))
	if err != nil {
		return nil, err
	}
	result := []*collaboration.Share{}
	for _, id := range allShareIds {
		s, err := m.getShareById(ctx, id)
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

	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, errtypes.UserRequired("error getting user from context")
	}

	result := []*collaboration.ReceivedShare{}

	ids, err := granteeToIndex(&provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id:   &provider.Grantee_UserId{UserId: user.Id},
	})
	if err != nil {
		return nil, err
	}
	receivedIds, err := m.indexer.FindBy(&collaboration.Share{}, "GranteeId", ids)
	if err != nil {
		return nil, err
	}
	for _, group := range user.Groups {
		index, err := granteeToIndex(&provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id:   &provider.Grantee_GroupId{GroupId: &groupv1beta1.GroupId{OpaqueId: group}},
		})
		if err != nil {
			return nil, err
		}
		groupIds, err := m.indexer.FindBy(&collaboration.Share{}, "GranteeId", index)
		if err != nil {
			return nil, err
		}
		receivedIds = append(receivedIds, groupIds...)
	}

	for _, id := range receivedIds {
		share, err := m.getShareById(ctx, id)
		if err != nil {
			return nil, err
		}
		metadata, err := m.downloadMetadata(ctx, share)
		if err != nil {
			return nil, err
		}
		result = append(result, &collaboration.ReceivedShare{
			Share:      share,
			State:      metadata.State,
			MountPoint: metadata.MountPoint,
		})
	}
	return result, nil
}

// GetReceivedShare returns the information for a received share.
func (m *Manager) GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}

	share, err := m.GetShare(ctx, ref)
	if err != nil {
		return nil, err
	}

	metadata, err := m.downloadMetadata(ctx, share)
	return &collaboration.ReceivedShare{
		Share:      share,
		State:      metadata.State,
		MountPoint: metadata.MountPoint,
	}, nil
}

// UpdateReceivedShare updates the received share with share state.
func (m *Manager) UpdateReceivedShare(ctx context.Context, share *collaboration.ReceivedShare, fieldMask *field_mask.FieldMask) (*collaboration.ReceivedShare, error) {
	if err := m.initialize(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (m *Manager) downloadMetadata(ctx context.Context, share *collaboration.Share) (ReceivedShareMetadata, error) {
	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return ReceivedShareMetadata{}, errtypes.UserRequired("error getting user from context")
	}

	metadataFn, err := metadataFilename(share, user)
	if err != nil {
		return ReceivedShareMetadata{}, err
	}
	data, err := m.storage.SimpleDownload(ctx, metadataFn)
	if err != nil {
		return ReceivedShareMetadata{}, err
	}
	metadata := ReceivedShareMetadata{}
	err = json.Unmarshal(data, &metadata)
	return metadata, err
}

func shareFilename(id string) string {
	return path.Join("shares", id)
}

func metadataFilename(s *collaboration.Share, g interface{}) (string, error) {
	var granteePart string
	switch v := g.(type) {
	case *userpb.User:
		granteePart = url.QueryEscape("user:" + v.Id.Idp + ":" + v.Id.OpaqueId)
	case *provider.Grantee:
		var err error
		granteePart, err = granteeToIndex(v)
		if err != nil {
			return "", err
		}
	}
	return path.Join("metadata", s.Id.OpaqueId, granteePart), nil
}

func indexOwnerFunc(v interface{}) (string, error) {
	share, ok := v.(*collaboration.Share)
	if !ok {
		return "", fmt.Errorf("given entity is not a share")
	}
	return userIdToIndex(share.Owner), nil
}

func userIdToIndex(id *userpb.UserId) string {
	return url.QueryEscape(id.Idp + ":" + id.OpaqueId)
}

func indexGranteeFunc(v interface{}) (string, error) {
	share, ok := v.(*collaboration.Share)
	if !ok {
		return "", fmt.Errorf("given entity is not a share")
	}
	return granteeToIndex(share.Grantee)
}

func granteeToIndex(grantee *provider.Grantee) (string, error) {
	switch {
	case grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER:
		return url.QueryEscape("user:" + grantee.GetUserId().Idp + ":" + grantee.GetUserId().OpaqueId), nil
	case grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP:
		return url.QueryEscape("group:" + grantee.GetGroupId().OpaqueId), nil
	default:
		return "", fmt.Errorf("unknown grantee type")
	}
}

func intersectSlices(a, b []string) []string {
	aMap := map[string]bool{}
	for _, s := range a {
		aMap[s] = true
	}
	result := []string{}
	for _, s := range b {
		if _, ok := aMap[s]; ok {
			result = append(result, s)
		}
	}
	return result
}
