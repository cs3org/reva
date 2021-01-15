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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
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
	// ShareTypeGroup ShareType = 1

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
	// ?
	AdditionalInfoOwner string `json:"additional_info_owner" xml:"additional_info_owner"`
	// ?
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
	// The uid of the receiver of the file. This is either
	// - a GID (group id) if it is being shared with a group or
	// - a UID (user id) if the share is shared with a user.
	ShareWith string `json:"share_with,omitempty" xml:"share_with,omitempty"`
	// The display name of the receiver of the file.
	ShareWithDisplayname string `json:"share_with_displayname,omitempty" xml:"share_with_displayname,omitempty"`
	// sharee Additional info
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
	ShareType int    `json:"shareType" xml:"shareType"`
	ShareWith string `json:"shareWith" xml:"shareWith"`
}

// UserShare2ShareData converts a cs3api user share into shareData data model
// TODO(jfd) merge userShare2ShareData with publicShare2ShareData
func UserShare2ShareData(ctx context.Context, share *collaboration.Share) (*ShareData, error) {
	sd := &ShareData{
		Permissions:  UserSharePermissions2OCSPermissions(share.GetPermissions()),
		ShareType:    ShareTypeUser,
		UIDOwner:     LocalUserIDToString(share.GetCreator()),
		UIDFileOwner: LocalUserIDToString(share.GetOwner()),
		ShareWith:    LocalUserIDToString(share.GetGrantee().GetId()),
	}

	if share.Id != nil && share.Id.OpaqueId != "" {
		sd.ID = share.Id.OpaqueId
	}
	if share.Ctime != nil {
		sd.STime = share.Ctime.Seconds // TODO CS3 api birth time = btime
	}
	// actually clients should be able to GET and cache the user info themselves ...
	// TODO only return the userid, let the clientso look up the displayname
	// TODO check grantee type for user vs group
	return sd, nil
}

// PublicShare2ShareData converts a cs3api public share into shareData data model
func PublicShare2ShareData(share *link.PublicShare, r *http.Request, publicURL string) *ShareData {
	var expiration string
	if share.Expiration != nil {
		expiration = timestampToExpiration(share.Expiration)
	} else {
		expiration = ""
	}

	sd := &ShareData{
		// share.permissions are mapped below
		// Displaynames are added later
		ID:           share.Id.OpaqueId,
		ShareType:    ShareTypePublicLink,
		STime:        share.Ctime.Seconds, // TODO CS3 api birth time = btime
		Token:        share.Token,
		Expiration:   expiration,
		MimeType:     share.Mtime.String(),
		Name:         share.DisplayName,
		MailSend:     0,
		URL:          publicURL + path.Join("/", "#/s/"+share.Token),
		Permissions:  publicSharePermissions2OCSPermissions(share.GetPermissions()),
		UIDOwner:     LocalUserIDToString(share.Creator),
		UIDFileOwner: LocalUserIDToString(share.Owner),
	}

	if share.PasswordProtected {
		sd.ShareWith = "***redacted***"
		sd.ShareWithDisplayname = "***redacted***"
	}

	return sd
	// actually clients should be able to GET and cache the user info themselves ...
	// TODO check grantee type for user vs group
}

// LocalUserIDToString transforms a cs3api user id into an ocs data model without domain name
func LocalUserIDToString(userID *userpb.UserId) string {
	if userID == nil || userID.OpaqueId == "" {
		return ""
	}
	return userID.OpaqueId
}

// UserIDToString transforms a cs3api user id into an ocs data model
// TODO This should be used instead of LocalUserIDToString bit it requires interpreting an @ on the client side
// TODO An alternative would be to send the idp / iss as an additional attribute. might be less intrusive
func UserIDToString(userID *userpb.UserId) string {
	if userID == nil || userID.OpaqueId == "" {
		return ""
	}
	if userID.Idp == "" {
		return userID.OpaqueId
	}
	return userID.OpaqueId + "@" + userID.Idp
}

// UserSharePermissions2OCSPermissions transforms cs3api permissions into OCS Permissions data model
func UserSharePermissions2OCSPermissions(sp *collaboration.SharePermissions) Permissions {
	if sp != nil {
		return RoleFromResourcePermissions(sp.GetPermissions()).OCSPermissions()
	}
	return PermissionInvalid
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

func publicSharePermissions2OCSPermissions(sp *link.PublicSharePermissions) Permissions {
	if sp != nil {
		return RoleFromResourcePermissions(sp.GetPermissions()).OCSPermissions()
	}
	return PermissionInvalid
}

// timestamp is assumed to be UTC ... just human readable ...
// FIXME and ambiguous / error prone because there is no time zone ...
func timestampToExpiration(t *types.Timestamp) string {
	return time.Unix(int64(t.Seconds), int64(t.Nanos)).UTC().Format("2006-01-02 15:05:05")
}
