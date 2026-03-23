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

package ocm

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/studio-b12/gowebdav"
)

func TestShareInfoFromPath(t *testing.T) {
	id, rel := shareInfoFromPath("/share123/sub/file.txt")
	if id.OpaqueId != "share123" {
		t.Errorf("shareID: got %q, want share123", id.OpaqueId)
	}
	if rel != "/sub/file.txt" {
		t.Errorf("rel: got %q, want /sub/file.txt", rel)
	}
}

func TestShareInfoFromPath_RootOnly(t *testing.T) {
	id, rel := shareInfoFromPath("/share-only")
	if id.OpaqueId != "share-only" {
		t.Errorf("shareID: got %q, want share-only", id.OpaqueId)
	}
	if rel != "/" {
		t.Errorf("rel: got %q, want /", rel)
	}
}

func TestShareInfoFromReference_PathBased(t *testing.T) {
	ref := &provider.Reference{Path: "/share-abc/docs/readme.md"}
	id, rel := shareInfoFromReference(ref)
	if id.OpaqueId != "share-abc" {
		t.Errorf("shareID: got %q, want share-abc", id.OpaqueId)
	}
	if rel != "/docs/readme.md" {
		t.Errorf("rel: got %q, want /docs/readme.md", rel)
	}
}

func TestShareInfoFromReference_ResourceIdWithColon(t *testing.T) {
	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{OpaqueId: "share-abc:sub"},
		Path:       "file.txt",
	}
	id, rel := shareInfoFromReference(ref)
	if id.OpaqueId != "share-abc" {
		t.Errorf("shareID: got %q, want share-abc", id.OpaqueId)
	}
	if rel != "sub/file.txt" {
		t.Errorf("rel: got %q, want sub/file.txt", rel)
	}
}

func TestShareInfoFromReference_ResourceIdNoColon(t *testing.T) {
	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{OpaqueId: "share-abc"},
		Path:       "file.txt",
	}
	id, rel := shareInfoFromReference(ref)
	if id.OpaqueId != "share-abc" {
		t.Errorf("shareID: got %q, want share-abc", id.OpaqueId)
	}
	if rel != "file.txt" {
		t.Errorf("rel: got %q, want file.txt", rel)
	}
}

func TestGetWebDAVProtocol_Found(t *testing.T) {
	webdav := &ocmpb.WebDAVProtocol{Uri: "https://remote/dav", SharedSecret: "s3cret"}
	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebdavOptions{WebdavOptions: webdav}},
	}
	got, ok := getWebDAVProtocol(protocols)
	if !ok {
		t.Fatal("expected to find WebDAV protocol")
	}
	if got.Uri != "https://remote/dav" {
		t.Errorf("uri: got %q, want https://remote/dav", got.Uri)
	}
}

func TestGetWebDAVProtocol_NotFound(t *testing.T) {
	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebappOptions{WebappOptions: &ocmpb.WebappProtocol{}}},
	}
	_, ok := getWebDAVProtocol(protocols)
	if ok {
		t.Error("expected not to find WebDAV protocol")
	}
}

func TestRequiresExchange_True(t *testing.T) {
	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebdavOptions{WebdavOptions: &ocmpb.WebDAVProtocol{
			Permissions: &ocmpb.SharePermissions{
				Permissions: &provider.ResourcePermissions{Stat: true},
			},
			Requirements: []string{"must-exchange-token"},
		}}},
	}
	if !requiresExchange(protocols) {
		t.Error("expected requiresExchange=true")
	}
}

func TestRequiresExchange_False(t *testing.T) {
	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebdavOptions{WebdavOptions: &ocmpb.WebDAVProtocol{
			Permissions: &ocmpb.SharePermissions{
				Permissions: &provider.ResourcePermissions{Stat: true},
			},
		}}},
	}
	if requiresExchange(protocols) {
		t.Error("expected requiresExchange=false when no requirements")
	}
}

