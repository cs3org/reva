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

package ocmshare

import (
	"strings"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/utils"
)

func sampleShare(opaqueID, token string) *ocmv1beta1.Share {
	return &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: opaqueID},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res-" + opaqueID},
		Token:      token,
		Creator:    &userpb.UserId{OpaqueId: "creator", Idp: "sender.example"},
	}
}

func jsonScope(t *testing.T, share *ocmv1beta1.Share) *authpb.Scope {
	t.Helper()
	val, err := utils.MarshalProtoV1ToJSON(share)
	if err != nil {
		t.Fatal(err)
	}
	return &authpb.Scope{
		Resource: &types.OpaqueEntry{Decoder: "json", Value: val},
		Role:     authpb.Role_ROLE_VIEWER,
	}
}

func requireInternal(t *testing.T, err error) {
	t.Helper()
	got, ok := err.(errtypes.InternalError)
	if !ok || string(got) != "resource should be json encoded" {
		t.Fatalf("error: got %T %v, want InternalError %q", err, err, "resource should be json encoded")
	}
}

func TestSharesFromScopes(t *testing.T) {
	codeFlow := jsonScope(t, sampleShare("share-alpha", ""))
	legacy := jsonScope(t, sampleShare("legacy-share", "legacy-token"))

	tests := []struct {
		name    string
		scopes  map[string]*authpb.Scope
		wantNil bool
		wantIDs map[string]string
		wantErr string
	}{
		{name: "nil map", scopes: nil, wantNil: true},
		{name: "empty map", scopes: map[string]*authpb.Scope{}, wantNil: true},
		{
			name:    "unrelated nil scope",
			scopes:  map[string]*authpb.Scope{"user": nil, "client_id": nil},
			wantNil: true,
		},
		{
			name: "nil matching scope",
			scopes: map[string]*authpb.Scope{
				"ocmshare:nil": nil,
				"user":         nil,
			},
			wantErr: "internal",
		},
		{
			name: "nil matching resource",
			scopes: map[string]*authpb.Scope{
				"ocmshare:nil-resource": {Role: authpb.Role_ROLE_VIEWER},
			},
			wantErr: "internal",
		},
		{
			name: "wrong decoder",
			scopes: map[string]*authpb.Scope{
				"ocmshare:plain": {
					Resource: &types.OpaqueEntry{Decoder: "plain", Value: []byte("share-alpha")},
					Role:     authpb.Role_ROLE_VIEWER,
				},
			},
			wantErr: "internal",
		},
		{
			name: "malformed json",
			scopes: map[string]*authpb.Scope{
				"ocmshare:broken": {
					Resource: &types.OpaqueEntry{Decoder: "json", Value: []byte("{")},
					Role:     authpb.Role_ROLE_VIEWER,
				},
			},
			wantErr: "json",
		},
		{
			name:   "code-flow share",
			scopes: map[string]*authpb.Scope{"ocmshare:other-key": codeFlow},
			wantIDs: map[string]string{
				"share-alpha": "",
			},
		},
		{
			name:   "legacy share",
			scopes: map[string]*authpb.Scope{"ocmshare:legacy-share": legacy},
			wantIDs: map[string]string{
				"legacy-share": "legacy-token",
			},
		},
		{
			name: "multiple shares",
			scopes: map[string]*authpb.Scope{
				"ocmshare:alpha":  codeFlow,
				"ocmshare:legacy": legacy,
				"user":            nil,
			},
			wantIDs: map[string]string{
				"share-alpha":  "",
				"legacy-share": "legacy-token",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shares, err := SharesFromScopes(tt.scopes)
			switch tt.wantErr {
			case "":
				if err != nil {
					t.Fatal(err)
				}
			case "internal":
				if shares != nil {
					t.Fatalf("shares: got %#v, want nil", shares)
				}
				requireInternal(t, err)
				return
			case "json":
				if shares != nil {
					t.Fatalf("shares: got %#v, want nil", shares)
				}
				var decoded ocmv1beta1.Share
				wantErr := utils.UnmarshalJSONToProtoV1([]byte("{"), &decoded)
				if err == nil || wantErr == nil || err.Error() != wantErr.Error() {
					t.Fatalf("error: got %v, want unmarshal error %v", err, wantErr)
				}
				return
			}
			if tt.wantNil {
				if shares != nil || err != nil {
					t.Fatalf("shares=%#v err=%v, want nil slice", shares, err)
				}
				return
			}
			if shares == nil {
				t.Fatal("expected decoded shares")
			}
			got := map[string]string{}
			for _, share := range shares {
				got[share.GetId().GetOpaqueId()] = share.GetToken()
			}
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("ids: got %#v, want %#v", got, tt.wantIDs)
			}
			for id, token := range tt.wantIDs {
				if got[id] != token {
					t.Fatalf("share %s token: got %q, want %q", id, got[id], token)
				}
			}
		})
	}
}

