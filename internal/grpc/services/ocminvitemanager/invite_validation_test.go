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
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
)

// fakeGateway is a minimal gateway mock for the GetUser RPC used by
// AcceptInvite when resolving the invitation initiator.
type fakeGateway struct {
	gateway.GatewayAPIClient

	mu           sync.Mutex
	getUserRes   *userpb.GetUserResponse
	getUserErr   error
	getUserCalls int
}

func (f *fakeGateway) GetUser(context.Context, *userpb.GetUserRequest, ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getUserCalls++
	return f.getUserRes, f.getUserErr
}

func (f *fakeGateway) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getUserCalls
}

// testResolver is a process-wide service.Clients whose Gateway is swappable
// so tests can inject the fake gateway into the global resolver. Tests that
// depend on it must stay sequential: there is a single global resolver.
type testResolver struct {
	revaservice.Clients
	mu sync.Mutex
	gw gateway.GatewayAPIClient
}

func (r *testResolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gw, nil
}

var (
	globalTestResolver     = &testResolver{}
	globalTestResolverOnce sync.Once
)

// stampGateway points the global test resolver at gw.
func stampGateway(gw gateway.GatewayAPIClient) {
	globalTestResolverOnce.Do(func() {
		revaservice.SetGlobal(globalTestResolver)
	})
	globalTestResolver.mu.Lock()
	globalTestResolver.gw = gw
	globalTestResolver.mu.Unlock()
}

var (
	initiatorTokenID = &userpb.UserId{OpaqueId: "initiator-user", Idp: "local.example.com", Type: userpb.UserType_USER_TYPE_PRIMARY}
	initiatorUser    = &userpb.User{
		Id:          initiatorTokenID,
		Mail:        "init@example.com",
		DisplayName: "Init User",
	}
)

func timestampAfter(d time.Duration) *typespb.Timestamp {
	return &typespb.Timestamp{Seconds: uint64(time.Now().Add(d).Unix())}
}

func validStoredToken() *invitepb.InviteToken {
	return &invitepb.InviteToken{
		Token:      "invite-secret",
		UserId:     initiatorTokenID,
		Expiration: timestampAfter(24 * time.Hour),
	}
}

func acceptInviteReq() *invitepb.AcceptInviteRequest {
	return &invitepb.AcceptInviteRequest{
		InviteToken: &invitepb.InviteToken{Token: "invite-secret"},
		RemoteUser: &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "marie@remote.example.org",
				Idp:      "https://remote.example.org",
				Type:     userpb.UserType_USER_TYPE_FEDERATED,
			},
		},
	}
}

func stampUserGateway(res *userpb.GetUserResponse, err error) *fakeGateway {
	gw := &fakeGateway{getUserRes: res, getUserErr: err}
	stampGateway(gw)
	return gw
}

func okGetUserResponse() *userpb.GetUserResponse {
	return &userpb.GetUserResponse{
		Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
		User:   initiatorUser,
	}
}

