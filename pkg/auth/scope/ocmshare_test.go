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
)

func TestCheckStorageRefForOCMShareWithShareID(t *testing.T) {
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "longlivedtoken",
	}

	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{OpaqueId: "share123"},
	}
	if !checkStorageRefForOCMShare(s, ref, "/ocm") {
		t.Error("expected shareId-based ref to match")
	}
}

func TestCheckStorageRefForOCMShareWithToken(t *testing.T) {
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "longlivedtoken",
	}

	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{OpaqueId: "longlivedtoken"},
	}
	if !checkStorageRefForOCMShare(s, ref, "/ocm") {
		t.Error("expected token-based ref to match")
	}
}

func TestCheckStorageRefForOCMShareWithPath(t *testing.T) {
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "longlivedtoken",
	}

	ref := &provider.Reference{Path: "/ocm/share123/somefile.txt"}
	if !checkStorageRefForOCMShare(s, ref, "/ocm") {
		t.Error("expected shareId-path ref to match")
	}

	ref = &provider.Reference{Path: "/ocm/longlivedtoken/somefile.txt"}
	if !checkStorageRefForOCMShare(s, ref, "/ocm") {
		t.Error("expected token-path ref to match")
	}
}

func TestCheckStorageRefEmptyShareIDDoesNotMatchAll(t *testing.T) {
	// Regression: empty shareId with HasPrefix was always-true
	s := &ocmv1beta1.Share{
		Id:         nil,
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "tok",
	}

	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{OpaqueId: "unrelated-resource"},
	}
	if checkStorageRefForOCMShare(s, ref, "/ocm") {
		t.Error("nil Id share should not match arbitrary resource via empty shareId prefix")
	}

	ref = &provider.Reference{Path: "/ocm/anything/file.txt"}
	if checkStorageRefForOCMShare(s, ref, "/ocm") {
		t.Error("nil Id share should not match arbitrary path via empty shareId prefix")
	}
}

func TestCheckStorageRefPathPrefixOcmInFilename(t *testing.T) {
	// Regression: path /ocm-m6-proof.txt was mistaken for OCM share because
	// HasPrefix("/ocm-m6-proof.txt", "/ocm") was true.
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "tok",
	}

	for _, path := range []string{"/ocm-m6-proof.txt", "/ocm-foo", "/ocm"} {
		ref := &provider.Reference{Path: path}
		if checkStorageRefForOCMShare(s, ref, "/ocm") {
			t.Errorf("path %q must not match OCM share (filename/folder starting with ocm)", path)
		}
	}
}

func TestCheckStorageRefResourceIdEmptyToken(t *testing.T) {
	// Regression: code-flow scope has empty Token; strings.HasPrefix(anyId, "") is true in Go,
	// so refs with ResourceId (e.g. personal file ocm-whatever.txt) must not match.
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "share-res"},
		Token:      "", // code-flow scope omits token
	}
	ref := &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "other", OpaqueId: "ocm-whatever-file-id"}}
	if checkStorageRefForOCMShare(s, ref, "/ocm") {
		t.Error("ref with ResourceId must not match OCM share when share has empty Token")
	}
	// Exact ResourceId match should still allow
	refSame := &provider.Reference{ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "share-res"}}
	if !checkStorageRefForOCMShare(s, refSame, "/ocm") {
		t.Error("ref matching share ResourceId should match")
	}
}

func TestCheckStorageRefPathEmptyTokenDoesNotMatchOtherShare(t *testing.T) {
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "share-res"},
		Token:      "",
	}

	ref := &provider.Reference{Path: "/ocm/other-share/file.txt"}
	if checkStorageRefForOCMShare(s, ref, "/ocm") {
		t.Error("code-flow share with empty token must not match another share path")
	}

	refSame := &provider.Reference{Path: "/ocm/share123/file.txt"}
	if !checkStorageRefForOCMShare(s, refSame, "/ocm") {
		t.Error("code-flow share should still match its own share-id path")
	}
}

