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
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
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

func TestDismantleTokenOCMClientIDClaim(t *testing.T) {
	mgr := newMinter(t, testSigningKey)
	user := federatedUser()
	scopes := codeFlowScope(t, "share-alpha", "exchange-code-alpha")
	raw, err := mgr.MintToken(context.Background(), user, scopes)
	if err != nil {
		t.Fatal(err)
	}
	base := payloadObject(t, raw)

	const mismatch = "ocm token client_id does not match share scope"
	tests := []struct {
		name    string
		rewrite func(map[string]any)
		wantErr string
	}{
		{name: "match"},
		{
			name: "mismatch",
			rewrite: func(payload map[string]any) {
				payload["client_id"] = "share-beta"
			},
			wantErr: mismatch,
		},
		{
			name: "receiver domain claim",
			rewrite: func(payload map[string]any) {
				// Form client_id is the receiver FQDN. The claim must stay the share id.
				payload["client_id"] = "receiver.example"
			},
			wantErr: mismatch,
		},
		{
			name: "claim-less",
			rewrite: func(payload map[string]any) {
				delete(payload, "client_id")
			},
		},
		{
			name: "present empty",
			rewrite: func(payload map[string]any) {
				payload["client_id"] = ""
			},
			wantErr: mismatch,
		},
		{
			name: "present null",
			rewrite: func(payload map[string]any) {
				payload["client_id"] = nil
			},
			wantErr: mismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawToken := raw
			if tt.rewrite != nil {
				payload := clonePayload(t, base)
				tt.rewrite(payload)
				rawToken = resignPayload(t, payload, testSigningKey)
			}

			gotUser, gotScope, err := mgr.DismantleToken(context.Background(), rawToken)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				shares, shareErr := scope.GetOCMSharesFromScopes(gotScope)
				if shareErr != nil {
					t.Fatal(shareErr)
				}
				if len(shares) != 1 || shares[0].GetId().GetOpaqueId() != "share-alpha" {
					t.Fatalf("scope share: %#v", shares)
				}
				if gotUser.GetId().GetOpaqueId() != user.Id.OpaqueId {
					t.Fatalf("user: got %#v", gotUser.GetId())
				}
				return
			}

			cred, ok := err.(errtypes.InvalidCredentials)
			if !ok || string(cred) != tt.wantErr {
				t.Fatalf("error: got %T %v, want InvalidCredentials %q", err, err, tt.wantErr)
			}
			if gotUser != nil || gotScope != nil {
				t.Fatalf("result: user=%v scopes=%v", gotUser, gotScope)
			}
			if _, _, retryErr := mgr.DismantleToken(context.Background(), rawToken); retryErr == nil {
				t.Fatal("rejected token dismantled on retry")
			}
		})
	}
}

func clonePayload(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func resignPayload(t *testing.T, payload map[string]any, secret string) string {
	t.Helper()
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, jwtv5.MapClaims(payload))
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
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

	machineOnly, err := scope.AddMachineScope(nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		scopes map[string]*authpb.Scope
	}{
		{name: "legacy direct-secret", scopes: legacyScope},
		{name: "non-ocm", scopes: nonOCM},
		{name: "machine", scopes: machineOnly},
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
	user := federatedUser()

	ambiguous := codeFlowScope(t, "share-alpha", "exchange-code-alpha")
	extra, err := scope.AddCodeFlowOCMShareScope(&ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share-beta"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res-beta"},
	}, authpb.Role_ROLE_VIEWER, ambiguous)
	if err != nil {
		t.Fatal(err)
	}
	blank := codeFlowScope(t, " ", "exchange-code-blank")
	malformed := map[string]*authpb.Scope{
		"ocmshare:broken": {
			Resource: &types.OpaqueEntry{Decoder: "json", Value: []byte("{")},
			Role:     authpb.Role_ROLE_VIEWER,
		},
	}

	tests := []struct {
		name   string
		scopes map[string]*authpb.Scope
	}{
		{name: "ambiguous", scopes: extra},
		{name: "blank id", scopes: blank},
		{name: "malformed json", scopes: malformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := mgr.MintToken(context.Background(), user, tt.scopes)
			if err == nil || raw != "" {
				t.Fatalf("mint: token=%q err=%v", raw, err)
			}
		})
	}
}

func TestMintTokenMachineRemintKeepsClientID(t *testing.T) {
	mgr := newMinter(t, testSigningKey)
	user := federatedUser()
	scopes := codeFlowScope(t, "share-alpha", "exchange-code-alpha")
	first, err := mgr.MintToken(context.Background(), user, scopes)
	if err != nil {
		t.Fatal(err)
	}
	withMachine, err := scope.AddMachineScope(scopes)
	if err != nil {
		t.Fatal(err)
	}
	second, err := mgr.MintToken(context.Background(), user, withMachine)
	if err != nil {
		t.Fatal(err)
	}

	for _, raw := range []string{first, second} {
		payload := payloadObject(t, raw)
		if payload["client_id"] != "share-alpha" {
			t.Fatalf("client_id: %#v", payload["client_id"])
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "exchange-code-alpha") {
			t.Fatal("reminted token embedded the exchanged code")
		}
		if _, _, err := mgr.DismantleToken(context.Background(), raw); err != nil {
			t.Fatal(err)
		}
	}

	_, dismantledScope, err := mgr.DismantleToken(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := dismantledScope[scope.MachineScope]; !ok {
		t.Fatal("machine scope missing after remint")
	}
	shares, err := scope.GetOCMSharesFromScopes(dismantledScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 || shares[0].GetId().GetOpaqueId() != "share-alpha" || shares[0].Token != "" {
		t.Fatalf("scope share: %#v", shares)
	}
}
