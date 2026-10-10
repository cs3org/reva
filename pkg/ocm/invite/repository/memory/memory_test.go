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

package memory

import (
	"context"
	"strconv"
	"sync"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
	"github.com/cs3org/reva/v3/pkg/ocm/invite/repository/internal/contracttest"
)

func newMemoryRepository(t *testing.T) invite.Repository {
	t.Helper()
	repo, err := New(context.Background(), nil)
	if err != nil {
		t.Fatalf("memory: error creating repository: %v", err)
	}
	return repo
}

func TestRepositoryContract(t *testing.T) {
	contracttest.Run(t, newMemoryRepository)
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

// TestFirstInsertWorks guards the first accepted-user insertion on a fresh
// repository: it used to panic on the assertion-before-ok check before any
// user could ever be stored.
func TestFirstInsertWorks(t *testing.T) {
	repo := newMemoryRepository(t)
	ctx := context.Background()
	initiator := newInitiatorID()
	alice := newRemoteUser("alice", "one.example.com")

	if err := repo.AddRemoteUser(ctx, initiator, alice); err != nil {
		t.Fatalf("first insert must succeed: %v", err)
	}
	got, err := repo.GetRemoteUser(ctx, initiator, alice.Id)
	if err != nil {
		t.Fatalf("first insert must be retrievable: %v", err)
	}
	if got.Id.GetOpaqueId() != "alice" {
		t.Fatalf("unexpected user: %+v", got)
	}
}

// TestWildcardIdpMatching keeps the empty-Idp wildcard semantics of the memory
// driver for Get and Delete.
func TestWildcardIdpMatching(t *testing.T) {
	repo := newMemoryRepository(t)
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

	if err := repo.DeleteRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetRemoteUser(ctx, initiator, &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}); err == nil {
		t.Fatal("expected the wildcard delete to remove the record, got nil")
	} else if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("expected the wildcard delete to remove the record, got %T %v", err, err)
	}
}

// TestTokenWithoutExpirationListed keeps the memory driver's behavior for the
// optional CS3 expiration field.
func TestTokenWithoutExpirationListed(t *testing.T) {
	repo := newMemoryRepository(t)
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

// TestConcurrentUniqueAdditions proves parallel inserts under one initiator
// do not lose users.
func TestConcurrentUniqueAdditions(t *testing.T) {
	repo := newMemoryRepository(t)
	ctx := context.Background()
	const goroutines, perGoroutine = 8, 25

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			initiator := newInitiatorID()
			for i := range perGoroutine {
				user := newRemoteUser("user-"+strconv.Itoa(g)+"-"+strconv.Itoa(i), "one.example.com")
				if err := repo.AddRemoteUser(ctx, initiator, user); err != nil {
					t.Errorf("concurrent add failed: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	users, err := repo.FindRemoteUsers(ctx, newInitiatorID(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != goroutines*perGoroutine {
		t.Fatalf("lost users during concurrent additions: got %d, want %d", len(users), goroutines*perGoroutine)
	}
}

// TestConcurrentDuplicateInsertion proves exactly one of many concurrent
// insertions of the same user succeeds.
func TestConcurrentDuplicateInsertion(t *testing.T) {
	repo := newMemoryRepository(t)
	ctx := context.Background()
	const attempts = 16

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		success  int
		dupCount int
	)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.AddRemoteUser(ctx, newInitiatorID(), newRemoteUser("alice", "one.example.com"))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				success++
			case err == invite.ErrUserAlreadyAccepted:
				dupCount++
			default:
				t.Errorf("unexpected error during duplicate insertion: %v", err)
			}
		}()
	}
	wg.Wait()

	if success != 1 {
		t.Fatalf("expected exactly one successful insertion, got %d", success)
	}
	if dupCount != attempts-1 {
		t.Fatalf("expected %d duplicate errors, got %d", attempts-1, dupCount)
	}
	users, err := repo.FindRemoteUsers(ctx, newInitiatorID(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("expected a single stored user, got %d", len(users))
	}
}

