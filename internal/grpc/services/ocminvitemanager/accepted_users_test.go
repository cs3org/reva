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

package ocminvitemanager

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
	jsoninvite "github.com/cs3org/reva/v3/pkg/ocm/invite/repository/json"
	"github.com/cs3org/reva/v3/pkg/utils"
)

// fakeRepo is a call-counting invite.Repository used by the service tests. It
// records the arguments of the repository-facing calls so tests can assert
// which identities reached the storage layer.
type fakeRepo struct {
	mu sync.Mutex

	tokens map[string]*invitepb.InviteToken

	// configurable results
	getTokenStubbed  bool
	getTokenStub     *invitepb.InviteToken
	getTokenStubErr  error
	getRemoteUserRes *userpb.User
	getRemoteUserErr error
	listTokensRes    []*invitepb.InviteToken
	listTokensErr    error
	findRes          []*userpb.User
	findErr          error
	addRemoteUserErr error
	deleteErr        error

	// counters and last-seen arguments
	addTokenCalls         int
	getTokenCalls         int
	listTokensCalls       int
	addRemoteUserCalls    int
	getRemoteUserCalls    int
	findRemoteUsersCalls  int
	deleteRemoteUserCalls int

	lastGetRemoteUserInitiator *userpb.UserId
	lastAddRemoteUserInitiator *userpb.UserId
	lastAddRemoteUser          *userpb.User
	lastDeleteInitiator        *userpb.UserId
}

func (f *fakeRepo) AddToken(_ context.Context, token *invitepb.InviteToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addTokenCalls++
	if f.tokens == nil {
		f.tokens = map[string]*invitepb.InviteToken{}
	}
	if token != nil {
		f.tokens[token.Token] = token
	}
	return nil
}

func (f *fakeRepo) GetToken(_ context.Context, token string) (*invitepb.InviteToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getTokenCalls++
	if f.getTokenStubbed {
		return f.getTokenStub, f.getTokenStubErr
	}
	if tkn, ok := f.tokens[token]; ok {
		return tkn, nil
	}
	return nil, invite.ErrTokenNotFound
}

func (f *fakeRepo) ListTokens(_ context.Context, _ *userpb.UserId) ([]*invitepb.InviteToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listTokensCalls++
	return f.listTokensRes, f.listTokensErr
}

func (f *fakeRepo) AddRemoteUser(_ context.Context, initiator *userpb.UserId, remoteUser *userpb.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addRemoteUserCalls++
	f.lastAddRemoteUserInitiator = initiator
	f.lastAddRemoteUser = remoteUser
	return f.addRemoteUserErr
}

func (f *fakeRepo) GetRemoteUser(_ context.Context, initiator *userpb.UserId, _ *userpb.UserId) (*userpb.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getRemoteUserCalls++
	f.lastGetRemoteUserInitiator = initiator
	return f.getRemoteUserRes, f.getRemoteUserErr
}

func (f *fakeRepo) FindRemoteUsers(_ context.Context, _ *userpb.UserId, _ string) ([]*userpb.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.findRemoteUsersCalls++
	return f.findRes, f.findErr
}

func (f *fakeRepo) DeleteRemoteUser(_ context.Context, initiator *userpb.UserId, _ *userpb.UserId) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteRemoteUserCalls++
	f.lastDeleteInitiator = initiator
	return f.deleteErr
}

func (f *fakeRepo) totalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addTokenCalls + f.getTokenCalls + f.listTokensCalls +
		f.addRemoteUserCalls + f.getRemoteUserCalls + f.findRemoteUsersCalls + f.deleteRemoteUserCalls
}

func newTestService(repo invite.Repository) *service {
	return &service{
		conf: &config{tokenExpiration: 24 * time.Hour},
		repo: repo,
	}
}

var (
	testInitiatorID = &userpb.UserId{OpaqueId: "initiator", Idp: "local.example.com", Type: userpb.UserType_USER_TYPE_PRIMARY}
	testCtxUser     = &userpb.User{Id: testInitiatorID, Username: "initiator", Mail: "initiator@example.com", DisplayName: "Initiator"}
)

func ctxWithUser(u *userpb.User) context.Context {
	return appctx.ContextSetUser(context.Background(), u)
}

func opaqueFilter(t *testing.T, id *userpb.UserId) *typespb.Opaque {
	t.Helper()
	d, err := utils.MarshalProtoV1ToJSON(id)
	if err != nil {
		t.Fatalf("error marshaling the user filter: %v", err)
	}
	return &typespb.Opaque{
		Map: map[string]*typespb.OpaqueEntry{
			"user-filter": {Decoder: "json", Value: d},
		},
	}
}

