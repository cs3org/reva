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
	"slices"
	"time"
)

// NotificationsCapability is the general notification discovery capability.
const NotificationsCapability = "notifications"

// NotificationStatusClass identifies recognized delivery responses.
type NotificationStatusClass int

const (
	NotificationStatusCreated        NotificationStatusClass = 201
	NotificationStatusBadRequest     NotificationStatusClass = 400
	NotificationStatusUnauthorized   NotificationStatusClass = 401
	NotificationStatusForbidden      NotificationStatusClass = 403
	NotificationStatusNotFound       NotificationStatusClass = 404
	NotificationStatusNotImplemented NotificationStatusClass = 501
	NotificationStatusUnavailable    NotificationStatusClass = 503
)

// NotificationPeer is the discovery outcome needed to send a notification.
type NotificationPeer struct {
	// Endpoint is the absolute URL of the peer's notifications endpoint.
	Endpoint     string
	Capabilities []string
}

// HasNotificationsCapability reports whether the peer advertises notifications.
func (p *NotificationPeer) HasNotificationsCapability() bool {
	return p != nil && slices.Contains(p.Capabilities, NotificationsCapability)
}

// NotificationResponse is what the transport keeps of a peer response: the
// status and an already parsed Retry-After delay, never raw peer bytes.
type NotificationResponse struct {
	Status     int
	RetryAfter time.Duration
}

// NotificationTransport uses each discovered peer's capabilities without shared
// mutable state. Errors may contain peer bytes and must only be classified.
type NotificationTransport interface {
	DiscoverNotificationPeer(ctx context.Context, domain string) (*NotificationPeer, error)
	PostNotification(ctx context.Context, peer *NotificationPeer, body []byte) (NotificationResponse, error)
}

// SendOutcome is the terminal state of one notification attempt.
type SendOutcome string

const (
	SendDelivered         SendOutcome = "delivered"
	SendCapabilityMissing SendOutcome = "capability_missing"
	SendDiscoveryFailed   SendOutcome = "discovery_failed"
	SendPostFailed        SendOutcome = "post_failed"
	SendRejected          SendOutcome = "rejected"
)

// SendFailure is the fixed category of a failed discovery or POST.
type SendFailure string

const (
	FailureCanceled SendFailure = "canceled"
	FailureTimeout  SendFailure = "timeout"
	FailureError    SendFailure = "error"
	// FailureNoSigningKey means required signing prevented a notification POST.
	FailureNoSigningKey SendFailure = "no_signing_key"
)

// SendResult describes one attempt. It holds no peer bytes.
type SendResult struct {
	Outcome SendOutcome
	// Status is the HTTP status received, if any.
	Status int
	// Class is 201 for any 2xx or a defined error class; zero otherwise.
	Class NotificationStatusClass
	// RetryAfter is the 503 delay, up to maxRetryAfter, for diagnostics only.
	RetryAfter time.Duration
	Failure    SendFailure
}

const maxRetryAfter = 24 * time.Hour

func classifyFailure(err error) SendFailure {
	var timeout interface{ Timeout() bool }
	switch {
	case errors.Is(err, errNoSigningKey):
		return FailureNoSigningKey
	case errors.Is(err, context.Canceled):
		return FailureCanceled
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &timeout) && timeout.Timeout():
		return FailureTimeout
	}
	return FailureError
}

// ClassifyNotificationResponse maps a response to its outcome. Any 2xx is
// a delivery.
func ClassifyNotificationResponse(r NotificationResponse) SendResult {
	if r.Status >= 200 && r.Status < 300 {
		return SendResult{Outcome: SendDelivered, Status: r.Status, Class: NotificationStatusCreated}
	}
	res := SendResult{Outcome: SendRejected, Status: r.Status}
	switch NotificationStatusClass(r.Status) {
	case NotificationStatusBadRequest, NotificationStatusUnauthorized, NotificationStatusForbidden,
		NotificationStatusNotFound, NotificationStatusNotImplemented:
		res.Class = NotificationStatusClass(r.Status)
	case NotificationStatusUnavailable:
		res.Class = NotificationStatusUnavailable
		res.RetryAfter = min(max(r.RetryAfter, 0), maxRetryAfter)
	}
	return res
}

// SendNotification makes one best-effort attempt for a capable peer.
func SendNotification(ctx context.Context, t NotificationTransport, domain string, body []byte) SendResult {
	peer, err := t.DiscoverNotificationPeer(ctx, domain)
	if err != nil {
		return SendResult{Outcome: SendDiscoveryFailed, Failure: classifyFailure(err)}
	}
	if !peer.HasNotificationsCapability() || peer.Endpoint == "" {
		return SendResult{Outcome: SendCapabilityMissing}
	}
	resp, err := t.PostNotification(ctx, peer, body)
	if err != nil {
		return SendResult{Outcome: SendPostFailed, Failure: classifyFailure(err)}
	}
	return ClassifyNotificationResponse(resp)
}
