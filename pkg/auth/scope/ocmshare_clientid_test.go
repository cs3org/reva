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

package scope

import (
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope/ocmshare"
)

func codeFlowShare(opaqueID string) *ocmv1beta1.Share {
	return &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: opaqueID},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res-" + opaqueID},
		Creator:    &userpb.UserId{OpaqueId: "creator", Idp: "sender.example"},
		AccessMethods: []*ocmv1beta1.AccessMethod{
			{Term: &ocmv1beta1.AccessMethod_WebdavOptions{
				WebdavOptions: &ocmv1beta1.WebDAVAccessMethod{
					Permissions: &provider.ResourcePermissions{Stat: true},
				},
			}},
		},
	}
}

func TestCodeFlowOCMShareClientID(t *testing.T) {
	share := codeFlowShare("provider-share")
	scopes, err := AddCodeFlowOCMShareScope(share, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	scopes["client_id"] = &authpb.Scope{
		Resource: &types.OpaqueEntry{Decoder: "plain", Value: []byte("request-client")},
		Role:     authpb.Role_ROLE_VIEWER,
	}
	// The map key is not the providerId. Only the decoded share id is.
	moved := scopes["ocmshare:provider-share"]
	delete(scopes, "ocmshare:provider-share")
	scopes["ocmshare:some-other-key"] = moved

	got, err := ocmshare.CodeFlowOCMShareClientID(scopes)
	if err != nil {
		t.Fatal(err)
	}
	if got != "provider-share" {
		t.Fatalf("client id: got %q, want provider-share", got)
	}
}

func TestCodeFlowOCMShareClientIDIgnoresLegacyAndOtherScopes(t *testing.T) {
	legacy := codeFlowShare("legacy-share")
	legacy.Token = "legacy-token-value"
	scopes, err := AddOCMShareScope(legacy, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	scopes["client_id"] = &authpb.Scope{
		Resource: &types.OpaqueEntry{Decoder: "plain", Value: []byte("not-a-provider")},
		Role:     authpb.Role_ROLE_VIEWER,
	}
	scopes["user"] = &authpb.Scope{Role: authpb.Role_ROLE_OWNER}

	got, err := ocmshare.CodeFlowOCMShareClientID(scopes)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("client id: got %q, want none", got)
	}
}

func TestCodeFlowOCMShareClientIDRejectsAmbiguousOrMalformed(t *testing.T) {
	alpha, err := AddCodeFlowOCMShareScope(codeFlowShare("share-alpha"), authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	both, err := AddCodeFlowOCMShareScope(codeFlowShare("share-beta"), authpb.Role_ROLE_VIEWER, alpha)
	if err != nil {
		t.Fatal(err)
	}

	blankShare := codeFlowShare(" ")
	blank, err := AddCodeFlowOCMShareScope(blankShare, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	missingID := codeFlowShare("share-missing")
	missingVal, err := AddCodeFlowOCMShareScope(&ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{},
		ResourceId: missingID.ResourceId,
	}, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		scopes map[string]*authpb.Scope
	}{
		{name: "two code-flow shares", scopes: both},
		{name: "blank opaque id", scopes: blank},
		{name: "missing opaque id", scopes: missingVal},
		{
			name: "malformed json",
			scopes: map[string]*authpb.Scope{
				"ocmshare:broken": {
					Resource: &types.OpaqueEntry{Decoder: "json", Value: []byte("{")},
					Role:     authpb.Role_ROLE_VIEWER,
				},
			},
		},
		{
			name: "non-json ocmshare payload",
			scopes: map[string]*authpb.Scope{
				"ocmshare:plain": {
					Resource: &types.OpaqueEntry{Decoder: "plain", Value: []byte("provider-share")},
					Role:     authpb.Role_ROLE_VIEWER,
				},
			},
		},
		{
			name: "nil ocmshare resource",
			scopes: map[string]*authpb.Scope{
				"ocmshare:nil": {Role: authpb.Role_ROLE_VIEWER},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ocmshare.CodeFlowOCMShareClientID(tt.scopes)
			if err == nil {
				t.Fatal("expected error")
			}
			if got != "" {
				t.Fatalf("client id: got %q, want empty", got)
			}
		})
	}
}
