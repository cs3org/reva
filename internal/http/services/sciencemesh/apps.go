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

package sciencemesh

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	pkgerrors "github.com/pkg/errors"
)

type appsHandler struct {
	gatewayClient  gateway.GatewayAPIClient
	ocmMountPoint  string
	ocmClient      *ocmd.OCMClient
	discoveryCache *ttlcache.Cache
}

type openInAppLaunch struct {
	AppURL      string `json:"app_url"`
	AccessToken string `json:"access_token"`
}

func (h *appsHandler) init(c *config) error {
	var err error
	h.gatewayClient, err = pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return err
	}
	h.ocmMountPoint = c.OCMMountPoint
	h.ocmClient = ocmd.NewClient(time.Duration(c.OCMClientTimeout)*time.Second, c.OCMClientInsecure)

	disco := ttlcache.NewCache()
	_ = disco.SetTTL(5 * time.Minute)
	h.discoveryCache = disco

	return nil
}

func (h *appsHandler) shareInfo(p string) (*ocmpb.ShareId, string) {
	p = strings.TrimPrefix(p, h.ocmMountPoint)
	shareID, rel := router.ShiftPath(p)
	if len(rel) > 0 {
		rel = rel[1:]
	}
	return &ocmpb.ShareId{OpaqueId: shareID}, rel
}

func (h *appsHandler) OpenInApp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "parameters could not be parsed", nil)
		return
	}

	path := r.Form.Get("file")
	if path == "" {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "missing file", nil)
		return
	}

	shareID, rel := h.shareInfo(path)

	share, webapp, err := h.receivedShareWebapp(ctx, shareID)
	if err != nil {
		writeAppsError(w, r, err)
		return
	}

	tokenEndpoint, err := h.tokenEndpoint(ctx, share)
	if err != nil {
		writeAppsError(w, r, err)
		return
	}

	accessToken, err := h.exchangeWebappToken(ctx, share, tokenEndpoint, webapp.SharedSecret)
	if err != nil {
		writeAppsError(w, r, err)
		return
	}

	appUrl, err := url.JoinPath(webapp.Uri, rel)
	if err != nil {
		appUrl = webapp.Uri
	}
	writeAppsJSON(w, r, openInAppLaunch{
		AppURL:      appUrl,
		AccessToken: accessToken,
	})
}

