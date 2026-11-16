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
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/publicshare"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/share/cache/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// urlLinkProvider answers each public link call with a copy of its share, like a
// provider behind a gRPC connection does.
type urlLinkProvider struct {
	link.LinkAPIClient

	share *link.PublicShare
}

func (p *urlLinkProvider) copy() *link.PublicShare {
	return proto.Clone(p.share).(*link.PublicShare)
}

func (p *urlLinkProvider) CreatePublicShare(_ context.Context, _ *link.CreatePublicShareRequest, _ ...grpc.CallOption) (*link.CreatePublicShareResponse, error) {
	return &link.CreatePublicShareResponse{Status: okStatus(), Share: p.copy()}, nil
}

func (p *urlLinkProvider) GetPublicShare(_ context.Context, _ *link.GetPublicShareRequest, _ ...grpc.CallOption) (*link.GetPublicShareResponse, error) {
	return &link.GetPublicShareResponse{Status: okStatus(), Share: p.copy()}, nil
}

func (p *urlLinkProvider) GetPublicShareByToken(_ context.Context, _ *link.GetPublicShareByTokenRequest, _ ...grpc.CallOption) (*link.GetPublicShareByTokenResponse, error) {
	return &link.GetPublicShareByTokenResponse{Status: okStatus(), Share: p.copy()}, nil
}

func (p *urlLinkProvider) UpdatePublicShare(_ context.Context, _ *link.UpdatePublicShareRequest, _ ...grpc.CallOption) (*link.UpdatePublicShareResponse, error) {
	return &link.UpdatePublicShareResponse{Status: okStatus(), Share: p.copy()}, nil
}

func (p *urlLinkProvider) ListPublicShares(_ context.Context, _ *link.ListPublicSharesRequest, _ ...grpc.CallOption) (*link.ListPublicSharesResponse, error) {
	return &link.ListPublicSharesResponse{Status: okStatus(), Share: []*link.PublicShare{p.copy()}}, nil
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
	stampPublicShareProvider(&urlLinkProvider{share: share})
	t.Cleanup(func() { stampPublicShareProvider(nil) })

	ricache, err := memory.New[*provider.ResourceInfo](nil)
	require.NoError(t, err)
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: owner})

	// shares makes each public link call and returns the share of each response.
	shares := func(t *testing.T, gw *svc) map[string]*link.PublicShare {
		create, err := gw.CreatePublicShare(ctx, &link.CreatePublicShareRequest{
			ResourceInfo: &provider.ResourceInfo{Id: share.ResourceId, Path: "/home/file"},
		})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, create.Status.Code)

		get, err := gw.GetPublicShare(ctx, &link.GetPublicShareRequest{Ref: ref})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, get.Status.Code)

		byToken, err := gw.GetPublicShareByToken(ctx, &link.GetPublicShareByTokenRequest{Token: share.Token})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, byToken.Status.Code)

		update, err := gw.UpdatePublicShare(ctx, &link.UpdatePublicShareRequest{Ref: ref})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, update.Status.Code)

		filters := []*link.ListPublicSharesRequest_Filter{publicshare.ResourceIDFilter(share.ResourceId)}
		list, err := gw.ListPublicShares(ctx, &link.ListPublicSharesRequest{Filters: filters})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, list.Status.Code)
		require.Len(t, list.Share, 1)

		existing, err := gw.ListExistingPublicShares(ctx, &link.ListPublicSharesRequest{Filters: filters})
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, existing.Status.Code)
		require.Len(t, existing.ShareInfos, 1)

		return map[string]*link.PublicShare{
			"create":   create.Share,
			"get":      get.Share,
			"byToken":  byToken.Share,
			"update":   update.Share,
			"list":     list.Share[0],
			"existing": existing.ShareInfos[0].PublicShare,
		}
	}

	t.Run("each share has the url of the link", func(t *testing.T) {
		gw := &svc{c: &config{PublicLinkBaseURL: "https://cernbox.cern.ch/"}, resourceInfoCache: ricache}
		for name, sh := range shares(t, gw) {
			assert.Equal(t, "https://cernbox.cern.ch/s/IHqwVca5jdPB3Ry", sh.GetUrl(), name)
		}
	})

	t.Run("without a base url the url stays empty", func(t *testing.T) {
		gw := &svc{c: &config{}, resourceInfoCache: ricache}
		for name, sh := range shares(t, gw) {
			assert.Empty(t, sh.GetUrl(), name)
		}
	})
}
