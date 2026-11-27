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
	"slices"
	"sync/atomic"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	revashare "github.com/cs3org/reva/v3/pkg/share"
	"github.com/cs3org/reva/v3/pkg/share/cache/memory"
	"github.com/cs3org/reva/v3/pkg/spaces"
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

// downStorage answers a stat only for the alive resources and does not know the
// gone ones. A stat of another resource waits for the end of the context, like
// a stat on an EOS instance that is down.
type downStorage struct {
	provider.ProviderAPIClient

	alive, gone []string
}

func (s *downStorage) Stat(ctx context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	id := req.Ref.GetResourceId()
	switch {
	case slices.Contains(s.alive, id.GetOpaqueId()):
		return &provider.StatResponse{Status: okStatus(), Info: &provider.ResourceInfo{Id: id}}, nil
	case slices.Contains(s.gone, id.GetOpaqueId()):
		return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

type receivedShares struct {
	collaboration.CollaborationAPIClient

	shares []*collaboration.ReceivedShare
}

func (r *receivedShares) ListReceivedShares(context.Context, *collaboration.ListReceivedSharesRequest, ...grpc.CallOption) (*collaboration.ListReceivedSharesResponse, error) {
	return &collaboration.ListReceivedSharesResponse{Status: okStatus(), Shares: r.shares}, nil
}

func TestListExistingReceivedSharesWithStorageDown(t *testing.T) {
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "me"}})
	share := func(id string) *collaboration.ReceivedShare {
		return &collaboration.ReceivedShare{
			State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
			Share: &collaboration.Share{
				Id:         &collaboration.ShareId{OpaqueId: id},
				ResourceId: &provider.ResourceId{StorageId: "storage", OpaqueId: id},
			},
		}
	}

	for name, tc := range map[string]struct {
		shares         []*collaboration.ReceivedShare
		want           []string
		wantIncomplete bool
	}{
		"a share on a storage that is down is left out": {
			shares:         []*collaboration.ReceivedShare{share("alive"), share("down")},
			want:           []string{"alive"},
			wantIncomplete: true,
		},
		"a share on a deleted resource does not make the list incomplete": {
			shares: []*collaboration.ReceivedShare{share("alive"), share("gone")},
			want:   []string{"alive"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			endpoint := t.Name() + "/storage"
			revaservice.SetClientAt(endpoint, &downStorage{alive: []string{"alive"}, gone: []string{"gone"}})
			t.Cleanup(func() { revaservice.SetClientAt(endpoint, nil) })
			stampGatewayPeers(&fakeStorageRegistryServer{storageEndpoint: endpoint}, &receivedShares{shares: tc.shares})

			riCache, err := memory.New[*provider.ResourceInfo](nil)
			require.NoError(t, err)
			gw := &svc{c: &config{}, resourceInfoCache: riCache, shareResolutionTimeout: 100 * time.Millisecond}

			type result struct {
				res *gateway.ListExistingReceivedSharesResponse
				err error
			}
			done := make(chan result, 1)
			go func() {
				res, err := gw.ListExistingReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
				done <- result{res, err}
			}()
			var r result
			select {
			case r = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("ListExistingReceivedShares does not return while a storage is down")
			}

			require.NoError(t, r.err)
			require.Equal(t, rpc.Code_CODE_OK, r.res.Status.Code)
			var ids []string
			for _, info := range r.res.ShareInfos {
				ids = append(ids, info.ReceivedShare.Share.Id.OpaqueId)
			}
			assert.ElementsMatch(t, tc.want, ids)
			_, incomplete := r.res.GetOpaque().GetMap()[spaces.OpaqueIncomplete]
			assert.Equal(t, tc.wantIncomplete, incomplete)
		})
	}
}
