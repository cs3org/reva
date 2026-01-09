package ocgraph

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	group "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/appctx"
	ocmconversions "github.com/cs3org/reva/v3/pkg/ocm/conversions"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/utils"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"github.com/pkg/errors"
)

func (s *svc) shareToLibregraphPerm(ctx context.Context, share *GenericShare) (*libregraph.Permission, error) {
	if share == nil {
		return nil, errors.New("share is nil")
	}

	switch share.shareType {
	case ShareTypeShare:
		return s.handleRegularShare(ctx, share)
	case ShareTypeOCMShare:
		return s.handleOCMShare(ctx, share)
	default:
		return s.handleLinkShare(ctx, share)
	}
}

func (s *svc) handleRegularShare(ctx context.Context, share *GenericShare) (*libregraph.Permission, error) {
	grantedTo, err := s.buildGrantedToForRegularShare(ctx, share.share.GetGrantee())
	if err != nil {
		return nil, err
	}

	invitation, err := s.buildInvitation(ctx, share.share.Creator)
	if err != nil {
		return nil, err
	}

	unifiedRoleDefinition, role := CS3ResourcePermissionsToUnifiedRole(ctx, share.share.Permissions.Permissions), ""
	if unifiedRoleDefinition != nil {
		role = *unifiedRoleDefinition.Id
	}

	perm := &libregraph.Permission{
		Id:                 libregraph.PtrString(share.ID),
		ExpirationDateTime: *cs3TimestampToNullableTime(share.share.Expiration),
		CreatedDateTime:    *cs3TimestampToNullableTime(share.share.GetCtime()),
		GrantedToV2:        grantedTo,
		Invitation:         invitation,
		Roles:              []string{role},
	}
	return perm, nil
}

func (s *svc) handleOCMShare(ctx context.Context, share *GenericShare) (*libregraph.Permission, error) {
	grantedTo, err := s.buildGrantedToForOCMShare(ctx, share.ocmshare.GetGrantee())
	if err != nil {
		return nil, err
	}

	invitation, err := s.buildInvitation(ctx, share.ocmshare.Creator)
	if err != nil {
		return nil, err
	}

	// Here we assume that the OCM shares offered by us always contain both `webdav` and `webapp` access methods with
	// identical permissions, therefore we use the first one here (`webdav`) to generate the libregraph representation
	unifiedRoleDefinition, role := CS3ResourcePermissionsToUnifiedRole(ctx, share.ocmshare.AccessMethods[0].GetWebdavOptions().Permissions), ""
	if unifiedRoleDefinition != nil {
		role = *unifiedRoleDefinition.Id
	}

	perm := &libregraph.Permission{
		Id:                 libregraph.PtrString(share.ID),
		ExpirationDateTime: *cs3TimestampToNullableTime(share.ocmshare.Expiration),
		CreatedDateTime:    *cs3TimestampToNullableTime(share.ocmshare.GetCtime()),
		GrantedToV2:        grantedTo,
		Invitation:         invitation,
		Roles:              []string{role},
	}
	return perm, nil
}

func (s *svc) handleLinkShare(ctx context.Context, share *GenericShare) (*libregraph.Permission, error) {
	nilTime := libregraph.NewNullableTime(nil)
	nilTime.Unset()

	lt, actions := SharingLinkTypeFromCS3Permissions(ctx, share.link.GetPermissions())

	var expTime libregraph.NullableTime
	if share.link.GetExpiration() != nil {
		expTime = *libregraph.NewNullableTime(libregraph.PtrTime(time.Unix(int64(share.link.GetExpiration().Seconds), 0)))
	} else {
		expTime = *nilTime
	}

	webUrl, _ := url.JoinPath(s.c.BaseURL, "/s", share.link.GetToken())

	perm := &libregraph.Permission{
		Id:                 libregraph.PtrString(share.ID),
		ExpirationDateTime: expTime,
		HasPassword:        libregraph.PtrBool(share.link.GetPasswordProtected()),
		CreatedDateTime:    *libregraph.NewNullableTime(libregraph.PtrTime(time.Unix(int64(share.link.GetCtime().Seconds), 0))),
		Link: &libregraph.SharingLink{
			Type:                  lt,
			LibreGraphDisplayName: libregraph.PtrString(share.link.GetDisplayName()),
			LibreGraphQuickLink:   libregraph.PtrBool(share.link.GetQuicklink()),
			WebUrl:                libregraph.PtrString(webUrl),
		},
		LibreGraphPermissionsActions: actions,
	}
	if share.link.NotifyUploads {
		perm.LibreGraphPermissionsActions = append(perm.LibreGraphPermissionsActions, "notifyUploads")
	}
	return perm, nil
}

