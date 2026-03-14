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

package ocmexchangedtoken

import (
	"context"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	jwt "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
)

func testJWTManager() *manager {
	tokenmgr, err := jwt.New(map[string]any{"secret": "testsecret123", "expires": int64(3600)})
	if err != nil {
		panic(err)
	}
	return &manager{tokenmgr: tokenmgr}
}

func mintTestToken(mgr *manager, shareID string) string {
	u := &userpb.User{
		Id: &userpb.UserId{OpaqueId: "remote-user", Idp: "remote.example.com", Type: userpb.UserType_USER_TYPE_FEDERATED},
	}
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: shareID},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
	}
	scopes, err := scope.AddCodeFlowOCMShareScope(s, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		panic(err)
	}
	tok, err := mgr.tokenmgr.MintToken(context.Background(), u, scopes)
	if err != nil {
		panic(err)
	}
	return tok
}

func TestAuthenticateValidJWT(t *testing.T) {
	mgr := testJWTManager()
	tok := mintTestToken(mgr, "share-abc")

	user, scopes, err := mgr.Authenticate(context.Background(), "share-abc", tok)
	if err != nil {
		t.Fatal(err)
	}
	if user.Id.OpaqueId != "remote-user" {
		t.Errorf("user: got %s, want remote-user", user.Id.OpaqueId)
	}
	shares, _ := scope.GetOCMSharesFromScopes(scopes)
	if len(shares) != 1 || shares[0].Id.GetOpaqueId() != "share-abc" {
		t.Errorf("expected scope with share-abc, got %v", shares)
	}
}

func TestAuthenticateMismatchedShareID(t *testing.T) {
	mgr := testJWTManager()
	tok := mintTestToken(mgr, "share-abc")

	_, _, err := mgr.Authenticate(context.Background(), "different-share", tok)
	if err == nil {
		t.Fatal("expected error for mismatched shareID")
	}
	if _, ok := err.(errtypes.InvalidCredentials); !ok {
		t.Errorf("expected InvalidCredentials, got %T: %v", err, err)
	}
}

func TestAuthenticateEmptyShareIDAcceptsAny(t *testing.T) {
	mgr := testJWTManager()
	tok := mintTestToken(mgr, "share-abc")

	user, _, err := mgr.Authenticate(context.Background(), "", tok)
	if err != nil {
		t.Fatal(err)
	}
	if user.Id.OpaqueId != "remote-user" {
		t.Errorf("user: got %s, want remote-user", user.Id.OpaqueId)
	}
}

func TestAuthenticateInvalidToken(t *testing.T) {
	mgr := testJWTManager()

	_, _, err := mgr.Authenticate(context.Background(), "share-abc", "not-a-jwt")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if _, ok := err.(errtypes.InvalidCredentials); !ok {
		t.Errorf("expected InvalidCredentials, got %T: %v", err, err)
	}
}

func TestAuthenticateExpiredToken(t *testing.T) {
	// Use a JWT manager with 0 TTL so the token is already expired
	tokenmgr, err := jwt.New(map[string]any{"secret": "testsecret123", "expires": int64(-1)})
	if err != nil {
		t.Fatal(err)
	}
	mgr := &manager{tokenmgr: tokenmgr}

	u := &userpb.User{
		Id: &userpb.UserId{OpaqueId: "remote-user", Idp: "remote.example.com"},
	}
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share-abc"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
	}
	scopes, _ := scope.AddCodeFlowOCMShareScope(s, authpb.Role_ROLE_VIEWER, nil)
	tok, _ := tokenmgr.MintToken(context.Background(), u, scopes)

	_, _, err = mgr.Authenticate(context.Background(), "share-abc", tok)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if _, ok := err.(errtypes.InvalidCredentials); !ok {
		t.Errorf("expected InvalidCredentials, got %T: %v", err, err)
	}
}
