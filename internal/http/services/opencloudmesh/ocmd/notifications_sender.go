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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/httpsig"
	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
)

// OutgoingNotification is a notification this server sends. Its
// SharedSecret is only written to the wire.
type OutgoingNotification struct {
	Type         string
	SenderDomain string
	ResourceType string
	ShareType    string
	Message      string
	SharedSecret NotificationSecret
	File         *NotificationFile
}

func (n OutgoingNotification) String() string {
	return "outgoing notification{type=" + n.Type + "}"
}

func (n OutgoingNotification) GoString() string { return n.String() }

type wireNotificationBody struct {
	Message      string            `json:"message,omitempty"`
	SharedSecret string            `json:"sharedSecret,omitempty"`
	File         *NotificationFile `json:"file,omitempty"`
}

type wireNotification struct {
	NotificationType string               `json:"notificationType"`
	SenderDomain     string               `json:"senderDomain"`
	ResourceType     string               `json:"resourceType,omitempty"`
	ShareType        string               `json:"shareType,omitempty"`
	Notification     wireNotificationBody `json:"notification"`
}

// MarshalJSON keeps the secret out of generic JSON forms, e.g. structured logs.
func (n OutgoingNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"notificationType": n.Type})
}

// wireBody is the request body sent to the peer.
func (n *OutgoingNotification) wireBody() ([]byte, error) {
	w := wireNotification{
		NotificationType: n.Type,
		SenderDomain:     n.SenderDomain,
		ResourceType:     n.ResourceType,
		ShareType:        n.ShareType,
		Notification: wireNotificationBody{
			Message:      n.Message,
			SharedSecret: n.SharedSecret.Reveal(),
			File:         n.File,
		},
	}
	return json.Marshal(w)
}

// ShareStateNotification is the payload of the share-state notices.
type ShareStateNotification struct {
	Type         string
	SenderDomain string
	ResourceType string
	ShareType    string
	// ProviderID is always the ID the provider assigned to the share: our own
	// ID when we are the provider, the received remote ID otherwise.
	ProviderID   string
	SharedSecret string
	Permissions  []string
}

// Build validates the share-state payload and returns the notification.
func (s ShareStateNotification) Build() (*OutgoingNotification, error) {
	switch s.Type {
	case NotificationShareAccepted, NotificationShareDeclined, NotificationShareUnshared:
		if len(s.Permissions) > 0 {
			return nil, errors.New("permissions are only sent with SHARE_CHANGE_PERMISSION")
		}
	case NotificationShareChangePermission:
		if s.SharedSecret != "" {
			return nil, errors.New("SHARE_CHANGE_PERMISSION carries no shared secret")
		}
	default:
		return nil, errors.New("not a share-state notification type")
	}
	if s.ResourceType == "" || s.ProviderID == "" {
		return nil, errors.New("incomplete share-state notification")
	}
	sender, err := providerdomain.CanonicalOCMHost(s.SenderDomain)
	if err != nil {
		return nil, errors.New("senderDomain is not an OCM host")
	}
	return &OutgoingNotification{
		Type:         s.Type,
		SenderDomain: sender,
		ResourceType: s.ResourceType,
		ShareType:    s.ShareType,
		SharedSecret: NotificationSecret(s.SharedSecret),
		File:         &NotificationFile{ProviderID: s.ProviderID, Permissions: s.Permissions},
	}, nil
}

// ocmNotificationTransport adapts OCMClient to the portable transitions.
type ocmNotificationTransport struct {
	c *OCMClient
	// key is the sending service's own notification signing key. It may be
	// nil; a peer that requires signed notifications is then failed before
	// any request.
	key *httpsig.Key
}

var (
	errInvalidPeer     = errors.New("peer is not an OCM host")
	errUnsupportedPeer = errors.New("peer host cannot be reached over HTTP as written")
	errNoSigningKey    = errors.New("peer requires signed notifications but no signing key is configured")
	errSigningFailed   = errors.New("notification signing failed")
)

// notificationDiscoveryBase is the URL of the peer identity, or false when
// HTTP cannot reach that exact host and port: the identity is never
// rewritten, e.g. by dropping an empty port or decoding a percent triplet.
func notificationDiscoveryBase(host string) (string, bool) {
	u, err := url.Parse(discoveryBase(host))
	if err != nil || u.Host != host || strings.ContainsAny(u.Hostname(), "%[") || strings.HasSuffix(host, ":") {
		return "", false
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return "", false
		}
	}
	return u.String(), true
}

