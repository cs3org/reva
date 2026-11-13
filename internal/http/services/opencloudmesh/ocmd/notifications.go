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

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/rs/zerolog"
)

const maxNotificationBody = 1 << 20

const maxLoggedTypeLength = 64

type notifHandler struct {
	registry          *NotificationRegistry
	endpoint          string
	strictHTTPSig     bool
	trustForwardedFor bool
	ocmClient         *OCMClient
}

func (h *notifHandler) init(c *config) error {
	h.registry = NewNotificationRegistry()
	h.endpoint = c.Endpoint
	h.strictHTTPSig = c.HTTPSigMode == "strict"
	h.trustForwardedFor = c.TrustForwardedFor
	for _, t := range []string{
		NotificationShareAccepted,
		NotificationShareDeclined,
		NotificationShareUnshared,
		NotificationShareChangePermission,
		NotificationUserRemoved,
		NotificationGroupRemoved,
	} {
		if err := h.registry.SetHandler(t, TypeHandlerFunc(logNotification)); err != nil {
			return err
		}
	}
	return nil
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
	// Valid unsupported requests must authenticate before returning 501.
	if err := h.authenticate(ctx, w, r, body, n); err != nil {
		return
	}
	if code == NotificationErrNotImplemented {
		notificationLog(log.Info(), n).Str("outcome", "not_implemented").Msg("OCM notification type not implemented")
		reqres.WriteError(w, r, reqres.APIErrorUnimplemented, NotificationReason(err), nil)
		return
	}

	outcome, err := h.registry.Dispatch(ctx, n)
	switch {
	case err != nil:
		notificationLog(log.Warn(), n).Str("outcome", "failed").Msg("OCM notification handler failed")
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error handling OCM notification", nil)
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
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error retrieving client IP from request", err)
		return err
	}
	gatewayClient, err := service.Gateway(ctx)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error getting gateway client", err)
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
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending a grpc isProviderAllowed request", err)
		return err
	}
	if providerAllowedResp == nil || providerAllowedResp.Status == nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "invalid provider policy response", nil)
		return errors.New("missing provider policy status")
	}
	if providerAllowedResp.Status.Code != rpc.Code_CODE_OK {
		notificationLog(log.Info(), n).Str("outcome", "rejected").Msg("notification sender provider not authorized")
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, "provider not authorized", nil)
		return errors.New("rejected")
	}
	disco, err := h.ocmClient.Discover(ctx, n.SenderDomain)
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
	typ := n.Type
	if len(typ) > maxLoggedTypeLength {
		typ = typ[:maxLoggedTypeLength]
	}
	return e.Str("type", typ).
		Str("class", string(n.Class))
}

func logNotification(ctx context.Context, n *Notification) error {
	e := notificationLog(appctx.GetLogger(ctx).Info(), n).
		Str("sender_domain", n.SenderDomain).
		Str("resource_type", n.ResourceType).
		Str("share_type", n.ShareType)
	if n.ProviderID != "" {
		e = e.Str("provider_id", n.ProviderID)
	}
	if n.User != nil {
		e = e.Str("user_id", n.User.UserID)
	}
	if n.Group != nil {
		e = e.Str("group_id", n.Group.GroupID)
	}
	e.Str("outcome", string(DispatchHandled)).Msg("received OCM notification")
	return nil
}