func (s *svc) buildGrantedToForRegularShare(ctx context.Context, grantee *provider.Grantee) (*libregraph.SharePointIdentitySet, error) {
	grantedTo := libregraph.NewSharePointIdentitySet()

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
			return nil, errors.New("Failed to fetch group info")
		}
		grantedTo.SetGroup(libregraph.Identity{
			Id:          libregraph.PtrString(grantee.GetGroupId().OpaqueId),
			DisplayName: g.DisplayName,
		})
	}

	return grantedTo, nil
}

func (s *svc) buildGrantedToForOCMShare(ctx context.Context, grantee *provider.Grantee) (*libregraph.SharePointIdentitySet, error) {
	gw, err := s.getClient()
	if err != nil {
		return nil, err
	}

	converter := ocmconversions.NewConverter(gw, &ocmconversions.Config{
		WebBase: s.c.WebBase,
	})

	return converter.CS3GranteeToSharePointIdentitySet(ctx, grantee)
}

// The user must exist, otherwise an error is returned, this representation is used to show who
// created the share in the LibreGraph API.
func (s *svc) buildInvitation(ctx context.Context, creator *user.UserId) (*libregraph.SharingInvitation, error) {
	u, err := s.getUserInfo(ctx, creator)
	if err != nil {
		return nil, errors.New("Failed to fetch user info")
	}

	invitation := libregraph.NewSharingInvitation()
	idSet := *libregraph.NewIdentitySet()
	idSet.SetUser(libregraph.Identity{
		Id:          libregraph.PtrString(creator.OpaqueId),
		DisplayName: u.DisplayName,
	})
	invitation.SetInvitedBy(idSet)

	return invitation, nil
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

func cs3TimestampToNullableTime(t *types.Timestamp) *libregraph.NullableTime {
	if t == nil {
		nilTime := libregraph.NewNullableTime(nil)
		nilTime.Unset()
		return nilTime
	}
	return libregraph.NewNullableTime(libregraph.PtrTime(time.Unix(int64(t.GetSeconds()), 0)))
}

func nullableTimeToCs3Timestamp(t libregraph.NullableTime) *types.Timestamp {
	if !t.IsSet() || t.Get() == nil {
		return nil
	}
	return &types.Timestamp{
		Seconds: uint64(t.Get().Unix()),
	}
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
		return NewFolderDropLinkPermissionSet().GetPermissions()
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
		return permissions.NewDeniedRole().CS3ResourcePermissions()
	}
}

func (s *svc) cs3GranteeToSharePointIdentitySet(ctx context.Context, grantee *provider.Grantee) (*libregraph.SharePointIdentitySet, error) {
	p := &libregraph.SharePointIdentitySet{}
	if grantee == nil {
		return p, nil
	}

	if u := grantee.GetUserId(); u != nil {
		user, err := s.getUserByID(ctx, u)
		if err != nil {
			return nil, err
		}
		if user != nil {
			p.User = &libregraph.Identity{
				DisplayName: user.DisplayName,
				Id:          libregraph.PtrString(u.OpaqueId),
			}
		}
	} else if g := grantee.GetGroupId(); g != nil {
		group, err := s.getGroupByID(ctx, g)
		if err != nil {
			return nil, err
		}
		if group != nil {
			p.Group = &libregraph.Identity{
				DisplayName: group.DisplayName,
				Id:          libregraph.PtrString(g.OpaqueId),
			}
		}
	}

	return p, nil
}

