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

package sql

import (
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	conversions "github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
)

// DBShare stores information about user and public shares.
type DBShare struct {
	ID           string
	UIDOwner     string
	UIDInitiator string
	ItemStorage  string
	ItemSource   string
	ShareWith    string
	Token        string
	Expiration   string
	Permissions  int
	ShareType    int
	ShareName    string
	STime        int
	FileTarget   string
	RejectedBy   string
	State        int
}

func formatGrantee(g *provider.Grantee) (int, string) {
	var granteeType int
	var formattedID string
	switch g.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		granteeType = 0
		formattedID = formatUserID(g.GetUserId())
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		granteeType = 1
		formattedID = formatGroupID(g.GetGroupId())
	default:
		granteeType = -1
	}
	return granteeType, formattedID
}

func extractGrantee(t int, g string) *provider.Grantee {
	var grantee provider.Grantee
	switch t {
	case 0:
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_USER
		grantee.Id = &provider.Grantee_UserId{UserId: extractUserID(g)}
	case 1:
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_GROUP
		grantee.Id = &provider.Grantee_GroupId{GroupId: extractGroupID(g)}
	default:
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_INVALID
	}
	return &grantee
}

func resourceTypeToItem(r provider.ResourceType) string {
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

func sharePermToInt(p *provider.ResourcePermissions) int {
	var perm int
	if p.CreateContainer {
		perm = 15
	} else if p.ListContainer {
		perm = 1
	}
	return perm
}

func intTosharePerm(p int) (*provider.ResourcePermissions, error) {
	perms, err := conversions.NewPermissions(p)
	if err != nil {
		return nil, err
	}

	return conversions.RoleFromOCSPermissions(perms).CS3ResourcePermissions(), nil
}

func intToShareState(g int) collaboration.ShareState {
	switch g {
	case 0:
		return collaboration.ShareState_SHARE_STATE_ACCEPTED
	case 1:
		return collaboration.ShareState_SHARE_STATE_PENDING
	default:
		return collaboration.ShareState_SHARE_STATE_INVALID
	}
}

func formatUserID(u *userpb.UserId) string {
	return u.OpaqueId
}

func extractUserID(u string) *userpb.UserId {
	return &userpb.UserId{OpaqueId: u}
}

func formatGroupID(u *grouppb.GroupId) string {
	return u.OpaqueId
}

func extractGroupID(u string) *grouppb.GroupId {
	return &grouppb.GroupId{OpaqueId: u}
}

func convertToCS3Share(s DBShare, storageMountID string) (*collaboration.Share, error) {
	ts := &typespb.Timestamp{
		Seconds: uint64(s.STime),
	}
	permissions, err := intTosharePerm(s.Permissions)
	if err != nil {
		return nil, err
	}
	return &collaboration.Share{
		Id: &collaboration.ShareId{
			OpaqueId: s.ID,
		},
		ResourceId: &provider.ResourceId{
			StorageId: storageMountID + "!" + s.ItemStorage,
			OpaqueId:  s.ItemSource,
		},
		Permissions: &collaboration.SharePermissions{Permissions: permissions},
		Grantee:     extractGrantee(s.ShareType, s.ShareWith),
		Owner:       extractUserID(s.UIDOwner),
		Creator:     extractUserID(s.UIDInitiator),
		Ctime:       ts,
		Mtime:       ts,
	}, nil
}

func convertToCS3ReceivedShare(s DBShare, storageMountID string) (*collaboration.ReceivedShare, error) {
	share, err := convertToCS3Share(s, storageMountID)
	if err != nil {
		return nil, err
	}
	var state collaboration.ShareState
	if s.RejectedBy != "" {
		state = collaboration.ShareState_SHARE_STATE_REJECTED
	} else {
		state = intToShareState(s.State)
	}
	return &collaboration.ReceivedShare{
		Share: share,
		State: state,
	}, nil
}

func convertToCS3PublicShare(s DBShare, storageMountID string) (*link.PublicShare, error) {
	ts := &typespb.Timestamp{
		Seconds: uint64(s.STime),
	}
	pwd := false
	if s.ShareWith != "" {
		pwd = true
	}
	var expires *typespb.Timestamp
	if s.Expiration != "" {
		t, err := time.Parse("2006-01-02 03:04:05", s.Expiration)
		if err == nil {
			expires = &typespb.Timestamp{
				Seconds: uint64(t.Unix()),
			}
		}
	}
	permissions, err := intTosharePerm(s.Permissions)
	if err != nil {
		return nil, err
	}
	return &link.PublicShare{
		Id: &link.PublicShareId{
			OpaqueId: s.ID,
		},
		ResourceId: &provider.ResourceId{
			StorageId: storageMountID + "!" + s.ItemStorage,
			OpaqueId:  s.ItemSource,
		},
		Permissions:       &link.PublicSharePermissions{Permissions: permissions},
		Owner:             extractUserID(s.UIDOwner),
		Creator:           extractUserID(s.UIDInitiator),
		Token:             s.Token,
		DisplayName:       s.ShareName,
		PasswordProtected: pwd,
		Expiration:        expires,
		Ctime:             ts,
		Mtime:             ts,
	}, err
}
