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

package sharehierarchy

import (
	"encoding/json"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

const (
	ShareeTypeUser  = "user"
	ShareeTypeGroup = "group"
)

// ConflictingShare identifies a share involved in a hierarchy conflict.
type ConflictingShare struct {
	ID             string `json:"id,omitempty"`
	ResourceID     string `json:"resource_id,omitempty"`
	Path           string `json:"path,omitempty"`
	PermissionType string `json:"permission_type,omitempty"`
	Sharee         string `json:"sharee,omitempty"`
	ShareeType     string `json:"sharee_type,omitempty"`
}

// HierarchyConflictError is the structured payload returned when a share operation is
// rejected due to hierarchy inconsistency. It is JSON-encoded into the gRPC
// Status.Message field so the HTTP layer can decode it and return a 409 with detail.
type HierarchyConflictError struct {
	// ErrorType is either "parent_conflict" or "child_conflict".
	ErrorType string `json:"error_type"`
	Message   string `json:"message"`
	// CanForce indicates whether the caller may retry with force=true to override the conflict.
	CanForce          bool               `json:"can_force"`
	ConflictingShares []ConflictingShare `json:"conflicting_shares,omitempty"`
}

func (e *HierarchyConflictError) Error() string { return e.Message }

// NewChildConflictError builds a HierarchyConflictError for a would-delete-children situation.
func NewChildConflictError(msg string, shares []ResolvedShare) *HierarchyConflictError {
	cs := make([]ConflictingShare, 0, len(shares))
	for _, resolved := range shares {
		s := resolved.Share
		sharee, shareeType := shareeInfo(s.Grantee)

		cs = append(cs, ConflictingShare{
			ID:             s.Id.OpaqueId,
			ResourceID:     s.ResourceId.StorageId + "!" + s.ResourceId.OpaqueId,
			Sharee:         sharee,
			ShareeType:     shareeType,
			PermissionType: PermLevelFromCS3(s.Permissions.GetPermissions()).RoleID(),
			Path:           resolved.Path,
		})
	}
	return &HierarchyConflictError{ErrorType: "child_conflict", CanForce: true, Message: msg, ConflictingShares: cs}
}

func shareeInfo(grantee *provider.Grantee) (string, string) {
	if grantee == nil {
		return "", ""
	}
	if userID := grantee.GetUserId(); userID != nil {
		return userID.OpaqueId, ShareeTypeUser
	}
	if groupID := grantee.GetGroupId(); groupID != nil {
		return groupID.OpaqueId, ShareeTypeGroup
	}
	return "", ""
}

// MarshalToJSON serialises the error to a JSON string suitable for embedding in a gRPC Status message.
func (e *HierarchyConflictError) MarshalToJSON() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// UnmarshalHierarchyConflictError tries to decode a gRPC Status message as a HierarchyConflictError.
// Returns nil if the message is not a valid HierarchyConflictError.
func UnmarshalHierarchyConflictError(msg string) *HierarchyConflictError {
	var e HierarchyConflictError
	if json.Unmarshal([]byte(msg), &e) == nil && e.ErrorType != "" {
		return &e
	}
	return nil
}
