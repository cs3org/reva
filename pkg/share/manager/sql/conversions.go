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

package sql

import (
	"encoding/json"
	"strconv"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"gorm.io/datatypes"
)

func convertFromCS3OCMShareType(shareType ocm.RecipientType) model.OcmShareType {
	switch shareType {
	case ocm.RecipientType_RECIPIENT_TYPE_USER:
		return model.OcmShareTypeUser
	case ocm.RecipientType_RECIPIENT_TYPE_GROUP:
		return model.OcmShareTypeGroup
	}
	return -1
}

func convertToCS3OCMShareType(recipientType model.OcmShareType) ocm.ShareType {
	switch recipientType {
	case model.OcmShareTypeUser:
		return ocm.ShareType_SHARE_TYPE_USER
	case model.OcmShareTypeGroup:
		return ocm.ShareType_SHARE_TYPE_GROUP
	}
	return ocm.ShareType_SHARE_TYPE_INVALID
}

func convertFromCS3OCMShareState(shareState ocm.ShareState) model.OcmShareState {
	switch shareState {
	case ocm.ShareState_SHARE_STATE_ACCEPTED:
		return model.OcmShareStateAccepted
	case ocm.ShareState_SHARE_STATE_PENDING:
		return model.OcmShareStatePending
	case ocm.ShareState_SHARE_STATE_REJECTED:
		return model.OcmShareStateRejected
	case ocm.ShareState_SHARE_STATE_TRANSFERRING:
		return model.OcmShareStateTransferring
	}
	return -1
}

func convertToCS3OCMShareState(state model.OcmShareState) ocm.ShareState {
	switch state {
	case model.OcmShareStateAccepted:
		return ocm.ShareState_SHARE_STATE_ACCEPTED
	case model.OcmShareStatePending:
		return ocm.ShareState_SHARE_STATE_PENDING
	case model.OcmShareStateRejected:
		return ocm.ShareState_SHARE_STATE_REJECTED
	case model.OcmShareStateTransferring:
		return ocm.ShareState_SHARE_STATE_TRANSFERRING
	}
	return ocm.ShareState_SHARE_STATE_INVALID
}

func convertToCS3OCMShare(s *model.OcmShare, am []*ocm.AccessMethod) *ocm.Share {
	granteeUserId, _ := ocmd.GetUserIdFromOCMAddress(s.ShareWith)
	share := &ocm.Share{
		Id: &ocm.ShareId{
			OpaqueId: strconv.Itoa(int(s.Id)),
		},
		ResourceId: &provider.ResourceId{
			StorageId: s.Instance,
			OpaqueId:  s.Inode,
		},
		Name:  s.Name,
		Token: s.Token,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: granteeUserId,
			},
		},
		Owner: &userpb.UserId{
			OpaqueId: s.Owner,
		},
		Creator: &userpb.UserId{
			OpaqueId: s.Initiator,
		},
		Ctime: &types.Timestamp{
			Seconds: uint64(s.Ctime),
		},
		Mtime: &types.Timestamp{
			Seconds: uint64(s.Mtime),
		},
		ShareType:     convertToCS3OCMShareType(s.RecipientType),
		AccessMethods: am,
	}
	if s.Expiration.Valid {
		share.Expiration = &types.Timestamp{
			Seconds: uint64(s.Expiration.V.Unix()),
		}
	}
	return share
}