// TestAcceptInviteRequestValidation rejects malformed requests before any
// storage or gateway access.
func TestAcceptInviteRequestValidation(t *testing.T) {
	validRemote := acceptInviteReq().GetRemoteUser()

	for _, tc := range []struct {
		name string
		req  *invitepb.AcceptInviteRequest
	}{
		{"nil request", nil},
		{"nil invite token", &invitepb.AcceptInviteRequest{RemoteUser: validRemote}},
		{"blank token", &invitepb.AcceptInviteRequest{InviteToken: &invitepb.InviteToken{Token: ""}, RemoteUser: validRemote}},
		{"whitespace token", &invitepb.AcceptInviteRequest{InviteToken: &invitepb.InviteToken{Token: "   "}, RemoteUser: validRemote}},
		{"nil remote user", &invitepb.AcceptInviteRequest{InviteToken: &invitepb.InviteToken{Token: "invite-secret"}}},
		{"remote user without id", &invitepb.AcceptInviteRequest{InviteToken: &invitepb.InviteToken{Token: "invite-secret"}, RemoteUser: &userpb.User{}}},
		{"remote user with blank id", &invitepb.AcceptInviteRequest{InviteToken: &invitepb.InviteToken{Token: "invite-secret"}, RemoteUser: &userpb.User{Id: &userpb.UserId{OpaqueId: "   "}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := newTestService(repo)

			resp, err := svc.AcceptInvite(context.Background(), tc.req)
			assertInvalidArgument(t, mustStatus(t, err, resp.GetStatus()))
			if repo.getTokenCalls != 0 || repo.addRemoteUserCalls != 0 {
				t.Fatalf("expected no storage access, got %d GetToken and %d AddRemoteUser calls", repo.getTokenCalls, repo.addRemoteUserCalls)
			}
		})
	}
}

// TestAcceptInviteRequestTokenCompanionFields proves the request's invite
// token only needs its secret: the accompanying UserId and Expiration fields
// are optional metadata and must not be required to reach the repository.
func TestAcceptInviteRequestTokenCompanionFields(t *testing.T) {
	repo := &fakeRepo{getTokenStubbed: true, getTokenStubErr: invite.ErrTokenNotFound}
	svc := newTestService(repo)

	resp, err := svc.AcceptInvite(context.Background(), acceptInviteReq())
	status := mustStatus(t, err, resp.GetStatus())
	assertInvalidArgument(t, status)
	if status.Message != "token invalid or not found" {
		t.Fatalf("expected the token-not-found diagnostic, got %q", status.Message)
	}
	if repo.getTokenCalls != 1 {
		t.Fatalf("expected the request to proceed to GetToken, got %d calls", repo.getTokenCalls)
	}
	if repo.addRemoteUserCalls != 0 {
		t.Fatalf("expected no AddRemoteUser call, got %d", repo.addRemoteUserCalls)
	}
}

// TestAcceptInviteStoredTokenValidation covers every malformed stored token:
// each must be answered with the constant token diagnostic and no gateway or
// storage writes.
func TestAcceptInviteStoredTokenValidation(t *testing.T) {
	nilExpiration := validStoredToken()
	nilExpiration.Expiration = nil
	expired := validStoredToken()
	expired.Expiration = timestampAfter(-24 * time.Hour)
	missingInitiator := validStoredToken()
	missingInitiator.UserId = nil

	blankSecret := validStoredToken()
	blankSecret.Token = "   "
	blankOwner := validStoredToken()
	blankOwner.UserId = &userpb.UserId{OpaqueId: "   ", Idp: initiatorTokenID.GetIdp()}
	whitespaceOwner := validStoredToken()
	whitespaceOwner.UserId = &userpb.UserId{OpaqueId: " ", Idp: initiatorTokenID.GetIdp()}

	for _, tc := range []struct {
		name        string
		storedToken *invitepb.InviteToken
		storedErr   error
	}{
		{"nil stored token", nil, nil},
		{"blank stored secret", blankSecret, nil},
		{"blank stored owner", blankOwner, nil},
		{"whitespace stored owner", whitespaceOwner, nil},
		{"stored token without initiator", missingInitiator, nil},
		{"stored token without expiration", nilExpiration, nil},
		{"expired stored token", expired, nil},
		{"missing stored token", nil, invite.ErrTokenNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{getTokenStubbed: true, getTokenStub: tc.storedToken, getTokenStubErr: tc.storedErr}
			gw := stampUserGateway(okGetUserResponse(), nil)
			svc := newTestService(repo)

			resp, err := svc.AcceptInvite(context.Background(), acceptInviteReq())
			status := mustStatus(t, err, resp.GetStatus())
			assertInvalidArgument(t, status)
			if status.Message != "token invalid or not found" {
				t.Fatalf("expected the token diagnostic, got %q", status.Message)
			}
			if gw.calls() != 0 {
				t.Fatalf("expected no GetUser call, got %d", gw.calls())
			}
			if repo.getTokenCalls != 1 {
				t.Fatalf("expected one GetToken call, got %d", repo.getTokenCalls)
			}
			if repo.addRemoteUserCalls != 0 {
				t.Fatalf("expected no AddRemoteUser call, got %d", repo.addRemoteUserCalls)
			}
		})
	}
}

func TestAcceptInvitePostCanonicalizationInvalidID(t *testing.T) {
	repo := &fakeRepo{getTokenStubbed: true, getTokenStub: validStoredToken()}
	gw := stampUserGateway(okGetUserResponse(), nil)
	req := acceptInviteReq()
	req.RemoteUser.Id.OpaqueId = " @remote.example.org"
	req.RemoteUser.Id.Idp = "https://remote.example.org"

	resp, err := newTestService(repo).AcceptInvite(context.Background(), req)
	status := mustStatus(t, err, resp.GetStatus())
	assertInvalidArgument(t, status)
	if !strings.Contains(status.Message, "invalid remote user") {
		t.Fatalf("expected invalid remote user diagnostic, got %q", status.Message)
	}
	if repo.getTokenCalls != 1 {
		t.Fatalf("expected one GetToken call, got %d", repo.getTokenCalls)
	}
	if gw.calls() != 1 {
		t.Fatalf("expected one GetUser call, got %d", gw.calls())
	}
	if repo.addRemoteUserCalls != 0 {
		t.Fatalf("expected no AddRemoteUser call, got %d", repo.addRemoteUserCalls)
	}
}