func TestRequiresExchange_NoWebDAV(t *testing.T) {
	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebappOptions{WebappOptions: &ocmpb.WebappProtocol{}}},
	}
	if requiresExchange(protocols) {
		t.Error("expected requiresExchange=false when no WebDAV protocol")
	}
}

func TestGetResourceInfo(t *testing.T) {
	id := &ocmpb.ShareId{OpaqueId: "share-abc"}
	got := getResourceInfo(id, "sub/file.txt")
	want := "share-abc:sub/file.txt"
	if got.OpaqueId != want {
		t.Errorf("OpaqueId: got %q, want %q", got.OpaqueId, want)
	}
}

func TestGetPathFromShareIDAndRelPath(t *testing.T) {
	id := &ocmpb.ShareId{OpaqueId: "share-abc"}
	got := getPathFromShareIDAndRelPath(id, "sub/file.txt")
	if got != "/share-abc/sub/file.txt" {
		t.Errorf("got %q, want /share-abc/sub/file.txt", got)
	}
}

func TestGetPathFromShareIDAndRelPath_Root(t *testing.T) {
	id := &ocmpb.ShareId{OpaqueId: "share-abc"}
	got := getPathFromShareIDAndRelPath(id, "")
	if got != "/share-abc" {
		t.Errorf("got %q, want /share-abc", got)
	}
}

type fakeFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (f *fakeFileInfo) Name() string      { return f.name }
func (f *fakeFileInfo) Size() int64       { return f.size }
func (f *fakeFileInfo) Mode() fs.FileMode { return f.mode }
func (f *fakeFileInfo) ModTime() time.Time {
	if f.modTime.IsZero() {
		return time.Unix(1700000000, 0)
	}
	return f.modTime
}
func (f *fakeFileInfo) IsDir() bool { return f.isDir }
func (f *fakeFileInfo) Sys() any    { return nil }

func testReceivedShare(id string, isFile bool) *ocmpb.ReceivedShare {
	srt := ocmpb.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER
	if isFile {
		srt = ocmpb.SharedResourceType_SHARE_RESOURCE_TYPE_FILE
	}
	return &ocmpb.ReceivedShare{
		Id:   &ocmpb.ShareId{OpaqueId: id},
		Name: "shared-doc.txt",
		Creator: &userpb.UserId{
			OpaqueId: "creator",
			Idp:      "sender.example.com",
		},
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: &userpb.UserId{
					OpaqueId: "receiver",
					Idp:      "nextcloud1.docker",
				},
			},
		},
		SharedResourceType: srt,
		Protocols: []*ocmpb.Protocol{
			{Term: &ocmpb.Protocol_WebdavOptions{WebdavOptions: &ocmpb.WebDAVProtocol{
				Uri:          "https://remote/dav",
				SharedSecret: "secret",
				Permissions: &ocmpb.SharePermissions{
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
						InitiateFileUpload:   true,
					},
				},
			}}},
		},
	}
}

func TestReceiverClientIDPrefersContextUserIDP(t *testing.T) {
	share := testReceivedShare("share-abc", false)
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "local-user", Idp: "local-context.example"},
	})

	got := receiverClientID(ctx, share)
	if got != "local-context.example" {
		t.Errorf("got %q, want local-context.example", got)
	}
}

func TestReceiverClientIDFallsBackToShareGranteeIDP(t *testing.T) {
	share := testReceivedShare("share-abc", false)

	got := receiverClientID(context.Background(), share)
	if got != "nextcloud1.docker" {
		t.Errorf("got %q, want nextcloud1.docker", got)
	}
}

