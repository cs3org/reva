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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
	"github.com/cs3org/reva/v3/pkg/ocm/invite/repository/internal/contracttest"
	"github.com/rs/zerolog"
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

func ctxWithCapturedWarnLog(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.WarnLevel)
	return appctx.WithLogger(context.Background(), &logger), &buf
}

func assertAtMostOneWarning(t *testing.T, log string, msg string) {
	t.Helper()
	if c := strings.Count(log, msg); c > 1 {
		t.Fatalf("expected at most one %q warning, got %d in %q", msg, c, log)
	}
}

func assertLogOmitsSentinels(t *testing.T, log string, sentinels ...string) {
	t.Helper()
	for _, s := range sentinels {
		if strings.Contains(log, s) {
			t.Fatalf("log must not contain %q, got %q", s, log)
		}
	}
}

// TestMalformedStoredInviteTokenMatrix covers corrupt token rows with a valid
// neighbor: invalid lookups fail closed, lists stay filtered, reads are stable.
func TestMalformedStoredInviteTokenMatrix(t *testing.T) {
	file := filepath.Join(t.TempDir(), "invites.json")
	mustWriteFile(t, file, `{
		"invites": {
			"good": {"token": "good", "user_id": {"idp": "local.example.com", "opaque_id": "initiator"}, "expiration": {"seconds": 32503680000}},
			"null-row": null,
			"missing-owner": {"token": "missing-owner", "expiration": {"seconds": 32503680000}},
			"empty-owner": {"token": "empty-owner", "user_id": {"opaque_id": "   ", "idp": "local.example.com"}, "expiration": {"seconds": 32503680000}},
			"blank-secret-key": {"token": "", "user_id": {"idp": "local.example.com", "opaque_id": "initiator"}, "expiration": {"seconds": 32503680000}}
		},
		"accepted_users": {}
	}`)
	repo := newRepositoryAtFile(t, file)
	ctx := context.Background()
	initiator := newInitiatorID()
	before := mustReadFile(t, file)

	for _, lookupKey := range []string{"null-row", "missing-owner", "empty-owner", "blank-secret-key"} {
		tkn, err := repo.GetToken(ctx, lookupKey)
		if tkn != nil {
			t.Fatalf("%s: expected nil token, got %+v", lookupKey, tkn)
		}
		if _, ok := err.(errtypes.InternalError); !ok {
			t.Fatalf("%s: expected InternalError, got %T: %v", lookupKey, err, err)
		}
	}

	tokens, err := repo.ListTokens(ctx, initiator)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].Token != "good" {
		t.Fatalf("expected only the valid token, got %v", tokens)
	}

	if after := mustReadFile(t, file); after != before {
		t.Fatal("read-only operations must not change file bytes")
	}
	reloaded := newRepositoryAtFile(t, file)
	if afterReload := mustReadFile(t, file); afterReload != before {
		t.Fatal("reopen must not change file bytes")
	}
	if tokens, err = reloaded.ListTokens(ctx, initiator); err != nil || len(tokens) != 1 {
		t.Fatalf("reloaded list mismatch: tokens=%v err=%v", tokens, err)
	}
}

// TestJSONSkippedRowDiagnostics emits at most one fixed warning per scan.
func TestJSONSkippedRowDiagnostics(t *testing.T) {
	file := filepath.Join(t.TempDir(), "invites.json")
	mustWriteFile(t, file, `{
		"invites": {
			"bad-a": null,
			"bad-b": {"token": "bad-b", "expiration": {"seconds": 32503680000}},
			"good": {"token": "good", "user_id": {"idp": "local.example.com", "opaque_id": "initiator"}, "expiration": {"seconds": 32503680000}}
		},
		"accepted_users": {
			"initiator": [
				null,
				{"username": "no-id"},
				{"id": {"opaque_id": "   ", "idp": "one.example.com"}, "username": "blank-id"},
				{"id": {"opaque_id": "good-u", "idp": "one.example.com"}, "username": "good-u", "display_name": "Display good-u", "mail": "good-u@one.example.com"}
			]
		}
	}`)
	repo := newRepositoryAtFile(t, file)
	initiator := newInitiatorID()
	goodUserID := &userpb.UserId{OpaqueId: "good-u", Idp: "one.example.com"}

	ctx, logBuf := ctxWithCapturedWarnLog(t)
	tokens, err := repo.ListTokens(ctx, initiator)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("list tokens: %v %v", tokens, err)
	}
	assertAtMostOneWarning(t, logBuf.String(), "skipping malformed stored invite token")
	logBuf.Reset()

	_, err = repo.FindRemoteUsers(ctx, initiator, "")
	if err != nil {
		t.Fatal(err)
	}
	assertAtMostOneWarning(t, logBuf.String(), "skipping malformed stored remote user")
	assertLogOmitsSentinels(t, logBuf.String(), "good-u", "no-id", "blank-id")
	logBuf.Reset()

	_, err = repo.GetRemoteUser(ctx, initiator, goodUserID)
	if err != nil {
		t.Fatal(err)
	}
	assertAtMostOneWarning(t, logBuf.String(), "skipping malformed stored remote user")
	logBuf.Reset()

	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("good-u", "one.example.com")); !errors.Is(err, invite.ErrUserAlreadyAccepted) {
		t.Fatalf("expected duplicate against valid row, got %v", err)
	}
	assertAtMostOneWarning(t, logBuf.String(), "skipping malformed stored remote user")
	logBuf.Reset()

	_ = repo.DeleteRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "ghost", Idp: "ghost.example.com"})
	assertAtMostOneWarning(t, logBuf.String(), "skipping malformed stored remote user")
}

