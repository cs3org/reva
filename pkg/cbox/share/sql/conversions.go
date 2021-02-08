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
	"fmt"
	"strings"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/utils"
)

func formatGrantee(g *provider.Grantee) (int, string) {
	var granteeType int
	var formattedID string
	uid, gid := utils.ExtractGranteeID(g)
	switch g.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		granteeType = 0
		formattedID = formatUserID(uid)
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		granteeType = 1
		formattedID = gid.OpaqueId
	default:
		granteeType = -1
	}
	return granteeType, formattedID
}

func extractGrantee(t int, g string) *provider.Grantee {
	var grantee *provider.Grantee
	switch t {
	case 0:
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_USER
		grantee.Id = &provider.Grantee_UserId{UserId: extractUserID(g)}
	case 1:
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_GROUP
		grantee.Id = &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: g}}
	default:
		grantee.Type = provider.GranteeType_GRANTEE_TYPE_INVALID
	}
	return grantee
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

func intTosharePerm(p int) *provider.ResourcePermissions {
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

func intToShareState(g int) collaboration.ShareState {
	switch g {
	case 0:
		return collaboration.ShareState_SHARE_STATE_PENDING
	case 1:
		return collaboration.ShareState_SHARE_STATE_ACCEPTED
	default:
		return collaboration.ShareState_SHARE_STATE_INVALID
	}
}

func formatUserID(u *userpb.UserId) string {
	if u.Idp != "" {
		return fmt.Sprintf("%s:%s", u.OpaqueId, u.Idp)
	}
	return u.OpaqueId
}

func extractUserID(u string) *userpb.UserId {
	parts := strings.Split(u, ":")
	if len(parts) > 1 {
		return &userpb.UserId{OpaqueId: parts[0], Idp: parts[1]}
	}
	return &userpb.UserId{OpaqueId: parts[0]}
}

func convertToCS3Share(s dbShare) *collaboration.Share {
	ts := &typespb.Timestamp{
		Seconds: uint64(s.STime),
	}
	return &collaboration.Share{
		Id: &collaboration.ShareId{
			OpaqueId: s.ID,
		},
		ResourceId:  &provider.ResourceId{OpaqueId: s.ItemSource, StorageId: s.Prefix},
		Permissions: &collaboration.SharePermissions{Permissions: intTosharePerm(s.Permissions)},
		Grantee:     extractGrantee(s.ShareType, s.ShareWith),
		Owner:       extractUserID(s.UIDOwner),
		Creator:     extractUserID(s.UIDInitiator),
		Ctime:       ts,
		Mtime:       ts,
	}
}

func convertToCS3ReceivedShare(s dbShare) *collaboration.ReceivedShare {
	share := convertToCS3Share(s)
	return &collaboration.ReceivedShare{
		Share: share,
		State: intToShareState(s.State),
	}
}
