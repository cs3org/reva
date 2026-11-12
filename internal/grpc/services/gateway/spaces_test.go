// Copyright 2018-2024 CERN
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

package gateway

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/share/cache/memory"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// listStorage lists the same entries for any folder.
type listStorage struct {
	provider.ProviderAPIClient

	infos []*provider.ResourceInfo
}

func (s *listStorage) ListContainer(context.Context, *provider.ListContainerRequest, ...grpc.CallOption) (*provider.ListContainerResponse, error) {
	return &provider.ListContainerResponse{Status: okStatus(), Infos: s.infos}, nil
}

// visibleSpaces finds only the spaces it holds, like the spaces registry
// that hides the projects of which the user is not a member.
type visibleSpaces struct {
	provider.SpacesAPIClient

	spaces map[string]*provider.StorageSpace
	lists  atomic.Int32
}

func (s *visibleSpaces) ListStorageSpaces(_ context.Context, req *provider.ListStorageSpacesRequest, _ ...grpc.CallOption) (*provider.ListStorageSpacesResponse, error) {
	s.lists.Add(1)
	res := &provider.ListStorageSpacesResponse{Status: okStatus()}
	if space, ok := s.spaces[req.Filters[0].GetId().GetOpaqueId()]; ok {
		res.StorageSpaces = append(res.StorageSpaces, space)
	}
	return res, nil
}

func TestListContainerLooksUpEachSpaceOnce(t *testing.T) {
	me := &userpb.User{Id: &userpb.UserId{Idp: "cern", OpaqueId: "me"}}
	ctx := appctx.ContextSetUser(context.Background(), me)

	entry := func(spaceID string) *provider.ResourceInfo {
		return &provider.ResourceInfo{
			Space: &provider.StorageSpace{
				Id:        &provider.StorageSpaceId{OpaqueId: spaceID},
				SpaceType: spaces.SpaceTypeProject.AsString(),
			},
			PermissionSet: &provider.ResourcePermissions{Stat: true},
		}
	}
	storage := &listStorage{infos: []*provider.ResourceInfo{
		entry("member"), entry("member"),
		entry("not-member"), entry("not-member"), entry("not-member"),
	}}
	endpoint := t.Name() + "/storage"
	revaservice.SetClientAt(endpoint, storage)
	t.Cleanup(func() { revaservice.SetClientAt(endpoint, nil) })
	stampGatewayPeers(&fakeStorageRegistryServer{storageEndpoint: endpoint}, nil)

	registry := &visibleSpaces{spaces: map[string]*provider.StorageSpace{
		"member": {Roles: []*provider.SpaceRole{{
			PermissionSet: &provider.ResourcePermissions{AddGrant: true},
			Recipients:    []*provider.Grantee{{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: me.Id}}},
		}}},
	}}
	stampSpaces(registry)
	t.Cleanup(func() { stampSpaces(nil) })

	spaceCache, err := memory.New[*provider.StorageSpace](nil)
	require.NoError(t, err)
	gw := &svc{c: &config{}, spaceInfoCache: spaceCache, spaceInfoCacheTTL: time.Hour}

	res, err := gw.ListContainer(ctx, &provider.ListContainerRequest{Ref: &provider.Reference{Path: "/project"}})
	require.NoError(t, err)
	require.Equal(t, rpc.Code_CODE_OK, res.Status.Code)

	assert.EqualValues(t, 2, registry.lists.Load(), "one lookup for each space, also for the space that is not found")
	for _, info := range res.Infos {
		assert.Equal(t, info.Space.Id.OpaqueId == "member", info.PermissionSet.AddGrant, info.Space.Id.OpaqueId)
	}
}
