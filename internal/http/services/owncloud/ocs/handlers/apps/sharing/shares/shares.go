// Copyright 2018-2023 CERN
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

package shares

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/notification"
	"github.com/cs3org/reva/pkg/notification/notificationhelper"
	"github.com/cs3org/reva/pkg/notification/trigger"
	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/cache"
	cachereg "github.com/cs3org/reva/pkg/share/cache/registry"
	warmupreg "github.com/cs3org/reva/pkg/share/cache/warmup/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/resourceid"
	"github.com/go-chi/chi/v5"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const (
	storageIDPrefix string = "shared::"
)

// Handler implements the shares part of the ownCloud sharing API.
type Handler struct {
	gatewayAddr            string
	storageRegistryAddr    string
	publicURL              string
	sharePrefix            string
	homeNamespace          string
	ocmMountPoint          string
	additionalInfoTemplate *template.Template
	userIdentifierCache    *ttlcache.Cache
	resourceInfoCache      cache.ResourceInfoCache
	resourceInfoCacheTTL   time.Duration
	listOCMShares          bool
	notificationHelper     *notificationhelper.NotificationHelper
	Log                    *zerolog.Logger
}

// we only cache the minimal set of data instead of the full user metadata.
type userIdentifiers struct {
	DisplayName string
	Username    string
	Mail        string
}

func getCacheWarmupManager(c *config.Config) (cache.Warmup, error) {
	if f, ok := warmupreg.NewFuncs[c.CacheWarmupDriver]; ok {
		return f(c.CacheWarmupDrivers[c.CacheWarmupDriver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.CacheWarmupDriver)
}

func getCacheManager(c *config.Config) (cache.ResourceInfoCache, error) {
	if f, ok := cachereg.NewFuncs[c.ResourceInfoCacheDriver]; ok {
		return f(c.ResourceInfoCacheDrivers[c.ResourceInfoCacheDriver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.ResourceInfoCacheDriver)
}

// Init initializes this and any contained handlers.
func (h *Handler) Init(c *config.Config, l *zerolog.Logger) {
	h.gatewayAddr = c.GatewaySvc
	h.storageRegistryAddr = c.StorageregistrySvc
	h.publicURL = c.Config.Host
	h.sharePrefix = c.SharePrefix
	h.homeNamespace = c.HomeNamespace
	h.ocmMountPoint = c.OCMMountPoint
	h.listOCMShares = c.ListOCMShares
	h.Log = l
	h.notificationHelper = notificationhelper.New("ocs", c.Notifications, l)
	h.additionalInfoTemplate, _ = template.New("additionalInfo").Parse(c.AdditionalInfoAttribute)
	h.resourceInfoCacheTTL = time.Second * time.Duration(c.ResourceInfoCacheTTL)

	h.userIdentifierCache = ttlcache.NewCache()
	_ = h.userIdentifierCache.SetTTL(time.Second * time.Duration(c.UserIdentifierCacheTTL))

	cache, err := getCacheManager(c)
	if err == nil {
		h.resourceInfoCache = cache
	}

	if h.resourceInfoCacheTTL > 0 {
		cwm, err := getCacheWarmupManager(c)
		if err == nil {
			go h.startCacheWarmup(cwm)
		}
	}
}

func (h *Handler) startCacheWarmup(c cache.Warmup) {
	time.Sleep(2 * time.Second)
	infos, err := c.GetResourceInfos()
	if err != nil {
		return
	}
	for _, r := range infos {
		key := resourceid.OwnCloudResourceIDWrap(r.Id)
		_ = h.resourceInfoCache.SetWithExpire(key, r, h.resourceInfoCacheTTL)
	}
}

func (h *Handler) extractReference(r *http.Request) (provider.Reference, error) {
	var ref provider.Reference
	if p := r.FormValue("path"); p != "" {
		ref = provider.Reference{Path: path.Join(h.homeNamespace, p)}
	} else if spaceRef := r.FormValue("space_ref"); spaceRef != "" {
		var err error
		ref, err = utils.ParseStorageSpaceReference(spaceRef)
		if err != nil {
			return provider.Reference{}, err
		}
	}
	return ref, nil
}

// CreateShare handles POST requests on /apps/files_sharing/api/v1/shares.
func (h *Handler) CreateShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	shareType, err := strconv.Atoi(r.FormValue("shareType"))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "shareType must be an integer", nil)
		return
	}
	// get user permissions on the shared file

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	ref, err := h.extractReference(r)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "could not parse the reference", fmt.Errorf("could not parse the reference"))
		return
	}

	statReq := provider.StatRequest{
		Ref: &ref,
	}

	log := appctx.GetLogger(ctx).With().Interface("ref", ref).Logger()

	statRes, err := client.Stat(ctx, &statReq)
	if err != nil {
		log.Debug().Err(err).Str("createShare", "shares").Msg("error on stat call")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing resource information", fmt.Errorf("error getting resource information"))
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		ocdav.HandleErrorStatus(&log, w, statRes.Status)
		return
	}

	// check user has share permissions
	if !conversions.RoleFromResourcePermissions(statRes.Info.PermissionSet).OCSPermissions().Contain(conversions.PermissionShare) {
		response.WriteOCSError(w, r, http.StatusNotFound, "No share permission", nil)
		return
	}

	switch shareType {
	case int(conversions.ShareTypeUser):
		// user collaborations default to coowner
		if role, val, err := h.extractPermissions(w, r, statRes.Info, conversions.NewCoownerRole()); err == nil {
			h.createUserShare(w, r, statRes.Info, role, val)
		}
	case int(conversions.ShareTypeGroup):
		// group collaborations default to coowner
		if role, val, err := h.extractPermissions(w, r, statRes.Info, conversions.NewCoownerRole()); err == nil {
			h.createGroupShare(w, r, statRes.Info, role, val)
		}
	case int(conversions.ShareTypePublicLink):
		// public links default to read only
		if _, _, err := h.extractPermissions(w, r, statRes.Info, conversions.NewViewerRole()); err == nil {
			h.createPublicLinkShare(w, r, statRes.Info)
		}
	case int(conversions.ShareTypeFederatedCloudShare):
		// federated shares default to read only
		if role, val, err := h.extractPermissions(w, r, statRes.Info, conversions.NewViewerRole()); err == nil {
			h.createFederatedCloudShare(w, r, statRes.Info, role, val)
		}
	case int(conversions.ShareTypeSpaceMembership):
		if role, val, err := h.extractPermissions(w, r, statRes.Info, conversions.NewViewerRole()); err == nil {
			switch role.Name {
			case conversions.RoleManager, conversions.RoleEditor, conversions.RoleViewer:
				h.addSpaceMember(w, r, statRes.Info, role, val)
			default:
				response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "invalid role for space member", nil)
				return
			}
		}
	default:
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "unknown share type", nil)
	}
}