func convertToCS3OCMReceivedShare(s *model.OcmReceivedShare, p []*ocm.Protocol) *ocm.ReceivedShare {
	ownerUserId, _ := ocmd.GetUserIdFromOCMAddress(s.Owner)
	creatorUserId, _ := ocmd.GetUserIdFromOCMAddress(s.Initiator)
	share := &ocm.ReceivedShare{
		Id: &ocm.ShareId{
			OpaqueId: strconv.Itoa(int(s.ID)),
		},
		RemoteShareId: s.RemoteShareID,
		Name:          s.Name,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: &userpb.UserId{
					OpaqueId: s.ShareWith,
				},
			},
		},
		Owner:   ownerUserId,
		Creator: creatorUserId,
		Ctime: &types.Timestamp{
			Seconds: uint64(s.Ctime),
		},
		Mtime: &types.Timestamp{
			Seconds: uint64(s.Mtime),
		},
		SharedResourceType: convertToCS3SharedResourceType(s.ItemType),
		ShareType:          convertToCS3OCMShareType(s.RecipientType),
		State:              convertToCS3OCMShareState(s.State),
		Protocols:          p,
	}
	if s.Expiration.Valid {
		share.Expiration = &types.Timestamp{
			Seconds: uint64(s.Expiration.V.Unix()),
		}
	}
	return share
}

func accessTypesIntToArray(at ocm.AccessType) []ocm.AccessType {
	switch at {
	case ocm.AccessType_ACCESS_TYPE_REMOTE:
		return []ocm.AccessType{at}
	case ocm.AccessType_ACCESS_TYPE_DATATX:
		return []ocm.AccessType{at}
	case ocm.AccessType_ACCESS_TYPE_REMOTE + ocm.AccessType_ACCESS_TYPE_DATATX:
		return []ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE, ocm.AccessType_ACCESS_TYPE_DATATX}
	default:
		return []ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE}
	}
}

func stringsFromJSON(r datatypes.JSON) []string {
	if r == nil {
		return nil
	}
	var reqs []string
	if err := json.Unmarshal(r, &reqs); err != nil {
		return nil
	}
	return reqs
}

func convertToCS3AccessMethod(m *model.OcmShareProtocol) *ocm.AccessMethod {
	switch m.Type {
	case model.WebDAVProtocol:
		return share.NewWebDavAccessMethod(
			permissions.RoleFromOCSPermissions(permissions.OcsPermissions(m.Permissions)).CS3ResourcePermissions(),
			accessTypesIntToArray(ocm.AccessType(m.AccessTypes)),
			stringsFromJSON(m.Requirements),
		)
	case model.WebappProtocol:
		return share.NewWebappAccessMethod(
			permissions.RoleFromOCSPermissions(permissions.OcsPermissions(m.Permissions)).CS3ResourcePermissions(),
			stringsFromJSON(m.Requirements),
			m.AppName,
		)
	}
	return nil
}

func convertToCS3Protocol(p *model.OcmReceivedShareProtocol) *ocm.Protocol {
	switch p.Type {
	case model.WebDAVProtocol:
		return share.NewWebDAVProtocol(p.Uri, p.SharedSecret,
			&ocm.SharePermissions{
				Permissions: permissions.RoleFromOCSPermissions(permissions.OcsPermissions(p.Permissions)).CS3ResourcePermissions(),
			},
			accessTypesIntToArray(ocm.AccessType(p.AccessTypes)),
			stringsFromJSON(p.Requirements),
		)
	case model.WebappProtocol:
		return share.NewWebappProtocol(p.Uri, p.SharedSecret,
			permissions.RoleFromOCSPermissions(permissions.OcsPermissions(p.Permissions)).CS3ResourcePermissions(),
			stringsFromJSON(p.Requirements),
			stringsFromJSON(p.Targets),
			p.AppName,
			p.AppIconHint,
			stringsFromJSON(p.MediaTypes),
		)
	case model.EmbeddedProtocol:
		return share.NewEmbeddedProtocol(string(p.Payload))
	}
	return nil
}

func convertToCS3SharedResourceType(t model.ItemType) ocm.SharedResourceType {
	switch t {
	case model.ItemTypeFile:
		return ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE
	case model.ItemTypeFolder:
		return ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER
	case model.ItemTypeEmbedded:
		return ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED
	}
	return ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID
}

func convertFromCS3ResourceType(t ocm.SharedResourceType) model.ItemType {
	switch t {
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE:
		return model.ItemTypeFile
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER:
		return model.ItemTypeFolder
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED:
		return model.ItemTypeEmbedded
	}
	return model.ItemTypeFile
}
