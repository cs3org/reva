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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	revashare "github.com/cs3org/reva/v3/pkg/share"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// permStorage answers a stat with the permissions of the caller on each resource.
type permStorage struct {
	provider.ProviderAPIClient

	perms map[string]*provider.ResourcePermissions
	stats atomic.Int32
}

func (s *permStorage) Stat(_ context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	s.stats.Add(1)
	id := req.Ref.GetResourceId()
	perms, ok := s.perms[id.GetOpaqueId()]
	if !ok {
		return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	return &provider.StatResponse{Status: okStatus(), Info: &provider.ResourceInfo{Id: id, PermissionSet: perms}}, nil
}

func TestListSharesOnlyReturnsManageableShares(t *testing.T) {
	me := &userpb.User{Id: &userpb.UserId{Idp: "cern", OpaqueId: "me"}}
	other := &userpb.UserId{OpaqueId: "other"}
	ctx := appctx.ContextSetUser(context.Background(), me)

	storage := &permStorage{perms: map[string]*provider.ResourcePermissions{
		"managed": {Stat: true, RemoveGrant: true},
		"read":    {Stat: true, ListGrants: true},
	}}
	endpoint := t.Name() + "/storage"
	revaservice.SetClientAt(endpoint, storage)
	t.Cleanup(func() { revaservice.SetClientAt(endpoint, nil) })

	shares := newFakeCollaborationServer()
	add := func(id, resource string, owner *userpb.UserId) {
		shares.shares[id] = &collaboration.Share{
			Id:         &collaboration.ShareId{OpaqueId: id},
			ResourceId: &provider.ResourceId{StorageId: "storage", OpaqueId: resource},
			// Like the SQL manager, the share has no idp in the owner.
			Owner:   &userpb.UserId{OpaqueId: owner.OpaqueId},
			Creator: me.Id,
		}
	}
	add("project-1", "managed", other)
	add("project-2", "managed", other)
	add("readable", "read", other)
	add("own", "read", me.Id)
	add("gone", "deleted", other)
	stampGatewayPeers(&fakeStorageRegistryServer{storageEndpoint: endpoint}, shares)

	gw := &svc{c: &config{}}
	creator := func(u *userpb.UserId) *collaboration.Filter {
		return &collaboration.Filter{Type: collaboration.Filter_TYPE_CREATOR, Term: &collaboration.Filter_Creator{Creator: u}}
	}

	t.Run("a listing without a resource or own creator filter is rejected", func(t *testing.T) {
		for name, filters := range map[string][]*collaboration.Filter{
			"no filter":     nil,
			"other creator": {creator(other)},
			"me and other":  {creator(me.Id), creator(other)},
			"space only":    {revashare.SpaceIDFilter("space")},
		} {
			res, err := gw.ListShares(ctx, &collaboration.ListSharesRequest{Filters: filters})
			require.NoError(t, err, name)
			assert.Equal(t, rpc.Code_CODE_INVALID_ARGUMENT, res.Status.Code, name)
		}
	})

	t.Run("only owned shares and shares on manageable resources are returned", func(t *testing.T) {
		storage.stats.Store(0)
		res, err := gw.ListShares(ctx, &collaboration.ListSharesRequest{Filters: []*collaboration.Filter{
			revashare.ResourceIDFilter(&provider.ResourceId{StorageId: "storage", OpaqueId: "managed"}),
		}})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, res.Status.Code)

		var ids []string
		for _, s := range res.Shares {
			ids = append(ids, s.Id.OpaqueId)
		}
		assert.ElementsMatch(t, []string{"project-1", "project-2", "own"}, ids)
		// One stat for each resource that the user does not own: managed, read and deleted.
		assert.Equal(t, int32(3), storage.stats.Load())
	})

	t.Run("an own creator filter is accepted", func(t *testing.T) {
		res, err := gw.ListShares(ctx, &collaboration.ListSharesRequest{Filters: []*collaboration.Filter{
			creator(me.Id),
		}})
		require.NoError(t, err)
		assert.Equal(t, rpc.Code_CODE_OK, res.Status.Code)
	})
}
