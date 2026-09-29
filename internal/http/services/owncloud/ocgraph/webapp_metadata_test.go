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