// NotifyShare handles GET requests on /apps/files_sharing/api/v1/shares/(shareid)/notify.
func (h *Handler) NotifyShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opaqueID := chi.URLParam(r, "shareid")

	c, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	shareRes, err := c.GetShare(ctx, &collaboration.GetShareRequest{
		Ref: &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: opaqueID,
				},
			},
		},
	})
	if err != nil || shareRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
		h.Log.Error().Err(err).Msg("error getting share")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting share", err)
		return
	}

	granter, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		h.Log.Error().Err(err).Msgf("error getting granter data")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting granter data", err)
	}

	resourceID := shareRes.Share.ResourceId
	statInfo, status, err := h.getResourceInfoByID(ctx, c, resourceID)
	if err != nil || status.Code != rpc.Code_CODE_OK {
		h.Log.Error().Err(err).Msg("error mapping share data")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}

	var recipient string

	granteeType := shareRes.Share.Grantee.Type
	if granteeType == provider.GranteeType_GRANTEE_TYPE_USER {
		granteeID := shareRes.Share.Grantee.GetUserId().OpaqueId
		granteeRes, err := c.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
			Claim:                  "username",
			Value:                  granteeID,
			SkipFetchingUserGroups: true,
		})
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grantee data", err)
			return
		}

		recipient = h.SendShareNotification(opaqueID, granter, granteeRes.User, statInfo)
	} else if granteeType == provider.GranteeType_GRANTEE_TYPE_GROUP {
		granteeID := shareRes.Share.Grantee.GetGroupId().OpaqueId
		granteeRes, err := c.GetGroupByClaim(ctx, &grouppb.GetGroupByClaimRequest{
			Claim:               "group_name",
			Value:               granteeID,
			SkipFetchingMembers: true,
		})
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grantee data", err)
			return
		}

		recipient = h.SendShareNotification(opaqueID, granter, granteeRes.Group, statInfo)
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	rb, _ := json.Marshal(map[string]interface{}{"recipients": []string{recipient}})
	_, err = w.Write(rb)
	if err != nil {
		h.Log.Error().Err(err).Msg("error writing response")
	}
}

// SendShareNotification sends a notification with information from a Share.
func (h *Handler) SendShareNotification(opaqueID string, granter *userpb.User, grantee interface{}, statInfo *provider.ResourceInfo) string {
	var granteeDisplayName, granteeName, recipient string
	isGranteeGroup := false

	if u, ok := grantee.(*userpb.User); ok {
		granteeDisplayName = u.DisplayName
		granteeName = u.Username
		recipient = u.Mail
	} else if g, ok := grantee.(*grouppb.Group); ok {
		granteeDisplayName = g.DisplayName
		granteeName = g.GroupName
		recipient = g.Mail
		isGranteeGroup = true
	}

	h.notificationHelper.TriggerNotification(&trigger.Trigger{
		Notification: &notification.Notification{
			TemplateName: "share-create-mail",
			Ref:          opaqueID,
			Recipients:   []string{recipient},
		},
		Ref: opaqueID,
		TemplateData: map[string]interface{}{
			"granteeDisplayName": granteeDisplayName,
			"granteeUserName":    granteeName,
			"granterDisplayName": granter.DisplayName,
			"granterUserName":    granter.Username,
			"path":               statInfo.Path,
			"isFolder":           statInfo.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			"isGranteeGroup":     isGranteeGroup,
			"base":               filepath.Base(statInfo.Path),
		},
	})
	h.Log.Debug().Msgf("notification trigger %s created", opaqueID)

	return recipient
}

