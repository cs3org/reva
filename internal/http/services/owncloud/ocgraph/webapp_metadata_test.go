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
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"google.golang.org/grpc"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/permissions"
)

func TestReceivedShareDriveItemAbsenceMatchesDriveItem(t *testing.T) {
	item := webdavOnlyDriveItem()
	multi := webdavOnlyDriveItem()
	multi.RemoteItem.Permissions = append(multi.RemoteItem.Permissions, secondGrant())

	tests := []struct {
		name string
		item *libregraph.DriveItem
		meta *receivedWebappMetadata
	}{
		{
			name: "nil metadata",
			item: item,
			meta: nil,
		},
		{
			name: "present false",
			item: item,
			meta: &receivedWebappMetadata{Present: false, AppName: "ignored"},
		},
		{
			name: "multiple permissions",
			item: multi,
			meta: nil,
		},
		{
			name: "omitted optional drive fields",
			item: sparseOptionalDriveItem(),
			meta: &receivedWebappMetadata{Present: false, AppName: "dropped"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := mustJSON(t, tt.item)
			got := mustJSON(t, newReceivedShareDriveItem(tt.item, tt.meta))
			if !bytes.Equal(got, want) {
				t.Fatalf("wrapper bytes differ\n got %s\nwant %s", got, want)
			}

			wantEnv := mustJSON(t, map[string]any{"value": []*libregraph.DriveItem{tt.item}})
			gotEnv := mustJSON(t, map[string]any{
				"value": []any{newReceivedShareDriveItem(tt.item, tt.meta)},
			})
			if !bytes.Equal(gotEnv, wantEnv) {
				t.Fatalf("envelope bytes differ\n got %s\nwant %s", gotEnv, wantEnv)
			}
		})
	}

	t.Run("nil item without metadata", func(t *testing.T) {
		want := mustJSON(t, (*libregraph.DriveItem)(nil))
		for _, meta := range []*receivedWebappMetadata{nil, {Present: false, AppName: "x"}} {
			got, err := json.Marshal(newReceivedShareDriveItem(nil, meta))
			if err != nil {
				t.Fatalf("marshal nil item: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("nil item bytes = %s, want %s", got, want)
			}
		}
	})

	t.Run("empty collection", func(t *testing.T) {
		want := mustJSON(t, map[string]any{"value": []*libregraph.DriveItem{}})
		got := mustJSON(t, map[string]any{"value": []any{}})
		if !bytes.Equal(got, want) {
			t.Fatalf("empty collection = %s, want %s", got, want)
		}
	})
}

func TestReceivedShareDriveItemAddsWebappSiblingOnly(t *testing.T) {
	names := []string{
		`say "hi" / path\ok`,
		"<b>app</b>",
		"caf\u00e9 \u4e2d",
		"",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			item := webdavOnlyDriveItem()
			meta := &receivedWebappMetadata{Present: true, AppName: name}
			base := mustJSON(t, item)
			got := mustJSON(t, newReceivedShareDriveItem(item, meta))

			assertWebappSibling(t, base, got, name)
			var decoded struct {
				RemoteItem struct {
					Permissions []json.RawMessage `json:"permissions"`
				} `json:"remoteItem"`
			}
			if err := json.Unmarshal(got, &decoded); err != nil {
				t.Fatal(err)
			}
			if len(decoded.RemoteItem.Permissions) != 1 {
				t.Fatalf("permissions length = %d, want 1", len(decoded.RemoteItem.Permissions))
			}
		})
	}

	t.Run("additional grant preserved", func(t *testing.T) {
		item := webdavOnlyDriveItem()
		item.RemoteItem.Permissions = append(item.RemoteItem.Permissions, secondGrant())
		base := mustJSON(t, item)
		got := mustJSON(t, newReceivedShareDriveItem(item, &receivedWebappMetadata{
			Present: true,
			AppName: "text",
		}))
		assertWebappSibling(t, base, got, "text")

		var decoded struct {
			RemoteItem struct {
				Permissions []json.RawMessage `json:"permissions"`
			} `json:"remoteItem"`
		}
		if err := json.Unmarshal(got, &decoded); err != nil {
			t.Fatal(err)
		}
		if len(decoded.RemoteItem.Permissions) != 2 {
			t.Fatalf("permissions length = %d, want 2", len(decoded.RemoteItem.Permissions))
		}
	})
}