func (h *appsHandler) receivedShareWebapp(ctx context.Context, id *ocmpb.ShareId) (*ocmpb.ReceivedShare, *ocmpb.WebappProtocol, error) {
	res, err := h.gatewayClient.GetReceivedOCMShare(ctx, &ocmpb.GetReceivedOCMShareRequest{
		Ref: &ocmpb.ShareReference{
			Spec: &ocmpb.ShareReference_Id{
				Id: id,
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		if res.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND {
			return nil, nil, errtypes.NotFound(res.Status.Message)
		}
		return nil, nil, errtypes.InternalError(res.Status.Message)
	}

	webapp, ok := getWebappProtocol(res.Share.Protocols)
	if !ok {
		return nil, nil, errtypes.BadRequest("share does not contain webapp protocol")
	}
	if webapp.Uri == "" {
		return nil, nil, errtypes.BadRequest("webapp protocol missing uri")
	}
	if webapp.SharedSecret == "" {
		return nil, nil, errtypes.BadRequest("webapp protocol missing shared secret")
	}
	if !slices.Contains(webapp.Requirements, "must-exchange-token") {
		return nil, nil, errtypes.BadRequest("webapp protocol does not require token exchange")
	}

	return res.Share, webapp, nil
}

func (h *appsHandler) tokenEndpoint(ctx context.Context, share *ocmpb.ReceivedShare) (string, error) {
	origin, err := senderOriginFromProtocols(share.Protocols)
	if err != nil {
		return "", err
	}

	if entry, err := h.discoveryCache.Get(origin); err == nil {
		return entry.(string), nil
	}

	disco, err := h.ocmClient.Discover(ctx, origin)
	if err != nil {
		return "", pkgerrors.Wrap(err, "OCM discovery failed for "+origin)
	}
	if disco.TokenEndPoint == "" {
		return "", errtypes.NotFound("sender discovery at " + origin + " has no tokenEndPoint")
	}

	_ = h.discoveryCache.Set(origin, disco.TokenEndPoint)
	return disco.TokenEndPoint, nil
}

func (h *appsHandler) exchangeWebappToken(ctx context.Context, share *ocmpb.ReceivedShare, tokenEndpoint, code string) (string, error) {
	clientID := receiverClientIDWithLookup(ctx, share, h.lookupReceiverUserIDP)
	accessToken, _, err := h.ocmClient.ExchangeToken(ctx, tokenEndpoint, code, clientID)
	if err != nil {
		return "", pkgerrors.Wrap(err, "token exchange failed")
	}
	return accessToken, nil
}

func (h *appsHandler) lookupReceiverUserIDP(ctx context.Context, userID *userpb.UserId) string {
	if h.gatewayClient == nil || userID == nil || userID.GetOpaqueId() == "" {
		return ""
	}

	res, err := h.gatewayClient.GetUser(ctx, &userpb.GetUserRequest{
		UserId:                 userID,
		SkipFetchingUserGroups: true,
	})
	if err != nil || res.GetStatus().GetCode() != rpcv1beta1.Code_CODE_OK || res.GetUser() == nil || res.GetUser().GetId() == nil {
		return ""
	}
	return res.GetUser().GetId().GetIdp()
}

func senderOriginFromProtocols(protocols []*ocmpb.Protocol) (string, error) {
	if dav, ok := getWebDAVProtocol(protocols); ok && dav.Uri != "" {
		return originFromURI(dav.Uri)
	}
	if webapp, ok := getWebappProtocol(protocols); ok && webapp.Uri != "" {
		return originFromURI(webapp.Uri)
	}
	return "", errtypes.NotFound("share has no protocol URI for sender discovery")
}

func originFromURI(rawURI string) (string, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil {
		return "", pkgerrors.Wrap(err, "could not parse sender protocol URI")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errtypes.BadRequest("sender protocol URI is not absolute")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func receiverClientID(ctx context.Context, share *ocmpb.ReceivedShare) string {
	if u, ok := appctx.ContextGetUser(ctx); ok && u.GetId() != nil && u.GetId().GetIdp() != "" {
		return u.GetId().GetIdp()
	}
	if share != nil && share.GetGrantee() != nil && share.GetGrantee().GetUserId() != nil {
		return share.GetGrantee().GetUserId().GetIdp()
	}
	return ""
}

func receiverClientIDWithLookup(ctx context.Context, share *ocmpb.ReceivedShare, lookup func(context.Context, *userpb.UserId) string) string {
	clientID := receiverClientID(ctx, share)
	if clientID == "" && lookup != nil && share != nil && share.GetGrantee() != nil && share.GetGrantee().GetUserId() != nil {
		clientID = lookup(ctx, share.GetGrantee().GetUserId())
	}
	return clientID
}

func getWebappProtocol(protocols []*ocmpb.Protocol) (*ocmpb.WebappProtocol, bool) {
	for _, p := range protocols {
		if t, ok := p.Term.(*ocmpb.Protocol_WebappOptions); ok {
			return t.WebappOptions, true
		}
	}
	return nil, false
}

func getWebDAVProtocol(protocols []*ocmpb.Protocol) (*ocmpb.WebDAVProtocol, bool) {
	for _, p := range protocols {
		if dav, ok := p.Term.(*ocmpb.Protocol_WebdavOptions); ok {
			return dav.WebdavOptions, true
		}
	}
	return nil, false
}

func writeAppsJSON(w http.ResponseWriter, r *http.Request, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error marshalling JSON response", err)
	}
}

func writeAppsError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		notFound           errtypes.NotFound
		badRequest         errtypes.BadRequest
		invalidCredentials errtypes.InvalidCredentials
		permissionDenied   errtypes.PermissionDenied
	)

	switch {
	case errors.As(err, &notFound):
		reqres.WriteError(w, r, reqres.APIErrorNotFound, notFound.Error(), err)
	case errors.As(err, &badRequest):
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, badRequest.Error(), err)
	case errors.As(err, &invalidCredentials):
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, invalidCredentials.Error(), err)
	case errors.As(err, &permissionDenied):
		reqres.WriteError(w, r, reqres.APIErrorUntrustedService, permissionDenied.Error(), err)
	default:
		reqres.WriteError(w, r, reqres.APIErrorServerError, err.Error(), err)
	}
}