func (h *Handler) extractPermissions(w http.ResponseWriter, r *http.Request, ri *provider.ResourceInfo, defaultPermissions *conversions.Role) (*conversions.Role, []byte, error) {
	reqRole, reqPermissions := r.FormValue("role"), r.FormValue("permissions")
	var role *conversions.Role

	// the share role overrides the requested permissions
	if reqRole != "" {
		role = conversions.RoleFromName(reqRole)
	} else {
		// map requested permissions
		if reqPermissions == "" {
			// TODO default link vs user share
			role = defaultPermissions
		} else {
			pint, err := strconv.Atoi(reqPermissions)
			if err != nil {
				response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "permissions must be an integer", nil)
				return nil, nil, err
			}
			perm, err := conversions.NewPermissions(pint)
			if err != nil {
				if err == conversions.ErrPermissionNotInRange {
					response.WriteOCSError(w, r, http.StatusNotFound, err.Error(), nil)
				} else {
					response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, err.Error(), nil)
				}
				return nil, nil, err
			}
			role = conversions.RoleFromOCSPermissions(perm)
		}
	}

	permissions := role.OCSPermissions()
	if ri != nil && ri.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
		// Single file shares should never have delete or create permissions
		permissions &^= conversions.PermissionCreate
		permissions &^= conversions.PermissionDelete
		if permissions == conversions.PermissionInvalid {
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Cannot set the requested share permissions", nil)
			return nil, nil, errors.New("cannot set the requested share permissions")
		}
	}

	existingPermissions := conversions.RoleFromResourcePermissions(ri.PermissionSet).OCSPermissions()
	if permissions == conversions.PermissionInvalid || !existingPermissions.Contain(permissions) {
		response.WriteOCSError(w, r, http.StatusNotFound, "Cannot set the requested share permissions", nil)
		return nil, nil, errors.New("cannot set the requested share permissions")
	}

	role = conversions.RoleFromOCSPermissions(permissions)
	roleMap := map[string]string{"name": role.Name}
	val, err := json.Marshal(roleMap)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "could not encode role", err)
		return nil, nil, err
	}

	return role, val, nil
}

// PublicShareContextName represent cross boundaries context for the name of the public share.
type PublicShareContextName string

// GetShare handles GET requests on /apps/files_sharing/api/v1/shares/(shareid).
func (h *Handler) GetShare(w http.ResponseWriter, r *http.Request) {
	var share *conversions.ShareData
	var resourceID *provider.ResourceId
	shareID := chi.URLParam(r, "shareid")
	ctx := r.Context()
	log := appctx.GetLogger(r.Context())
	log.Debug().Str("shareID", shareID).Msg("get share by id")
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	log.Debug().Str("shareID", shareID).Msg("get public share by id")
	psRes, err := client.GetPublicShare(r.Context(), &link.GetPublicShareRequest{
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: &link.PublicShareId{
					OpaqueId: shareID,
				},
			},
		},
	})

	// FIXME: the backend is returning an err when the public share is not found
	// the below code can be uncommented once error handling is normalized
	// to return Code_CODE_NOT_FOUND when a public share was not found
	/*
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error making GetPublicShare grpc request", err)
			return
		}

		if psRes.Status.Code != rpc.Code_CODE_OK && psRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
			logger.Error().Err(err).Msgf("grpc get public share request failed, code: %v", psRes.Status.Code.String)
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc get public share request failed", err)
			return
		}

	*/

	if err == nil && psRes.GetShare() != nil {
		share = conversions.PublicShare2ShareData(psRes.Share, r, h.publicURL)
		resourceID = psRes.Share.ResourceId
	}

	if share == nil {
		// check if we have a user share
		log.Debug().Str("shareID", shareID).Msg("get user share by id")
		uRes, err := client.GetShare(r.Context(), &collaboration.GetShareRequest{
			Ref: &collaboration.ShareReference{
				Spec: &collaboration.ShareReference_Id{
					Id: &collaboration.ShareId{
						OpaqueId: shareID,
					},
				},
			},
		})

		// FIXME: the backend is returning an err when the public share is not found
		// the below code can be uncommented once error handling is normalized
		// to return Code_CODE_NOT_FOUND when a public share was not found
		/*
			if err != nil {
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error making GetShare grpc request", err)
				return
			}

			if uRes.Status.Code != rpc.Code_CODE_OK && uRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
				log.Error().Err(err).Msgf("grpc get user share request failed, code: %v", uRes.Status.Code)
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc get user share request failed", err)
				return
			}
		*/

		if err == nil && uRes.GetShare() != nil {
			resourceID = uRes.Share.ResourceId
			share, err = conversions.CS3Share2ShareData(ctx, uRes.Share)
			if err != nil {
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
				return
			}
		}
	}

	if share == nil {
		log.Debug().Str("shareID", shareID).Msg("no share found with this id")
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "share not found", nil)
		return
	}

	info, status, err := h.getResourceInfoByID(ctx, client, resourceID)
	if err != nil {
		log.Error().Err(err).Msg("error mapping share data")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		log.Error().Err(err).Str("status", status.Code.String()).Msg("error mapping share data")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}

	err = h.addFileInfo(ctx, share, info)
	if err != nil {
		log.Error().Err(err).Msg("error mapping share data")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
	}
	h.mapUserIds(ctx, client, share)

	response.WriteOCSSuccess(w, r, []*conversions.ShareData{share})
}