func TestReceivedShareDriveItemMetadataErrors(t *testing.T) {
	t.Run("nil item", func(t *testing.T) {
		got, err := newReceivedShareDriveItem(nil, &receivedWebappMetadata{
			Present: true,
			AppName: "text",
		}).MarshalJSON()
		if err == nil {
			t.Fatal("expected error")
		}
		if got != nil {
			t.Fatalf("partial bytes %s", got)
		}
	})

	t.Run("missing remoteItem", func(t *testing.T) {
		item := webdavOnlyDriveItem()
		item.RemoteItem = nil
		got, err := newReceivedShareDriveItem(item, &receivedWebappMetadata{
			Present: true,
			AppName: "text",
		}).MarshalJSON()
		if err == nil {
			t.Fatal("expected error")
		}
		if got != nil {
			t.Fatalf("partial bytes %s", got)
		}
	})

	t.Run("base marshal failure", func(t *testing.T) {
		item := webdavOnlyDriveItem()
		item.Root = map[string]any{"bad": failJSON{}}
		got, err := newReceivedShareDriveItem(item, &receivedWebappMetadata{
			Present: true,
			AppName: "text",
		}).MarshalJSON()
		if err == nil {
			t.Fatal("expected error")
		}
		if len(got) != 0 {
			t.Fatalf("partial bytes %s", got)
		}
	})

	t.Run("extension encode failure", func(t *testing.T) {
		// remoteItem is an object, but a field value is not JSON, so the
		// extension cannot be written back.
		got, err := extendReceivedShareRemoteItem(
			[]byte(`{"id":"item-1","remoteItem":{"id":"remote-1","bad":oops}}`),
			"text",
		)
		if err == nil {
			t.Fatal("expected error")
		}
		if len(got) != 0 {
			t.Fatalf("partial bytes %s", got)
		}
	})

	cases := []struct {
		name string
		raw  string
	}{
		{"non-object remoteItem", `{"remoteItem":[]}`},
		{"null remoteItem", `{"remoteItem":null}`},
		{"string remoteItem", `{"remoteItem":"file"}`},
		{"malformed", `{`},
		{"root array", `[]`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extendReceivedShareRemoteItem([]byte(tt.raw), "text")
			if err == nil {
				t.Fatal("expected error")
			}
			if got != nil {
				t.Fatalf("partial bytes %s", got)
			}
		})
	}
}

