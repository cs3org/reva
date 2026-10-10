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

package json

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
	"github.com/cs3org/reva/v3/pkg/ocm/invite/repository/internal/contracttest"
)

func newJSONRepository(t *testing.T) invite.Repository {
	t.Helper()
	repo, err := New(context.Background(), map[string]any{
		"file": filepath.Join(t.TempDir(), "invites.json"),
	})
	if err != nil {
		t.Fatalf("json: error creating repository: %v", err)
	}
	return repo
}

func TestRepositoryContract(t *testing.T) {
	contracttest.Run(t, newJSONRepository)
}

func newInitiatorID() *userpb.UserId {
	return &userpb.UserId{OpaqueId: "initiator", Idp: "local.example.com", Type: userpb.UserType_USER_TYPE_PRIMARY}
}

func newRemoteUser(opaqueID, idp string) *userpb.User {
	return &userpb.User{
		Id:          &userpb.UserId{OpaqueId: opaqueID, Idp: idp, Type: userpb.UserType_USER_TYPE_FEDERATED},
		Username:    opaqueID,
		DisplayName: "Display " + opaqueID,
		Mail:        opaqueID + "@" + idp,
	}
}

func newRepositoryAtFile(t *testing.T, file string) invite.Repository {
	t.Helper()
	repo, err := New(context.Background(), map[string]any{"file": file})
	if err != nil {
		t.Fatalf("json: error creating repository: %v", err)
	}
	return repo
}

func mustWriteFile(t *testing.T, file, content string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatalf("json: error writing fixture file: %v", err)
	}
}

func mustReadFile(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("json: error reading file: %v", err)
	}
	return string(data)
}

// TestNullTokenRowProvesBehavior guards a JSON null token row: the lookup must
// never return success for it and must not break the valid neighbor.
func TestNullTokenRow(t *testing.T) {
	file := filepath.Join(t.TempDir(), "invites.json")
	mustWriteFile(t, file, `{
		"invites": {
			"good": {"token": "good", "user_id": {"idp": "local.example.com", "opaque_id": "initiator"}, "expiration": {"seconds": 32503680000}},
			"null-row": null
		},
		"accepted_users": {}
	}`)
	repo := newRepositoryAtFile(t, file)
	ctx := context.Background()
	initiator := newInitiatorID()

	tkn, err := repo.GetToken(ctx, "null-row")
	if tkn != nil {
		t.Fatalf("expected no token for a null row, got %+v", tkn)
	}
	if err == nil {
		t.Fatal("expected an error for a null token row, got nil")
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Fatalf("expected errtypes.InternalError, got %T: %v", err, err)
	}

	// the valid neighbor remains usable
	if tkn, err = repo.GetToken(ctx, "good"); err != nil {
		t.Fatalf("valid neighbor token unusable: %v", err)
	}
	if tkn.Token != "good" {
		t.Fatalf("unexpected token: %+v", tkn)
	}

	// enumeration skips the malformed row
	tokens, err := repo.ListTokens(ctx, initiator)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].Token != "good" {
		t.Fatalf("expected only the valid token to be listed, got %v", tokens)
	}
}

