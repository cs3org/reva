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
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
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
	"github.com/cs3org/reva/v3/pkg/utils"
	jwtv5 "github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
)

// testJWTSigningKey is a local HS256 fixture, not a deployed credential.
const testJWTSigningKey = "test-jwt-signing-key"

// mockGW is a minimal gateway mock that satisfies gateway.GatewayAPIClient.
// Only GetOCMShareByToken and GetAcceptedUser are implemented.
type mockGW struct {
	gateway.GatewayAPIClient

	share         *ocm.Share
	shareErr      rpc.Code
	shareMsg      string
	remoteErr     rpc.Code
	remoteMsg     string
	lastToken     string
	shareCalls    int
	acceptedCalls int
	shareRPCErr   error
	nilResponse   bool
	nilStatus     bool
	lastAccepted  *ocminvite.GetAcceptedUserRequest
}

func (m *mockGW) GetOCMShareByToken(
	_ context.Context,
	req *ocm.GetOCMShareByTokenRequest,
	_ ...grpc.CallOption,
) (*ocm.GetOCMShareByTokenResponse, error) {
	m.shareCalls++
	if req != nil {
		m.lastToken = req.Token
	}
	if m.shareRPCErr != nil {
		return nil, m.shareRPCErr
	}
	if m.nilResponse {
		return nil, nil
	}
	var status *rpc.Status
	if !m.nilStatus {
		status = &rpc.Status{Code: m.shareErr, Message: m.shareMsg}
	}
	return &ocm.GetOCMShareByTokenResponse{
		Status: status,
		Share:  m.share,
	}, nil
}

func (m *mockGW) GetAcceptedUser(
	_ context.Context,
	req *ocminvite.GetAcceptedUserRequest,
	_ ...grpc.CallOption,
) (*ocminvite.GetAcceptedUserResponse, error) {
	m.acceptedCalls++
	m.lastAccepted = req
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

	user, scopes, err := mgr.Authenticate(context.Background(), "remote.example.com", "code123")
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

	user, _, err := mgr.Authenticate(context.Background(), "remote.example.com", "code123")
	if err != nil {
		t.Fatal(err)
	}
	if user.Id.OpaqueId != "accepted-user" {
		t.Errorf("user: got %s, want accepted-user", user.Id.OpaqueId)
	}
}

func TestAuthenticateShareNotFound(t *testing.T) {
	gw := &mockGW{shareErr: rpc.Code_CODE_NOT_FOUND, shareMsg: "not found"}
	stampGateway(gw)
	mgr := &manager{
		c: &config{},
	}

	_, _, err := mgr.Authenticate(context.Background(), "remote.example.com", "bad-code")
	if err == nil {
		t.Fatal("expected error for not found share")
	}
	if _, ok := err.(errtypes.NotFound); !ok {
		t.Errorf("expected NotFound, got %T: %v", err, err)
	}
	if gw.shareCalls != 1 || gw.acceptedCalls != 0 {
		t.Fatalf("calls: share=%d accepted=%d", gw.shareCalls, gw.acceptedCalls)
	}
}

func TestAuthenticatePermissionDenied(t *testing.T) {
	gw := &mockGW{shareErr: rpc.Code_CODE_PERMISSION_DENIED, shareMsg: "denied"}
	stampGateway(gw)
	mgr := &manager{
		c: &config{},
	}

	_, _, err := mgr.Authenticate(context.Background(), "remote.example.com", "bad-code")
	if err == nil {
		t.Fatal("expected error for permission denied")
	}
	if _, ok := err.(errtypes.InvalidCredentials); !ok {
		t.Errorf("expected InvalidCredentials, got %T: %v", err, err)
	}
	if gw.shareCalls != 1 || gw.acceptedCalls != 0 {
		t.Fatalf("calls: share=%d accepted=%d", gw.shareCalls, gw.acceptedCalls)
	}
}