// TestConcurrentReadWrite proves lookups and scans can run while writers add
// and delete records without racing or panicking.
func TestConcurrentReadWrite(t *testing.T) {
	repo := newMemoryRepository(t)
	ctx := context.Background()

	// seed one record so readers can also observe a hit
	if err := repo.AddRemoteUser(ctx, newInitiatorID(), newRemoteUser("alice", "one.example.com")); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var writers sync.WaitGroup
	for w := range 4 {
		writers.Add(1)
		go func(w int) {
			defer writers.Done()
			initiator := newInitiatorID()
			for i := range 50 {
				user := newRemoteUser("writer-"+strconv.Itoa(w)+"-"+strconv.Itoa(i), "one.example.com")
				if err := repo.AddRemoteUser(ctx, initiator, user); err != nil {
					t.Errorf("writer add failed: %v", err)
					return
				}
				if err := repo.DeleteRemoteUser(ctx, initiator, user.Id); err != nil {
					t.Errorf("writer delete failed: %v", err)
					return
				}
			}
		}(w)
	}

	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			initiator := newInitiatorID()
			aliceID := &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := repo.GetRemoteUser(ctx, initiator, aliceID); err != nil {
					if _, ok := err.(errtypes.NotFound); !ok {
						t.Errorf("reader lookup failed: %v", err)
						return
					}
				}
				if _, err := repo.FindRemoteUsers(ctx, initiator, ""); err != nil {
					t.Errorf("reader scan failed: %v", err)
					return
				}
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()
}

// injectStoredUser writes directly into the manager's storage to simulate
// rows that predate the validation guards.
func injectStoredUser(t *testing.T, repo invite.Repository, initiator *userpb.UserId, user *userpb.User) {
	t.Helper()
	m, ok := repo.(*manager)
	if !ok {
		t.Fatalf("expected *manager, got %T", repo)
	}
	m.acceptedUsersMu.Lock()
	defer m.acceptedUsersMu.Unlock()
	m.acceptedUsers[initiator.GetOpaqueId()] = append(m.acceptedUsers[initiator.GetOpaqueId()], user)
}

// TestInjectedMalformedStoredUsersSkipped proves Get and Find skip corrupted
// stored rows instead of crashing, while valid neighbors stay usable.
func TestInjectedMalformedStoredUsersSkipped(t *testing.T) {
	repo := newMemoryRepository(t)
	ctx := context.Background()
	initiator := newInitiatorID()
	alice := newRemoteUser("alice", "one.example.com")

	injectStoredUser(t, repo, initiator, nil)
	injectStoredUser(t, repo, initiator, &userpb.User{})
	injectStoredUser(t, repo, initiator, &userpb.User{Id: &userpb.UserId{OpaqueId: "   "}})
	if err := repo.AddRemoteUser(ctx, initiator, alice); err != nil {
		t.Fatal(err)
	}

	if got, err := repo.GetRemoteUser(ctx, initiator, alice.Id); err != nil {
		t.Fatalf("valid neighbor must stay retrievable: %v", err)
	} else if got.Id.GetOpaqueId() != "alice" {
		t.Fatalf("unexpected user: %+v", got)
	}

	found, err := repo.FindRemoteUsers(ctx, initiator, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Id.GetOpaqueId() != "alice" {
		t.Fatalf("expected only the valid user to be enumerated, got %+v", found)
	}

	// malformed rows must not block duplicates or deletes of valid rows
	if err := repo.DeleteRemoteUser(ctx, initiator, alice.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetRemoteUser(ctx, initiator, alice.Id); err == nil {
		t.Fatal("expected the valid user to be deleted, got nil")
	} else if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("expected errtypes.NotFound after delete, got %T %v", err, err)
	}
}

// TestInjectedTypedNilToken proves a corrupted token record surfaces as an
// internal error instead of a nil token.
func TestInjectedTypedNilToken(t *testing.T) {
	repo := newMemoryRepository(t)
	ctx := context.Background()

	m, ok := repo.(*manager)
	if !ok {
		t.Fatalf("expected *manager, got %T", repo)
	}
	m.Invites.Store("corrupt", (*invitepb.InviteToken)(nil))

	if _, err := repo.GetToken(ctx, "corrupt"); err == nil {
		t.Fatal("expected an error for a typed-nil stored token, got nil")
	} else if _, ok := err.(errtypes.InternalError); !ok {
		t.Fatalf("expected errtypes.InternalError, got %T: %v", err, err)
	}

	tokens, err := repo.ListTokens(ctx, newInitiatorID())
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 0 {
		t.Fatalf("expected the malformed token to be skipped, got %+v", tokens)
	}
}
