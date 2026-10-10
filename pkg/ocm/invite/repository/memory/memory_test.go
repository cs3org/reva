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
	"bytes"
	"context"
	stdjson "encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
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

func ctxWithCapturedWarnLog(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.WarnLevel)
	return appctx.WithLogger(context.Background(), &logger), &buf
}

func injectStoredToken(t *testing.T, repo invite.Repository, key string, value any) {
	t.Helper()
	m, ok := repo.(*manager)
	if !ok {
		t.Fatalf("expected *manager, got %T", repo)
	}
	m.Invites.Store(key, value)
}

// TestMemoryMalformedStoredInviteTokenMatrix rejects corrupt stored tokens while
// listing only the valid neighbor.
func TestMemoryMalformedStoredInviteTokenMatrix(t *testing.T) {
	repo := newMemoryRepository(t)
	ctx := context.Background()
	initiator := newInitiatorID()
	valid := &invitepb.InviteToken{
		Token:      "good",
		UserId:     initiator,
		Expiration: &typespb.Timestamp{Seconds: 32503680000},
	}
	if err := repo.AddToken(ctx, valid); err != nil {
		t.Fatal(err)
	}

	injectStoredToken(t, repo, "typed-nil", (*invitepb.InviteToken)(nil))
	injectStoredToken(t, repo, "wrong-type", "not-a-token")
	injectStoredToken(t, repo, "missing-owner", &invitepb.InviteToken{Token: "missing-owner"})
	injectStoredToken(t, repo, "blank-owner", &invitepb.InviteToken{Token: "blank-owner", UserId: &userpb.UserId{OpaqueId: ""}})
	injectStoredToken(t, repo, "whitespace-owner", &invitepb.InviteToken{Token: "whitespace-owner", UserId: &userpb.UserId{OpaqueId: "   "}})
	injectStoredToken(t, repo, "blank-secret", &invitepb.InviteToken{Token: "   ", UserId: initiator})

	for _, key := range []string{"typed-nil", "wrong-type", "missing-owner", "blank-owner", "whitespace-owner", "blank-secret"} {
		token, err := repo.GetToken(ctx, key)
		if token != nil {
			t.Fatalf("%s: expected nil token, got %+v", key, token)
		}
		if err == nil {
			t.Fatalf("%s: expected error, got nil", key)
		} else if _, ok := err.(errtypes.InternalError); !ok {
			t.Fatalf("%s: expected InternalError, got %T: %v", key, err, err)
		}
	}

	tokens, err := repo.ListTokens(ctx, initiator)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].Token != "good" {
		t.Fatalf("expected only the valid token, got %v", tokens)
	}
}

func assertOneSkippedRowWarning(t *testing.T, data, want string) {
	t.Helper()
	decoder := stdjson.NewDecoder(strings.NewReader(data))
	count := 0
	for {
		var entry map[string]any
		err := decoder.Decode(&entry)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal("invalid captured warning JSON")
		}
		if len(entry) != 2 || entry["level"] != "warn" || entry["message"] != want {
			t.Fatal("unexpected skipped-row diagnostic fields")
		}
		count++
	}
	if count != 1 {
		t.Fatalf("expected one skipped-row warning, got %d", count)
	}
}

func assertMemoryBadRequest(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected bad request error, got nil")
	}
	if _, ok := err.(errtypes.BadRequest); !ok {
		t.Fatalf("expected errtypes.BadRequest, got %T: %v", err, err)
	}
}

func seedMemorySkippedRowRepository(t *testing.T) (invite.Repository, *userpb.UserId, *userpb.User) {
	t.Helper()
	repo := newMemoryRepository(t)
	owner := newInitiatorID()
	injectStoredToken(t, repo, "bad-a", (*invitepb.InviteToken)(nil))
	injectStoredToken(t, repo, "bad-b", &invitepb.InviteToken{Token: "SYNTHETIC-CORRUPT-TOKEN"})
	injectStoredToken(t, repo, "good", &invitepb.InviteToken{
		Token: "good", UserId: owner, Expiration: &typespb.Timestamp{Seconds: 32503680000},
	})
	injectStoredUser(t, repo, owner, nil)
	injectStoredUser(t, repo, owner, &userpb.User{Username: "SYNTHETIC-CORRUPT-USER", Mail: "SYNTHETIC-CORRUPT-MAIL"})
	injectStoredUser(t, repo, owner, &userpb.User{
		Id:       &userpb.UserId{OpaqueId: "   ", Idp: "one.example.com"},
		Username: "SYNTHETIC-CORRUPT-USER", Mail: "SYNTHETIC-CORRUPT-MAIL",
	})
	goodUser := newRemoteUser("good-u", "one.example.com")
	if err := repo.AddRemoteUser(context.Background(), owner, goodUser); err != nil {
		t.Fatal(err)
	}
	return repo, owner, goodUser
}

