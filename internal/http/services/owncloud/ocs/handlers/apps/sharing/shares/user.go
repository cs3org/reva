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
	"net/http"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

func (h *Handler) createUserShare(w http.ResponseWriter, r *http.Request, statInfo *provider.ResourceInfo, role *conversions.Role, roleVal []byte) {
	ctx := r.Context()
	c, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	shareWith := r.FormValue("shareWith")
	if shareWith == "" {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "missing shareWith", nil)
		return
	}

	userRes, err := c.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim: "username",
		Value: shareWith,
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error searching recipient", err)
		return
	}

	if userRes.Status.Code != rpc.Code_CODE_OK {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "user not found", err)
		return
	}

	createShareReq := &collaboration.CreateShareRequest{
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"role": {
					Decoder: "json",
					Value:   roleVal,
				},
			},
		},
		ResourceInfo: statInfo,
		Grant: &collaboration.ShareGrant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_UserId{UserId: userRes.User.GetId()},
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: role.CS3ResourcePermissions(),
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
	s, err := conversions.CS3Share2ShareData(ctx, createShareResponse.Share)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error mapping share data", err)
		return
	}
	err = h.addFileInfo(ctx, s, statInfo)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error adding fileinfo to share", err)
		return
	}
	h.mapUserIds(ctx, c, s)

	response.WriteOCSSuccess(w, r, s)
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

func (h *Handler) listUserShares(r *http.Request, filters []*collaboration.ListSharesRequest_Filter) ([]*conversions.ShareData, *rpc.Status, error) {
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
			return ocsDataPayload, nil, err
		}

		// do list shares request. filtered
		lsUserSharesResponse, err := c.ListShares(ctx, &lsUserSharesRequest)
		if err != nil {
			return ocsDataPayload, nil, err
		}
		if lsUserSharesResponse.Status.Code != rpc.Code_CODE_OK {
			return ocsDataPayload, lsUserSharesResponse.Status, nil
		}

		// build OCS response payload
		for _, s := range lsUserSharesResponse.Shares {
			data, err := conversions.CS3Share2ShareData(ctx, s)
			if err != nil {
				log.Debug().Interface("share", s).Interface("shareData", data).Err(err).Msg("could not CS3Share2ShareData, skipping")
				continue
			}

			// prepare the stat request
			statReq := &provider.StatRequest{
				Ref: &provider.Reference{
					Spec: &provider.Reference_Id{Id: s.ResourceId},
				},
			}

			statResponse, err := c.Stat(ctx, statReq)
			if err != nil || statResponse.Status.Code != rpc.Code_CODE_OK {
				log.Debug().Interface("share", s).Interface("response", statResponse).Interface("shareData", data).Err(err).Msg("could not stat share, skipping")
				continue
			}

			if err := h.addFileInfo(ctx, data, statResponse.Info); err != nil {
				log.Debug().Interface("share", s).Interface("info", statResponse.Info).Interface("shareData", data).Err(err).Msg("could not add file info, skipping")
				continue
			}
			h.mapUserIds(ctx, c, data)

			log.Debug().Interface("share", s).Interface("info", rInfo).Interface("shareData", data).Msg("mapped")
			ocsDataPayload = append(ocsDataPayload, data)
		}
	}

	return ocsDataPayload, nil, nil
}
