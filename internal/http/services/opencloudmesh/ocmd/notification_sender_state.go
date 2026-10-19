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

// A notification is attempted at most once: there is no retry or queue.

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"time"
)

// NotificationsCapability is the discovery capability of a receiver that
// handles notifications.
const NotificationsCapability = "notifications"

// NotificationStatusClass is the closed set of notification response
// classes. Only senders use 503: the receiver never answers with it.
type NotificationStatusClass int

const (
	NotificationStatusCreated        NotificationStatusClass = http.StatusCreated
	NotificationStatusBadRequest     NotificationStatusClass = http.StatusBadRequest
	NotificationStatusUnauthorized   NotificationStatusClass = http.StatusUnauthorized
	NotificationStatusForbidden      NotificationStatusClass = http.StatusForbidden
	NotificationStatusNotFound       NotificationStatusClass = http.StatusNotFound
	NotificationStatusNotImplemented NotificationStatusClass = http.StatusNotImplemented
	NotificationStatusUnavailable    NotificationStatusClass = http.StatusServiceUnavailable
)

// ReceiverStatus returns the response class of a parsed notification.
func ReceiverStatus(err error) NotificationStatusClass {
	if err == nil {
		return NotificationStatusCreated
	}
	if code, ok := NotificationErrorCodeOf(err); ok && code == NotificationErrNotImplemented {
		return NotificationStatusNotImplemented
	}
	return NotificationStatusBadRequest
}

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

// NotificationTransport performs the discovery and the single POST.
type NotificationTransport interface {
	DiscoverNotificationPeer(ctx context.Context, domain string) (*NotificationPeer, error)
	PostNotification(ctx context.Context, endpoint string, body []byte) (status int, header http.Header, err error)
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

// SendResult describes one attempt. Err holds transport or discovery
// errors only; it never contains the payload.
type SendResult struct {
	Outcome SendOutcome
	// Status is the HTTP status received, if any.
	Status int
	// Class is set for the recorded error classes; zero otherwise.
	Class NotificationStatusClass
	// RetryAfter is the delay-seconds of a 503 Retry-After, up to
	// maxRetryAfter, for diagnostics only. Other values are not kept,
	// because a peer can reflect any request value in the header.
	RetryAfter time.Duration
	Err        error
}

const maxRetryAfter = 24 * time.Hour

// ClassifyNotificationResponse maps a response to its outcome. Any 2xx is
// a delivery.
func ClassifyNotificationResponse(status int, header http.Header) SendResult {
	if status >= 200 && status < 300 {
		return SendResult{Outcome: SendDelivered, Status: status, Class: NotificationStatusCreated}
	}
	res := SendResult{Outcome: SendRejected, Status: status}
	switch NotificationStatusClass(status) {
	case NotificationStatusBadRequest, NotificationStatusUnauthorized, NotificationStatusForbidden,
		NotificationStatusNotFound, NotificationStatusNotImplemented:
		res.Class = NotificationStatusClass(status)
	case NotificationStatusUnavailable:
		res.Class = NotificationStatusUnavailable
		if secs, err := strconv.ParseUint(header.Get("Retry-After"), 10, 32); err == nil {
			res.RetryAfter = min(time.Duration(secs)*time.Second, maxRetryAfter)
		}
	}
	return res
}

// SendNotification runs Discover -> HasCapability -> Post -> classify once.
// It never posts when the peer does not advertise notifications.
func SendNotification(ctx context.Context, t NotificationTransport, domain string, body []byte) SendResult {
	peer, err := t.DiscoverNotificationPeer(ctx, domain)
	if err != nil {
		return SendResult{Outcome: SendDiscoveryFailed, Err: err}
	}
	if !peer.HasNotificationsCapability() || peer.Endpoint == "" {
		return SendResult{Outcome: SendCapabilityMissing}
	}
	status, header, err := t.PostNotification(ctx, peer.Endpoint, body)
	if err != nil {
		return SendResult{Outcome: SendPostFailed, Status: status, Err: err}
	}
	return ClassifyNotificationResponse(status, header)
}