func memoryAcceptedUserOrder(t *testing.T, repo invite.Repository, owner *userpb.UserId) []string {
	t.Helper()
	m := repo.(*manager)
	m.acceptedUsersMu.RLock()
	defer m.acceptedUsersMu.RUnlock()
	users := m.acceptedUsers[owner.GetOpaqueId()]
	order := make([]string, len(users))
	for i, user := range users {
		if user == nil || user.Id == nil {
			order[i] = ""
			continue
		}
		order[i] = user.Id.GetOpaqueId()
	}
	return order
}

func memoryInviteTokenKeys(t *testing.T, repo invite.Repository) []string {
	t.Helper()
	m := repo.(*manager)
	var keys []string
	m.Invites.Range(func(key, _ any) bool {
		keys = append(keys, key.(string))
		return true
	})
	slices.Sort(keys)
	return keys
}

func assertMemoryLogOmitsSentinels(t *testing.T, log string) {
	t.Helper()
	for _, s := range []string{
		"SYNTHETIC-CORRUPT-USER", "SYNTHETIC-CORRUPT-MAIL", "SYNTHETIC-CORRUPT-TOKEN",
		"good-u", "new-u", "one.example.com",
	} {
		if strings.Contains(log, s) {
			t.Fatalf("log must not contain %q", s)
		}
	}
}

