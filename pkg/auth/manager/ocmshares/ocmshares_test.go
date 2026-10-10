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

package ocmshares

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocminvite "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// fakeGW is a minimal gateway mock with call counters and independent
// transport errors, statuses and payloads for the two RPCs the direct-secret
// manager uses.
type fakeGW struct {
	gateway.GatewayAPIClient

	mu                sync.Mutex
	shareRes          *ocm.GetOCMShareByTokenResponse
	shareErr          error
	userRes           *ocminvite.GetAcceptedUserResponse
	userErr           error
	shareCalls        int
	acceptedUserCalls int
}

func (f *fakeGW) GetOCMShareByToken(context.Context, *ocm.GetOCMShareByTokenRequest, ...grpc.CallOption) (*ocm.GetOCMShareByTokenResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shareCalls++
	return f.shareRes, f.shareErr
}

func (f *fakeGW) GetAcceptedUser(context.Context, *ocminvite.GetAcceptedUserRequest, ...grpc.CallOption) (*ocminvite.GetAcceptedUserResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acceptedUserCalls++
	return f.userRes, f.userErr
}

func (f *fakeGW) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shareCalls, f.acceptedUserCalls
}

func newManager() *manager {
	return &manager{c: &config{}}
}

func validShare(id string) *ocm.Share {
	return &ocm.Share{
		Id:         &ocm.ShareId{OpaqueId: id},
		ResourceId: &provider.ResourceId{StorageId: "stor", OpaqueId: "res"},
		Token:      "secret-token",
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: &userpb.UserId{OpaqueId: "grantee", Idp: "remote.example.com", Type: userpb.UserType_USER_TYPE_FEDERATED},
			},
		},
		Creator: &userpb.UserId{OpaqueId: "creator", Idp: "local.example.com"},
		Owner:   &userpb.UserId{OpaqueId: "creator", Idp: "local.example.com"},
		AccessMethods: []*ocm.AccessMethod{
			{
				Term: &ocm.AccessMethod_WebdavOptions{
					WebdavOptions: &ocm.WebDAVAccessMethod{
						Permissions: permissions.NewViewerRole().CS3ResourcePermissions(),
						AccessTypes: []ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE},
					},
				},
			},
		},
	}
}

func editorShare(id string) *ocm.Share {
	s := validShare(id)
	s.AccessMethods = []*ocm.AccessMethod{
		{
			Term: &ocm.AccessMethod_WebdavOptions{
				WebdavOptions: &ocm.WebDAVAccessMethod{
					Permissions: permissions.NewEditorRole().CS3ResourcePermissions(),
					AccessTypes: []ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE},
				},
			},
		},
	}
	return s
}

func okUserResponse() *ocminvite.GetAcceptedUserResponse {
	return &ocminvite.GetAcceptedUserResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		RemoteUser: &userpb.User{
			Id: &userpb.UserId{OpaqueId: "grantee", Idp: "remote.example.com", Type: userpb.UserType_USER_TYPE_FEDERATED},
		},
	}
}

func stampOK(share *ocm.Share) *fakeGW {
	fake := &fakeGW{
		shareRes: &ocm.GetOCMShareByTokenResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: share},
		userRes:  okUserResponse(),
	}
	stampGateway(fake)
	return fake
}

func assertInvalidCredentials(t *testing.T, err error, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	ic, ok := err.(errtypes.InvalidCredentials)
	if !ok {
		t.Fatalf("expected errtypes.InvalidCredentials, got %T: %v", err, err)
	}
	if !strings.Contains(ic.Error(), wantMsg) {
		t.Fatalf("expected error containing %q, got %q", wantMsg, ic.Error())
	}
}

func assertNoAcceptedUserCall(t *testing.T, fake *fakeGW) {
	t.Helper()
	if _, acceptedCalls := fake.counts(); acceptedCalls != 0 {
		t.Fatalf("GetAcceptedUser must not be called after a rejection, got %d calls", acceptedCalls)
	}
}

func authenticate(ctx context.Context, requestedID string) (*userpb.User, map[string]*authpb.Scope, error) {
	return newManager().Authenticate(ctx, requestedID, "secret-token")
}

