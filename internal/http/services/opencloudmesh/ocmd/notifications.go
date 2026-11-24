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
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/rs/zerolog"
)

const maxNotificationBody = 1 << 20

type notifHandler struct {
	registry          *NotificationRegistry
	endpoint          string
	strictHTTPSig     bool
	trustForwardedFor bool
	ocmClient         *OCMClient
}

func (h *notifHandler) init(c *config) {
	h.registry = NewNotificationRegistry()
	h.endpoint = c.Endpoint
	h.strictHTTPSig = c.HTTPSigMode == "strict"
	h.trustForwardedFor = c.TrustForwardedFor
}

// Notifications receives OCM notifications and returns an empty 201 on success.
func (h *notifHandler) Notifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	body, err := readNotificationBody(w, r)
	if err != nil {
		log.Info().Str("class", string(NotificationClassMalformed)).Str("outcome", "rejected").Msg("invalid OCM notification")
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, err.Error(), nil)
		return
	}

	n, err := h.registry.Parse(body)
	code, _ := NotificationErrorCodeOf(err)
	if err != nil && code != NotificationErrNotImplemented {
		reason := NotificationReason(err)
		notificationLog(log.Info(), n).Str("outcome", "rejected").Str("reason", reason).Msg("invalid OCM notification")
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, reason, nil)
		return
	}
	// A 501 response is also an authenticated receipt.
	if err := h.authenticate(ctx, w, r, body, n); err != nil {
		return
	}
	outcome, err := h.registry.Dispatch(ctx, n)
	switch {
	case err != nil:
		notificationLog(log.Warn(), n).Str("outcome", "failed").Msg("OCM notification handler failed")
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error handling OCM notification", nil)
		return
	case outcome == DispatchNotImplemented || (outcome == DispatchIgnored && n.Class == NotificationClassSpec):
		notificationLog(log.Info(), n).Str("outcome", string(DispatchNotImplemented)).Msg("OCM notification type not implemented")
		reqres.WriteError(w, r, reqres.APIErrorUnimplemented, "notification type not implemented", nil)
		return
	case outcome == DispatchIgnored:
		notificationLog(log.Info(), n).Str("outcome", string(outcome)).Msg("OCM notification ignored")
	}
	w.WriteHeader(http.StatusCreated)
}

// Verify the original body bytes; re-encoding would change the signed content.
func (h *notifHandler) authenticate(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte, n *Notification) error {
	log := appctx.GetLogger(ctx)
	if n == nil || n.SenderDomain == "" {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "notification senderDomain is required", nil)
		return errors.New("rejected")
	}
	senderIP, err := utils.GetClientIP(r, h.trustForwardedFor)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error retrieving client IP from request", nil)
		return err
	}
	gatewayClient, err := service.Gateway(ctx)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error getting gateway client", nil)
		return err
	}
	providerAllowedResp, err := gatewayClient.IsProviderAllowed(ctx, &ocmprovider.IsProviderAllowedRequest{
		Provider: &ocmprovider.ProviderInfo{
			Domain: n.SenderDomain,
			Services: []*ocmprovider.Service{
				{Host: senderIP},
			},
		},
	})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending a grpc isProviderAllowed request", nil)
		return err
	}
	if providerAllowedResp == nil || providerAllowedResp.Status == nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "invalid provider policy response", nil)
		return errors.New("missing provider policy status")
	}
	// The provider authorizers report an unknown provider or a wrong sender IP
	// with these codes. Other codes are a policy backend failure.
	switch providerAllowedResp.Status.Code {
	case rpc.Code_CODE_OK:
	case rpc.Code_CODE_NOT_FOUND, rpc.Code_CODE_PERMISSION_DENIED, rpc.Code_CODE_INVALID_ARGUMENT:
		notificationLog(log.Info(), n).Str("outcome", "rejected").Msg("notification sender provider not authorized")
		reqres.WriteError(w, r, reqres.APIErrorUntrustedService, "provider not authorized", nil)
		return errors.New("rejected")
	default:
		notificationLog(log.Warn(), n).Str("outcome", "failed").Str("policy_status", providerAllowedResp.Status.Code.String()).Msg("notification sender provider policy unavailable")
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error checking provider policy", nil)
		return errors.New("provider policy failed")
	}
	base, ok := notificationDiscoveryBase(n.SenderDomain)
	if !ok {
		notificationLog(log.Info(), n).Str("outcome", "rejected").Msg("notification sender host cannot be discovered")
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error discovering the sending OCM server", nil)
		return errors.New("unsupported sender host")
	}
	// Discovery may log peer-supplied response data; this route reports only
	// fixed categories for untrusted notification senders.
	discard := zerolog.Nop()
	disco, err := h.ocmClient.Discover(appctx.WithLogger(ctx, &discard), base)
	if err != nil {
		notificationLog(log.Info(), n).Str("outcome", "rejected").Msg("notification sender discovery failed")
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error discovering the sending OCM server", nil)
		return err
	}
	if err := verifySignature(ctx, h.ocmClient, r, body, h.endpoint, h.strictHTTPSig, disco); err != nil {
		notificationLog(log.Info(), n).Str("outcome", "rejected").Msg("notification signature verification failed")
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, "invalid HTTP signature", nil)
		return err
	}
	return nil
}

// Keep the exact identity: dropping an empty port or decoding escapes
// could reach a different peer.
func notificationDiscoveryBase(host string) (string, bool) {
	u, err := url.Parse("https://" + host)
	if err != nil || u.Host != host || strings.ContainsAny(u.Hostname(), "%[") || strings.HasSuffix(host, ":") {
		return "", false
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return "", false
		}
	}
	if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" {
		u.Scheme = "http"
	}
	return u.String(), true
}

func readNotificationBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return nil, errors.New("notification content type must be application/json")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxNotificationBody))
	if err != nil {
		return nil, &NotificationError{Code: NotificationErrMalformed}
	}
	return body, nil
}

func notificationLog(e *zerolog.Event, n *Notification) *zerolog.Event {
	if n == nil {
		return e
	}
	return e.Str("class", string(n.Class))
}