// TestMalformedUserRowsSkipped proves that corrupt accepted-user rows are
// skipped during scans without breaking valid neighbors, and that read-only
// handling leaves the file untouched.
func TestMalformedUserRowsSkipped(t *testing.T) {
	file := filepath.Join(t.TempDir(), "invites.json")
	mustWriteFile(t, file, `{
		"invites": {},
		"accepted_users": {
			"initiator": [
				null,
				{"username": "no-id"},
				{"id": {"idp": "one.example.com", "opaque_id": "   "}, "username": "blank-id"},
				{"id": {"idp": "one.example.com", "opaque_id": "alice"}, "username": "alice", "display_name": "Display alice", "mail": "alice@one.example.com"}
			]
		}
	}`)
	repo := newRepositoryAtFile(t, file)
	ctx := context.Background()
	initiator := newInitiatorID()
	before := mustReadFile(t, file)

	aliceID := &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}

	got, err := repo.GetRemoteUser(ctx, initiator, aliceID)
	if err != nil {
		t.Fatalf("valid neighbor row unusable: %v", err)
	}
	if got.Id.GetOpaqueId() != "alice" {
		t.Fatalf("unexpected user: %+v", got)
	}

	users, err := repo.FindRemoteUsers(ctx, initiator, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("expected only the valid row to be listed, got %d", len(users))
	}

	// the malformed rows must not be reported as duplicates of anything:
	// the rejected duplicate performs no write
	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("alice", "one.example.com")); !errors.Is(err, invite.ErrUserAlreadyAccepted) {
		t.Fatalf("expected duplicate detection against the valid row only, got %v", err)
	}
	if after := mustReadFile(t, file); after != before {
		t.Fatal("the rejected duplicate must not rewrite the file")
	}
	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("no-id", "one.example.com")); err != nil {
		t.Fatalf("malformed row must not block a new valid user: %v", err)
	}

	// deleting the valid row keeps the malformed rows on disk
	if err := repo.DeleteRemoteUser(ctx, initiator, aliceID); err != nil {
		t.Fatal(err)
	}
	after := mustReadFile(t, file)
	for _, needle := range []string{"no-id", "blank-id"} {
		if !strings.Contains(after, needle) {
			t.Fatalf("malformed row %q was removed from the file", needle)
		}
	}
	if _, err := repo.GetRemoteUser(ctx, initiator, aliceID); err == nil {
		t.Fatal("expected errtypes.NotFound after delete, got nil")
	} else if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("expected errtypes.NotFound after delete, got %T %v", err, err)
	}
}

// TestInvalidInputLeavesFileUnchanged proves rejected caller input performs no
// write and leaves both the file and the model unchanged.
func TestInvalidInputLeavesFileUnchanged(t *testing.T) {
	file := filepath.Join(t.TempDir(), "invites.json")
	repo := newRepositoryAtFile(t, file)
	ctx := context.Background()
	initiator := newInitiatorID()

	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("alice", "one.example.com")); err != nil {
		t.Fatal(err)
	}
	before := mustReadFile(t, file)

	if err := repo.AddRemoteUser(ctx, nil, newRemoteUser("bob", "one.example.com")); err == nil {
		t.Fatal("expected an error for a nil initiator")
	}
	if err := repo.AddRemoteUser(ctx, initiator, nil); err == nil {
		t.Fatal("expected an error for a nil remote user")
	}
	if _, err := repo.GetRemoteUser(ctx, initiator, nil); err == nil {
		t.Fatal("expected an error for a nil remote id")
	}
	if err := repo.DeleteRemoteUser(ctx, initiator, nil); err == nil {
		t.Fatal("expected an error for a nil remote id")
	}
	if err := repo.AddToken(ctx, nil); err == nil {
		t.Fatal("expected an error for a nil token")
	}
	if _, err := repo.GetToken(ctx, ""); err == nil {
		t.Fatal("expected an error for a blank token secret")
	}
	if _, err := repo.ListTokens(ctx, nil); err == nil {
		t.Fatal("expected an error for a nil initiator")
	}
	if _, err := repo.FindRemoteUsers(ctx, nil, ""); err == nil {
		t.Fatal("expected an error for a nil initiator")
	}

	if after := mustReadFile(t, file); after != before {
		t.Fatal("invalid input must not change the file bytes")
	}
	if _, err := repo.GetRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}); err != nil {
		t.Fatalf("valid record lost after rejected input: %v", err)
	}
}