// UpdateShare handles PUT requests on /apps/files_sharing/api/v1/shares/(shareid).
func (h *Handler) UpdateShare(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareid")
	// FIXME: isPublicShare is already doing a GetShare and GetPublicShare,
	// we should just reuse that object when doing updates
	if h.isPublicShare(r, shareID) {
		h.updatePublicShare(w, r, shareID)
		return
	}
	h.updateShare(w, r, shareID) // TODO PUT is used with incomplete data to update a share}
}

func (h *Handler) updateShare(w http.ResponseWriter, r *http.Request, shareID string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	pval := r.FormValue("permissions")
	if pval == "" {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "permissions missing", nil)
		return
	}

	pint, err := strconv.Atoi(pval)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "permissions must be an integer", nil)
		return
	}
	permissions, err := conversions.NewPermissions(pint)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, err.Error(), nil)
		return
	}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	uReq := &collaboration.UpdateShareRequest{
		Ref: &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: shareID,
				},
			},
		},
		Field: &collaboration.UpdateShareRequest_UpdateField{
			Field: &collaboration.UpdateShareRequest_UpdateField_Permissions{
				Permissions: &collaboration.SharePermissions{
					// this completely overwrites the permissions for this user
					Permissions: conversions.RoleFromOCSPermissions(permissions).CS3ResourcePermissions(),
				},
			},
		},
	}
	uRes, err := client.UpdateShare(ctx, uReq)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc update share request", err)
		return
	}

	if uRes.Status.Code != rpc.Code_CODE_OK {
		if uRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc update share request failed", err)
		return
	}

	share, err := conversions.CS3Share2ShareData(ctx, uRes.Share)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}

	statReq := provider.StatRequest{Ref: &provider.Reference{
		ResourceId: uRes.Share.ResourceId,
	}}

	statRes, err := client.Stat(r.Context(), &statReq)
	if err != nil {
		log.Debug().Err(err).Str("shares", "update user share").Msg("error during stat")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing resource information", fmt.Errorf("error getting resource information"))
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "update user share: resource not found", err)
			return
		}

		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc stat request failed for stat after updating user share", err)
		return
	}

	err = h.addFileInfo(r.Context(), share, statRes.Info)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, err.Error(), err)
		return
	}
	h.mapUserIds(ctx, client, share)

	response.WriteOCSSuccess(w, r, share)
}

// RemoveShare handles DELETE requests on /apps/files_sharing/api/v1/shares/(shareid).
func (h *Handler) RemoveShare(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareid")
	switch {
	case h.isPublicShare(r, shareID):
		h.removePublicShare(w, r, shareID)
	case h.isUserShare(r, shareID):
		h.removeUserShare(w, r, shareID)
	default:
		// The request is a remove space member request.
		h.removeSpaceMember(w, r, shareID)
	}
}

// ListShares handles GET requests on /apps/files_sharing/api/v1/shares.
func (h *Handler) ListShares(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("shared_with_me") != "" {
		var err error
		listSharedWithMe, err := strconv.ParseBool(r.FormValue("shared_with_me"))
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		}
		if listSharedWithMe {
			h.listSharesWithMe(w, r)
			return
		}
	}
	h.listSharesWithOthers(w, r)
}

const (
	ocsStateUnknown  = -1
	ocsStateAccepted = 0
	ocsStatePending  = 1
	ocsStateRejected = 2
)

