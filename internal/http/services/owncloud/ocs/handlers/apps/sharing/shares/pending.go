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
	"context"
	"fmt"
	"net/http"
	"path"
	"sort"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/go-chi/chi/v5"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

const (
	// shareidkey is the key user to obtain the id of the share to update. It is present in the request URL.
	shareidkey string = "shareid"
)

// AcceptReceivedShare handles Post Requests on /apps/files_sharing/api/v1/shares/{shareid}
func (h *Handler) AcceptReceivedShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	shareID := chi.URLParam(r, shareidkey)
	client, err := h.getClient()
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	rs, ocsResponse := getReceivedShareFromID(ctx, client, shareID)
	if ocsResponse != nil {
		response.WriteOCSResponse(w, r, *ocsResponse, nil)
		return
	}

	sharedResource, ocsResponse := getSharedResource(ctx, client, rs.Share.Share.ResourceId)
	if ocsResponse != nil {
		response.WriteOCSResponse(w, r, *ocsResponse, nil)
		return
	}

	lrs, ocsResponse := getSharesList(ctx, client)
	if ocsResponse != nil {
		response.WriteOCSResponse(w, r, *ocsResponse, nil)
		return
	}

	// we need to sort the received shares by mount point in order to make things easier to evaluate.
	base := path.Base(sharedResource.GetInfo().GetPath())
	mount := base
	var mountPoints []string
	sharesToAccept := map[string]bool{shareID: true}
	for _, s := range lrs.Shares {
		if utils.ResourceIDEqual(s.Share.ResourceId, rs.Share.Share.GetResourceId()) {
			if s.State == collaboration.ShareState_SHARE_STATE_ACCEPTED {
				mount = s.MountPoint.Path
			} else {
				sharesToAccept[s.Share.Id.OpaqueId] = true
			}
		} else {
			if s.State == collaboration.ShareState_SHARE_STATE_ACCEPTED {
				mountPoints = append(mountPoints, s.MountPoint.Path)
			}
		}
	}

	sort.Strings(mountPoints)

	// now we have a list of shares, we want to iterate over all of them and check for name collisions
	// FIXME: adjust logic
	/*
		for i, mp := range mountPoints {
			if mp == mount {
				mount = fmt.Sprintf("%s (%s)", base, strconv.Itoa(i+1))
			}
		}
	*/

	for id := range sharesToAccept {
		h.updateReceivedShare(w, r, id, false, mount)
	}
}

// RejectReceivedShare handles DELETE Requests on /apps/files_sharing/api/v1/shares/{shareid}
func (h *Handler) RejectReceivedShare(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareid")
	h.updateReceivedShare(w, r, shareID, true, "")
}

func (h *Handler) updateReceivedShare(w http.ResponseWriter, r *http.Request, shareID string, rejectShare bool, mountPoint string) {
	ctx := r.Context()
	logger := appctx.GetLogger(ctx)

	client, err := h.getClient()
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	// we need to add a path to the share
	shareRequest := &collaboration.UpdateReceivedShareRequest{
		Share: &collaboration.ReceivedShare{
			Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: shareID}},
			MountPoint: &provider.Reference{
				Path: mountPoint,
			},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	}
	if rejectShare {
		shareRequest.Share.State = collaboration.ShareState_SHARE_STATE_REJECTED
	} else {
		shareRequest.UpdateMask.Paths = append(shareRequest.UpdateMask.Paths, "mount_point")
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
		h.logProblems(status, err, "could not stat, skipping")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc get resource info failed", errors.Errorf("code: %d, message: %s", status.Code, status.Message))
		return
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

// getReceivedShareFromID uses a client to the gateway to fetch a share based on its ID.
func getReceivedShareFromID(ctx context.Context, client GatewayClient, shareID string) (*collaboration.GetReceivedShareResponse, *response.Response) {
	s, err := client.GetReceivedShare(ctx, &collaboration.GetReceivedShareRequest{
		Ref: &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: shareID,
				}},
		},
	})

	if err != nil {
		e := errors.Wrap(err, fmt.Sprintf("could not get share with ID: `%s`", shareID))
		return nil, arbitraryOcsResponse(response.MetaServerError.StatusCode, e.Error())
	}

	if s.Status.Code != rpc.Code_CODE_OK {
		if s.Status.Code == rpc.Code_CODE_NOT_FOUND {
			e := fmt.Errorf("share not found")
			return nil, arbitraryOcsResponse(response.MetaNotFound.StatusCode, e.Error())
		}

		e := fmt.Errorf("invalid share: %s", s.GetStatus().GetMessage())
		return nil, arbitraryOcsResponse(response.MetaBadRequest.StatusCode, e.Error())
	}

	return s, nil
}

// getSharedResource attempts to get a shared resource from the storage from the resource reference.
func getSharedResource(ctx context.Context, client GatewayClient, resID *provider.ResourceId) (*provider.StatResponse, *response.Response) {
	res, err := client.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: resID,
		},
	})
	if err != nil {
		e := fmt.Errorf("could not get reference")
		return nil, arbitraryOcsResponse(response.MetaServerError.StatusCode, e.Error())
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			e := fmt.Errorf("not found")
			return nil, arbitraryOcsResponse(response.MetaNotFound.StatusCode, e.Error())
		}
		e := fmt.Errorf(res.GetStatus().GetMessage())
		return nil, arbitraryOcsResponse(response.MetaServerError.StatusCode, e.Error())
	}

	return res, nil
}

// getSharedResource gets the list of all shares for the current user.
func getSharesList(ctx context.Context, client GatewayClient) (*collaboration.ListReceivedSharesResponse, *response.Response) {
	shares, err := client.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
	if err != nil {
		e := errors.Wrap(err, "error getting shares list")
		return nil, arbitraryOcsResponse(response.MetaNotFound.StatusCode, e.Error())
	}

	if shares.Status.Code != rpc.Code_CODE_OK {
		if shares.Status.Code == rpc.Code_CODE_NOT_FOUND {
			e := fmt.Errorf("not found")
			return nil, arbitraryOcsResponse(response.MetaNotFound.StatusCode, e.Error())
		}
		e := fmt.Errorf(shares.GetStatus().GetMessage())
		return nil, arbitraryOcsResponse(response.MetaServerError.StatusCode, e.Error())
	}
	return shares, nil
}

// arbitraryOcsResponse abstracts the boilerplate that is creating a response.Response struct.
func arbitraryOcsResponse(statusCode int, message string) *response.Response {
	r := response.Response{
		OCS: &response.Payload{
			XMLName: struct{}{},
			Meta:    response.Meta{},
			Data:    nil,
		},
	}

	r.OCS.Meta.StatusCode = statusCode
	r.OCS.Meta.Message = message
	return &r
}
