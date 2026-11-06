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

package ocgraph

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	labels "github.com/cs3org/go-cs3apis/cs3/labels/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"google.golang.org/grpc"

	"github.com/cs3org/reva/v3/pkg/appctx"
)

type favoritesGateway struct {
	receivedSharesGateway
	spaces    []*provider.StorageSpace
	favorites []*provider.ResourceId
	err       error
}

func (g *favoritesGateway) ListStorageSpaces(
	_ context.Context,
	_ *provider.ListStorageSpacesRequest,
	_ ...grpc.CallOption,
) (*provider.ListStorageSpacesResponse, error) {
	return &provider.ListStorageSpacesResponse{
		Status:        &rpc.Status{Code: rpc.Code_CODE_OK},
		StorageSpaces: g.spaces,
	}, nil
}

func (g *favoritesGateway) ListResourcesForLabel(
	_ context.Context,
	req *labels.ListResourcesForLabelRequest,
	_ ...grpc.CallOption,
) (*labels.ListResourcesForLabelResponse, error) {
	if g.err != nil {
		return nil, g.err
	}
	refs := []*provider.Reference{}
	if req.Labels == "favorite" {
		for _, id := range g.favorites {
			refs = append(refs, &provider.Reference{ResourceId: id})
		}
	}
	return &labels.ListResourcesForLabelResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Ref:    refs,
	}, nil
}

func TestListMountpointsReportsFavorite(t *testing.T) {
	favorite := receivedShareInfo()
	other := receivedShareInfo()
	other.ReceivedShare.Share.Id = &collaboration.ShareId{OpaqueId: "share-2"}
	other.ResourceInfo.Id = &provider.ResourceId{StorageId: "storage", OpaqueId: "other", SpaceId: favorite.ResourceInfo.Id.SpaceId}

	for _, tc := range []struct {
		name string
		err  error
		want map[string]bool
	}{
		{name: "labels ok", want: map[string]bool{"share-1": true, "share-2": false}},
		{name: "labels fail", err: errors.New("labels down"), want: map[string]bool{"share-1": false, "share-2": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stampGateway(&favoritesGateway{
				receivedSharesGateway: receivedSharesGateway{
					shares: []*gateway.ReceivedShareResourceInfo{favorite, other},
				},
				// The label row has no space ID, so the match must ignore it.
				favorites: []*provider.ResourceId{{StorageId: "storage", OpaqueId: "opaque"}},
				err:       tc.err,
			})
			s := &svc{c: &config{WebBase: "https://example.test/files/spaces"}}

			ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "me"}})
			query := url.Values{"$filter": {"driveType eq 'mountpoint'"}}
			req := httptest.NewRequest(http.MethodGet, "/me/drives?"+query.Encode(), nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			s.listMySpaces(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}

			var body struct {
				Value []struct {
					DriveAlias string `json:"driveAlias"`
					DriveType  string `json:"driveType"`
					Favorite   *bool  `json:"favorite"`
				} `json:"value"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Value) != len(tc.want) {
				t.Fatalf("got %d drives, want %d: %s", len(body.Value), len(tc.want), rec.Body.String())
			}
			for _, d := range body.Value {
				if d.DriveType != "mountpoint" {
					t.Errorf("drive %s: driveType = %q, want mountpoint", d.DriveAlias, d.DriveType)
				}
				if d.Favorite == nil {
					t.Fatalf("drive %s: no favorite key in %s", d.DriveAlias, rec.Body.String())
				}
				if *d.Favorite != tc.want[d.DriveAlias] {
					t.Errorf("drive %s: favorite = %v, want %v", d.DriveAlias, *d.Favorite, tc.want[d.DriveAlias])
				}
			}
		})
	}
}

func TestListProjectsReportsFavorite(t *testing.T) {
	project := func(name, inode string) *provider.StorageSpace {
		return &provider.StorageSpace{
			Id:        &provider.StorageSpaceId{OpaqueId: "eos$" + name},
			Name:      name,
			SpaceType: "project",
			Root:      &provider.ResourceId{StorageId: "eos", SpaceId: name, OpaqueId: inode},
			RootInfo: &provider.ResourceInfo{
				Path:          "/eos/project/" + name,
				PermissionSet: &provider.ResourcePermissions{Stat: true},
			},
		}
	}
	stampGateway(&favoritesGateway{
		spaces:    []*provider.StorageSpace{project("fav", "100"), project("other", "200")},
		favorites: []*provider.ResourceId{{StorageId: "eos", OpaqueId: "100"}},
	})
	s := &svc{c: &config{}}

	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "me"}})
	query := url.Values{"$filter": {"driveType eq 'project'"}}
	req := httptest.NewRequest(http.MethodGet, "/me/drives?"+query.Encode(), nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.listMySpaces(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Value []struct {
			Name     string `json:"name"`
			Favorite bool   `json:"favorite"`
		} `json:"value"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"fav": true, "other": false}
	if len(body.Value) != len(want) {
		t.Fatalf("got %d drives, want %d: %s", len(body.Value), len(want), rec.Body.String())
	}
	for _, d := range body.Value {
		if d.Favorite != want[d.Name] {
			t.Errorf("drive %s: favorite = %v, want %v", d.Name, d.Favorite, want[d.Name])
		}
	}
}
