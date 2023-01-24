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

package sciencemesh

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/internal/http/services/reqres"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/smtpclient"
)

type tokenHandler struct {
	gatewayClient    gateway.GatewayAPIClient
	smtpCredentials  *smtpclient.SMTPCredentials
	meshDirectoryURL string
}

func (h *tokenHandler) init(c *config) error {
	var err error
	h.gatewayClient, err = pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return err
	}

	if c.SMTPCredentials != nil {
		h.smtpCredentials = smtpclient.NewSMTPCredentials(c.SMTPCredentials)
	}

	h.meshDirectoryURL = c.MeshDirectoryURL
	return nil
}

// Generate generates an invitation token and if a recipient is specified,
// will send an email containing the link the user will use to accept the
// invitation.
func (h *tokenHandler) Generate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	token, err := h.gatewayClient.GenerateInviteToken(ctx, &invitepb.GenerateInviteTokenRequest{
		Description: r.URL.Query().Get("description"),
	})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error generating token", err)
		return
	}

	if r.FormValue("recipient") != "" && h.smtpCredentials != nil {
		usr := ctxpkg.ContextMustGetUser(ctx)

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
			reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending token by mail", err)
			return
		}
	}

	if err := json.NewEncoder(w).Encode(token.InviteToken); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error marshalling token data", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

type acceptInviteRequest struct {
	Token          string `json:"token"`
	ProviderDomain string `json:"providerDomain"`
}

// AcceptInvite accepts an invitation from the user in the remote provider.
func (h *tokenHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	req, err := getAcceptInviteRequest(r)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "missing parameters in request", err)
		return
	}

	if req.Token == "" || req.ProviderDomain == "" {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "token and providerDomain must not be null", nil)
		return
	}

	providerInfo, err := h.gatewayClient.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
		Domain: req.ProviderDomain,
	})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending a grpc get invite by domain info request", err)
		return
	}
	if providerInfo.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "grpc forward invite request failed", errors.New(providerInfo.Status.Message))
		return
	}

	forwardInviteReq := &invitepb.ForwardInviteRequest{
		InviteToken: &invitepb.InviteToken{
			Token: req.Token,
		},
		OriginSystemProvider: providerInfo.ProviderInfo,
	}
	forwardInviteResponse, err := h.gatewayClient.ForwardInvite(ctx, forwardInviteReq)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending a grpc forward invite request", err)
		return
	}
	if forwardInviteResponse.Status.Code != rpc.Code_CODE_OK {
		switch forwardInviteResponse.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			reqres.WriteError(w, r, reqres.APIErrorNotFound, "token not found", nil)
			return
		case rpc.Code_CODE_INVALID_ARGUMENT:
			reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "token has expired", nil)
			return
		case rpc.Code_CODE_ALREADY_EXISTS:
			reqres.WriteError(w, r, reqres.APIErrorAlreadyExist, "user already known", nil)
			return
		case rpc.Code_CODE_PERMISSION_DENIED:
			reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, "remove service not trusted", nil)
			return
		default:
			reqres.WriteError(w, r, reqres.APIErrorServerError, "unexpected error: "+forwardInviteResponse.Status.Message, errors.New(forwardInviteResponse.Status.Message))
			return
		}
	}

	w.WriteHeader(http.StatusOK)

	log.Info().Str("token", req.Token).Str("provider", req.ProviderDomain).Msgf("invite forwarded")
}

func getAcceptInviteRequest(r *http.Request) (*acceptInviteRequest, error) {
	var req acceptInviteRequest
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && contentType == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
	} else {
		req.Token, req.ProviderDomain = r.FormValue("token"), r.FormValue("providerDomain")
	}
	return &req, nil
}

// FindAccepted returns the list of all the users that accepted the invitation
// to the authenticated user.
func (h *tokenHandler) FindAccepted(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	res, err := h.gatewayClient.FindAcceptedUsers(ctx, &invitepb.FindAcceptedUsersRequest{})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending a grpc find accepted users request", err)
		return
	}

	if err := json.NewEncoder(w).Encode(res.AcceptedUsers); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error marshalling token data", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