func TestAuthenticateValidViewerShare(t *testing.T) {
	fake := stampOK(validShare("share-abc"))

	user, scopes, err := authenticate(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.Id.OpaqueId != "grantee" {
		t.Fatalf("unexpected user: %v", user)
	}
	if user.Opaque == nil || user.Opaque.Map["ocm-share-role"] == nil || string(user.Opaque.Map["ocm-share-role"].Value) != "viewer" {
		t.Fatalf("expected the viewer role attached to the user, got %v", user.Opaque)
	}
	if s := scopes["ocmshare:share-abc"]; s == nil || s.Role != authpb.Role_ROLE_VIEWER {
		t.Fatalf("expected the viewer role on the ocmshare scope, got %v", s)
	}
	shareCalls, acceptedCalls := fake.counts()
	if shareCalls != 1 || acceptedCalls != 1 {
		t.Fatalf("unexpected gateway calls: share=%d accepted=%d", shareCalls, acceptedCalls)
	}

	// the scope keeps the direct-secret share record intact
	scoped, err := scope.GetOCMSharesFromScopes(scopes)
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 {
		t.Fatalf("expected 1 share in the scope, got %d", len(scoped))
	}
	got := scoped[0]
	if got.Token != "secret-token" {
		t.Errorf("scope token: got %q, want secret-token", got.Token)
	}
	if got.Id.GetOpaqueId() != "share-abc" {
		t.Errorf("scope share id: got %s, want share-abc", got.Id.GetOpaqueId())
	}
	if got.Creator.GetOpaqueId() != "creator" || got.Creator.GetIdp() != "local.example.com" {
		t.Errorf("scope creator: got %v", got.Creator)
	}
	if len(got.AccessMethods) != 1 {
		t.Errorf("scope access methods: got %d, want 1", len(got.AccessMethods))
	}
}

func TestAuthenticateValidEditorShare(t *testing.T) {
	stampOK(editorShare("share-abc"))

	user, scopes, err := authenticate(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.Id.OpaqueId != "grantee" {
		t.Fatalf("unexpected user: %v", user)
	}
	if s := scopes["ocmshare:share-abc"]; s == nil || s.Role != authpb.Role_ROLE_EDITOR {
		t.Fatalf("expected the editor role on the ocmshare scope, got %v", s)
	}
	if user.Opaque == nil || user.Opaque.Map["ocm-share-role"] == nil || string(user.Opaque.Map["ocm-share-role"].Value) != "editor" {
		t.Fatalf("expected the editor role attached to the user, got %v", user.Opaque)
	}
}

func TestAuthenticateMatchingRequestedShareID(t *testing.T) {
	stampOK(validShare("share-abc"))

	user, _, err := authenticate(context.Background(), "share-abc")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.Id.OpaqueId != "grantee" {
		t.Fatalf("unexpected user: %v", user)
	}
}

func TestAuthenticateRequestedShareIDMismatch(t *testing.T) {
	fake := stampOK(validShare("share-abc"))

	user, scopes, err := authenticate(context.Background(), "other-share")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	assertInvalidCredentials(t, err, "invalid shared secret")
	assertNoAcceptedUserCall(t, fake)
}

func TestAuthenticateMismatchBeatsMissingGrantee(t *testing.T) {
	fake := stampOK(func() *ocm.Share {
		s := validShare("share-abc")
		s.Grantee = nil
		return s
	}())

	user, scopes, err := authenticate(context.Background(), "other-share")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	// the requested-share mismatch keeps its priority over the grantee check
	assertInvalidCredentials(t, err, "invalid shared secret")
	if strings.Contains(err.Error(), "missing grantee") {
		t.Fatalf("mismatch must take precedence, got %q", err.Error())
	}
	assertNoAcceptedUserCall(t, fake)
}

func TestAuthenticateShareTransportErrorWins(t *testing.T) {
	transportErr := errors.New("connection refused")
	fake := &fakeGW{
		shareErr: transportErr,
		shareRes: &ocm.GetOCMShareByTokenResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_OK},
			Share:  malformedShareCases()[0].share,
		},
	}
	stampGateway(fake)

	user, scopes, err := authenticate(context.Background(), "share-abc")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	if !errors.Is(err, transportErr) {
		t.Fatalf("expected the transport error to win, got %v", err)
	}
	assertNoAcceptedUserCall(t, fake)
}

