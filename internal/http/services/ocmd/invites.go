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

package ocmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/smtpclient"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils"
)

type invitesHandler struct {
	smtpCredentials  *smtpclient.SMTPCredentials
	gatewayAddr      string
	meshDirectoryURL string
}

func (h *invitesHandler) init(c *Config) {
	h.gatewayAddr = c.GatewaySvc
	if c.SMTPCredentials != nil {
		h.smtpCredentials = smtpclient.NewSMTPCredentials(c.SMTPCredentials)
	}
	h.meshDirectoryURL = c.MeshDirectoryURL
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

	gatewayClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error getting gateway grpc client", err)
		return
	}

	token, err := gatewayClient.GenerateInviteToken(ctx, &invitepb.GenerateInviteTokenRequest{})
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error generating token", err)
		return
	}

	if r.FormValue("recipient") != "" && h.smtpCredentials != nil {

		usr := user.ContextMustGetUser(ctx)

		// TODO: the message body needs to point to the meshdirectory service
		subject := fmt.Sprintf("ScienceMesh: %s wants to collaborate with you", usr.DisplayName)
		body := "Hi,\n\n" +
			usr.DisplayName + " (" + usr.Mail + ") wants to start sharing OCM resources with you. " +
			"To accept the invite, please visit the following URL:\n" +
			h.meshDirectoryURL + "?token=" + token.InviteToken.Token + "&providerDomain=" + usr.Id.Idp + "\n\n" +
			"Alternatively, you can visit your mesh provider and use the following details:\n" +
			"Token: " + token.InviteToken.Token + "\n" +
			"ProviderDomain: " + usr.Id.Idp + "\n\n" +
			"Best,\nThe ScienceMesh team"

		err = h.smtpCredentials.SendMail(r.FormValue("recipient"), subject, body)
		if err != nil {
			WriteError(w, r, APIErrorServerError, "error sending token by mail", err)
			return
		}
	}

	jsonResponse, err := json.Marshal(token.InviteToken)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error marshalling token data", err)
		return
	}

	// Write response
	_, err = w.Write(jsonResponse)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error writing token data", err)
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
		WriteError(w, r, APIErrorServerError, "error getting gateway grpc client", err)
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
	if providerInfo.Status.Code != rpc.Code_CODE_OK {
		WriteError(w, r, APIErrorServerError, "grpc forward invite request failed", errors.New(providerInfo.Status.Message))
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
		WriteError(w, r, APIErrorServerError, "grpc forward invite request failed", errors.New(forwardInviteResponse.Status.Message))
		return
	}

	_, err = w.Write([]byte("Accepted invite from: " + r.FormValue("providerDomain")))
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error writing token data", err)
		return
	}
	w.WriteHeader(http.StatusOK)

	log.Info().Msgf("Invite forwarded to: %s", r.FormValue("providerDomain"))
}

func (h *invitesHandler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	token, userID, recipientProvider := r.FormValue("token"), r.FormValue("userID"), r.FormValue("recipientProvider")
	name, email := r.FormValue("name"), r.FormValue("email")
	if token == "" || userID == "" || recipientProvider == "" || email == "" {
		WriteError(w, r, APIErrorInvalidParameter, "missing parameters in request", nil)
		return
	}

	gatewayClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error getting gateway grpc client", err)
		return
	}

	clientIP, err := utils.GetClientIP(r)
	if err != nil {
		WriteError(w, r, APIErrorServerError, fmt.Sprintf("error retrieving client IP from request: %s", r.RemoteAddr), err)
		return
	}
	providerInfo := ocmprovider.ProviderInfo{
		Domain: recipientProvider,
		Services: []*ocmprovider.Service{
			&ocmprovider.Service{
				Host: clientIP,
			},
		},
	}

	providerAllowedResp, err := gatewayClient.IsProviderAllowed(ctx, &ocmprovider.IsProviderAllowedRequest{
		Provider: &providerInfo,
	})
	if err != nil {
		WriteError(w, r, APIErrorServerError, "error sending a grpc is provider allowed request", err)
		return
	}
	if providerAllowedResp.Status.Code != rpc.Code_CODE_OK {
		WriteError(w, r, APIErrorUnauthenticated, "provider not authorized", errors.New(providerAllowedResp.Status.Message))
		return
	}

	userObj := &userpb.User{
		Id: &userpb.UserId{
			OpaqueId: userID,
			Idp:      recipientProvider,
		},
		Mail:        email,
		DisplayName: name,
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
		WriteError(w, r, APIErrorServerError, "grpc accept invite request failed", errors.New(acceptInviteResponse.Status.Message))
		return
	}

	log.Info().Msgf("User: %+v added to accepted users.", userObj)
}