func TestReceiverClientIDReturnsEmptyWhenUnavailable(t *testing.T) {
	share := testReceivedShare("share-abc", false)
	share.Grantee = nil

	got := receiverClientID(context.Background(), share)
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestReceiverClientIDWithLookupFallsBackToGatewayUserIDP(t *testing.T) {
	share := testReceivedShare("share-abc", false)
	share.Grantee.GetUserId().Idp = ""

	got := receiverClientIDWithLookup(context.Background(), share, func(_ context.Context, userID *userpb.UserId) string {
		if userID.GetOpaqueId() != "receiver" {
			t.Fatalf("lookup user id: got %q, want receiver", userID.GetOpaqueId())
		}
		return "local-gateway.example"
	})
	if got != "local-gateway.example" {
		t.Errorf("got %q, want local-gateway.example", got)
	}
}

func TestReceiverClientIDWithLookupSkipsGatewayWhenShareAlreadyHasIDP(t *testing.T) {
	share := testReceivedShare("share-abc", false)
	lookupCalled := false

	got := receiverClientIDWithLookup(context.Background(), share, func(_ context.Context, _ *userpb.UserId) string {
		lookupCalled = true
		return "unexpected.example"
	})
	if got != "nextcloud1.docker" {
		t.Errorf("got %q, want nextcloud1.docker", got)
	}
	if lookupCalled {
		t.Error("expected lookup not to be called when share grantee already has an idp")
	}
}

func TestConvertStatToResourceInfo_File(t *testing.T) {
	fi := &fakeFileInfo{name: "file.txt", size: 1024}
	share := testReceivedShare("share-abc", true)

	info := convertStatToResourceInfo(fi, share, "sub/file.txt")

	if info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		t.Errorf("type: got %v, want FILE", info.Type)
	}
	// for file shares, the name comes from share.Name
	if info.Name != "shared-doc.txt" {
		t.Errorf("name: got %q, want shared-doc.txt", info.Name)
	}
	if info.Size != 1024 {
		t.Errorf("size: got %d, want 1024", info.Size)
	}
	if info.Path != "/share-abc/sub/file.txt" {
		t.Errorf("path: got %q, want /share-abc/sub/file.txt", info.Path)
	}
	if info.Id.OpaqueId != "share-abc:sub/file.txt" {
		t.Errorf("id: got %q, want share-abc:sub/file.txt", info.Id.OpaqueId)
	}
	if info.Owner.OpaqueId != "creator" {
		t.Errorf("owner: got %q, want creator", info.Owner.OpaqueId)
	}
	if !info.PermissionSet.InitiateFileDownload {
		t.Error("expected InitiateFileDownload permission")
	}
}

func TestConvertStatToResourceInfo_Dir(t *testing.T) {
	fi := &fakeFileInfo{name: "docs", size: 0, isDir: true}
	share := testReceivedShare("share-abc", false)

	info := convertStatToResourceInfo(fi, share, "docs")

	if info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		t.Errorf("type: got %v, want CONTAINER", info.Type)
	}
	// for folder shares, the name comes from FileInfo.Name()
	if info.Name != "docs" {
		t.Errorf("name: got %q, want docs", info.Name)
	}
}

func TestIsWebDAV401_True(t *testing.T) {
	err := gowebdav.NewPathError("GET", "/test", http.StatusUnauthorized)
	if !isWebDAV401(err) {
		t.Error("expected isWebDAV401=true for 401 PathError")
	}
}

func TestIsWebDAV401_OtherStatus(t *testing.T) {
	err := gowebdav.NewPathError("GET", "/test", http.StatusForbidden)
	if isWebDAV401(err) {
		t.Error("expected isWebDAV401=false for 403 PathError")
	}
}

func TestIsWebDAV401_PlainError(t *testing.T) {
	err := fmt.Errorf("some error")
	if isWebDAV401(err) {
		t.Error("expected isWebDAV401=false for plain error")
	}
}

func TestIsWebDAV401_OsPathError(t *testing.T) {
	err := &os.PathError{Op: "GET", Path: "/test", Err: fmt.Errorf("not a StatusError")}
	if isWebDAV401(err) {
		t.Error("expected isWebDAV401=false for PathError with non-StatusError inner")
	}
}