func TestAuthenticateMismatchBeatsMalformedShare(t *testing.T) {
	for _, shareCase := range malformedShareCases() {
		for _, tc := range []struct {
			name        string
			requestedID string
			wantMsg     string
		}{
			{"empty request uses malformed diagnostic", "", "malformed ocm share record"},
			{"nonempty mismatch uses invalid secret", "other-share", "invalid shared secret"},
		} {
			t.Run(shareCase.name+"/"+tc.name, func(t *testing.T) {
				fake := stampOK(shareCase.share)
				user, scopes, err := authenticate(context.Background(), tc.requestedID)
				if user != nil || scopes != nil {
					t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
				}
				assertInvalidCredentials(t, err, tc.wantMsg)
				assertNoAcceptedUserCall(t, fake)
			})
		}
	}
}

func TestAuthenticateNilShareResponse(t *testing.T) {
	stampGateway(&fakeGW{}) // no share response, no transport error

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Fatalf("expected errtypes.InternalError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "missing ocm share response") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestAuthenticateNilShareStatus(t *testing.T) {
	stampGateway(&fakeGW{shareRes: &ocm.GetOCMShareByTokenResponse{}})

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Fatalf("expected errtypes.InternalError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "missing ocm share response") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func malformedShareCases() []struct {
	name  string
	share *ocm.Share
} {
	return []struct {
		name  string
		share *ocm.Share
	}{
		{"nil share", nil},
		{"nil share id", func() *ocm.Share {
			s := validShare("share-abc")
			s.Id = nil
			return s
		}()},
		{"blank share id", func() *ocm.Share {
			s := validShare("share-abc")
			s.Id = &ocm.ShareId{OpaqueId: ""}
			return s
		}()},
		{"whitespace share id", func() *ocm.Share {
			s := validShare("share-abc")
			s.Id = &ocm.ShareId{OpaqueId: "   "}
			return s
		}()},
	}
}

func TestAuthenticateRejectsMalformedShareRecords(t *testing.T) {
	for _, tc := range malformedShareCases() {
		t.Run(tc.name, func(t *testing.T) {
			fake := stampOK(tc.share)
			user, scopes, err := authenticate(context.Background(), "")
			if user != nil || scopes != nil {
				t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
			}
			assertInvalidCredentials(t, err, "malformed ocm share record")
			assertNoAcceptedUserCall(t, fake)
		})
	}
}

func TestAuthenticateRejectsMalformedGrantees(t *testing.T) {
	base := func() *ocm.Share { return validShare("share-abc") }
	cases := []struct {
		name  string
		share *ocm.Share
	}{
		{"nil grantee", func() *ocm.Share {
			s := base()
			s.Grantee = nil
			return s
		}()},
		{"grantee without oneof", func() *ocm.Share {
			s := base()
			s.Grantee = &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER}
			return s
		}()},
		{"group grantee", func() *ocm.Share {
			s := base()
			s.Grantee = &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
				Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: "group"}},
			}
			return s
		}()},
		{"invalid grantee type with user payload", func() *ocm.Share {
			s := base()
			s.Grantee.Type = provider.GranteeType_GRANTEE_TYPE_INVALID
			return s
		}()},
		{"unknown grantee enum with user payload", func() *ocm.Share {
			s := base()
			s.Grantee.Type = provider.GranteeType(99)
			return s
		}()},
		{"user type with group oneof", func() *ocm.Share {
			s := base()
			s.Grantee = &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: "group"}},
			}
			return s
		}()},
		{"typed-nil user wrapper", func() *ocm.Share {
			s := base()
			s.Grantee = &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   (*provider.Grantee_UserId)(nil),
			}
			return s
		}()},
		{"nil user payload", func() *ocm.Share {
			s := base()
			s.Grantee = &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_UserId{UserId: nil},
			}
			return s
		}()},
		{"blank user id", func() *ocm.Share {
			s := base()
			s.Grantee = &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: "", Idp: "remote.example.com"}},
			}
			return s
		}()},
		{"whitespace user id", func() *ocm.Share {
			s := base()
			s.Grantee = &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: "   ", Idp: "remote.example.com"}},
			}
			return s
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := stampOK(tc.share)
			user, scopes, err := authenticate(context.Background(), "")
			if user != nil || scopes != nil {
				t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
			}
			assertInvalidCredentials(t, err, "ocm share is missing grantee")
			assertNoAcceptedUserCall(t, fake)
		})
	}
}