func (s *svc) ocmGranteeToSharePointIdentitySet(ctx context.Context, grantee *provider.Grantee) (*libregraph.SharePointIdentitySet, error) {
	gw, err := s.getClient()
	if err != nil {
		return nil, err
	}

	converter := ocmconversions.NewConverter(gw, &ocmconversions.Config{
		WebBase: s.c.WebBase,
	})

	return converter.CS3GranteeToSharePointIdentitySet(ctx, grantee)
}

func (s *svc) cs3ReceivedShareToDriveItem(ctx context.Context, rsi *gateway.ReceivedShareResourceInfo) (*libregraph.DriveItem, error) {
	if rsi.ReceivedShare == nil || rsi.ResourceInfo == nil || rsi.ReceivedShare.Share == nil {
		return nil, errors.New("cannot convert nil share into libregraph drive")
	}
	createdTime := utils.TSToTime(rsi.ReceivedShare.Share.Ctime)

	creator, err := s.getUserByID(ctx, rsi.ReceivedShare.Share.Creator)
	if err != nil {
		return nil, err
	}

	grantee, err := s.cs3GranteeToSharePointIdentitySet(ctx, rsi.ReceivedShare.Share.Grantee)
	if err != nil {
		return nil, err
	}

	relativePath, err := spaces.PathRelativeToSpaceRoot(rsi.ResourceInfo)
	if err != nil {
		return nil, err
	}

	roles := make([]string, 0, 1)
	role := CS3ResourcePermissionsToUnifiedRole(ctx, rsi.ResourceInfo.PermissionSet)

	if role != nil {
		roles = append(roles, *role.Id)
	}

	webUrl, _ := url.JoinPath(s.c.WebBase, rsi.ResourceInfo.Path)

	d := &libregraph.DriveItem{
		UIHidden:          libregraph.PtrBool(rsi.ReceivedShare.Hidden),
		ClientSynchronize: libregraph.PtrBool(true),
		CreatedBy: &libregraph.IdentitySet{
			User: &libregraph.Identity{
				DisplayName: creator.DisplayName,
				Id:          libregraph.PtrString(creator.Id.OpaqueId),
			},
		},
		ETag:                 &rsi.ResourceInfo.Etag,
		Id:                   libregraph.PtrString(libregraphShareID(rsi.ReceivedShare.Share.Id)),
		LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(rsi.ResourceInfo.Mtime)),
		Name:                 libregraph.PtrString(rsi.ResourceInfo.Name),
		ParentReference: &libregraph.ItemReference{
			DriveId:   libregraph.PtrString(spaces.ConcatStorageSpaceID(ShareJailID, ShareJailID)),
			DriveType: libregraph.PtrString("virtual"),
			Id:        libregraph.PtrString(spaces.EncodeResourceID(&provider.ResourceId{OpaqueId: ShareJailID, StorageId: ShareJailID, SpaceId: ShareJailID})),
		},
		RemoteItem: &libregraph.RemoteItem{
			CreatedBy: &libregraph.IdentitySet{
				User: &libregraph.Identity{
					DisplayName: creator.DisplayName,
					Id:          libregraph.PtrString(creator.Id.OpaqueId),
				},
			},
			ETag: &rsi.ResourceInfo.Etag,
			File: &libregraph.OpenGraphFile{
				MimeType: &rsi.ResourceInfo.MimeType,
			},
			Id:                   libregraph.PtrString(encodeSpaceIDForShareJail(rsi.ResourceInfo)),
			LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(rsi.ResourceInfo.Mtime)),
			Name:                 libregraph.PtrString(rsi.ResourceInfo.Name),
			Path:                 libregraph.PtrString(relativePath),
			WebUrl:               libregraph.PtrString(webUrl),
			// ParentReference: &libregraph.ItemReference{
			// 	DriveId:   libregraph.PtrString(spaces.EncodeResourceID(share.ResourceInfo.ParentId)),
			// 	DriveType: nil, // FIXME: no way to know it unless we hardcode it
			// },
			Permissions: []libregraph.Permission{
				{
					CreatedDateTime: *libregraph.NewNullableTime(&createdTime),
					GrantedToV2:     grantee,
					Invitation: &libregraph.SharingInvitation{
						InvitedBy: &libregraph.IdentitySet{
							User: &libregraph.Identity{
								DisplayName: creator.DisplayName,
								Id:          libregraph.PtrString(creator.Id.OpaqueId),
							},
						},
					},
					Roles: roles,
				},
			},
			Size: libregraph.PtrInt64(int64(rsi.ResourceInfo.Size)),
		},
		Size: libregraph.PtrInt64(int64(rsi.ResourceInfo.Size)),
	}

	if rsi.ResourceInfo.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		d.Folder = libregraph.NewFolder()
	} else {
		d.File = &libregraph.OpenGraphFile{
			MimeType: &rsi.ResourceInfo.MimeType,
		}
	}

	return d, nil
}

