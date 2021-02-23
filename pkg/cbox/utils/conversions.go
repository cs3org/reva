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

package utils

import (
	"fmt"
	"strings"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

type DBShare struct {
	ID           string
	UIDOwner     string
	UIDInitiator string
	Prefix       string
	ItemSource   string
	ShareWith    string
	Token        string
	Expiration   string
	Permissions  int
	ShareType    int
	ShareName    string
	STime        int
	FileTarget   string
	State        int
}

func FormatGrantee(g *provider.Grantee) (int, string) {
	var granteeType int
	var formattedID string
	switch g.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		granteeType = 0
		formattedID = FormatUserID(g.GetUserId())
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		granteeType = 1
		formattedID = FormatGroupID(g.GetGroupId())
	default:
		granteeType = -1
	}
	return granteeType, formattedID
}

func ExtractGrantee(t int, g string) *provider.Grantee {
	var grantee *provider.Grantee
	switch t {
	case 0:
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_USER
		grantee.Id = &provider.Grantee_UserId{UserId: ExtractUserID(g)}
	case 1:
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_GROUP
		grantee.Id = &provider.Grantee_GroupId{GroupId: ExtractGroupID(g)}
	default:
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_INVALID
	}
	return grantee
}

func ResourceTypeToItem(r provider.ResourceType) string {
	switch r {
	case provider.ResourceType_RESOURCE_TYPE_FILE:
		return "file"
	case provider.ResourceType_RESOURCE_TYPE_CONTAINER:
		return "folder"
	case provider.ResourceType_RESOURCE_TYPE_REFERENCE:
		return "reference"
	case provider.ResourceType_RESOURCE_TYPE_SYMLINK:
		return "symlink"
	default:
		return ""
	}
}

func SharePermToInt(p *provider.ResourcePermissions) int {
	var perm int
	if p.CreateContainer {
		perm = 15
	} else if p.ListContainer {
		perm = 1
	}
	return perm
}

func IntTosharePerm(p int) *provider.ResourcePermissions {
	switch p {
	case 1:
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
		}
	case 15:
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,

			Move:               true,
			InitiateFileUpload: true,
			RestoreFileVersion: true,
			RestoreRecycleItem: true,
			CreateContainer:    true,
			Delete:             true,
			PurgeRecycle:       true,
		}
	default:
		return &provider.ResourcePermissions{}
	}
}

func IntToShareState(g int) collaboration.ShareState {
	switch g {
	case 0:
		return collaboration.ShareState_SHARE_STATE_PENDING
	case 1:
		return collaboration.ShareState_SHARE_STATE_ACCEPTED
	default:
		return collaboration.ShareState_SHARE_STATE_INVALID
	}
}

func FormatUserID(u *userpb.UserId) string {
	if u.Idp != "" {
		return fmt.Sprintf("%s:%s", u.OpaqueId, u.Idp)
	}
	return u.OpaqueId
}

func ExtractUserID(u string) *userpb.UserId {
	parts := strings.Split(u, ":")
	if len(parts) > 1 {
		return &userpb.UserId{OpaqueId: parts[0], Idp: parts[1]}
	}
	return &userpb.UserId{OpaqueId: parts[0]}
}

func FormatGroupID(u *grouppb.GroupId) string {
	if u.Idp != "" {
		return fmt.Sprintf("%s:%s", u.OpaqueId, u.Idp)
	}
	return u.OpaqueId
}

func ExtractGroupID(u string) *grouppb.GroupId {
	parts := strings.Split(u, ":")
	if len(parts) > 1 {
		return &grouppb.GroupId{OpaqueId: parts[0], Idp: parts[1]}
	}
	return &grouppb.GroupId{OpaqueId: parts[0]}
}

func ConvertToCS3Share(s DBShare) *collaboration.Share {
	ts := &typespb.Timestamp{
		Seconds: uint64(s.STime),
	}
	return &collaboration.Share{
		Id: &collaboration.ShareId{
			OpaqueId: s.ID,
		},
		ResourceId:  &provider.ResourceId{OpaqueId: s.ItemSource, StorageId: s.Prefix},
		Permissions: &collaboration.SharePermissions{Permissions: IntTosharePerm(s.Permissions)},
		Grantee:     ExtractGrantee(s.ShareType, s.ShareWith),
		Owner:       ExtractUserID(s.UIDOwner),
		Creator:     ExtractUserID(s.UIDInitiator),
		Ctime:       ts,
		Mtime:       ts,
	}
}

func ConvertToCS3ReceivedShare(s DBShare) *collaboration.ReceivedShare {
	share := ConvertToCS3Share(s)
	return &collaboration.ReceivedShare{
		Share: share,
		State: IntToShareState(s.State),
	}
}

func ConvertToCS3PublicShare(s DBShare) *link.PublicShare {
	ts := &typespb.Timestamp{
		Seconds: uint64(s.STime),
	}
	pwd := false
	if s.ShareWith != "" {
		pwd = true
	}
	var expires uint64
	if s.Expiration != "" {
		t, err := time.Parse("2006-01-02 03:04:05", s.Expiration)
		if err == nil {
			expires = uint64(t.Unix())
		}
	}
	return &link.PublicShare{
		Id: &link.PublicShareId{
			OpaqueId: s.ID,
		},
		ResourceId:        &provider.ResourceId{OpaqueId: s.ItemSource, StorageId: s.Prefix},
		Permissions:       &link.PublicSharePermissions{Permissions: IntTosharePerm(s.Permissions)},
		Owner:             ExtractUserID(s.UIDOwner),
		Creator:           ExtractUserID(s.UIDInitiator),
		Token:             s.Token,
		DisplayName:       s.ShareName,
		PasswordProtected: pwd,
		Expiration: &typespb.Timestamp{
			Seconds: expires,
		},
		Ctime: ts,
		Mtime: ts,
	}
}
