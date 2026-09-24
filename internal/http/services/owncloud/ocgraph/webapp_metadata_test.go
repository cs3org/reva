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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"google.golang.org/grpc"

	"github.com/cs3org/reva/v3/pkg/appctx"
)

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

func TestGetSharedWithMeExposesPersistedWebappName(t *testing.T) {
	const (
		appName                 = "CodiMD"
		receiverLocalConfig     = "LocalPad"
		secretMustNotLeak       = "secret-must-stay-in-storage"
		webdavSecretMustNotLeak = "webdav-secret-must-stay-in-storage"
	)
	share := receivedOCMShareWithWebapp("ocm-share-webapp", "remote.txt", appName)
	body := sharedWithMeOCM(t, share)

	if bytes.Contains(body, []byte(secretMustNotLeak)) || bytes.Contains(body, []byte(webdavSecretMustNotLeak)) {
		t.Fatalf("secret leaked into handler body %s", body)
	}
	if bytes.Contains(body, []byte(receiverLocalConfig)) {
		t.Fatalf("receiver local config name leaked into handler body %s", body)
	}
	if bytes.Contains(body, []byte("appIconHint")) || bytes.Contains(body, []byte("mediaTypes")) {
		t.Fatalf("storage-only webapp fields leaked into handler body %s", body)
	}

	values := decodeSharedWithMeValues(t, body)
	if len(values) != 1 {
		t.Fatalf("value length = %d, want 1", len(values))
	}
	envelope := decodeObject(t, body)
	if len(envelope) != 1 {
		t.Fatalf("envelope keys = %d, want 1", len(envelope))
	}
	if _, ok := envelope["value"]; !ok {
		t.Fatal("envelope is missing value")
	}

	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "me"},
	})
	s := sharedWithMeService(t)
	drive, err := s.OCMReceivedShareToDriveItem(ctx, share)
	if err != nil {
		t.Fatal(err)
	}
	base := mustJSON(t, drive)
	assertWebappSibling(t, base, values[0], appName)
	assertPermissionCountAllowingWebapp(t, values[0], 1)

	meta := receivedShareWebappMetadata(share)
	if meta == nil || !meta.Present || meta.AppName != appName {
		t.Fatalf("adapter metadata = %+v", meta)
	}
	var want bytes.Buffer
	if err := encodeSharedWithMe(&want, []any{newReceivedShareDriveItem(drive, meta)}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, want.Bytes()) {
		t.Fatalf("handler bytes differ\n got %s\nwant %s", body, want.Bytes())
	}
}

func TestGetSharedWithMeWebDAVOnlyOmitsWebapp(t *testing.T) {
	share := receivedOCMShareInfo()
	body := sharedWithMeOCM(t, share)
	if bytes.Contains(body, []byte(receivedWebappJSONKey)) {
		t.Fatalf("webdav-only body contains %s: %s", receivedWebappJSONKey, body)
	}

	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "me"},
	})
	s := sharedWithMeService(t)
	drive, err := s.OCMReceivedShareToDriveItem(ctx, share)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := encodeSharedWithMe(&want, []any{newReceivedShareDriveItem(drive, nil)}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, want.Bytes()) {
		t.Fatalf("webdav-only bytes differ from absent metadata\n got %s\nwant %s", body, want.Bytes())
	}
	if receivedShareWebappMetadata(share) != nil {
		t.Fatal("webdav-only adapter metadata is not nil")
	}
}

func TestGetSharedWithMeWebappNamesStayOnMatchingShare(t *testing.T) {
	const receiverLocalConfig = "LocalPad"
	first := receivedOCMShareWithWebapp("ocm-share-a", "a.txt", "CodiMD")
	second := receivedOCMShareWithWebapp("ocm-share-b", "b.txt", "Etherpad")
	body := sharedWithMeOCM(t, first, second)
	if bytes.Contains(body, []byte(receiverLocalConfig)) {
		t.Fatalf("receiver local config name leaked into handler body %s", body)
	}

	values := decodeSharedWithMeValues(t, body)
	if len(values) != 2 {
		t.Fatalf("value length = %d, want 2", len(values))
	}

	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "me"},
	})
	s := sharedWithMeService(t)
	for i, share := range []*ocm.ReceivedShare{first, second} {
		drive, err := s.OCMReceivedShareToDriveItem(ctx, share)
		if err != nil {
			t.Fatal(err)
		}
		assertWebappSibling(t, mustJSON(t, drive), values[i], share.Protocols[1].GetWebappOptions().GetAppName())
		assertPermissionCountAllowingWebapp(t, values[i], 1)
	}

	firstName := webappName(t, values[0])
	secondName := webappName(t, values[1])
	if firstName != "CodiMD" || secondName != "Etherpad" {
		t.Fatalf("names = %q, %q", firstName, secondName)
	}
	if bytes.Contains(remoteItemOf(t, values[0]), []byte("Etherpad")) {
		t.Fatal("first remoteItem inherited Etherpad")
	}
	if bytes.Contains(remoteItemOf(t, values[1]), []byte("CodiMD")) {
		t.Fatal("second remoteItem inherited CodiMD")
	}
}