func TestCodeFlowOCMShareClientIDSelection(t *testing.T) {
	codeFlow := jsonScope(t, sampleShare("Share-Alpha", ""))
	spaced := jsonScope(t, sampleShare(" share alpha ", ""))
	legacy := jsonScope(t, sampleShare("legacy-share", "legacy-token"))
	second := jsonScope(t, sampleShare("share-beta", ""))
	blank := jsonScope(t, sampleShare("   ", ""))
	missing := jsonScope(t, &ocmv1beta1.Share{Id: &ocmv1beta1.ShareId{}})

	tests := []struct {
		name    string
		scopes  map[string]*authpb.Scope
		want    string
		wantErr string
	}{
		{name: "nil map", scopes: nil, want: ""},
		{name: "unrelated only", scopes: map[string]*authpb.Scope{"user": nil}, want: ""},
		{
			name: "exact opaque spelling",
			scopes: map[string]*authpb.Scope{
				"ocmshare:not-the-id": codeFlow,
				"client_id":           nil,
			},
			want: "Share-Alpha",
		},
		{
			name:   "interior spaces stay",
			scopes: map[string]*authpb.Scope{"ocmshare:spaced": spaced},
			want:   " share alpha ",
		},
		{
			name: "legacy is ignored",
			scopes: map[string]*authpb.Scope{
				"ocmshare:legacy": legacy,
				"user":            {Role: authpb.Role_ROLE_OWNER},
			},
			want: "",
		},
		{
			name: "legacy plus code-flow",
			scopes: map[string]*authpb.Scope{
				"ocmshare:legacy": legacy,
				"ocmshare:code":   codeFlow,
			},
			want: "Share-Alpha",
		},
		{
			name: "malformed legacy still fails",
			scopes: map[string]*authpb.Scope{
				"ocmshare:code": codeFlow,
				"ocmshare:broken": {
					Resource: &types.OpaqueEntry{Decoder: "json", Value: []byte("{")},
					Role:     authpb.Role_ROLE_VIEWER,
				},
			},
			wantErr: "json",
		},
		{
			name: "nil legacy resource still fails",
			scopes: map[string]*authpb.Scope{
				"ocmshare:code":   codeFlow,
				"ocmshare:legacy": {Role: authpb.Role_ROLE_VIEWER},
			},
			wantErr: "internal",
		},
		{
			name:    "blank opaque id",
			scopes:  map[string]*authpb.Scope{"ocmshare:blank": blank},
			wantErr: "credentials",
		},
		{
			name:    "missing opaque id",
			scopes:  map[string]*authpb.Scope{"ocmshare:missing": missing},
			wantErr: "credentials",
		},
		{
			name: "multiple code-flow shares",
			scopes: map[string]*authpb.Scope{
				"ocmshare:alpha": codeFlow,
				"ocmshare:beta":  second,
			},
			wantErr: "credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CodeFlowOCMShareClientID(tt.scopes)
			switch tt.wantErr {
			case "":
				if err != nil {
					t.Fatal(err)
				}
				if got != tt.want {
					t.Fatalf("client id: got %q, want %q", got, tt.want)
				}
			case "internal":
				if got != "" {
					t.Fatalf("client id: got %q, want empty", got)
				}
				requireInternal(t, err)
			case "json":
				if got != "" {
					t.Fatalf("client id: got %q, want empty", got)
				}
				var decoded ocmv1beta1.Share
				wantErr := utils.UnmarshalJSONToProtoV1([]byte("{"), &decoded)
				if err == nil || wantErr == nil || err.Error() != wantErr.Error() {
					t.Fatalf("error: got %v, want unmarshal error %v", err, wantErr)
				}
			case "credentials":
				if got != "" {
					t.Fatalf("client id: got %q, want empty", got)
				}
				cred, ok := err.(errtypes.InvalidCredentials)
				if !ok || strings.TrimSpace(string(cred)) == "" {
					t.Fatalf("error: got %T %v, want InvalidCredentials", err, err)
				}
			default:
				t.Fatalf("unknown wantErr %q", tt.wantErr)
			}
		})
	}
}