func applyShareMutators(share *ocm.Share, mutators ...func(*ocm.Share)) *ocm.Share {
	for _, mutate := range mutators {
		mutate(share)
	}
	return share
}

func mutateResource(resourceID string) func(*ocm.Share) {
	return func(s *ocm.Share) {
		s.ResourceId = &provider.ResourceId{StorageId: "stor", OpaqueId: resourceID}
	}
}

func mutateOwner(idp string) func(*ocm.Share) {
	return func(s *ocm.Share) {
		s.Owner = &userpb.UserId{OpaqueId: "owner", Idp: idp}
	}
}

func mutateCreator(idp string) func(*ocm.Share) {
	return func(s *ocm.Share) {
		s.Creator = &userpb.UserId{OpaqueId: "creator", Idp: idp}
	}
}

func mutateRecipient(idp string) func(*ocm.Share) {
	return func(s *ocm.Share) {
		if s.GetGrantee().GetUserId() == nil {
			s.Grantee = &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id: &provider.Grantee_UserId{
					UserId: &userpb.UserId{
						OpaqueId: "grantee",
						Idp:      idp,
						Type:     userpb.UserType_USER_TYPE_FEDERATED,
					},
				},
			}
			return
		}
		s.Grantee.GetUserId().Idp = idp
	}
}

func mutateNilGrantee(s *ocm.Share) {
	s.Grantee = nil
}

func mutateAbsentGranteeID(s *ocm.Share) {
	s.Grantee = &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER}
}

func mutateGroupGrantee(s *ocm.Share) {
	s.Grantee = &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
		Id: &provider.Grantee_GroupId{
			GroupId: &grouppb.GroupId{OpaqueId: "grp", Idp: "receiver-a.example"},
		},
	}
}

func mutateNilUserPayload(s *ocm.Share) {
	s.Grantee = &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id:   &provider.Grantee_UserId{},
	}
}

func mutateMissingOpaqueID(s *ocm.Share) {
	s.Id = nil
}

func mutateBlankOpaqueID(s *ocm.Share) {
	s.Id = &ocm.ShareId{OpaqueId: "   "}
}

func mutateMissingOpaqueAndGrantee(s *ocm.Share) {
	s.Id = nil
	s.Grantee = nil
}

func mutateEmptyRecipient(s *ocm.Share) {
	mutateRecipient("")(s)
}

func mutateInvalidRecipient(s *ocm.Share) {
	mutateRecipient("https://receiver.example")(s)
}