func mustStatus(t *testing.T, err error, status *rpcv1beta1.Status) *rpcv1beta1.Status {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected rpc error: %v", err)
	}
	if status == nil {
		t.Fatal("expected a status, got nil")
	}
	return status
}

func assertInvalidArgument(t *testing.T, status *rpcv1beta1.Status) {
	t.Helper()
	if status.Code != rpcv1beta1.Code_CODE_INVALID_ARGUMENT {
		t.Fatalf("expected CODE_INVALID_ARGUMENT, got %v: %q", status.Code, status.Message)
	}
}

func assertUnauthenticated(t *testing.T, status *rpcv1beta1.Status) {
	t.Helper()
	if status.Code != rpcv1beta1.Code_CODE_UNAUTHENTICATED {
		t.Fatalf("expected CODE_UNAUTHENTICATED, got %v: %q", status.Code, status.Message)
	}
}

func TestGetAcceptedUserNilRequest(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo)

	resp, err := svc.GetAcceptedUser(context.Background(), nil)
	assertInvalidArgument(t, mustStatus(t, err, resp.GetStatus()))
	if repo.totalCalls() != 0 {
		t.Fatalf("expected zero repository calls, got %d", repo.totalCalls())
	}
}

func TestGetAcceptedUserInvalidRemoteUserID(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  *invitepb.GetAcceptedUserRequest
	}{
		{"nil remote user id", &invitepb.GetAcceptedUserRequest{}},
		{"blank remote user id", &invitepb.GetAcceptedUserRequest{RemoteUserId: &userpb.UserId{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := newTestService(repo)

			resp, err := svc.GetAcceptedUser(ctxWithUser(testCtxUser), tc.req)
			assertInvalidArgument(t, mustStatus(t, err, resp.GetStatus()))
			if repo.totalCalls() != 0 {
				t.Fatalf("expected zero repository calls, got %d", repo.totalCalls())
			}
		})
	}
}

func TestGetAcceptedUserRejectsInvalidFilters(t *testing.T) {
	remoteID := &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}

	for _, tc := range []struct {
		name string
		req  *invitepb.GetAcceptedUserRequest
	}{
		{"missing filter entry", &invitepb.GetAcceptedUserRequest{
			RemoteUserId: remoteID,
			Opaque:       &typespb.Opaque{Map: map[string]*typespb.OpaqueEntry{}},
		}},
		{"nil filter entry", &invitepb.GetAcceptedUserRequest{
			RemoteUserId: remoteID,
			Opaque: &typespb.Opaque{Map: map[string]*typespb.OpaqueEntry{
				"user-filter": nil,
			}},
		}},
		{"bad filter json", &invitepb.GetAcceptedUserRequest{
			RemoteUserId: remoteID,
			Opaque: &typespb.Opaque{Map: map[string]*typespb.OpaqueEntry{
				"user-filter": {Decoder: "json", Value: []byte("{not json")},
			}},
		}},
		{"decoded empty id", &invitepb.GetAcceptedUserRequest{
			RemoteUserId: remoteID,
			Opaque: &typespb.Opaque{Map: map[string]*typespb.OpaqueEntry{
				"user-filter": {Decoder: "json", Value: []byte(`{"idp":"local.example.com"}`)},
			}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := newTestService(repo)

			resp, err := svc.GetAcceptedUser(context.Background(), tc.req)
			status := mustStatus(t, err, resp.GetStatus())
			assertInvalidArgument(t, status)
			// the diagnostic must be the constant filter message, not a
			// request dump with opaque data
			if status.Message != "invalid user filter" {
				t.Fatalf("expected the constant filter diagnostic, got %q", status.Message)
			}
			if repo.totalCalls() != 0 {
				t.Fatalf("expected zero repository calls, got %d", repo.totalCalls())
			}
		})
	}
}

func TestGetAcceptedUserNilContextUserDoesNotFallBack(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo)

	req := &invitepb.GetAcceptedUserRequest{
		RemoteUserId: &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"},
		Opaque:       opaqueFilter(t, testInitiatorID), // a valid fallback attempt
	}
	resp, err := svc.GetAcceptedUser(ctxWithUser(nil), req)
	assertInvalidArgument(t, mustStatus(t, err, resp.GetStatus()))
	if repo.totalCalls() != 0 {
		t.Fatalf("a broken context identity must not fall back to the opaque filter, got %d calls", repo.totalCalls())
	}
}

