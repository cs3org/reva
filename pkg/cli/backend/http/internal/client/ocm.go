// Copyright 2018-2026 CERN
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

package client

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// Sciencemesh endpoints. These are the user-facing side of OCM: the
// provider-to-provider protocol lives at /ocm and is not something a person
// calls.
const (
	ocmGenerateInvite = "/sciencemesh/generate-invite"
	ocmListInvite     = "/sciencemesh/list-invite"
	ocmAcceptInvite   = "/sciencemesh/accept-invite"
	ocmFindAccepted   = "/sciencemesh/find-accepted-users"
	ocmDeleteAccepted = "/sciencemesh/delete-accepted-user"
	ocmListProviders  = "/sciencemesh/list-providers"
)

// RecipientRemote is the recipient type for a user at another OCM provider.
// The object id is "user@provider.example.org".
const RecipientRemote = "remote"

// Invite is a federated sharing invitation.
type Invite struct {
	Token       string     `json:"token"`
	Description string     `json:"description,omitempty"`
	Expiration  *time.Time `json:"expiration,omitempty"`
	// Link is what you send to the other person. It carries the token and the
	// provider, so they do not have to assemble either by hand.
	Link string `json:"link,omitempty"`
}

type rawInvite struct {
	Token       string `json:"token"`
	Description string `json:"description"`
	Expiration  uint64 `json:"expiration"`
	InviteLink  string `json:"invite_link"`
}

func (r rawInvite) toInvite() Invite {
	inv := Invite{Token: r.Token, Description: r.Description, Link: r.InviteLink}
	if r.Expiration > 0 {
		t := time.Unix(int64(r.Expiration), 0)
		inv.Expiration = &t
	}
	return inv
}

// RemoteUser is a user at another OCM provider who has accepted an invitation.
type RemoteUser struct {
	DisplayName string `json:"display_name,omitempty"`
	// IDP is the remote provider's identity provider.
	IDP string `json:"idp"`
	// UserID identifies the user at that provider.
	UserID string `json:"user_id"`
	Mail   string `json:"mail,omitempty"`
}

// Address renders the user in the "user@provider" form that share recipients
// take.
func (u RemoteUser) Address() string {
	if u.UserID == "" || u.IDP == "" {
		return u.UserID
	}
	return u.UserID + "@" + strings.TrimPrefix(strings.TrimPrefix(u.IDP, "https://"), "http://")
}

// GenerateInvite creates an invitation token. When recipient is a mail address
// and the server is configured to send mail, it is also mailed to them.
func (c *Client) GenerateInvite(ctx context.Context, description, recipient string) (*Invite, error) {
	body := map[string]string{}
	if description != "" {
		body["description"] = description
	}
	if recipient != "" {
		body["recipient"] = recipient
	}

	var raw rawInvite
	if err := c.postJSON(ctx, c.URL(ocmGenerateInvite), "generate an invitation", "", body, &raw); err != nil {
		return nil, ocmUnavailable(err, "generate an invitation")
	}
	if raw.Token == "" {
		return nil, cberr.New(cberr.KindOther, "generate an invitation", "",
			"the server returned no invitation token")
	}
	inv := raw.toInvite()
	return &inv, nil
}

// ListInvites returns the invitations the caller has created.
func (c *Client) ListInvites(ctx context.Context) ([]Invite, error) {
	var raws []rawInvite
	if err := c.getJSON(ctx, c.URL(ocmListInvite), "list invitations", "", &raws); err != nil {
		return nil, ocmUnavailable(err, "list invitations")
	}
	out := make([]Invite, 0, len(raws))
	for _, r := range raws {
		out = append(out, r.toInvite())
	}
	return out, nil
}

// AcceptInvite accepts an invitation issued by another provider.
func (c *Client) AcceptInvite(ctx context.Context, token, providerDomain string) error {
	if token == "" || providerDomain == "" {
		return cberr.Usagef("accepting an invitation needs both the token and the provider it came from")
	}
	body := map[string]string{"token": token, "providerDomain": providerDomain}
	if err := c.postJSON(ctx, c.URL(ocmAcceptInvite), "accept an invitation", token, body, nil); err != nil {
		return ocmUnavailable(err, "accept an invitation")
	}
	return nil
}

