// Copyright 2018-2021 CERN
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

// Package conversions sits between CS3 type definitions and OCS API Responses
package conversions

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"time"

	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/user"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	publicsharemgr "github.com/cs3org/reva/pkg/publicshare/manager/registry"
	usermgr "github.com/cs3org/reva/pkg/user/manager/registry"
)

const (
	// ShareTypeUser refers to user shares
	ShareTypeUser ShareType = 0

	// ShareTypePublicLink refers to public link shares
	ShareTypePublicLink ShareType = 3

	// ShareTypeGroup represents a group share
	ShareTypeGroup ShareType = 1

	// ShareTypeFederatedCloudShare represents a federated share
	ShareTypeFederatedCloudShare ShareType = 6
)

// ResourceType indicates the OCS type of the resource
type ResourceType int

func (rt ResourceType) String() (s string) {
	switch rt {
	case 0:
		s = "invalid"
	case 1:
		s = "file"
	case 2:
		s = "folder"
	case 3:
		s = "reference"
	default:
		s = "invalid"
	}
	return
}

// ShareType denotes a type of share
type ShareType int

// ShareData represents https://doc.owncloud.com/server/developer_manual/core/ocs-share-api.html#response-attributes-1
type ShareData struct {
	// TODO int?
	ID string `json:"id" xml:"id"`
	// The share’s type
	ShareType ShareType `json:"share_type" xml:"share_type"`
	// The username of the owner of the share.
	UIDOwner string `json:"uid_owner" xml:"uid_owner"`
	// The display name of the owner of the share.
	DisplaynameOwner string `json:"displayname_owner" xml:"displayname_owner"`
	// Additional info to identify the share owner, eg. the email or username
	AdditionalInfoOwner string `json:"additional_info_owner" xml:"additional_info_owner"`
	// The permission attribute set on the file.
	// TODO(jfd) change the default to read only
	Permissions Permissions `json:"permissions" xml:"permissions"`
	// The UNIX timestamp when the share was created.
	STime uint64 `json:"stime" xml:"stime"`
	// ?
	Parent string `json:"parent" xml:"parent"`
	// The UNIX timestamp when the share expires.
	Expiration string `json:"expiration" xml:"expiration"`
	// The public link to the item being shared.
	Token string `json:"token" xml:"token"`
	// The unique id of the user that owns the file or folder being shared.
	UIDFileOwner string `json:"uid_file_owner" xml:"uid_file_owner"`
	// The display name of the user that owns the file or folder being shared.
	DisplaynameFileOwner string `json:"displayname_file_owner" xml:"displayname_file_owner"`
	// Additional info to identify the file owner, eg. the email or username
	AdditionalInfoFileOwner string `json:"additional_info_file_owner" xml:"additional_info_file_owner"`
	// share state, 0 = accepted, 1 = pending, 2 = declined
	State int `json:"state" xml:"state"`
	// The path to the shared file or folder.
	Path string `json:"path" xml:"path"`
	// The type of the object being shared. This can be one of 'file' or 'folder'.
	ItemType string `json:"item_type" xml:"item_type"`
	// The RFC2045-compliant mimetype of the file.
	MimeType  string `json:"mimetype" xml:"mimetype"`
	StorageID string `json:"storage_id" xml:"storage_id"`
	Storage   uint64 `json:"storage" xml:"storage"`
	// The unique node id of the item being shared.
	ItemSource string `json:"item_source" xml:"item_source"`
	// The unique node id of the item being shared. For legacy reasons item_source and file_source attributes have the same value.
	FileSource string `json:"file_source" xml:"file_source"`
	// The unique node id of the parent node of the item being shared.
	FileParent string `json:"file_parent" xml:"file_parent"`
	// The basename of the shared file.
	FileTarget string `json:"file_target" xml:"file_target"`
	// The uid of the share recipient. This is either
	// - a GID (group id) if it is being shared with a group or
	// - a UID (user id) if the share is shared with a user.
	// - a password for public links
	ShareWith string `json:"share_with,omitempty" xml:"share_with,omitempty"`
	// The display name of the share recipient
	ShareWithDisplayname string `json:"share_with_displayname,omitempty" xml:"share_with_displayname,omitempty"`
	// Additional info to identify the share recipient, eg. the email or username
	ShareWithAdditionalInfo string `json:"share_with_additional_info" xml:"share_with_additional_info"`
	// Whether the recipient was notified, by mail, about the share being shared with them.
	MailSend int `json:"mail_send" xml:"mail_send"`
	// Name of the public share
	Name string `json:"name" xml:"name"`
	// URL of the public share
	URL string `json:"url,omitempty" xml:"url,omitempty"`
	// Attributes associated
	Attributes string `json:"attributes,omitempty" xml:"attributes,omitempty"`
	// PasswordProtected represents a public share is password protected
	// PasswordProtected bool `json:"password_protected,omitempty" xml:"password_protected,omitempty"`
}

// ShareeData holds share recipient search results
type ShareeData struct {
	Exact   *ExactMatchesData `json:"exact" xml:"exact"`
	Users   []*MatchData      `json:"users" xml:"users"`
	Groups  []*MatchData      `json:"groups" xml:"groups"`
	Remotes []*MatchData      `json:"remotes" xml:"remotes"`
}