func (s *svc) cs3ShareToDriveItem(ctx context.Context, info *provider.ResourceInfo, shares []*GenericShare) (*libregraph.DriveItem, error) {
	relativePath, err := spaces.PathRelativeToSpaceRoot(info)
	if err != nil {
		return nil, err
	}

	parentRelativePath := path.Dir(relativePath)
	if parentRelativePath == "." {
		parentRelativePath = ""
	}

	permissions, err := s.cs3sharesToPermissions(ctx, shares)
	if err != nil {
		return nil, err
	}

	if info.ParentId.SpaceId == "" {
		info.ParentId.SpaceId = spaces.PathToSpaceID(info.Path)
	}
	webUrl, _ := url.JoinPath(s.c.WebBase, info.Path)

	d := &libregraph.DriveItem{
		ETag:                 libregraph.PtrString(info.Etag),
		Id:                   libregraph.PtrString(spaces.EncodeResourceID(info.Id)),
		LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(info.Mtime)),
		Name:                 libregraph.PtrString(info.Name),
		ParentReference: &libregraph.ItemReference{
			DriveId: libregraph.PtrString(spaces.ConcatStorageSpaceID(info.ParentId.StorageId, info.ParentId.SpaceId)),
			// DriveType: libregraph.PtrString(info.Space.SpaceType),
			Id:   libregraph.PtrString(spaces.EncodeResourceID(info.ParentId)),
			Name: libregraph.PtrString(path.Base(relativePath)),
			Path: libregraph.PtrString(parentRelativePath),
		},
		WebUrl:      libregraph.PtrString(webUrl),
		Permissions: permissions,

		Size: libregraph.PtrInt64(int64(info.Size)),
	}

	if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		d.Folder = libregraph.NewFolder()
	} else {
		d.File = &libregraph.OpenGraphFile{
			MimeType: &info.MimeType,
		}
	}

	return d, nil
}

func (s *svc) OCMReceivedShareToDriveItem(ctx context.Context, receivedOCMShare *ocm.ReceivedShare) (*libregraph.DriveItem, error) {
	gw, err := s.getClient()
	if err != nil {
		return nil, err
	}

	converter := ocmconversions.NewConverter(gw, &ocmconversions.Config{
		WebBase: s.c.WebBase,
	})

	// Create a wrapper function to convert UnifiedRoleDefinition
	roleConverter := func(ctx context.Context, perms *provider.ResourcePermissions) *ocmconversions.UnifiedRoleDefinition {
		role := CS3ResourcePermissionsToUnifiedRole(ctx, perms)
		if role == nil {
			return nil
		}
		return &ocmconversions.UnifiedRoleDefinition{
			Id: role.Id,
		}
	}

	return converter.OCMReceivedShareToDriveItem(ctx, receivedOCMShare, roleConverter)
}

