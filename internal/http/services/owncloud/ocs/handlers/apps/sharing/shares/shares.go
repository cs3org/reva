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
	"encoding/json"
	"fmt"
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
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) error {
	h.gatewayAddr = c.GatewaySvc
	return nil
}

// Maps oc10 permissions to roles
var ocPermToRole = map[int]string{
	1:  "viewer",
	15: "coowner",
	31: "editor",
	// 5: contributor (?)
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
		case "PUT": // TODO this implementation does NOT differenciates between public vs user shares. A distinction has to be made.
			// update share needs to make a distinction between public / user shares
			if h.isPublicShare(r, strings.ReplaceAll(head, "/", "")) {
				h.updatePublicShare(w, r, strings.ReplaceAll(head, "/", ""))
			} else {
				h.updateShare(w, r, head) // TODO PUT is used with incomplete data to update a share
			}
		case "DELETE":
			// All CRUD operations on shares need to make a distinction between public shares and user shares.
			h.removeShare(w, r, head)
		default:
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Only GET, POST and PUT are allowed", nil)
		}
	}
}

func permissionFromRequest(r *http.Request, h *Handler) *provider.ResourcePermissions {
	// phoenix sends: {"permissions": 15}. See ocPermToRole struct for mapping
	permKey, err := strconv.Atoi(r.FormValue("permissions"))
	if err != nil {
		log.Error().Str("permissionFromRequest", "shares").Msgf("invalid type: %T", permKey)
	}

	perm, ok := ocPermToRole[permKey]
	if !ok {
		log.Error().Str("permissionFromRequest", "shares").Msgf("invalid oC permission: %v", perm)
	}

	p, err := h.role2CS3Permissions(perm)
	if err != nil {
		log.Error().Str("permissionFromRequest", "shares").Msgf("role to cs3permission %v", perm)
	}

	return p
}

func expirationTimestampFromRequest(r *http.Request, h *Handler) *types.Timestamp {
	var expireTime time.Time
	var err error

	expireDate := r.FormValue("expireDate")
	if expireDate != "" {
		expireTime, err = time.Parse("2006-01-02T15:04:05Z0700", expireDate)
		if err != nil {
			log.Error().Str("expiration", "create public share").Msgf("date format invalid: %v", expireDate)
		}
		final := expireTime.UnixNano()

		return &types.Timestamp{
			Seconds: uint64(final / 1000000000),
			Nanos:   uint32(final % 1000000000),
		}
	}

	return nil
}

func (h *Handler) updatePublicShare(w http.ResponseWriter, r *http.Request, token string) {
	updates := []*link.UpdatePublicShareRequest_Update{}
	shares := make([]*conversions.ShareData, 0)
	logger := appctx.GetLogger(r.Context())

	gwC, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		log.Err(err).Str("updatePublicShare ref:", token).Msg("updating")
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "error getting a connection to the gateway service", nil)
		return
	}

	before, err := gwC.GetPublicShare(r.Context(), &link.GetPublicShareRequest{
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: &link.PublicShareId{
					OpaqueId: token,
				},
			},
		},
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "failed to get public share", nil)
		return
	}

	if r.FormValue("name") != before.GetShare().DisplayName {
		updates = append(updates, &link.UpdatePublicShareRequest_Update{
			Type:        link.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME,
			DisplayName: r.FormValue("name"),
		})
	}

	// Permissions
	publicSharePermissions := &link.PublicSharePermissions{
		Permissions: permissionFromRequest(r, h),
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

	// ExpireDate
	newExpiration := expirationTimestampFromRequest(r, h)
	beforeExpiration, _ := json.Marshal(before.Share.Expiration)
	afterExpiration, _ := json.Marshal(newExpiration)
	if newExpiration != nil || (string(afterExpiration) != string(beforeExpiration)) {
		logger.Info().Str("shares", "update").Msgf("updating expire date from %v to: %v", string(beforeExpiration), string(afterExpiration))
		updates = append(updates, &link.UpdatePublicShareRequest_Update{
			Type: link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
			Grant: &link.Grant{
				Expiration: newExpiration,
			},
		})
	}

	// Password
	if len(r.FormValue("password")) > 0 {
		logger.Info().Str("shares", "update").Msg("password updated")
		updates = append(updates, &link.UpdatePublicShareRequest_Update{
			Type: link.UpdatePublicShareRequest_Update_TYPE_PASSWORD,
			Grant: &link.Grant{
				Password: r.FormValue("password"),
			},
		})
	}

	// Updates are atomical. See: https://github.com/cs3org/cs3apis/pull/67#issuecomment-617651428 so in order to get the latest updated version
	if len(updates) == 0 {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "There is nothing to update.", nil) // TODO(refs) error? a simple noop might suffice
		return
	}

	req := &link.UpdatePublicShareResponse{}
	for k := range updates {
		req, err = gwC.UpdatePublicShare(r.Context(), &link.UpdatePublicShareRequest{
			Ref: &link.PublicShareReference{
				Spec: &link.PublicShareReference_Id{
					Id: &link.PublicShareId{
						OpaqueId: token,
					},
				},
			},
			Update: updates[k],
		})
		if err != nil {
			log.Err(err).Str("updatePublicShare ref:", token).Msg("sending update request to public link provider")
		}
	}

	statReq := provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Id{
				Id: req.GetShare().GetResourceId(),
			},
		},
	}

	statRes, err := gwC.Stat(r.Context(), &statReq)
	if err != nil {
		log.Debug().Err(err).Str("shares", "update public share").Msg("error during stat")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing resource information", fmt.Errorf("error getting resource information"))
		return
	}

	s := conversions.PublicShare2ShareData(req.Share, r)
	err = h.addFileInfo(r.Context(), s, statRes.Info)

	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error enhancing response with share data", err)
		return
	}

	shares = append(shares, s)
	response.WriteOCSSuccess(w, r, shares)
}

