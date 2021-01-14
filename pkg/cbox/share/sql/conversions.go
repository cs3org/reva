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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

func granteeTypeToInt(g provider.GranteeType) int {
	switch g {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		return 0
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		return 1
	default:
		return -1
	}
}

func intToGranteeType(g int) provider.GranteeType {
	switch g {
	case 0:
		return provider.GranteeType_GRANTEE_TYPE_USER
	case 1:
		return provider.GranteeType_GRANTEE_TYPE_GROUP
	default:
		return provider.GranteeType_GRANTEE_TYPE_INVALID
	}
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
		Grantee:     &provider.Grantee{Type: intToGranteeType(s.ShareType), Id: extractUserID(s.ShareWith)},
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