func (s *svc) cs3sharesToPermissions(ctx context.Context, shares []*GenericShare) ([]libregraph.Permission, error) {
	log := appctx.GetLogger(ctx)
	permissions := make([]libregraph.Permission, 0, len(shares))

	for _, e := range shares {
		if e.share != nil {
			createdTime := utils.TSToTime(e.share.Ctime)

			creator, err := s.getUserByID(ctx, e.share.Creator)
			if err != nil {
				log.Error().Err(err).Msg("Failed to convert cs3 share to permission in ocgraph - ignoring this share")
				continue
			}

			grantee, err := s.cs3GranteeToSharePointIdentitySet(ctx, e.share.Grantee)
			if err != nil {
				log.Error().Err(err).Msg("Failed to convert cs3 share to permission in ocgraph - ignoring this share")
				continue
			}

			roles := make([]string, 0, 1)
			role := CS3ResourcePermissionsToUnifiedRole(ctx, e.share.Permissions.Permissions)
			if role != nil {
				roles = append(roles, *role.Id)
			}
			permissions = append(permissions, libregraph.Permission{
				CreatedDateTime: *libregraph.NewNullableTime(&createdTime),
				GrantedToV2:     grantee,
				Invitation: &libregraph.SharingInvitation{
					InvitedBy: &libregraph.IdentitySet{
						User: &libregraph.Identity{
							DisplayName: creator.DisplayName,
							Id:          libregraph.PtrString(creator.Id.OpaqueId),
						},
					},
				},
				Roles: roles,
			})
		} else if e.link != nil {
			// TODO(jgeens): can this be deduplicated with handleLinkShare?
			createdTime := utils.TSToTime(e.link.Ctime)
			linktype, _ := SharingLinkTypeFromCS3Permissions(ctx, e.link.Permissions)

			permisison := libregraph.Permission{
				CreatedDateTime: *libregraph.NewNullableTime(&createdTime),
				HasPassword:     libregraph.PtrBool(e.link.PasswordProtected),
				Id:              libregraph.PtrString(e.link.Token),
				Link: &libregraph.SharingLink{
					LibreGraphDisplayName: libregraph.PtrString(e.link.DisplayName),
					LibreGraphQuickLink:   libregraph.PtrBool(e.link.Quicklink),
					PreventsDownload:      libregraph.PtrBool(false),
					Type:                  linktype,
				},
			}
			if e.link.NotifyUploads {
				permisison.LibreGraphPermissionsActions = append(permisison.LibreGraphPermissionsActions, "notifyUploads")
			}

			permissions = append(permissions, permisison)

		} else if e.ocmshare != nil {
			createdTime := utils.TSToTime(e.ocmshare.Ctime)

			creator, err := s.getUserByID(ctx, e.ocmshare.Creator)
			if err != nil {
				log.Error().Err(err).Msg("Failed to convert cs3 ocm share to permission in ocgraph - ignoring this share")
				continue
			}

			grantee, err := s.ocmGranteeToSharePointIdentitySet(ctx, e.ocmshare.Grantee)
			if err != nil {
				log.Error().Err(err).Msg("Failed to convert cs3 ocm share to permission in ocgraph - ignoring this share")
				continue
			}
			roles := make([]string, 0, 1)
			if len(e.ocmshare.AccessMethods) == 0 {
				log.Warn().Msg("Found an OCM share without access methods in cs3sharesToPermissions - ignoring this share")
				continue
			}
			// Go through the different access methods to find the webdav permissions
			// We assume that all access methods have the same permissions, so we break
			// after the first match
			// TODO(rwelande): maybe in the future we want to show different permissions for
			// different access methods
			for _, am := range e.ocmshare.AccessMethods {
				if am.GetWebdavOptions() != nil && am.GetWebdavOptions().GetPermissions() != nil {
					role := CS3ResourcePermissionsToUnifiedRole(ctx, am.GetWebdavOptions().GetPermissions())
					if role != nil {
						roles = append(roles, *role.Id)
					}
					break
				}
			}
			permissions = append(permissions, libregraph.Permission{
				CreatedDateTime: *libregraph.NewNullableTime(&createdTime),
				GrantedToV2:     grantee,
				Invitation: &libregraph.SharingInvitation{
					InvitedBy: &libregraph.IdentitySet{
						User: &libregraph.Identity{
							DisplayName: creator.DisplayName,
							Id:          libregraph.PtrString(creator.Id.OpaqueId),
						},
					},
				},
				Roles: roles,
			})
		} else {
			log.Warn().Msg("Found a nil share in cs3sharesToPermissions - ignoring this share")
		}
	}

	return permissions, nil
}