func (h *Handler) listSharesWithMe(w http.ResponseWriter, r *http.Request) {
	// which pending state to list
	stateFilter := getStateFilter(r.FormValue("state"))

	log := appctx.GetLogger(r.Context())
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	ctx := r.Context()

	var pinfo *provider.ResourceInfo
	p := r.URL.Query().Get("path")
	// we need to lookup the resource id so we can filter the list of shares later
	if p != "" {
		// prefix the path with the owners home, because ocs share requests are relative to the home dir
		target := path.Join(h.homeNamespace, r.FormValue("path"))

		var status *rpc.Status
		pinfo, status, err = h.getResourceInfoByPath(ctx, client, target)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc stat request", err)
			return
		}
		if status.Code != rpc.Code_CODE_OK {
			switch status.Code {
			case rpc.Code_CODE_NOT_FOUND:
				response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "path not found", nil)
			case rpc.Code_CODE_PERMISSION_DENIED:
				response.WriteOCSError(w, r, response.MetaUnauthorized.StatusCode, "permission denied", nil)
			default:
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc stat request failed", nil)
			}
			return
		}
	}

	filters := []*collaboration.Filter{}
	var shareTypes []string
	shareTypesParam := r.URL.Query().Get("share_types")
	if shareTypesParam != "" {
		shareTypes = strings.Split(shareTypesParam, ",")
	}
	for _, s := range shareTypes {
		if s == "" {
			continue
		}
		shareType, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "invalid share type", err)
			return
		}
		switch shareType {
		case int(conversions.ShareTypeUser):
			filters = append(filters, share.UserGranteeFilter())
		case int(conversions.ShareTypeGroup):
			filters = append(filters, share.GroupGranteeFilter())
		}
	}

	if len(shareTypes) != 0 && len(filters) == 0 {
		// If a share_types filter was set for anything other than user or group shares just return an empty response
		response.WriteOCSSuccess(w, r, []*conversions.ShareData{})
		return
	}

	lrsRes, err := client.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{Filters: filters})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc ListReceivedShares request", err)
		return
	}

	if lrsRes.Status.Code != rpc.Code_CODE_OK {
		if lrsRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc ListReceivedShares request failed", err)
		return
	}

	// in a jailed namespace we have to point to the mount point in the users /Shares jail
	// to do that we have to list the /Shares jail and use those paths instead of stating the shared resources
	// The stat results would start with a path outside the jail and thus be inaccessible

	var shareJailInfos []*provider.ResourceInfo

	if h.sharePrefix != "/" {
		// we only need the path from the share jail for accepted shares
		if stateFilter == collaboration.ShareState_SHARE_STATE_ACCEPTED || stateFilter == ocsStateUnknown {
			// only log errors. They may happen but we can continue trying to at least list the shares
			lcRes, err := client.ListContainer(ctx, &provider.ListContainerRequest{
				Ref: &provider.Reference{Path: path.Join(h.homeNamespace, h.sharePrefix)},
			})
			if err != nil || lcRes.Status.Code != rpc.Code_CODE_OK {
				h.logProblems(lcRes.GetStatus(), err, "could not list container, continuing without share jail path info", log)
			} else {
				shareJailInfos = lcRes.Infos
			}
		}
	}

	shares := make([]*conversions.ShareData, 0, len(lrsRes.GetShares()))

	// TODO(refs) filter out "invalid" shares
	for _, rs := range lrsRes.GetShares() {
		if stateFilter != ocsStateUnknown && rs.GetState() != stateFilter {
			continue
		}
		var info *provider.ResourceInfo
		if pinfo != nil {
			// check if the shared resource matches the path resource
			if !utils.ResourceIDEqual(rs.Share.ResourceId, pinfo.Id) {
				// try next share
				continue
			}
			// we can reuse the stat info
			info = pinfo
		} else {
			var status *rpc.Status
			info, status, err = h.getResourceInfoByID(ctx, client, rs.Share.ResourceId)
			if err != nil || status.Code != rpc.Code_CODE_OK {
				h.logProblems(status, err, "could not stat, skipping", log)
				continue
			}
		}

		data, err := conversions.CS3Share2ShareData(r.Context(), rs.Share)
		if err != nil {
			log.Debug().Interface("share", rs.Share).Interface("shareData", data).Err(err).Msg("could not CS3Share2ShareData, skipping")
			continue
		}

		data.State = mapState(rs.GetState())

		if err := h.addFileInfo(ctx, data, info); err != nil {
			log.Debug().Interface("received_share", rs).Interface("info", info).Interface("shareData", data).Err(err).Msg("could not add file info, skipping")
			continue
		}
		h.mapUserIds(r.Context(), client, data)

		if data.State == ocsStateAccepted {
			// only accepted shares can be accessed when jailing users into their home.
			// in this case we cannot stat shared resources that are outside the users home (/home),
			// the path (/users/u-u-i-d/foo) will not be accessible

			// in a global namespace we can access the share using the full path
			// in a jailed namespace we have to point to the mount point in the users /Shares jail
			// - needed for oc10 hot migration
			// or use the /dav/spaces/<space id> endpoint?

			// list /Shares and match fileids with list of received shares
			// - only works for a /Shares folder jail
			// - does not work for freely mountable shares as in oc10 because we would need to iterate over the whole tree, there is no listing of mountpoints, yet

			// can we return the mountpoint when the gateway resolves the listing of shares?
			// - no, the gateway only sees the same list any has the same options as the ocs service
			// - we would need to have a list of mountpoints for the shares -> owncloudstorageprovider for hot migration

			// best we can do for now is stat the /Shares jail if it is set and return those paths

			// if we are in a jail and the current share has been accepted use the stat from the share jail
			// Needed because received shares can be jailed in a folder in the users home

			if h.sharePrefix != "/" {
				// if we have share jail infos use them to build the path
				if sji := findMatch(shareJailInfos, rs.Share.ResourceId); sji != nil {
					// override path with info from share jail
					data.FileTarget = path.Join(h.sharePrefix, path.Base(sji.Path))
					data.Path = path.Join(h.sharePrefix, path.Base(sji.Path))
				} else {
					data.FileTarget = path.Join(h.sharePrefix, path.Base(info.Path))
					data.Path = path.Join(h.sharePrefix, path.Base(info.Path))
				}
			} else {
				data.FileTarget = info.Path
				data.Path = info.Path
			}
		}

		shares = append(shares, data)
		log.Debug().Msgf("share: %+v", *data)
	}

	if h.listOCMShares {
		// include ocm shares in the response
		lst, err := h.listReceivedFederatedShares(ctx, client)
		if err != nil {
			log.Err(err).Msg("error listing received ocm shares")
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error listing received ocm shares", err)
			return
		}
		shares = append(shares, lst...)
	}

	response.WriteOCSSuccess(w, r, shares)
}

