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
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/utils"
)

func TestGetOCMSharesFromScopesNilWhenNothingMatches(t *testing.T) {
	tests := []struct {
		name   string
		scopes map[string]*authpb.Scope
	}{
		{name: "nil map", scopes: nil},
		{name: "empty map", scopes: map[string]*authpb.Scope{}},
		{name: "unrelated nil", scopes: map[string]*authpb.Scope{"user": nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shares, err := GetOCMSharesFromScopes(tt.scopes)
			if err != nil {
				t.Fatal(err)
			}
			if shares != nil {
				t.Fatalf("shares: got %#v, want nil slice", shares)
			}
			leaf, leafErr := ocmshare.SharesFromScopes(tt.scopes)
			if leafErr != nil || leaf != nil {
				t.Fatalf("leaf: shares=%#v err=%v", leaf, leafErr)
			}
		})
	}
}

func TestGetOCMSharesFromScopesForwardsDecoder(t *testing.T) {
	share := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share-alpha"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res-alpha"},
		Creator:    &userpb.UserId{OpaqueId: "creator"},
	}
	scopes, err := AddCodeFlowOCMShareScope(share, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := AddOCMShareScope(&ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "legacy-share"},
		ResourceId: share.ResourceId,
		Token:      "legacy-token",
	}, authpb.Role_ROLE_VIEWER, scopes)
	if err != nil {
		t.Fatal(err)
	}

	parent, parentErr := GetOCMSharesFromScopes(legacy)
	leaf, leafErr := ocmshare.SharesFromScopes(legacy)
	if parentErr != nil || leafErr != nil {
		t.Fatalf("parent=%v leaf=%v", parentErr, leafErr)
	}
	if parent == nil || leaf == nil || len(parent) != 2 || len(leaf) != len(parent) {
		t.Fatalf("parent=%d leaf=%d, want two shares", len(parent), len(leaf))
	}

	broken := map[string]*authpb.Scope{
		"ocmshare:broken": {
			Resource: &types.OpaqueEntry{Decoder: "json", Value: []byte("{")},
			Role:     authpb.Role_ROLE_VIEWER,
		},
	}
	_, parentErr = GetOCMSharesFromScopes(broken)
	_, leafErr = ocmshare.SharesFromScopes(broken)
	var decoded ocmv1beta1.Share
	wantErr := utils.UnmarshalJSONToProtoV1([]byte("{"), &decoded)
	if parentErr == nil || leafErr == nil || wantErr == nil ||
		parentErr.Error() != leafErr.Error() || parentErr.Error() != wantErr.Error() {
		t.Fatalf("malformed forward: parent=%v leaf=%v want=%v", parentErr, leafErr, wantErr)
	}

	nilScope := map[string]*authpb.Scope{"ocmshare:nil": nil}
	_, parentErr = GetOCMSharesFromScopes(nilScope)
	_, leafErr = ocmshare.SharesFromScopes(nilScope)
	parentInternal, parentOK := parentErr.(errtypes.InternalError)
	leafInternal, leafOK := leafErr.(errtypes.InternalError)
	if !parentOK || !leafOK || string(parentInternal) != string(leafInternal) ||
		string(parentInternal) != "resource should be json encoded" {
		t.Fatalf("nil scope forward: parent=%v leaf=%v", parentErr, leafErr)
	}
}
