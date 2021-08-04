// Copyright 2018-2021 CERN
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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"text/template"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/rs/zerolog/log"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/bluele/gcache"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/share/cache"
	"github.com/cs3org/reva/pkg/share/cache/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/pkg/errors"
)

// Handler implements the shares part of the ownCloud sharing API
type Handler struct {
	gatewayAddr            string
	publicURL              string
	sharePrefix            string
	homeNamespace          string
	additionalInfoTemplate *template.Template
	userIdentifierCache    *ttlcache.Cache
	resourceInfoCache      gcache.Cache
	resourceInfoCacheTTL   time.Duration
}

// we only cache the minimal set of data instead of the full user metadata
type userIdentifiers struct {
	DisplayName string
	Username    string
	Mail        string
}

func getCacheWarmupManager(c *config.Config) (cache.Warmup, error) {
	if f, ok := registry.NewFuncs[c.CacheWarmupDriver]; ok {
		return f(c.CacheWarmupDrivers[c.CacheWarmupDriver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.CacheWarmupDriver)
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) error {
	h.gatewayAddr = c.GatewaySvc
	h.publicURL = c.Config.Host
	h.sharePrefix = c.SharePrefix
	h.homeNamespace = c.HomeNamespace
	h.resourceInfoCache = gcache.New(c.ResourceInfoCacheSize).LFU().Build()
	h.resourceInfoCacheTTL = time.Second * time.Duration(c.ResourceInfoCacheTTL)

	h.additionalInfoTemplate, _ = template.New("additionalInfo").Parse(c.AdditionalInfoAttribute)

	h.userIdentifierCache = ttlcache.NewCache()
	_ = h.userIdentifierCache.SetTTL(time.Second * 60)

	if h.resourceInfoCacheTTL > 0 {
		cwm, err := getCacheWarmupManager(c)
		if err == nil {
			go h.startCacheWarmup(cwm)
		}
	}

	return nil
}

func (h *Handler) startCacheWarmup(c cache.Warmup) {
	infos, err := c.GetResourceInfos()
	if err != nil {
		return
	}
	for _, r := range infos {
		key := wrapResourceID(r.Id)
		_ = h.resourceInfoCache.SetWithExpire(key, r, time.Second*h.resourceInfoCacheTTL)
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	var head string
	head, r.URL.Path = router.ShiftPath(r.URL.Path)

	log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")

	switch head {
	case "":
		switch r.Method {
		case "OPTIONS":
			w.WriteHeader(http.StatusOK)
		case "GET":
			if h.isListSharesWithMe(w, r) {
				h.listSharesWithMe(w, r)
			} else {
				h.listSharesWithOthers(w, r)
			}
		case "POST":
			h.createShare(w, r)
		default:
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Only GET, POST and PUT are allowed", nil)
		}

	case "pending":
		var shareID string
		shareID, r.URL.Path = router.ShiftPath(r.URL.Path)

		log.Debug().Str("share_id", shareID).Str("tail", r.URL.Path).Msg("http routing")

		switch r.Method {
		case "POST":
			h.updateReceivedShare(w, r, shareID, false)
		case "DELETE":
			h.updateReceivedShare(w, r, shareID, true)
		default:
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Only POST and DELETE are allowed", nil)
		}

	case "remote_shares":
		var shareID string
		shareID, r.URL.Path = router.ShiftPath(r.URL.Path)

		log.Debug().Str("share_id", shareID).Str("tail", r.URL.Path).Msg("http routing")

		switch r.Method {
		case "GET":
			if shareID == "" {
				h.listFederatedShares(w, r)
			} else {
				h.getFederatedShare(w, r, shareID)
			}
		default:
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Only GET method is allowed", nil)
		}

	default:
		switch r.Method {
		case "GET":
			h.getShare(w, r, head)
		case "PUT":
			// FIXME: isPublicShare is already doing a GetShare and GetPublicShare,
			// we should just reuse that object when doing updates
			if h.isPublicShare(r, strings.ReplaceAll(head, "/", "")) {
				h.updatePublicShare(w, r, strings.ReplaceAll(head, "/", ""))
				return
			}
			h.updateShare(w, r, head) // TODO PUT is used with incomplete data to update a share
		case "DELETE":
			shareID := strings.ReplaceAll(head, "/", "")
			if h.isPublicShare(r, shareID) {
				h.removePublicShare(w, r, shareID)
				return
			}
			h.removeUserShare(w, r, head)
		default:
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Only GET, POST and PUT are allowed", nil)
		}
	}
}

func (h *Handler) createShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	shareType, err := strconv.Atoi(r.FormValue("shareType"))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "shareType must be an integer", nil)
		return
	}
	// get user permissions on the shared file

	client, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	// prefix the path with the owners home, because ocs share requests are relative to the home dir
	fn := path.Join(h.homeNamespace, r.FormValue("path"))

	statReq := provider.StatRequest{
		Ref: &provider.Reference{Path: fn},
	}

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()

	statRes, err := client.Stat(ctx, &statReq)
	if err != nil {
		sublog.Debug().Err(err).Str("createShare", "shares").Msg("error on stat call")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing resource information", fmt.Errorf("error getting resource information"))
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		ocdav.HandleErrorStatus(&sublog, w, statRes.Status)
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
	default:
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "unknown share type", nil)
	}
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

