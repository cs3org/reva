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

package outcoming

import (
	"context"
	"testing"

	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	authscope "github.com/cs3org/reva/v3/pkg/auth/scope"
)

func TestMakeRelative(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/foo/bar", "./foo/bar"},
		{"foo/bar", "foo/bar"},
		{"/", "./"},
		{"", ""},
	}
	for _, tc := range tests {
		got := makeRelative(tc.in)
		if got != tc.want {
			t.Errorf("makeRelative(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCandidateAndRelativePathFromRef_PathOnly(t *testing.T) {
	tests := []struct {
		name    string
		ref     *provider.Reference
		wantKey string
		wantRel string
	}{
		{
			name:    "share-specific dav path",
			ref:     &provider.Reference{Path: "/ocm/share123/sub/file.txt"},
			wantKey: "share123",
			wantRel: "/sub/file.txt",
		},
		{
			name:    "root-mounted dav path",
			ref:     &provider.Reference{Path: "/ocm"},
			wantKey: "",
			wantRel: "/",
		},
		{
			name:    "legacy bare token path",
			ref:     &provider.Reference{Path: "/token123/sub/file.txt"},
			wantKey: "token123",
			wantRel: "/sub/file.txt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotKey, gotRel := candidateAndRelativePathFromRef(tc.ref)
			if gotKey != tc.wantKey || gotRel != tc.wantRel {
				t.Fatalf("candidateAndRelativePathFromRef(%+v) = (%q, %q), want (%q, %q)", tc.ref, gotKey, gotRel, tc.wantKey, tc.wantRel)
			}
		})
	}
}

func TestCandidateAndRelativePathFromRef_ResourceID(t *testing.T) {
	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{OpaqueId: "share123:dir"},
		Path:       "file.txt",
	}

	gotKey, gotRel := candidateAndRelativePathFromRef(ref)
	if gotKey != "share123" || gotRel != "dir/file.txt" {
		t.Fatalf("candidateAndRelativePathFromRef(%+v) = (%q, %q), want (%q, %q)", ref, gotKey, gotRel, "share123", "dir/file.txt")
	}
}

func TestExposedPathFromReference_PathOnly(t *testing.T) {
	ref := &provider.Reference{Path: "/share123/sub/file.txt"}
	got := exposedPathFromReference(ref)
	if got != "/share123/sub/file.txt" {
		t.Errorf("got %q, want /share123/sub/file.txt", got)
	}
}

func TestExposedPathFromReference_ResourceIdWithColon(t *testing.T) {
	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{StorageId: "tkn:sub/path"},
		Path:       "file.txt",
	}
	got := exposedPathFromReference(ref)
	if got != "/tkn/sub/path/file.txt" {
		t.Errorf("got %q, want /tkn/sub/path/file.txt", got)
	}
}

func TestExposedPathFromReference_ResourceIdNoColon(t *testing.T) {
	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{StorageId: "tkn"},
		Path:       "file.txt",
	}
	got := exposedPathFromReference(ref)
	if got != "/tkn/file.txt" {
		t.Errorf("got %q, want /tkn/file.txt", got)
	}
}

func TestGetPermissionsFromShare_WebDAV(t *testing.T) {
	perms := &provider.ResourcePermissions{InitiateFileDownload: true, Stat: true}
	share := &ocmv1beta1.Share{
		AccessMethods: []*ocmv1beta1.AccessMethod{
			{
				Term: &ocmv1beta1.AccessMethod_WebdavOptions{
					WebdavOptions: &ocmv1beta1.WebDAVAccessMethod{
						Permissions: perms,
					},
				},
			},
		},
	}
	got := getPermissionsFromShare(share)
	if got != perms {
		t.Error("expected exact permission pointer from WebDAV method")
	}
}

func TestGetPermissionsFromShare_WebAppReadWrite(t *testing.T) {
	share := &ocmv1beta1.Share{
		AccessMethods: []*ocmv1beta1.AccessMethod{
			{
				Term: &ocmv1beta1.AccessMethod_WebappOptions{
					WebappOptions: &ocmv1beta1.WebappAccessMethod{
						ViewMode: providerv1beta1.ViewMode_VIEW_MODE_READ_WRITE,
					},
				},
			},
		},
	}
	got := getPermissionsFromShare(share)
	if got == nil {
		t.Fatal("expected non-nil permissions for read-write webapp")
	}
	if !got.InitiateFileUpload {
		t.Error("editor role should allow InitiateFileUpload")
	}
}

func TestGetPermissionsFromShare_WebAppViewOnly(t *testing.T) {
	share := &ocmv1beta1.Share{
		AccessMethods: []*ocmv1beta1.AccessMethod{
			{
				Term: &ocmv1beta1.AccessMethod_WebappOptions{
					WebappOptions: &ocmv1beta1.WebappAccessMethod{
						ViewMode: providerv1beta1.ViewMode_VIEW_MODE_VIEW_ONLY,
					},
				},
			},
		},
	}
	got := getPermissionsFromShare(share)
	if got == nil {
		t.Fatal("expected non-nil permissions for view-only webapp")
	}
	if got.InitiateFileUpload {
		t.Error("viewer role should not allow InitiateFileUpload")
	}
}

func TestGetPermissionsFromShare_Empty(t *testing.T) {
	share := &ocmv1beta1.Share{AccessMethods: nil}
	got := getPermissionsFromShare(share)
	if got != nil {
		t.Errorf("expected nil permissions for empty methods, got %v", got)
	}
}

