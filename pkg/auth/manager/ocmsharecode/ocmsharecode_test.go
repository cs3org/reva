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

package ocmsharecode

import (
	"context"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocminvite "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"google.golang.org/grpc"
)

// mockGW is a minimal gateway mock that satisfies gateway.GatewayAPIClient.
// Only GetOCMShareByToken and GetAcceptedUser are implemented.
type mockGW struct {
	gateway.GatewayAPIClient

	share     *ocm.Share
	shareErr  rpc.Code
	shareMsg  string
	remoteErr rpc.Code
	remoteMsg string
}

func (m *mockGW) GetOCMShareByToken(_ context.Context, _ *ocm.GetOCMShareByTokenRequest, _ ...grpc.CallOption) (*ocm.GetOCMShareByTokenResponse, error) {
	return &ocm.GetOCMShareByTokenResponse{
		Status: &rpc.Status{Code: m.shareErr, Message: m.shareMsg},
		Share:  m.share,
	}, nil
}

func (m *mockGW) GetAcceptedUser(_ context.Context, _ *ocminvite.GetAcceptedUserRequest, _ ...grpc.CallOption) (*ocminvite.GetAcceptedUserResponse, error) {
	return &ocminvite.GetAcceptedUserResponse{
		Status: &rpc.Status{Code: m.remoteErr, Message: m.remoteMsg},
		RemoteUser: &userpb.User{
			Id: &userpb.UserId{OpaqueId: "accepted-user", Idp: "remote.example.com", Type: userpb.UserType_USER_TYPE_FEDERATED},
		},
	}, nil
}

func testShare(id, token string) *ocm.Share {
	return &ocm.Share{
		Id:         &ocm.ShareId{OpaqueId: id},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      token,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: &userpb.UserId{OpaqueId: "grantee", Idp: "remote.example.com", Type: userpb.UserType_USER_TYPE_FEDERATED},
			},
		},
		Creator: &userpb.UserId{OpaqueId: "creator", Idp: "local.example.com"},
		Owner:   &userpb.UserId{OpaqueId: "creator", Idp: "local.example.com"},
		AccessMethods: []*ocm.AccessMethod{
			{
				Term: &ocm.AccessMethod_WebdavOptions{
					WebdavOptions: &ocm.WebDAVAccessMethod{
						Permissions:  permissions.NewViewerRole().CS3ResourcePermissions(),
						Requirements: []string{"must-exchange-token"},
						AccessTypes:  []ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE},
					},
				},
			},
		},
	}
}

func TestAuthenticateValidCode(t *testing.T) {
	s := testShare("share-abc", "code123")
	mgr := &manager{
		c:  &config{},
		gw: &mockGW{share: s, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK},
	}

	user, scopes, err := mgr.Authenticate(context.Background(), "nextcloud1.docker", "code123")
	if err != nil {
		t.Fatal(err)
	}
	if user.Id.OpaqueId != "accepted-user" {
		t.Errorf("user: got %s, want accepted-user", user.Id.OpaqueId)
	}

	// Scope should be code-flow (no Token embedded)
	shares, _ := scope.GetOCMSharesFromScopes(scopes)
	if len(shares) != 1 {
		t.Fatalf("expected 1 share in scope, got %d", len(shares))
	}
	if shares[0].Token != "" {
		t.Errorf("code-flow scope should not carry token, got %q", shares[0].Token)
	}
	if shares[0].Id.GetOpaqueId() != "share-abc" {
		t.Errorf("scope shareId: got %s, want share-abc", shares[0].Id.GetOpaqueId())
	}
	if _, ok := scopes["user"]; ok {
		t.Error("code-flow token must not contain 'user' scope key")
	}
}

func TestAuthenticateClientIDDoesNotNeedShareIDMatch(t *testing.T) {
	s := testShare("share-abc", "code123")
	mgr := &manager{
		c:  &config{},
		gw: &mockGW{share: s, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK},
	}

	user, _, err := mgr.Authenticate(context.Background(), "nextcloud1.docker", "code123")
	if err != nil {
		t.Fatal(err)
	}
	if user.Id.OpaqueId != "accepted-user" {
		t.Errorf("user: got %s, want accepted-user", user.Id.OpaqueId)
	}
}

func TestAuthenticateEmptyShareIDAccepts(t *testing.T) {
	s := testShare("share-abc", "code123")
	mgr := &manager{
		c:  &config{},
		gw: &mockGW{share: s, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK},
	}

	user, _, err := mgr.Authenticate(context.Background(), "", "code123")
	if err != nil {
		t.Fatal(err)
	}
	if user.Id.OpaqueId != "accepted-user" {
		t.Errorf("user: got %s, want accepted-user", user.Id.OpaqueId)
	}
}

func TestAuthenticateShareNotFound(t *testing.T) {
	mgr := &manager{
		c:  &config{},
		gw: &mockGW{shareErr: rpc.Code_CODE_NOT_FOUND, shareMsg: "not found"},
	}

	_, _, err := mgr.Authenticate(context.Background(), "share-abc", "bad-code")
	if err == nil {
		t.Fatal("expected error for not found share")
	}
	if _, ok := err.(errtypes.NotFound); !ok {
		t.Errorf("expected NotFound, got %T: %v", err, err)
	}
}

func TestAuthenticatePermissionDenied(t *testing.T) {
	mgr := &manager{
		c:  &config{},
		gw: &mockGW{shareErr: rpc.Code_CODE_PERMISSION_DENIED, shareMsg: "denied"},
	}

	_, _, err := mgr.Authenticate(context.Background(), "share-abc", "bad-code")
	if err == nil {
		t.Fatal("expected error for permission denied")
	}
	if _, ok := err.(errtypes.InvalidCredentials); !ok {
		t.Errorf("expected InvalidCredentials, got %T: %v", err, err)
	}
}

func TestGetRoleTreatsWebappUploadAsEditor(t *testing.T) {
	share := &ocm.Share{
		AccessMethods: []*ocm.AccessMethod{
			{
				Term: &ocm.AccessMethod_WebappOptions{
					WebappOptions: &ocm.WebappAccessMethod{
						Permissions: permissions.NewEditorRole().CS3ResourcePermissions(),
					},
				},
			},
		},
	}

	role, roleStr := getRole(share)
	if role != authpb.Role_ROLE_EDITOR {
		t.Fatalf("getRole() role = %v, want %v", role, authpb.Role_ROLE_EDITOR)
	}
	if roleStr != "editor" {
		t.Fatalf("getRole() roleStr = %q, want %q", roleStr, "editor")
	}
}

func TestGetRoleTreatsWebappReadAsViewer(t *testing.T) {
	share := &ocm.Share{
		AccessMethods: []*ocm.AccessMethod{
			{
				Term: &ocm.AccessMethod_WebappOptions{
					WebappOptions: &ocm.WebappAccessMethod{
						Permissions: permissions.NewViewerRole().CS3ResourcePermissions(),
					},
				},
			},
		},
	}

	role, roleStr := getRole(share)
	if role != authpb.Role_ROLE_VIEWER {
		t.Fatalf("getRole() role = %v, want %v", role, authpb.Role_ROLE_VIEWER)
	}
	if roleStr != "viewer" {
		t.Fatalf("getRole() roleStr = %q, want %q", roleStr, "viewer")
	}
}
