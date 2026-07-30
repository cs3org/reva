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

package gateway

import (
	"context"
	"testing"
	"time"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/internal/grpc/interceptors/auth"
	"github.com/cs3org/reva/v3/internal/grpc/services/gateway/ratelimiters"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc"
)

type recordingBackend struct {
	envelopes []model.Envelope
}

func (b *recordingBackend) Publish(_ context.Context, envelope model.Envelope) error {
	b.envelopes = append(b.envelopes, envelope)
	return nil
}

// daemonCtx builds a context authenticated as user and carrying the machine
// scope, as a re-signed daemon token would.
func daemonCtx(user *userpb.User) context.Context {
	ctx := appctx.ContextSetUser(context.Background(), user)
	return appctx.ContextSetScopes(ctx, map[string]*authpb.Scope{scope.MachineScope: {}})
}

func TestPublishEventRequiresMachineScope(t *testing.T) {
	s := &svc{eventBackend: &recordingBackend{}, eventLimiter: ratelimiters.Noop{}}
	event := notifications.EncodeEvent("share.created", []string{"bob@example.org"}, nil)
	user := &userpb.User{Id: &userpb.UserId{OpaqueId: "alice"}, Mail: "alice@example.org"}

	// A normal user token (no machine scope) is rejected.
	ctx := appctx.ContextSetUser(context.Background(), user)
	res, err := s.PublishEvent(ctx, &gateway.PublishEventRequest{Event: event})
	if err != nil {
		t.Fatalf("PublishEvent returned error: %v", err)
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_PERMISSION_DENIED {
		t.Fatalf("without machine scope: code = %v, want PERMISSION_DENIED", code)
	}

	// The same context re-signed with the machine scope is accepted.
	res, err = s.PublishEvent(daemonCtx(user), &gateway.PublishEventRequest{Event: event})
	if err != nil {
		t.Fatalf("PublishEvent returned error: %v", err)
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		t.Fatalf("with machine scope: code = %v, msg = %q, want OK", code, res.GetStatus().GetMessage())
	}
}

func TestPublishEventRateLimitsPerSubmittingUser(t *testing.T) {
	s := &svc{eventBackend: &recordingBackend{}, eventLimiter: ratelimiters.NewFixedWindow(1, time.Minute)}
	event := notifications.EncodeEvent("share.created", []string{"bob@example.org"}, nil)

	alice := daemonCtx(&userpb.User{Id: &userpb.UserId{OpaqueId: "alice"}})
	res, err := s.PublishEvent(alice, &gateway.PublishEventRequest{Event: event})
	if err != nil {
		t.Fatalf("PublishEvent returned error: %v", err)
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		t.Fatalf("first send: code = %v, want OK", code)
	}
	res, _ = s.PublishEvent(alice, &gateway.PublishEventRequest{Event: event})
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_RESOURCE_EXHAUSTED {
		t.Fatalf("second send for same user: code = %v, want RESOURCE_EXHAUSTED", code)
	}

	// A different submitting user is not limited.
	carol := daemonCtx(&userpb.User{Id: &userpb.UserId{OpaqueId: "carol"}})
	res, _ = s.PublishEvent(carol, &gateway.PublishEventRequest{Event: event})
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		t.Fatalf("different submitting user: code = %v, want OK", code)
	}
}

func TestPublishEventPublishesEventOnly(t *testing.T) {
	backend := &recordingBackend{}
	s := &svc{eventBackend: backend, eventLimiter: ratelimiters.Noop{}}
	event := notifications.EncodeEvent("office.mention", []string{"bob@example.org"}, map[string]any{"document_id": "doc-1"})

	ctx := daemonCtx(&userpb.User{Id: &userpb.UserId{OpaqueId: "alice"}, Mail: "alice@example.org"})
	res, err := s.PublishEvent(ctx, &gateway.PublishEventRequest{Event: event})
	if err != nil {
		t.Fatalf("PublishEvent returned error: %v", err)
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		t.Fatalf("code = %v, msg = %q, want OK", code, res.GetStatus().GetMessage())
	}

	if len(backend.envelopes) != 1 {
		t.Fatalf("published envelopes = %d, want 1", len(backend.envelopes))
	}
	envelope := backend.envelopes[0]
	if envelope.EventType != "office.mention" {
		t.Fatalf("event type = %q, want office.mention", envelope.EventType)
	}
	if envelope.SubmittingUser == "" {
		t.Fatal("submitting user is empty, want the context user")
	}
	if envelope.Type != "" || envelope.DedupKey != "" || len(envelope.Handlers) != 0 {
		t.Fatalf("envelope contains resolved delivery policy: %+v", envelope)
	}
}

