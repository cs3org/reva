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

package ocgraph

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/permissions"
)

type labeledItemsBody struct {
	Value []struct {
		Name   string    `json:"name"`
		Labels *[]string `json:"@libre.graph.me.labels"`
	} `json:"value"`
}

func TestSharedWithMeReportsLabels(t *testing.T) {
	labeled := receivedShareInfo()
	other := receivedShareInfo()
	other.ReceivedShare.Share.Id = &collaboration.ShareId{OpaqueId: "share-2"}
	other.ResourceInfo.Id = &provider.ResourceId{StorageId: "storage", OpaqueId: "other", SpaceId: labeled.ResourceInfo.Id.SpaceId}
	other.ResourceInfo.Name = "other.txt"
	other.ResourceInfo.Path = "/users/alice/other.txt"

	for _, tc := range []struct {
		name string
		err  error
		// A nil entry means that the item has no labels field.
		want map[string][]string
	}{
		{name: "labels ok", want: map[string][]string{"readme.txt": {"favorite", "work"}, "other.txt": {}}},
		{name: "labels fail", err: errors.New("labels down"), want: map[string][]string{"readme.txt": nil, "other.txt": nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stampGateway(&labelsGateway{
				receivedSharesGateway: receivedSharesGateway{
					shares: []*gateway.ReceivedShareResourceInfo{labeled, other},
					user:   &userpb.User{Id: &userpb.UserId{OpaqueId: "creator"}},
				},
				// The label row has no space ID, so the match must ignore it.
				labels: map[string][]string{"storage!opaque": {"favorite", "work"}},
				err:    tc.err,
			})
			s := &svc{c: &config{WebBase: "https://example.test/files/spaces"}}

			ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "me"}})
			req := httptest.NewRequest(http.MethodGet, "/me/drive/sharedWithMe", nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			s.getSharedWithMe(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}

			var body labeledItemsBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Value) != len(tc.want) {
				t.Fatalf("got %d items, want %d: %s", len(body.Value), len(tc.want), rec.Body.String())
			}
			for _, item := range body.Value {
				want := tc.want[item.Name]
				if want == nil {
					if item.Labels != nil {
						t.Errorf("item %s: labels = %v, want no labels field", item.Name, *item.Labels)
					}
					continue
				}
				if item.Labels == nil {
					t.Fatalf("item %s: no labels field in %s", item.Name, rec.Body.String())
				}
				if !slices.Equal(*item.Labels, want) {
					t.Errorf("item %s: labels = %v, want %v", item.Name, *item.Labels, want)
				}
			}
		})
	}
}

func TestSharedByMeReportsLabels(t *testing.T) {
	resource := func(name, inode string) *provider.ResourceInfo {
		info := receivedShareInfo().ResourceInfo
		info.Id.OpaqueId = inode
		info.ParentId = &provider.ResourceId{StorageId: "storage", SpaceId: info.Id.SpaceId, OpaqueId: "parent"}
		info.Name = name
		info.Path = "/users/alice/" + name
		return info
	}
	shared := resource("shared.txt", "100")
	linked := resource("linked.txt", "200")
	created := &types.Timestamp{Seconds: 1700000000}
	viewer := permissions.NewViewerRole().CS3ResourcePermissions()

	stampGateway(&labelsGateway{
		receivedSharesGateway: receivedSharesGateway{user: &userpb.User{Id: &userpb.UserId{OpaqueId: "me"}}},
		// "Shared with others" and "Shared via link" read the same endpoint, so the test has one share of each kind.
		sentShares: []*gateway.ShareResourceInfo{{
			Share: &collaboration.Share{
				Id:         &collaboration.ShareId{OpaqueId: "share-1"},
				ResourceId: shared.Id,
				Creator:    &userpb.UserId{OpaqueId: "me"},
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: "grantee"}},
				},
				Permissions: &collaboration.SharePermissions{Permissions: viewer},
				Ctime:       created,
			},
			ResourceInfo: shared,
		}},
		publicShares: []*gateway.PublicShareResourceInfo{{
			PublicShare: &link.PublicShare{
				Id:          &link.PublicShareId{OpaqueId: "link-1"},
				ResourceId:  linked.Id,
				Token:       "token",
				Permissions: &link.PublicSharePermissions{Permissions: viewer},
				Ctime:       created,
			},
			ResourceInfo: linked,
		}},
		labels: map[string][]string{"storage!100": {"favorite"}},
	})
	s := &svc{c: &config{}}

	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "me"}})
	req := httptest.NewRequest(http.MethodGet, "/me/drive/sharedByMe", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.getSharedByMe(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	var body labeledItemsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"shared.txt": {"favorite"}, "linked.txt": {}}
	if len(body.Value) != len(want) {
		t.Fatalf("got %d items, want %d: %s", len(body.Value), len(want), rec.Body.String())
	}
	for _, item := range body.Value {
		if item.Labels == nil {
			t.Fatalf("item %s: no labels field in %s", item.Name, rec.Body.String())
		}
		if !slices.Equal(*item.Labels, want[item.Name]) {
			t.Errorf("item %s: labels = %v, want %v", item.Name, *item.Labels, want[item.Name])
		}
	}
}
