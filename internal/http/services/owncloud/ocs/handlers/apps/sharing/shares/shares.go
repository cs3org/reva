// Copyright 2018-2020 CERN
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
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/rs/zerolog/log"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/ttlmap"
	"github.com/pkg/errors"
)

// Handler implements the shares part of the ownCloud sharing API
type Handler struct {
	gatewayAddr      string
	publicURL        string
	displayNameCache *ttlmap.TTLMap
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) error {
	h.gatewayAddr = c.GatewaySvc
	h.publicURL = c.Config.Host
	h.displayNameCache = ttlmap.New(1000, 60)
	return nil
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
			h.acceptShare(w, r, shareID)
		case "DELETE":
			h.rejectShare(w, r, shareID)
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
	shareType, err := strconv.Atoi(r.FormValue("shareType"))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "shareType must be an integer", nil)
		return
	}

	switch shareType {
	case int(conversions.ShareTypeUser):
		h.createUserShare(w, r)
	case int(conversions.ShareTypePublicLink):
		h.createPublicLinkShare(w, r)
	case int(conversions.ShareTypeFederatedCloudShare):
		h.createFederatedCloudShare(w, r)
	default:
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "unknown share type", nil)
	}
}

func (h *Handler) createUserShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}
	// prefix the path with the owners home, because ocs share requests are relative to the home dir
	// TODO the path actually depends on the configured webdav_namespace
	hRes, err := c.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc get home request", err)
		return
	}

	prefix := hRes.GetPath()
	sharepath := r.FormValue("path")
	// if user sharing is disabled
	if h.gatewayAddr == "" {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "user sharing service not configured", nil)
		return
	}

	shareWith := r.FormValue("shareWith")
	if shareWith == "" {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "missing shareWith", nil)
		return
	}

	userRes, err := c.GetUser(ctx, &userpb.GetUserRequest{
		UserId: &userpb.UserId{OpaqueId: shareWith},
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error searching recipient", err)
		return
	}

	if userRes.Status.Code != rpc.Code_CODE_OK {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "user not found", err)
		return
	}

	statRes, err := h.stat(ctx, path.Join(prefix, sharepath))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, fmt.Sprintf("stat on file %s failed", sharepath), err)
		return
	}

	var permissions conversions.Permissions

	role := r.FormValue("role")
	// 2. if we don't have a role try to map the permissions
	if role == "" {
		pval := r.FormValue("permissions")
		if pval == "" {
			// default is all permissions / role coowner
			permissions = conversions.PermissionAll
			role = conversions.RoleCoowner
		} else {
			pint, err := strconv.Atoi(pval)
			if err != nil {
				response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "permissions must be an integer", nil)
				return
			}
			permissions, err = conversions.NewPermissions(pint)
			if err != nil {
				if err == conversions.ErrPermissionNotInRange {
					response.WriteOCSError(w, r, http.StatusNotFound, err.Error(), nil)
				} else {
					response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, err.Error(), nil)
				}
				return
			}
			role = conversions.Permissions2Role(permissions)
		}
	}

	if statRes.Info != nil && statRes.Info.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
		// Single file shares should never have delete or create permissions
		permissions &^= conversions.PermissionCreate
		permissions &^= conversions.PermissionDelete
	}

	var resourcePermissions *provider.ResourcePermissions
	resourcePermissions = asCS3Permissions(permissions, resourcePermissions)

	roleMap := map[string]string{"name": role}
	val, err := json.Marshal(roleMap)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "could not encode role", err)
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc stat request failed", err)
		return
	}

	createShareReq := &collaboration.CreateShareRequest{
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"role": {
					Decoder: "json",
					Value:   val,
				},
			},
		},
		ResourceInfo: statRes.Info,
		Grant: &collaboration.ShareGrant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   userRes.User.GetId(),
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: resourcePermissions,
			},
		},
	}

	createShareResponse, err := c.CreateShare(ctx, createShareReq)
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
	s, err := conversions.UserShare2ShareData(ctx, createShareResponse.Share)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}
	err = h.addFileInfo(ctx, s, statRes.Info)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error adding fileinfo to share", err)
		return
	}
	h.addDisplaynames(ctx, c, s)

	response.WriteOCSSuccess(w, r, s)
}

