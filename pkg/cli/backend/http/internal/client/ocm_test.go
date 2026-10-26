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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

func TestGenerateInvite(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, ocmGenerateInvite, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"token":"abc123","description":"joint analysis","expiration":1767225600,
		  "invite_link":"https://cernbox.test/ocm/invite?token=abc123"}`)
	})

	invite, err := f.client().GenerateInvite(context.Background(), "joint analysis", "")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Token != "abc123" {
		t.Errorf("Token = %q", invite.Token)
	}
	if invite.Link == "" {
		t.Error("the invite link is what the user has to pass on, and it was dropped")
	}
	if invite.Expiration == nil || !invite.Expiration.Equal(time.Unix(1767225600, 0)) {
		t.Errorf("Expiration = %v", invite.Expiration)
	}
}

func TestGenerateInviteOmitsEmptyFields(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, ocmGenerateInvite, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"token":"abc123"}`)
	})

	if _, err := f.client().GenerateInvite(context.Background(), "", ""); err != nil {
		t.Fatal(err)
	}
	body := f.lastRequest(http.MethodPost).Body
	var sent map[string]string
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, body)
	}
	// Sending an empty recipient would ask the server to mail nobody.
	if _, present := sent["recipient"]; present {
		t.Errorf("recipient should be omitted when unset: %s", body)
	}
	if _, present := sent["description"]; present {
		t.Errorf("description should be omitted when unset: %s", body)
	}
}

func TestGenerateInviteWithoutToken(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, ocmGenerateInvite, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	})

	_, err := f.client().GenerateInvite(context.Background(), "", "")
	if err == nil || !strings.Contains(err.Error(), "no invitation token") {
		t.Errorf("got %v, want a clear error", err)
	}
}

// TestOCMUnavailableIsExplained: the sciencemesh service is optional, and a
// bare 404 reads as "you have no invitations" rather than "this deployment does
// not do federated sharing".
func TestOCMUnavailableIsExplained(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, ocmGenerateInvite, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	_, err := f.client().GenerateInvite(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "does not expose federated sharing") {
		t.Errorf("got %v, want an explanation rather than a bare 404", err)
	}
}

func TestListInvites(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, ocmListInvite, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"token":"abc","description":"one"},{"token":"def"}]`)
	})

	invites, err := f.client().ListInvites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(invites) != 2 || invites[0].Token != "abc" {
		t.Errorf("invites = %+v", invites)
	}
	if invites[1].Expiration != nil {
		t.Error("an absent expiration should stay nil rather than become the epoch")
	}
}

func TestAcceptInvite(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, ocmAcceptInvite, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	})

	if err := f.client().AcceptInvite(context.Background(), "abc123", "other-lab.org"); err != nil {
		t.Fatal(err)
	}
	body := f.lastRequest(http.MethodPost).Body
	// The field is providerDomain, not provider: reva decodes it strictly.
	if !strings.Contains(body, `"providerDomain":"other-lab.org"`) {
		t.Errorf("body = %s", body)
	}
	if !strings.Contains(body, `"token":"abc123"`) {
		t.Errorf("body = %s", body)
	}
}

func TestAcceptInviteRequiresBothParts(t *testing.T) {
	f := newFakeServer(t)
	c := f.client()

	if err := c.AcceptInvite(context.Background(), "abc", ""); cberr.KindOf(err) != cberr.KindUsage {
		t.Errorf("got %v, want a usage error for a missing provider", err)
	}
	if err := c.AcceptInvite(context.Background(), "", "other-lab.org"); cberr.KindOf(err) != cberr.KindUsage {
		t.Errorf("got %v, want a usage error for a missing token", err)
	}
}

func TestAcceptedUsers(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, ocmFindAccepted, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"display_name":"Alice","idp":"https://other-lab.org","user_id":"alice","mail":"alice@other-lab.org"}]`)
	})

	users, err := f.client().AcceptedUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("got %d users", len(users))
	}
	// The address is what gets passed to --with-remote, so the scheme must be
	// stripped: "alice@https://other-lab.org" is not a recipient.
	if got := users[0].Address(); got != "alice@other-lab.org" {
		t.Errorf("Address() = %q, want alice@other-lab.org", got)
	}
}

