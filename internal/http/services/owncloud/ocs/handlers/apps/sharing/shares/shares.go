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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/rs/zerolog/log"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/pkg/errors"
)

// Handler implements the shares part of the ownCloud sharing API
type Handler struct {
	gatewayAddr string
	publicURL   string
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) error {
	h.gatewayAddr = c.GatewaySvc
	h.publicURL = c.Config.Host
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
				"role": &types.OpaqueEntry{
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
	s, err := h.userShare2ShareData(ctx, createShareResponse.Share)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}
	err = h.addFileInfo(ctx, s, statRes.Info)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error adding fileinfo to share", err)
		return
	}
	response.WriteOCSSuccess(w, r, s)
}

func (h *Handler) createPublicLinkShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

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

	statReq := provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path.Join(prefix, r.FormValue("path")), // TODO replace path with target
			},
		},
	}

	statRes, err := c.Stat(ctx, &statReq)
	if err != nil {
		log.Debug().Err(err).Str("createShare", "shares").Msg("error on stat call")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing resource information", fmt.Errorf("error getting resource information"))
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "resource not found", fmt.Errorf("error creating share on non-existing resource"))
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error when querying resource", fmt.Errorf("error when querying resource information while creating share, status %d", statRes.Status.Code))
		return
	}

	err = r.ParseForm()
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Could not parse form from request", err)
		return
	}

	newPermissions, err := permissionFromRequest(r, h)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Could not read permission from request", err)
		return
	}

	if newPermissions == nil {
		// default perms: read-only
		// TODO: the default might change depending on allowed permissions and configs
		newPermissions, err = ocPublicPermToCs3(1, h)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "Could not convert default permissions", err)
			return
		}
	}

	req := link.CreatePublicShareRequest{
		ResourceInfo: statRes.GetInfo(),
		Grant: &link.Grant{
			Permissions: &link.PublicSharePermissions{
				Permissions: newPermissions,
			},
			Password: r.FormValue("password"),
		},
	}

	expireTimeString, ok := r.Form["expireDate"]
	if ok {
		if expireTimeString[0] != "" {
			expireTime, err := parseTimestamp(expireTimeString[0])
			if err != nil {
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "invalid datetime format", err)
				return
			}
			if expireTime != nil {
				req.Grant.Expiration = expireTime
			}
		}
	}

	// set displayname and password protected as arbitrary metadata
	req.ResourceInfo.ArbitraryMetadata = &provider.ArbitraryMetadata{
		Metadata: map[string]string{
			"name": r.FormValue("name"),
			// "password": r.FormValue("password"),
		},
	}

	createRes, err := c.CreatePublicShare(ctx, &req)
	if err != nil {
		log.Debug().Err(err).Str("createShare", "shares").Msgf("error creating a public share to resource id: %v", statRes.Info.GetId())
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error creating public share", fmt.Errorf("error creating a public share to resource id: %v", statRes.Info.GetId()))
		return
	}

	if createRes.Status.Code != rpc.Code_CODE_OK {
		log.Debug().Err(errors.New("create public share failed")).Str("shares", "createShare").Msgf("create public share failed with status code: %v", createRes.Status.Code.String())
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc create public share request failed", err)
		return
	}

	s := conversions.PublicShare2ShareData(createRes.Share, r, h.publicURL)
	err = h.addFileInfo(ctx, s, statRes.Info)

	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error enhancing response with share data", err)
		return
	}

	response.WriteOCSSuccess(w, r, s)
}