// ExactMatchesData hold exact matches
type ExactMatchesData struct {
	Users   []*MatchData `json:"users" xml:"users"`
	Groups  []*MatchData `json:"groups" xml:"groups"`
	Remotes []*MatchData `json:"remotes" xml:"remotes"`
}

// MatchData describes a single match
type MatchData struct {
	Label string          `json:"label" xml:"label"`
	Value *MatchValueData `json:"value" xml:"value"`
}

// MatchValueData holds the type and actual value
type MatchValueData struct {
	ShareType               int    `json:"shareType" xml:"shareType"`
	ShareWith               string `json:"shareWith" xml:"shareWith"`
	ShareWithAdditionalInfo string `json:"shareWithAdditionalInfo" xml:"shareWithAdditionalInfo"`
}

// CS3Share2ShareData converts a cs3api user share into shareData data model
func CS3Share2ShareData(ctx context.Context, share *collaboration.Share) (*ShareData, error) {
	sd := &ShareData{
		// share.permissions are mapped below
		// Displaynames are added later
		UIDOwner:     LocalUserIDToString(share.GetCreator()),
		UIDFileOwner: LocalUserIDToString(share.GetOwner()),
	}

	if share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER {
		sd.ShareType = ShareTypeUser
		sd.ShareWith = LocalUserIDToString(share.Grantee.GetUserId())
	} else if share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		sd.ShareType = ShareTypeGroup
		sd.ShareWith = LocalGroupIDToString(share.Grantee.GetGroupId())
	}

	if share.Id != nil {
		sd.ID = share.Id.OpaqueId
	}
	if share.GetPermissions() != nil && share.GetPermissions().GetPermissions() != nil {
		sd.Permissions = RoleFromResourcePermissions(share.GetPermissions().GetPermissions()).OCSPermissions()
	}
	if share.Ctime != nil {
		sd.STime = share.Ctime.Seconds // TODO CS3 api birth time = btime
	}
	return sd, nil
}

// PublicShare2ShareData converts a cs3api public share into shareData data model
func PublicShare2ShareData(share *link.PublicShare, r *http.Request, publicURL string) *ShareData {
	sd := &ShareData{
		// share.permissions are mapped below
		// Displaynames are added later
		ShareType:    ShareTypePublicLink,
		Token:        share.Token,
		Name:         share.DisplayName,
		MailSend:     0,
		URL:          publicURL + path.Join("/", "#/s/"+share.Token),
		UIDOwner:     LocalUserIDToString(share.Creator),
		UIDFileOwner: LocalUserIDToString(share.Owner),
	}
	if share.Id != nil {
		sd.ID = share.Id.OpaqueId
	}
	if share.GetPermissions() != nil && share.GetPermissions().GetPermissions() != nil {
		sd.Permissions = RoleFromResourcePermissions(share.GetPermissions().GetPermissions()).OCSPermissions()
	}
	if share.Expiration != nil {
		sd.Expiration = timestampToExpiration(share.Expiration)
	}
	if share.Ctime != nil {
		sd.STime = share.Ctime.Seconds // TODO CS3 api birth time = btime
	}

	// hide password
	if share.PasswordProtected {
		sd.ShareWith = "***redacted***"
		sd.ShareWithDisplayname = "***redacted***"
	}

	return sd
}

// LocalUserIDToString transforms a cs3api user id into an ocs data model without domain name
// TODO ocs uses user names ... so an additional lookup is needed. see mapUserIds()
func LocalUserIDToString(userID *userpb.UserId) string {
	if userID == nil || userID.OpaqueId == "" {
		return ""
	}
	return userID.OpaqueId
}

// LocalGroupIDToString transforms a cs3api group id into an ocs data model without domain name
func LocalGroupIDToString(groupID *grouppb.GroupId) string {
	if groupID == nil || groupID.OpaqueId == "" {
		return ""
	}
	return groupID.OpaqueId
}

// GetUserManager returns a connection to a user share manager
func GetUserManager(manager string, m map[string]map[string]interface{}) (user.Manager, error) {
	if f, ok := usermgr.NewFuncs[manager]; ok {
		return f(m[manager])
	}

	return nil, fmt.Errorf("driver %s not found for user manager", manager)
}

// GetPublicShareManager returns a connection to a public share manager
func GetPublicShareManager(manager string, m map[string]map[string]interface{}) (publicshare.Manager, error) {
	if f, ok := publicsharemgr.NewFuncs[manager]; ok {
		return f(m[manager])
	}

	return nil, fmt.Errorf("driver %s not found for public shares manager", manager)
}

// timestamp is assumed to be UTC ... just human readable ...
// FIXME and ambiguous / error prone because there is no time zone ...
func timestampToExpiration(t *types.Timestamp) string {
	return time.Unix(int64(t.Seconds), int64(t.Nanos)).UTC().Format("2006-01-02 15:05:05")
}

// ParseTimestamp tries to parses the ocs expiry into a CS3 Timestamp
func ParseTimestamp(timestampString string) (*types.Timestamp, error) {
	parsedTime, err := time.Parse("2006-01-02T15:04:05Z0700", timestampString)
	if err != nil {
		parsedTime, err = time.Parse("2006-01-02", timestampString)
	}
	if err != nil {
		return nil, fmt.Errorf("datetime format invalid: %v", timestampString)
	}
	final := parsedTime.UnixNano()

	return &types.Timestamp{
		Seconds: uint64(final / 1000000000),
		Nanos:   uint32(final % 1000000000),
	}, nil
}