// PublicShareContextName represent cross boundaries context for the name of the public share
type PublicShareContextName string

func (h *Handler) getShare(w http.ResponseWriter, r *http.Request, shareID string) {
	var share *conversions.ShareData
	var resourceID *provider.ResourceId
	ctx := r.Context()
	logger := appctx.GetLogger(r.Context())
	logger.Debug().Str("shareID", shareID).Msg("get share by id")
	client, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	logger.Debug().Str("shareID", shareID).Msg("get public share by id")
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
		logger.Debug().Str("shareID", shareID).Msg("get user share by id")
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
				logger.Error().Err(err).Msgf("grpc get user share request failed, code: %v", uRes.Status.Code)
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
		logger.Debug().Str("shareID", shareID).Msg("no share found with this id")
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

func (h *Handler) updateShare(w http.ResponseWriter, r *http.Request, shareID string) {
	ctx := r.Context()

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

	client, err := pool.GetGatewayServiceClient(h.gatewayAddr)
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

func (h *Handler) isListSharesWithMe(w http.ResponseWriter, r *http.Request) (listSharedWithMe bool) {
	if r.FormValue("shared_with_me") != "" {
		var err error
		listSharedWithMe, err = strconv.ParseBool(r.FormValue("shared_with_me"))
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		}
	}
	return
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

	client, err := pool.GetGatewayServiceClient(h.gatewayAddr)
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

	lrsRes, err := client.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
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
				h.logProblems(status, err, "could not stat, skipping")
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
			// Needed because received shares can be jailed in a folder in the users home
			data.FileTarget = path.Join(h.sharePrefix, info.Path)
			data.Path = path.Join(h.sharePrefix, info.Path)
		}

		shares = append(shares, data)
		log.Debug().Msgf("share: %+v", *data)
	}

	response.WriteOCSSuccess(w, r, shares)
}

func (h *Handler) listSharesWithOthers(w http.ResponseWriter, r *http.Request) {
	shares := make([]*conversions.ShareData, 0)

	filters := []*collaboration.ListSharesRequest_Filter{}
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

	shareTypes := strings.Split(r.URL.Query().Get("share_types"), ",")
	for _, s := range shareTypes {
		shareType, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil && s != "" {
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "invalid share type", err)
			return
		}
		if s == "" || shareType == int(conversions.ShareTypeUser) || shareType == int(conversions.ShareTypeGroup) {
			userShares, status, err := h.listUserShares(r, filters)
			h.logProblems(status, err, "could not listUserShares")
			shares = append(shares, userShares...)
		}
		if s == "" || shareType == int(conversions.ShareTypePublicLink) {
			publicShares, status, err := h.listPublicShares(r, linkFilters)
			h.logProblems(status, err, "could not listPublicShares")
			shares = append(shares, publicShares...)
		}
	}

	response.WriteOCSSuccess(w, r, shares)
}