func findMatch(shareJailInfos []*provider.ResourceInfo, id *provider.ResourceId) *provider.ResourceInfo {
	for i := range shareJailInfos {
		if shareJailInfos[i].Id != nil && shareJailInfos[i].Id.StorageId == id.StorageId && shareJailInfos[i].Id.OpaqueId == id.OpaqueId {
			return shareJailInfos[i]
		}
	}
	return nil
}

func (h *Handler) listSharesWithOthers(w http.ResponseWriter, r *http.Request) {
	shares := make([]*conversions.ShareData, 0)

	log := appctx.GetLogger(r.Context())

	filters := []*collaboration.Filter{}
	linkFilters := []*link.ListPublicSharesRequest_Filter{}
	var e error

	// shared with others
	p := r.URL.Query().Get("path")
	if p != "" {
		// prefix the path with the owners home, because ocs share requests are relative to the home dir
		filters, linkFilters, e = h.addFilters(w, r, h.homeNamespace)
		if e != nil {
			// result has been written as part of addFilters
			return
		}
	}

	var shareTypes []string
	shareTypesParam := r.URL.Query().Get("share_types")
	if shareTypesParam != "" {
		shareTypes = strings.Split(shareTypesParam, ",")
	}

	listPublicShares := len(shareTypes) == 0 // if no share_types filter was set we want to list all share by default
	listUserShares := len(shareTypes) == 0   // if no share_types filter was set we want to list all share by default
	for _, s := range shareTypes {
		if s == "" {
			continue
		}
		shareType, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "invalid share type", err)
			return
		}

		switch shareType {
		case int(conversions.ShareTypeUser):
			listUserShares = true
			filters = append(filters, share.UserGranteeFilter())
		case int(conversions.ShareTypeGroup):
			listUserShares = true
			filters = append(filters, share.GroupGranteeFilter())
		case int(conversions.ShareTypePublicLink):
			listPublicShares = true
		}
	}

	if listPublicShares {
		publicShares, status, err := h.listPublicShares(r, linkFilters)
		h.logProblems(status, err, "could not listPublicShares", log)
		shares = append(shares, publicShares...)
	}
	if listUserShares {
		userShares, status, err := h.listUserShares(r, filters)
		h.logProblems(status, err, "could not listUserShares", log)
		shares = append(shares, userShares...)
	}

	response.WriteOCSSuccess(w, r, shares)
}

func (h *Handler) logProblems(s *rpc.Status, e error, msg string, log *zerolog.Logger) {
	if e != nil {
		// errors need to be taken care of
		log.Error().Err(e).Msg(msg)
	}
	if s != nil && s.Code != rpc.Code_CODE_OK {
		switch s.Code {
		// not found and permission denied can happen during normal operations
		case rpc.Code_CODE_NOT_FOUND:
			log.Debug().Interface("status", s).Msg(msg)
		case rpc.Code_CODE_PERMISSION_DENIED:
			log.Debug().Interface("status", s).Msg(msg)
		default:
			// anything else should not happen, someone needs to dig into it
			log.Error().Interface("status", s).Msg(msg)
		}
	}
}

func (h *Handler) addFilters(w http.ResponseWriter, r *http.Request, prefix string) ([]*collaboration.Filter, []*link.ListPublicSharesRequest_Filter, error) {
	collaborationFilters := []*collaboration.Filter{}
	linkFilters := []*link.ListPublicSharesRequest_Filter{}
	ctx := r.Context()

	// first check if the file exists
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return nil, nil, err
	}

	target := path.Join(prefix, r.FormValue("path"))
	info, status, err := h.getResourceInfoByPath(ctx, client, target)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc stat request", err)
		return nil, nil, err
	}

	if status.Code != rpc.Code_CODE_OK {
		err = errors.New(status.Message)
		if status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", err)
			return nil, nil, err
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc stat request failed", err)
		return nil, nil, err
	}

	collaborationFilters = append(collaborationFilters, share.ResourceIDFilter(info.Id))

	linkFilters = append(linkFilters, publicshare.ResourceIDFilter(info.Id))

	return collaborationFilters, linkFilters, nil
}