func TestGetSharedWithMeEncodesReceivedWebappMetadata(t *testing.T) {
	rsi := receivedShareInfo()
	endpoint := "ocgraph-webapp-" + t.Name()
	gw := &receivedSharesGateway{
		shares: []*gateway.ReceivedShareResourceInfo{rsi},
		user: &userpb.User{
			Id:          &userpb.UserId{OpaqueId: "creator"},
			DisplayName: "Creator",
		},
	}
	stampGateway(gw)
	s := &svc{c: &config{
		GatewaySvc: endpoint,
		WebBase:    "https://example.test/files/spaces",
		OCMEnabled: false,
	}}

	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "me"},
	})
	req := httptest.NewRequest(http.MethodGet, "/me/drive/sharedWithMe", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.getSharedWithMe(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	item, err := s.cs3ReceivedShareToDriveItem(ctx, rsi)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := encodeSharedWithMe(&want, []any{item}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rec.Body.Bytes(), want.Bytes()) {
		t.Fatalf("handler bytes differ\n got %s\nwant %s", rec.Body.Bytes(), want.Bytes())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(receivedWebappJSONKey)) {
		t.Fatalf("nil metadata leaked into handler body %s", rec.Body.Bytes())
	}

	meta := &receivedWebappMetadata{Present: true, AppName: "caf\u00e9 \"<ok>\""}
	var got bytes.Buffer
	if err := encodeSharedWithMe(&got, []any{newReceivedShareDriveItem(item, meta)}); err != nil {
		t.Fatal(err)
	}
	encoded := decodeSharedWithMeItem(t, got.Bytes())
	assertWebappSibling(t, mustJSON(t, item), encoded, meta.AppName)
}

func TestGetSharedWithMeOCMEnabledOmitsNilWebapp(t *testing.T) {
	rsi := receivedShareInfo()
	ocmShare := receivedOCMShareInfo()
	gw := &receivedSharesGateway{
		shares:    []*gateway.ReceivedShareResourceInfo{rsi},
		ocmShares: []*ocm.ReceivedShare{ocmShare},
		user: &userpb.User{
			Id:          &userpb.UserId{OpaqueId: "creator"},
			DisplayName: "Creator",
		},
	}
	stampGateway(gw)
	s := &svc{c: &config{
		GatewaySvc: "ocgraph-webapp-ocm-" + t.Name(),
		WebBase:    "https://example.test/files/spaces",
		OCMEnabled: true,
	}}
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "me"},
	})
	req := httptest.NewRequest(http.MethodGet, "/me/drive/sharedWithMe", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.getSharedWithMe(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(receivedWebappJSONKey)) {
		t.Fatalf("nil metadata leaked into handler body %s", rec.Body.Bytes())
	}

	local, err := s.cs3ReceivedShareToDriveItem(ctx, rsi)
	if err != nil {
		t.Fatal(err)
	}
	received, err := s.OCMReceivedShareToDriveItem(ctx, ocmShare)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := encodeSharedWithMe(&want, []any{
		local,
		newReceivedShareDriveItem(received, nil),
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rec.Body.Bytes(), want.Bytes()) {
		t.Fatalf("handler bytes differ\n got %s\nwant %s", rec.Body.Bytes(), want.Bytes())
	}

	values := decodeSharedWithMeValues(t, rec.Body.Bytes())
	if len(values) != 2 {
		t.Fatalf("value length = %d, want 2", len(values))
	}
	if !bytes.Equal(values[0], mustJSON(t, local)) {
		t.Fatalf("local item is not a DriveItem\n got %s\nwant %s", values[0], mustJSON(t, local))
	}
	if !bytes.Equal(values[1], mustJSON(t, newReceivedShareDriveItem(received, nil))) {
		t.Fatalf("received item wrapper bytes differ\n got %s", values[1])
	}
	assertPermissionCount(t, values[0], 1)
	assertPermissionCount(t, values[1], 1)
}

func TestGetSharedWithMeEmptyValue(t *testing.T) {
	stampGateway(&receivedSharesGateway{})
	s := &svc{c: &config{OCMEnabled: false}}
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "me"},
	})
	req := httptest.NewRequest(http.MethodGet, "/me/drive/sharedWithMe", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.getSharedWithMe(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	var want bytes.Buffer
	if err := encodeSharedWithMe(&want, []any{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rec.Body.Bytes(), want.Bytes()) {
		t.Fatalf("empty body = %s, want %s", rec.Body.Bytes(), want.Bytes())
	}
}

type receivedSharesGateway struct {
	gateway.GatewayAPIClient
	shares    []*gateway.ReceivedShareResourceInfo
	ocmShares []*ocm.ReceivedShare
	user      *userpb.User
}

func (g *receivedSharesGateway) ListExistingReceivedShares(
	_ context.Context,
	_ *collaboration.ListReceivedSharesRequest,
	_ ...grpc.CallOption,
) (*gateway.ListExistingReceivedSharesResponse, error) {
	return &gateway.ListExistingReceivedSharesResponse{
		Status:     &rpc.Status{Code: rpc.Code_CODE_OK},
		ShareInfos: g.shares,
	}, nil
}

func (g *receivedSharesGateway) GetUser(
	_ context.Context,
	_ *userpb.GetUserRequest,
	_ ...grpc.CallOption,
) (*userpb.GetUserResponse, error) {
	return &userpb.GetUserResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		User:   g.user,
	}, nil
}

func (g *receivedSharesGateway) ListReceivedOCMShares(
	_ context.Context,
	_ *ocm.ListReceivedOCMSharesRequest,
	_ ...grpc.CallOption,
) (*ocm.ListReceivedOCMSharesResponse, error) {
	return &ocm.ListReceivedOCMSharesResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Shares: g.ocmShares,
	}, nil
}

