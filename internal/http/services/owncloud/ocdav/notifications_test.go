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

package ocdav

import (
	"context"
	"encoding/json"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type uploadNotificationGateway struct {
	gateway.GatewayAPIClient
	usersByID    map[string]*userpb.User
	statByID     map[string]*provider.ResourceInfo
	publishedEvs []*gateway.Event
}

func (g *uploadNotificationGateway) PublishEvent(_ context.Context, req *gateway.PublishEventRequest, _ ...grpc.CallOption) (*gateway.PublishEventResponse, error) {
	g.publishedEvs = append(g.publishedEvs, req.GetEvent())
	return &gateway.PublishEventResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

func (g *uploadNotificationGateway) GetUser(_ context.Context, req *userpb.GetUserRequest, _ ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	if u, ok := g.usersByID[req.GetUserId().GetOpaqueId()]; ok {
		return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: u}, nil
	}
	return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func (g *uploadNotificationGateway) Stat(_ context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	if info, ok := g.statByID[req.GetRef().GetResourceId().GetOpaqueId()]; ok {
		return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: info}, nil
	}
	return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

// The upload email must name the shared folder the file landed in, resolved by
// stat-ing the link's target folder, not the uploaded file. This pins the
// regression where the share path resolved to the uploaded file's own path.
func TestSendUploadNotificationDescribesFolderNotFile(t *testing.T) {
	sharedconf.Init(&revaconfig.Shared{JWTSecret: "upload-notification-test-secret"})
	if sharedconf.GetJWTSecret("") == "" {
		t.Skip("shared jwt secret is not initialised, cannot elevate to machine scope")
	}

	uploader := &userpb.User{Id: &userpb.UserId{Idp: "public", OpaqueId: "publiclink:abc", Type: userpb.UserType_USER_TYPE_PRIMARY}}
	ctx := appctx.ContextSetUser(context.Background(), uploader)

	owner := &userpb.User{Id: &userpb.UserId{Idp: "idp", OpaqueId: "owner"}, Mail: "owner@cern.ch"}
	folderID := &provider.ResourceId{StorageId: "eoshome", OpaqueId: "folder-uuid"}
	share := &link.PublicShare{
		Id:                           &link.PublicShareId{OpaqueId: "share-1"},
		Token:                        "VB8CsMtt3796SDo",
		DisplayName:                  "AI guidelines",
		ResourceId:                   folderID,
		Owner:                        owner.Id,
		NotifyUploads:                true,
		NotifyUploadsExtraRecipients: "extra@cern.ch",
	}
	shareJSON, err := json.Marshal(share)
	if err != nil {
		t.Fatalf("marshalling public share: %v", err)
	}

	info := &provider.ResourceInfo{
		Id:       &provider.ResourceId{StorageId: "public", OpaqueId: "VB8CsMtt3796SDo/file-uuid"},
		Name:     "it-sd-ai-guidelines.pdf",
		Path:     "/public/VB8CsMtt3796SDo/it-sd-ai-guidelines.pdf",
		Type:     provider.ResourceType_RESOURCE_TYPE_FILE,
		MimeType: "application/pdf",
		Size:     1024,
		Opaque: &types.Opaque{Map: map[string]*types.OpaqueEntry{
			"link-share": {Decoder: "json", Value: shareJSON},
		}},
	}

	gw := &uploadNotificationGateway{
		usersByID: map[string]*userpb.User{"owner": owner},
		statByID: map[string]*provider.ResourceInfo{
			"folder-uuid": {
				Id:   folderID,
				Name: "SharedFolder",
				Path: "/eos/project/c/cernbox/SharedFolder",
				Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			},
		},
	}
	s := &svc{c: &Config{GatewaySvc: "endpoint"}}

	s.sendUploadNotification(ctx, gw, info, zerolog.Logger{})

	if len(gw.publishedEvs) != 1 {
		t.Fatalf("published events = %d, want 1", len(gw.publishedEvs))
	}
	event := gw.publishedEvs[0]
	if event.GetType() != model.EventUpload {
		t.Fatalf("event type = %q, want %q", event.GetType(), model.EventUpload)
	}

	recipients, templateData, err := notifications.DecodeEvent(event)
	if err != nil {
		t.Fatalf("DecodeEvent() returned error: %v", err)
	}

	wantRecipients := map[string]bool{"extra@cern.ch": true, "owner@cern.ch": true}
	if len(recipients) != len(wantRecipients) {
		t.Fatalf("recipients = %v, want %v", recipients, wantRecipients)
	}
	for _, recipient := range recipients {
		if !wantRecipients[recipient.GetMail()] {
			t.Fatalf("unexpected recipient %q, want one of %v", recipient.GetMail(), wantRecipients)
		}
	}

	wantData := map[string]string{
		"share_name":    "SharedFolder",
		"share_path":    "/eos/project/c/cernbox/SharedFolder",
		"share_token":   "VB8CsMtt3796SDo",
		"resource_name": "it-sd-ai-guidelines.pdf",
		"resource_path": "/public/VB8CsMtt3796SDo/it-sd-ai-guidelines.pdf",
	}
	for key, want := range wantData {
		got, ok := templateData[key].(string)
		if !ok {
			t.Fatalf("template data %q = %v, want string %q", key, templateData[key], want)
		}
		if got != want {
			t.Fatalf("template data %q = %q, want %q", key, got, want)
		}
	}
}