func TestCheckOCMShareRefByID(t *testing.T) {
	s := &ocmv1beta1.Share{
		Id:    &ocmv1beta1.ShareId{OpaqueId: "share123"},
		Token: "tok",
	}

	ref := &ocmv1beta1.ShareReference{
		Spec: &ocmv1beta1.ShareReference_Id{
			Id: &ocmv1beta1.ShareId{OpaqueId: "share123"},
		},
	}
	if !checkOCMShareRef(s, ref) {
		t.Error("expected match by share ID")
	}

	ref = &ocmv1beta1.ShareReference{
		Spec: &ocmv1beta1.ShareReference_Id{
			Id: &ocmv1beta1.ShareId{OpaqueId: "other"},
		},
	}
	if checkOCMShareRef(s, ref) {
		t.Error("expected no match for different share ID")
	}
}

func TestCheckOCMShareRefByToken(t *testing.T) {
	s := &ocmv1beta1.Share{
		Id:    &ocmv1beta1.ShareId{OpaqueId: "share123"},
		Token: "tok",
	}

	ref := &ocmv1beta1.ShareReference{
		Spec: &ocmv1beta1.ShareReference_Token{Token: "tok"},
	}
	if !checkOCMShareRef(s, ref) {
		t.Error("expected match by token")
	}
}

func TestAddCodeFlowScopeOmitsToken(t *testing.T) {
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "should-not-appear",
		Creator:    &userpb.UserId{OpaqueId: "creator"},
		AccessMethods: []*ocmv1beta1.AccessMethod{
			{
				Term: &ocmv1beta1.AccessMethod_WebdavOptions{
					WebdavOptions: &ocmv1beta1.WebDAVAccessMethod{
						Permissions: &provider.ResourcePermissions{Stat: true},
					},
				},
			},
		},
	}

	scopes, err := AddCodeFlowOCMShareScope(s, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}

	shares, err := GetOCMSharesFromScopes(scopes)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 {
		t.Fatalf("expected 1 share in scope, got %d", len(shares))
	}
	if shares[0].Token != "" {
		t.Errorf("code-flow scope should not carry token, got %q", shares[0].Token)
	}
	if shares[0].Id.GetOpaqueId() != "share123" {
		t.Errorf("scope share Id: got %s, want share123", shares[0].Id.GetOpaqueId())
	}
	if shares[0].Creator == nil || len(shares[0].AccessMethods) != 1 {
		t.Fatalf("code-flow scope lost share metadata: %+v", shares[0])
	}
}

func TestCheckStorageRefShareIDPrefixRejects(t *testing.T) {
	// share-id "share123" must NOT match a resource whose OpaqueId merely starts with it.
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "longlivedtoken",
	}
	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{OpaqueId: "share123extra"},
	}
	if checkStorageRefForOCMShare(s, ref, "/ocm") {
		t.Error("share-id prefix of a longer opaque id must reject")
	}
}

func TestCheckStorageRefTokenPrefixRejects(t *testing.T) {
	// token "legacy-token" must NOT match a resource whose OpaqueId merely starts with it.
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share-xyz"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "legacy-token",
	}
	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{OpaqueId: "legacy-token-extra"},
	}
	if checkStorageRefForOCMShare(s, ref, "/ocm") {
		t.Error("token prefix of a longer opaque id must reject")
	}
}

func TestAddOCMShareScopeCarriesToken(t *testing.T) {
	s := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "the-token",
		Creator:    &userpb.UserId{OpaqueId: "creator"},
		AccessMethods: []*ocmv1beta1.AccessMethod{
			{
				Term: &ocmv1beta1.AccessMethod_WebdavOptions{
					WebdavOptions: &ocmv1beta1.WebDAVAccessMethod{
						Permissions: &provider.ResourcePermissions{Stat: true},
					},
				},
			},
		},
	}

	scopes, err := AddOCMShareScope(s, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}

	shares, err := GetOCMSharesFromScopes(scopes)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 {
		t.Fatalf("expected 1 share in scope, got %d", len(shares))
	}
	if shares[0].Token != "the-token" {
		t.Errorf("legacy scope should carry token, got %q", shares[0].Token)
	}
	if shares[0].Creator == nil || len(shares[0].AccessMethods) != 1 {
		t.Fatalf("legacy scope lost share metadata: %+v", shares[0])
	}
}
