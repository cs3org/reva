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

// maxNotificationBody bounds a received notification body. Larger bodies
// are rejected as malformed before decoding.
const maxNotificationBody = 1 << 20

// maxLoggedTypeLength bounds the received notificationType in logs.
const maxLoggedTypeLength = 64

type notifHandler struct {
	registry *NotificationRegistry
	endpoint string
	// strictHTTPSig mirrors the shares handler: a peer that did not sign is
	// only accepted in permissive mode and when it does not advertise http-sig.
	strictHTTPSig     bool
	trustForwardedFor bool
	ocmClient         *OCMClient
}

func (h *notifHandler) init(c *config) error {
	h.registry = NewNotificationRegistry()
	h.endpoint = c.Endpoint
	h.strictHTTPSig = c.HTTPSigMode == "strict"
	h.trustForwardedFor = c.TrustForwardedFor
	// No effect is wired at this layer: valid notifications are only logged.
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

// Notifications receives notifications from remote OCM servers, see
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1notifications/post
// A handler outcome never changes the bodyless 201 of a valid notification.
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
	switch ReceiverStatus(err) {
	case NotificationStatusNotImplemented:
		// A valid request of an unimplemented type still authenticates first:
		// its refusal is deferred until policy and signature checks have run.
	case NotificationStatusBadRequest:
		reason := NotificationReason(err)
		notificationLog(log.Info(), n).Str("outcome", "rejected").Str("reason", reason).Msg("invalid OCM notification")
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, reason, nil)
		return
	}
	if err := h.authenticate(ctx, w, r, body, n); err != nil {
		return
	}
	if ReceiverStatus(err) == NotificationStatusNotImplemented {
		notificationLog(log.Info(), n).Str("outcome", "not_implemented").Msg("OCM notification type not implemented")
		reqres.WriteError(w, r, reqres.APIErrorUnimplemented, NotificationReason(err), nil)
		return
	}

	outcome, err := h.registry.Dispatch(ctx, n)
	switch {
	case err != nil:
		notificationLog(log.Warn(), n).Str("outcome", string(outcome)).Str("error", err.Error()).Msg("OCM notification handler failed")
	case outcome == DispatchIgnored:
		notificationLog(log.Info(), n).Str("outcome", string(outcome)).Msg("OCM notification of unknown type ignored")
	}
	w.WriteHeader(http.StatusCreated)
}

// authenticate applies the provider policy of the sender domain, discovers
// that canonical origin through the guarded public-only client and verifies
// the request signature before any effect or accepted response. Fixed
// messages only: a remote keyid, algorithm or transport error is not trusted
// input and never reaches the response or the log.
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

// readNotificationBody accepts JSON bodies up to maxNotificationBody.
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

// notificationLog adds fixed metadata fields, never the payload or the shared secret.
func notificationLog(e *zerolog.Event, n *Notification) *zerolog.Event {
	if n == nil {
		return e
	}
	typ := n.Type
	if len(typ) > maxLoggedTypeLength {
		typ = typ[:maxLoggedTypeLength]
	}
	return e.Str("type", typ).
		Str("class", string(n.Class)).
		Bool("has_shared_secret", n.SharedSecret != "")
}

// logNotification records the identity fields of a valid notification and has no other effect.
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