func (h *Handler) stat(ctx context.Context, path string) (*provider.StatResponse, error) {
	c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		return nil, fmt.Errorf("error getting grpc gateway client: %s", err.Error())
	}
	statReq := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path,
			},
		},
	}

	statRes, err := c.Stat(ctx, statReq)
	if err != nil {
		return nil, fmt.Errorf("error sending a grpc stat request: %s", err.Error())
	}
	return statRes, nil
}

// PublicShareContextName represent cross boundaries context for the name of the public share
type PublicShareContextName string

// TODO sort out mapping, this is just a first guess
// TODO use roles to make this configurable
func asCS3Permissions(p conversions.Permissions, rp *provider.ResourcePermissions) *provider.ResourcePermissions {
	if rp == nil {
		rp = &provider.ResourcePermissions{}
	}

	if p.Contain(conversions.PermissionRead) {
		rp.ListContainer = true
		rp.ListGrants = true
		rp.ListFileVersions = true
		rp.ListRecycle = true
		rp.Stat = true
		rp.GetPath = true
		rp.GetQuota = true
		rp.InitiateFileDownload = true
	}
	if p.Contain(conversions.PermissionWrite) {
		rp.InitiateFileUpload = true
		rp.RestoreFileVersion = true
		rp.RestoreRecycleItem = true
	}
	if p.Contain(conversions.PermissionCreate) {
		rp.CreateContainer = true
		// FIXME permissions mismatch: double check create vs write file
		rp.InitiateFileUpload = true
		if p.Contain(conversions.PermissionWrite) {
			rp.Move = true // TODO move only when create and write?
		}
	}
	if p.Contain(conversions.PermissionDelete) {
		rp.Delete = true
		rp.PurgeRecycle = true
	}
	if p.Contain(conversions.PermissionShare) {
		rp.AddGrant = true
		rp.RemoveGrant = true // TODO when are you able to unshare / delete
		rp.UpdateGrant = true
	}
	return rp
}

func (h *Handler) map2CS3Permissions(role string, p conversions.Permissions) (*provider.ResourcePermissions, error) {
	// TODO replace usage of this method with asCS3Permissions
	rp := &provider.ResourcePermissions{
		ListContainer:        p.Contain(conversions.PermissionRead),
		ListGrants:           p.Contain(conversions.PermissionRead),
		ListFileVersions:     p.Contain(conversions.PermissionRead),
		ListRecycle:          p.Contain(conversions.PermissionRead),
		Stat:                 p.Contain(conversions.PermissionRead),
		GetPath:              p.Contain(conversions.PermissionRead),
		GetQuota:             p.Contain(conversions.PermissionRead),
		InitiateFileDownload: p.Contain(conversions.PermissionRead),

		// FIXME: uploader role with only write permission can use InitiateFileUpload, not anything else
		Move:               p.Contain(conversions.PermissionWrite),
		InitiateFileUpload: p.Contain(conversions.PermissionWrite),
		CreateContainer:    p.Contain(conversions.PermissionCreate),
		Delete:             p.Contain(conversions.PermissionDelete),
		RestoreFileVersion: p.Contain(conversions.PermissionWrite),
		RestoreRecycleItem: p.Contain(conversions.PermissionWrite),
		PurgeRecycle:       p.Contain(conversions.PermissionDelete),

		AddGrant:    p.Contain(conversions.PermissionShare),
		RemoveGrant: p.Contain(conversions.PermissionShare), // TODO when are you able to unshare / delete
		UpdateGrant: p.Contain(conversions.PermissionShare),
	}
	return rp, nil
}

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
			share, err = conversions.UserShare2ShareData(ctx, uRes.Share)
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

	// prepare the stat request
	statReq := &provider.StatRequest{
		// prepare the reference
		Ref: &provider.Reference{
			// using ResourceId from the share
			Spec: &provider.Reference_Id{Id: resourceID},
		},
	}

	statResponse, err := client.Stat(ctx, statReq)
	if err != nil {
		log.Error().Err(err).Msg("error mapping share data")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}

	if statResponse.Status.Code != rpc.Code_CODE_OK {
		log.Error().Err(err).Str("status", statResponse.Status.Code.String()).Msg("error mapping share data")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}

	err = h.addFileInfo(ctx, share, statResponse.Info)
	if err != nil {
		log.Error().Err(err).Str("status", statResponse.Status.Code.String()).Msg("error mapping share data")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
	}
	h.addDisplaynames(ctx, client, share)

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

	uClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
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
					Permissions: asCS3Permissions(permissions, nil),
				},
			},
		},
	}
	uRes, err := uClient.UpdateShare(ctx, uReq)
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

	gReq := &collaboration.GetShareRequest{
		Ref: &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: shareID,
				},
			},
		},
	}
	gRes, err := uClient.GetShare(ctx, gReq)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc get share request", err)
		return
	}

	if gRes.Status.Code != rpc.Code_CODE_OK {
		if gRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc get share request failed", err)
		return
	}

	share, err := conversions.UserShare2ShareData(ctx, gRes.Share)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}

	statReq := provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Id{
				Id: gRes.Share.ResourceId,
			},
		},
	}

	statRes, err := uClient.Stat(r.Context(), &statReq)
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
	h.addDisplaynames(ctx, uClient, share)

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

