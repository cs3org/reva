package ocgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	linkv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/spaces"
	"github.com/go-chi/chi/v5"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"github.com/pkg/errors"
)

type ShareOrLink struct {
	shareType string // "share" or "link"
	ID        string
	link      *linkv1beta1.PublicShare
	share     *collaborationv1beta1.Share
}

func (s *svc) getDrivePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	resourceID, err := s.parseResourceID(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	actions, roles, perms, err := s.getPermissionsByCs3Reference(ctx, &providerpb.Reference{
		ResourceId: resourceID,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting permissions by cs3 reference")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.writePermissions(ctx, w, actions, roles, perms)
}

func (s *svc) getShareOrLink(ctx context.Context, shareID string, resourceId *providerpb.ResourceId) (*ShareOrLink, error) {
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
		return &ShareOrLink{
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
		return &ShareOrLink{
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

	// Decode the requested permissions

	// Unfortunately, we need to do something sketchy here: we can only read `r.Body` once
	// since it is a ReadCloser. However, when parsing its contents into a struct using
	// the json decoder, we lose info about whether a value was explicitly set to nil, or
	// whether it was just absent. For example, when removing the expiration time, the body
	// will have an entry like `expiration: nil`.
	// To fix this, we duplicate the stream using a TeeReader, and manually check for the
	// presence of certain fields to check if these should be updated

	// Buffer to store the copy
	var bodyCopy bytes.Buffer
	// Body stores a copy of the stream
	body := io.TeeReader(r.Body, &bodyCopy)

	permission := &libregraph.Permission{}
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(permission); err != nil {
		log.Error().Err(err).Interface("Body", r.Body).Msg("failed unmarshalling request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	permission.Id = libregraph.PtrString(shareID)

	if shareOrLink.shareType == "share" {
		s.updateSharePermissions(ctx, w, bodyCopy.String(), &collaborationv1beta1.ShareId{OpaqueId: shareOrLink.ID}, permission, resourceID)
	} else {
		s.updateLinkPermissions(ctx, w, bodyCopy.String(), &linkv1beta1.PublicShareId{OpaqueId: shareOrLink.ID}, permission, resourceID)
	}
}

func (s *svc) parseResourceID(r *http.Request) (*providerpb.ResourceId, error) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	resourceID := chi.URLParam(r, "resource-id")
	resourceID, _ = url.QueryUnescape(resourceID)
	storageID, _, itemID, ok := spaces.DecodeResourceID(resourceID)
	if !ok {
		log.Error().Str("resource-id", resourceID).Msg("resource id cannot be decoded")
		return nil, errtypes.BadRequest("resource id cannot be decoded")
	}
	return &providerpb.ResourceId{
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

func (s *svc) updateLinkPermissions(ctx context.Context, w http.ResponseWriter, requestBody string, linkId *linkv1beta1.PublicShareId, permission *libregraph.Permission, resourceId *providerpb.ResourceId) {
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	statRes, err := gw.Stat(ctx, &providerpb.StatRequest{
		Ref: &providerpb.Reference{
			ResourceId: resourceId,
		},
	})
	if err != nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Err(err).Msg("Failed to stat resource")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	update, err := getLinkUpdate(permission, &statRes.Info.Type, requestBody)
	if err != nil {
		log.Error().Err(err).Msg("nothing provided to update")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res, err := gw.UpdatePublicShare(ctx, &linkv1beta1.UpdatePublicShareRequest{
		Ref: &linkv1beta1.PublicShareReference{
			Spec: &linkv1beta1.PublicShareReference_Id{
				Id: linkId,
			},
		},
		Update: update,
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

	lgPerm, err := s.shareToLibregraphPerm(ctx, &ShareOrLink{
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

func (s *svc) updateSharePermissions(ctx context.Context, w http.ResponseWriter, requestBody string, shareId *collaborationv1beta1.ShareId, lgPerm *libregraph.Permission, resourceId *providerpb.ResourceId) {
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	statRes, err := gw.Stat(ctx, &providerpb.StatRequest{
		Ref: &providerpb.Reference{
			ResourceId: resourceId,
		},
	})
	if err != nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Err(err).Msg("Failed to stat resource")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	perms, err := s.lgPermToCS3Perm(ctx, lgPerm, statRes.Info.Type)
	if err != nil {
		log.Error().Err(err).Msg("nothing provided to update")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	res, err := gw.UpdateShare(ctx, &collaborationv1beta1.UpdateShareRequest{
		Ref: &collaborationv1beta1.ShareReference{
			Spec: &collaborationv1beta1.ShareReference_Id{
				Id: shareId,
			},
		},
		Field: &collaborationv1beta1.UpdateShareRequest_UpdateField{
			Field: &collaborationv1beta1.UpdateShareRequest_UpdateField_Permissions{
				Permissions: &collaborationv1beta1.SharePermissions{
					Permissions: perms,
				},
			},
		},
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

	lgPerm, err = s.shareToLibregraphPerm(ctx, &ShareOrLink{
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
	lgPerm, err := s.shareToLibregraphPerm(ctx, &ShareOrLink{
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

	actions, roles, perms, err := s.getPermissionsByCs3Reference(ctx, &providerpb.Reference{
		Path: path,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting permissions by cs3 reference")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.writePermissions(ctx, w, actions, roles, perms)
}

func (s *svc) getPermissionsByCs3Reference(ctx context.Context, ref *providerpb.Reference) (actions []string, roles []*libregraph.UnifiedRoleDefinition, perms []*libregraph.Permission, err error) {
	log := appctx.GetLogger(ctx)
	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		return nil, nil, nil, err
	}

	statRes, err := gw.Stat(ctx, &providerpb.StatRequest{
		Ref: ref,
	})
	if err != nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("ref", ref).Int("code", int(statRes.Status.Code)).Str("message", statRes.Status.Message).Msg("error statting resource")
		return nil, nil, nil, err
	}

	perms = make([]*libregraph.Permission, 0)

	shares, err := gw.ListShares(ctx, &collaborationv1beta1.ListSharesRequest{
		Filters: []*collaborationv1beta1.Filter{
			{
				Type: collaborationv1beta1.Filter_TYPE_RESOURCE_ID,
				Term: &collaborationv1beta1.Filter_ResourceId{
					ResourceId: statRes.Info.GetId(),
				},
			},
		},
	})
	if err != nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("ref", ref).Int("code", int(statRes.Status.Code)).Str("message", statRes.Status.Message).Msg("error getting shares for resource")
		return nil, nil, nil, err
	}
	for _, share := range shares.GetShares() {
		sharePerms, err := s.shareToLibregraphPerm(ctx, &ShareOrLink{
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

	links, err := gw.ListPublicShares(ctx, &linkv1beta1.ListPublicSharesRequest{
		Filters: []*linkv1beta1.ListPublicSharesRequest_Filter{
			{
				Type: linkv1beta1.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID,
				Term: &linkv1beta1.ListPublicSharesRequest_Filter_ResourceId{
					ResourceId: statRes.Info.GetId(),
				},
			},
		},
	})
	if err != nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("ref", ref).Int("code", int(statRes.Status.Code)).Str("message", statRes.Status.Message).Msg("error getting links for resource")
		return nil, nil, nil, err
	}

	for _, link := range links.Share {
		linkPerms, err := s.shareToLibregraphPerm(ctx, &ShareOrLink{
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
	roles = GetApplicableRoleDefinitionsForActions(actions)

	return actions, roles, perms, nil

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

func getLinkUpdate(permission *libregraph.Permission, resourceType *providerpb.ResourceType, body string) (*linkv1beta1.UpdatePublicShareRequest_Update, error) {
	if strings.Contains(body, "expiration") {
		return &linkv1beta1.UpdatePublicShareRequest_Update{
			Type: linkv1beta1.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
			Grant: &linkv1beta1.Grant{
				Expiration: nullableTimeToCs3Timestamp(permission.ExpirationDateTime),
			},
		}, nil
	} else if permission.Link != nil && permission.Link.Type != nil {
		permissions, err := CS3ResourcePermissionsFromSharingLink(permission.Link.GetType(), *resourceType)
		if err != nil {
			return nil, errors.Wrap(err, "error converting link type to permissions")
		}
		return &linkv1beta1.UpdatePublicShareRequest_Update{
			Type: linkv1beta1.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
			Grant: &linkv1beta1.Grant{
				Permissions: &linkv1beta1.PublicSharePermissions{
					Permissions: permissions,
				},
			},
		}, nil
	} else {
		return nil, errors.New("body contained nothing to update")
	}
}
