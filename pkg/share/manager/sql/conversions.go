// Copyright 2018-2025 CERN
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
	"strconv"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
)

func convertFromCS3OCMShareType(shareType ocm.ShareType) model.OcmShareType {
	switch shareType {
	case ocm.ShareType_SHARE_TYPE_USER:
		return model.OcmShareTypeUser
	case ocm.ShareType_SHARE_TYPE_GROUP:
		return model.OcmShareTypeGroup
	}
	return -1
}

func convertFromCS3OCMShareState(shareState ocm.ShareState) model.OcmShareState {
	switch shareState {
	case ocm.ShareState_SHARE_STATE_ACCEPTED:
		return model.OcmShareStateAccepted
	case ocm.ShareState_SHARE_STATE_PENDING:
		return model.OcmShareStatePending
	case ocm.ShareState_SHARE_STATE_REJECTED:
		return model.OcmShareStateRejected
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
			StorageId: s.StorageId,
			OpaqueId:  s.FileId,
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
		ShareType:     ocm.ShareType_SHARE_TYPE_USER,
		AccessMethods: am,
	}
	if s.Expiration.Valid {
		share.Expiration = &types.Timestamp{
			Seconds: uint64(s.Expiration.Int64),
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
		ResourceType: convertToCS3ResourceType(s.ItemType),
		ShareType:    ocm.ShareType_SHARE_TYPE_USER,
		State:        convertToCS3OCMShareState(s.State),
		Protocols:    p,
	}
	if s.Expiration.Valid {
		share.Expiration = &types.Timestamp{
			Seconds: uint64(s.Expiration.Int64),
		}
	}
	return share
}

func viewModeToInt(v appprovider.ViewMode) int {
	switch v {
	case appprovider.ViewMode_VIEW_MODE_INVALID:
		return 0
	case appprovider.ViewMode_VIEW_MODE_VIEW_ONLY:
		return 1
	case appprovider.ViewMode_VIEW_MODE_READ_ONLY:
		return 2
	case appprovider.ViewMode_VIEW_MODE_READ_WRITE:
		return 3
	case appprovider.ViewMode_VIEW_MODE_PREVIEW:
		return 4
	}
	return -1
}

func convertToCS3AccessMethod(m *model.OcmShareProtocol) *ocm.AccessMethod {
	switch m.Type {
	case model.WebDAVProtocol:
		return share.NewWebDavAccessMethod(
			conversions.RoleFromOCSPermissions(conversions.Permissions(m.Permissions)).CS3ResourcePermissions(),
			[]string{}) // TODO persist requirements
	case model.WebappProtocol:
		return share.NewWebappAccessMethod(appprovider.ViewMode(m.Permissions))
	case model.TransferProtocol:
		return share.NewTransferAccessMethod()
	}
	return nil
}

func convertToCS3Protocol(p *model.OcmReceivedShareProtocol) *ocm.Protocol {
	switch p.Type {
	case model.WebDAVProtocol:
		return share.NewWebDAVProtocol(p.Uri, p.SharedSecret, &ocm.SharePermissions{
			Permissions: conversions.RoleFromOCSPermissions(conversions.Permissions(p.Permissions)).CS3ResourcePermissions(),
		}, []string{}) // TODO persist requirements
	case model.WebappProtocol:
		return share.NewWebappProtocol(p.Uri, appprovider.ViewMode(p.Permissions))
	case model.TransferProtocol:
		return share.NewTransferProtocol(p.Uri, p.SharedSecret, uint64(p.Size))
	}
	return nil
}

func convertToCS3ResourceType(t model.ItemType) provider.ResourceType {
	switch t {
	case model.ItemTypeFile:
		return provider.ResourceType_RESOURCE_TYPE_FILE
	case model.ItemTypeFolder:
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_INVALID
}

func convertFromCS3ResourceType(t provider.ResourceType) model.ItemType {
	switch t {
	case provider.ResourceType_RESOURCE_TYPE_FILE:
		return model.ItemTypeFile
	case provider.ResourceType_RESOURCE_TYPE_CONTAINER:
		return model.ItemTypeFolder
	}
	return model.ItemTypeFile
}