func TestAuthenticateNilGranteeWithDebugLogging(t *testing.T) {
	logger := zerolog.New(io.Discard).Level(zerolog.DebugLevel)
	ctx := appctx.WithLogger(context.Background(), &logger)
	fake := stampOK(func() *ocm.Share {
		s := validShare("share-abc")
		s.Grantee = nil
		return s
	}())

	// the debug call arguments are evaluated even when logging would be
	// filtered, so a nil grantee must be rejected before the log statement
	user, scopes, err := authenticate(ctx, "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	assertInvalidCredentials(t, err, "ocm share is missing grantee")
	assertNoAcceptedUserCall(t, fake)
}

func TestAuthenticateNilGranteeWithoutDebugLogging(t *testing.T) {
	// no logger in the context: the disabled logger still evaluates arguments
	fake := stampOK(func() *ocm.Share {
		s := validShare("share-abc")
		s.Grantee = nil
		return s
	}())

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	assertInvalidCredentials(t, err, "ocm share is missing grantee")
	assertNoAcceptedUserCall(t, fake)
}

func TestAuthenticateRejectsMalformedCreators(t *testing.T) {
	cases := []struct {
		name  string
		share *ocm.Share
	}{
		{"missing creator", func() *ocm.Share {
			s := validShare("share-abc")
			s.Creator = nil
			return s
		}()},
		{"blank creator", func() *ocm.Share {
			s := validShare("share-abc")
			s.Creator = &userpb.UserId{OpaqueId: "   ", Idp: "local.example.com"}
			return s
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := stampOK(tc.share)
			user, scopes, err := authenticate(context.Background(), "")
			if user != nil || scopes != nil {
				t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
			}
			assertInvalidCredentials(t, err, "ocm share is missing creator")
			assertNoAcceptedUserCall(t, fake)
		})
	}
}

func TestAuthenticateRejectsMalformedAccessMethods(t *testing.T) {
	withMethods := func(methods []*ocm.AccessMethod) *ocm.Share {
		s := validShare("share-abc")
		s.AccessMethods = methods
		return s
	}
	cases := []struct {
		name  string
		share *ocm.Share
	}{
		{"nil method entry", withMethods([]*ocm.AccessMethod{nil})},
		{"method without term", withMethods([]*ocm.AccessMethod{{}})},
		{"typed-nil webdav wrapper", withMethods([]*ocm.AccessMethod{{Term: (*ocm.AccessMethod_WebdavOptions)(nil)}})},
		{"missing webdav options", withMethods([]*ocm.AccessMethod{{Term: &ocm.AccessMethod_WebdavOptions{}}})},
		{"nil webdav permissions", withMethods([]*ocm.AccessMethod{{Term: &ocm.AccessMethod_WebdavOptions{WebdavOptions: &ocm.WebDAVAccessMethod{}}}})},
		{"typed-nil webapp wrapper", withMethods([]*ocm.AccessMethod{{Term: (*ocm.AccessMethod_WebappOptions)(nil)}})},
		{"missing webapp options", withMethods([]*ocm.AccessMethod{{Term: &ocm.AccessMethod_WebappOptions{}}})},
		{"nil webapp permissions", withMethods([]*ocm.AccessMethod{{Term: &ocm.AccessMethod_WebappOptions{WebappOptions: &ocm.WebappAccessMethod{}}}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := stampOK(tc.share)
			user, scopes, err := authenticate(context.Background(), "")
			if user != nil || scopes != nil {
				t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
			}
			assertInvalidCredentials(t, err, "malformed ocm share access method")
			assertNoAcceptedUserCall(t, fake)
		})
	}
}

func TestAuthenticateMustExchangeTokenShare(t *testing.T) {
	s := validShare("share-abc")
	s.AccessMethods = []*ocm.AccessMethod{
		{
			Term: &ocm.AccessMethod_WebdavOptions{
				WebdavOptions: &ocm.WebDAVAccessMethod{
					Permissions:  permissions.NewViewerRole().CS3ResourcePermissions(),
					Requirements: []string{"must-exchange-token"},
				},
			},
		},
	}
	fake := stampOK(s)

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	assertInvalidCredentials(t, err, "share requires token exchange")
	assertNoAcceptedUserCall(t, fake)
}

func TestAuthenticateShareNotFound(t *testing.T) {
	stampGateway(&fakeGW{
		shareRes: &ocm.GetOCMShareByTokenResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND, Message: "share not found"},
		},
	})

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("expected errtypes.NotFound, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "share not found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestAuthenticateSharePermissionDenied(t *testing.T) {
	stampGateway(&fakeGW{
		shareRes: &ocm.GetOCMShareByTokenResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED, Message: "share denied"},
		},
	})

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	assertInvalidCredentials(t, err, "share denied")
}