func TestGetAcceptedUserContextUserWithoutIDDoesNotFallBack(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo)

	req := &invitepb.GetAcceptedUserRequest{
		RemoteUserId: &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"},
		Opaque:       opaqueFilter(t, testInitiatorID), // a valid fallback attempt
	}
	resp, err := svc.GetAcceptedUser(ctxWithUser(&userpb.User{}), req)
	assertInvalidArgument(t, mustStatus(t, err, resp.GetStatus()))
	if repo.totalCalls() != 0 {
		t.Fatalf("a broken context identity must not fall back to the opaque filter, got %d calls", repo.totalCalls())
	}
}

func TestGetAcceptedUserContextUserOverridesOpaqueFilter(t *testing.T) {
	repo := &fakeRepo{getRemoteUserRes: &userpb.User{Id: &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}}}
	svc := newTestService(repo)

	otherFilter := &userpb.UserId{OpaqueId: "someone-else", Idp: "local.example.com"}
	req := &invitepb.GetAcceptedUserRequest{
		RemoteUserId: &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"},
		Opaque:       opaqueFilter(t, otherFilter),
	}
	resp, err := svc.GetAcceptedUser(ctxWithUser(testCtxUser), req)
	status := mustStatus(t, err, resp.GetStatus())
	if status.Code != rpcv1beta1.Code_CODE_OK {
		t.Fatalf("expected CODE_OK, got %v: %q", status.Code, status.Message)
	}
	if got := repo.lastGetRemoteUserInitiator; got.GetOpaqueId() != testInitiatorID.GetOpaqueId() || got.GetIdp() != testInitiatorID.GetIdp() {
		t.Fatalf("the context identity must win over the opaque filter, got %v", got)
	}
	if resp.GetRemoteUser().GetId().GetOpaqueId() != "alice" {
		t.Fatalf("unexpected remote user: %v", resp.GetRemoteUser())
	}
}

func TestGetAcceptedUserOpaqueFilterWithoutContext(t *testing.T) {
	repo := &fakeRepo{getRemoteUserRes: &userpb.User{Id: &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}}}
	svc := newTestService(repo)

	req := &invitepb.GetAcceptedUserRequest{
		RemoteUserId: &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"},
		Opaque:       opaqueFilter(t, testInitiatorID),
	}
	resp, err := svc.GetAcceptedUser(context.Background(), req)
	status := mustStatus(t, err, resp.GetStatus())
	if status.Code != rpcv1beta1.Code_CODE_OK {
		t.Fatalf("expected CODE_OK, got %v: %q", status.Code, status.Message)
	}
	if got := repo.lastGetRemoteUserInitiator; got.GetOpaqueId() != testInitiatorID.GetOpaqueId() || got.GetIdp() != testInitiatorID.GetIdp() {
		t.Fatalf("expected the decoded opaque identity, got %v", got)
	}
}

func TestGetAcceptedUserRepositoryNotFound(t *testing.T) {
	repo := &fakeRepo{getRemoteUserErr: errtypes.NotFound("remote user missing")}
	svc := newTestService(repo)

	req := &invitepb.GetAcceptedUserRequest{
		RemoteUserId: &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"},
		Opaque:       opaqueFilter(t, testInitiatorID),
	}
	resp, err := svc.GetAcceptedUser(context.Background(), req)
	status := mustStatus(t, err, resp.GetStatus())
	if status.Code != rpcv1beta1.Code_CODE_NOT_FOUND {
		t.Fatalf("expected CODE_NOT_FOUND, got %v: %q", status.Code, status.Message)
	}
}