func (h *Handler) createFederatedCloudShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

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

	shareWithUser, shareWithProvider := r.FormValue("shareWithUser"), r.FormValue("shareWithProvider")
	if shareWithUser == "" || shareWithProvider == "" {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "missing shareWith parameters", nil)
		return
	}

	providerInfoResp, err := c.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
		Domain: shareWithProvider,
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc get invite by domain info request", err)
		return
	}

	remoteUserRes, err := c.GetRemoteUser(ctx, &invitepb.GetRemoteUserRequest{
		RemoteUserId: &userpb.UserId{OpaqueId: shareWithUser, Idp: shareWithProvider},
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error searching recipient", err)
		return
	}
	if remoteUserRes.Status.Code != rpc.Code_CODE_OK {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "user not found", err)
		return
	}

	var permissions conversions.Permissions
	var role string

	pval := r.FormValue("permissions")
	if pval == "" {
		// by default only allow read permissions / assign viewer role
		permissions = conversions.PermissionRead
		role = conversions.RoleViewer
	} else {
		pint, err := strconv.Atoi(pval)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "permissions must be an integer", nil)
			return
		}
		permissions, err = conversions.NewPermissions(pint)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, err.Error(), nil)
			return
		}
		role = conversions.Permissions2Role(permissions)
	}

	var resourcePermissions *provider.ResourcePermissions
	resourcePermissions, err = h.map2CS3Permissions(role, permissions)
	if err != nil {
		log.Warn().Err(err).Msg("unknown role, mapping legacy permissions")
		resourcePermissions = asCS3Permissions(permissions, nil)
	}

	permissionMap := map[string]string{"name": strconv.Itoa(int(permissions))}
	val, err := json.Marshal(permissionMap)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "could not encode role", err)
		return
	}

	statReq := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path.Join(prefix, r.FormValue("path")),
			},
		},
	}
	statRes, err := c.Stat(ctx, statReq)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc stat request", err)
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

	createShareReq := &ocm.CreateOCMShareRequest{
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"permissions": &types.OpaqueEntry{
					Decoder: "json",
					Value:   val,
				},
			},
		},
		ResourceId: statRes.Info.Id,
		Grant: &ocm.ShareGrant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   remoteUserRes.RemoteUser.GetId(),
			},
			Permissions: &ocm.SharePermissions{
				Permissions: resourcePermissions,
			},
		},
		RecipientMeshProvider: providerInfoResp.ProviderInfo,
	}

	createShareResponse, err := c.CreateOCMShare(ctx, createShareReq)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc create ocm share request", err)
		return
	}
	if createShareResponse.Status.Code != rpc.Code_CODE_OK {
		if createShareResponse.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc create ocm share request failed", err)
		return
	}

	response.WriteOCSSuccess(w, r, "OCM Share created")
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
			share, err = h.userShare2ShareData(ctx, uRes.Share)
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

	share, err := h.userShare2ShareData(ctx, gRes.Share)
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

	response.WriteOCSSuccess(w, r, share)
}

func (h *Handler) removePublicShare(w http.ResponseWriter, r *http.Request, shareID string) {
	ctx := r.Context()

	c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	req := &link.RemovePublicShareRequest{
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: &link.PublicShareId{
					OpaqueId: shareID,
				},
			},
		},
	}

	res, err := c.RemovePublicShare(ctx, req)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc delete share request", err)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc delete share request failed", err)
		return
	}

	response.WriteOCSSuccess(w, r, nil)
}

func (h *Handler) removeUserShare(w http.ResponseWriter, r *http.Request, shareID string) {
	ctx := r.Context()

	uClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	uReq := &collaboration.RemoveShareRequest{
		Ref: &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: shareID,
				},
			},
		},
	}
	uRes, err := uClient.RemoveShare(ctx, uReq)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc delete share request", err)
		return
	}

	if uRes.Status.Code != rpc.Code_CODE_OK {
		if uRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc delete share request failed", err)
		return
	}
	response.WriteOCSSuccess(w, r, nil)
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

const ocsStateAccepted = 0
const ocsStatePending = 1
const ocsStateRejected = 2