// AcceptedUsers returns the remote users who can be shared with.
func (c *Client) AcceptedUsers(ctx context.Context) ([]RemoteUser, error) {
	var users []RemoteUser
	if err := c.getJSON(ctx, c.URL(ocmFindAccepted), "list federated contacts", "", &users); err != nil {
		return nil, ocmUnavailable(err, "list federated contacts")
	}
	return users, nil
}

// RemoveAcceptedUser drops a remote user from the accepted list.
func (c *Client) RemoveAcceptedUser(ctx context.Context, idp, userID string) error {
	if idp == "" || userID == "" {
		return cberr.Usagef("removing a federated contact needs both the provider and the user id")
	}
	body := map[string]string{"idp": idp, "user_id": userID}
	if err := c.sendJSON(ctx, http.MethodDelete, c.URL(ocmDeleteAccepted),
		"remove a federated contact", userID, body, nil); err != nil {
		return ocmUnavailable(err, "remove a federated contact")
	}
	return nil
}

// OCMProvider is a federation partner.
type OCMProvider struct {
	Name     string `json:"name,omitempty"`
	FullName string `json:"full_name,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Homepage string `json:"homepage,omitempty"`
}

// ListProviders returns the OCM providers this instance federates with.
func (c *Client) ListProviders(ctx context.Context) ([]OCMProvider, error) {
	var providers []OCMProvider
	if err := c.getJSON(ctx, c.URL(ocmListProviders), "list federation partners", "", &providers); err != nil {
		return nil, ocmUnavailable(err, "list federation partners")
	}
	return providers, nil
}

// OCMReceivedPrefix marks a received federated share. reva encodes the id of
// an OCM share this way, and it is the only reliable way to tell a federated
// received share from a local one in a listing that holds both.
const OCMReceivedPrefix = "ocm-received$"

// FederatedShare is a share received from another provider.
type FederatedShare struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Owner is who shared it, as "user@provider".
	Owner string `json:"owner,omitempty"`
	// Provider is the domain it came from.
	Provider string `json:"provider,omitempty"`
	// Role is the role held on the shared resource.
	Role string `json:"role,omitempty"`
	// Accepted reports whether the share has been accepted locally.
	Accepted bool `json:"accepted"`
}

// ReceivedFederatedShares lists shares other providers have made with the
// caller.
//
// This reads the graph sharedWithMe endpoint, not the OCS remote_shares one.
// OCS has a route for it, but reva's handler is an empty stub that writes
// nothing, so a client built on it would report "no federated shares" however
// many there were. The graph endpoint returns them for real, provided the
// server has ocm_enabled set on its graph service.
func (c *Client) ReceivedFederatedShares(ctx context.Context) ([]FederatedShare, error) {
	items, err := c.SharedWithMe(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]FederatedShare, 0)
	for _, item := range items {
		if !item.Federated {
			continue
		}
		share := FederatedShare{
			ID:       item.ID,
			Name:     item.Name,
			Role:     item.Role,
			Accepted: item.Accepted,
		}
		if item.SharedBy != nil {
			share.Owner = firstNonEmptyString(item.SharedBy.ID, item.SharedBy.DisplayName)
			if _, domain, ok := strings.Cut(share.Owner, "@"); ok {
				share.Provider = domain
			}
		}
		out = append(out, share)
	}
	return out, nil
}

// ocmUnavailable turns a 404 into an explanation. The sciencemesh service is
// optional, and a bare "not found" would read as "no invitations" rather than
// "this deployment does not do federated sharing".
func ocmUnavailable(err error, op string) error {
	if cberr.KindOf(err) != cberr.KindNotFound {
		return err
	}
	return cberr.New(cberr.KindOther, op, "",
		"this deployment does not expose federated sharing (OCM)")
}
