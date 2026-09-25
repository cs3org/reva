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
	"errors"
	"net/http"
	"strings"
	"time"

	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
)

// launchClient is the remote discovery and token-exchange surface. Production
// uses one ocmd.NewPublicOnlyClient for both hops. Tests may install a factory.
type launchClient interface {
	Discover(ctx context.Context, endpoint string) (*wellknown.OcmDiscoveryData, error)
	ExchangeToken(ctx context.Context, tokenEndpoint, code, clientID string) (string, int64, error)
}

var _ launchClient = (*ocmd.OCMClient)(nil)

type appsHandler struct {
	ocmMountPoint   string
	receiverDomain  string
	clientTimeout   time.Duration
	clientInsecure  bool
	newLaunchClient func(timeout time.Duration, insecure bool) launchClient
	// webappReceiveTargets overrides published local targets when non-nil.
	// Nil uses the published set. An explicit empty set disables receipt.
	webappReceiveTargets *[]string
}

type openInAppResponse struct {
	AppURL      string `json:"app_url"`
	AccessToken string `json:"access_token"`
}

func (h *appsHandler) init(c *config) error {
	if err := providerdomain.Validate(c.ProviderDomain); err != nil {
		return err
	}
	h.ocmMountPoint = c.OCMMountPoint
	h.receiverDomain = c.ProviderDomain
	h.clientTimeout = time.Duration(c.OCMClientTimeout) * time.Second
	h.clientInsecure = c.OCMClientInsecure
	return nil
}

func (h *appsHandler) remoteClient() launchClient {
	if h.newLaunchClient != nil {
		return h.newLaunchClient(h.clientTimeout, h.clientInsecure)
	}
	return ocmd.NewPublicOnlyClient(h.clientTimeout, h.clientInsecure)
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

	filePath := r.Form.Get("file")
	if filePath == "" {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "missing file", nil)
		return
	}
	if err := validateShareFilePath(filePath); err != nil {
		writeLaunchError(w, r, err)
		return
	}

	shareID, rel := h.shareInfo(filePath)
	payload, err := h.buildLaunch(ctx, shareID, rel)
	if err != nil {
		writeLaunchError(w, r, err)
		return
	}
	writeLaunchJSON(w, r, payload)
}

func (h *appsHandler) buildLaunch(ctx context.Context, shareID *ocmpb.ShareId, rel string) (openInAppResponse, error) {
	var none openInAppResponse
	if err := providerdomain.Validate(h.receiverDomain); err != nil {
		return none, redactLaunchError(errtypes.BadRequest(err.Error()), "", "")
	}
	share, webapp, err := h.receivedWebapp(ctx, shareID)
	if err != nil {
		return none, redactLaunchError(err, "", "")
	}
	secret := webapp.GetSharedSecret()
	fail := func(err error, token string) (openInAppResponse, error) {
		return none, redactLaunchError(err, secret, token)
	}

	receiverTargets := wellknown.ResolveLocalWebappReceiveTargets(h.webappReceiveTargets)
	if err := ocmd.ValidateWebappLaunch(
		webapp.GetUri(),
		secret,
		webapp.GetRequirements(),
		webapp.GetTargets(),
		receiverTargets,
	); err != nil {
		if errors.Is(err, ocmd.ErrWebappMFAUnproven) {
			return fail(errtypes.PermissionDenied(ocmd.ErrWebappMFAUnproven.Error()), "")
		}
		if errors.Is(err, ocmd.ErrInvalidProtocolURI) {
			return fail(errtypes.BadRequest("malformed remote URL"), "")
		}
		return fail(errtypes.BadRequest(err.Error()), "")
	}

	appURI, err := requireHTTPSAppURI(webapp.GetUri())
	if err != nil {
		return fail(err, "")
	}
	if strings.TrimSpace(h.receiverDomain) == "" {
		return fail(errtypes.BadRequest("provider domain is not configured"), "")
	}

	origin, err := senderDiscoveryOrigin(share.GetProtocols())
	if err != nil {
		return fail(err, "")
	}

	client := h.remoteClient()
	if client == nil {
		return fail(errtypes.InternalError("launch client is not available"), "")
	}
	disco, err := client.Discover(ctx, origin)
	if err != nil {
		return fail(err, "")
	}
	tokenURL, err := resolveTokenEndpoint(disco)
	if err != nil {
		return fail(err, "")
	}
	accessToken, _, err := client.ExchangeToken(
		ctx,
		tokenURL,
		secret,
		h.receiverDomain,
	)
	if err != nil {
		return fail(err, accessToken)
	}
	if strings.TrimSpace(accessToken) == "" {
		return fail(errtypes.InternalError("token exchange returned an empty access token"), "")
	}

	launched, err := joinShareRelativePath(appURI, rel)
	if err != nil {
		return fail(err, accessToken)
	}
	return openInAppResponse{
		AppURL:      launched,
		AccessToken: accessToken,
	}, nil
}