func (g *receivedSharesGateway) GetAcceptedUser(
	_ context.Context,
	_ *invitepb.GetAcceptedUserRequest,
	_ ...grpc.CallOption,
) (*invitepb.GetAcceptedUserResponse, error) {
	return nil, errors.New("accepted user lookup skipped")
}

type failJSON struct{}

func (failJSON) MarshalJSON() ([]byte, error) {
	return nil, errors.New("marshal failed")
}

func sparseOptionalDriveItem() *libregraph.DriveItem {
	item := webdavOnlyDriveItem()
	item.Description = nil
	item.ClientSynchronize = nil
	item.UIHidden = nil
	item.File = nil
	item.Size = nil
	item.ETag = nil
	item.WebDavUrl = nil
	item.LastModifiedDateTime = nil
	item.RemoteItem.File = nil
	item.RemoteItem.ETag = nil
	item.RemoteItem.Size = nil
	item.RemoteItem.Path = nil
	item.RemoteItem.WebUrl = nil
	item.RemoteItem.WebDavUrl = nil
	item.RemoteItem.LastModifiedDateTime = nil
	return item
}

func webdavOnlyDriveItem() *libregraph.DriveItem {
	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	grant := libregraph.Permission{
		Id:              libregraph.PtrString("grant-1"),
		Roles:           []string{"b1e2218d-eef8-4d4c-b82d-0f1a1b48f3b5"},
		CreatedDateTime: *libregraph.NewNullableTime(&created),
		GrantedToV2: &libregraph.SharePointIdentitySet{
			User: &libregraph.Identity{
				Id:          libregraph.PtrString("grantee"),
				DisplayName: "Grantee",
			},
		},
		Invitation: &libregraph.SharingInvitation{
			InvitedBy: &libregraph.IdentitySet{
				User: &libregraph.Identity{
					Id:          libregraph.PtrString("creator"),
					DisplayName: "Creator",
				},
			},
		},
	}
	return &libregraph.DriveItem{
		Id:                   libregraph.PtrString("item-1"),
		Name:                 libregraph.PtrString("notes.txt"),
		ETag:                 libregraph.PtrString("etag-1"),
		Size:                 libregraph.PtrInt64(12),
		WebDavUrl:            libregraph.PtrString("https://example.test/dav/notes.txt"),
		Description:          libregraph.PtrString("optional description"),
		ClientSynchronize:    libregraph.PtrBool(true),
		UIHidden:             libregraph.PtrBool(false),
		LastModifiedDateTime: libregraph.PtrTime(created),
		File: &libregraph.OpenGraphFile{
			MimeType: libregraph.PtrString("text/plain"),
		},
		RemoteItem: &libregraph.RemoteItem{
			Id:                   libregraph.PtrString("remote-1"),
			Name:                 libregraph.PtrString("notes.txt"),
			ETag:                 libregraph.PtrString("etag-1"),
			Size:                 libregraph.PtrInt64(12),
			Path:                 libregraph.PtrString("notes.txt"),
			WebUrl:               libregraph.PtrString("https://example.test/files/notes.txt"),
			WebDavUrl:            libregraph.PtrString("https://example.test/dav/notes.txt"),
			LastModifiedDateTime: libregraph.PtrTime(created),
			Permissions:          []libregraph.Permission{grant},
		},
	}
}