func TestGetAcceptedUserMalformedSuccess(t *testing.T) {
	for _, tc := range []struct {
		name string
		repo *fakeRepo
	}{
		{"nil user", &fakeRepo{}},
		{"user without id", &fakeRepo{getRemoteUserRes: &userpb.User{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(tc.repo)

			req := &invitepb.GetAcceptedUserRequest{
				RemoteUserId: &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"},
				Opaque:       opaqueFilter(t, testInitiatorID),
			}
			resp, err := svc.GetAcceptedUser(context.Background(), req)
			status := mustStatus(t, err, resp.GetStatus())
			if status.Code != rpcv1beta1.Code_CODE_INTERNAL {
				t.Fatalf("expected CODE_INTERNAL for a malformed success, got %v: %q", status.Code, status.Message)
			}
		})
	}
}

func TestProtectedMethodsRejectNilRequests(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		run  func(*service) *rpcv1beta1.Status
	}{
		{"GenerateInviteToken", func(s *service) *rpcv1beta1.Status {
			resp, err := s.GenerateInviteToken(ctx, nil)
			return mustStatus(t, err, resp.GetStatus())
		}},
		{"ListInviteTokens", func(s *service) *rpcv1beta1.Status {
			resp, err := s.ListInviteTokens(ctx, nil)
			return mustStatus(t, err, resp.GetStatus())
		}},
		{"FindAcceptedUsers", func(s *service) *rpcv1beta1.Status {
			resp, err := s.FindAcceptedUsers(ctx, nil)
			return mustStatus(t, err, resp.GetStatus())
		}},
		{"DeleteAcceptedUser", func(s *service) *rpcv1beta1.Status {
			resp, err := s.DeleteAcceptedUser(ctx, nil)
			return mustStatus(t, err, resp.GetStatus())
		}},
		{"GetAcceptedUser", func(s *service) *rpcv1beta1.Status {
			resp, err := s.GetAcceptedUser(ctx, nil)
			return mustStatus(t, err, resp.GetStatus())
		}},
		{"AcceptInvite", func(s *service) *rpcv1beta1.Status {
			resp, err := s.AcceptInvite(ctx, nil)
			return mustStatus(t, err, resp.GetStatus())
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			assertInvalidArgument(t, tc.run(newTestService(repo)))
			if repo.totalCalls() != 0 {
				t.Fatalf("expected zero repository calls, got %d", repo.totalCalls())
			}
		})
	}
}

func protectedMethodCases(t *testing.T, ctx context.Context) []struct {
	name string
	run  func(*service) *rpcv1beta1.Status
} {
	return []struct {
		name string
		run  func(*service) *rpcv1beta1.Status
	}{
		{"GenerateInviteToken", func(s *service) *rpcv1beta1.Status {
			resp, err := s.GenerateInviteToken(ctx, &invitepb.GenerateInviteTokenRequest{})
			return mustStatus(t, err, resp.GetStatus())
		}},
		{"ListInviteTokens", func(s *service) *rpcv1beta1.Status {
			resp, err := s.ListInviteTokens(ctx, &invitepb.ListInviteTokensRequest{})
			return mustStatus(t, err, resp.GetStatus())
		}},
		{"FindAcceptedUsers", func(s *service) *rpcv1beta1.Status {
			resp, err := s.FindAcceptedUsers(ctx, &invitepb.FindAcceptedUsersRequest{})
			return mustStatus(t, err, resp.GetStatus())
		}},
		{"DeleteAcceptedUser", func(s *service) *rpcv1beta1.Status {
			resp, err := s.DeleteAcceptedUser(ctx, &invitepb.DeleteAcceptedUserRequest{})
			return mustStatus(t, err, resp.GetStatus())
		}},
	}
}

func TestProtectedMethodsRequireContextUser(t *testing.T) {
	for _, tc := range protectedMethodCases(t, context.Background()) {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			assertUnauthenticated(t, tc.run(newTestService(repo)))
			if repo.totalCalls() != 0 {
				t.Fatalf("expected zero repository calls, got %d", repo.totalCalls())
			}
		})
	}
}

func TestProtectedMethodsRejectMalformedContextUser(t *testing.T) {
	for _, ctxUser := range []struct {
		name string
		user *userpb.User
	}{
		{"typed-nil context user", nil},
		{"context user without id", &userpb.User{}},
		{"context user with blank id", &userpb.User{Id: &userpb.UserId{OpaqueId: "   "}}},
	} {
		for _, tc := range protectedMethodCases(t, ctxWithUser(ctxUser.user)) {
			t.Run(ctxUser.name+"/"+tc.name, func(t *testing.T) {
				repo := &fakeRepo{}
				assertUnauthenticated(t, tc.run(newTestService(repo)))
				if repo.totalCalls() != 0 {
					t.Fatalf("expected zero repository calls, got %d", repo.totalCalls())
				}
			})
		}
	}
}

