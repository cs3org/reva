package ocgraph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	linkv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/go-chi/chi/v5"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"github.com/pkg/errors"
)

type GenericShare struct {
	shareType string // "share", "link" or "ocmshare"
	ID        string
	link      *linkv1beta1.PublicShare
	share     *collaborationv1beta1.Share
	ocmshare  *ocm.Share
}

func (s *svc) getDrivePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	resourceID, err := s.parseResourceID(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	actions, roles, perms, err := s.getPermissionsByCs3Reference(ctx, &provider.Reference{
		ResourceId: resourceID,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting permissions by cs3 reference")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.writePermissions(ctx, w, actions, roles, perms)
}

func (s *svc) getShareOrLink(ctx context.Context, shareID string, resourceId *provider.ResourceId) (*GenericShare, error) {
	log := appctx.GetLogger(ctx)
	// Next, we need to determine if it is a link or a permission update request
	// we try to get a share, if this succeeds, it's a share, otherwise we assume it's a link
	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		return nil, err
	}

	share, err := gw.GetShare(ctx, &collaborationv1beta1.GetShareRequest{
		Ref: &collaborationv1beta1.ShareReference{
			Spec: &collaborationv1beta1.ShareReference_Id{
				Id: &collaborationv1beta1.ShareId{
					OpaqueId: shareID,
				},
			},
		},
	})

	if err == nil && share != nil && share.Status.Code == rpcv1beta1.Code_CODE_OK {
		if share.Share.ResourceId.StorageId != resourceId.StorageId || share.Share.ResourceId.OpaqueId != resourceId.OpaqueId {
			log.Error().Str("share-id", shareID).Str("resource-id", resourceId.String()).Msg("share does not match resource id")
			return nil, errtypes.BadRequest("share id does not match resource id")
		}
		return &GenericShare{
			shareType: "share",
			ID:        shareID,
			share:     share.Share,
		}, nil
	}

	link, err := gw.GetPublicShare(ctx, &linkv1beta1.GetPublicShareRequest{
		Ref: &linkv1beta1.PublicShareReference{
			Spec: &linkv1beta1.PublicShareReference_Id{
				Id: &linkv1beta1.PublicShareId{
					OpaqueId: shareID,
				},
			},
		},
	})

	if err == nil && link != nil && link.Status.Code == rpcv1beta1.Code_CODE_OK {
		if link.Share.ResourceId.StorageId != resourceId.StorageId || link.Share.ResourceId.OpaqueId != resourceId.OpaqueId {
			log.Error().Str("share-id", shareID).Str("resource-id", resourceId.String()).Msg("link does not match resource id")
			return nil, errtypes.BadRequest("share id does not match resource id")

		}
		return &GenericShare{
			shareType: "link",
			ID:        shareID,
			link:      link.Share,
		}, nil
	}

	return nil, err
}

func (s *svc) updateDrivePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	resourceID, err := s.parseResourceID(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shareID := chi.URLParam(r, "share-id")
	shareID, _ = url.QueryUnescape(shareID)
	if shareID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shareOrLink, err := s.getShareOrLink(ctx, shareID, resourceID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	permission := &libregraph.Permission{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(permission); err != nil {
		log.Error().Err(err).Interface("Body", r.Body).Msg("failed unmarshalling request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	permission.Id = libregraph.PtrString(shareID)

	if shareOrLink.shareType == "share" {
		s.updateSharePermissions(ctx, w, shareOrLink.share, permission, resourceID)
	} else {
		s.updateLinkPermissions(ctx, w, shareOrLink.link, permission, resourceID)
	}
}

func (s *svc) parseResourceID(r *http.Request) (*provider.ResourceId, error) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	resourceID := chi.URLParam(r, "resource-id")
	resourceID, _ = url.QueryUnescape(resourceID)
	storageID, _, itemID, ok := spaces.DecodeResourceID(resourceID)
	if !ok {
		log.Error().Str("resource-id", resourceID).Msg("resource id cannot be decoded")
		return nil, errtypes.BadRequest("resource id cannot be decoded")
	}
	return &provider.ResourceId{
		StorageId: storageID,
		OpaqueId:  itemID,
	}, nil
}

func (s *svc) deleteDrivePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resourceID, err := s.parseResourceID(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shareID := chi.URLParam(r, "share-id")
	shareID, _ = url.QueryUnescape(shareID)
	if shareID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shareOrLink, err := s.getShareOrLink(ctx, shareID, resourceID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if shareOrLink.shareType == "share" {
		s.deleteSharePermissions(ctx, w, r, &collaborationv1beta1.ShareId{OpaqueId: shareOrLink.ID})
	} else {
		s.deleteLinkPermissions(ctx, w, r, &linkv1beta1.PublicShareId{OpaqueId: shareOrLink.ID})
	}
}

func (s *svc) updateLinkPermissions(ctx context.Context, w http.ResponseWriter, link *linkv1beta1.PublicShare, permission *libregraph.Permission, resourceId *provider.ResourceId) {
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	statRes, err := gw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: resourceId,
		},
	})
	if err != nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Err(err).Msg("Failed to stat resource")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	updates, err := s.getLinkUpdates(ctx, link, permission, statRes.Info.Type)
	if err != nil {
		log.Error().Err(err).Msg("nothing provided to update")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	uRes := &linkv1beta1.UpdatePublicShareResponse{}
	lgPerm := &libregraph.Permission{}

	for _, update := range updates {
		uRes, err = gw.UpdatePublicShare(ctx, &linkv1beta1.UpdatePublicShareRequest{
			Ref: &linkv1beta1.PublicShareReference{
				Spec: &linkv1beta1.PublicShareReference_Id{
					Id: link.Id,
				},
			},
			Update: update,
		})
		if err != nil {
			log.Error().Err(err).Msg("error updating public share")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if uRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			log.Error().Interface("response", uRes).Msg("error updating public share")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		lgPerm, err = s.shareToLibregraphPerm(ctx, &GenericShare{
			shareType: "link",
			ID:        uRes.GetShare().GetId().GetOpaqueId(),
			link:      uRes.GetShare(),
		})
		if err != nil || lgPerm == nil {
			log.Error().Err(err).Any("link", uRes.GetShare()).Err(err).Any("lgPerm", lgPerm).Msg("error converting created link to permissions")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	_ = json.NewEncoder(w).Encode(lgPerm)
}

func (s *svc) updateSharePermissions(ctx context.Context, w http.ResponseWriter, share *collaborationv1beta1.Share, lgPerm *libregraph.Permission, resourceId *provider.ResourceId) {
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	statRes, err := gw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: resourceId,
		},
	})
	if err != nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Err(err).Msg("Failed to stat resource")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	update, err := s.getShareUpdate(ctx, lgPerm, statRes.Info.Type)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res, err := gw.UpdateShare(ctx, &collaborationv1beta1.UpdateShareRequest{
		Ref: &collaborationv1beta1.ShareReference{
			Spec: &collaborationv1beta1.ShareReference_Id{
				Id: share.Id,
			},
		},
		Field: update,
	})
	if err != nil {
		log.Error().Err(err).Msg("error updating public share")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("response", res).Msg("error updating public share")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	lgPerm, err = s.shareToLibregraphPerm(ctx, &GenericShare{
		shareType: "share",
		ID:        res.GetShare().GetId().GetOpaqueId(),
		share:     res.GetShare(),
	})
	if err != nil || lgPerm == nil {
		log.Error().Err(err).Any("link", res.GetShare()).Err(err).Any("lgPerm", lgPerm).Msg("error converting created link to permissions")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(lgPerm)
}

func (s *svc) updateLinkPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	resourceID, err := s.parseResourceID(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shareID := chi.URLParam(r, "share-id")
	shareID, _ = url.QueryUnescape(shareID)
	if shareID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shareOrLink, err := s.getShareOrLink(ctx, shareID, resourceID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if shareOrLink.shareType != "link" {
		w.WriteHeader(http.StatusNotFound)
	}

	password := &libregraph.SharingLinkPassword{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(password); err != nil {
		log.Error().Err(err).Interface("Body", r.Body).Msg("failed unmarshalling request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	res, err := gw.UpdatePublicShare(ctx, &linkv1beta1.UpdatePublicShareRequest{
		Ref: &linkv1beta1.PublicShareReference{
			Spec: &linkv1beta1.PublicShareReference_Id{
				Id: &linkv1beta1.PublicShareId{
					OpaqueId: shareOrLink.ID,
				},
			},
		},
		Update: &linkv1beta1.UpdatePublicShareRequest_Update{
			Type: linkv1beta1.UpdatePublicShareRequest_Update_TYPE_PASSWORD,
			Grant: &linkv1beta1.Grant{
				Password: password.GetPassword(),
			},
		},
	})

	if err != nil || res.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Err(err).Interface("response", res).Msg("error updating public share password")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	lgPerm, err := s.shareToLibregraphPerm(ctx, &GenericShare{
		shareType: "link",
		ID:        res.GetShare().GetId().GetOpaqueId(),
		link:      res.GetShare(),
	})
	if err != nil || lgPerm == nil {
		log.Error().Err(err).Any("link", res.GetShare()).Err(err).Any("lgPerm", lgPerm).Msg("error converting created link to permissions")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(lgPerm)
}

func (s *svc) deleteLinkPermissions(ctx context.Context, w http.ResponseWriter, r *http.Request, linkId *linkv1beta1.PublicShareId) {
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	res, err := gw.RemovePublicShare(ctx, &linkv1beta1.RemovePublicShareRequest{
		Ref: &linkv1beta1.PublicShareReference{
			Spec: &linkv1beta1.PublicShareReference_Id{
				Id: linkId,
			},
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("error removing public share")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("response", res).Msg("error removing public share")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *svc) deleteSharePermissions(ctx context.Context, w http.ResponseWriter, r *http.Request, shareId *collaborationv1beta1.ShareId) {
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	res, err := gw.RemoveShare(ctx, &collaborationv1beta1.RemoveShareRequest{
		Ref: &collaborationv1beta1.ShareReference{
			Spec: &collaborationv1beta1.ShareReference_Id{
				Id: shareId,
			},
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("error removing share")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("response", res).Msg("error removing share")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *svc) getRootDrivePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	spaceID := chi.URLParam(r, "space-id")
	spaceID, _ = url.QueryUnescape(spaceID)
	_, path, ok := spaces.DecodeStorageSpaceID(spaceID)
	if !ok {
		log.Error().Str("space-id", spaceID).Msg("space id cannot be decoded")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	actions, roles, perms, err := s.getPermissionsByCs3Reference(ctx, &provider.Reference{
		Path: path,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting permissions by cs3 reference")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.writePermissions(ctx, w, actions, roles, perms)
}

func (s *svc) getPermissionsByCs3Reference(ctx context.Context, ref *provider.Reference) (actions []string, roles []*libregraph.UnifiedRoleDefinition, perms []*libregraph.Permission, err error) {
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		return nil, nil, nil, err
	}

	statRes, err := gw.Stat(ctx, &provider.StatRequest{
		Ref: ref,
	})
	if err != nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("ref", ref).Int("code", int(statRes.Status.Code)).Str("message", statRes.Status.Message).Msg("error statting resource")
		return nil, nil, nil, err
	}
	perms = make([]*libregraph.Permission, 0)
	shares, err := s.getSharesForResource(ctx, gw, statRes.Info)
	// TODO: error handling?

	for _, share := range shares {
		sharePerms, err := s.shareToLibregraphPerm(ctx, &GenericShare{
			shareType: "share",
			ID:        share.GetId().GetOpaqueId(),
			share:     share,
		})
		if err == nil {
			perms = append(perms, sharePerms)
		} else {
			log.Error().Err(err).Any("share", share).Msg("error converting share to libregraph permission")
		}
	}

	links, err := s.getPublicSharesForResource(ctx, gw, statRes.Info)
	// TODO: error handling?

	for _, link := range links {
		linkPerms, err := s.shareToLibregraphPerm(ctx, &GenericShare{
			shareType: "link",
			ID:        link.GetId().GetOpaqueId(),
			link:      link,
		})
		if err == nil {
			perms = append(perms, linkPerms)
		} else {
			log.Error().Err(err).Any("link", link).Msg("error converting link to libregraph permission")
		}
	}

	actions = CS3ResourcePermissionsToLibregraphActions(statRes.Info.PermissionSet)
	roles = GetApplicableRoleDefinitionsForActions(actions, statRes.Info)

	return actions, roles, perms, nil
}

func (s *svc) getSharesForResource(ctx context.Context, gw gatewayv1beta1.GatewayAPIClient, ri *provider.ResourceInfo) ([]*collaborationv1beta1.Share, error) {
	log := appctx.GetLogger(ctx)
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return nil, errors.New("no user provided")
	}

	req := &collaborationv1beta1.ListSharesRequest{
		Filters: []*collaborationv1beta1.Filter{
			{
				Type: collaborationv1beta1.Filter_TYPE_RESOURCE_ID,
				Term: &collaborationv1beta1.Filter_ResourceId{
					ResourceId: ri.Id,
				},
			},
		},
	}

	// If we are not in a project, or a project where
	// the user is not an admin, we filter for shares belonging to the user
	if !s.userHasAdminAccessToProject(ctx, ri) {
		req.Filters = append(req.Filters, &collaborationv1beta1.Filter{
			Type: collaborationv1beta1.Filter_TYPE_CREATOR,
			Term: &collaborationv1beta1.Filter_Creator{
				Creator: user.Id,
			},
		})
	}

	shareRes, err := gw.ListShares(ctx, req)

	if err != nil {
		return nil, err
	}
	if shareRes.Status != nil && shareRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("ref", ri.Id).Int("code", int(shareRes.Status.Code)).Str("message", shareRes.Status.Message).Msg("error getting shares for resource")
		return nil, err
	}

	return shareRes.Shares, nil
}

func (s *svc) getPublicSharesForResource(ctx context.Context, gw gatewayv1beta1.GatewayAPIClient, ri *provider.ResourceInfo) ([]*linkv1beta1.PublicShare, error) {
	log := appctx.GetLogger(ctx)
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return nil, errors.New("no user provided")
	}

	req := &linkv1beta1.ListPublicSharesRequest{
		Filters: []*linkv1beta1.ListPublicSharesRequest_Filter{
			{
				Type: linkv1beta1.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID,
				Term: &linkv1beta1.ListPublicSharesRequest_Filter_ResourceId{
					ResourceId: ri.Id,
				},
			},
		},
	}

	// If we are not in a project, or a project where
	// the user does not have ListGrant rights
	if !s.userHasAdminAccessToProject(ctx, ri) {
		req.Filters = append(req.Filters, &linkv1beta1.ListPublicSharesRequest_Filter{
			Type: linkv1beta1.ListPublicSharesRequest_Filter_TYPE_CREATOR,
			Term: &linkv1beta1.ListPublicSharesRequest_Filter_Creator{
				Creator: user.Id,
			},
		})
	}

	linksRes, err := gw.ListPublicShares(ctx, req)
	if err != nil {
		return nil, err
	}
	if linksRes.Status != nil || linksRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("ref", ri.Id).Int("code", int(linksRes.Status.Code)).Str("message", linksRes.Status.Message).Msg("error getting links for resource")
		return nil, err
	}

	return linksRes.Share, nil
}

func (s *svc) userHasAdminAccessToProject(ctx context.Context, ri *provider.ResourceInfo) bool {
	if ri.Space == nil {
		return false
	}
	if ri.Space.SpaceType != spaces.SpaceTypeProject.AsString() {
		return false
	}
	if ri.Space.PermissionSet != nil && ri.Space.PermissionSet.ListGrants {
		return true
	}
	return false
}

func (s *svc) writePermissions(ctx context.Context, w http.ResponseWriter, actions []string, roles []*libregraph.UnifiedRoleDefinition, perms []*libregraph.Permission) {
	if err := json.NewEncoder(w).Encode(map[string]any{
		"@libre.graph.permissions.actions.allowedValues": actions,
		"@libre.graph.permissions.roles.allowedValues":   roles,
		"value": perms,
	}); err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Msg("error marshalling permissions as json")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *svc) getLinkUpdates(ctx context.Context, link *linkv1beta1.PublicShare, permission *libregraph.Permission, resourceType provider.ResourceType) ([]*linkv1beta1.UpdatePublicShareRequest_Update, error) {
	updates := []*linkv1beta1.UpdatePublicShareRequest_Update{}

	defaultExpirationDefined := time.Second * time.Duration(s.c.PubRWLinkDefaultExpiration)
	maxExpirationDefined := time.Second * time.Duration(s.c.PubRWLinkMaxExpiration)
	endOfDay := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 23, 59, 59, 0, time.Now().Location())
	defaultExpiration := endOfDay.Add(defaultExpirationDefined)
	maxExpiration := endOfDay.Add(maxExpirationDefined)

	isExpirationEnforced := maxExpirationDefined > 0
	isEditorLink := false
	if permission.Link != nil && permission.Link.Type != nil {
		isEditorLink = permission.Link.GetType() == libregraph.EDIT
	} else if link.Permissions != nil {
		isEditorLink = conversions.RoleFromResourcePermissions(link.Permissions.Permissions).Name == conversions.RoleEditor
	}

	if permission.ExpirationDateTime.IsSet() {
		finalExpiration := permission.ExpirationDateTime
		if isEditorLink && isExpirationEnforced {
			if permission.ExpirationDateTime.Get() == nil {
				finalExpiration.Set(&defaultExpiration)
			} else if permission.ExpirationDateTime.Get().After(maxExpiration) {
				finalExpiration.Set(&maxExpiration)
			}
		}
		updates = append(updates, &linkv1beta1.UpdatePublicShareRequest_Update{
			Type: linkv1beta1.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
			Grant: &linkv1beta1.Grant{
				Expiration: nullableTimeToCs3Timestamp(finalExpiration),
			},
		})
	}
	if permission.Link != nil && permission.Link.LibreGraphDisplayName != nil {
		if permission.Link.LibreGraphDisplayName == nil || *permission.Link.LibreGraphDisplayName == "" {
			return nil, errtypes.BadRequest("Link name cannot be empty")
		}
		updates = append(updates, &linkv1beta1.UpdatePublicShareRequest_Update{
			Type:        linkv1beta1.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME,
			DisplayName: *permission.Link.LibreGraphDisplayName,
		})
	}
	if permission.Link != nil && permission.Link.Type != nil {
		permissions, err := CS3ResourcePermissionsFromSharingLink(permission.Link.GetType(), resourceType)
		if err != nil {
			return nil, errors.Wrap(err, "error converting link type to permissions")
		}

		finalExpiration := libregraph.NullableTime{}
		if isEditorLink && isExpirationEnforced {
			if !permission.ExpirationDateTime.IsSet() && link.Expiration == nil {
				finalExpiration.Set(&defaultExpiration)
			} else if permission.ExpirationDateTime.IsSet() {
				if permission.ExpirationDateTime.Get() != nil && permission.ExpirationDateTime.Get().After(maxExpiration) {
					finalExpiration.Set(&maxExpiration)
				}
			} else if link.Expiration != nil {
				if endOfDay.Add(time.Second * time.Duration(link.Expiration.Seconds)).After(maxExpiration) {
					finalExpiration.Set(&maxExpiration)
				}
			}
		}

		updates = append(updates, &linkv1beta1.UpdatePublicShareRequest_Update{
			Type: linkv1beta1.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
			Grant: &linkv1beta1.Grant{
				Permissions: &linkv1beta1.PublicSharePermissions{
					Permissions: permissions,
				},
			},
		})
		if finalExpiration.IsSet() {
			updates = append(updates, &linkv1beta1.UpdatePublicShareRequest_Update{
				Type: linkv1beta1.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
				Grant: &linkv1beta1.Grant{
					Expiration: nullableTimeToCs3Timestamp(finalExpiration),
				},
			})
		}
	}
	if len(updates) == 0 {
		return nil, errors.New("body contained nothing to update")
	}
	return updates, nil
}

func (s *svc) getShareUpdate(ctx context.Context, permission *libregraph.Permission, resourceType provider.ResourceType) (*collaborationv1beta1.UpdateShareRequest_UpdateField, error) {
	if permission.ExpirationDateTime.IsSet() {
		return &collaborationv1beta1.UpdateShareRequest_UpdateField{
			Field: &collaborationv1beta1.UpdateShareRequest_UpdateField_Expiration{
				Expiration: nullableTimeToCs3Timestamp(permission.ExpirationDateTime),
			},
		}, nil
	}
	perms, err := s.lgPermToCS3Perm(ctx, permission, resourceType)
	if err != nil || perms == nil {
		return nil, errors.New("Failed to extract permissions")
	}
	return &collaborationv1beta1.UpdateShareRequest_UpdateField{
		Field: &collaborationv1beta1.UpdateShareRequest_UpdateField_Permissions{
			Permissions: &collaborationv1beta1.SharePermissions{
				Permissions: perms,
			},
		},
	}, nil
}
