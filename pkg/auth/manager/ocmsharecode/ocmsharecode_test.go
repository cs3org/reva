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
	"encoding/base64"
	"strings"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocminvite "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/token"
	jwtpkg "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	jwtv5 "github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
)

// testJWTSigningKey is a local HS256 fixture, not a deployed credential.
const testJWTSigningKey = "test-jwt-signing-key"

// mockGW is a minimal gateway mock that satisfies gateway.GatewayAPIClient.
// Only GetOCMShareByToken and GetAcceptedUser are implemented.
type mockGW struct {
	gateway.GatewayAPIClient

	share     *ocm.Share
	shareErr  rpc.Code
	shareMsg  string
	remoteErr rpc.Code
	remoteMsg string
	lastToken string
}

func (m *mockGW) GetOCMShareByToken(_ context.Context, req *ocm.GetOCMShareByTokenRequest, _ ...grpc.CallOption) (*ocm.GetOCMShareByTokenResponse, error) {
	if req != nil {
		m.lastToken = req.Token
	}
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
	stampGateway(&mockGW{share: s, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK})
	mgr := &manager{
		c: &config{},
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
	stampGateway(&mockGW{share: s, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK})
	mgr := &manager{
		c: &config{},
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
	stampGateway(&mockGW{share: s, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK})
	mgr := &manager{
		c: &config{},
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
	stampGateway(&mockGW{shareErr: rpc.Code_CODE_NOT_FOUND, shareMsg: "not found"})
	mgr := &manager{
		c: &config{},
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
	stampGateway(&mockGW{shareErr: rpc.Code_CODE_PERMISSION_DENIED, shareMsg: "denied"})
	mgr := &manager{
		c: &config{},
	}

	_, _, err := mgr.Authenticate(context.Background(), "share-abc", "bad-code")
	if err == nil {
		t.Fatal("expected error for permission denied")
	}
	if _, ok := err.(errtypes.InvalidCredentials); !ok {
		t.Errorf("expected InvalidCredentials, got %T: %v", err, err)
	}
}

func distinctShare(opaqueID, resourceID, ownerIdP, senderDomain string) *ocm.Share {
	s := testShare(opaqueID, "exchange-code-"+opaqueID)
	s.ResourceId = &provider.ResourceId{StorageId: "stor", OpaqueId: resourceID}
	s.Owner = &userpb.UserId{OpaqueId: "owner", Idp: ownerIdP}
	s.Creator = &userpb.UserId{OpaqueId: "creator", Idp: senderDomain}
	return s
}

func newTestMinter(t *testing.T) token.Manager {
	t.Helper()
	mgr, err := jwtpkg.New(map[string]any{
		"secret":  testJWTSigningKey,
		"expires": int64(3600),
	})
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

type clientIDClaims struct {
	ClientID string `json:"client_id"`
	jwtv5.RegisteredClaims
}

func signedClientID(t *testing.T, mgr token.Manager, raw string) string {
	t.Helper()
	parsed, err := jwtv5.ParseWithClaims(raw, &clientIDClaims{}, func(tok *jwtv5.Token) (any, error) {
		return []byte(testJWTSigningKey), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Method.Alg() != "HS256" || !parsed.Valid {
		t.Fatalf("signature: alg=%s valid=%v", parsed.Method.Alg(), parsed.Valid)
	}
	if _, _, err := mgr.DismantleToken(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	exp, ok := mgr.(token.ValidatedExpiry)
	if !ok {
		t.Fatal("minter does not implement ValidatedExpiry")
	}
	if _, err := exp.ValidatedExpiresAt(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	claims, ok := parsed.Claims.(*clientIDClaims)
	if !ok {
		t.Fatal("unexpected claims type")
	}
	return claims.ClientID
}

func TestAuthenticateClientIDClaimFollowsResolvedShare(t *testing.T) {
	alpha := distinctShare(
		"share-alpha",
		"res-alpha",
		"owner-alpha.example",
		"sender-alpha.example",
	)
	beta := distinctShare(
		"share-beta",
		"res-beta",
		"owner-beta.example",
		"sender-beta.example",
	)
	minter := newTestMinter(t)

	firstReceiver := "receiver-a.example"
	secondReceiver := "receiver-b.example"
	gotAlphaFirst := mintAuthenticatedShare(t, minter, alpha, firstReceiver)
	gotAlphaSecond := mintAuthenticatedShare(t, minter, alpha, secondReceiver)
	gotBeta := mintAuthenticatedShare(t, minter, beta, firstReceiver)

	if gotAlphaFirst != "share-alpha" || gotAlphaSecond != "share-alpha" {
		t.Fatalf("alpha client_id: %q and %q, want share-alpha", gotAlphaFirst, gotAlphaSecond)
	}
	if gotBeta != "share-beta" {
		t.Fatalf("beta client_id: got %q, want share-beta", gotBeta)
	}
	if gotAlphaFirst == gotBeta {
		t.Fatal("client_id leaked between shares")
	}
}

func mintAuthenticatedShare(t *testing.T, minter token.Manager, share *ocm.Share, receiver string) string {
	t.Helper()
	gw := &mockGW{share: share, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK}
	stampGateway(gw)
	mgr := &manager{c: &config{}}

	user, scopes, err := mgr.Authenticate(context.Background(), receiver, share.Token)
	if err != nil {
		t.Fatal(err)
	}
	if gw.lastToken != share.Token {
		t.Fatalf("lookup key: got %q, want exchanged code %q", gw.lastToken, share.Token)
	}
	if gw.lastToken == receiver {
		t.Fatal("request client_id was used as the share lookup key")
	}

	shares, err := scope.GetOCMSharesFromScopes(scopes)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 {
		t.Fatalf("scope shares: got %d, want 1", len(shares))
	}
	if shares[0].GetId().GetOpaqueId() != share.Id.OpaqueId {
		t.Fatalf("scope opaque id: got %q, want %q", shares[0].GetId().GetOpaqueId(), share.Id.OpaqueId)
	}
	if shares[0].Token != "" {
		t.Fatalf("code-flow scope carried token %q", shares[0].Token)
	}
	if shares[0].GetResourceId().GetOpaqueId() != share.ResourceId.OpaqueId {
		t.Fatalf("resource id: got %q", shares[0].GetResourceId().GetOpaqueId())
	}

	raw, err := minter.MintToken(context.Background(), user, scopes)
	if err != nil {
		t.Fatal(err)
	}
	clientID := signedClientID(t, minter, raw)
	for _, unexpected := range []string{
		receiver,
		share.ResourceId.OpaqueId,
		share.Owner.Idp,
		share.Creator.Idp,
		share.Token,
	} {
		if clientID == unexpected {
			t.Fatalf("client_id %q matched %q instead of the share opaque id", clientID, unexpected)
		}
	}

	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts: got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if bytesContains(payload, share.Token) {
		t.Fatal("code-flow token embedded the exchanged code")
	}
	return clientID
}

func bytesContains(haystack []byte, needle string) bool {
	return strings.Contains(string(haystack), needle)
}

func TestAuthenticateMissingProviderID(t *testing.T) {
	tests := []struct {
		name  string
		share *ocm.Share
	}{
		{name: "nil share", share: nil},
		{name: "nil id", share: testShare("share-abc", "code123")},
		{name: "blank opaque id", share: testShare("", "code123")},
		{name: "whitespace opaque id", share: testShare("   ", "code123")},
	}
	tests[1].share.Id = nil

	minter := newTestMinter(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stampGateway(&mockGW{share: tt.share, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK})
			mgr := &manager{c: &config{}}
			user, scopes, err := mgr.Authenticate(context.Background(), "receiver.example", "code123")
			if err == nil {
				t.Fatal("expected missing provider id to fail code-flow auth")
			}
			if _, ok := err.(errtypes.InvalidCredentials); !ok {
				t.Errorf("expected InvalidCredentials, got %T: %v", err, err)
			}
			if user != nil || scopes != nil {
				t.Fatalf("auth result: user=%v scopes=%v, want no token inputs", user, scopes)
			}
		})
	}

	blank, err := scope.AddCodeFlowOCMShareScope(testShare(" ", "code123"), authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := minter.MintToken(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "accepted-user", Idp: "remote.example.com"},
	}, blank)
	if err == nil || raw != "" {
		t.Fatalf("blank provider id mint: token=%q err=%v", raw, err)
	}
}

func TestMintAmbiguousOrMalformedCodeFlowScope(t *testing.T) {
	minter := newTestMinter(t)
	user := &userpb.User{Id: &userpb.UserId{OpaqueId: "accepted-user", Idp: "remote.example.com"}}

	alphaShare := distinctShare("share-alpha", "res-alpha", "owner-a.example", "sender-a.example")
	alpha, err := scope.AddCodeFlowOCMShareScope(alphaShare, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	betaShare := distinctShare("share-beta", "res-beta", "owner-b.example", "sender-b.example")
	both, err := scope.AddCodeFlowOCMShareScope(betaShare, authpb.Role_ROLE_VIEWER, alpha)
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := minter.MintToken(context.Background(), user, both); err == nil || raw != "" {
		t.Fatalf("ambiguous mint: token=%q err=%v", raw, err)
	}

	malformed := map[string]*authpb.Scope{
		"ocmshare:broken": {
			Resource: &types.OpaqueEntry{Decoder: "json", Value: []byte("{")},
			Role:     authpb.Role_ROLE_VIEWER,
		},
	}
	if raw, err := minter.MintToken(context.Background(), user, malformed); err == nil || raw != "" {
		t.Fatalf("malformed mint: token=%q err=%v", raw, err)
	}
}