func (h *Handler) listSharesWithMe(w http.ResponseWriter, r *http.Request) {
	// which pending state to list
	switch r.FormValue("state") {
	case "all":
		// no filter
	case "0": // accepted
		// TODO implement accepted filter
	case "1": // pending
		// TODO implement pending filter
	case "2": // rejected
		// TODO implement rejected filter
	default:
		// TODO only list accepted shares
	}

	gwc, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
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
	// TODO(refs) filter out "invalid" shares
	for _, rs := range lrsRes.GetShares() {
		statRequest := provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Id{
					Id: rs.Share.ResourceId,
				},
			},
		}

		statResponse, err := gwc.Stat(r.Context(), &statRequest)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, err.Error(), err)
			return
		}

		data, err := h.userShare2ShareData(r.Context(), rs.Share)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, err.Error(), err)
			return
		}

		switch rs.GetState() {
		case collaboration.ShareState_SHARE_STATE_PENDING:
			data.State = ocsStatePending
		case collaboration.ShareState_SHARE_STATE_ACCEPTED:
			data.State = ocsStateAccepted
		case collaboration.ShareState_SHARE_STATE_REJECTED:
			data.State = ocsStateRejected
		default:
			data.State = -1
		}

		err = h.addFileInfo(r.Context(), data, statResponse.Info)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, err.Error(), err)
			return
		}

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

		filters, linkFilters, err = h.addFilters(w, r, hRes.GetPath())
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, err.Error(), err)
			return
		}
	}

	userShares, err := h.listUserShares(r, filters)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, err.Error(), err)
		return
	}

	publicShares, err := h.listPublicShares(r, linkFilters)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, err.Error(), err)
		return
	}
	shares = append(shares, append(userShares, publicShares...)...)

	response.WriteOCSSuccess(w, r, shares)
}

func (h *Handler) listPublicShares(r *http.Request, filters []*link.ListPublicSharesRequest_Filter) ([]*conversions.ShareData, error) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	// TODO(refs) why is this guard needed? Are we moving towards a gateway only for service discovery? without a gateway this is dead code.
	if h.gatewayAddr != "" {
		c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
		if err != nil {
			return nil, err
		}

		req := link.ListPublicSharesRequest{
			Filters: filters,
		}

		res, err := c.ListPublicShares(ctx, &req)
		if err != nil {
			return nil, err
		}

		ocsDataPayload := make([]*conversions.ShareData, 0)
		for _, share := range res.GetShare() {

			statRequest := &provider.StatRequest{
				Ref: &provider.Reference{
					Spec: &provider.Reference_Id{
						Id: share.ResourceId,
					},
				},
			}

			statResponse, err := c.Stat(ctx, statRequest)
			if err != nil {
				return nil, err
			}

			sData := conversions.PublicShare2ShareData(share, r, h.publicURL)
			if statResponse.Status.Code != rpc.Code_CODE_OK {
				return nil, err
			}

			sData.Name = share.DisplayName

			if h.addFileInfo(ctx, sData, statResponse.Info) != nil {
				return nil, err
			}

			log.Debug().Interface("share", share).Interface("info", statResponse.Info).Interface("shareData", share).Msg("mapped")

			ocsDataPayload = append(ocsDataPayload, sData)

		}

		return ocsDataPayload, nil
	}

	return nil, errors.New("bad request")
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
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return collaborationFilters, linkFilters, errors.New("fixme")
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc stat request failed", err)
		return collaborationFilters, linkFilters, errors.New("fixme")
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

