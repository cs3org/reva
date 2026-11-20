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

package wopi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	authscope "github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
)

type mockGateway struct {
	gateway.GatewayAPIClient
	statResp *provider.StatResponse
	statErr  error
}

func (m *mockGateway) Stat(_ context.Context, _ *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	return m.statResp, m.statErr
}

func TestGetPathForExternalLinkPrefersOCMShareID(t *testing.T) {
	stampGateway(&mockGateway{
		statResp: &provider.StatResponse{
			Info: &provider.ResourceInfo{
				Path: "/home/user/docs/file.txt",
				Id:   &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
			},
		},
	})

	share := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share-123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "legacy-token",
	}
	scopes, err := authscope.AddOCMShareScope(share, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatalf("AddOCMShareScope returned error: %v", err)
	}

	ctx := appctx.ContextSetScopes(context.Background(), scopes)
	resource := &provider.ResourceInfo{
		Path: "/home/user/docs/file.txt",
		Id:   &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
	}

	got, err := getPathForExternalLink(ctx, scopes, resource, ocmLinkURLPrefix)
	if err != nil {
		t.Fatalf("getPathForExternalLink returned error: %v", err)
	}
	if got != ocmLinkURLPrefix+"share-123" {
		t.Fatalf("getPathForExternalLink() = %q, want %q", got, ocmLinkURLPrefix+"share-123")
	}
}

func TestGetPathForExternalLinkFallsBackToToken(t *testing.T) {
	stampGateway(&mockGateway{
		statResp: &provider.StatResponse{
			Info: &provider.ResourceInfo{
				Path: "/home/user/docs/file.txt",
				Id:   &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
			},
		},
	})

	share := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "legacy-token",
	}
	scopes, err := authscope.AddOCMShareScope(share, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatalf("AddOCMShareScope returned error: %v", err)
	}

	ctx := appctx.ContextSetScopes(context.Background(), scopes)
	resource := &provider.ResourceInfo{
		Path: "/home/user/docs/file.txt",
		Id:   &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
	}

	got, err := getPathForExternalLink(ctx, scopes, resource, ocmLinkURLPrefix)
	if err != nil {
		t.Fatalf("getPathForExternalLink returned error: %v", err)
	}
	if got != ocmLinkURLPrefix+"legacy-token" {
		t.Fatalf("getPathForExternalLink() = %q, want %q", got, ocmLinkURLPrefix+"legacy-token")
	}
}

func TestGetAppURLForSingleFileOCMShare(t *testing.T) {
	var query url.Values
	wopiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_, _ = w.Write([]byte(`{"app-url":"https://app.example/open"}`))
	}))
	defer wopiServer.Close()

	stampGateway(&mockGateway{statResp: &provider.StatResponse{Info: &provider.ResourceInfo{
		Path: "/ocm/share-123",
		Id:   &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
	}}})
	share := &ocmv1beta1.Share{
		Id:         &ocmv1beta1.ShareId{OpaqueId: "share-123"},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
	}
	scopes, err := authscope.AddCodeFlowOCMShareScope(share, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	user := &userpb.User{
		Id:     &userpb.UserId{OpaqueId: "remote", Idp: "remote.example"},
		Opaque: &typespb.Opaque{Map: map[string]*typespb.OpaqueEntry{"ocm-share-role": {Value: []byte("viewer")}}},
	}
	ctx := appctx.ContextSetUser(context.Background(), user)
	ctx = appctx.ContextSetScopes(ctx, scopes)
	tkn, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	ctx = appctx.ContextSetToken(ctx, tkn)

	p := &wopiProvider{
		conf:       &config{WopiURL: wopiServer.URL, FolderBaseURL: "https://box.example", JWTSecret: "secret"},
		wopiClient: httpclient.New(),
		appURLs:    map[string]map[string]string{"view": {".md": "https://app.example/view-md"}},
	}
	// A single-file share has no extension in its path, so it comes from the media type.
	resource := &provider.ResourceInfo{
		Path:     "/ocm/share-123",
		MimeType: "text/markdown",
		Id:       &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Owner:    &userpb.UserId{OpaqueId: "owner", Idp: "box.example"},
	}
	if _, _, _, err := p.GetAppURL(ctx, resource, appprovider.ViewMode_VIEW_MODE_READ_ONLY, "token"); err != nil {
		t.Fatal(err)
	}
	if got := query.Get("appurl"); got != "https://app.example/view-md" {
		t.Fatalf("appurl = %q", got)
	}
	if got := query.Get("usertype"); got != "ocm" {
		t.Fatalf("usertype = %q", got)
	}
}
