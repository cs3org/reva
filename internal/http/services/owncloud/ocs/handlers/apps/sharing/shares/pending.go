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
	"path"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (h *Handler) updateReceivedShare(w http.ResponseWriter, r *http.Request, shareID string, rejectShare bool) {
	ctx := r.Context()
	logger := appctx.GetLogger(ctx)

	client, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	ref := &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{
			Id: &collaboration.ShareId{
				OpaqueId: shareID,
			},
		},
	}

	shareRequest := &collaboration.UpdateReceivedShareRequest{
		Ref: ref,
		Field: &collaboration.UpdateReceivedShareRequest_UpdateField{
			Field: &collaboration.UpdateReceivedShareRequest_UpdateField_State{
				State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
			},
		},
	}
	if rejectShare {
		shareRequest.Field = &collaboration.UpdateReceivedShareRequest_UpdateField{
			Field: &collaboration.UpdateReceivedShareRequest_UpdateField_State{
				State: collaboration.ShareState_SHARE_STATE_REJECTED,
			},
		}
	}

	shareRes, err := client.UpdateReceivedShare(ctx, shareRequest)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc update received share request failed", err)
		return
	}

	if shareRes.Status.Code != rpc.Code_CODE_OK {
		if shareRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc update received share request failed", errors.Errorf("code: %d, message: %s", shareRes.Status.Code, shareRes.Status.Message))
		return
	}

	getRes, err := client.GetReceivedShare(ctx, &collaboration.GetReceivedShareRequest{
		Ref: ref,
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc get received share request failed", err)
		return
	}

	if getRes.Status.Code != rpc.Code_CODE_OK {
		if getRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc get share request failed", errors.Errorf("code: %d, message: %s", getRes.Status.Code, getRes.Status.Message))
		return
	}

	rs := getRes.Share

	info, status, err := h.getResourceInfoByID(ctx, client, rs.Share.ResourceId)
	if err != nil || status.Code != rpc.Code_CODE_OK {
		h.logProblems(status, err, "could not stat, skipping")
	}

	data, err := conversions.CS3Share2ShareData(r.Context(), rs.Share)
	if err != nil {
		logger.Debug().Interface("share", rs.Share).Interface("shareData", data).Err(err).Msg("could not CS3Share2ShareData, skipping")
	}

	data.State = mapState(rs.GetState())

	if err := h.addFileInfo(ctx, data, info); err != nil {
		logger.Debug().Interface("received_share", rs).Interface("info", info).Interface("shareData", data).Err(err).Msg("could not add file info, skipping")
	}
	h.mapUserIds(r.Context(), client, data)

	if data.State == ocsStateAccepted {
		// Needed because received shares can be jailed in a folder in the users home
		data.FileTarget = path.Join(h.sharePrefix, path.Base(info.Path))
		data.Path = path.Join(h.sharePrefix, path.Base(info.Path))
	}

	response.WriteOCSSuccess(w, r, data)
}