// DiscoverNotificationPeer uses only the canonical /.well-known/ocm document
// of the peer identity. The endpoint it advertises is only used for routing.
func (t ocmNotificationTransport) DiscoverNotificationPeer(ctx context.Context, domain string) (*NotificationPeer, error) {
	host, err := providerdomain.CanonicalOCMHost(domain)
	if err != nil {
		return nil, errInvalidPeer
	}
	base, ok := notificationDiscoveryBase(host)
	if !ok {
		return nil, errUnsupportedPeer
	}
	disco, err := t.c.DiscoverCanonical(ctx, base)
	if err != nil {
		return nil, err
	}
	peer := &NotificationPeer{}
	if !disco.Enabled {
		return peer, nil
	}
	peer.Capabilities = disco.Capabilities
	route, err := absoluteDiscoveryBase(disco.Endpoint)
	if err != nil {
		return nil, errors.New("peer discovery has no usable endpoint")
	}
	endpoint, err := url.JoinPath(route.String(), notificationsPath)
	if err != nil {
		return nil, errors.New("peer discovery has no usable endpoint")
	}
	peer.Endpoint = endpoint
	return peer, nil
}

// errPostFailed and postTimeoutError replace transport errors, whose text
// can quote peer bytes from a malformed status line or header.
var errPostFailed = errors.New("notification request failed")

type postTimeoutError struct{}

func (postTimeoutError) Error() string { return "notification request timed out" }
func (postTimeoutError) Timeout() bool { return true }

// safePostError keeps only the category of a transport error.
func safePostError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
		return postTimeoutError{}
	}
	return errPostFailed
}

// maxRetryAfterDigits bounds the delay-seconds value that is parsed at all.
const maxRetryAfterDigits = 9

// parseRetryAfter reads a delay-seconds Retry-After. An HTTP date or any
// other value is ignored.
func parseRetryAfter(v string) time.Duration {
	if v == "" || len(v) > maxRetryAfterDigits {
		return 0
	}
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return 0
		}
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return time.Duration(n) * time.Second
}

// PostNotification sends the body once, without following redirects. A
// configured key signs exactly the bytes that are sent, even for a peer that
// does not advertise http-sig; a peer that requires signatures without a
// local key fails before anything is sent. The response body is ignored and
// read only up to the control-plane limit.
func (t ocmNotificationTransport) PostNotification(ctx context.Context, peer *NotificationPeer, body []byte) (NotificationResponse, error) {
	if peer == nil || peer.Endpoint == "" {
		return NotificationResponse{}, errInvalidPeer
	}
	if t.key == nil && slices.Contains(peer.Capabilities, "http-sig") {
		return NotificationResponse{}, errNoSigningKey
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, peer.Endpoint, bytes.NewReader(body))
	if err != nil {
		return NotificationResponse{}, errPostFailed
	}
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	if t.key != nil {
		if err := t.key.Sign(req, body); err != nil {
			return NotificationResponse{}, errSigningFailed
		}
	}
	// Never follow a redirect: a 307/308 would re-send the body, including
	// the optional secret, to another URL. A 3xx is an unclassified rejection.
	noRedirect := *t.c.client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noRedirect.Do(req)
	if err != nil {
		return NotificationResponse{}, safePostError(ctx, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, DefaultResponseLimit))
	return NotificationResponse{Status: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}, nil
}

// NewNotification sends a notification to the OCM server at domain if it
// advertises the notifications capability. It is attempted at most once.
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1notifications/post
func (c *OCMClient) NewNotification(ctx context.Context, domain string, n *OutgoingNotification, key *httpsig.Key) SendResult {
	log := appctx.GetLogger(ctx)
	if n == nil {
		return SendResult{Outcome: SendPostFailed, Failure: FailureError}
	}
	body, err := n.wireBody()
	if err != nil {
		return SendResult{Outcome: SendPostFailed, Failure: FailureError}
	}
	res := SendNotification(ctx, ocmNotificationTransport{c: c, key: key}, domain, body)
	event := log.Info()
	if res.Outcome != SendDelivered {
		event = log.Warn()
	}
	event = event.Str("type", n.Type).Str("domain", domain).Str("outcome", string(res.Outcome))
	if res.Status != 0 {
		event = event.Int("status", res.Status)
	}
	if res.Class != 0 {
		event = event.Int("class", int(res.Class))
	}
	if res.RetryAfter > 0 {
		event = event.Dur("retry_after", res.RetryAfter)
	}
	if res.Failure != "" {
		event = event.Str("failure", string(res.Failure))
	}
	event.Msg("OCM notification sent")
	return res
}