const testJWTSecret = "events-e2e-secret"

// producerToken reproduces what an HTTP producer (ocdav/ocs) does: it holds an
// already-authenticated context for some acting user carrying that user's own
// base scopes, then re-signs it with the machine scope through
// ContextWithMachineScope. It returns the token that would travel to the gateway
// over the wire.
func producerToken(t *testing.T, u *userpb.User, baseScopes map[string]*authpb.Scope) string {
	t.Helper()

	ctx := context.Background()
	ctx = appctx.ContextSetUser(ctx, u)
	ctx = appctx.ContextSetScopes(ctx, baseScopes)
	ctx = appctx.ContextSetToken(ctx, mintToken(t, u, baseScopes))

	elevated, err := scope.ContextWithMachineScope(ctx)
	if err != nil {
		t.Fatalf("elevate to machine scope: %v", err)
	}
	tkn, ok := appctx.ContextGetToken(elevated)
	if !ok {
		t.Fatal("no token on elevated context")
	}
	return tkn
}

func mintToken(t *testing.T, u *userpb.User, scopes map[string]*authpb.Scope) string {
	t.Helper()
	tm, err := jwt.New(map[string]any{"secret": testJWTSecret})
	if err != nil {
		t.Fatalf("new token manager: %v", err)
	}
	tkn, err := tm.MintToken(context.Background(), u, scopes)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return tkn
}

// callPublishEvent drives a token through the real gRPC auth interceptor and, on
// success, into the real PublishEvent handler, exactly as a live gateway would.
// It returns the handler response (nil when the interceptor rejected the token)
// and the interceptor error.
func callPublishEvent(t *testing.T, s *svc, tkn string, req *gateway.PublishEventRequest) (*gateway.PublishEventResponse, error) {
	t.Helper()

	interceptor, err := auth.NewUnary(map[string]any{
		"token_manager":  "jwt",
		"token_managers": map[string]any{"jwt": map[string]any{"secret": testJWTSecret}},
	}, nil)
	if err != nil {
		t.Fatalf("build auth interceptor: %v", err)
	}

	ctx := appctx.ContextSetToken(context.Background(), tkn)
	resp, err := interceptor(ctx, req, &grpc.UnaryServerInfo{
		FullMethod: "/cs3.gateway.v1beta1.GatewayAPI/PublishEvent",
	}, func(ctx context.Context, req any) (any, error) {
		return s.PublishEvent(ctx, req.(*gateway.PublishEventRequest))
	})
	if err != nil {
		return nil, err
	}
	return resp.(*gateway.PublishEventResponse), nil
}

func uploadEventRequest() *gateway.PublishEventRequest {
	return &gateway.PublishEventRequest{
		Event: notifications.EncodeEvent(model.EventUpload, []string{"owner@example.org"}, map[string]any{
			"resource_name": "report.pdf",
		}),
	}
}