// TestMemorySkippedRowDiagnostics emits exactly one fixed warning per scan.
func TestMemorySkippedRowDiagnostics(t *testing.T) {
	userWarn := "skipping malformed stored remote user"
	tokenWarn := "skipping malformed stored invite token"
	goodUserID := &userpb.UserId{OpaqueId: "good-u", Idp: "one.example.com"}

	for _, tc := range []struct {
		name     string
		userWarn bool
		mutate   bool
		run      func(t *testing.T, repo invite.Repository, owner *userpb.UserId, goodUser *userpb.User, ctx context.Context)
		check    func(t *testing.T, repo invite.Repository, owner *userpb.UserId, goodUser *userpb.User, beforeOrder []string, beforeKeys []string)
	}{
		{
			name: "tokens", userWarn: false,
			run: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, ctx context.Context) {
				tokens, err := repo.ListTokens(ctx, owner)
				if err != nil || len(tokens) != 1 || tokens[0].Token != "good" {
					t.Fatalf("list tokens: %v", tokens)
				}
			},
			check: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, beforeOrder, beforeKeys []string) {
				if !slices.Equal(memoryAcceptedUserOrder(t, repo, owner), beforeOrder) {
					t.Fatal("tokens row changed accepted-user order")
				}
				if !slices.Equal(memoryInviteTokenKeys(t, repo), beforeKeys) {
					t.Fatal("tokens row changed invite keys")
				}
			},
		},
		{
			name: "add", userWarn: true, mutate: true,
			run: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, ctx context.Context) {
				if err := repo.AddRemoteUser(ctx, owner, newRemoteUser("new-u", "one.example.com")); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, beforeOrder, beforeKeys []string) {
				users, err := repo.FindRemoteUsers(context.Background(), owner, "")
				if err != nil || len(users) != 2 || users[1].Id.GetOpaqueId() != "new-u" {
					t.Fatalf("add row: %v %v", users, err)
				}
				if len(beforeOrder)+1 != len(memoryAcceptedUserOrder(t, repo, owner)) {
					t.Fatal("add row did not append exactly one user")
				}
			},
		},
		{
			name: "duplicate", userWarn: true,
			run: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, ctx context.Context) {
				err := repo.AddRemoteUser(ctx, owner, newRemoteUser("good-u", "one.example.com"))
				if !errors.Is(err, invite.ErrUserAlreadyAccepted) {
					t.Fatalf("duplicate: %v", err)
				}
			},
			check: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, beforeOrder, beforeKeys []string) {
				if !slices.Equal(memoryAcceptedUserOrder(t, repo, owner), beforeOrder) {
					t.Fatal("duplicate row changed accepted-user order")
				}
				if !slices.Equal(memoryInviteTokenKeys(t, repo), beforeKeys) {
					t.Fatal("duplicate row changed invite keys")
				}
			},
		},
		{
			name: "get", userWarn: true,
			run: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, goodUser *userpb.User, ctx context.Context) {
				got, err := repo.GetRemoteUser(ctx, owner, goodUser.Id)
				if err != nil || got.Id.GetOpaqueId() != "good-u" {
					t.Fatalf("get: %v %v", got, err)
				}
			},
			check: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, beforeOrder, beforeKeys []string) {
				if !slices.Equal(memoryAcceptedUserOrder(t, repo, owner), beforeOrder) {
					t.Fatal("get row changed accepted-user order")
				}
				if !slices.Equal(memoryInviteTokenKeys(t, repo), beforeKeys) {
					t.Fatal("get row changed invite keys")
				}
			},
		},
		{
			name: "find", userWarn: true,
			run: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, ctx context.Context) {
				users, err := repo.FindRemoteUsers(ctx, owner, "")
				if err != nil || len(users) != 1 {
					t.Fatalf("find: %v %v", users, err)
				}
			},
			check: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, beforeOrder, beforeKeys []string) {
				if !slices.Equal(memoryAcceptedUserOrder(t, repo, owner), beforeOrder) {
					t.Fatal("find row changed accepted-user order")
				}
				if !slices.Equal(memoryInviteTokenKeys(t, repo), beforeKeys) {
					t.Fatal("find row changed invite keys")
				}
			},
		},
		{
			name: "delete", userWarn: true, mutate: true,
			run: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, goodUser *userpb.User, ctx context.Context) {
				if err := repo.DeleteRemoteUser(ctx, owner, goodUser.Id); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, goodUser *userpb.User, beforeOrder, beforeKeys []string) {
				if _, err := repo.GetRemoteUser(context.Background(), owner, goodUserID); err == nil {
					t.Fatal("delete row must remove good user")
				}
				users, err := repo.FindRemoteUsers(context.Background(), owner, "")
				if err != nil || len(users) != 0 {
					t.Fatalf("delete row left valid users: %v", users)
				}
			},
		},
		{
			name: "noop", userWarn: true,
			run: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, ctx context.Context) {
				if err := repo.DeleteRemoteUser(ctx, owner, &userpb.UserId{OpaqueId: "ghost", Idp: "ghost.example.com"}); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, repo invite.Repository, owner *userpb.UserId, _ *userpb.User, beforeOrder, beforeKeys []string) {
				if !slices.Equal(memoryAcceptedUserOrder(t, repo, owner), beforeOrder) {
					t.Fatal("noop row changed accepted-user order")
				}
				if !slices.Equal(memoryInviteTokenKeys(t, repo), beforeKeys) {
					t.Fatal("noop row changed invite keys")
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, owner, goodUser := seedMemorySkippedRowRepository(t)
			beforeOrder := memoryAcceptedUserOrder(t, repo, owner)
			beforeKeys := memoryInviteTokenKeys(t, repo)
			ctx, logBuf := ctxWithCapturedWarnLog(t)
			tc.run(t, repo, owner, goodUser, ctx)
			want := userWarn
			if !tc.userWarn {
				want = tokenWarn
			}
			assertOneSkippedRowWarning(t, logBuf.String(), want)
			assertMemoryLogOmitsSentinels(t, logBuf.String())
			tc.check(t, repo, owner, goodUser, beforeOrder, beforeKeys)
		})
	}

	for _, tc := range []struct {
		name string
		run  func(repo invite.Repository, owner *userpb.UserId, ctx context.Context) error
	}{
		{"AddRemoteUser nil owner", func(repo invite.Repository, _ *userpb.UserId, ctx context.Context) error {
			return repo.AddRemoteUser(ctx, nil, newRemoteUser("new-u", "one.example.com"))
		}},
		{"GetRemoteUser nil remote id", func(repo invite.Repository, owner *userpb.UserId, ctx context.Context) error {
			_, err := repo.GetRemoteUser(ctx, owner, nil)
			return err
		}},
		{"FindRemoteUsers nil owner", func(repo invite.Repository, _ *userpb.UserId, ctx context.Context) error {
			_, err := repo.FindRemoteUsers(ctx, nil, "")
			return err
		}},
		{"DeleteRemoteUser nil remote id", func(repo invite.Repository, owner *userpb.UserId, ctx context.Context) error {
			return repo.DeleteRemoteUser(ctx, owner, nil)
		}},
		{"ListTokens nil owner", func(repo invite.Repository, _ *userpb.UserId, ctx context.Context) error {
			_, err := repo.ListTokens(ctx, nil)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, owner, _ := seedMemorySkippedRowRepository(t)
			beforeOrder := memoryAcceptedUserOrder(t, repo, owner)
			beforeKeys := memoryInviteTokenKeys(t, repo)
			ctx, logBuf := ctxWithCapturedWarnLog(t)
			err := tc.run(repo, owner, ctx)
			assertMemoryBadRequest(t, err)
			if logBuf.Len() != 0 {
				t.Fatal("invalid input must not emit warnings")
			}
			if !slices.Equal(memoryAcceptedUserOrder(t, repo, owner), beforeOrder) {
				t.Fatal("invalid input changed accepted-user order")
			}
			if !slices.Equal(memoryInviteTokenKeys(t, repo), beforeKeys) {
				t.Fatal("invalid input changed invite keys")
			}
		})
	}
}

