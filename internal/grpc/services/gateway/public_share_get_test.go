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

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// fakeLinkProvider returns its share for any reference.
type fakeLinkProvider struct {
	link.LinkAPIClient

	share *link.PublicShare
}

func (f *fakeLinkProvider) GetPublicShare(_ context.Context, _ *link.GetPublicShareRequest, _ ...grpc.CallOption) (*link.GetPublicShareResponse, error) {
	return &link.GetPublicShareResponse{Status: okStatus(), Share: f.share}, nil
}

func TestGetPublicShare(t *testing.T) {
	share := &link.PublicShare{
		Id:         &link.PublicShareId{OpaqueId: "1"},
		Token:      "IHqwVca5jdPB3Ry",
		ResourceId: &provider.ResourceId{StorageId: "storage", OpaqueId: "file"},
	}
	byToken := &link.GetPublicShareRequest{Ref: &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: share.Token}}}
	byID := &link.GetPublicShareRequest{Ref: &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: share.Id}}}

	// A viewer link gives no grant permissions on the resource.
	storage := &permStorage{perms: map[string]*provider.ResourcePermissions{
		"file": {Stat: true, InitiateFileDownload: true},
	}}
	endpoint := t.Name() + "/storage"
	revaservice.SetClientAt(endpoint, storage)
	t.Cleanup(func() { revaservice.SetClientAt(endpoint, nil) })
	stampGatewayPeers(&fakeStorageRegistryServer{storageEndpoint: endpoint}, newFakeCollaborationServer())
	stampPublicShareProvider(&fakeLinkProvider{share: share})
	t.Cleanup(func() { stampPublicShareProvider(nil) })

	gw := &svc{c: &config{}}
	user := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "owner"}})

	linkScope := func(token string) context.Context {
		scopes, err := scope.AddPublicShareScope(&link.PublicShare{
			Id: &link.PublicShareId{OpaqueId: "1"}, Token: token, ResourceId: share.ResourceId,
		}, authpb.Role_ROLE_VIEWER, nil)
		require.NoError(t, err)
		return appctx.ContextSetScopes(user, scopes)
	}

	t.Run("a public link session gets its own link by token", func(t *testing.T) {
		res, err := gw.GetPublicShare(linkScope(share.Token), byToken)
		require.NoError(t, err)
		require.Equal(t, rpc.Code_CODE_OK, res.Status.Code)
		assert.Equal(t, share.Token, res.Share.Token)
	})

	t.Run("a public link session of another link is denied", func(t *testing.T) {
		res, err := gw.GetPublicShare(linkScope("other"), byToken)
		require.NoError(t, err)
		assert.Equal(t, rpc.Code_CODE_PERMISSION_DENIED, res.Status.Code)
	})

	t.Run("a user without grant permissions is denied", func(t *testing.T) {
		for name, req := range map[string]*link.GetPublicShareRequest{"by token": byToken, "by id": byID} {
			res, err := gw.GetPublicShare(user, req)
			require.NoError(t, err, name)
			assert.Equal(t, rpc.Code_CODE_PERMISSION_DENIED, res.Status.Code, name)
		}
	})
}