// TestPublishEventEndToEnd drives a token minted the way each auth manager mints
// it through the real auth interceptor and the real PublishEvent handler. Unlike
// the handler-only tests above (which inject the machine scope straight into the
// context), this exercises token dismantling and scope verification, the layer
// where a public-link or lightweight caller (which has no owner scope) was being
// rejected with "core access token is invalid". It is the regression guard for
// that whole class of caller.
func TestPublishEventEndToEnd(t *testing.T) {
	sharedconf.Init(&revaconfig.Shared{JWTSecret: testJWTSecret})
	if sharedconf.GetJWTSecret("") != testJWTSecret {
		t.Skipf("shared jwt secret already initialised to a different value by another test")
	}

	primaryUser := &userpb.User{
		Id:   &userpb.UserId{Idp: "https://idp.example.org", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_PRIMARY},
		Mail: "einstein@example.org",
	}
	publicUser := &userpb.User{
		Id: &userpb.UserId{OpaqueId: "publiclink:871283", Type: userpb.UserType_USER_TYPE_GUEST},
	}
	lightweightUser := &userpb.User{
		Id:   &userpb.UserId{Idp: "https://idp.example.org", OpaqueId: "guest-1", Type: userpb.UserType_USER_TYPE_LIGHTWEIGHT},
		Mail: "guest@example.org",
	}

	ownerScopes, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatalf("owner scope: %v", err)
	}
	publicShareScopes, err := scope.AddPublicShareScope(&link.PublicShare{
		Id:         &link.PublicShareId{OpaqueId: "871283"},
		Token:      "VB8CsMtt3796SDo",
		ResourceId: &provider.ResourceId{StorageId: "eoshome-j", OpaqueId: "345842884"},
	}, authpb.Role_ROLE_UPLOADER, nil)
	if err != nil {
		t.Fatalf("public share scope: %v", err)
	}
	lightweightScopes, err := scope.AddLightweightAccountScope(authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatalf("lightweight scope: %v", err)
	}

	accepted := []struct {
		name       string
		user       *userpb.User
		baseScopes map[string]*authpb.Scope
	}{
		{"primary user", primaryUser, ownerScopes},
		{"public link user", publicUser, publicShareScopes},
		{"lightweight user", lightweightUser, lightweightScopes},
	}

	for _, tt := range accepted {
		t.Run("accepts elevated "+tt.name, func(t *testing.T) {
			backend := &recordingBackend{}
			s := &svc{eventBackend: backend, eventLimiter: ratelimiters.Noop{}}

			resp, err := callPublishEvent(t, s, producerToken(t, tt.user, tt.baseScopes), uploadEventRequest())
			if err != nil {
				t.Fatalf("interceptor rejected elevated token: %v", err)
			}
			if code := resp.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
				t.Fatalf("status = %s (%s), want OK", code, resp.GetStatus().GetMessage())
			}
			if len(backend.envelopes) != 1 {
				t.Fatalf("published = %d envelopes, want 1", len(backend.envelopes))
			}
			env := backend.envelopes[0]
			if env.EventType != model.EventUpload {
				t.Errorf("event type = %q, want %q", env.EventType, model.EventUpload)
			}
			if want := notifications.UserIDString(tt.user.GetId()); env.SubmittingUser != want {
				t.Errorf("submitting user = %q, want %q", env.SubmittingUser, want)
			}
		})
	}

	t.Run("rejects public link user without machine scope", func(t *testing.T) {
		backend := &recordingBackend{}
		s := &svc{eventBackend: backend, eventLimiter: ratelimiters.Noop{}}

		tkn := mintToken(t, publicUser, publicShareScopes)
		if _, err := callPublishEvent(t, s, tkn, uploadEventRequest()); err == nil {
			t.Fatal("expected interceptor to reject a public-share-only token for PublishEvent")
		}
		if len(backend.envelopes) != 0 {
			t.Fatalf("published = %d envelopes, want 0", len(backend.envelopes))
		}
	})

	t.Run("rejects primary user without machine scope", func(t *testing.T) {
		backend := &recordingBackend{}
		s := &svc{eventBackend: backend, eventLimiter: ratelimiters.Noop{}}

		// An owner-scoped token passes scope verification (owner grants all), so
		// the request reaches the handler, which must still refuse it for lacking
		// the machine scope.
		resp, err := callPublishEvent(t, s, mintToken(t, primaryUser, ownerScopes), uploadEventRequest())
		if err != nil {
			t.Fatalf("interceptor error: %v", err)
		}
		if code := resp.GetStatus().GetCode(); code != rpc.Code_CODE_PERMISSION_DENIED {
			t.Fatalf("status = %s, want PERMISSION_DENIED", code)
		}
		if len(backend.envelopes) != 0 {
			t.Fatalf("published = %d envelopes, want 0", len(backend.envelopes))
		}
	})
}
