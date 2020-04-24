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

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

func (h *Handler) getShare(w http.ResponseWriter, r *http.Request, shareID string) {

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

func (h *Handler) listAllShares(w http.ResponseWriter, r *http.Request) {

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