const ocsStateUnknown = -1
const ocsStateAccepted = 0
const ocsStatePending = 1
const ocsStateRejected = 2

func (h *Handler) listSharesWithMe(w http.ResponseWriter, r *http.Request) {
	// which pending state to list
	var stateFilter collaboration.ShareState
	switch r.FormValue("state") {
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

	gwc, err := pool.GetGatewayServiceClient(h.gatewayAddr)
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
		// TODO the path actually depends on the configured webdav_namespace
		hRes, err := gwc.GetHome(ctx, &provider.GetHomeRequest{})
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc get home request", err)
			return
		}

		target := path.Join(hRes.Path, r.FormValue("path"))

		statReq := &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: target,
				},
			},
		}

		statRes, err := gwc.Stat(ctx, statReq)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc stat request", err)
			return
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			switch statRes.Status.Code {
			case rpc.Code_CODE_NOT_FOUND:
				response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "path not found", nil)
			case rpc.Code_CODE_PERMISSION_DENIED:
				response.WriteOCSError(w, r, response.MetaUnauthorized.StatusCode, "permission denied", nil)
			default:
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc stat request failed", nil)
			}
			return
		}

		pinfo = statRes.GetInfo()
	}

	lrsReq := collaboration.ListReceivedSharesRequest{}

	lrsRes, err := gwc.ListReceivedShares(r.Context(), &lrsReq)
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
	lrsRes.GetShares()

	shares := make([]*conversions.ShareData, 0)

	var info *provider.ResourceInfo
	// TODO(refs) filter out "invalid" shares
	for _, rs := range lrsRes.GetShares() {

		if stateFilter != ocsStateUnknown && rs.GetState() != stateFilter {
			continue
		}
		if pinfo != nil {
			// check if the shared resource matches the path resource
			if rs.Share.ResourceId.StorageId != pinfo.GetId().StorageId ||
				rs.Share.ResourceId.OpaqueId != pinfo.GetId().OpaqueId {
				// try next share
				continue
			}
			// we can reuse the stat info
			info = pinfo
		} else {
			// we need to do a stat call
			statRequest := provider.StatRequest{
				Ref: &provider.Reference{
					Spec: &provider.Reference_Id{
						Id: rs.Share.ResourceId,
					},
				},
			}

			statRes, err := gwc.Stat(r.Context(), &statRequest)
			if err != nil || statRes.Status.Code != rpc.Code_CODE_OK {
				h.logProblems(statRes.GetStatus(), err, "could not stat, skipping")
				continue
			}

			info = statRes.GetInfo()
		}

		data, err := conversions.UserShare2ShareData(r.Context(), rs.Share)
		if err != nil {
			log.Debug().Interface("share", rs.Share).Interface("shareData", data).Err(err).Msg("could not UserShare2ShareData, skipping")
			continue
		}

		switch rs.GetState() {
		case collaboration.ShareState_SHARE_STATE_PENDING:
			data.State = ocsStatePending
		case collaboration.ShareState_SHARE_STATE_ACCEPTED:
			data.State = ocsStateAccepted
		case collaboration.ShareState_SHARE_STATE_REJECTED:
			data.State = ocsStateRejected
		default:
			data.State = ocsStateUnknown
		}

		if err := h.addFileInfo(ctx, data, info); err != nil {
			log.Debug().Interface("received_share", rs).Interface("info", info).Interface("shareData", data).Err(err).Msg("could not add file info, skipping")
			continue
		}
		h.addDisplaynames(r.Context(), gwc, data)

		shares = append(shares, data)
	}

	response.WriteOCSSuccess(w, r, shares)
}

