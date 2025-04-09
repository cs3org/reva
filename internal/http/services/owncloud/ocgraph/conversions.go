package ocgraph

import (
	"path"
	"time"

	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	libregraph "github.com/owncloud/libre-graph-api-go"
)

func (s *svc) libreGraphPermissionFromCS3PublicShare(createdLink *link.PublicShare) *libregraph.Permission {
	lt, actions := SharingLinkTypeFromCS3Permissions(createdLink.GetPermissions())

	perm := libregraph.NewPermission()
	perm.Id = libregraph.PtrString(createdLink.GetId().GetOpaqueId())
	perm.Link = &libregraph.SharingLink{
		Type:                  lt,
		PreventsDownload:      libregraph.PtrBool(false),
		LibreGraphDisplayName: libregraph.PtrString(createdLink.GetDisplayName()),
		LibreGraphQuickLink:   libregraph.PtrBool(createdLink.GetQuicklink()),
	}
	perm.LibreGraphPermissionsActions = actions

	// TODO: this is wrong; results in https:/cbox-ocisdev-diogo.cern.ch/files/spaces/s/1Yp1g8i6MLQkUmz
	// instead of https:/cbox-ocisdev-diogo.cern.ch/s/1Yp1g8i6MLQkUmz
	webURL := path.Join(s.c.WebBase, "s", createdLink.GetToken())
	perm.Link.SetWebUrl(webURL)

	// set expiration date
	if createdLink.GetExpiration() != nil {
		perm.SetExpirationDateTime(cs3TimestampToTime(createdLink.GetExpiration()).UTC())
	}

	// set cTime
	if createdLink.GetCtime() != nil {
		perm.SetCreatedDateTime(cs3TimestampToTime(createdLink.GetCtime()).UTC())
	}

	perm.SetHasPassword(createdLink.GetPasswordProtected())

	return perm
}

func cs3TimestampToTime(t *types.Timestamp) time.Time {
	return time.Unix(int64(t.GetSeconds()), int64(t.GetNanos()))
}

func LinkTypeToPermissions(lt libregraph.SharingLinkType) *provider.ResourcePermissions {
	switch lt {
	case libregraph.VIEW:
		return conversions.NewViewerRole().CS3ResourcePermissions()
	case libregraph.EDIT:
		return conversions.NewEditorRole().CS3ResourcePermissions()
	case libregraph.UPLOAD:
		return conversions.NewUploaderRole().CS3ResourcePermissions()
	case libregraph.BLOCKS_DOWNLOAD:
		fallthrough
	case libregraph.INTERNAL:
		fallthrough
	default:
		return conversions.NewDeniedRole().CS3ResourcePermissions()
	}
}
