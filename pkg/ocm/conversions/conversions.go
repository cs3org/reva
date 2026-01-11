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

package conversions

import (
	"context"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/utils"
	libregraph "github.com/owncloud/libre-graph-api-go"
)

// Config contains the configuration for OCM conversions
type Config struct {
	WebBase string
}

// Converter provides methods to convert OCM shares to libregraph representations
type Converter struct {
	gatewayClient gateway.GatewayAPIClient
	config        *Config
}

// NewConverter creates a new OCM converter
func NewConverter(gatewayClient gateway.GatewayAPIClient, config *Config) *Converter {
	return &Converter{
		gatewayClient: gatewayClient,
		config:        config,
	}
}

// OCMReceivedShareToDriveItem converts an OCM received share to a libregraph DriveItem
func (c *Converter) OCMReceivedShareToDriveItem(ctx context.Context, receivedOCMShare *ocm.ReceivedShare, unifiedRoleConverter func(context.Context, *provider.ResourcePermissions) *UnifiedRoleDefinition) (*libregraph.DriveItem, error) {
	createdTime := utils.TSToTime(receivedOCMShare.Ctime)

	grantee, err := c.CS3GranteeToSharePointIdentitySet(ctx, receivedOCMShare.Grantee)
	if err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	log.Debug().Interface("receivedOCMShare", receivedOCMShare).Msg("processing received OCM share")

	var permissions *provider.ResourcePermissions
	for _, p := range receivedOCMShare.Protocols {
		if p.GetWebdavOptions() != nil {
			permissions = p.GetWebdavOptions().GetPermissions().Permissions
			log.Debug().Str("webdav_uri", p.GetWebdavOptions().GetUri()).Str("shared_secret", p.GetWebdavOptions().GetSharedSecret()).Msg("processing webdav options")
		} else if p.GetWebappOptions() != nil {
			log.Debug().Str("webapp_uri", p.GetWebappOptions().GetUri()).Str("shared_secret", p.GetWebappOptions().GetSharedSecret()).Msg("processing webapp options")
		} else {
			log.Debug().Any("protocol", p).Msg("unknown access method, skipping")
		}
	}

	// using mtime as a makeshift etag
	etag := receivedOCMShare.Mtime.String()

	roles := make([]string, 0, 1)
	if unifiedRoleConverter != nil {
		role := unifiedRoleConverter(ctx, permissions)
		if role != nil {
			roles = append(roles, *role.Id)
		}
	}

	lgOCMUser := &libregraph.Identity{
		DisplayName:        c.getDisplayNameForOCMUser(ctx, receivedOCMShare.Creator),
		Id:                 libregraph.PtrString(utils.PrintOCMUserId(receivedOCMShare.Creator)),
		LibreGraphUserType: libregraph.PtrString("Federated"),
	}

	d := &libregraph.DriveItem{
		UIHidden:          libregraph.PtrBool(false), // Doesn't exist for OCM shares
		ClientSynchronize: libregraph.PtrBool(false),
		CreatedBy: &libregraph.IdentitySet{
			User: lgOCMUser,
		},

		ETag:                 &etag,
		Id:                   libregraph.PtrString(receivedOCMShare.Id.OpaqueId),
		LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(receivedOCMShare.Mtime)),
		Name:                 libregraph.PtrString(receivedOCMShare.Name),
		ParentReference: &libregraph.ItemReference{
			DriveId:   libregraph.PtrString(spaces.ConcatStorageSpaceID(ShareJailID, ShareJailID)),
			DriveType: libregraph.PtrString("virtual"),
			Id:        libregraph.PtrString(spaces.EncodeResourceID(&provider.ResourceId{OpaqueId: ShareJailID, StorageId: ShareJailID, SpaceId: ShareJailID})),
		},
		RemoteItem: &libregraph.RemoteItem{
			CreatedBy: &libregraph.IdentitySet{
				User: lgOCMUser,
			},
			ETag:                 &etag,
			Id:                   libregraph.PtrString(spaces.EncodeOCMShareID(receivedOCMShare.Id.OpaqueId)),
			LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(receivedOCMShare.Mtime)),
			WebUrl:               libregraph.PtrString(c.config.WebBase + "/ocm-share/" + receivedOCMShare.Name),
			Name:                 libregraph.PtrString(receivedOCMShare.Name),
			Permissions: []libregraph.Permission{
				{
					CreatedDateTime: *libregraph.NewNullableTime(&createdTime),
					GrantedToV2:     grantee,
					Invitation: &libregraph.SharingInvitation{
						InvitedBy: &libregraph.IdentitySet{
							User: lgOCMUser,
						},
					},
					Roles: roles,
				},
			},
			Size: libregraph.PtrInt64(int64(0)), // OCM shares do not have a size
		},
		Size: libregraph.PtrInt64(int64(0)),
	}

	if receivedOCMShare.SharedResourceType == ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER {
		d.Folder = libregraph.NewFolder()
	}
	return d, nil
}

// CS3GranteeToSharePointIdentitySet converts a CS3 grantee to a SharePoint identity set for OCM users
func (c *Converter) CS3GranteeToSharePointIdentitySet(ctx context.Context, grantee *provider.Grantee) (*libregraph.SharePointIdentitySet, error) {
	p := &libregraph.SharePointIdentitySet{}
	if grantee == nil {
		return p, nil
	}

	if u := grantee.GetUserId(); u != nil {
		p.User = &libregraph.Identity{
			DisplayName:        c.getDisplayNameForOCMUser(ctx, u),
			Id:                 libregraph.PtrString(utils.PrintOCMUserId(u)),
			LibreGraphUserType: libregraph.PtrString("Federated"),
		}
	}

	return p, nil
}

func (c *Converter) getDisplayNameForOCMUser(ctx context.Context, userId *userpb.UserId) string {
	log := appctx.GetLogger(ctx)
	if c.gatewayClient == nil {
		log.Error().Msg("getDisplayNameForOCMUser: gateway client is nil")
		return "Federated User"
	}

	// Pass the current user as opaque parameter. It seems the getUserFilter in internal/grpc/services/ocminvitemanager/ocminvitemanager.og
	// is not able to return the current user and this way we can force it.
	// TODO(lopresti) here we want a GetRemoteUser CS3 API that only requires the invitee's id
	user := appctx.ContextMustGetUser(ctx)
	userFilter, err := utils.MarshalProtoV1ToJSON(user.Id)
	if err != nil {
		log.Error().Err(err).Msg("getDisplayNameForOCMUser: failed to marshal current user id to json")
		return "Federated User"
	}

	remoteUserRes, err := c.gatewayClient.GetAcceptedUser(ctx, &invitepb.GetAcceptedUserRequest{
		RemoteUserId: userId,
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"user-filter": {
					Decoder: "json",
					Value:   userFilter,
				},
			},
		},
	})
	if err != nil || remoteUserRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Err(err).Any("response", remoteUserRes).Any("remoteUser", userId).Msg("failed to fetch OCM user")
		return "Federated User"
	}

	return remoteUserRes.RemoteUser.DisplayName
}

// ShareJailID is the jail ID for shares
const ShareJailID = "a0ca6a90-a365-4782-871e-d44447bbc668"

// UnifiedRoleDefinition represents a unified role definition
// This is a simplified version for the converter to avoid circular dependencies
type UnifiedRoleDefinition struct {
	Id *string
}