func secondGrant() libregraph.Permission {
	created := time.Date(2024, 6, 7, 8, 9, 10, 0, time.UTC)
	return libregraph.Permission{
		Id:              libregraph.PtrString("grant-2"),
		Roles:           []string{"2d00ce52-1fc2-4dbc-8b95-a73b73395f5a"},
		CreatedDateTime: *libregraph.NewNullableTime(&created),
		GrantedToV2: &libregraph.SharePointIdentitySet{
			User: &libregraph.Identity{
				Id:          libregraph.PtrString("other"),
				DisplayName: "Other",
			},
		},
		Invitation: &libregraph.SharingInvitation{
			InvitedBy: &libregraph.IdentitySet{
				User: &libregraph.Identity{
					Id:          libregraph.PtrString("creator"),
					DisplayName: "Creator",
				},
			},
		},
	}
}

func receivedShareInfo() *gateway.ReceivedShareResourceInfo {
	spaceID := base32.StdEncoding.EncodeToString([]byte("/users/alice"))
	created := &types.Timestamp{Seconds: 1700000000}
	return &gateway.ReceivedShareResourceInfo{
		ReceivedShare: &collaboration.ReceivedShare{
			Share: &collaboration.Share{
				Id:      &collaboration.ShareId{OpaqueId: "share-1"},
				Creator: &userpb.UserId{OpaqueId: "creator"},
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id: &provider.Grantee_UserId{
						UserId: &userpb.UserId{OpaqueId: "grantee"},
					},
				},
				Ctime: created,
			},
		},
		ResourceInfo: &provider.ResourceInfo{
			Id: &provider.ResourceId{
				StorageId: "storage",
				OpaqueId:  "opaque",
				SpaceId:   spaceID,
			},
			Path:          "/users/alice/readme.txt",
			Name:          "readme.txt",
			Etag:          "etag",
			MimeType:      "text/plain",
			Size:          4,
			Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
			PermissionSet: permissions.NewViewerRole().CS3ResourcePermissions(),
			Mtime:         created,
		},
	}
}

func receivedOCMShareInfo() *ocm.ReceivedShare {
	created := &types.Timestamp{Seconds: 1700000000}
	return &ocm.ReceivedShare{
		Id:   &ocm.ShareId{OpaqueId: "ocm-share-1"},
		Name: "remote.txt",
		Creator: &userpb.UserId{
			OpaqueId: "remote-creator",
			Idp:      "remote.example",
		},
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: &userpb.UserId{
					OpaqueId: "me",
					Idp:      "local.example",
				},
			},
		},
		Ctime:              created,
		Mtime:              created,
		SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
		Protocols: []*ocm.Protocol{
			{
				Term: &ocm.Protocol_WebdavOptions{
					WebdavOptions: &ocm.WebDAVProtocol{
						Uri: "https://remote.example/dav/remote.txt",
						Permissions: &ocm.SharePermissions{
							Permissions: permissions.NewViewerRole().CS3ResourcePermissions(),
						},
					},
				},
			},
		},
	}
}

func decodeSharedWithMeItem(t *testing.T, raw []byte) []byte {
	t.Helper()
	values := decodeSharedWithMeValues(t, raw)
	if len(values) != 1 {
		t.Fatalf("value length = %d, want 1", len(values))
	}
	return values[0]
}

func decodeSharedWithMeValues(t *testing.T, raw []byte) []json.RawMessage {
	t.Helper()
	envelope := decodeObject(t, raw)
	var values []json.RawMessage
	if err := json.Unmarshal(envelope["value"], &values); err != nil {
		t.Fatalf("decode value: %v", err)
	}
	return values
}