func (h *Handler) logProblems(s *rpc.Status, e error, msg string) {
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

func (h *Handler) addFilters(w http.ResponseWriter, r *http.Request, prefix string) ([]*collaboration.ListSharesRequest_Filter, []*link.ListPublicSharesRequest_Filter, error) {
	collaborationFilters := []*collaboration.ListSharesRequest_Filter{}
	linkFilters := []*link.ListPublicSharesRequest_Filter{}
	ctx := r.Context()

	// first check if the file exists
	client, err := pool.GetGatewayServiceClient(h.gatewayAddr)
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

	collaborationFilters = append(collaborationFilters, &collaboration.ListSharesRequest_Filter{
		Type: collaboration.ListSharesRequest_Filter_TYPE_RESOURCE_ID,
		Term: &collaboration.ListSharesRequest_Filter_ResourceId{
			ResourceId: info.Id,
		},
	})

	linkFilters = append(linkFilters, &link.ListPublicSharesRequest_Filter{
		Type: link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID,
		Term: &link.ListPublicSharesRequest_Filter_ResourceId{
			ResourceId: info.Id,
		},
	})

	return collaborationFilters, linkFilters, nil
}

func wrapResourceID(r *provider.ResourceId) string {
	return wrap(r.StorageId, r.OpaqueId)
}

// The fileID must be encoded
// - XML safe, because it is going to be used in the propfind result
// - url safe, because the id might be used in a url, eg. the /dav/meta nodes
// which is why we base64 encode it
func wrap(sid string, oid string) string {
	return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", sid, oid)))
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
		s.StorageID = info.Id.StorageId + "!" + info.Id.OpaqueId
		// TODO Storage: int
		s.ItemSource = wrapResourceID(info.Id)
		s.FileSource = s.ItemSource
		s.FileTarget = path.Join("/", info.Path)
		s.Path = path.Join("/", info.Path)
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

// mustGetIdentifiers always returns a struct with identifiers, if the user or group could not be found they will all be empty
func (h *Handler) mustGetIdentifiers(ctx context.Context, client gateway.GatewayAPIClient, id string, isGroup bool) *userIdentifiers {
	sublog := appctx.GetLogger(ctx).With().Str("id", id).Logger()
	if id == "" {
		return &userIdentifiers{}
	}

	if idIf, err := h.userIdentifierCache.Get(id); err == nil {
		sublog.Debug().Msg("cache hit")
		return idIf.(*userIdentifiers)
	}

	sublog.Debug().Msg("cache miss")
	var ui *userIdentifiers

	if isGroup {
		res, err := client.GetGroup(ctx, &grouppb.GetGroupRequest{
			GroupId: &grouppb.GroupId{
				OpaqueId: id,
			},
		})
		if err != nil {
			sublog.Err(err).Msg("could not look up group")
			return &userIdentifiers{}
		}
		if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
			sublog.Err(err).
				Int32("code", int32(res.GetStatus().GetCode())).
				Str("message", res.GetStatus().GetMessage()).
				Msg("get group call failed")
			return &userIdentifiers{}
		}
		if res.Group == nil {
			sublog.Debug().
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
		})
		if err != nil {
			sublog.Err(err).Msg("could not look up user")
			return &userIdentifiers{}
		}
		if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
			sublog.Err(err).
				Int32("code", int32(res.GetStatus().GetCode())).
				Str("message", res.GetStatus().GetMessage()).
				Msg("get user call failed")
			return &userIdentifiers{}
		}
		if res.User == nil {
			sublog.Debug().
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
	return h.getResourceInfo(ctx, client, wrapResourceID(id), &provider.Reference{ResourceId: id})
}

// getResourceInfo retrieves the resource info to a target.
// This method utilizes caching if it is enabled.
func (h *Handler) getResourceInfo(ctx context.Context, client gateway.GatewayAPIClient, key string, ref *provider.Reference) (*provider.ResourceInfo, *rpc.Status, error) {
	logger := appctx.GetLogger(ctx)

	var pinfo *provider.ResourceInfo
	var status *rpc.Status
	if infoIf, err := h.resourceInfoCache.Get(key); h.resourceInfoCacheTTL > 0 && err == nil {
		logger.Debug().Msgf("cache hit for resource %+v", key)
		pinfo = infoIf.(*provider.ResourceInfo)
		status = &rpc.Status{Code: rpc.Code_CODE_OK}
	} else {
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

func (h *Handler) createCs3Share(ctx context.Context, w http.ResponseWriter, r *http.Request, client gateway.GatewayAPIClient, req *collaboration.CreateShareRequest, info *provider.ResourceInfo) {
	createShareResponse, err := client.CreateShare(ctx, req)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc create share request", err)
		return
	}
	if createShareResponse.Status.Code != rpc.Code_CODE_OK {
		if createShareResponse.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc create share request failed", err)
		return
	}
	s, err := conversions.CS3Share2ShareData(ctx, createShareResponse.Share)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}
	err = h.addFileInfo(ctx, s, info)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error adding fileinfo to share", err)
		return
	}
	h.mapUserIds(ctx, client, s)

	response.WriteOCSSuccess(w, r, s)
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