type acceptedUserKey struct {
	opaque string
	idp    string
}

type liveModelSnapshot struct {
	invites       map[string]string
	acceptedUsers map[string][]acceptedUserKey
}

func managerFromRepo(t *testing.T, repo invite.Repository) *manager {
	t.Helper()
	m, ok := repo.(*manager)
	if !ok {
		t.Fatal("expected *manager repository")
	}
	return m
}

func snapshotLiveModel(t *testing.T, m *manager) liveModelSnapshot {
	t.Helper()
	snap := liveModelSnapshot{
		invites:       make(map[string]string, len(m.model.Invites)),
		acceptedUsers: make(map[string][]acceptedUserKey, len(m.model.AcceptedUsers)),
	}
	for key, token := range m.model.Invites {
		if token == nil {
			snap.invites[key] = ""
			continue
		}
		snap.invites[key] = token.GetToken()
	}
	for initiator, users := range m.model.AcceptedUsers {
		order := make([]acceptedUserKey, len(users))
		for i, user := range users {
			if user == nil || user.Id == nil {
				continue
			}
			order[i] = acceptedUserKey{opaque: user.Id.GetOpaqueId(), idp: user.Id.GetIdp()}
		}
		snap.acceptedUsers[initiator] = order
	}
	return snap
}

func assertLiveSnapshotsEqual(t *testing.T, before, after liveModelSnapshot) {
	t.Helper()
	if len(before.invites) != len(after.invites) {
		t.Fatalf("invite map size changed: before=%d after=%d", len(before.invites), len(after.invites))
	}
	for key, beforeSecret := range before.invites {
		afterSecret, ok := after.invites[key]
		if !ok {
			t.Fatalf("invite key %q disappeared from live model", key)
		}
		if afterSecret != beforeSecret {
			t.Fatalf("invite %q changed: before=%q after=%q", key, beforeSecret, afterSecret)
		}
	}
	for key, beforeUsers := range before.acceptedUsers {
		afterUsers, ok := after.acceptedUsers[key]
		if !ok {
			t.Fatalf("accepted-user key %q disappeared from live model", key)
		}
		if len(beforeUsers) != len(afterUsers) {
			t.Fatalf("accepted-user list length changed for %q: before=%d after=%d", key, len(beforeUsers), len(afterUsers))
		}
		for i := range beforeUsers {
			if beforeUsers[i] != afterUsers[i] {
				t.Fatalf("accepted-user order changed for %q at %d: before=%+v after=%+v", key, i, beforeUsers[i], afterUsers[i])
			}
		}
	}
}

func assertNoInviteTempFiles(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".ocm-invites-*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) > 0 {
		t.Fatalf("expected no temporary invite files, found %v", matches)
	}
}

func blockJSONSaveAtConfiguredPath(t *testing.T, configuredPath string) (savedPath string, restore func()) {
	t.Helper()
	saved := configuredPath + ".saved"
	if err := os.Rename(configuredPath, saved); err != nil {
		t.Fatalf("rename file aside: %v", err)
	}
	if err := os.Mkdir(configuredPath, 0700); err != nil {
		t.Fatalf("create blocking directory: %v", err)
	}
	return saved, func() {
		if err := os.RemoveAll(configuredPath); err != nil {
			t.Fatalf("remove blocking directory: %v", err)
		}
		if err := os.Rename(saved, configuredPath); err != nil {
			t.Fatalf("restore saved file: %v", err)
		}
	}
}