func (h *Handler) listUserShares(r *http.Request, filters []*collaboration.ListSharesRequest_Filter) ([]*conversions.ShareData, error) {
	var rInfo *provider.ResourceInfo
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	lsUserSharesRequest := collaboration.ListSharesRequest{
		Filters: filters,
	}

	ocsDataPayload := make([]*conversions.ShareData, 0)
	if h.gatewayAddr != "" {
		// get a connection to the users share provider
		c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
		if err != nil {
			return nil, err
		}

		// do list shares request. unfiltered
		lsUserSharesResponse, err := c.ListShares(ctx, &lsUserSharesRequest)
		if err != nil {
			return nil, err
		}

		if lsUserSharesResponse.Status.Code != rpc.Code_CODE_OK {
			return nil, errors.New("could not ListShares")
		}

		// build OCS response payload
		for _, s := range lsUserSharesResponse.Shares {
			share, err := h.userShare2ShareData(ctx, s)
			if err != nil {
				return nil, err
			}

			// prepare the stat request
			statReq := &provider.StatRequest{
				// prepare the reference
				Ref: &provider.Reference{
					// using ResourceId from the share
					Spec: &provider.Reference_Id{Id: s.ResourceId},
				},
			}

			statResponse, err := c.Stat(ctx, statReq)
			if err != nil {
				return nil, err
			}

			if statResponse.Status.Code != rpc.Code_CODE_OK {
				return nil, errors.New("could not stat share target")
			}

			err = h.addFileInfo(ctx, share, statResponse.Info)
			if err != nil {
				return nil, err
			}

			log.Debug().Interface("share", s).Interface("info", rInfo).Interface("shareData", share).Msg("mapped")
			ocsDataPayload = append(ocsDataPayload, share)
		}
	}

	return ocsDataPayload, nil
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

		c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
		if err != nil {
			return err
		}

		// owner, err := c.GetUser(ctx, &userpb.GetUserRequest{
		// 	UserId: info.Owner,
		// })
		// if err != nil {
		// 	return err
		// }

		// if owner.Status.Code == rpc.Code_CODE_OK {
		// 	// TODO the user from GetUser might not have an ID set, so we are using the one we have
		// 	s.DisplaynameFileOwner = owner.GetUser().DisplayName
		// } else {
		// 	err := errors.New("could not look up share owner")
		// 	log.Err(err).
		// 		Str("user_idp", info.Owner.GetIdp()).
		// 		Str("user_opaque_id", info.Owner.GetOpaqueId()).
		// 		Str("code", owner.Status.Code.String()).
		// 		Msg(owner.Status.Message)
		// 	return err
		// }

		// file owner might not yet be set. Use file info
		if s.UIDFileOwner == "" {
			// TODO we don't know if info.Owner is always set.
			s.UIDFileOwner = response.UserIDToString(info.Owner)
		}
		if s.DisplaynameFileOwner == "" && info.Owner != nil {
			owner, err := c.GetUser(ctx, &userpb.GetUserRequest{
				UserId: info.Owner,
			})
			if err != nil {
				return err
			}

			if owner.Status.Code == rpc.Code_CODE_OK {
				// TODO the user from GetUser might not have an ID set, so we are using the one we have
				s.DisplaynameFileOwner = owner.GetUser().DisplayName
			} else {
				err := errors.New("could not look up share owner")
				log.Err(err).
					Str("user_idp", info.Owner.GetIdp()).
					Str("user_opaque_id", info.Owner.GetOpaqueId()).
					Str("code", owner.Status.Code.String()).
					Msg(owner.Status.Message)
				return err
			}
		}
		// share owner might not yet be set. Use file info
		if s.UIDOwner == "" {
			// TODO we don't know if info.Owner is always set.
			s.UIDOwner = response.UserIDToString(info.Owner)
		}
		if s.DisplaynameOwner == "" && info.Owner != nil {
			owner, err := c.GetUser(ctx, &userpb.GetUserRequest{
				UserId: info.Owner,
			})

			if err != nil {
				return err
			}

			if owner.Status.Code == rpc.Code_CODE_OK {
				// TODO the user from GetUser might not have an ID set, so we are using the one we have
				s.DisplaynameOwner = owner.User.DisplayName
			} else {
				err := errors.New("could not look up file owner")
				log.Err(err).
					Str("user_idp", info.Owner.GetIdp()).
					Str("user_opaque_id", info.Owner.GetOpaqueId()).
					Str("code", owner.Status.Code.String()).
					Msg(owner.Status.Message)
				return err
			}
		}
	}
	return nil
}