func (h *Handler) addFileInfo(ctx context.Context, s *conversions.ShareData, info *provider.ResourceInfo) error {
	log := appctx.GetLogger(ctx)
	if info != nil {
		// TODO The owner is not set in the storage stat metadata ...
		parsedMt, _, err := mime.ParseMediaType(info.MimeType)
		if err != nil {
			// Should never happen. We log anyways so that we know if it happens.
			log.Warn().Err(err).Msg("failed to parse mimetype")
		}
		s.MimeType = parsedMt
		// TODO STime:     &types.Timestamp{Seconds: info.Mtime.Seconds, Nanos: info.Mtime.Nanos},
		// TODO Storage: int
		s.ItemSource = resourceid.OwnCloudResourceIDWrap(info.Id)
		s.FileSource = s.ItemSource
		switch {
		case h.sharePrefix == "/":
			s.FileTarget = info.Path
			s.Path = info.Path
		case s.ShareType == conversions.ShareTypePublicLink:
			s.FileTarget = path.Join("/", path.Base(info.Path))
			s.Path = path.Join("/", path.Base(info.Path))
		default:
			s.FileTarget = path.Join(h.sharePrefix, path.Base(info.Path))
			s.Path = path.Join("/", path.Base(info.Path))
		}
		s.StorageID = storageIDPrefix + s.FileTarget
		// TODO FileParent:
		// item type
		s.ItemType = conversions.ResourceType(info.GetType()).String()

		// file owner might not yet be set. Use file info
		if s.UIDFileOwner == "" {
			s.UIDFileOwner = info.GetOwner().GetOpaqueId()
		}
		// share owner might not yet be set. Use file info
		if s.UIDOwner == "" {
			s.UIDOwner = info.GetOwner().GetOpaqueId()
		}
	}
	return nil
}

// mustGetIdentifiers always returns a struct with identifiers, if the user or group could not be found they will all be empty.
func (h *Handler) mustGetIdentifiers(ctx context.Context, client gateway.GatewayAPIClient, id string, isGroup bool) *userIdentifiers {
	log := appctx.GetLogger(ctx).With().Str("id", id).Logger()
	if id == "" {
		return &userIdentifiers{}
	}

	if idIf, err := h.userIdentifierCache.Get(id); err == nil {
		log.Debug().Msg("cache hit")
		return idIf.(*userIdentifiers)
	}

	log.Debug().Msg("cache miss")
	var ui *userIdentifiers

	if isGroup {
		res, err := client.GetGroup(ctx, &grouppb.GetGroupRequest{
			GroupId: &grouppb.GroupId{
				OpaqueId: id,
			},
			SkipFetchingMembers: true,
		})
		if err != nil {
			log.Err(err).Msg("could not look up group")
			return &userIdentifiers{}
		}
		if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
			log.Err(err).
				Int32("code", int32(res.GetStatus().GetCode())).
				Str("message", res.GetStatus().GetMessage()).
				Msg("get group call failed")
			return &userIdentifiers{}
		}
		if res.Group == nil {
			log.Debug().
				Int32("code", int32(res.GetStatus().GetCode())).
				Str("message", res.GetStatus().GetMessage()).
				Msg("group not found")
			return &userIdentifiers{}
		}
		ui = &userIdentifiers{
			DisplayName: res.Group.DisplayName,
			Username:    res.Group.GroupName,
			Mail:        res.Group.Mail,
		}
	} else {
		res, err := client.GetUser(ctx, &userpb.GetUserRequest{
			UserId: &userpb.UserId{
				OpaqueId: id,
			},
			SkipFetchingUserGroups: true,
		})
		if err != nil {
			log.Err(err).Msg("could not look up user")
			return &userIdentifiers{}
		}
		if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
			log.Err(err).
				Int32("code", int32(res.GetStatus().GetCode())).
				Str("message", res.GetStatus().GetMessage()).
				Msg("get user call failed")
			return &userIdentifiers{}
		}
		if res.User == nil {
			log.Debug().
				Int32("code", int32(res.GetStatus().GetCode())).
				Str("message", res.GetStatus().GetMessage()).
				Msg("user not found")
			return &userIdentifiers{}
		}
		ui = &userIdentifiers{
			DisplayName: res.User.DisplayName,
			Username:    res.User.Username,
			Mail:        res.User.Mail,
		}
	}
	_ = h.userIdentifierCache.Set(id, ui)
	log.Debug().Str("id", id).Msg("cache update")
	return ui
}

func (h *Handler) mapUserIds(ctx context.Context, client gateway.GatewayAPIClient, s *conversions.ShareData) {
	if s.UIDOwner != "" {
		owner := h.mustGetIdentifiers(ctx, client, s.UIDOwner, false)
		s.UIDOwner = owner.Username
		if s.DisplaynameOwner == "" {
			s.DisplaynameOwner = owner.DisplayName
		}
		if s.AdditionalInfoFileOwner == "" {
			s.AdditionalInfoFileOwner = h.getAdditionalInfoAttribute(ctx, owner)
		}
	}

	if s.UIDFileOwner != "" {
		fileOwner := h.mustGetIdentifiers(ctx, client, s.UIDFileOwner, false)
		s.UIDFileOwner = fileOwner.Username
		if s.DisplaynameFileOwner == "" {
			s.DisplaynameFileOwner = fileOwner.DisplayName
		}
		if s.AdditionalInfoOwner == "" {
			s.AdditionalInfoOwner = h.getAdditionalInfoAttribute(ctx, fileOwner)
		}
	}

	if s.ShareWith != "" && s.ShareWith != "***redacted***" {
		shareWith := h.mustGetIdentifiers(ctx, client, s.ShareWith, s.ShareType == conversions.ShareTypeGroup)
		s.ShareWith = shareWith.Username
		if s.ShareWithDisplayname == "" {
			s.ShareWithDisplayname = shareWith.DisplayName
		}
		if s.ShareWithAdditionalInfo == "" {
			s.ShareWithAdditionalInfo = h.getAdditionalInfoAttribute(ctx, shareWith)
		}
	}
}