func seedJSONMutationRepository(t *testing.T) (invite.Repository, context.Context, string, *userpb.UserId) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "invites.json")
	mustWriteFile(t, file, `{
		"invites": {
			"seed-token": {
				"token": "seed-token",
				"user_id": {"idp": "local.example.com", "opaque_id": "initiator"},
				"expiration": {"seconds": 32503680000}
			}
		},
		"accepted_users": {
			"initiator": [
				null,
				{"id": {"opaque_id": "alice", "idp": "one.example.com"}, "username": "alice", "display_name": "Display alice", "mail": "alice@one.example.com"},
				{"id": {"opaque_id": "bob", "idp": "one.example.com"}, "username": "bob", "display_name": "Display bob", "mail": "bob@one.example.com"}
			],
			"other-initiator": [
				{"id": {"opaque_id": "carol", "idp": "two.example.com"}, "username": "carol", "display_name": "Display carol", "mail": "carol@two.example.com"}
			]
		}
	}`)
	repo := newRepositoryAtFile(t, file)
	return repo, context.Background(), file, newInitiatorID()
}

func assertReopenMatchesLiveModel(t *testing.T, live invite.Repository, file string, ctx context.Context, initiator *userpb.UserId) {
	t.Helper()
	reloaded := newRepositoryAtFile(t, file)

	liveTokens, err := live.ListTokens(ctx, initiator)
	if err != nil {
		t.Fatalf("live list tokens: %v", err)
	}
	reloadedTokens, err := reloaded.ListTokens(ctx, initiator)
	if err != nil {
		t.Fatalf("reloaded list tokens: %v", err)
	}
	if len(liveTokens) != len(reloadedTokens) {
		t.Fatalf("token count mismatch: live=%d reloaded=%d", len(liveTokens), len(reloadedTokens))
	}
	liveSet := map[string]struct{}{}
	for _, token := range liveTokens {
		liveSet[token.GetToken()] = struct{}{}
	}
	for _, token := range reloadedTokens {
		if _, ok := liveSet[token.GetToken()]; !ok {
			t.Fatalf("reloaded token %q missing from live model", token.GetToken())
		}
	}

	liveUsers, err := live.FindRemoteUsers(ctx, initiator, "")
	if err != nil {
		t.Fatalf("live find users: %v", err)
	}
	reloadedUsers, err := reloaded.FindRemoteUsers(ctx, initiator, "")
	if err != nil {
		t.Fatalf("reloaded find users: %v", err)
	}
	if len(liveUsers) != len(reloadedUsers) {
		t.Fatalf("accepted-user count mismatch: live=%d reloaded=%d", len(liveUsers), len(reloadedUsers))
	}
	liveUsersSet := map[acceptedUserKey]struct{}{}
	for _, user := range liveUsers {
		liveUsersSet[acceptedUserKey{user.Id.GetOpaqueId(), user.Id.GetIdp()}] = struct{}{}
	}
	for _, user := range reloadedUsers {
		key := acceptedUserKey{user.Id.GetOpaqueId(), user.Id.GetIdp()}
		if _, ok := liveUsersSet[key]; !ok {
			t.Fatalf("reloaded user %+v missing from live model", key)
		}
	}
}

func assertSaveFailureRollback(t *testing.T, repo invite.Repository, file string, ctx context.Context, initiator *userpb.UserId, beforeBytes string, beforeSnap liveModelSnapshot, mutate func() error) {
	t.Helper()
	mgr := managerFromRepo(t, repo)
	savedPath, restore := blockJSONSaveAtConfiguredPath(t, file)

	if err := mutate(); err == nil {
		t.Fatal("expected persistence failure, got nil")
	} else if !strings.Contains(err.Error(), "json: error saving model") {
		t.Fatalf("expected wrapped save error, got %v", err)
	}

	afterSnap := snapshotLiveModel(t, mgr)
	assertLiveSnapshotsEqual(t, beforeSnap, afterSnap)
	if afterBytes := mustReadFile(t, savedPath); afterBytes != beforeBytes {
		t.Fatal("file bytes changed after failed persistence")
	}
	assertNoInviteTempFiles(t, filepath.Dir(file))

	restore()
	assertReopenMatchesLiveModel(t, repo, file, ctx, initiator)
}