// TestReloadPreservesRecords proves valid records survive a restart.
func TestReloadPreservesRecords(t *testing.T) {
	file := filepath.Join(t.TempDir(), "invites.json")
	repo := newRepositoryAtFile(t, file)
	ctx := context.Background()
	initiator := newInitiatorID()
	alice := newRemoteUser("alice", "one.example.com")
	token := &invitepb.InviteToken{
		Token:      "reload-secret",
		UserId:     initiator,
		Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(24 * time.Hour).Unix())},
	}

	if err := repo.AddRemoteUser(ctx, initiator, alice); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddToken(ctx, token); err != nil {
		t.Fatal(err)
	}

	reloaded := newRepositoryAtFile(t, file)
	if got, err := reloaded.GetRemoteUser(ctx, initiator, alice.Id); err != nil {
		t.Fatalf("accepted user lost after reload: %v", err)
	} else if got.Id.GetOpaqueId() != "alice" {
		t.Fatalf("unexpected user after reload: %+v", got)
	}
	if tkn, err := reloaded.GetToken(ctx, "reload-secret"); err != nil {
		t.Fatalf("token lost after reload: %v", err)
	} else if tkn.Token != "reload-secret" {
		t.Fatalf("unexpected token after reload: %+v", tkn)
	}
}

// TestWildcardIdpMatching keeps the empty-Idp wildcard semantics of the JSON
// driver for Get and Delete.
func TestWildcardIdpMatching(t *testing.T) {
	repo := newJSONRepository(t)
	ctx := context.Background()
	initiator := newInitiatorID()
	alice := newRemoteUser("alice", "one.example.com")

	if err := repo.AddRemoteUser(ctx, initiator, alice); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: ""})
	if err != nil {
		t.Fatalf("wildcard lookup failed: %v", err)
	}
	if got.Id.GetOpaqueId() != "alice" {
		t.Fatalf("unexpected user: %+v", got)
	}

	if _, err := repo.GetRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: "two.example.com"}); err == nil {
		t.Fatal("expected errtypes.NotFound for a non-matching idp, got nil")
	} else if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("expected errtypes.NotFound for a non-matching idp, got %T %v", err, err)
	}

	// the wildcard idp also applies to deletes
	if err := repo.DeleteRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}); err == nil {
		t.Fatal("expected the wildcard delete to remove the record, got nil")
	} else if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("expected the wildcard delete to remove the record, got %T %v", err, err)
	}
}

// TestTokenWithoutExpirationListed keeps the driver's existing behavior for
// the optional CS3 expiration field: a core-valid token without expiry is
// storable, retrievable and listed as not expired.
func TestTokenWithoutExpirationListed(t *testing.T) {
	repo := newJSONRepository(t)
	ctx := context.Background()
	initiator := newInitiatorID()
	token := &invitepb.InviteToken{
		Token:  "no-expiry-secret",
		UserId: initiator,
	}

	if err := repo.AddToken(ctx, token); err != nil {
		t.Fatalf("token without expiration must stay storable: %v", err)
	}
	if tkn, err := repo.GetToken(ctx, "no-expiry-secret"); err != nil {
		t.Fatalf("token without expiration must stay retrievable: %v", err)
	} else if tkn.Token != "no-expiry-secret" {
		t.Fatalf("unexpected token: %+v", tkn)
	}
	tokens, err := repo.ListTokens(ctx, initiator)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 {
		t.Fatalf("token without expiration must remain listed, got %d", len(tokens))
	}
}

// TestExpiredTokenLookupKept guards the existing expiry policy: an expired
// token is still returned by the secret lookup but excluded from listing.
func TestExpiredTokenLookupKept(t *testing.T) {
	repo := newJSONRepository(t)
	ctx := context.Background()
	initiator := newInitiatorID()
	token := &invitepb.InviteToken{
		Token:      "expired-secret",
		UserId:     initiator,
		Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())},
	}

	if err := repo.AddToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetToken(ctx, "expired-secret"); err != nil {
		t.Fatalf("lookup behavior for expired tokens must be kept: %v", err)
	}
	tokens, err := repo.ListTokens(ctx, initiator)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 0 {
		t.Fatalf("expired token must not be listed, got %d", len(tokens))
	}
}
