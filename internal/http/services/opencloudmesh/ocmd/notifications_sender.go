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
	"io"
	"net/http"
	"net/url"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/pkg/errors"
)

// OutgoingNotification is a notification this server sends. Its
// SharedSecret is only written to the wire; every log and string form of
// the notification redacts it.
type OutgoingNotification struct {
	Type         string
	SenderDomain string
	ResourceType string
	ShareType    string
	// EnvelopeProviderID duplicates notification.file.providerId in the
	// deprecated top-level providerId. This is the temporary
	// DEV-NC-PROVIDERID deviation for share-state notices: peers still on
	// the dated Nextcloud receiver require it. Retire it together with the
	// Nextcloud notification rework.
	EnvelopeProviderID bool
	Message            string
	SharedSecret       NotificationSecret
	File               *NotificationFile
}

// String implements fmt.Stringer without payload values.
func (n OutgoingNotification) String() string {
	return "outgoing notification{type=" + n.Type + "}"
}

// GoString implements fmt.GoStringer.
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
	ProviderID       string               `json:"providerId,omitempty"`
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
	if n.EnvelopeProviderID && n.File != nil {
		w.ProviderID = n.File.ProviderID
	}
	return json.Marshal(w)
}

// ShareStateNotification is the payload of the share-state notices. The
// providerId is the identifier the provider assigned to the share: for
// SHARE_UNSHARED and SHARE_CHANGE_PERMISSION it is this server's own share
// ID, for SHARE_ACCEPTED and SHARE_DECLINED it is the remote share ID that
// the provider sent. The optional secret is the share's protocol secret:
// the outgoing token for SHARE_UNSHARED, the received secret for
// SHARE_ACCEPTED and SHARE_DECLINED. Permissions are only sent with
// SHARE_CHANGE_PERMISSION.
type ShareStateNotification struct {
	Type         string
	SenderDomain string
	ResourceType string
	ShareType    string
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
	if s.SenderDomain == "" || s.ResourceType == "" || s.ProviderID == "" {
		return nil, errors.New("incomplete share-state notification")
	}
	return &OutgoingNotification{
		Type:               s.Type,
		SenderDomain:       s.SenderDomain,
		ResourceType:       s.ResourceType,
		ShareType:          s.ShareType,
		EnvelopeProviderID: true,
		SharedSecret:       NotificationSecret(s.SharedSecret),
		File:               &NotificationFile{ProviderID: s.ProviderID, Permissions: s.Permissions},
	}, nil
}

// ocmNotificationTransport adapts OCMClient to the portable transitions.
type ocmNotificationTransport struct {
	c *OCMClient
}

// DiscoverNotificationPeer reuses the client's discovery, including the
// legacy /ocm-provider fallback and the response size limit.
func (t ocmNotificationTransport) DiscoverNotificationPeer(ctx context.Context, domain string) (*NotificationPeer, error) {
	disco, err := t.c.Discover(ctx, domain)
	if err != nil {
		return nil, err
	}
	peer := &NotificationPeer{}
	if !disco.Enabled {
		return peer, nil
	}
	peer.Capabilities = disco.Capabilities
	base, err := absoluteDiscoveryBase(disco.Endpoint)
	if err != nil {
		return nil, errors.New("peer discovery has no usable endpoint")
	}
	endpoint, err := url.JoinPath(base.String(), notificationsPath)
	if err != nil {
		return nil, errors.New("peer discovery has no usable endpoint")
	}
	peer.Endpoint = endpoint
	return peer, nil
}

// PostNotification sends the body once, without following redirects. The
// response body is ignored and read only up to the control-plane limit.
func (t ocmNotificationTransport) PostNotification(ctx context.Context, endpoint string, body []byte) (int, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, errors.New("error creating notification request")
	}
	req.Header.Set("Content-Type", "application/json")
	// Never follow a redirect: a 307/308 would re-send the body, including
	// the optional secret, to another URL. A 3xx is an unclassified rejection.
	noRedirect := *t.c.client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noRedirect.Do(req)
	if err != nil {
		return 0, nil, errors.Wrap(err, "error sending notification")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, DefaultResponseLimit))
	return resp.StatusCode, resp.Header, nil
}

// NewNotification sends one notification to the OCM server at domain, if it
// advertises the notifications capability. It is attempted exactly once:
// there is no retry, queue or re-publish, and the result never undoes the
// change that caused the notification. The caller logs or ignores it.
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1notifications/post
func (c *OCMClient) NewNotification(ctx context.Context, domain string, n *OutgoingNotification) SendResult {
	log := appctx.GetLogger(ctx)
	if n == nil {
		return SendResult{Outcome: SendPostFailed, Err: errors.New("nil notification")}
	}
	body, err := n.wireBody()
	if err != nil {
		return SendResult{Outcome: SendPostFailed, Err: errors.New("notification could not be encoded")}
	}
	res := SendNotification(ctx, ocmNotificationTransport{c: c}, domain, body)
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
	if res.RetryAfter != "" {
		event = event.Str("retry_after", res.RetryAfter)
	}
	if res.Err != nil {
		event = event.Str("error", res.Err.Error())
	}
	event.Msg("OCM notification sent")
	return res
}
