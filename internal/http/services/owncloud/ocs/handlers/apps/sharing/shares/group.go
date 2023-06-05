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
	"strconv"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

func (h *Handler) createGroupShare(w http.ResponseWriter, r *http.Request, statInfo *provider.ResourceInfo, role *conversions.Role, roleVal []byte) {
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

	groupRes, err := c.GetGroupByClaim(ctx, &grouppb.GetGroupByClaimRequest{
		Claim:               "group_name",
		Value:               shareWith,
		SkipFetchingMembers: true,
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error searching recipient", err)
		return
	}
	if groupRes.Status.Code != rpc.Code_CODE_OK {
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "group not found", err)
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
				Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
				Id:   &provider.Grantee_GroupId{GroupId: groupRes.Group.GetId()},
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: role.CS3ResourcePermissions(),
			},
		},
	}

	if shareID, ok := h.createCs3Share(ctx, w, r, c, createShareReq, statInfo); ok {
		notify, _ := strconv.ParseBool(r.FormValue("notify"))
		if notify {
			granter, ok := ctxpkg.ContextGetUser(ctx)
			if ok {
				h.SendShareNotification(shareID.OpaqueId, granter, groupRes.Group, statInfo)
			}
		}
	}
}
