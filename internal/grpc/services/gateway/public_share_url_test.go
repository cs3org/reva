// Copyright 2018-2026 CERN
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
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/publicshare"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/share/cache/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// urlLinkProvider answers each public link call with its share. Create and update
// also return an opaque entry of the provider, which the gateway must keep.
type urlLinkProvider struct {
	fakeLinkProvider
}

func providerOpaque() *types.Opaque {
	return &types.Opaque{Map: map[string]*types.OpaqueEntry{
		"provider": {Decoder: "plain", Value: []byte("kept")},
	}}
}

func (p *urlLinkProvider) CreatePublicShare(_ context.Context, _ *link.CreatePublicShareRequest, _ ...grpc.CallOption) (*link.CreatePublicShareResponse, error) {
	return &link.CreatePublicShareResponse{Status: okStatus(), Share: p.share, Opaque: providerOpaque()}, nil
}

func (p *urlLinkProvider) UpdatePublicShare(_ context.Context, _ *link.UpdatePublicShareRequest, _ ...grpc.CallOption) (*link.UpdatePublicShareResponse, error) {
	return &link.UpdatePublicShareResponse{Status: okStatus(), Share: p.share, Opaque: providerOpaque()}, nil
}

func (p *urlLinkProvider) ListPublicShares(_ context.Context, _ *link.ListPublicSharesRequest, _ ...grpc.CallOption) (*link.ListPublicSharesResponse, error) {
	return &link.ListPublicSharesResponse{Status: okStatus(), Share: []*link.PublicShare{p.share}}, nil
}

func TestPublicLinkURL(t *testing.T) {
	owner := &userpb.UserId{OpaqueId: "owner"}
	share := &link.PublicShare{
		Id:         &link.PublicShareId{OpaqueId: "1"},
		Token:      "IHqwVca5jdPB3Ry",
		ResourceId: &provider.ResourceId{StorageId: "storage", OpaqueId: "file"},
		Owner:      owner,
	}
	ref := &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: share.Id}}

	storage := &permStorage{perms: map[string]*provider.ResourcePermissions{
		"file": {Stat: true, AddGrant: true, UpdateGrant: true},
	}}
	endpoint := t.Name() + "/storage"
	revaservice.SetClientAt(endpoint, storage)
	t.Cleanup(func() { revaservice.SetClientAt(endpoint, nil) })
	stampGatewayPeers(&fakeStorageRegistryServer{storageEndpoint: endpoint}, newFakeCollaborationServer())
	stampPublicShareProvider(&urlLinkProvider{fakeLinkProvider{share: share}})
	t.Cleanup(func() { stampPublicShareProvider(nil) })

	ricache, err := memory.New[*provider.ResourceInfo](nil)
	require.NoError(t, err)
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: owner})

	// opaques makes each public link call and returns the opaque that holds the url.
	opaques := func(t *testing.T, gw *svc) map[string]*types.Opaque {
		create, err := gw.CreatePublicShare(ctx, &link.CreatePublicShareRequest{
			ResourceInfo: &provider.ResourceInfo{Id: share.ResourceId, Path: "/home/file"},
		})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, create.Status.Code)

		get, err := gw.GetPublicShare(ctx, &link.GetPublicShareRequest{Ref: ref})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, get.Status.Code)

		update, err := gw.UpdatePublicShare(ctx, &link.UpdatePublicShareRequest{Ref: ref})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, update.Status.Code)

		list, err := gw.ListExistingPublicShares(ctx, &link.ListPublicSharesRequest{
			Filters: []*link.ListPublicSharesRequest_Filter{publicshare.ResourceIDFilter(share.ResourceId)},
		})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, list.Status.Code)
		require.Len(t, list.ShareInfos, 1)

		return map[string]*types.Opaque{
			"create": create.Opaque,
			"get":    get.Opaque,
			"update": update.Opaque,
			"list":   list.ShareInfos[0].Opaque,
		}
	}

	t.Run("each response has the url of the link", func(t *testing.T) {
		gw := &svc{c: &config{PublicLinkBaseURL: "https://cernbox.cern.ch/"}, resourceInfoCache: ricache}
		got := opaques(t, gw)
		for name, o := range got {
			entry := o.GetMap()["url"]
			require.NotNil(t, entry, name)
			assert.Equal(t, "plain", entry.Decoder, name)
			assert.Equal(t, "https://cernbox.cern.ch/s/IHqwVca5jdPB3Ry", string(entry.Value), name)
		}
		// The url is added next to the entries of the provider.
		assert.Equal(t, "kept", string(got["create"].GetMap()["provider"].GetValue()))
		assert.Equal(t, "kept", string(got["update"].GetMap()["provider"].GetValue()))
	})

	t.Run("without a base url no url is added", func(t *testing.T) {
		gw := &svc{c: &config{}, resourceInfoCache: ricache}
		got := opaques(t, gw)
		for name, o := range got {
			assert.NotContains(t, o.GetMap(), "url", name)
		}
		assert.Nil(t, got["get"])
		assert.Nil(t, got["list"])
		assert.Len(t, got["create"].GetMap(), 1)
	})
}
