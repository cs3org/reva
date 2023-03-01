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
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/reqres"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/go-playground/validator/v10"
	"google.golang.org/grpc/metadata"
)

var validate = validator.New()

type sharesHandler struct {
	gatewayClient gateway.GatewayAPIClient
}

func (h *sharesHandler) init(c *config) error {
	var err error
	h.gatewayClient, err = pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	return err
}

type createShareRequest struct {
	SourcePath        string `json:"sourcePath" validate:"required"`
	TargetPath        string `json:"targetPath" validate:"required"`
	Type              string `json:"type"`
	Role              string `json:"role" validate:"oneof=viewer editor"`
	RecipientUsername string `json:"recipientUsername" validate:"required"`
	RecipientHost     string `json:"recipientHost" validate:"required"`
	// FIXME: the client should not authenticate here
	LoginType     string `json:"loginType" validate:"required"`
	LoginUsername string `json:"loginUsername" validate:"required"`
	LoginPassword string `json:"loginPassword" validate:"required"`
}

// CreateShare creates an OCM share.
func (h *sharesHandler) CreateShare(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	req, err := getCreateShareRequest(r)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "invalid parameters", err)
		return
	}

	// FIXME: the client should be already authenticated
	res, err := h.gatewayClient.Authenticate(context.Background(), &gateway.AuthenticateRequest{
		Type:         req.LoginType,
		ClientId:     req.LoginUsername,
		ClientSecret: req.LoginPassword,
	})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "unexpected error", err)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, res.Status.Message, errors.New(res.Status.Message))
		return
	}

	ctx := r.Context()
	ctx = ctxpkg.ContextSetToken(ctx, res.Token)
	ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, res.Token)

	statRes, err := h.gatewayClient.Stat(ctx, &providerv1beta1.StatRequest{
		Ref: &providerv1beta1.Reference{
			Path: req.SourcePath,
		},
	})
	switch {
	case err != nil:
		reqres.WriteError(w, r, reqres.APIErrorServerError, "unexpected error", err)
		return
	case statRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		reqres.WriteError(w, r, reqres.APIErrorNotFound, statRes.Status.Message, nil)
		return
	case statRes.Status.Code != rpc.Code_CODE_OK:
		reqres.WriteError(w, r, reqres.APIErrorServerError, statRes.Status.Message, errors.New(statRes.Status.Message))
		return
	}

	recipientProviderInfo, err := h.gatewayClient.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
		Domain: req.RecipientHost,
	})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending a grpc get invite by domain info request", err)
		return
	}
	if recipientProviderInfo.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorNotFound, recipientProviderInfo.Status.Message, errors.New(recipientProviderInfo.Status.Message))
		return
	}

	perm, viewMode := getPermissionsByRole(req.Role)

	shareRes, err := h.gatewayClient.CreateOCMShare(ctx, &ocmv1beta1.CreateOCMShareRequest{
		ResourceId: statRes.Info.Id,
		Grantee: &providerv1beta1.Grantee{
			Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER,
			Id: &providerv1beta1.Grantee_UserId{
				UserId: &userv1beta1.UserId{
					Idp:      req.RecipientHost,
					OpaqueId: req.RecipientUsername,
				},
			},
		},
		RecipientMeshProvider: recipientProviderInfo.ProviderInfo,
		AccessMethods: []*ocmv1beta1.AccessMethod{
			share.NewWebDavAccessMethod(perm),
			share.NewWebappAccessMethod(viewMode),
		},
	})
	switch {
	case err != nil:
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending a grpc CreateOCMShare", err)
		return
	case shareRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		reqres.WriteError(w, r, reqres.APIErrorNotFound, shareRes.Status.Message, nil)
		return
	case shareRes.Status.Code == rpc.Code_CODE_ALREADY_EXISTS:
		reqres.WriteError(w, r, reqres.APIErrorAlreadyExist, shareRes.Status.Message, nil)
		return
	case shareRes.Status.Code != rpc.Code_CODE_OK:
		reqres.WriteError(w, r, reqres.APIErrorAlreadyExist, shareRes.Status.Message, errors.New(shareRes.Status.Message))
		return
	}

	if err := json.NewEncoder(w).Encode(shareRes); err != nil {
		log.Error().Err(err).Msg("error encoding response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getPermissionsByRole(role string) (*providerv1beta1.ResourcePermissions, appprovider.ViewMode) {
	switch role {
	case "viewer":
		return conversions.NewViewerRole().CS3ResourcePermissions(), appprovider.ViewMode_VIEW_MODE_READ_ONLY
	case "editor":
		return conversions.NewEditorRole().CS3ResourcePermissions(), appprovider.ViewMode_VIEW_MODE_READ_WRITE
	}
	return nil, 0
}

func getCreateShareRequest(r *http.Request) (*createShareRequest, error) {
	var req createShareRequest
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && contentType == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("body request not recognised")
	}
	// set defaults
	if req.Type == "" {
		req.Type = "viewer"
	}
	// validate the request
	if err := validate.Struct(req); err != nil {
		return nil, err
	}
	return &req, nil
}
