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
	"net/http"
	"path"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// AcceptReceivedShare handles Post Requests on /apps/files_sharing/api/v1/shares/{shareid}.
func (h *Handler) AcceptReceivedShare(w http.ResponseWriter, r *http.Request) {
	shareID, _ := mux.ParamsFromRequest(r).Get("shareid")
	if h.isFederatedReceivedShare(r, shareID) {
		h.updateReceivedFederatedShare(w, r, shareID, false)
	} else {
		h.updateReceivedShare(w, r, shareID, false)
	}
}

// RejectReceivedShare handles DELETE Requests on /apps/files_sharing/api/v1/shares/{shareid}.
func (h *Handler) RejectReceivedShare(w http.ResponseWriter, r *http.Request) {
	shareID, _ := mux.ParamsFromRequest(r).Get("shareid")
	if h.isFederatedReceivedShare(r, shareID) {
		h.updateReceivedFederatedShare(w, r, shareID, true)
	} else {
		h.updateReceivedShare(w, r, shareID, true)
	}
}

func (h *Handler) updateReceivedShare(w http.ResponseWriter, r *http.Request, shareID string, rejectShare bool) {
	ctx := r.Context()
	logger := appctx.GetLogger(ctx)

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	shareRequest := &collaboration.UpdateReceivedShareRequest{
		Share: &collaboration.ReceivedShare{
			Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: shareID}},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	}
	if rejectShare {
		shareRequest.Share.State = collaboration.ShareState_SHARE_STATE_REJECTED
	} else {
		// TODO find free mount point and pass it on with an updated field mask
		shareRequest.Share.State = collaboration.ShareState_SHARE_STATE_ACCEPTED
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

	rs := shareRes.GetShare()

	info, status, err := h.getResourceInfoByID(ctx, client, rs.Share.ResourceId)
	if err != nil || status.Code != rpc.Code_CODE_OK {
		h.logProblems(status, err, "could not stat, skipping", logger)
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
		data.Path = path.Join(h.sharePrefix, path.Base(info.Path))
	}

	response.WriteOCSSuccess(w, r, []*conversions.ShareData{data})
}

func (h *Handler) updateReceivedFederatedShare(w http.ResponseWriter, r *http.Request, shareID string, rejectShare bool) {
	ctx := r.Context()

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	share, err := client.GetReceivedOCMShare(ctx, &ocmv1beta1.GetReceivedOCMShareRequest{
		Ref: &ocmv1beta1.ShareReference{
			Spec: &ocmv1beta1.ShareReference_Id{
				Id: &ocmv1beta1.ShareId{
					OpaqueId: shareID,
				},
			},
		},
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc update received share request failed", err)
		return
	}
	if share.Status.Code != rpc.Code_CODE_OK {
		if share.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc update received share request failed", errors.Errorf("code: %d, message: %s", share.Status.Code, share.Status.Message))
		return
	}

	req := &ocmv1beta1.UpdateReceivedOCMShareRequest{
		Share: &ocmv1beta1.ReceivedShare{
			Id: &ocmv1beta1.ShareId{
				OpaqueId: shareID,
			},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	}
	if rejectShare {
		req.Share.State = ocmv1beta1.ShareState_SHARE_STATE_REJECTED
	} else {
		req.Share.State = ocmv1beta1.ShareState_SHARE_STATE_ACCEPTED
	}

	updateRes, err := client.UpdateReceivedOCMShare(ctx, req)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc update received share request failed", err)
		return
	}

	if updateRes.Status.Code != rpc.Code_CODE_OK {
		if updateRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc update received share request failed", errors.Errorf("code: %d, message: %s", updateRes.Status.Code, updateRes.Status.Message))
		return
	}

	data, err := conversions.ReceivedOCMShare2ShareData(share.Share, h.ocmLocalMount(share.Share))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc update received share request failed", err)
		return
	}
	h.mapUserIdsReceivedFederatedShare(ctx, client, data)
	data.State = mapOCMState(req.Share.State)
	response.WriteOCSSuccess(w, r, []*conversions.ShareData{data})
}
