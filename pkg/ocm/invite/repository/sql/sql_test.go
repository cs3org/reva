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

package sql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
	"github.com/cs3org/reva/v3/pkg/ocm/invite/repository/internal/contracttest"
)

// newSQLiteRepository returns a repository backed by a fresh SQLite database
// and closes the underlying connection when the test finishes.
func newSQLiteRepository(t *testing.T) invite.Repository {
	t.Helper()
	repo, err := New(context.Background(), map[string]any{
		"db_engine": "sqlite",
		"db_name":   filepath.Join(t.TempDir(), "invites.db"),
	})
	if err != nil {
		t.Fatalf("sql: error creating repository: %v", err)
	}
	sqlDB, err := repo.(*mgr).db.DB()
	if err != nil {
		t.Fatalf("sql: error obtaining the underlying connection: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("sql: error closing the test database: %v", err)
		}
	})
	return repo
}

func TestRepositoryContract(t *testing.T) {
	contracttest.Run(t, newSQLiteRepository)
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

func coreValidToken(secret string, expiration time.Duration) *invitepb.InviteToken {
	return &invitepb.InviteToken{
		Token:       secret,
		UserId:      newInitiatorID(),
		Expiration:  &typespb.Timestamp{Seconds: uint64(time.Now().Add(expiration).Unix())},
		Description: "sql test token",
	}
}

// TestInvalidInputSkipsDatabase proves validation runs before any query or
// write is constructed: a manager with a nil database rejects invalid input
// without dereferencing it.
func TestInvalidInputSkipsDatabase(t *testing.T) {
	repo := &mgr{c: &Config{}}
	ctx := context.Background()
	initiator := newInitiatorID()
	remoteID := &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}
	remoteUser := newRemoteUser("alice", "one.example.com")

	assertBadRequest := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected errtypes.BadRequest, got nil")
		}
		if _, ok := err.(errtypes.BadRequest); !ok {
			t.Fatalf("expected errtypes.BadRequest, got %T: %v", err, err)
		}
	}

	assertBadRequest(t, repo.AddToken(ctx, nil))
	assertBadRequest(t, repo.AddToken(ctx, &invitepb.InviteToken{}))
	assertBadRequest(t, repo.AddToken(ctx, &invitepb.InviteToken{Token: "secret"}))

	if _, err := repo.GetToken(ctx, ""); err == nil {
		t.Fatal("expected an error for a blank token secret")
	} else if _, ok := err.(errtypes.BadRequest); !ok {
		t.Fatalf("expected errtypes.BadRequest, got %T: %v", err, err)
	}

	if tokens, err := repo.ListTokens(ctx, nil); err == nil {
		t.Fatal("expected an error for a nil initiator")
	} else if _, ok := err.(errtypes.BadRequest); !ok {
		t.Fatalf("expected errtypes.BadRequest, got %T: %v", err, err)
	} else if tokens != nil {
		t.Fatalf("expected a nil token list, got %v", tokens)
	}

	assertBadRequest(t, repo.AddRemoteUser(ctx, nil, remoteUser))
	assertBadRequest(t, repo.AddRemoteUser(ctx, initiator, nil))
	assertBadRequest(t, repo.AddRemoteUser(ctx, initiator, &userpb.User{}))

	if _, err := repo.GetRemoteUser(ctx, initiator, nil); err == nil {
		t.Fatal("expected an error for a nil remote id")
	} else if _, ok := err.(errtypes.BadRequest); !ok {
		t.Fatalf("expected errtypes.BadRequest, got %T: %v", err, err)
	}
	if _, err := repo.GetRemoteUser(ctx, nil, remoteID); err == nil {
		t.Fatal("expected an error for a nil initiator")
	} else if _, ok := err.(errtypes.BadRequest); !ok {
		t.Fatalf("expected errtypes.BadRequest, got %T: %v", err, err)
	}

	if users, err := repo.FindRemoteUsers(ctx, nil, ""); err == nil {
		t.Fatal("expected an error for a nil initiator")
	} else if _, ok := err.(errtypes.BadRequest); !ok {
		t.Fatalf("expected errtypes.BadRequest, got %T: %v", err, err)
	} else if users != nil {
		t.Fatalf("expected a nil user list, got %v", users)
	}

	assertBadRequest(t, repo.DeleteRemoteUser(ctx, initiator, nil))
	assertBadRequest(t, repo.DeleteRemoteUser(ctx, nil, remoteID))
}