func TestJSONMutationSaveFailureLeavesStateUnchanged(t *testing.T) {
	t.Run("AddToken", func(t *testing.T) {
		repo, ctx, file, initiator := seedJSONMutationRepository(t)
		mgr := managerFromRepo(t, repo)
		beforeBytes := mustReadFile(t, file)
		beforeSnap := snapshotLiveModel(t, mgr)

		assertSaveFailureRollback(t, repo, file, ctx, initiator, beforeBytes, beforeSnap, func() error {
			return repo.AddToken(ctx, &invitepb.InviteToken{
				Token:      "new-token",
				UserId:     initiator,
				Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(24 * time.Hour).Unix())},
			})
		})
	})

	t.Run("token replacement", func(t *testing.T) {
		repo, ctx, file, initiator := seedJSONMutationRepository(t)
		mgr := managerFromRepo(t, repo)
		beforeBytes := mustReadFile(t, file)
		beforeSnap := snapshotLiveModel(t, mgr)

		assertSaveFailureRollback(t, repo, file, ctx, initiator, beforeBytes, beforeSnap, func() error {
			return repo.AddToken(ctx, &invitepb.InviteToken{
				Token:      "seed-token",
				UserId:     initiator,
				Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(48 * time.Hour).Unix())},
			})
		})

		if tkn, err := repo.GetToken(ctx, "seed-token"); err != nil {
			t.Fatalf("seed token lookup failed: %v", err)
		} else if tkn.GetExpiration().GetSeconds() != 32503680000 {
			t.Fatalf("token replacement must not publish on failed save, expiration=%d", tkn.GetExpiration().GetSeconds())
		}
	})

	t.Run("AddRemoteUser", func(t *testing.T) {
		repo, ctx, file, initiator := seedJSONMutationRepository(t)
		mgr := managerFromRepo(t, repo)
		beforeBytes := mustReadFile(t, file)
		beforeSnap := snapshotLiveModel(t, mgr)

		assertSaveFailureRollback(t, repo, file, ctx, initiator, beforeBytes, beforeSnap, func() error {
			return repo.AddRemoteUser(ctx, initiator, newRemoteUser("dave", "one.example.com"))
		})
	})

	t.Run("DeleteRemoteUser", func(t *testing.T) {
		repo, ctx, file, initiator := seedJSONMutationRepository(t)
		mgr := managerFromRepo(t, repo)
		beforeBytes := mustReadFile(t, file)
		beforeSnap := snapshotLiveModel(t, mgr)
		aliceID := &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}

		assertSaveFailureRollback(t, repo, file, ctx, initiator, beforeBytes, beforeSnap, func() error {
			return repo.DeleteRemoteUser(ctx, initiator, aliceID)
		})

		if _, err := repo.GetRemoteUser(ctx, initiator, aliceID); err != nil {
			t.Fatalf("deleted row must remain available after failed save: %v", err)
		}
	})
}

func TestJSONSuccessfulMutationsSurviveReopen(t *testing.T) {
	repo, ctx, file, initiator := seedJSONMutationRepository(t)
	otherInitiator := &userpb.UserId{OpaqueId: "other-initiator", Idp: "local.example.com", Type: userpb.UserType_USER_TYPE_PRIMARY}

	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("dave", "one.example.com")); err != nil {
		t.Fatalf("add remote user: %v", err)
	}
	assertReopenMatchesLiveModel(t, repo, file, ctx, initiator)

	if err := repo.AddToken(ctx, &invitepb.InviteToken{
		Token:      "persisted-token",
		UserId:     initiator,
		Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(24 * time.Hour).Unix())},
	}); err != nil {
		t.Fatalf("add token: %v", err)
	}
	assertReopenMatchesLiveModel(t, repo, file, ctx, initiator)

	if err := repo.DeleteRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "bob", Idp: "one.example.com"}); err != nil {
		t.Fatalf("delete remote user: %v", err)
	}
	assertReopenMatchesLiveModel(t, repo, file, ctx, initiator)

	if users, err := repo.FindRemoteUsers(ctx, otherInitiator, ""); err != nil || len(users) != 1 || users[0].Id.GetOpaqueId() != "carol" {
		t.Fatalf("other initiator data lost: users=%v err=%v", users, err)
	}
}

func TestJSONNoOpDeleteLeavesBytesUnchanged(t *testing.T) {
	repo, ctx, file, initiator := seedJSONMutationRepository(t)
	before := mustReadFile(t, file)

	if err := repo.DeleteRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "ghost", Idp: "ghost.example.com"}); err != nil {
		t.Fatalf("no-op delete must succeed: %v", err)
	}
	if after := mustReadFile(t, file); after != before {
		t.Fatal("no-op delete must not rewrite the file")
	}
	assertNoInviteTempFiles(t, filepath.Dir(file))
}

