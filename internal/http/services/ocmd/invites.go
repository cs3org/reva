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

package ocmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	invitepb "github.com/cs3org/go-cs3apis/cs3/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	userPkg "github.com/cs3org/reva/pkg/user"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
)

type invitesHandler struct {
	gatewayAddr string
}

func (h *invitesHandler) init(c *Config) {
	h.gatewayAddr = c.GatewaySvc
}

func (h *invitesHandler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")

		switch head {
		case "":
			h.generateInviteToken(w, r)
		case "forward":
			h.forwardInvite(w, r)
		case "accept":
			h.acceptInvite(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (h *invitesHandler) generateInviteToken(w http.ResponseWriter, r *http.Request) {
}

func (h *invitesHandler) forwardInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type Request struct {
		Token        string
		ProviderInfo string
	}

	if r.Body == nil {
		http.Error(w, "Null body", 400)
		return
	}
	var req Request

	// Try to decode the request body into the struct. If there is an error,
	// respond to the client with the error message and a 400 status code.
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	gatewayClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	// response, err := gatewayClient.GetInfoByDomain(ctx, request)
	if err != nil {
		WriteError(w, r, APIErrorServerError, fmt.Sprintf("error getting invite grpc client on addr: %v", h.gatewayAddr), err)
		return
	}

	expireTime := time.Now()

	contextUser, _ := userPkg.ContextGetUser(ctx)
	token := &invitepb.InviteToken{
		Token:  "blbl",
		UserId: contextUser.GetId(),
		Expiration: &types.Timestamp{
			Nanos:   uint32(expireTime.UnixNano()),
			Seconds: uint64(expireTime.Unix()),
		},
	}

	//TODO Update these values with values from GetInfoByDomain response
	forwardInviteReq := &invitepb.ForwardInviteRequest{
		InviteToken: token,
		OriginSystemProvider: &ocm.ProviderInfo{
			Domain:         "domain",
			ApiVersion:     "ApiVersion",
			ApiEndpoint:    "APIEndPoint",
			WebdavEndpoint: "WebdavEndpoint",
		},
	}

	forwardInviteResponse, err := gatewayClient.ForwardInvite(ctx, forwardInviteReq)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error sending a grpc forward invite request", err)
		return
	}
	if forwardInviteResponse.Status.Code != rpc.Code_CODE_OK {
		if forwardInviteResponse.Status.Code == rpc.Code_CODE_NOT_FOUND {
			WriteError(w, r, APIErrorNotFound, "not found", nil)
			return
		}
		WriteError(w, r, APIErrorServerError, "grpc forward invite request failed", err)
		return
	}
}

func (h *invitesHandler) acceptInvite(w http.ResponseWriter, r *http.Request) {
}