func TestRemoteUserAddress(t *testing.T) {
	tests := []struct {
		user RemoteUser
		want string
	}{
		{RemoteUser{UserID: "alice", IDP: "https://other-lab.org"}, "alice@other-lab.org"},
		{RemoteUser{UserID: "alice", IDP: "http://other-lab.org"}, "alice@other-lab.org"},
		{RemoteUser{UserID: "alice", IDP: "other-lab.org"}, "alice@other-lab.org"},
		{RemoteUser{UserID: "alice"}, "alice"},
	}
	for _, tt := range tests {
		if got := tt.user.Address(); got != tt.want {
			t.Errorf("Address(%+v) = %q, want %q", tt.user, got, tt.want)
		}
	}
}

func TestRemoveAcceptedUser(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodDelete, ocmDeleteAccepted, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	})

	if err := f.client().RemoveAcceptedUser(context.Background(), "other-lab.org", "alice"); err != nil {
		t.Fatal(err)
	}
	body := f.lastRequest(http.MethodDelete).Body
	if !strings.Contains(body, `"user_id":"alice"`) || !strings.Contains(body, `"idp":"other-lab.org"`) {
		t.Errorf("body = %s", body)
	}
}

func TestRemoveAcceptedUserRequiresBothParts(t *testing.T) {
	f := newFakeServer(t)
	if err := f.client().RemoveAcceptedUser(context.Background(), "", "alice"); cberr.KindOf(err) != cberr.KindUsage {
		t.Errorf("got %v, want a usage error", err)
	}
}

func TestListProviders(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, ocmListProviders, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"OtherLab","full_name":"The Other Laboratory","domain":"other-lab.org",
		  "homepage":"https://other-lab.org"}]`)
	})

	providers, err := f.client().ListProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].Domain != "other-lab.org" {
		t.Errorf("providers = %+v", providers)
	}
}

// TestReceivedFederatedSharesFiltersOnTheOCMPrefix: sharedWithMe returns local
// and federated shares in one listing, and only the id prefix tells them apart.
func TestReceivedFederatedSharesFiltersOnTheOCMPrefix(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, graphBeta+"/me/drive/sharedWithMe", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"value":[
		  {"id":"local-1","name":"local-folder","@client.synchronize":true,
		   "createdBy":{"user":{"id":"marie","displayName":"Marie Curie"}},
		   "permissions":[{"id":"p1","roles":["`+RoleViewer+`"]}]},
		  {"id":"ocm-1","name":"shared-data","@client.synchronize":true,
		   "remoteItem":{"id":"`+OCMReceivedPrefix+`ABC","name":"shared-data"},
		   "createdBy":{"user":{"id":"alice@other-lab.org","displayName":"Alice"}},
		   "permissions":[{"id":"p2","roles":["`+RoleEditor+`"]}]}
		]}`)
	})

	shares, err := f.client().ReceivedFederatedShares(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 {
		t.Fatalf("got %d federated shares, want only the OCM one: %+v", len(shares), shares)
	}

	s := shares[0]
	if s.Name != "shared-data" {
		t.Errorf("name = %q", s.Name)
	}
	if s.Owner != "alice@other-lab.org" {
		t.Errorf("owner = %q", s.Owner)
	}
	// The provider is split out of the address, so a listing can show where a
	// share came from without the reader parsing it.
	if s.Provider != "other-lab.org" {
		t.Errorf("provider = %q", s.Provider)
	}
	if s.Role != "editor" {
		t.Errorf("role = %q", s.Role)
	}
	if !s.Accepted {
		t.Error("the share should read as accepted")
	}
}

func TestReceivedFederatedSharesWithNoneAtAll(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, graphBeta+"/me/drive/sharedWithMe", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"value":[]}`)
	})

	shares, err := f.client().ReceivedFederatedShares(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 0 {
		t.Errorf("got %d shares, want none", len(shares))
	}
}
