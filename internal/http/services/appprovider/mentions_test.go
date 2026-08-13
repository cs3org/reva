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

package appprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"google.golang.org/grpc"
)

const mentionsTestJWTSecret = "mentions-test-secret"

type mentionGateway struct {
	gateway.GatewayAPIClient
	statResp     *provider.StatResponse
	statErr      error
	usersByName  map[string]*userpb.User
	usersByID    map[string]*userpb.User
	groups       map[string]*grouppb.Group
	publishedEvs []*gateway.Event
}

func (m *mentionGateway) PublishEvent(_ context.Context, req *gateway.PublishEventRequest, _ ...grpc.CallOption) (*gateway.PublishEventResponse, error) {
	m.publishedEvs = append(m.publishedEvs, req.GetEvent())
	return &gateway.PublishEventResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

func (m *mentionGateway) Stat(_ context.Context, _ *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	return m.statResp, m.statErr
}

func (m *mentionGateway) GetUserByClaim(_ context.Context, req *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	if req.Claim != "username" {
		return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	if u, ok := m.usersByName[req.Value]; ok {
		return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: u}, nil
	}
	return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func (m *mentionGateway) GetGroupByClaim(_ context.Context, req *grouppb.GetGroupByClaimRequest, _ ...grpc.CallOption) (*grouppb.GetGroupByClaimResponse, error) {
	if req.Claim != "group_name" {
		return &grouppb.GetGroupByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	if g, ok := m.groups[req.Value]; ok {
		return &grouppb.GetGroupByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Group: g}, nil
	}
	return &grouppb.GetGroupByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func (m *mentionGateway) GetUser(_ context.Context, req *userpb.GetUserRequest, _ ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	if req.UserId == nil {
		return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	if u, ok := m.usersByID[userIDKey(req.UserId)]; ok {
		return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: u}, nil
	}
	return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func TestDecodeMentionRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "valid with inferred user type",
			body: `{"path":"/doc.docx","mentions":[{"username":"alice"}]}`,
		},
		{
			name:    "requires document reference",
			body:    `{"mentions":[{"username":"alice"}]}`,
			wantErr: "missing file_id or path",
		},
		{
			name:    "requires mentions",
			body:    `{"path":"/doc.docx","mentions":[]}`,
			wantErr: "missing mentions",
		},
		{
			name:    "rejects unknown mention type",
			body:    `{"path":"/doc.docx","mentions":[{"type":"channel","username":"alice"}]}`,
			wantErr: "mention type must be user or group",
		},
		{
			name:    "rejects group without groupname",
			body:    `{"path":"/doc.docx","mentions":[{"type":"group"}]}`,
			wantErr: "group mention is missing groupname",
		},
		{
			name:    "rejects invalid json fields",
			body:    `{"path":"/doc.docx","mentions":[{"username":"alice"}],"extra":true}`,
			wantErr: "invalid JSON request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/app/mentions", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			got, err := decodeMentionRequest(rec, req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("decodeMentionRequest() returned error: %v", err)
				}
				if got.Mentions[0].Type == "" {
					t.Fatalf("decodeMentionRequest() did not infer mention type")
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("decodeMentionRequest() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestResourceReferenceFromRequestValue(t *testing.T) {
	resourceID := &provider.ResourceId{StorageId: "storage", SpaceId: "space", OpaqueId: "opaque"}
	encoded := spaces.EncodeToStringifiedResourceID(resourceID)

	t.Run("path fallback", func(t *testing.T) {
		ref, err := resourceReferenceFromRequestValue("", "/a/b.docx")
		if err != nil {
			t.Fatalf("resourceReferenceFromRequestValue() returned error: %v", err)
		}
		if ref.Path != "/a/b.docx" {
			t.Fatalf("ref.Path = %q, want /a/b.docx", ref.Path)
		}
	})

	t.Run("spaces id", func(t *testing.T) {
		ref, err := resourceReferenceFromRequestValue(encoded, "")
		if err != nil {
			t.Fatalf("resourceReferenceFromRequestValue() returned error: %v", err)
		}
		if ref.ResourceId == nil || ref.ResourceId.OpaqueId != "opaque" {
			t.Fatalf("ref.ResourceId = %+v, want opaque id", ref.ResourceId)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		if _, err := resourceReferenceFromRequestValue("not a resource id", ""); err == nil {
			t.Fatalf("resourceReferenceFromRequestValue() returned nil error for invalid id")
		}
	})
}

func TestResolveMentionRecipientsExpandsGroupsDeduplicatesAndSkipsSelf(t *testing.T) {
	author := mentionUser("author", "author@cern.ch")
	alice := mentionUser("alice", "alice@cern.ch")
	bob := mentionUser("bob", "bob@cern.ch")
	noMail := mentionUser("nomail", "")

	gw := &mentionGateway{
		usersByName: map[string]*userpb.User{
			"alice":  alice,
			"nomail": noMail,
		},
		usersByID: map[string]*userpb.User{
			userIDKey(alice.Id):  alice,
			userIDKey(bob.Id):    bob,
			userIDKey(author.Id): author,
		},
		groups: map[string]*grouppb.Group{
			"team": {
				GroupName: "team",
				Members:   []*userpb.UserId{alice.Id, bob.Id, author.Id},
			},
		},
	}

	resolved := resolveMentionRecipients(context.Background(), gw, []mentionTarget{
		{Type: "user", Username: "alice"},
		{Type: "user", Username: "nomail"},
		{Type: "group", GroupName: "team"},
		{Type: "group", GroupName: "missing"},
	}, author)

	if len(resolved.users) != 2 {
		t.Fatalf("resolved users = %d, want 2", len(resolved.users))
	}
	if resolved.users[0].Username != "alice" || resolved.users[1].Username != "bob" {
		t.Fatalf("resolved users = %v, want alice and bob", []string{resolved.users[0].Username, resolved.users[1].Username})
	}
	if len(resolved.rejected) != 2 {
		t.Fatalf("rejected = %d, want 2", len(resolved.rejected))
	}
	if resolved.rejected[0].Reason != "user_has_no_email" || resolved.rejected[1].Reason != "group_not_found" {
		t.Fatalf("rejected reasons = %q, %q", resolved.rejected[0].Reason, resolved.rejected[1].Reason)
	}
}

func TestHandleMentionsAcceptsResolvedMentions(t *testing.T) {
	sharedconf.Init(&revaconfig.Shared{JWTSecret: mentionsTestJWTSecret})
	if sharedconf.GetJWTSecret("") == "" {
		t.Skip("shared jwt secret is not initialised, cannot elevate to machine scope")
	}

	endpoint := "mentions-" + t.Name()
	author := mentionUser("author", "author@cern.ch")
	alice := mentionUser("alice", "alice@cern.ch")
	bob := mentionUser("bob", "bob@cern.ch")
	resourceID := &provider.ResourceId{StorageId: "storage", SpaceId: "space", OpaqueId: "file"}

	gw := &mentionGateway{
		statResp: &provider.StatResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_OK},
			Info: &provider.ResourceInfo{
				Id:   resourceID,
				Path: "/spaces/project/report.docx",
				Type: provider.ResourceType_RESOURCE_TYPE_FILE,
			},
		},
		usersByName: map[string]*userpb.User{
			"alice": alice,
		},
		usersByID: map[string]*userpb.User{
			userIDKey(alice.Id):  alice,
			userIDKey(bob.Id):    bob,
			userIDKey(author.Id): author,
		},
		groups: map[string]*grouppb.Group{
			"team": {
				GroupName: "team",
				Members:   []*userpb.UserId{alice.Id, bob.Id, author.Id},
			},
		},
	}
	pool.RegisterGatewayServiceClient(gw, endpoint)

	s := &svc{
		conf: &Config{GatewaySvc: endpoint},
	}

	body := `{
		"file_id":"` + spaces.EncodeToStringifiedResourceID(resourceID) + `",
		"mentions":[
			{"type":"user","username":"alice"},
			{"type":"group","groupname":"team"}
		],
		"event_id":"event-1",
		"comment_text":"Can you check this?",
		"anchor_text":"Total cost",
		"document_url":"https://cernbox.example/report.docx",
		"app_name":"office"
	}`
	req := httptest.NewRequest(http.MethodPost, "/app/mentions", strings.NewReader(body))
	req = req.WithContext(appctx.ContextSetUser(req.Context(), author))
	rec := httptest.NewRecorder()

	s.handleMentions(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var response mentionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("response json: %v", err)
	}
	if len(response.Accepted) != 2 {
		t.Fatalf("accepted = %d, want 2", len(response.Accepted))
	}
	if len(response.Rejected) != 0 {
		t.Fatalf("rejected = %d, want 0", len(response.Rejected))
	}

	if len(gw.publishedEvs) != 1 {
		t.Fatalf("published events = %d, want 1", len(gw.publishedEvs))
	}
	event := gw.publishedEvs[0]
	if event.GetType() != model.EventOfficeMention {
		t.Fatalf("event type = %q, want %q", event.GetType(), model.EventOfficeMention)
	}

	recipients, templateData, err := notifications.DecodeEvent(event)
	if err != nil {
		t.Fatalf("DecodeEvent() returned error: %v", err)
	}
	// One event carries every recipient; the notifications worker fans it out.
	wantRecipients := map[string]bool{"alice@cern.ch": true, "bob@cern.ch": true}
	if len(recipients) != len(wantRecipients) {
		t.Fatalf("recipients = %v, want %v", recipients, wantRecipients)
	}
	for _, recipient := range recipients {
		if !wantRecipients[recipient.GetMail()] {
			t.Fatalf("unexpected recipient %q, want one of %v", recipient.GetMail(), wantRecipients)
		}
		if recipient.GetId().GetOpaqueId() == "" {
			t.Fatalf("recipient %q carries no user id", recipient.GetMail())
		}
	}

	// The dedup key template in the notifications config reads resource_id, so an
	// absent or empty one silently drops the accumulation.
	wantData := map[string]string{
		"resource_id":            spaces.EncodeToStringifiedResourceID(resourceID),
		"resource_name":          "report.docx",
		"resource_path":          "/spaces/project/report.docx",
		"mentioner_display_name": "author display",
		"mentioner_username":     "author",
		"comment_text":           "Can you check this?",
		"anchor_text":            "Total cost",
		"document_url":           "https://cernbox.example/report.docx",
		"app_name":               "office",
		"event_id":               "event-1",
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

func TestMentionsEndpointIsProtected(t *testing.T) {
	s := &svc{}
	for _, path := range s.Unprotected() {
		if path == "/mentions" {
			t.Fatalf("/mentions must not be listed as unprotected")
		}
	}
}

func mentionUser(username, mail string) *userpb.User {
	return &userpb.User{
		Id:          &userpb.UserId{Idp: "idp", OpaqueId: username},
		Username:    username,
		Mail:        mail,
		DisplayName: username + " display",
	}
}

func userIDKey(id *userpb.UserId) string {
	if id == nil {
		return ""
	}
	return id.Idp + ":" + id.OpaqueId
}
