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

package jwt_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/token"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	jwtv5 "github.com/golang-jwt/jwt/v5"
)

// testSigningKey is a local HS256 fixture, not a deployed credential.
const testSigningKey = "test-jwt-signing-key"

type mintedClaims struct {
	jwtv5.RegisteredClaims
	ClientID string `json:"client_id,omitempty"`
}

func newMinter(t *testing.T, secret string) token.Manager {
	t.Helper()
	mgr, err := jwt.New(map[string]any{"secret": secret, "expires": int64(3600)})
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func federatedUser() *userpb.User {
	return &userpb.User{
		Id: &userpb.UserId{
			OpaqueId: "accepted-user",
			Idp:      "accepted.example",
			Type:     userpb.UserType_USER_TYPE_FEDERATED,
		},
	}
}

func codeFlowScope(t *testing.T, opaqueID, exchangeCode string) map[string]*authpb.Scope {
	t.Helper()
	share := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: opaqueID},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res-" + opaqueID},
		Token:      exchangeCode,
		Creator:    &userpb.UserId{OpaqueId: "creator", Idp: "sender.example"},
		AccessMethods: []*ocmv1beta1.AccessMethod{
			{Term: &ocmv1beta1.AccessMethod_WebdavOptions{
				WebdavOptions: &ocmv1beta1.WebDAVAccessMethod{
					Permissions: &provider.ResourcePermissions{Stat: true},
				},
			}},
		},
	}
	scopes, err := scope.AddCodeFlowOCMShareScope(share, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	return scopes
}

func payloadObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts: got %d", len(parts))
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestMintTokenCodeFlowClientID(t *testing.T) {
	mgr := newMinter(t, testSigningKey)
	user := federatedUser()
	scopes := codeFlowScope(t, "share-alpha", "exchange-code-alpha")
	before := time.Now()
	raw, err := mgr.MintToken(context.Background(), user, scopes)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := jwtv5.ParseWithClaims(raw, &mintedClaims{}, func(tok *jwtv5.Token) (any, error) {
		return []byte(testSigningKey), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Method.Alg() != "HS256" || !parsed.Valid {
		t.Fatalf("signature: alg=%s valid=%v", parsed.Method.Alg(), parsed.Valid)
	}
	got, ok := parsed.Claims.(*mintedClaims)
	if !ok {
		t.Fatal("unexpected claims type")
	}
	if got.ClientID != "share-alpha" {
		t.Fatalf("client_id: got %q, want share-alpha", got.ClientID)
	}
	if got.Issuer != user.Id.Idp {
		t.Fatalf("issuer: got %q, want %q", got.Issuer, user.Id.Idp)
	}
	if len(got.Audience) != 1 || got.Audience[0] != "reva" {
		t.Fatalf("audience: %#v", got.Audience)
	}
	if got.ExpiresAt == nil || got.IssuedAt == nil {
		t.Fatal("expected issued-at and expiry")
	}
	if got.ExpiresAt.Time.Before(before.Add(3590*time.Second)) || got.ExpiresAt.Time.After(before.Add(3610*time.Second)) {
		t.Fatalf("expiry: got %s", got.ExpiresAt.Time)
	}

	dismantledUser, dismantledScope, err := mgr.DismantleToken(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if dismantledUser.GetId().GetOpaqueId() != user.Id.OpaqueId || dismantledUser.GetId().GetIdp() != user.Id.Idp {
		t.Fatalf("user: got %#v", dismantledUser.GetId())
	}
	shares, err := scope.GetOCMSharesFromScopes(dismantledScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 || shares[0].GetId().GetOpaqueId() != "share-alpha" || shares[0].Token != "" {
		t.Fatalf("scope share: %#v", shares)
	}

	exp, ok := mgr.(token.ValidatedExpiry)
	if !ok {
		t.Fatal("minter does not implement ValidatedExpiry")
	}
	expiresAt, err := exp.ValidatedExpiresAt(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !expiresAt.Equal(got.ExpiresAt.Time) {
		t.Fatalf("validated expiry: got %s, want %s", expiresAt, got.ExpiresAt.Time)
	}

	payload := payloadObject(t, raw)
	if payload["client_id"] != "share-alpha" {
		t.Fatalf("payload client_id: %#v", payload["client_id"])
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "exchange-code-alpha") {
		t.Fatal("code-flow token embedded the exchanged code")
	}

	allowed, err := scope.VerifyScope(context.Background(), dismantledScope, "/dav/ocm/share-alpha/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("code-flow scope no longer authorizes its DAV path")
	}
	rejected, err := scope.VerifyScope(context.Background(), dismantledScope, "/dav/ocm/share-beta/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if rejected {
		t.Fatal("client_id claim widened DAV authorization")
	}

	other := newMinter(t, "different-test-signing-key")
	if _, _, err := other.DismantleToken(context.Background(), raw); err == nil {
		t.Fatal("token validated under a different signing key")
	}
}

func TestMintTokenLegacyAndNonOCMOmitClientID(t *testing.T) {
	mgr := newMinter(t, testSigningKey)
	user := federatedUser()

	legacyShare := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "legacy-share"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res-legacy"},
		Token:      "legacy-token-value",
		Creator:    &userpb.UserId{OpaqueId: "creator"},
	}
	legacyScope, err := scope.AddOCMShareScope(legacyShare, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	nonOCM := map[string]*authpb.Scope{
		"user": {Role: authpb.Role_ROLE_OWNER},
	}

	tests := []struct {
		name   string
		scopes map[string]*authpb.Scope
	}{
		{name: "legacy direct-secret", scopes: legacyScope},
		{name: "non-ocm", scopes: nonOCM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := mgr.MintToken(context.Background(), user, tt.scopes)
			if err != nil {
				t.Fatal(err)
			}
			payload := payloadObject(t, raw)
			if _, ok := payload["client_id"]; ok {
				t.Fatalf("client_id present: %#v", payload["client_id"])
			}
			if _, _, err := mgr.DismantleToken(context.Background(), raw); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMintTokenAmbiguousCodeFlowFails(t *testing.T) {
	mgr := newMinter(t, testSigningKey)
	scopes := codeFlowScope(t, "share-alpha", "exchange-code-alpha")
	extra, err := scope.AddCodeFlowOCMShareScope(&ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share-beta"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res-beta"},
	}, authpb.Role_ROLE_VIEWER, scopes)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := mgr.MintToken(context.Background(), federatedUser(), extra)
	if err == nil || raw != "" {
		t.Fatalf("ambiguous mint: token=%q err=%v", raw, err)
	}
}
