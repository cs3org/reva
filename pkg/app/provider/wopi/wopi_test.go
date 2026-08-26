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
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	authscope "github.com/cs3org/reva/v3/pkg/auth/scope"
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