func TestAuthenticateAcceptedUserTransportError(t *testing.T) {
	userErr := errors.New("accepted user lookup failed")
	stampGateway(&fakeGW{
		shareRes: &ocm.GetOCMShareByTokenResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: validShare("share-abc")},
		userErr:  userErr,
	})

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	if !errors.Is(err, userErr) {
		t.Fatalf("expected the transport error to be returned, got %v", err)
	}
}

func TestAuthenticateNilAcceptedUserResponse(t *testing.T) {
	stampGateway(&fakeGW{
		shareRes: &ocm.GetOCMShareByTokenResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: validShare("share-abc")},
	})

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Fatalf("expected errtypes.InternalError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "missing accepted user response") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestAuthenticateNilAcceptedUserStatus(t *testing.T) {
	stampGateway(&fakeGW{
		shareRes: &ocm.GetOCMShareByTokenResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: validShare("share-abc")},
		userRes:  &ocminvite.GetAcceptedUserResponse{},
	})

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Fatalf("expected errtypes.InternalError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "missing accepted user response") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestAuthenticateAcceptedUserNotFoundUsesUserMessage(t *testing.T) {
	stampGateway(&fakeGW{
		shareRes: &ocm.GetOCMShareByTokenResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_OK, Message: "share lookup message"},
			Share:  validShare("share-abc"),
		},
		userRes: &ocminvite.GetAcceptedUserResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND, Message: "accepted user message"},
		},
	})

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	if _, ok := err.(errtypes.NotFound); !ok {
		t.Fatalf("expected errtypes.NotFound, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "accepted user message") {
		t.Fatalf("the NOT_FOUND error must carry the accepted-user status message, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "share lookup message") {
		t.Fatalf("the NOT_FOUND error must not carry the share status message, got %q", err.Error())
	}
}

func TestAuthenticateAcceptedUserOtherNonOK(t *testing.T) {
	stampGateway(&fakeGW{
		shareRes: &ocm.GetOCMShareByTokenResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: validShare("share-abc")},
		userRes: &ocminvite.GetAcceptedUserResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED, Message: "denied user"},
		},
	})

	user, scopes, err := authenticate(context.Background(), "")
	if user != nil || scopes != nil {
		t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Fatalf("expected errtypes.InternalError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "denied user") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestAuthenticateMalformedAcceptedUserPayload(t *testing.T) {
	cases := []struct {
		name    string
		userRes *ocminvite.GetAcceptedUserResponse
	}{
		{"nil remote user", &ocminvite.GetAcceptedUserResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		}},
		{"remote user without id", &ocminvite.GetAcceptedUserResponse{
			Status:     &rpc.Status{Code: rpc.Code_CODE_OK},
			RemoteUser: &userpb.User{},
		}},
		{"remote user with blank id", &ocminvite.GetAcceptedUserResponse{
			Status:     &rpc.Status{Code: rpc.Code_CODE_OK},
			RemoteUser: &userpb.User{Id: &userpb.UserId{OpaqueId: "   "}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeGW{
				shareRes: &ocm.GetOCMShareByTokenResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: validShare("share-abc")},
				userRes:  tc.userRes,
			}
			stampGateway(fake)
			user, scopes, err := authenticate(context.Background(), "")
			if user != nil || scopes != nil {
				t.Fatalf("expected nil auth outputs, got %v %v", user, scopes)
			}
			if _, ok := err.(errtypes.InternalError); !ok {
				t.Fatalf("expected errtypes.InternalError, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), "malformed accepted user response") {
				t.Fatalf("unexpected error message: %v", err)
			}
			if _, acceptedCalls := fake.counts(); acceptedCalls != 1 {
				t.Fatalf("expected exactly one GetAcceptedUser call, got %d", acceptedCalls)
			}
		})
	}
}
