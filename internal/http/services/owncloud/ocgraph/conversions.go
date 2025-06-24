package ocgraph

import (
	"context"
	"path"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"github.com/pkg/errors"
)

func (s *svc) shareToLibregraphPerm(ctx context.Context, share *ShareOrLink) (*libregraph.Permission, error) {
	if share == nil {
		return nil, errors.New("share is nil")
	}

	nilTime := libregraph.NewNullableTime(nil)
	nilTime.Unset()

	if share.shareType == "share" {
		grantedTo := libregraph.NewSharePointIdentitySet()
		grantee := share.share.GetGrantee()
		switch grantee.Type {
		case provider.GranteeType_GRANTEE_TYPE_USER:
			u, err := s.getUserInfo(ctx, grantee.GetUserId())
			if err != nil {
				return nil, errors.New("Failed to fetch user info")
			}
			grantedTo.SetUser(libregraph.Identity{
				Id:          libregraph.PtrString(grantee.GetUserId().OpaqueId),
				DisplayName: u.DisplayName,
			})
		case provider.GranteeType_GRANTEE_TYPE_GROUP:
			g, err := s.getGroupInfo(ctx, grantee.GetGroupId())
			if err != nil {
				return nil, errors.New("Failed to fetch user info")
			}
			grantedTo.SetGroup(libregraph.Identity{
				Id:          libregraph.PtrString(grantee.GetGroupId().OpaqueId),
				DisplayName: g.DisplayName,
			})
		}
		u, err := s.getUserInfo(ctx, share.share.Creator)
		if err != nil {
			return nil, errors.New("Failed to fetch user info")
		}
		invitation := libregraph.NewSharingInvitation()
		idSet := *libregraph.NewIdentitySet()
		idSet.SetUser(libregraph.Identity{
			Id:          libregraph.PtrString(share.share.Creator.OpaqueId),
			DisplayName: u.DisplayName,
		})
		invitation.SetInvitedBy(idSet)

		unifiedRoleDefinition, role := CS3ResourcePermissionsToUnifiedRole(ctx, share.share.Permissions.Permissions), ""
		if unifiedRoleDefinition != nil {
			role = *unifiedRoleDefinition.Id
		}

		perm := &libregraph.Permission{
			Id:                 libregraph.PtrString(share.ID),
			ExpirationDateTime: *nilTime,
			CreatedDateTime:    *libregraph.NewNullableTime(libregraph.PtrTime(time.Unix(int64(share.share.GetCtime().Seconds), 0))),
			GrantedToV2:        grantedTo,
			Invitation:         invitation,
			Roles:              []string{role},
		}
		return perm, nil
	} else {
		lt, actions := SharingLinkTypeFromCS3Permissions(ctx, share.link.GetPermissions())
		var expTime libregraph.NullableTime
		if share.link.GetExpiration() != nil {
			expTime = *libregraph.NewNullableTime(libregraph.PtrTime(time.Unix(int64(share.link.GetExpiration().Seconds), 0)))
		} else {
			expTime = *nilTime
		}
		perm := &libregraph.Permission{
			Id:                 libregraph.PtrString(share.ID),
			ExpirationDateTime: expTime,
			HasPassword:        libregraph.PtrBool(share.link.GetPasswordProtected()),
			CreatedDateTime:    *libregraph.NewNullableTime(libregraph.PtrTime(time.Unix(int64(share.link.GetCtime().Seconds), 0))),
			Link: &libregraph.SharingLink{
				Type:                  lt,
				LibreGraphDisplayName: libregraph.PtrString(share.link.GetDisplayName()),
				LibreGraphQuickLink:   libregraph.PtrBool(share.link.GetQuicklink()),
				WebUrl:                libregraph.PtrString(path.Join(s.c.BaseURL, "s", share.link.GetToken())),
			},
			LibreGraphPermissionsActions: actions,
		}
		return perm, nil
	}
}

func (s *svc) lgPermToCS3Perm(ctx context.Context, lgPerm *libregraph.Permission, resourceType provider.ResourceType) (*provider.ResourcePermissions, error) {
	if lgPerm == nil {
		return nil, errors.New("no permissions passed")
	}
	if lgPerm.Link != nil && lgPerm.Link.Type != nil {
		return LinkTypeToPermissions(*lgPerm.Link.Type, resourceType), nil
	}
	if lgPerm.Roles != nil {
		rolePerms := make([]libregraph.UnifiedRolePermission, 0)
		for _, role := range lgPerm.Roles {
			def, ok := UnifiedRoleIDToDefinition(role)
			if ok {
				rolePerms = append(rolePerms, def.RolePermissions...)
			}
		}
		return PermissionsToCS3ResourcePermissions(rolePerms), nil
	}
	return nil, nil

}

func cs3TimestampToTime(t *types.Timestamp) time.Time {
	return time.Unix(int64(t.GetSeconds()), int64(t.GetNanos()))
}

func LinkTypeToPermissions(lt libregraph.SharingLinkType, resourceType provider.ResourceType) *provider.ResourcePermissions {
	switch lt {
	case libregraph.VIEW:
		return NewViewLinkPermissionSet().GetPermissions()
	case libregraph.EDIT:
		if resourceType == provider.ResourceType_RESOURCE_TYPE_FILE {
			return NewFileEditLinkPermissionSet().GetPermissions()
		}
		return NewFolderEditLinkPermissionSet().GetPermissions()
	case libregraph.UPLOAD:
		return NewFolderUploadLinkPermissionSet().GetPermissions()
	case libregraph.CREATE_ONLY:
		if resourceType == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			return NewFolderDropLinkPermissionSet().GetPermissions()
		}
		fallthrough
	case libregraph.BLOCKS_DOWNLOAD:
		fallthrough
	case libregraph.INTERNAL:
		fallthrough
	default:
		return conversions.NewDeniedRole().CS3ResourcePermissions()
	}
}