func TestAcceptInviteValidToken(t *testing.T) {
	repo := &fakeRepo{getTokenStubbed: true, getTokenStub: validStoredToken()}
	stampUserGateway(okGetUserResponse(), nil)
	svc := newTestService(repo)

	resp, err := svc.AcceptInvite(context.Background(), acceptInviteReq())
	status := mustStatus(t, err, resp.GetStatus())
	if status.Code != rpcv1beta1.Code_CODE_OK {
		t.Fatalf("expected CODE_OK, got %v: %q", status.Code, status.Message)
	}
	if repo.getTokenCalls != 1 {
		t.Fatalf("expected one GetToken call, got %d", repo.getTokenCalls)
	}
	if repo.addRemoteUserCalls != 1 {
		t.Fatalf("expected one AddRemoteUser call, got %d", repo.addRemoteUserCalls)
	}

	// the initiator recorded must be the token owner
	if got := repo.lastAddRemoteUserInitiator; got.GetOpaqueId() != initiatorTokenID.GetOpaqueId() || got.GetIdp() != initiatorTokenID.GetIdp() {
		t.Fatalf("expected the token initiator, got %v", got)
	}
	// the remote identity must be canonicalized against the remote Idp
	if got := repo.lastAddRemoteUser.GetId(); got.GetOpaqueId() != "marie" || got.GetIdp() != "remote.example.org" {
		t.Fatalf("expected the canonicalized remote identity, got %v", got)
	}

	// the response describes the initiator, not the accepting remote user
	if resp.GetUserId().GetOpaqueId() != initiatorTokenID.GetOpaqueId() {
		t.Fatalf("expected the initiator in the response, got %v", resp.GetUserId())
	}
	if resp.GetEmail() != initiatorUser.Mail || resp.GetDisplayName() != initiatorUser.DisplayName {
		t.Fatalf("expected the initiator identity in the response, got %q %q", resp.GetEmail(), resp.GetDisplayName())
	}
}

func TestAcceptInviteAlreadyAccepted(t *testing.T) {
	repo := &fakeRepo{
		getTokenStubbed:  true,
		getTokenStub:     validStoredToken(),
		addRemoteUserErr: invite.ErrUserAlreadyAccepted,
	}
	stampUserGateway(okGetUserResponse(), nil)
	svc := newTestService(repo)

	resp, err := svc.AcceptInvite(context.Background(), acceptInviteReq())
	status := mustStatus(t, err, resp.GetStatus())
	if status.Code != rpcv1beta1.Code_CODE_ALREADY_EXISTS {
		t.Fatalf("expected CODE_ALREADY_EXISTS, got %v: %q", status.Code, status.Message)
	}
}

// TestAcceptInviteGetUserInfoFailures covers the gateway lookup of the
// initiator: every failure must surface as CODE_INTERNAL and must never reach
// the repository write.
func TestAcceptInviteGetUserInfoFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		res  *userpb.GetUserResponse
		err  error
	}{
		{"transport error", nil, errors.New("gateway unreachable")},
		{"nil response", nil, nil},
		{"nil status", &userpb.GetUserResponse{}, nil},
		{"non ok status", &userpb.GetUserResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_NOT_FOUND, Message: "user missing"}}, nil},
		{"nil user payload", &userpb.GetUserResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}}, nil},
		{"user payload without id", &userpb.GetUserResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}, User: &userpb.User{}}, nil},
		{"user payload with blank id", &userpb.GetUserResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}, User: &userpb.User{Id: &userpb.UserId{OpaqueId: "  "}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{getTokenStubbed: true, getTokenStub: validStoredToken()}
			stampUserGateway(tc.res, tc.err)
			svc := newTestService(repo)

			resp, err := svc.AcceptInvite(context.Background(), acceptInviteReq())
			status := mustStatus(t, err, resp.GetStatus())
			if status.Code != rpcv1beta1.Code_CODE_INTERNAL {
				t.Fatalf("expected CODE_INTERNAL, got %v: %q", status.Code, status.Message)
			}
			if repo.addRemoteUserCalls != 0 {
				t.Fatalf("expected no AddRemoteUser call, got %d", repo.addRemoteUserCalls)
			}
		})
	}
}