func TestJSONDeleteRollbackSpareSliceCapacity(t *testing.T) {
	repo, ctx, file, initiator := seedJSONMutationRepository(t)
	mgr := managerFromRepo(t, repo)
	key := initiator.GetOpaqueId()

	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("erin", "one.example.com")); err != nil {
		t.Fatalf("seed erin: %v", err)
	}

	valid := make([]*userpb.User, 0, 8)
	for _, user := range mgr.model.AcceptedUsers[key] {
		if invite.ValidateRemoteUser(user) == nil {
			valid = append(valid, user)
		}
	}
	users := make([]*userpb.User, len(valid), 8)
	copy(users, valid)
	mgr.model.AcceptedUsers[key] = users

	beforeSnap := snapshotLiveModel(t, mgr)
	beforeBytes := mustReadFile(t, file)
	savedPath, restore := blockJSONSaveAtConfiguredPath(t, file)
	if err := repo.DeleteRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "bob", Idp: "one.example.com"}); err == nil {
		t.Fatal("expected persistence failure, got nil")
	} else if !strings.Contains(err.Error(), "json: error saving model") {
		t.Fatalf("expected wrapped save error, got %v", err)
	}

	assertLiveSnapshotsEqual(t, beforeSnap, snapshotLiveModel(t, mgr))
	if afterBytes := mustReadFile(t, savedPath); afterBytes != beforeBytes {
		t.Fatal("file bytes changed after failed persistence")
	}
	assertNoInviteTempFiles(t, filepath.Dir(file))
	restore()

	liveUsers := mgr.model.AcceptedUsers[key]
	if len(liveUsers) != 3 {
		t.Fatalf("live slice length changed during failed delete: got %d", len(liveUsers))
	}
	if liveUsers[1].Id.GetOpaqueId() != "bob" {
		t.Fatalf("swap-with-last mutated live backing array: index 1 is %q", liveUsers[1].Id.GetOpaqueId())
	}
}

func TestJSONSavePreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "invites.json")
	mustWriteFile(t, file, `{}`)
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}

	repo := newRepositoryAtFile(t, file)
	ctx := context.Background()
	initiator := newInitiatorID()
	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("alice", "one.example.com")); err != nil {
		t.Fatalf("add remote user: %v", err)
	}

	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected mode 0600 after save, got %o", info.Mode().Perm())
	}
}

func TestJSONSaveThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "invites-target.json")
	link := filepath.Join(dir, "invites.json")
	mustWriteFile(t, target, `{}`)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	repo := newRepositoryAtFile(t, link)
	ctx := context.Background()
	initiator := newInitiatorID()
	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("alice", "one.example.com")); err != nil {
		t.Fatalf("add through symlink: %v", err)
	}

	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if linkInfo.Mode()&fs.ModeSymlink == 0 {
		t.Fatal("configured path must remain a symlink")
	}

	targetData, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(targetData), "alice") {
		t.Fatalf("symlink target was not updated: %q", string(targetData))
	}

	reloaded := newRepositoryAtFile(t, link)
	if _, err := reloaded.GetRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}); err != nil {
		t.Fatalf("reopen through symlink failed: %v", err)
	}
	assertNoInviteTempFiles(t, dir)
}

func TestJSONConcurrentReadWrite(t *testing.T) {
	repo := newJSONRepository(t)
	ctx := context.Background()
	initiator := newInitiatorID()
	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("alice", "one.example.com")); err != nil {
		t.Fatal(err)
	}

	const workers = 16
	var wg sync.WaitGroup
	errCh := make(chan error, workers*3)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := repo.GetRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}); err != nil {
				if _, ok := err.(errtypes.NotFound); !ok {
					errCh <- err
				}
			}
			if _, err := repo.FindRemoteUsers(ctx, initiator, ""); err != nil {
				errCh <- err
			}
			if _, err := repo.ListTokens(ctx, initiator); err != nil {
				errCh <- err
			}
		}(i)

		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			user := newRemoteUser(fmt.Sprintf("worker-%d", i), "one.example.com")
			if err := repo.AddRemoteUser(ctx, initiator, user); err != nil {
				if !errors.Is(err, invite.ErrUserAlreadyAccepted) {
					errCh <- err
				}
			}
		}(i)

		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := repo.AddToken(ctx, &invitepb.InviteToken{
				Token:      fmt.Sprintf("token-%d", i),
				UserId:     initiator,
				Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(24 * time.Hour).Unix())},
			}); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent operation failed: %v", err)
		}
	}

	users, err := repo.FindRemoteUsers(ctx, initiator, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range users {
		if invite.ValidateRemoteUser(user) != nil {
			t.Fatalf("observed invalid user in concurrent reads: %+v", user)
		}
	}
	tokens, err := repo.ListTokens(ctx, initiator)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens {
		if invite.ValidateInviteToken(token) != nil {
			t.Fatalf("observed invalid token in concurrent reads: %+v", token)
		}
	}
}
