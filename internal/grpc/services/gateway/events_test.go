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
	"github.com/cs3org/reva/v3/internal/grpc/services/gateway/ratelimiters"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
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
