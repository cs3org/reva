// Copyright 2018-2024 CERN
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
	"net/http"
	"strconv"
	"strings"
	"sync"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/v3/pkg/appctx"

	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
)

func (h *Handler) createUserShare(w http.ResponseWriter, r *http.Request, statInfo *provider.ResourceInfo, role *conversions.Role, roleVal []byte) {
	ctx := r.Context()
	c, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	shareWith := r.FormValue("shareWith")
	if shareWith == "" {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "missing shareWith", nil)
		return
	}
	sharees := strings.Split(shareWith, ",")

	shares := make([]*conversions.ShareData, 0)

	for _, sharee := range sharees {
		userRes, err := c.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
			Claim:                  "username",
			Value:                  sharee,
			SkipFetchingUserGroups: true,
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

		newShare, err := h.createCs3Share(ctx, r, c, createShareReq, statInfo)
		if err == nil {
			if newShare != nil {
				shares = append(shares, newShare)
			}
			notify, _ := strconv.ParseBool(r.FormValue("notify"))
			if notify {
				granter, ok := appctx.ContextGetUser(ctx)
				if ok {
					h.SendShareNotification(newShare.ID, granter, userRes.User, statInfo)
				}
			}
		} else {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "An error occurred when creating the share", err)
		}
	}
	response.WriteOCSSuccess(w, r, shares)
}

func (h *Handler) isUserShare(r *http.Request, oid string) bool {
	logger := appctx.GetLogger(r.Context())
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		logger.Err(err).Send()
	}

	getShareRes, err := client.GetShare(r.Context(), &collaboration.GetShareRequest{
		Ref: &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: oid,
				},
			},
		},
	})
	if err != nil {
		logger.Err(err).Send()
		return false
	}

	return getShareRes.GetShare() != nil
}

func (h *Handler) removeUserShare(w http.ResponseWriter, r *http.Request, shareID string) {
	ctx := r.Context()

	uClient, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	shareRef := &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{
			Id: &collaboration.ShareId{
				OpaqueId: shareID,
			},
		},
	}
	// Get the share, so that we can include it in the response.
	getShareResp, err := uClient.GetShare(ctx, &collaboration.GetShareRequest{Ref: shareRef})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc delete share request", err)
		return
	} else if getShareResp.Status.Code != rpc.Code_CODE_OK {
		if getShareResp.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "deleting share failed", err)
		return
	}

	data, err := conversions.CS3Share2ShareData(ctx, getShareResp.Share)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "deleting share failed", err)
		return
	}
	// A deleted share should not have an ID.
	data.ID = ""

	uReq := &collaboration.RemoveShareRequest{Ref: shareRef}
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
	response.WriteOCSSuccess(w, r, data)
}

func (h *Handler) isFederatedShare(r *http.Request, shareID string) bool {
	log := appctx.GetLogger(r.Context())
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		log.Err(err).Send()
		return false
	}

	getShareRes, err := client.GetOCMShare(r.Context(), &ocmpb.GetOCMShareRequest{
		Ref: &ocmpb.ShareReference{
			Spec: &ocmpb.ShareReference_Id{
				Id: &ocmpb.ShareId{
					OpaqueId: shareID,
				},
			},
		},
	})
	if err != nil {
		log.Err(err).Send()
		return false
	}

	return getShareRes.GetShare() != nil
}

func (h *Handler) isFederatedReceivedShare(r *http.Request, shareID string) bool {
	log := appctx.GetLogger(r.Context())
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		log.Err(err).Send()
		return false
	}

	getShareRes, err := client.GetReceivedOCMShare(r.Context(), &ocmpb.GetReceivedOCMShareRequest{
		Ref: &ocmpb.ShareReference{
			Spec: &ocmpb.ShareReference_Id{
				Id: &ocmpb.ShareId{
					OpaqueId: shareID,
				},
			},
		},
	})
	if err != nil {
		log.Err(err).Send()
		return false
	}

	return getShareRes.GetShare() != nil
}

func (h *Handler) removeFederatedShare(w http.ResponseWriter, r *http.Request, shareID string) {
	ctx := r.Context()

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	shareRef := &ocmpb.ShareReference_Id{Id: &ocmpb.ShareId{OpaqueId: shareID}}
	// Get the share, so that we can include it in the response.
	getShareResp, err := client.GetOCMShare(ctx, &ocmpb.GetOCMShareRequest{Ref: &ocmpb.ShareReference{Spec: shareRef}})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc delete share request", err)
		return
	}
	if getShareResp.Status.Code != rpc.Code_CODE_OK {
		if getShareResp.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "deleting share failed", err)
		return
	}

	data, err := conversions.OCMShare2ShareData(getShareResp.Share)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "deleting share failed", err)
		return
	}
	// A deleted share should not have an ID.
	data.ID = ""

	uRes, err := client.RemoveOCMShare(ctx, &ocmpb.RemoveOCMShareRequest{Ref: &ocmpb.ShareReference{Spec: shareRef}})
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
	response.WriteOCSSuccess(w, r, data)
}