// TestAddTokenWithoutExpirationNotSupported keeps the documented rejection of
// non-expiring invitations instead of encoding a nil expiry as a timestamp.
func TestAddTokenWithoutExpirationNotSupported(t *testing.T) {
	repo := &mgr{c: &Config{}}
	token := &invitepb.InviteToken{
		Token:  "no-expiry-secret",
		UserId: newInitiatorID(),
	}

	err := repo.AddToken(context.Background(), token)
	if err == nil {
		t.Fatal("expected errtypes.NotSupported, got nil")
	}
	if _, ok := err.(errtypes.NotSupported); !ok {
		t.Fatalf("expected errtypes.NotSupported, got %T: %v", err, err)
	}
	if want := "non-expiring invitations are not supported by the sql repository"; err.Error() != "error: not supported: "+want {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestExactIdpMatching keeps the driver's exact-Idp semantics, including a
// genuine empty-Idp stored row: an empty query idp matches only empty rows.
func TestExactIdpMatching(t *testing.T) {
	repo := newSQLiteRepository(t)
	ctx := context.Background()
	initiator := newInitiatorID()
	one := newRemoteUser("alice", "one.example.com")
	empty := newRemoteUser("bob", "")

	if err := repo.AddRemoteUser(ctx, initiator, one); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddRemoteUser(ctx, initiator, empty); err != nil {
		t.Fatalf("a user with an empty idp must be storable: %v", err)
	}

	if _, err := repo.GetRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: "two.example.com"}); err == nil {
		t.Fatal("expected errtypes.NotFound for a non-matching idp, got nil")
	} else if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("expected errtypes.NotFound for a non-matching idp, got %T %v", err, err)
	}
	if _, err := repo.GetRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: ""}); err == nil {
		t.Fatal("empty idp must not wildcard-match a stored idp, got nil")
	} else if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("empty idp must not wildcard-match a stored idp, got %T %v", err, err)
	}
	if got, err := repo.GetRemoteUser(ctx, initiator, one.Id); err != nil {
		t.Fatalf("exact idp lookup failed: %v", err)
	} else if got.Id.GetIdp() != "one.example.com" {
		t.Fatalf("unexpected user: %+v", got)
	}
	if got, err := repo.GetRemoteUser(ctx, initiator, empty.Id); err != nil {
		t.Fatalf("empty stored idp must be exactly retrievable: %v", err)
	} else if got.Id.GetIdp() != "" {
		t.Fatalf("unexpected user: %+v", got)
	}

	// an empty query idp only deletes the empty-idp row
	if err := repo.DeleteRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetRemoteUser(ctx, initiator, one.Id); err != nil {
		t.Fatalf("empty idp delete must not remove a stored idp row: %v", err)
	}
	if err := repo.DeleteRemoteUser(ctx, initiator, empty.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetRemoteUser(ctx, initiator, empty.Id); err == nil {
		t.Fatal("expected the empty-idp row to be deleted, got nil")
	} else if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("expected the empty-idp row to be deleted, got %T %v", err, err)
	}
}

// TestSoftDeleteRevival keeps the unique-key and revival semantics of the
// driver on top of the shared contract suite.
func TestSoftDeleteRevival(t *testing.T) {
	repo := newSQLiteRepository(t)
	ctx := context.Background()
	initiator := newInitiatorID()
	alice := newRemoteUser("alice", "one.example.com")

	if err := repo.AddRemoteUser(ctx, initiator, alice); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("alice", "one.example.com")); err != invite.ErrUserAlreadyAccepted {
		t.Fatalf("expected ErrUserAlreadyAccepted, got %v", err)
	}
	if err := repo.DeleteRemoteUser(ctx, initiator, alice.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetRemoteUser(ctx, initiator, alice.Id); err == nil {
		t.Fatal("expected errtypes.NotFound after delete, got nil")
	} else if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("expected errtypes.NotFound after delete, got %T %v", err, err)
	}
	// the soft-deleted row is revived rather than duplicated
	if err := repo.AddRemoteUser(ctx, initiator, alice); err != nil {
		t.Fatalf("re-adding a soft-deleted user must revive it: %v", err)
	}
	users, err := repo.FindRemoteUsers(ctx, initiator, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("expected a single revived row, got %d", len(users))
	}
	if err := repo.AddRemoteUser(ctx, initiator, newRemoteUser("alice", "one.example.com")); err != invite.ErrUserAlreadyAccepted {
		t.Fatalf("expected ErrUserAlreadyAccepted after revival, got %v", err)
	}
}
