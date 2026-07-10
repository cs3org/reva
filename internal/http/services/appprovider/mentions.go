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
	"fmt"
	"net/http"
	"net/url"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/utils/resourceid"
)

const (
	maxMentionBodyBytes = 1 << 20
	maxMentionTargets   = 100
	maxMentionTextLen   = 2000
)

type mentionRequest struct {
	FileID      string          `json:"file_id"`
	Path        string          `json:"path"`
	Mentions    []mentionTarget `json:"mentions"`
	EventID     string          `json:"event_id"`
	CommentText string          `json:"comment_text"`
	AnchorText  string          `json:"anchor_text"`
	DocumentURL string          `json:"document_url"`
	AppName     string          `json:"app_name"`
}

type mentionTarget struct {
	Type      string `json:"type"`
	Username  string `json:"username"`
	GroupName string `json:"groupname"`
	OpaqueID  string `json:"opaque_id"`
}

type mentionResponse struct {
	Accepted []mentionResult `json:"accepted"`
	Rejected []mentionResult `json:"rejected"`
}

type mentionResult struct {
	Type      string `json:"type,omitempty"`
	Username  string `json:"username,omitempty"`
	GroupName string `json:"groupname,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func (s *svc) handleMentions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	req, err := decodeMentionRequest(w, r)
	if err != nil {
		writeError(w, r, appErrorInvalidParameter, err.Error(), nil)
		return
	}

	author, ok := appctx.ContextGetUser(ctx)
	if !ok || author == nil {
		writeError(w, r, appErrorUnauthenticated, "missing authenticated user", nil)
		return
	}

	if _, err := validateDocumentURL(req.DocumentURL); err != nil {
		writeError(w, r, appErrorInvalidParameter, err.Error(), nil)
		return
	}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(s.conf.GatewaySvc))
	if err != nil {
		writeError(w, r, appErrorServerError, "error getting grpc gateway client", err)
		return
	}

	fileRef, err := resourceReferenceFromRequestValue(req.FileID, req.Path)
	if err != nil {
		writeError(w, r, appErrorInvalidParameter, err.Error(), nil)
		return
	}

	statRes, err := client.Stat(ctx, &provider.StatRequest{Ref: &fileRef})
	if err != nil {
		writeError(w, r, appErrorServerError, "failed to stat the file", err)
		return
	}
	if statRes.Status == nil || statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
		writeError(w, r, appErrorNotFound, "file does not exist", nil)
		return
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		writeError(w, r, appErrorServerError, "failed to stat the file", nil)
		return
	}
	if statRes.Info == nil || statRes.Info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		writeError(w, r, appErrorInvalidParameter, "the given document reference does not point to a file", nil)
		return
	}

	resolved := resolveMentionRecipients(ctx, client, req.Mentions, author)
	if len(resolved.users) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(mentionResponse{Rejected: resolved.rejected})
		return
	}

	documentID := documentIDForNotification(statRes.Info, &fileRef)
	for _, recipient := range resolved.users {
		resolved.accepted = append(resolved.accepted, mentionResult{Type: "user", Username: recipient.Username})
	}

	log.Info().
		Str("file_id", documentID).
		Str("event_id", req.EventID).
		Int("accepted", len(resolved.accepted)).
		Int("rejected", len(resolved.rejected)).
		Msg("office mention notifications accepted")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(mentionResponse{
		Accepted: resolved.accepted,
		Rejected: resolved.rejected,
	})
}

func decodeMentionRequest(w http.ResponseWriter, r *http.Request) (*mentionRequest, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMentionBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req mentionRequest
	if err := dec.Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid JSON request body")
	}

	req.FileID = strings.TrimSpace(req.FileID)
	req.Path = strings.TrimSpace(req.Path)
	req.EventID = strings.TrimSpace(req.EventID)
	req.CommentText = strings.TrimSpace(req.CommentText)
	req.AnchorText = strings.TrimSpace(req.AnchorText)
	req.DocumentURL = strings.TrimSpace(req.DocumentURL)
	req.AppName = strings.TrimSpace(req.AppName)

	if req.FileID == "" && req.Path == "" {
		return nil, fmt.Errorf("missing file_id or path")
	}
	if len(req.Mentions) == 0 {
		return nil, fmt.Errorf("missing mentions")
	}
	if len(req.Mentions) > maxMentionTargets {
		return nil, fmt.Errorf("too many mentions")
	}
	if len(req.CommentText) > maxMentionTextLen {
		return nil, fmt.Errorf("comment_text is too long")
	}
	if len(req.AnchorText) > maxMentionTextLen {
		return nil, fmt.Errorf("anchor_text is too long")
	}

	for i := range req.Mentions {
		req.Mentions[i].Type = strings.ToLower(strings.TrimSpace(req.Mentions[i].Type))
		req.Mentions[i].Username = strings.TrimSpace(req.Mentions[i].Username)
		req.Mentions[i].GroupName = strings.TrimSpace(req.Mentions[i].GroupName)
		req.Mentions[i].OpaqueID = strings.TrimSpace(req.Mentions[i].OpaqueID)

		if req.Mentions[i].Type == "" {
			switch {
			case req.Mentions[i].Username != "":
				req.Mentions[i].Type = "user"
			case req.Mentions[i].GroupName != "":
				req.Mentions[i].Type = "group"
			}
		}
		switch req.Mentions[i].Type {
		case "user":
			if req.Mentions[i].Username == "" {
				return nil, fmt.Errorf("user mention is missing username")
			}
		case "group":
			if req.Mentions[i].GroupName == "" {
				return nil, fmt.Errorf("group mention is missing groupname")
			}
		default:
			return nil, fmt.Errorf("mention type must be user or group")
		}
	}

	return &req, nil
}

func resourceReferenceFromRequestValue(fileID, path string) (provider.Reference, error) {
	if fileID == "" {
		if path == "" {
			return provider.Reference{}, fmt.Errorf("missing file_id or path")
		}
		return provider.Reference{Path: path}, nil
	}

	if resourceID, ok := spaces.ParseResourceID(fileID); ok {
		return provider.Reference{ResourceId: resourceID}, nil
	}

	if resourceID := resourceid.OwnCloudResourceIDUnwrap(fileID); resourceID != nil {
		return provider.Reference{ResourceId: resourceID}, nil
	}

	if resourceID, err := spaces.ResourceIdFromString(fileID); err == nil {
		return provider.Reference{ResourceId: resourceID}, nil
	}

	return provider.Reference{}, fmt.Errorf("invalid file_id")
}

func validateDocumentURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", nil
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid document_url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid document_url")
	}
	return rawURL, nil
}

type resolvedMentionRecipients struct {
	users    []*userpb.User
	accepted []mentionResult
	rejected []mentionResult
	seen     map[string]struct{}
}

func resolveMentionRecipients(ctx context.Context, client gateway.GatewayAPIClient, mentions []mentionTarget, author *userpb.User) resolvedMentionRecipients {
	resolved := resolvedMentionRecipients{
		seen: make(map[string]struct{}),
	}
	authorKey := userKey(author)

	for _, mention := range mentions {
		switch mention.Type {
		case "user":
			u, reason := getUserByUsername(ctx, client, mention.Username)
			if reason != "" {
				resolved.rejected = append(resolved.rejected, mentionResult{Type: "user", Username: mention.Username, Reason: reason})
				continue
			}
			resolved.addUser(u, authorKey, mentionResult{Type: "user", Username: mention.Username})
		case "group":
			groupRes, err := client.GetGroupByClaim(ctx, &grouppb.GetGroupByClaimRequest{
				Claim:               "group_name",
				Value:               mention.GroupName,
				SkipFetchingMembers: false,
			})
			if err != nil || groupRes.Status == nil || groupRes.Status.Code != rpc.Code_CODE_OK || groupRes.Group == nil {
				resolved.rejected = append(resolved.rejected, mentionResult{Type: "group", GroupName: mention.GroupName, Reason: "group_not_found"})
				continue
			}
			if len(groupRes.Group.Members) == 0 {
				resolved.rejected = append(resolved.rejected, mentionResult{Type: "group", GroupName: mention.GroupName, Reason: "group_has_no_members"})
				continue
			}

			for _, memberID := range groupRes.Group.Members {
				userRes, err := client.GetUser(ctx, &userpb.GetUserRequest{
					UserId:                 memberID,
					SkipFetchingUserGroups: true,
				})
				if err != nil || userRes.Status == nil || userRes.Status.Code != rpc.Code_CODE_OK || userRes.User == nil {
					resolved.rejected = append(resolved.rejected, mentionResult{Type: "group", GroupName: mention.GroupName, Reason: "group_member_not_found"})
					continue
				}
				resolved.addUser(userRes.User, authorKey, mentionResult{Type: "group", GroupName: mention.GroupName})
			}
		}
	}

	return resolved
}

func (r *resolvedMentionRecipients) addUser(u *userpb.User, authorKey string, source mentionResult) {
	if u.Mail == "" {
		source.Reason = "user_has_no_email"
		if source.Username == "" {
			source.Username = u.Username
		}
		r.rejected = append(r.rejected, source)
		return
	}

	key := userKey(u)
	if key == "" {
		key = u.Mail
	}
	if key == authorKey {
		return
	}
	if _, ok := r.seen[key]; ok {
		return
	}
	r.seen[key] = struct{}{}
	r.users = append(r.users, u)
}

func getUserByUsername(ctx context.Context, client gateway.GatewayAPIClient, username string) (*userpb.User, string) {
	userRes, err := client.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim:                  "username",
		Value:                  username,
		SkipFetchingUserGroups: true,
	})
	if err != nil || userRes.Status == nil || userRes.Status.Code != rpc.Code_CODE_OK || userRes.User == nil {
		return nil, "user_not_found"
	}
	return userRes.User, ""
}

func userKey(u *userpb.User) string {
	if u == nil {
		return ""
	}
	if u.Id != nil {
		return u.Id.Idp + ":" + u.Id.OpaqueId
	}
	if u.Username != "" {
		return "username:" + u.Username
	}
	return "mail:" + u.Mail
}

func documentIDForNotification(info *provider.ResourceInfo, ref *provider.Reference) string {
	if info != nil && info.Id != nil {
		return spaces.EncodeToStringifiedResourceID(info.Id)
	}
	if ref != nil && ref.ResourceId != nil {
		return spaces.EncodeToStringifiedResourceID(ref.ResourceId)
	}
	if ref != nil {
		return ref.Path
	}
	return ""
}