func TestProtectedMethodsWithValidUser(t *testing.T) {
	ctx := ctxWithUser(testCtxUser)

	t.Run("GenerateInviteToken", func(t *testing.T) {
		repo := &fakeRepo{}
		resp, err := newTestService(repo).GenerateInviteToken(ctx, &invitepb.GenerateInviteTokenRequest{})
		status := mustStatus(t, err, resp.GetStatus())
		if status.Code != rpcv1beta1.Code_CODE_OK {
			t.Fatalf("expected CODE_OK, got %v: %q", status.Code, status.Message)
		}
		if resp.GetInviteToken().GetUserId().GetOpaqueId() != testInitiatorID.GetOpaqueId() {
			t.Fatalf("token must be issued for the context user, got %v", resp.GetInviteToken().GetUserId())
		}
		if repo.addTokenCalls != 1 {
			t.Fatalf("expected one AddToken call, got %d", repo.addTokenCalls)
		}
	})

	t.Run("ListInviteTokens", func(t *testing.T) {
		repo := &fakeRepo{listTokensRes: []*invitepb.InviteToken{{Token: "stored-secret"}}}
		resp, err := newTestService(repo).ListInviteTokens(ctx, &invitepb.ListInviteTokensRequest{})
		status := mustStatus(t, err, resp.GetStatus())
		if status.Code != rpcv1beta1.Code_CODE_OK {
			t.Fatalf("expected CODE_OK, got %v: %q", status.Code, status.Message)
		}
		if len(resp.GetInviteTokens()) != 1 {
			t.Fatalf("expected the stored token, got %v", resp.GetInviteTokens())
		}
	})

	t.Run("FindAcceptedUsersEmptyQuery", func(t *testing.T) {
		repo := &fakeRepo{}
		resp, err := newTestService(repo).FindAcceptedUsers(ctx, &invitepb.FindAcceptedUsersRequest{})
		status := mustStatus(t, err, resp.GetStatus())
		if status.Code != rpcv1beta1.Code_CODE_OK {
			t.Fatalf("expected CODE_OK for a valid empty query, got %v: %q", status.Code, status.Message)
		}
		if len(resp.GetAcceptedUsers()) != 0 {
			t.Fatalf("expected an empty list, got %v", resp.GetAcceptedUsers())
		}
	})

	t.Run("DeleteAcceptedUserIdempotent", func(t *testing.T) {
		repo := &fakeRepo{}
		svc := newTestService(repo)
		req := &invitepb.DeleteAcceptedUserRequest{RemoteUserId: &userpb.UserId{OpaqueId: "alice", Idp: "one.example.com"}}
		for range 2 {
			resp, err := svc.DeleteAcceptedUser(ctx, req)
			status := mustStatus(t, err, resp.GetStatus())
			if status.Code != rpcv1beta1.Code_CODE_OK {
				t.Fatalf("expected CODE_OK, got %v: %q", status.Code, status.Message)
			}
		}
		if repo.deleteRemoteUserCalls != 2 {
			t.Fatalf("expected two delete calls, got %d", repo.deleteRemoteUserCalls)
		}
		if got := repo.lastDeleteInitiator; got.GetOpaqueId() != testInitiatorID.GetOpaqueId() {
			t.Fatalf("expected the context initiator, got %v", got)
		}
	})

	t.Run("DeleteAcceptedUserInvalidRemoteID", func(t *testing.T) {
		repo := &fakeRepo{}
		resp, err := newTestService(repo).DeleteAcceptedUser(ctx, &invitepb.DeleteAcceptedUserRequest{})
		assertInvalidArgument(t, mustStatus(t, err, resp.GetStatus()))
		if repo.totalCalls() != 0 {
			t.Fatalf("expected zero repository calls, got %d", repo.totalCalls())
		}
	})
}

// TestGetAcceptedUserJSONRepositoryNilRemoteID runs the service on top of the
// real JSON repository: a nil RemoteUserId must be rejected before the
// repository is called, without a panic and without touching the file.
func TestGetAcceptedUserJSONRepositoryNilRemoteID(t *testing.T) {
	file := filepath.Join(t.TempDir(), "invites.json")
	repo, err := jsoninvite.New(context.Background(), map[string]any{"file": file})
	if err != nil {
		t.Fatalf("json: error creating repository: %v", err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("json: error reading the invite file: %v", err)
	}

	svc := newTestService(repo)
	resp, err := svc.GetAcceptedUser(ctxWithUser(testCtxUser), &invitepb.GetAcceptedUserRequest{})
	assertInvalidArgument(t, mustStatus(t, err, resp.GetStatus()))

	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("json: error reading the invite file: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the invite file must stay unchanged by a rejected request")
	}
}