func assertPermissionCount(t *testing.T, item []byte, want int) {
	t.Helper()
	root := decodeObject(t, item)
	if _, ok := root[receivedWebappJSONKey]; ok {
		t.Fatalf("%s placed on drive item", receivedWebappJSONKey)
	}
	remote := decodeObject(t, root["remoteItem"])
	if _, ok := remote[receivedWebappJSONKey]; ok {
		t.Fatalf("%s present on remoteItem", receivedWebappJSONKey)
	}
	var perms []json.RawMessage
	if err := json.Unmarshal(remote["permissions"], &perms); err != nil {
		t.Fatal(err)
	}
	if len(perms) != want {
		t.Fatalf("permissions length = %d, want %d", len(perms), want)
	}
}

func assertWebappSibling(t *testing.T, base, got []byte, appName string) {
	t.Helper()

	baseRoot := decodeObject(t, base)
	gotRoot := decodeObject(t, got)
	if _, ok := gotRoot[receivedWebappJSONKey]; ok {
		t.Fatalf("%s placed on drive item", receivedWebappJSONKey)
	}
	if len(gotRoot) != len(baseRoot) {
		t.Fatalf("root keys = %d, want %d", len(gotRoot), len(baseRoot))
	}
	for key, raw := range baseRoot {
		if key == "remoteItem" {
			continue
		}
		if !bytes.Equal(gotRoot[key], raw) {
			t.Fatalf("root field %s changed\n got %s\nwant %s", key, gotRoot[key], raw)
		}
	}

	baseRemote := decodeObject(t, baseRoot["remoteItem"])
	gotRemote := decodeObject(t, gotRoot["remoteItem"])
	rawWebapp, ok := gotRemote[receivedWebappJSONKey]
	if !ok {
		t.Fatal("missing remoteItem webapp")
	}
	if len(gotRemote) != len(baseRemote)+1 {
		t.Fatalf("remoteItem keys = %d, want %d", len(gotRemote), len(baseRemote)+1)
	}
	for key, raw := range baseRemote {
		if !bytes.Equal(gotRemote[key], raw) {
			t.Fatalf("remoteItem field %s changed\n got %s\nwant %s", key, gotRemote[key], raw)
		}
	}

	var perms []map[string]json.RawMessage
	if err := json.Unmarshal(gotRemote["permissions"], &perms); err != nil {
		t.Fatal(err)
	}
	if len(perms) < 1 {
		t.Fatal("permissions empty")
	}
	if _, ok := perms[0]["grantedToV2"]; !ok {
		t.Fatal("grantedToV2 missing on permissions[0]")
	}
	if _, ok := perms[0]["invitation"]; !ok {
		t.Fatal("invitation missing on permissions[0]")
	}
	if !bytes.Equal(perms[0]["grantedToV2"], decodePermField(t, baseRemote["permissions"], 0, "grantedToV2")) {
		t.Fatal("grantedToV2 changed")
	}
	if !bytes.Equal(perms[0]["invitation"], decodePermField(t, baseRemote["permissions"], 0, "invitation")) {
		t.Fatal("invitation changed")
	}

	webapp := decodeObject(t, rawWebapp)
	if len(webapp) != 1 {
		t.Fatalf("webapp keys = %d, want 1", len(webapp))
	}
	var decodedName string
	if err := json.Unmarshal(webapp["appName"], &decodedName); err != nil {
		t.Fatal(err)
	}
	if decodedName != appName {
		t.Fatalf("appName = %q, want %q", decodedName, appName)
	}
	for _, forbidden := range []string{
		"secret",
		"token",
		"http://",
		"https://",
		"@libre.graph.permissions.actions",
	} {
		if bytes.Contains(rawWebapp, []byte(forbidden)) {
			t.Fatalf("webapp contains %s: %s", forbidden, rawWebapp)
		}
	}
}

func decodeObject(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("decode object: %v; raw %s", err, raw)
	}
	return obj
}

func decodePermField(t *testing.T, raw []byte, index int, field string) json.RawMessage {
	t.Helper()
	var perms []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &perms); err != nil {
		t.Fatal(err)
	}
	return perms[index][field]
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
