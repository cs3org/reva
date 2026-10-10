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

// Package contracttest holds the shared behavioral suite for invite
// repository adapters. It is test support: only the adapter test packages
// under pkg/ocm/invite/repository are expected to import it.
package contracttest

import (
	"context"
	"errors"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
)

// Run exercises the invite.Repository contract against a repository produced
// by newRepository: caller-input validation, the valid CRUD flows, and the
// record-matching guarantees that do not depend on the storage backend.
// Backend-specific behavior (wildcard versus exact Idp matching, expiry
// policy) is covered by the adapter test packages, not here.
func Run(t *testing.T, newRepository func(*testing.T) invite.Repository) {
	t.Helper()

	ctx := context.Background()

	newInitiator := func() *userpb.UserId {
		return &userpb.UserId{
			OpaqueId: "initiator",
			Idp:      "local.example.com",
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
		}
	}
	newRemoteUser := func(opaqueID, idp string) *userpb.User {
		return &userpb.User{
			Id:          &userpb.UserId{OpaqueId: opaqueID, Idp: idp, Type: userpb.UserType_USER_TYPE_FEDERATED},
			Username:    opaqueID,
			DisplayName: "Display " + opaqueID,
			Mail:        opaqueID + "@" + idp,
		}
	}
	newToken := func(secret string, expiration time.Duration) *invitepb.InviteToken {
		return &invitepb.InviteToken{
			Token:       secret,
			UserId:      newInitiator(),
			Expiration:  &typespb.Timestamp{Seconds: uint64(time.Now().Add(expiration).Unix())},
			Description: "contract test token",
		}
	}

	assertBadRequest := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected errtypes.BadRequest, got nil")
		}
		if _, ok := err.(errtypes.BadRequest); !ok {
			t.Fatalf("expected errtypes.BadRequest, got %T: %v", err, err)
		}
	}
	assertNotFound := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected errtypes.NotFound, got nil")
		}
		if _, ok := err.(errtypes.NotFound); !ok {
			t.Fatalf("expected errtypes.NotFound, got %T: %v", err, err)
		}
	}
	assertUsersEqual := func(t *testing.T, got, want *userpb.User) {
		t.Helper()
		if got == nil {
			t.Fatal("expected a stored user, got nil")
		}
		if got.Id.GetOpaqueId() != want.Id.GetOpaqueId() || got.Id.GetIdp() != want.Id.GetIdp() {
			t.Fatalf("stored user id mismatch: got %v, want %v", got.Id, want.Id)
		}
		// username is not part of the shared contract: the sql schema does
		// not store it
		if got.Mail != want.Mail || got.DisplayName != want.DisplayName {
			t.Fatalf("stored user detail mismatch: got %v, want %v", got, want)
		}
	}

	t.Run("AddTokenRejectsInvalidInput", func(t *testing.T) {
		repo := newRepository(t)

		for _, token := range []*invitepb.InviteToken{
			nil,
			{},
			{Token: "   ", UserId: newInitiator()},
			{Token: "contract-secret", UserId: nil},
			{Token: "contract-secret", UserId: &userpb.UserId{}},
			{Token: "contract-secret", UserId: &userpb.UserId{OpaqueId: "  "}},
		} {
			assertBadRequest(t, repo.AddToken(ctx, token))
		}

		// rejected writes must leave no trace
		if _, err := repo.GetToken(ctx, "contract-secret"); !errors.Is(err, invite.ErrTokenNotFound) {
			t.Fatalf("expected ErrTokenNotFound for rejected token, got %v", err)
		}
		tokens, err := repo.ListTokens(ctx, newInitiator())
		if err != nil {
			t.Fatal(err)
		}
		if len(tokens) != 0 {
			t.Fatalf("expected no stored tokens, got %d", len(tokens))
		}
	})

	t.Run("GetTokenRejectsBlankToken", func(t *testing.T) {
		repo := newRepository(t)

		for _, secret := range []string{"", "   "} {
			tkn, err := repo.GetToken(ctx, secret)
			if tkn != nil {
				t.Fatalf("expected nil token for blank secret %q, got %v", secret, tkn)
			}
			assertBadRequest(t, err)
		}
	})

	t.Run("ListTokensRejectsInvalidInitiator", func(t *testing.T) {
		repo := newRepository(t)

		for _, initiator := range []*userpb.UserId{nil, {}, {OpaqueId: " "}} {
			tokens, err := repo.ListTokens(ctx, initiator)
			if tokens != nil {
				t.Fatalf("expected nil token list, got %v", tokens)
			}
			assertBadRequest(t, err)
		}
	})

	t.Run("AddRemoteUserRejectsInvalidInput", func(t *testing.T) {
		repo := newRepository(t)
		valid := newRemoteUser("alice", "one.example.com")

		for _, tc := range []struct {
			name       string
			initiator  *userpb.UserId
			remoteUser *userpb.User
		}{
			{"nil initiator", nil, valid},
			{"blank initiator", &userpb.UserId{}, valid},
			{"whitespace initiator", &userpb.UserId{OpaqueId: " "}, valid},
			{"nil remote user", newInitiator(), nil},
			{"remote user without id", newInitiator(), &userpb.User{}},
			{"remote user with blank id", newInitiator(), &userpb.User{Id: &userpb.UserId{}}},
			{"remote user with whitespace id", newInitiator(), &userpb.User{Id: &userpb.UserId{OpaqueId: " "}}},
		} {
			assertBadRequest(t, repo.AddRemoteUser(ctx, tc.initiator, tc.remoteUser))
		}

		// rejected writes must leave no trace: the same user can still be added
		if err := repo.AddRemoteUser(ctx, newInitiator(), valid); err != nil {
			t.Fatalf("valid add after rejected adds failed: %v", err)
		}
		if _, err := repo.GetRemoteUser(ctx, newInitiator(), valid.Id); err != nil {
			t.Fatalf("expected stored user after valid add, got %v", err)
		}
	})

	t.Run("GetRemoteUserRejectsInvalidInput", func(t *testing.T) {
		repo := newRepository(t)
		initiator := newInitiator()
		remoteID := &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}
		neighbor := newRemoteUser("alice", "one.example.com")
		if err := repo.AddRemoteUser(ctx, initiator, neighbor); err != nil {
			t.Fatalf("seed valid neighbor failed: %v", err)
		}
		before, err := repo.FindRemoteUsers(ctx, initiator, "")
		if err != nil {
			t.Fatal(err)
		}

		for _, tc := range []struct {
			name      string
			initiator *userpb.UserId
			remote    *userpb.UserId
		}{
			{"nil initiator", nil, remoteID},
			{"blank initiator", &userpb.UserId{}, remoteID},
			{"whitespace initiator", &userpb.UserId{OpaqueId: " "}, remoteID},
			{"nil remote id", initiator, nil},
			{"blank remote id", initiator, &userpb.UserId{}},
			{"whitespace remote id", initiator, &userpb.UserId{OpaqueId: " "}},
		} {
			user, err := repo.GetRemoteUser(ctx, tc.initiator, tc.remote)
			if user != nil {
				t.Fatalf("%s: expected nil user, got %v", tc.name, user)
			}
			assertBadRequest(t, err)
		}

		after, err := repo.FindRemoteUsers(ctx, initiator, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(after) != len(before) {
			t.Fatalf("rejected lookups changed record count: before=%d after=%d", len(before), len(after))
		}
		if _, err := repo.GetRemoteUser(ctx, initiator, remoteID); err != nil {
			t.Fatalf("valid neighbor must remain retrievable: %v", err)
		}
	})

	t.Run("FindRemoteUsersRejectsInvalidInitiator", func(t *testing.T) {
		repo := newRepository(t)

		for _, initiator := range []*userpb.UserId{nil, {}, {OpaqueId: " "}} {
			users, err := repo.FindRemoteUsers(ctx, initiator, "alice")
			if users != nil {
				t.Fatalf("expected nil user list, got %v", users)
			}
			assertBadRequest(t, err)
		}

		// an empty query remains valid and returns an empty result
		users, err := repo.FindRemoteUsers(ctx, newInitiator(), "")
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 0 {
			t.Fatalf("expected empty result for empty query, got %d", len(users))
		}
	})

	t.Run("DeleteRemoteUserRejectsInvalidInput", func(t *testing.T) {
		repo := newRepository(t)
		initiator := newInitiator()
		remoteID := &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}
		neighbor := newRemoteUser("alice", "one.example.com")
		if err := repo.AddRemoteUser(ctx, initiator, neighbor); err != nil {
			t.Fatalf("seed valid neighbor failed: %v", err)
		}
		before, err := repo.FindRemoteUsers(ctx, initiator, "")
		if err != nil {
			t.Fatal(err)
		}

		for _, tc := range []struct {
			name      string
			initiator *userpb.UserId
			remote    *userpb.UserId
		}{
			{"nil initiator", nil, remoteID},
			{"blank initiator", &userpb.UserId{}, remoteID},
			{"whitespace initiator", &userpb.UserId{OpaqueId: " "}, remoteID},
			{"nil remote id", initiator, nil},
			{"blank remote id", initiator, &userpb.UserId{}},
			{"whitespace remote id", initiator, &userpb.UserId{OpaqueId: " "}},
		} {
			assertBadRequest(t, repo.DeleteRemoteUser(ctx, tc.initiator, tc.remote))
		}

		after, err := repo.FindRemoteUsers(ctx, initiator, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(after) != len(before) {
			t.Fatalf("rejected deletes changed record count: before=%d after=%d", len(before), len(after))
		}
		if _, err := repo.GetRemoteUser(ctx, initiator, remoteID); err != nil {
			t.Fatalf("valid neighbor must remain after rejected deletes: %v", err)
		}

		// a valid delete with no matching user remains a successful no-op
		if err := repo.DeleteRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "ghost", Idp: "ghost.example.com"}); err != nil {
			t.Fatalf("expected no-op delete, got %v", err)
		}
	})

	t.Run("RemoteUserLifecycle", func(t *testing.T) {
		repo := newRepository(t)
		alice := newRemoteUser("alice", "one.example.com")

		if err := repo.AddRemoteUser(ctx, newInitiator(), alice); err != nil {
			t.Fatal(err)
		}

		got, err := repo.GetRemoteUser(ctx, newInitiator(), alice.Id)
		if err != nil {
			t.Fatal(err)
		}
		assertUsersEqual(t, got, alice)

		users, err := repo.FindRemoteUsers(ctx, newInitiator(), "")
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 {
			t.Fatalf("expected 1 accepted user, got %d", len(users))
		}
		if users, err = repo.FindRemoteUsers(ctx, newInitiator(), "alice"); err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 {
			t.Fatalf("expected 1 accepted user matching the query, got %d", len(users))
		}
		if users, err = repo.FindRemoteUsers(ctx, newInitiator(), "zzz-no-match"); err != nil {
			t.Fatal(err)
		}
		if len(users) != 0 {
			t.Fatalf("expected no match, got %d", len(users))
		}

		// adding the same identity again reports the existing acceptance
		if err := repo.AddRemoteUser(ctx, newInitiator(), newRemoteUser("alice", "one.example.com")); !errors.Is(err, invite.ErrUserAlreadyAccepted) {
			t.Fatalf("expected ErrUserAlreadyAccepted, got %v", err)
		}
		if users, err = repo.FindRemoteUsers(ctx, newInitiator(), ""); err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 {
			t.Fatalf("duplicate add must not store a second row, got %d", len(users))
		}

		// delete, miss, re-add
		if err := repo.DeleteRemoteUser(ctx, newInitiator(), alice.Id); err != nil {
			t.Fatal(err)
		}
		_, err = repo.GetRemoteUser(ctx, newInitiator(), alice.Id)
		assertNotFound(t, err)
		if err := repo.AddRemoteUser(ctx, newInitiator(), alice); err != nil {
			t.Fatalf("re-adding after delete failed: %v", err)
		}
		if got, err = repo.GetRemoteUser(ctx, newInitiator(), alice.Id); err != nil {
			t.Fatal(err)
		}
		assertUsersEqual(t, got, alice)
	})

	t.Run("ValueEquivalentInitiatorKeys", func(t *testing.T) {
		repo := newRepository(t)
		alice := newRemoteUser("alice", "one.example.com")

		if err := repo.AddRemoteUser(ctx, newInitiator(), alice); err != nil {
			t.Fatal(err)
		}

		// every fresh allocation of an equal-value initiator must see the
		// stored record: identity keys are values, not pointers
		for range 25 {
			if _, err := repo.GetRemoteUser(ctx, newInitiator(), alice.Id); err != nil {
				t.Fatalf("equal-value initiator missed the stored user: %v", err)
			}
			users, err := repo.FindRemoteUsers(ctx, newInitiator(), "")
			if err != nil {
				t.Fatal(err)
			}
			if len(users) != 1 {
				t.Fatalf("equal-value initiator missed the stored user list, got %d", len(users))
			}
		}

		if err := repo.DeleteRemoteUser(ctx, newInitiator(), alice.Id); err != nil {
			t.Fatalf("delete under an equal-value initiator failed: %v", err)
		}
		if _, err := repo.GetRemoteUser(ctx, newInitiator(), alice.Id); err == nil {
			t.Fatal("expected the stored user to be deleted")
		}
	})

	t.Run("InitiatorIsolation", func(t *testing.T) {
		repo := newRepository(t)
		alice := newRemoteUser("alice", "one.example.com")
		other := &userpb.UserId{OpaqueId: "other-initiator", Idp: "local.example.com"}

		if err := repo.AddRemoteUser(ctx, newInitiator(), alice); err != nil {
			t.Fatal(err)
		}

		_, err := repo.GetRemoteUser(ctx, other, alice.Id)
		assertNotFound(t, err)
		users, err := repo.FindRemoteUsers(ctx, other, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 0 {
			t.Fatalf("records leaked across initiators, got %d", len(users))
		}

		// deleting under another initiator is a no-op and keeps the record
		if err := repo.DeleteRemoteUser(ctx, other, alice.Id); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetRemoteUser(ctx, newInitiator(), alice.Id); err != nil {
			t.Fatalf("record disappeared after a no-op delete: %v", err)
		}
	})

	t.Run("ProviderDistinctIdentities", func(t *testing.T) {
		repo := newRepository(t)
		one := newRemoteUser("shared-id", "one.example.com")
		two := newRemoteUser("shared-id", "two.example.com")

		if err := repo.AddRemoteUser(ctx, newInitiator(), one); err != nil {
			t.Fatal(err)
		}
		if err := repo.AddRemoteUser(ctx, newInitiator(), two); err != nil {
			t.Fatalf("same opaque id at a different provider must not be a duplicate: %v", err)
		}

		got, err := repo.GetRemoteUser(ctx, newInitiator(), one.Id)
		if err != nil {
			t.Fatal(err)
		}
		assertUsersEqual(t, got, one)
		if got, err = repo.GetRemoteUser(ctx, newInitiator(), two.Id); err != nil {
			t.Fatal(err)
		}
		assertUsersEqual(t, got, two)

		users, err := repo.FindRemoteUsers(ctx, newInitiator(), "")
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Fatalf("expected 2 provider-distinct users, got %d", len(users))
		}
	})

	t.Run("TokenLifecycle", func(t *testing.T) {
		repo := newRepository(t)
		token := newToken("contract-secret", 24*time.Hour)

		if err := repo.AddToken(ctx, token); err != nil {
			t.Fatal(err)
		}

		got, err := repo.GetToken(ctx, "contract-secret")
		if err != nil {
			t.Fatal(err)
		}
		// the sql driver stores the initiator through FormatUserID, which
		// keeps only the opaque id; Idp and user type are driver-dependent
		if got.Token != token.Token || got.UserId.GetOpaqueId() != token.UserId.GetOpaqueId() {
			t.Fatalf("stored token mismatch: got %+v", got)
		}

		if _, err := repo.GetToken(ctx, "unknown-secret"); !errors.Is(err, invite.ErrTokenNotFound) {
			t.Fatalf("expected ErrTokenNotFound, got %v", err)
		}

		tokens, err := repo.ListTokens(ctx, newInitiator())
		if err != nil {
			t.Fatal(err)
		}
		if len(tokens) != 1 || tokens[0].Token != "contract-secret" {
			t.Fatalf("expected the stored token for its initiator, got %v", tokens)
		}
		if tokens, err = repo.ListTokens(ctx, &userpb.UserId{OpaqueId: "other-initiator", Idp: "local.example.com"}); err != nil {
			t.Fatal(err)
		}
		if len(tokens) != 0 {
			t.Fatalf("tokens leaked across initiators, got %d", len(tokens))
		}

		if err := repo.AddToken(ctx, newToken("contract-secret-2", 24*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if tokens, err = repo.ListTokens(ctx, newInitiator()); err != nil {
			t.Fatal(err)
		}
		if len(tokens) != 2 {
			t.Fatalf("expected 2 stored tokens, got %d", len(tokens))
		}
	})
}