// TODO(jfd) merge userShare2ShareData with publicShare2ShareData
func (h *Handler) userShare2ShareData(ctx context.Context, share *collaboration.Share) (*conversions.ShareData, error) {
	sd := &conversions.ShareData{
		Permissions: conversions.UserSharePermissions2OCSPermissions(share.GetPermissions()),
		ShareType:   conversions.ShareTypeUser,
	}

	c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)

	if share.Creator != nil {
		creator, err := c.GetUser(ctx, &userpb.GetUserRequest{
			UserId: share.Creator,
		})
		if err != nil {
			return nil, err
		}

		if creator.Status.Code == rpc.Code_CODE_OK {
			// TODO the user from GetUser might not have an ID set, so we are using the one we have
			sd.UIDOwner = response.UserIDToString(share.Creator)
			sd.DisplaynameOwner = creator.GetUser().DisplayName
		} else {
			log.Err(errors.Wrap(err, "could not look up creator")).
				Str("user_idp", share.Creator.GetIdp()).
				Str("user_opaque_id", share.Creator.GetOpaqueId()).
				Str("code", creator.Status.Code.String()).
				Msg(creator.Status.Message)
			return nil, err
		}
	}
	if share.Owner != nil {
		owner, err := c.GetUser(ctx, &userpb.GetUserRequest{
			UserId: share.Owner,
		})
		if err != nil {
			return nil, err
		}

		if owner.Status.Code == rpc.Code_CODE_OK {
			// TODO the user from GetUser might not have an ID set, so we are using the one we have
			sd.UIDFileOwner = response.UserIDToString(share.Owner)
			sd.DisplaynameFileOwner = owner.GetUser().DisplayName
		} else {
			log.Err(errors.Wrap(err, "could not look up owner")).
				Str("user_idp", share.Owner.GetIdp()).
				Str("user_opaque_id", share.Owner.GetOpaqueId()).
				Str("code", owner.Status.Code.String()).
				Msg(owner.Status.Message)
			return nil, err
		}
	}
	if share.Grantee.Id != nil {
		grantee, err := c.GetUser(ctx, &userpb.GetUserRequest{
			UserId: share.Grantee.GetId(),
		})
		if err != nil {
			return nil, err
		}

		if grantee.Status.Code == rpc.Code_CODE_OK {
			// TODO the user from GetUser might not have an ID set, so we are using the one we have
			sd.ShareWith = response.UserIDToString(share.Grantee.Id)
			sd.ShareWithDisplayname = grantee.GetUser().DisplayName
		} else {
			log.Err(errors.Wrap(err, "could not look up grantee")).
				Str("user_idp", share.Grantee.GetId().GetIdp()).
				Str("user_opaque_id", share.Grantee.GetId().GetOpaqueId()).
				Str("code", grantee.Status.Code.String()).
				Msg(grantee.Status.Message)
			return nil, err
		}
	}
	if share.Id != nil && share.Id.OpaqueId != "" {
		sd.ID = share.Id.OpaqueId
	}
	if share.Ctime != nil {
		sd.STime = share.Ctime.Seconds // TODO CS3 api birth time = btime
	}
	// actually clients should be able to GET and cache the user info themselves ...
	// TODO check grantee type for user vs group
	return sd, nil
}

func (h *Handler) isPublicShare(r *http.Request, oid string) bool {
	logger := appctx.GetLogger(r.Context())
	client, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		logger.Err(err)
	}

	psRes, err := client.GetPublicShare(r.Context(), &link.GetPublicShareRequest{
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: &link.PublicShareId{
					OpaqueId: oid,
				},
			},
		},
	})
	if err != nil {
		logger.Err(err)
	}

	if psRes.GetShare() != nil {
		return true
	}

	// check if we have a user share
	uRes, err := client.GetShare(r.Context(), &collaboration.GetShareRequest{
		Ref: &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: oid,
				},
			},
		},
	})
	if err != nil {
		logger.Err(err)
	}

	if uRes.GetShare() != nil {
		return false
	}

	// TODO token is neither a public or a user share.
	return false
}