func TestTokenIsExpired(t *testing.T) {
	initiator := newInitiatorID()
	coreValid := &invitepb.InviteToken{
		Token:  "secret",
		UserId: initiator,
	}
	const now = uint64(200)

	tests := []struct {
		name       string
		expiration *typespb.Timestamp
		want       bool
	}{
		{name: "nil expiration", expiration: nil, want: false},
		{name: "seconds before now", expiration: &typespb.Timestamp{Seconds: 199}, want: true},
		{name: "seconds equal now", expiration: &typespb.Timestamp{Seconds: 200}, want: false},
		{name: "seconds after now", expiration: &typespb.Timestamp{Seconds: 201}, want: false},
		{
			name:       "nonzero nanos does not alter seconds-only policy",
			expiration: &typespb.Timestamp{Seconds: 200, Nanos: 999999999},
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &invitepb.InviteToken{
				Token:      coreValid.Token,
				UserId:     coreValid.UserId,
				Expiration: tt.expiration,
			}
			if got := tokenIsExpired(token, now); got != tt.want {
				t.Fatalf("tokenIsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func tokenSecrets(tokens []*invitepb.InviteToken) []string {
	secrets := make([]string, len(tokens))
	for i, tok := range tokens {
		secrets[i] = tok.Token
	}
	slices.Sort(secrets)
	return secrets
}

func TestListTokensFiltersElapsedExpiration(t *testing.T) {
	repo := newMemoryRepository(t)
	ctx := context.Background()
	initiator := newInitiatorID()
	otherOwner := &userpb.UserId{
		OpaqueId: "other-initiator",
		Idp:      "local.example.com",
		Type:     userpb.UserType_USER_TYPE_PRIMARY,
	}
	now := time.Now()

	expired := &invitepb.InviteToken{
		Token:      "elapsed-secret",
		UserId:     initiator,
		Expiration: &typespb.Timestamp{Seconds: uint64(now.Add(-24 * time.Hour).Unix())},
	}
	future := &invitepb.InviteToken{
		Token:      "future-secret",
		UserId:     initiator,
		Expiration: &typespb.Timestamp{Seconds: uint64(now.Add(24 * time.Hour).Unix())},
	}
	noExpiry := &invitepb.InviteToken{
		Token:  "no-expiry-secret",
		UserId: initiator,
	}
	otherOwnerToken := &invitepb.InviteToken{
		Token:      "other-owner-secret",
		UserId:     otherOwner,
		Expiration: &typespb.Timestamp{Seconds: uint64(now.Add(24 * time.Hour).Unix())},
	}

	for _, tok := range []*invitepb.InviteToken{expired, future, noExpiry, otherOwnerToken} {
		if err := repo.AddToken(ctx, tok); err != nil {
			t.Fatalf("AddToken(%q): %v", tok.Token, err)
		}
	}

	listed, err := repo.ListTokens(ctx, initiator)
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	wantListed := []string{"future-secret", "no-expiry-secret"}
	if got := tokenSecrets(listed); !slices.Equal(got, wantListed) {
		t.Fatalf("ListTokens secrets = %v, want %v", got, wantListed)
	}

	gotExpired, err := repo.GetToken(ctx, "elapsed-secret")
	if err != nil {
		t.Fatalf("GetToken(elapsed): %v", err)
	}
	if gotExpired.Token != expired.Token {
		t.Fatalf("GetToken(elapsed) = %+v, want elapsed token retained", gotExpired)
	}
}