func TestReceivedWebappMetadataMissingRemoteItemPropagates(t *testing.T) {
	share := receivedOCMShareWithWebapp("ocm-share-missing", "remote.txt", "CodiMD")
	meta := receivedShareWebappMetadata(share)
	if meta == nil || meta.AppName != "CodiMD" {
		t.Fatalf("adapter metadata = %+v", meta)
	}

	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "me"},
	})
	stampGateway(&receivedSharesGateway{
		user: &userpb.User{
			Id:          &userpb.UserId{OpaqueId: "creator"},
			DisplayName: "Creator",
		},
	})
	s := sharedWithMeService(t)
	drive, err := s.OCMReceivedShareToDriveItem(ctx, share)
	if err != nil {
		t.Fatal(err)
	}
	drive.RemoteItem = nil

	var buf bytes.Buffer
	err = encodeSharedWithMe(&buf, []any{
		newReceivedShareDriveItem(webdavOnlyDriveItem(), nil),
		newReceivedShareDriveItem(drive, meta),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if buf.Len() != 0 {
		t.Fatalf("partial bytes %s", buf.Bytes())
	}

	got, err := extendReceivedShareRemoteItem([]byte(`{"remoteItem":[]}`), meta.AppName)
	if err == nil {
		t.Fatal("expected error for non-object remoteItem")
	}
	if got != nil {
		t.Fatalf("partial bytes %s", got)
	}
}

func sharedWithMeService(t *testing.T) *svc {
	t.Helper()
	return &svc{c: &config{
		GatewaySvc: "ocgraph-webapp-producer-" + t.Name(),
		WebBase:    "https://example.test/files/spaces",
		OCMEnabled: true,
	}}
}

func sharedWithMeOCM(t *testing.T, shares ...*ocm.ReceivedShare) []byte {
	t.Helper()
	gw := &receivedSharesGateway{
		ocmShares: shares,
		user: &userpb.User{
			Id:          &userpb.UserId{OpaqueId: "creator"},
			DisplayName: "Creator",
		},
	}
	stampGateway(gw)
	s := sharedWithMeService(t)
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "me"},
	})
	req := httptest.NewRequest(http.MethodGet, "/me/drive/sharedWithMe", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.getSharedWithMe(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
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

func receivedOCMShareWithWebapp(id, name, appName string) *ocm.ReceivedShare {
	share := receivedOCMShareInfo()
	share.Id = &ocm.ShareId{OpaqueId: id}
	share.Name = name
	share.Protocols = append(share.Protocols, &ocm.Protocol{
		Term: &ocm.Protocol_WebappOptions{
			WebappOptions: &ocm.WebappProtocol{
				Uri:          "https://remote.example/open/" + name,
				SharedSecret: "secret-must-stay-in-storage",
				AppName:      appName,
				AppIconHint:  "image/png",
				MediaTypes:   []string{"text/markdown"},
			},
		},
	})
	share.Protocols[0].GetWebdavOptions().SharedSecret = "webdav-secret-must-stay-in-storage"
	return share
}

func assertPermissionCountAllowingWebapp(t *testing.T, item []byte, want int) {
	t.Helper()
	root := decodeObject(t, item)
	if _, ok := root[receivedWebappJSONKey]; ok {
		t.Fatalf("%s placed on drive item", receivedWebappJSONKey)
	}
	remote := decodeObject(t, root["remoteItem"])
	var perms []json.RawMessage
	if err := json.Unmarshal(remote["permissions"], &perms); err != nil {
		t.Fatal(err)
	}
	if len(perms) != want {
		t.Fatalf("permissions length = %d, want %d", len(perms), want)
	}
	if _, ok := remote[receivedWebappJSONKey]; !ok {
		t.Fatal("missing remoteItem webapp")
	}
}

func webappName(t *testing.T, item []byte) string {
	t.Helper()
	remote := decodeObject(t, remoteItemOf(t, item))
	webapp := decodeObject(t, remote[receivedWebappJSONKey])
	var name string
	if err := json.Unmarshal(webapp["appName"], &name); err != nil {
		t.Fatal(err)
	}
	return name
}

func remoteItemOf(t *testing.T, item []byte) []byte {
	t.Helper()
	return decodeObject(t, item)["remoteItem"]
}