func TestFixResourceInfo(t *testing.T) {
	perms := &provider.ResourcePermissions{Stat: true}
	share := &ocmv1beta1.Share{Id: &ocmv1beta1.ShareId{OpaqueId: "share-x"}}
	info := &provider.ResourceInfo{Path: "/home/user/docs/file.txt"}
	shareInfo := &provider.ResourceInfo{Path: "/home/user/docs"}

	fixResourceInfo(info, shareInfo, share, perms)

	want := "/share-x/file.txt"
	if info.Path != want {
		t.Errorf("path: got %q, want %q", info.Path, want)
	}
	if info.PermissionSet != perms {
		t.Error("permission set not applied")
	}
}

func TestFixResourceInfo_RootLevel(t *testing.T) {
	perms := &provider.ResourcePermissions{Stat: true}
	share := &ocmv1beta1.Share{Id: &ocmv1beta1.ShareId{OpaqueId: "share-y"}}
	info := &provider.ResourceInfo{Path: "/data/shared"}
	shareInfo := &provider.ResourceInfo{Path: "/data/shared"}

	fixResourceInfo(info, shareInfo, share, perms)

	if info.Path != "/share-y" {
		t.Errorf("path: got %q, want /share-y", info.Path)
	}
}

func TestGetUploadProtocol(t *testing.T) {
	protocols := []*gateway.FileUploadProtocol{
		{Protocol: "tus", UploadEndpoint: "https://host/tus", Token: "tus-tok"},
		{Protocol: "simple", UploadEndpoint: "https://host/simple", Token: "simple-tok"},
	}

	ep, tok, ok := getUploadProtocol(protocols, "simple")
	if !ok {
		t.Fatal("expected to find simple protocol")
	}
	if ep != "https://host/simple" || tok != "simple-tok" {
		t.Errorf("got ep=%q tok=%q", ep, tok)
	}

	_, _, ok = getUploadProtocol(protocols, "nonexistent")
	if ok {
		t.Error("should not find nonexistent protocol")
	}
}

func TestGetDownloadProtocol(t *testing.T) {
	protocols := []*gateway.FileDownloadProtocol{
		{Protocol: "spaces", DownloadEndpoint: "https://host/spaces", Token: "sp-tok"},
		{Protocol: "simple", DownloadEndpoint: "https://host/simple", Token: "si-tok"},
	}

	ep, tok, ok := getDownloadProtocol(protocols, []string{"simple", "spaces"})
	if !ok {
		t.Fatal("expected to find a matching protocol")
	}
	// spaces comes first in the protocol list, so it should match first
	if ep != "https://host/spaces" || tok != "sp-tok" {
		t.Errorf("got ep=%q tok=%q", ep, tok)
	}

	_, _, ok = getDownloadProtocol(protocols, []string{"nonexistent"})
	if ok {
		t.Error("should not find nonexistent protocol")
	}
}

func TestShareIDFromContextScopes(t *testing.T) {
	share := &ocmv1beta1.Share{Id: &ocmv1beta1.ShareId{OpaqueId: "share-from-scope"}}
	scopes, err := authscope.AddCodeFlowOCMShareScope(share, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatalf("AddCodeFlowOCMShareScope returned error: %v", err)
	}

	ctx := appctx.ContextSetScopes(context.Background(), scopes)
	got := shareIDFromContextScopes(ctx)
	if got != "share-from-scope" {
		t.Fatalf("shareIDFromContextScopes() = %q, want %q", got, "share-from-scope")
	}
}

func TestShareIDFromContextScopesRequiresSingleOCMShare(t *testing.T) {
	shareA := &ocmv1beta1.Share{Id: &ocmv1beta1.ShareId{OpaqueId: "share-a"}}
	shareB := &ocmv1beta1.Share{Id: &ocmv1beta1.ShareId{OpaqueId: "share-b"}}

	scopes, err := authscope.AddCodeFlowOCMShareScope(shareA, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatalf("AddCodeFlowOCMShareScope(shareA) returned error: %v", err)
	}
	scopes, err = authscope.AddCodeFlowOCMShareScope(shareB, authpb.Role_ROLE_VIEWER, scopes)
	if err != nil {
		t.Fatalf("AddCodeFlowOCMShareScope(shareB) returned error: %v", err)
	}

	ctx := appctx.ContextSetScopes(context.Background(), scopes)
	if got := shareIDFromContextScopes(ctx); got != "" {
		t.Fatalf("shareIDFromContextScopes() = %q, want empty string", got)
	}
}

func TestShareFromContextScopesMatchesShareID(t *testing.T) {
	share := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share-1"},
		Token:      "legacy-token",
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Creator:    &userpb.UserId{},
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

	scopes, err := authscope.AddOCMShareScope(share, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatalf("AddOCMShareScope returned error: %v", err)
	}

	ctx := appctx.ContextSetScopes(context.Background(), scopes)
	got := shareFromContextScopes(ctx, "share-1")
	if got == nil {
		t.Fatal("shareFromContextScopes() returned nil")
	}
	if got.GetToken() != "legacy-token" || got.GetCreator() == nil || len(got.GetAccessMethods()) != 1 {
		t.Fatalf("shareFromContextScopes() returned incomplete share: %+v", got)
	}
}

func TestShareFromContextScopesMatchesLegacyToken(t *testing.T) {
	share := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share-2"},
		Token:      "legacy-token",
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Creator:    &userpb.UserId{},
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

	scopes, err := authscope.AddOCMShareScope(share, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatalf("AddOCMShareScope returned error: %v", err)
	}

	ctx := appctx.ContextSetScopes(context.Background(), scopes)
	got := shareFromContextScopes(ctx, "legacy-token")
	if got == nil || got.GetId().GetOpaqueId() != "share-2" {
		t.Fatalf("shareFromContextScopes() = %+v, want share-2", got)
	}
}
