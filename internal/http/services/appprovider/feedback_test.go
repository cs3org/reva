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

package appprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"google.golang.org/grpc"
)

type feedbackGateway struct {
	gateway.GatewayAPIClient
	published *gateway.PublishEventRequest
	status    rpc.Code
	err       error
}

func (g *feedbackGateway) PublishEvent(_ context.Context, req *gateway.PublishEventRequest, _ ...grpc.CallOption) (*gateway.PublishEventResponse, error) {
	g.published = req
	if g.err != nil {
		return nil, g.err
	}
	code := g.status
	if code == rpc.Code_CODE_INVALID {
		code = rpc.Code_CODE_OK
	}
	return &gateway.PublishEventResponse{Status: &rpc.Status{Code: code}, EventId: "evt-1"}, nil
}

// TestHandleFeedbackPublishesEvent drives a plain-text feedback submission
// through the handler and asserts the published event carries the submitter's
// name and the text, addressed to the configured recipient.
func TestHandleFeedbackPublishesEvent(t *testing.T) {
	sharedconf.Init(&revaconfig.Shared{JWTSecret: "feedback-test-secret"})

	endpoint := "feedback-" + t.Name()
	gw := &feedbackGateway{}
	stampGateway(gw)

	s := &svc{conf: &Config{GatewaySvc: endpoint, FeedbackRecipient: "cernbox-admins@cern.ch"}}
	submitter := mentionUser("einstein", "einstein@cern.ch")

	req := httptest.NewRequest(http.MethodPost, "/app/feedback", strings.NewReader("The upload dialog is confusing.\nPlease fix."))
	req = req.WithContext(appctx.ContextSetUser(req.Context(), submitter))
	rec := httptest.NewRecorder()

	s.handleFeedback(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if gw.published == nil {
		t.Fatal("no event was published")
	}
	if got := gw.published.GetEvent().GetType(); got != model.EventFeedback {
		t.Fatalf("event type = %q, want %q", got, model.EventFeedback)
	}

	recipients, templateData, err := notifications.DecodeEvent(gw.published.GetEvent())
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if len(recipients) != 1 || recipients[0] != "cernbox-admins@cern.ch" {
		t.Fatalf("recipients = %+v, want [cernbox-admins@cern.ch]", recipients)
	}
	if templateData["submitter_display_name"] != "einstein display" {
		t.Errorf("submitter_display_name = %v, want %q", templateData["submitter_display_name"], "einstein display")
	}
	if templateData["text"] != "The upload dialog is confusing.\nPlease fix." {
		t.Errorf("text = %v, want the submitted feedback", templateData["text"])
	}
}

func TestHandleFeedbackValidation(t *testing.T) {
	sharedconf.Init(&revaconfig.Shared{JWTSecret: "feedback-test-secret"})
	submitter := mentionUser("einstein", "einstein@cern.ch")

	tests := []struct {
		name      string
		recipient string
		user      bool
		body      string
		wantCode  int
	}{
		{"unconfigured recipient", "", true, "hi", http.StatusInternalServerError},
		{"missing user", "to@cern.ch", false, "hi", http.StatusUnauthorized},
		{"empty text", "to@cern.ch", true, "   ", http.StatusBadRequest},
		{"too long", "to@cern.ch", true, strings.Repeat("x", maxFeedbackTextLen+1), http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := "feedback-" + t.Name()
			stampGateway(&feedbackGateway{})
			s := &svc{conf: &Config{GatewaySvc: endpoint, FeedbackRecipient: tt.recipient}}

			req := httptest.NewRequest(http.MethodPost, "/app/feedback", strings.NewReader(tt.body))
			if tt.user {
				req = req.WithContext(appctx.ContextSetUser(req.Context(), submitter))
			}
			rec := httptest.NewRecorder()

			s.handleFeedback(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantCode, rec.Body.String())
			}
		})
	}
}

func TestFeedbackEndpointIsProtected(t *testing.T) {
	s := &svc{}
	for _, path := range s.Unprotected() {
		if path == "/feedback" {
			t.Fatalf("/feedback must not be listed as unprotected")
		}
	}
}