func (s *svc) toGrantee(ctx context.Context, recipientType string, id string) (*provider.Grantee, error) {
	gw, err := s.getClient()
	if err != nil {
		return nil, err
	}

	switch recipientType {
	case "user":
		userRes, err := gw.GetUserByClaim(ctx, &user.GetUserByClaimRequest{
			Claim:                  "username",
			Value:                  id,
			SkipFetchingUserGroups: true,
		})
		if err != nil || userRes.Status == nil || userRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			return nil, errors.New("failed to fetch sharee data")
		}
		return &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id:   &provider.Grantee_UserId{UserId: userRes.User.GetId()},
		}, nil
	case "group":
		groupRes, err := gw.GetGroupByClaim(ctx, &group.GetGroupByClaimRequest{
			Claim:               "group_name",
			Value:               id,
			SkipFetchingMembers: true,
		})
		if err != nil || groupRes.Status == nil || groupRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			return nil, errors.New("failed to fetch sharee data")
		}
		return &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id:   &provider.Grantee_GroupId{GroupId: groupRes.Group.GetId()},
		}, nil
	default:
		return nil, errors.New(recipientType + " is not a valid granteetype")
	}
}

func (s *svc) convertShareToSpace(rsi *gateway.ReceivedShareResourceInfo) *libregraph.Drive {
	// the prefix of the remote_item.id and rootid
	spacePath, _ := spaces.ResourceToSpacePath(rsi.ResourceInfo)
	// Drive Alias does not contain the first '/'
	driveAlias := strings.TrimPrefix(spacePath, "/")
	resourceRelativePath, _ := spaces.PathRelativeToSpaceRoot(rsi.ResourceInfo)

	drive := &libregraph.Drive{
		Id:         libregraph.PtrString(libregraphShareID(rsi.ReceivedShare.Share.Id)),
		DriveType:  libregraph.PtrString("mountpoint"),
		DriveAlias: libregraph.PtrString(rsi.ReceivedShare.Share.Id.OpaqueId), // this is not used, but must not be the same alias as the drive item
		Name:       filepath.Base(rsi.ResourceInfo.Path),
		WebUrl:     libregraph.PtrString(fullURL(s.c.WebBase, rsi.ResourceInfo.Path)),
		Quota: &libregraph.Quota{
			Total:     libregraph.PtrInt64(24154390300000),
			Used:      libregraph.PtrInt64(3141592),
			Remaining: libregraph.PtrInt64(24154387158408),
		},
		Root: &libregraph.DriveItem{
			Id:        libregraph.PtrString(fmt.Sprintf("%s$%s!%s", ShareJailID, ShareJailID, rsi.ReceivedShare.Share.Id.OpaqueId)),
			WebDavUrl: libregraph.PtrString(fullURL(s.c.WebDavBase, rsi.ResourceInfo.Path)),
			RemoteItem: &libregraph.RemoteItem{
				DriveAlias: nil,
				ETag:       libregraph.PtrString(rsi.ResourceInfo.Etag),
				Folder:     &libregraph.Folder{},
				// The Id must correspond to the id in the OCS response, for the time being
				// It is in the form <something>!<something-else>
				Id:                   libregraph.PtrString(spaces.EncodeResourceID(rsi.ResourceInfo.Id)),
				LastModifiedDateTime: libregraph.PtrTime(time.Unix(int64(rsi.ResourceInfo.Mtime.Seconds), int64(rsi.ResourceInfo.Mtime.Nanos))),
				Name:                 libregraph.PtrString(filepath.Base(rsi.ResourceInfo.Path)),
				Path:                 libregraph.PtrString(resourceRelativePath),
				// RootId must have the same token before ! as Id
				// the second part for the time being is not used
				RootId: libregraph.PtrString(fmt.Sprintf("%s$%s!unused_root_id", rsi.ResourceInfo.Id.StorageId, rsi.ResourceInfo.Id.SpaceId)),
				Size:   libregraph.PtrInt64(int64(rsi.ResourceInfo.Size)),
			},
		},
		Special: []libregraph.DriveItem{},
	}

	if spacePath != "" {
		drive.Root.RemoteItem.DriveAlias = libregraph.PtrString(driveAlias)
	}

	return drive
}