func (h *Handler) updatePublicShare(w http.ResponseWriter, r *http.Request, shareID string) {
	updates := []*link.UpdatePublicShareRequest_Update{}
	logger := appctx.GetLogger(r.Context())

	gwC, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		log.Err(err).Str("shareID", shareID).Msg("updatePublicShare")
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "error getting a connection to the gateway service", nil)
		return
	}

	before, err := gwC.GetPublicShare(r.Context(), &link.GetPublicShareRequest{
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: &link.PublicShareId{
					OpaqueId: shareID,
				},
			},
		},
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "failed to get public share", nil)
		return
	}

	err = r.ParseForm()
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Could not parse form from request", err)
		return
	}

	// indicates whether values to update were found,
	// to check if the request was valid,
	// not whether an actual update has been performed
	updatesFound := false

	newName, ok := r.Form["name"]
	if ok {
		updatesFound = true
		if newName[0] != before.Share.DisplayName {
			updates = append(updates, &link.UpdatePublicShareRequest_Update{
				Type:        link.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME,
				DisplayName: newName[0],
			})
		}
	}

	// Permissions
	newPermissions, err := permissionFromRequest(r, h)
	logger.Debug().Interface("newPermissions", newPermissions).Msg("Parsed permissions")
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "invalid permissions", err)
		return
	}

	// update permissions if given
	if newPermissions != nil {
		updatesFound = true
		publicSharePermissions := &link.PublicSharePermissions{
			Permissions: newPermissions,
		}
		beforePerm, _ := json.Marshal(before.GetShare().Permissions)
		afterPerm, _ := json.Marshal(publicSharePermissions)
		if string(beforePerm) != string(afterPerm) {
			logger.Info().Str("shares", "update").Msgf("updating permissions from %v to: %v", string(beforePerm), string(afterPerm))
			updates = append(updates, &link.UpdatePublicShareRequest_Update{
				Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
				Grant: &link.Grant{
					Permissions: publicSharePermissions,
				},
			})
		}
	}

	// ExpireDate
	expireTimeString, ok := r.Form["expireDate"]
	// check if value is set and must be updated or cleared
	if ok {
		updatesFound = true
		var newExpiration *types.Timestamp
		if expireTimeString[0] != "" {
			newExpiration, err = parseTimestamp(expireTimeString[0])
			if err != nil {
				response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "invalid datetime format", err)
				return
			}
		}

		beforeExpiration, _ := json.Marshal(before.Share.Expiration)
		afterExpiration, _ := json.Marshal(newExpiration)
		if string(afterExpiration) != string(beforeExpiration) {
			logger.Debug().Str("shares", "update").Msgf("updating expire date from %v to: %v", string(beforeExpiration), string(afterExpiration))
			updates = append(updates, &link.UpdatePublicShareRequest_Update{
				Type: link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
				Grant: &link.Grant{
					Expiration: newExpiration,
				},
			})
		}
	}

	// Password
	newPassword, ok := r.Form["password"]
	// update or clear password
	if ok {
		updatesFound = true
		logger.Info().Str("shares", "update").Msg("password updated")
		updates = append(updates, &link.UpdatePublicShareRequest_Update{
			Type: link.UpdatePublicShareRequest_Update_TYPE_PASSWORD,
			Grant: &link.Grant{
				Password: newPassword[0],
			},
		})
	}

	publicShare := before.Share

	// Updates are atomical. See: https://github.com/cs3org/cs3apis/pull/67#issuecomment-617651428 so in order to get the latest updated version
	if len(updates) > 0 {
		uRes := &link.UpdatePublicShareResponse{Share: before.Share}
		for k := range updates {
			uRes, err = gwC.UpdatePublicShare(r.Context(), &link.UpdatePublicShareRequest{
				Ref: &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: &link.PublicShareId{
							OpaqueId: shareID,
						},
					},
				},
				Update: updates[k],
			})
			if err != nil {
				log.Err(err).Str("shareID", shareID).Msg("sending update request to public link provider")
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "Error sending update request to public link provider", err)
				return
			}
		}
		publicShare = uRes.Share
	} else if !updatesFound {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "No updates specified in request", nil)
		return
	}

	statReq := provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Id{
				Id: before.Share.ResourceId,
			},
		},
	}

	statRes, err := gwC.Stat(r.Context(), &statReq)
	if err != nil {
		log.Debug().Err(err).Str("shares", "update public share").Msg("error during stat")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing resource information", fmt.Errorf("error getting resource information"))
		return
	}

	s := conversions.PublicShare2ShareData(publicShare, r, h.publicURL)
	err = h.addFileInfo(r.Context(), s, statRes.Info)

	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error enhancing response with share data", err)
		return
	}

	response.WriteOCSSuccess(w, r, s)
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