func distinctShare(opaqueID, resourceID, ownerIdP, senderDomain, recipient string) *ocm.Share {
	share := testShare(opaqueID, "exchange-code-"+opaqueID)
	return applyShareMutators(
		share,
		mutateResource(resourceID),
		mutateOwner(ownerIdP),
		mutateCreator(senderDomain),
		mutateRecipient(recipient),
	)
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
		"receiver-a.example",
	)
	beta := distinctShare(
		"share-beta",
		"res-beta",
		"owner-beta.example",
		"sender-beta.example",
		"receiver-b.example",
	)
	minter := newTestMinter(t)

	gotAlpha := mintAuthenticatedShare(t, minter, alpha, "receiver-a.example")
	gotBeta := mintAuthenticatedShare(t, minter, beta, "receiver-b.example")
	if gotAlpha != "share-alpha" {
		t.Fatalf("alpha client_id: got %q, want share-alpha", gotAlpha)
	}
	if gotBeta != "share-beta" {
		t.Fatalf("beta client_id: got %q, want share-beta", gotBeta)
	}
	if gotAlpha == gotBeta {
		t.Fatal("client_id leaked between shares")
	}

	gw := &mockGW{share: alpha, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK}
	stampGateway(gw)
	mgr := &manager{c: &config{}}
	user, scopes, err := mgr.Authenticate(context.Background(), "receiver-b.example", alpha.Token)
	cred, ok := err.(errtypes.InvalidCredentials)
	if !ok || string(cred) != "ocm share receiver does not match client_id" {
		t.Fatalf("unrelated receiver: got %T %v", err, err)
	}
	if user != nil || scopes != nil {
		t.Fatalf("auth result: user=%v scopes=%v, want no token inputs", user, scopes)
	}
	if gw.shareCalls != 1 || gw.acceptedCalls != 0 {
		t.Fatalf("calls: share=%d accepted=%d", gw.shareCalls, gw.acceptedCalls)
	}
	if gw.lastToken != alpha.Token {
		t.Fatalf("lookup key: got %q, want %q", gw.lastToken, alpha.Token)
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

	if gw.acceptedCalls != 1 || gw.lastAccepted == nil {
		t.Fatalf("accepted-user lookups: %d", gw.acceptedCalls)
	}
	if gw.lastAccepted.GetRemoteUserId().GetOpaqueId() != share.GetGrantee().GetUserId().GetOpaqueId() {
		t.Fatalf("accepted user: got %#v", gw.lastAccepted.GetRemoteUserId())
	}
	if gw.lastAccepted.GetRemoteUserId().GetIdp() != share.GetGrantee().GetUserId().GetIdp() {
		t.Fatalf("accepted user idp: got %q", gw.lastAccepted.GetRemoteUserId().GetIdp())
	}
	filter := gw.lastAccepted.GetOpaque().GetMap()["user-filter"]
	if filter == nil || filter.Decoder != "json" {
		t.Fatal("accepted-user filter missing")
	}
	var filtered userpb.UserId
	if err := utils.UnmarshalJSONToProtoV1(filter.Value, &filtered); err != nil {
		t.Fatal(err)
	}
	if filtered.GetOpaqueId() != share.GetCreator().GetOpaqueId() || filtered.GetIdp() != share.GetCreator().GetIdp() {
		t.Fatalf("user filter: got %#v, want creator", filtered)
	}
	if filtered.GetIdp() == share.GetGrantee().GetUserId().GetIdp() {
		t.Fatal("user filter used the recipient instead of the creator")
	}
	if share.GetOwner().GetIdp() != share.GetCreator().GetIdp() && filtered.GetIdp() == share.GetOwner().GetIdp() {
		t.Fatal("user filter used the owner instead of the creator")
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
	storedRecipient := share.GetGrantee().GetUserId().GetIdp()
	for _, unexpected := range []string{
		receiver,
		storedRecipient,
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
		{name: "nil id", share: applyShareMutators(testShare("share-abc", "code123"), mutateMissingOpaqueID)},
		{name: "blank opaque id", share: testShare("", "code123")},
		{name: "whitespace opaque id", share: testShare("   ", "code123")},
	}

	minter := newTestMinter(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &mockGW{share: tt.share, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK}
			stampGateway(gw)
			mgr := &manager{c: &config{}}
			user, scopes, err := mgr.Authenticate(context.Background(), "receiver.example", "code123")
			if err == nil {
				t.Fatal("expected missing provider id to fail code-flow auth")
			}
			cred, ok := err.(errtypes.InvalidCredentials)
			if !ok || string(cred) != "ocm share is missing provider id" {
				t.Errorf("expected InvalidCredentials missing provider id, got %T: %v", err, err)
			}
			if user != nil || scopes != nil {
				t.Fatalf("auth result: user=%v scopes=%v, want no token inputs", user, scopes)
			}
			if gw.shareCalls != 1 || gw.acceptedCalls != 0 {
				t.Fatalf("calls: share=%d accepted=%d", gw.shareCalls, gw.acceptedCalls)
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

	alphaShare := distinctShare("share-alpha", "res-alpha", "owner-a.example", "sender-a.example", "receiver-a.example")
	alpha, err := scope.AddCodeFlowOCMShareScope(alphaShare, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	betaShare := distinctShare("share-beta", "res-beta", "owner-b.example", "sender-b.example", "receiver-b.example")
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
