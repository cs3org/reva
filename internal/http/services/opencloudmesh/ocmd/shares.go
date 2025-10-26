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
	"context"
	"encoding/base64"
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
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/go-playground/validator/v10"
	"github.com/studio-b12/gowebdav"
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
	log.Info().Any("req", req).Str("Remote", r.RemoteAddr).Err(err).Msg("OCM /shares request received")
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

	protocols, legacy, err := getAndResolveProtocols(ctx, req.Protocols, owner.Idp)
	if err != nil || len(protocols) == 0 {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "error with protocols payload", err)
		return
	}

	if legacy && req.ResourceType == "file" {
		// in case of legacy OCM v1.0 shares, we have to PROPFIND the remote resource to check the type,
		// because remote systems such as Nextcloud may send "file" even if the resource is a folder.
		c := gowebdav.NewClient(protocols[0].GetWebdavOptions().Uri, "", "")
		c.SetHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(protocols[0].GetWebdavOptions().SharedSecret+":")))
		target, err := c.Stat("")
		if err != nil {
			log.Info().Err(err).Str("endpoint", protocols[0].GetWebdavOptions().Uri).Msg("error stating remote resource, assuming file")
		} else if target.IsDir() {
			req.ResourceType = "folder"
		}
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
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

func getUserIDFromOCMUser(user string) (*userpb.UserId, error) {
	id, idp, err := getIDAndMeshProvider(user)
	if err != nil {
		return nil, err
	}
	idp = strings.TrimPrefix(idp, "https://") // strip off leading scheme if present (despite being not OCM compliant). This is the case in Nextcloud and oCIS
	return &userpb.UserId{
		OpaqueId: id,
		Idp:      idp,
		// the remote user is a federated account for the local reva
		Type: userpb.UserType_USER_TYPE_FEDERATED,
	}, nil
}

func getIDAndMeshProvider(user string) (string, string, error) {
	last := strings.LastIndex(user, "@")
	if last == -1 {
		return "", "", fmt.Errorf("%s not in the form <id>@<provider>", user)
	}

	id, provider := user[:last], user[last+1:]

	if id == "" {
		return "", "", errors.New("id cannot be empty")
	}
	if provider == "" {
		return "", "", errors.New("provider cannot be empty")
	}

	return id, provider, nil
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

func getAndResolveProtocols(ctx context.Context, p Protocols, ownerServer string) (protos []*ocm.Protocol, legacy bool, err error) {
	protos = make([]*ocm.Protocol, 0, len(p))
	legacy = false
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
				return nil, false, errtypes.BadRequest(fmt.Sprintf("incoming OCM share with requirements %+v not supported at this endpoint", reqs))
			}
		case "webapp":
			uri = ocmProto.GetWebappOptions().Uri
		}

		// If the `uri` contains a hostname, use it as is
		u, _ := url.Parse(uri)
		if u.Host != "" {
			protos = append(protos, ocmProto)
			continue
		}
		// otherwise use as endpoint the owner's server from the payload
		remoteRoot, err := discoverOcmRoot(ctx, ownerServer, protocolName)
		if err != nil {
			return nil, false, err
		}
		if strings.HasPrefix(uri, "/") {
			// only take the host from remoteRoot and append the absolute uri
			u, _ := url.Parse(remoteRoot)
			u.Path = uri
			uri = u.String()
		} else if uri == "" {
			// case of an OCM v1.0 share with no uri, use root
			uri = remoteRoot
			legacy = true
		} else {
			// relative uri
			uri, _ = url.JoinPath(remoteRoot, uri)
		}

		switch protocolName {
		case "webdav":
			ocmProto.GetWebdavOptions().Uri = uri
		case "webapp":
			ocmProto.GetWebappOptions().Uri = uri
		}
		protos = append(protos, ocmProto)
	}

	return protos, legacy, nil
}

func discoverOcmRoot(ctx context.Context, ownerServer string, proto string) (string, error) {
	// implements the OCM discovery logic to fetch the root at the remote host that sent the share for the given proto, see
	// https://cs3org.github.io/OCM-API/docs.html?branch=v1.1.0&repo=OCM-API&user=cs3org#/paths/~1ocm-provider/get
	log := appctx.GetLogger(ctx)

	ocmClient := NewClient(time.Duration(10)*time.Second, true)
	ocmCaps, err := ocmClient.Discover(ctx, "https://"+ownerServer)
	if err != nil {
		log.Warn().Str("sender", ownerServer).Err(err).Msg("failed to discover OCM sender")
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
			log.Debug().Str("sender", ownerServer).Str("proto", proto).Str("URL", u.String()).Msg("resolved protocol URL")
			return u.String(), nil
		}
	}

	log.Warn().Str("sender", ownerServer).Interface("response", ocmCaps).Msg("missing protocol root")
	return "", errtypes.NotFound(fmt.Sprintf("root not found on OCM discovery for protocol %s", proto))
}