func (h *Handler) listSharesWithOthers(w http.ResponseWriter, r *http.Request) {
	shares := make([]*conversions.ShareData, 0)
	filters := []*collaboration.ListSharesRequest_Filter{}
	linkFilters := []*link.ListPublicSharesRequest_Filter{}
	var err error

	// shared with others
	p := r.URL.Query().Get("path")
	if p != "" {
		c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting storage grpc client", err)
			return
		}

		// prefix the path with the owners home, because ocs share requests are relative to the home dir
		// TODO the path actually depends on the configured webdav_namespace
		hRes, err := c.GetHome(r.Context(), &provider.GetHomeRequest{})
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc get home request", err)
			return
		}

		if hRes.Status.Code != rpc.Code_CODE_OK {
			switch hRes.Status.Code {
			case rpc.Code_CODE_NOT_FOUND:
				response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "path not found", nil)
			case rpc.Code_CODE_PERMISSION_DENIED:
				response.WriteOCSError(w, r, response.MetaUnauthorized.StatusCode, "permission denied", nil)
			default:
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc stat request failed", nil)
			}
			return
		}

		filters, linkFilters, err = h.addFilters(w, r, hRes.GetPath())
		if err != nil {
			// result has been written as part of addFilters
			return
		}
	}

	userShares, status, err := h.listUserShares(r, filters)
	h.logProblems(status, err, "could not listUserShares")

	publicShares, status, err := h.listPublicShares(r, linkFilters)
	h.logProblems(status, err, "could not listPublicShares")

	shares = append(shares, append(userShares, publicShares...)...)

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
	var info *provider.ResourceInfo
	ctx := r.Context()

	// first check if the file exists
	gwClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return nil, nil, err
	}

	target := path.Join(prefix, r.FormValue("path"))

	statReq := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: target,
			},
		},
	}

	res, err := gwClient.Stat(ctx, statReq)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc stat request", err)
		return nil, nil, err
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		err = errors.New(res.Status.Message)
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", err)
			return nil, nil, err
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc stat request failed", err)
		return nil, nil, err
	}

	info = res.Info

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
		s.StorageID = info.Id.StorageId
		// TODO Storage: int
		s.ItemSource = wrapResourceID(info.Id)
		s.FileSource = s.ItemSource
		s.FileTarget = path.Join("/", path.Base(info.Path))
		s.Path = path.Join("/", path.Base(info.Path)) // TODO hm this might have to be relative to the users home ... depends on the webdav_namespace config
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

func (h *Handler) getDisplayname(ctx context.Context, c gateway.GatewayAPIClient, userid string) string {
	log := appctx.GetLogger(ctx)
	if userid == "" {
		return ""
	}
	if dn := h.displayNameCache.Get(userid); dn != "" {
		log.Debug().Str("userid", userid).Msg("cache hit")
		return dn
	}
	log.Debug().Str("userid", userid).Msg("cache miss")
	res, err := c.GetUser(ctx, &userpb.GetUserRequest{
		UserId: &userpb.UserId{
			OpaqueId: userid,
		},
	})
	if err != nil {
		log.Err(err).
			Str("userid", userid).
			Msg("could not look up user")
		return ""
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		log.Err(err).
			Str("opaque_id", userid).
			Int32("code", int32(res.GetStatus().GetCode())).
			Str("message", res.GetStatus().GetMessage()).
			Msg("get user call failed")
		return ""
	}
	if res.User == nil {
		log.Debug().
			Str("opaque_id", userid).
			Int32("code", int32(res.GetStatus().GetCode())).
			Str("message", res.GetStatus().GetMessage()).
			Msg("user not found")
		return ""
	}
	if res.User.DisplayName == "" {
		log.Debug().
			Str("opaque_id", userid).
			Int32("code", int32(res.GetStatus().GetCode())).
			Str("message", res.GetStatus().GetMessage()).
			Msg("Displayname empty")
		return ""
	}

	h.displayNameCache.Put(userid, res.User.DisplayName)
	log.Debug().Str("userid", userid).Msg("cache update")
	return res.User.DisplayName
}

