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
	"net/http"
	"path"
	"strconv"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

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
					Decoder: "plain",
					Value:   []byte(strconv.Itoa(int(permissions))),
				},
				"name": &types.OpaqueEntry{
					Decoder: "plain",
					Value:   []byte(statRes.Info.Path),
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

func (h *Handler) getFederatedShare(w http.ResponseWriter, r *http.Request, shareID string) {

	// TODO: Implement response with HAL schemating
	ctx := r.Context()

	gatewayClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	listOCMSharesRequest := &ocm.GetOCMShareRequest{
		Ref: &ocm.ShareReference{
			Spec: &ocm.ShareReference_Id{
				Id: &ocm.ShareId{
					OpaqueId: shareID,
				},
			},
		},
	}
	ocmShareResponse, err := gatewayClient.GetOCMShare(ctx, listOCMSharesRequest)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc get ocm share request", err)
		return
	}

	share := ocmShareResponse.GetShare()
	if share == nil {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "share not found", err)
		return
	}
	response.WriteOCSSuccess(w, r, share)
}

func (h *Handler) listFederatedShares(w http.ResponseWriter, r *http.Request) {

	// TODO Implement pagination.
	// TODO Implement response with HAL schemating
	ctx := r.Context()

	gatewayClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	listOCMSharesResponse, err := gatewayClient.ListOCMShares(ctx, &ocm.ListOCMSharesRequest{})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc list ocm share request", err)
		return
	}

	shares := listOCMSharesResponse.GetShares()
	if shares == nil {
		shares = make([]*ocm.Share, 0)
	}
	response.WriteOCSSuccess(w, r, shares)
}
