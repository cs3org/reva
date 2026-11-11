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
	"slices"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	labelspb "github.com/cs3org/go-cs3apis/cs3/labels/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"google.golang.org/grpc"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/labels"
)

type labelsGateway struct {
	receivedSharesGateway
	spaces []*provider.StorageSpace
	// labels is keyed on <storage_id>!<opaque_id>.
	labels map[string][]string
	err    error
}

func (g *labelsGateway) ListStorageSpaces(
	_ context.Context,
	_ *provider.ListStorageSpacesRequest,
	_ ...grpc.CallOption,
) (*provider.ListStorageSpacesResponse, error) {
	return &provider.ListStorageSpacesResponse{
		Status:        &rpc.Status{Code: rpc.Code_CODE_OK},
		StorageSpaces: g.spaces,
	}, nil
}

func (g *labelsGateway) ListLabels(
	_ context.Context,
	req *labelspb.ListLabelsRequest,
	_ ...grpc.CallOption,
) (*labelspb.ListLabelsResponse, error) {
	if g.err != nil {
		return nil, g.err
	}
	var keys []string
	if err := json.Unmarshal(req.GetOpaque().GetMap()[labels.OpaqueResources].GetValue(), &keys); err != nil {
		return nil, err
	}
	requested := map[string][]string{}
	for _, key := range keys {
		if l, ok := g.labels[key]; ok {
			requested[key] = l
		}
	}
	value, err := json.Marshal(requested)
	if err != nil {
		return nil, err
	}
	return &labelspb.ListLabelsResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Opaque: &types.Opaque{Map: map[string]*types.OpaqueEntry{
			labels.OpaqueLabels: {Decoder: "json", Value: value},
		}},
	}, nil
}

type labeledDrivesBody struct {
	Value []struct {
		Name       string `json:"name"`
		DriveAlias string `json:"driveAlias"`
		DriveType  string `json:"driveType"`
		Root       struct {
			Labels *[]string `json:"@libre.graph.me.labels"`
		} `json:"root"`
	} `json:"value"`
}

func TestListMountpointsReportsLabels(t *testing.T) {
	labeled := receivedShareInfo()
	other := receivedShareInfo()
	other.ReceivedShare.Share.Id = &collaboration.ShareId{OpaqueId: "share-2"}
	other.ResourceInfo.Id = &provider.ResourceId{StorageId: "storage", OpaqueId: "other", SpaceId: labeled.ResourceInfo.Id.SpaceId}

	for _, tc := range []struct {
		name string
		err  error
		// A nil entry means that the drive root has no labels field.
		want map[string][]string
	}{
		{name: "labels ok", want: map[string][]string{"share-1": {"favorite", "work"}, "share-2": {}}},
		{name: "labels fail", err: errors.New("labels down"), want: map[string][]string{"share-1": nil, "share-2": nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stampGateway(&labelsGateway{
				receivedSharesGateway: receivedSharesGateway{
					shares: []*gateway.ReceivedShareResourceInfo{labeled, other},
				},
				// The label row has no space ID, so the match must ignore it.
				labels: map[string][]string{"storage!opaque": {"favorite", "work"}},
				err:    tc.err,
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

			var body labeledDrivesBody
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
				want := tc.want[d.DriveAlias]
				if want == nil {
					if d.Root.Labels != nil {
						t.Errorf("drive %s: labels = %v, want no labels field", d.DriveAlias, *d.Root.Labels)
					}
					continue
				}
				if d.Root.Labels == nil {
					t.Fatalf("drive %s: no labels field in %s", d.DriveAlias, rec.Body.String())
				}
				if !slices.Equal(*d.Root.Labels, want) {
					t.Errorf("drive %s: labels = %v, want %v", d.DriveAlias, *d.Root.Labels, want)
				}
			}
		})
	}
}

func TestListProjectsReportsLabels(t *testing.T) {
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
	stampGateway(&labelsGateway{
		spaces: []*provider.StorageSpace{project("fav", "100"), project("other", "200")},
		labels: map[string][]string{"eos!100": {"favorite"}},
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

	var body labeledDrivesBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"fav": {"favorite"}, "other": {}}
	if len(body.Value) != len(want) {
		t.Fatalf("got %d drives, want %d: %s", len(body.Value), len(want), rec.Body.String())
	}
	for _, d := range body.Value {
		if d.Root.Labels == nil {
			t.Fatalf("drive %s: no labels field in %s", d.Name, rec.Body.String())
		}
		if !slices.Equal(*d.Root.Labels, want[d.Name]) {
			t.Errorf("drive %s: labels = %v, want %v", d.Name, *d.Root.Labels, want[d.Name])
		}
	}
}
