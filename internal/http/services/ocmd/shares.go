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

package ocmd

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/pkg/errors"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmcore "github.com/cs3org/go-cs3apis/cs3/ocm/core/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	ocmproviderhttp "github.com/cs3org/reva/internal/http/services/ocmprovider"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/internal/http/services/reqres"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

type sharesHandler struct {
	gatewayClient              gateway.GatewayAPIClient
	exposeRecipientDisplayName bool
}

func (h *sharesHandler) init(c *config) error {
	var err error
	h.gatewayClient, err = pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return err
	}
	h.exposeRecipientDisplayName = c.ExposeRecipientDisplayName
	return nil
}

type createShareRequest struct {
	ShareWith         string    `json:"shareWith" validate:"required"`                  // identifier of the recipient of the share
	Name              string    `json:"name" validate:"required"`                       // name of the resource
	Description       string    `json:"description"`                                    // (optional) description of the resource
	ProviderID        string    `json:"providerId" validate:"required"`                 // unique identifier of the resource at provider side
	Owner             string    `json:"owner" validate:"required"`                      // unique identifier of the owner at provider side
	Sender            string    `json:"sender" validate:"required"`                     // unique indentifier of the user who wants to share the resource at provider side
	OwnerDisplayName  string    `json:"ownerDisplayName"`                               // display name of the owner of the resource
	SenderDisplayName string    `json:"senderDisplayName"`                              // dispay name of the user who wants to share the resource
	ShareType         string    `json:"shareType" validate:"required,oneof=user group"` // recipient share type (user or group)
	ResourceType      string    `json:"resourceType" validate:"required,oneof=file folder"`
	Expiration        uint64    `json:"expiration"`
	Protocols         Protocols `json:"protocol" validate:"required"`
}

// CreateShare sends all the informations to the consumer needed to start
// synchronization between the two services.
func (h *sharesHandler) CreateShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	req, err := getCreateShareRequest(r)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, err.Error(), nil)
		return
	}

	_, meshProvider, err := getIDAndMeshProvider(req.Sender)
	log.Debug().Msgf("Determined Mesh Provider '%s' from req.Sender '%s'", meshProvider, req.Sender)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, err.Error(), nil)
		return
	}

	clientIP, err := utils.GetClientIP(r)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, fmt.Sprintf("error retrieving client IP from request: %s", r.RemoteAddr), err)
		return
	}
	providerInfo := ocmprovider.ProviderInfo{
		Domain: meshProvider,
		Services: []*ocmprovider.Service{
			{
				Host: clientIP,
			},
		},
	}

	providerAllowedResp, err := h.gatewayClient.IsProviderAllowed(ctx, &ocmprovider.IsProviderAllowedRequest{
		Provider: &providerInfo,
	})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending a grpc is provider allowed request", err)
		return
	}
	if providerAllowedResp.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, "provider not authorized", errors.New(providerAllowedResp.Status.Message))
		return
	}

	shareWith, _, err := getIDAndMeshProvider(req.ShareWith)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, err.Error(), nil)
		return
	}

	userRes, err := h.gatewayClient.GetUser(ctx, &userpb.GetUserRequest{
		UserId: &userpb.UserId{OpaqueId: shareWith}, SkipFetchingUserGroups: true,
	})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error searching recipient", err)
		return
	}
	if userRes.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorNotFound, "user not found", errors.New(userRes.Status.Message))
		return
	}

	owner, err := getUserIDFromOCMUser(req.Owner)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, err.Error(), nil)
		return
	}

	sender, err := getUserIDFromOCMUser(req.Sender)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, err.Error(), nil)
		return
	}

	protocols, err := getAndResolveProtocols(req.Protocols, r)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, err.Error(), nil)
		return
	}

	createShareReq := &ocmcore.CreateOCMCoreShareRequest{
		Description:  req.Description,
		Name:         req.Name,
		ResourceId:   req.ProviderID,
		Owner:        owner,
		Sender:       sender,
		ShareWith:    userRes.User.Id,
		ResourceType: getResourceTypeFromOCMRequest(req.ResourceType),
		ShareType:    getOCMShareType(req.ShareType),
		Protocols:    protocols,
	}

	if req.Expiration != 0 {
		createShareReq.Expiration = &types.Timestamp{
			Seconds: req.Expiration,
		}
	}

	createShareResp, err := h.gatewayClient.CreateOCMCoreShare(ctx, createShareReq)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error creating ocm share", err)
		return
	}

	if userRes.Status.Code != rpc.Code_CODE_OK {
		// TODO: define errors in the cs3apis
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error creating ocm share", errors.New(createShareResp.Status.Message))
		return
	}

	response := map[string]any{}

	if h.exposeRecipientDisplayName {
		response["recipientDisplayName"] = userRes.User.DisplayName
	}

	_ = json.NewEncoder(w).Encode(response)
	w.WriteHeader(http.StatusCreated)
}

