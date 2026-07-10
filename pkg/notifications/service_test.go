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

package notifications

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/notifications/handlers"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
)

type recordingBackend struct {
	envelopes []model.Envelope
}

func (b *recordingBackend) Publish(_ context.Context, envelope model.Envelope) error {
	b.envelopes = append(b.envelopes, envelope)
	return nil
}

func TestSendNotificationRateLimitsPerSubmittingUser(t *testing.T) {
	backend := &recordingBackend{}
	limiter := NewFixedWindowRateLimiter(1, time.Minute)
	svc := NewSendService(backend, limiter)

	req := model.SendRequest{
		Type:           model.TypeDirect,
		SubmittingUser: "alice",
		Recipients:     []string{"bob@example.org"},
		Handlers:       []string{handlers.EmailHandlerName},
	}

	if _, err := svc.SendNotification(context.Background(), req); err != nil {
		t.Fatalf("first send failed: %v", err)
	}

	_, err := svc.SendNotification(context.Background(), req)
	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("second send error = %v, want RateLimitError", err)
	}

	req.SubmittingUser = "carol"
	if _, err := svc.SendNotification(context.Background(), req); err != nil {
		t.Fatalf("different submitting user should not be limited: %v", err)
	}
}

func TestSendNotificationRequiresDedupKeyForAccumulated(t *testing.T) {
	svc := NewSendService(&recordingBackend{}, NoopRateLimiter{})

	_, err := svc.SendNotification(context.Background(), model.SendRequest{
		Type:           model.TypeAccumulated,
		SubmittingUser: "alice",
		Recipients:     []string{"bob@example.org"},
		Handlers:       []string{handlers.EmailHandlerName},
		Accumulation: model.AccumulationPolicy{
			WindowSeconds: 60,
		},
	})
	if err == nil {
		t.Fatal("expected missing dedup key to fail")
	}
}