func (h *Handler) createShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	shareType, err := strconv.Atoi(r.FormValue("shareType"))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "shareType must be an integer", nil)
		return
	}

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

	if shareType == int(conversions.ShareTypeUser) {

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

		var permissions conversions.Permissions

		role := r.FormValue("role")
		// 2. if we don't have a role try to map the permissions
		if role == "" {
			pval := r.FormValue("permissions")
			if pval == "" {
				// by default only allow read permissions / assign viewer role
				role = conversions.RoleViewer
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

		var resourcePermissions *provider.ResourcePermissions
		resourcePermissions, err = h.role2CS3Permissions(role)
		if err != nil {
			log.Warn().Err(err).Msg("unknown role, mapping legacy permissions")
			resourcePermissions = asCS3Permissions(permissions, nil)
		}

		roleMap := map[string]string{"name": role}
		val, err := json.Marshal(roleMap)
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
		s.Path = r.FormValue("path") // use path without user prefix
		// s.MailSend = "0"
		response.WriteOCSSuccess(w, r, s)
		return
	}

	// create a public link share
	if shareType == int(conversions.ShareTypePublicLink) {

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

		// phoenix sends: {"permissions": 15}. See ocPermToRole struct for mapping
		permKey, err := strconv.Atoi(r.FormValue("permissions"))
		if err != nil {
			log.Error().Str("createShare", "shares").Msgf("invalid type: %T", permKey)
		}

		perm, ok := ocPermToRole[permKey]
		if !ok {
			log.Error().Str("createShare", "shares").Msgf("invalid oC permission: %v", perm)
		}

		p, err := h.role2CS3Permissions(perm)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "invalid role", err)
			return
		}

		req := link.CreatePublicShareRequest{
			ResourceInfo: statRes.GetInfo(),
			Grant: &link.Grant{
				Permissions: &link.PublicSharePermissions{
					Permissions: p,
				},
				Password: r.FormValue("password"),
			},
		}

		expireDate := r.FormValue("expireDate")
		if expireDate != "" {
			if err != nil {
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "invalid date format", err)
				return
			}
			req.Grant.Expiration = expirationTimestampFromRequest(r, h)
		}

		// set displayname and password protected as arbitrary metadata
		req.ResourceInfo.ArbitraryMetadata = &provider.ArbitraryMetadata{
			Metadata: map[string]string{
				"name":     r.FormValue("name"),
				"password": r.FormValue("password"),
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

		s := conversions.PublicShare2ShareData(createRes.Share, r)
		err = h.addFileInfo(ctx, s, statRes.Info)

		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error enhancing response with share data", err)
			return
		}

		response.WriteOCSSuccess(w, r, s)

		return
	}

	if shareType == int(conversions.ShareTypeFederatedCloudShare) {

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
		resourcePermissions, err = h.role2CS3Permissions(role)
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
		return
	}

	response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "unknown share type", nil)
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

func (h *Handler) role2CS3Permissions(r string) (*provider.ResourcePermissions, error) {
	switch r {
	case conversions.RoleViewer:
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
		}, nil
	case conversions.RoleEditor:
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,

			Move:               true,
			InitiateFileUpload: true,
			RestoreFileVersion: true,
			RestoreRecycleItem: true,
			CreateContainer:    true,
			Delete:             true,
			PurgeRecycle:       true,
		}, nil
	case conversions.RoleCoowner:
		return &provider.ResourcePermissions{
			ListContainer:        true,
			ListGrants:           true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,

			Move:               true,
			InitiateFileUpload: true,
			RestoreFileVersion: true,
			RestoreRecycleItem: true,
			CreateContainer:    true,
			Delete:             true,
			PurgeRecycle:       true,

			AddGrant:    true,
			RemoveGrant: true, // TODO when are you able to unshare / delete
			UpdateGrant: true,
		}, nil
	default:
		return nil, fmt.Errorf("unknown role: %s", r)
	}
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

	response.WriteOCSSuccess(w, r, share)
}

func (h *Handler) removeShare(w http.ResponseWriter, r *http.Request, shareID string) {
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

			sData := conversions.PublicShare2ShareData(share, r)
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

func (h *Handler) addFileInfo(ctx context.Context, s *conversions.ShareData, info *provider.ResourceInfo) error {
	if info != nil {
		// TODO The owner is not set in the storage stat metadata ...
		s.MimeType = info.MimeType
		// TODO STime:     &types.Timestamp{Seconds: info.Mtime.Seconds, Nanos: info.Mtime.Nanos},
		s.StorageID = info.Id.StorageId
		// TODO Storage: int
		s.ItemSource = info.Id.OpaqueId
		s.FileSource = info.Id.OpaqueId
		s.FileTarget = path.Join("/", path.Base(info.Path))
		s.Path = info.Path // TODO hm this might have to be relative to the users home ... depends on the webdav_namespace config
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