func (h *Handler) getAdditionalInfoAttribute(ctx context.Context, u *userIdentifiers) string {
	var buf bytes.Buffer
	if err := h.additionalInfoTemplate.Execute(&buf, u); err != nil {
		log := appctx.GetLogger(ctx)
		log.Warn().Err(err).Msg("failed to parse additional info template")
		return ""
	}
	return buf.String()
}

func (h *Handler) getResourceInfoByPath(ctx context.Context, client gateway.GatewayAPIClient, path string) (*provider.ResourceInfo, *rpc.Status, error) {
	return h.getResourceInfo(ctx, client, path, &provider.Reference{
		Path: path,
	})
}

func (h *Handler) getResourceInfoByID(ctx context.Context, client gateway.GatewayAPIClient, id *provider.ResourceId) (*provider.ResourceInfo, *rpc.Status, error) {
	return h.getResourceInfo(ctx, client, resourceid.OwnCloudResourceIDWrap(id), &provider.Reference{ResourceId: id})
}

// getResourceInfo retrieves the resource info to a target.
// This method utilizes caching if it is enabled.
func (h *Handler) getResourceInfo(ctx context.Context, client gateway.GatewayAPIClient, key string, ref *provider.Reference) (*provider.ResourceInfo, *rpc.Status, error) {
	logger := appctx.GetLogger(ctx)

	var pinfo *provider.ResourceInfo
	var status *rpc.Status
	var err error
	var foundInCache bool
	if h.resourceInfoCacheTTL > 0 && h.resourceInfoCache != nil {
		if pinfo, err = h.resourceInfoCache.Get(key); err == nil {
			logger.Debug().Msgf("cache hit for resource %+v", key)
			status = &rpc.Status{Code: rpc.Code_CODE_OK}
			foundInCache = true
		}
	}
	if !foundInCache {
		logger.Debug().Msgf("cache miss for resource %+v, statting", key)
		statReq := &provider.StatRequest{
			Ref: ref,
		}

		statRes, err := client.Stat(ctx, statReq)
		if err != nil {
			return nil, nil, err
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			return nil, statRes.Status, nil
		}

		pinfo = statRes.GetInfo()
		status = statRes.Status
		if h.resourceInfoCacheTTL > 0 {
			_ = h.resourceInfoCache.SetWithExpire(key, pinfo, h.resourceInfoCacheTTL)
		}
	}

	return pinfo, status, nil
}

func (h *Handler) createCs3Share(ctx context.Context, w http.ResponseWriter, r *http.Request, client gateway.GatewayAPIClient, req *collaboration.CreateShareRequest, info *provider.ResourceInfo) (*collaboration.ShareId, bool) {
	createShareResponse, err := client.CreateShare(ctx, req)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc create share request", err)
		return nil, false
	}
	if createShareResponse.Status.Code != rpc.Code_CODE_OK {
		if createShareResponse.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return nil, false
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc create share request failed", err)
		return nil, false
	}
	s, err := conversions.CS3Share2ShareData(ctx, createShareResponse.Share)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return nil, false
	}
	err = h.addFileInfo(ctx, s, info)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error adding fileinfo to share", err)
		return nil, false
	}
	h.mapUserIds(ctx, client, s)

	response.WriteOCSSuccess(w, r, s)
	return createShareResponse.Share.Id, true
}

func mapState(state collaboration.ShareState) int {
	var mapped int
	switch state {
	case collaboration.ShareState_SHARE_STATE_PENDING:
		mapped = ocsStatePending
	case collaboration.ShareState_SHARE_STATE_ACCEPTED:
		mapped = ocsStateAccepted
	case collaboration.ShareState_SHARE_STATE_REJECTED:
		mapped = ocsStateRejected
	default:
		mapped = ocsStateUnknown
	}
	return mapped
}

func getStateFilter(s string) collaboration.ShareState {
	var stateFilter collaboration.ShareState
	switch s {
	case "all":
		stateFilter = ocsStateUnknown // no filter
	case "0": // accepted
		stateFilter = collaboration.ShareState_SHARE_STATE_ACCEPTED
	case "1": // pending
		stateFilter = collaboration.ShareState_SHARE_STATE_PENDING
	case "2": // rejected
		stateFilter = collaboration.ShareState_SHARE_STATE_REJECTED
	default:
		stateFilter = collaboration.ShareState_SHARE_STATE_ACCEPTED
	}
	return stateFilter
}
