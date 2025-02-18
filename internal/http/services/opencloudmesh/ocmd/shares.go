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

package ocmd

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/internal/http/services/reqres"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
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

// CreateShare implements the OCM /shares call.
func (h *sharesHandler) CreateShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	req, err := getCreateShareRequest(r)
	log.Info().Any("req", req).Msg("OCM /shares request received")
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
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending a grpc isProviderAllowed request", err)
		return
	}
	if providerAllowedResp.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, "provider not authorized", errors.New(providerAllowedResp.Status.Message))
		return
	}

	shareWith, _, err := getIDAndMeshProvider(req.ShareWith)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "error with mesh provider", err)
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
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "error with remote owner", err)
		return
	}

	sender, err := getUserIDFromOCMUser(req.Sender)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "error with remote sender", err)
		return
	}

	protocols, err := getAndResolveProtocols(req.Protocols, r)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "error with protocols payload", err)
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

	log.Info().Any("req", createShareReq).Msg("CreateOCMCoreShare payload")
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

func getCreateShareRequest(r *http.Request) (*NewShareRequest, error) {
	var req NewShareRequest
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && contentType == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, errors.Wrap(err, "malformed OCM /shares request")
		}
	} else {
		return nil, errors.New("malformed OCM /shares request payload")
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
	switch t {
	case "user":
		return ocm.ShareType_SHARE_TYPE_USER
	case "group":
		return ocm.ShareType_SHARE_TYPE_GROUP
	default:
		// for now assume user share if not provided
		return ocm.ShareType_SHARE_TYPE_USER
	}
}

func getAndResolveProtocols(p Protocols, r *http.Request) ([]*ocm.Protocol, error) {
	protos := make([]*ocm.Protocol, 0, len(p))
	for _, data := range p {
		var uri string
		ocmProto := data.ToOCMProtocol()
		protocolName := GetProtocolName(data)
		switch protocolName {
		case "webdav":
			uri = ocmProto.GetWebdavOptions().Uri
			reqs := ocmProto.GetWebdavOptions().Requirements
			if len(reqs) > 0 {
				// we currently do not support any kind of requirement
				return nil, errtypes.BadRequest(fmt.Sprintf("incoming OCM share with requirements %+v not supported at this endpoint", reqs))
			}
		case "webapp":
			uri = ocmProto.GetWebappOptions().Uri
		}

		// Irrespective from the presence of a full `uri` in the payload (deprecated), validate the
		// remote is an OCM server and resolve the remote root
		// yet skip this if the remote is localhost (for integration tests)
		if strings.Contains(uri, "localhost") {
			protos = append(protos, ocmProto)
			continue
		}
		remoteRoot, err := discoverOcmRoot(r, protocolName)
		if err != nil {
			return nil, err
		}

		uri, _ = url.JoinPath(remoteRoot, uri[strings.LastIndex(uri, "/")+1:])
		switch protocolName {
		case "webdav":
			ocmProto.GetWebdavOptions().Uri = uri
		case "webapp":
			ocmProto.GetWebappOptions().Uri = uri
		}
		protos = append(protos, ocmProto)
	}

	return protos, nil
}

func discoverOcmRoot(r *http.Request, proto string) (string, error) {
	// implements the OCM discovery logic to fetch the root at the remote host that sent the share for the given proto, see
	// https://cs3org.github.io/OCM-API/docs.html?branch=v1.1.0&repo=OCM-API&user=cs3org#/paths/~1ocm-provider/get
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	// assume the sender host is either given in the usual reverse proxy headers or as RemoteAddr, and that the
	// remote end listens on https regardless if the incoming connection got its TLS terminated upstream of us
	senderURL := r.Header.Get("X-Real-Ip")
	if senderURL == "" {
		senderURL = r.Header.Get("X-Forwarded-For")
	}
	if senderURL == "" {
		senderURL = r.RemoteAddr
	}
	senderURL = "https://" + senderURL[:strings.LastIndex(senderURL, ":")]
	log.Debug().Str("sender", senderURL).Msg("received OCM share, attempting to discover sender endpoint")

	ocmClient := NewClient(time.Duration(10)*time.Second, true)
	ocmCaps, err := ocmClient.Discover(ctx, senderURL)
	if err != nil {
		log.Warn().Str("sender", senderURL).Err(err).Msg("failed to discover OCM sender")
		return "", err
	}
	for _, t := range ocmCaps.ResourceTypes {
		protoRoot, ok := t.Protocols[proto]
		if ok {
			// assume the first resourceType that exposes a root is OK to use: as a matter of fact,
			// no implementation exists yet that exposes multiple resource types with different roots.
			u, _ := url.Parse(ocmCaps.Endpoint)
			u.Path = protoRoot
			u.RawQuery = ""
			return u.String(), nil
		}
	}

	log.Warn().Str("sender", r.Host).Interface("response", ocmCaps).Msg("missing root")
	return "", errtypes.NotFound(fmt.Sprintf("root not found on OCM discovery for protocol %s", proto))
}
