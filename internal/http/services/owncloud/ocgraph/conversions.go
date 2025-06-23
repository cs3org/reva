package ocgraph

import (
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	libregraph "github.com/owncloud/libre-graph-api-go"
)

// func (s *svc) libreGraphPermissionFromCS3PublicShare(ctx context.Context, createdLink *link.PublicShare) *libregraph.Permission {
// 	lt, actions := SharingLinkTypeFromCS3Permissions(ctx, createdLink.GetPermissions())
// 	baseURI := s.c.BaseURL

// 	perm := libregraph.NewPermission()
// 	perm.Id = libregraph.PtrString(createdLink.GetId().GetOpaqueId())
// 	perm.Link = &libregraph.SharingLink{
// 		Type:                  lt,
// 		PreventsDownload:      libregraph.PtrBool(false),
// 		LibreGraphDisplayName: libregraph.PtrString(createdLink.GetDisplayName()),
// 		LibreGraphQuickLink:   libregraph.PtrBool(createdLink.GetQuicklink()),
// 	}
// 	perm.LibreGraphPermissionsActions = actions

// 	webURL := path.Join(baseURI, "s", createdLink.GetToken())
// 	perm.Link.SetWebUrl(webURL)

// 	// set expiration date
// 	if createdLink.GetExpiration() != nil {
// 		perm.SetExpirationDateTime(cs3TimestampToTime(createdLink.GetExpiration()).UTC())
// 	}

// 	// set cTime
// 	if createdLink.GetCtime() != nil {
// 		perm.SetCreatedDateTime(cs3TimestampToTime(createdLink.GetCtime()).UTC())
// 	}

// 	perm.SetHasPassword(createdLink.GetPasswordProtected())

// 	return perm
// }

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
