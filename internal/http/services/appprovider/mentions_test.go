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
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/notification/trigger"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"google.golang.org/grpc"
)

type fakeNotificationTriggerer struct {
	triggers []*trigger.Trigger
	stopped  bool
}

func (f *fakeNotificationTriggerer) TriggerNotification(t *trigger.Trigger) {
	f.triggers = append(f.triggers, t)
}

func (f *fakeNotificationTriggerer) Stop() {
	f.stopped = true
}

type mentionGateway struct {
	gateway.GatewayAPIClient
	statResp    *provider.StatResponse
	statErr     error
	usersByName map[string]*userpb.User
	usersByID   map[string]*userpb.User
	groups      map[string]*grouppb.Group
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

func TestHandleMentionsPublishesTriggers(t *testing.T) {
	endpoint := "mentions-" + t.Name()
	author := mentionUser("author", "author@cern.ch")
	alice := mentionUser("alice", "alice@cern.ch")
	bob := mentionUser("bob", "bob@cern.ch")
	resourceID := &provider.ResourceId{StorageId: "storage", SpaceId: "space", OpaqueId: "file"}

	pool.RegisterGatewayServiceClient(&mentionGateway{
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
	}, endpoint)

	notifier := &fakeNotificationTriggerer{}
	s := &svc{
		conf:               &Config{GatewaySvc: endpoint},
		notificationHelper: notifier,
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
	if len(notifier.triggers) != 2 {
		t.Fatalf("triggers = %d, want 2", len(notifier.triggers))
	}

	first := notifier.triggers[0]
	if first.Notification.TemplateName != mentionTemplateName {
		t.Fatalf("template = %q, want %q", first.Notification.TemplateName, mentionTemplateName)
	}
	if first.Notification.Recipients[0] != "alice@cern.ch" {
		t.Fatalf("first recipient = %q, want alice@cern.ch", first.Notification.Recipients[0])
	}
	if first.Sender != "author@cern.ch" {
		t.Fatalf("sender = %q, want author@cern.ch", first.Sender)
	}
	if first.TemplateData["commentText"] != "Can you check this?" {
		t.Fatalf("commentText = %v", first.TemplateData["commentText"])
	}
	if first.TemplateData["anchorText"] != "Total cost" {
		t.Fatalf("anchorText = %v", first.TemplateData["anchorText"])
	}

	second := notifier.triggers[1]
	if second.Notification.Recipients[0] != "bob@cern.ch" {
		t.Fatalf("second recipient = %q, want bob@cern.ch", second.Notification.Recipients[0])
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
}

func TestHandleMentionsRequiresNotificationHelper(t *testing.T) {
	s := &svc{conf: &Config{}}
	req := httptest.NewRequest(http.MethodPost, "/app/mentions", strings.NewReader(`{"path":"/doc.docx","mentions":[{"username":"alice"}]}`))
	rec := httptest.NewRecorder()

	s.handleMentions(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
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
