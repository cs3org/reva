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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
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

	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gatewayClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		WriteError(w, r, APIErrorServerError, fmt.Sprintf("error getting storage grpc client on addr: %v", h.gatewayAddr), err)
		log.Err(err).Msg(fmt.Sprintf("error getting storage grpc client on addr: %v", h.gatewayAddr))
		return
	}

	token, err := gatewayClient.GenerateInviteToken(ctx, &invitepb.GenerateInviteTokenRequest{})

	if err != nil {
		WriteError(w, r, APIErrorServerError, "error generating token", err)
		return
	}

	jsonResponse, err := json.Marshal(token.InviteToken)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error marshalling token data", err)
		log.Err(err).Msg("error marshal token data.")
		return
	}

	// Write response
	_, err = w.Write(jsonResponse)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error writing token data", err)
		log.Err(err).Msg("error writing shares data.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func (h *invitesHandler) forwardInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	if r.FormValue("token") == "" || r.FormValue("providerDomain") == "" {
		WriteError(w, r, APIErrorInvalidParameter, "token and providerDomain must not be null", nil)
		return
	}

	gatewayClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		WriteError(w, r, APIErrorServerError, fmt.Sprintf("error getting invite grpc client on addr: %v", h.gatewayAddr), err)
		return
	}

	token := &invitepb.InviteToken{
		Token: r.FormValue("token"),
	}

	providerInfo, err := gatewayClient.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
		Domain: r.FormValue("providerDomain"),
	})
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error sending a grpc get invite by domain info request", err)
		return
	}

	forwardInviteReq := &invitepb.ForwardInviteRequest{
		InviteToken:          token,
		OriginSystemProvider: providerInfo.ProviderInfo,
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

	log.Info().Msg("Invite forwarded.")
}

func (h *invitesHandler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	token, userID, email, recipientProvider := r.FormValue("token"), r.FormValue("userID"), r.FormValue("email"), r.FormValue("recipientProvider")
	if token == "" || userID == "" || recipientProvider == "" || email == "" {
		WriteError(w, r, APIErrorInvalidParameter, "missing parameters in request", nil)
		return
	}

	gatewayClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		WriteError(w, r, APIErrorServerError, fmt.Sprintf("error getting storage grpc client on addr: %v", h.gatewayAddr), err)
		return
	}

	userObj := &userpb.User{
		Id: &userpb.UserId{
			OpaqueId: userID,
			Idp:      recipientProvider,
		},
		Mail: email,
	}
	providerAllowedResp, err := gatewayClient.IsProviderAllowed(ctx, &ocmprovider.IsProviderAllowedRequest{
		User: userObj,
	})
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error authorizing provider", err)
		return
	}
	if providerAllowedResp.Status.Code != rpc.Code_CODE_OK {
		WriteError(w, r, APIErrorUnauthenticated, "provider not authorized", err)
		return
	}

	acceptInviteRequest := &invitepb.AcceptInviteRequest{
		InviteToken: &invitepb.InviteToken{
			Token: token,
		},
		RemoteUser: userObj,
	}
	acceptInviteResponse, err := gatewayClient.AcceptInvite(ctx, acceptInviteRequest)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error sending a grpc accept invite request", err)
		return
	}
	if acceptInviteResponse.Status.Code != rpc.Code_CODE_OK {
		WriteError(w, r, APIErrorServerError, "grpc accept invite request failed", err)
		return
	}

	log.Info().Msg("User added to accepted users.")
}