func (h *Handler) addDisplaynames(ctx context.Context, c gateway.GatewayAPIClient, s *conversions.ShareData) {
	if s.DisplaynameOwner == "" {
		s.DisplaynameOwner = h.getDisplayname(ctx, c, s.UIDOwner)
	}
	if s.DisplaynameFileOwner == "" {
		s.DisplaynameFileOwner = h.getDisplayname(ctx, c, s.UIDFileOwner)
	}
	if s.ShareWithDisplayname == "" {
		s.ShareWithDisplayname = h.getDisplayname(ctx, c, s.ShareWith)
	}
}

func parseTimestamp(timestampString string) (*types.Timestamp, error) {
	parsedTime, err := time.Parse("2006-01-02T15:04:05Z0700", timestampString)
	if err != nil {
		parsedTime, err = time.Parse("2006-01-02", timestampString)
	}
	if err != nil {
		return nil, fmt.Errorf("datetime format invalid: %v", timestampString)
	}
	final := parsedTime.UnixNano()

	return &types.Timestamp{
		Seconds: uint64(final / 1000000000),
		Nanos:   uint32(final % 1000000000),
	}, nil
}

func ocPublicPermToCs3(permKey int, h *Handler) (*provider.ResourcePermissions, error) {
	role, ok := ocPublicPermToRole[permKey]
	if !ok {
		log.Error().Str("ocPublicPermToCs3", "shares").Msgf("invalid oC permission: %s", role)
		return nil, fmt.Errorf("invalid oC permission: %s", role)
	}

	perm, err := conversions.NewPermissions(permKey)
	if err != nil {
		return nil, err
	}

	p, err := h.map2CS3Permissions(role, perm)
	if err != nil {
		log.Error().Str("permissionFromRequest", "shares").Msgf("role to cs3permission %v", perm)
		return nil, fmt.Errorf("role to cs3permission failed: %v", perm)
	}

	return p, nil
}

func permissionFromRequest(r *http.Request, h *Handler) (*provider.ResourcePermissions, error) {
	var err error
	// phoenix sends: {"permissions": 15}. See ocPublicPermToRole struct for mapping

	permKey := 1

	// note: "permissions" value has higher priority than "publicUpload"

	// handle legacy "publicUpload" arg that overrides permissions differently depending on the scenario
	// https://github.com/owncloud/core/blob/v10.4.0/apps/files_sharing/lib/Controller/Share20OcsController.php#L447
	publicUploadString, ok := r.Form["publicUpload"]
	if ok {
		publicUploadFlag, err := strconv.ParseBool(publicUploadString[0])
		if err != nil {
			log.Error().Err(err).Str("publicUpload", publicUploadString[0]).Msg("could not parse publicUpload argument")
			return nil, err
		}

		if publicUploadFlag {
			// all perms except reshare
			permKey = 15
		}
	} else {
		permissionsString, ok := r.Form["permissions"]
		if !ok {
			// no permission values given
			return nil, nil
		}

		permKey, err = strconv.Atoi(permissionsString[0])
		if err != nil {
			log.Error().Str("permissionFromRequest", "shares").Msgf("invalid type: %T", permKey)
			return nil, fmt.Errorf("invalid type: %T", permKey)
		}
	}

	p, err := ocPublicPermToCs3(permKey, h)
	if err != nil {
		return nil, err
	}
	return p, err
}

// TODO: add mapping for user share permissions to role

// Maps oc10 public link permissions to roles
var ocPublicPermToRole = map[int]string{
	// Recipients can view and download contents.
	1: "viewer",
	// Recipients can view, download, edit, delete and upload contents
	15: "editor",
	// Recipients can upload but existing contents are not revealed
	4: "uploader",
	// Recipients can view, download and upload contents
	5: "contributor",
}