func (h *Handler) listUserShares(r *http.Request, filters []*collaboration.Filter, ctxPath string) ([]*conversions.ShareData, *rpc.Status, error) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	lsUserSharesRequest := collaboration.ListSharesRequest{
		Filters: filters,
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				appctx.ResoucePathCtx: {Decoder: "plain", Value: []byte(ctxPath)},
			},
		},
	}

	ocsDataPayload := make([]*conversions.ShareData, 0)
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		return ocsDataPayload, nil, err
	}

	// do list shares request. filtered
	lsUserSharesResponse, err := client.ListShares(ctx, &lsUserSharesRequest)
	if err != nil {
		return ocsDataPayload, nil, err
	}
	if lsUserSharesResponse.Status.Code != rpc.Code_CODE_OK {
		return ocsDataPayload, lsUserSharesResponse.Status, nil
	}

	var wg sync.WaitGroup
	workers := 50
	input := make(chan *collaboration.Share, len(lsUserSharesResponse.Shares))
	output := make(chan *conversions.ShareData, len(lsUserSharesResponse.Shares))

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(ctx context.Context, client gateway.GatewayAPIClient, input chan *collaboration.Share, output chan *conversions.ShareData, wg *sync.WaitGroup) {
			defer wg.Done()

			// build OCS response payload
			for s := range input {
				data, err := conversions.CS3Share2ShareData(ctx, s)
				if err != nil {
					log.Debug().Interface("share", s.Id).Err(err).Msg("CS3Share2ShareData returned error, skipping")
					return
				}

				info, status, err := h.getResourceInfoByID(ctx, client, s.ResourceId)
				if err != nil || status.Code != rpc.Code_CODE_OK {
					log.Debug().Interface("share", s.Id).Interface("status", status).Err(err).Msg("could not stat share, skipping")
					return
				}

				if err := h.addFileInfo(ctx, data, info); err != nil {
					log.Debug().Interface("share", s.Id).Err(err).Msg("could not add file info, skipping")
					return
				}
				h.mapUserIds(ctx, client, data)

				log.Debug().Interface("share", s.Id).Msg("mapped")
				output <- data
			}
		}(ctx, client, input, output, &wg)
	}

	for _, share := range lsUserSharesResponse.Shares {
		input <- share
	}
	close(input)
	wg.Wait()
	close(output)

	for s := range output {
		ocsDataPayload = append(ocsDataPayload, s)
	}

	if h.listOCMShares {
		// include the ocm shares
		ocmShares, err := h.listOutcomingFederatedShares(ctx, client, convertToOCMFilters(filters))
		if err != nil {
			return nil, nil, err
		}
		ocsDataPayload = append(ocsDataPayload, ocmShares...)
	}

	return ocsDataPayload, nil, nil
}

func convertToOCMFilters(filters []*collaboration.Filter) []*ocmpb.ListOCMSharesRequest_Filter {
	ocmfilters := []*ocmpb.ListOCMSharesRequest_Filter{}
	for _, f := range filters {
		switch v := f.Term.(type) {
		case *collaboration.Filter_ResourceId:
			ocmfilters = append(ocmfilters, &ocmpb.ListOCMSharesRequest_Filter{
				Type: ocmpb.ListOCMSharesRequest_Filter_TYPE_RESOURCE_ID,
				Term: &ocmpb.ListOCMSharesRequest_Filter_ResourceId{
					ResourceId: v.ResourceId,
				},
			})
		case *collaboration.Filter_Creator:
			ocmfilters = append(ocmfilters, &ocmpb.ListOCMSharesRequest_Filter{
				Type: ocmpb.ListOCMSharesRequest_Filter_TYPE_CREATOR,
				Term: &ocmpb.ListOCMSharesRequest_Filter_Creator{
					Creator: v.Creator,
				},
			})
		case *collaboration.Filter_Owner:
			ocmfilters = append(ocmfilters, &ocmpb.ListOCMSharesRequest_Filter{
				Type: ocmpb.ListOCMSharesRequest_Filter_TYPE_OWNER,
				Term: &ocmpb.ListOCMSharesRequest_Filter_Owner{
					Owner: v.Owner,
				},
			})
		}
	}
	return ocmfilters
}
