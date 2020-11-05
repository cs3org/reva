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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

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
			return nil, nil, err
		}

		// do list shares request. filtered
		lsUserSharesResponse, err := c.ListShares(ctx, &lsUserSharesRequest)
		if err != nil || lsUserSharesResponse.Status.Code != rpc.Code_CODE_OK {
			return nil, lsUserSharesResponse.Status, err
		}

		// build OCS response payload
		for _, s := range lsUserSharesResponse.Shares {
			data, err := conversions.UserShare2ShareData(ctx, s)
			if err != nil {
				log.Debug().Interface("share", s).Interface("shareData", data).Err(err).Msg("could not UserShare2ShareData, skipping")
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
			h.addDisplaynames(ctx, c, data)

			log.Debug().Interface("share", s).Interface("info", rInfo).Interface("shareData", data).Msg("mapped")
			ocsDataPayload = append(ocsDataPayload, data)
		}
	}

	return ocsDataPayload, nil, nil
}