func getUserIDFromOCMUser(user string) (*userpb.UserId, error) {
	id, idp, err := getIDAndMeshProvider(user)
	if err != nil {
		return nil, err
	}
	return &userpb.UserId{
		OpaqueId: id,
		Idp:      idp,
		// the remote user is a federated account for the local reva
		Type: userpb.UserType_USER_TYPE_FEDERATED,
	}, nil
}

func getIDAndMeshProvider(user string) (string, string, error) {
	// the user is in the form of dimitri@apiwise.nl
	split := strings.Split(user, "@")
	if len(split) < 2 {
		return "", "", errors.New("not in the form <id>@<provider>")
	}
	return strings.Join(split[:len(split)-1], "@"), split[len(split)-1], nil
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
	// validate the request
	if err := validate.Struct(req); err != nil {
		return nil, err
	}
	return &req, nil
}

func getResourceTypeFromOCMRequest(t string) providerpb.ResourceType {
	switch t {
	case "file":
		return providerpb.ResourceType_RESOURCE_TYPE_FILE
	case "folder":
		return providerpb.ResourceType_RESOURCE_TYPE_CONTAINER
	default:
		return providerpb.ResourceType_RESOURCE_TYPE_INVALID
	}
}

func getOCMShareType(t string) ocm.ShareType {
	if t == "user" {
		return ocm.ShareType_SHARE_TYPE_USER
	}
	return ocm.ShareType_SHARE_TYPE_GROUP
}

func getAndResolveProtocols(p Protocols, r *http.Request) ([]*ocm.Protocol, error) {
	protos := make([]*ocm.Protocol, 0, len(p))
	for _, data := range p {
		ocmProto := data.ToOCMProtocol()
		if GetProtocolName(data) == "webdav" && ocmProto.GetWebdavOptions().Uri == "" {
			// This is an OCM 1.0 payload with only webdav: we need to resolve the remote URL
			remoteRoot, err := discoverOcmWebdavRoot(r)
			if err != nil {
				return nil, err
			}
			ocmProto.GetWebdavOptions().Uri = filepath.Join(remoteRoot, ocmProto.GetWebdavOptions().SharedSecret)
		}
		protos = append(protos, ocmProto)
	}
	return protos, nil
}

func discoverOcmWebdavRoot(r *http.Request) (string, error) {
	// implements the OCM discovery logic to fetch the WebDAV root at the remote host that sent the share, see
	// https://cs3org.github.io/OCM-API/docs.html?branch=v1.1.0&repo=OCM-API&user=cs3org#/paths/~1ocm-provider/get
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	log.Debug().Str("sender", r.Host).Msg("received OCM 1.0 share, attempting to discover sender endpoint")

	httpReq, err := rhttp.NewRequest(ctx, http.MethodGet, r.Host+"/ocm-provider", nil)
	if err != nil {
		return "", err
	}
	httpClient := rhttp.GetHTTPClient(
		rhttp.Timeout(time.Duration(10 * int64(time.Second))),
	)
	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		return "", errors.Wrap(err, "failed to contact OCM sender server")
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusOK {
		return "", errtypes.InternalError("Invalid HTTP response on OCM discovery")
	}
	body, err := io.ReadAll(httpRes.Body)
	if err != nil {
		return "", err
	}

	var result ocmproviderhttp.DiscoveryData
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Warn().Str("sender", r.Host).Str("response", string(body)).Msg("malformed response")
		return "", errtypes.InternalError("Invalid payload on OCM discovery")
	}

	for _, t := range result.ResourceTypes {
		webdavRoot, ok := t.Protocols["webdav"]
		if ok {
			// assume the first resourceType that exposes a webdav root is OK to use: as a matter of fact,
			// no implementation exists yet that exposes multiple resource types with different roots.
			return filepath.Join(result.Endpoint, webdavRoot), nil
		}
	}

	log.Warn().Str("sender", r.Host).Str("response", string(body)).Msg("missing webdav root")
	return "", errtypes.NotFound("WebDAV root not found on OCM discovery")
}
