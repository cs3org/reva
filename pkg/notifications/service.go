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
	"time"

	"github.com/google/uuid"

	"github.com/cs3org/reva/v3/pkg/notifications/backends"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
)

// SendService implements the gateway-side SendNotification logic.
type SendService struct {
	backend backends.Backend
	limiter RateLimiter
	now     func() time.Time
	newID   func() string
}

// NewSendService creates a gateway-side notification sender.
func NewSendService(backend backends.Backend, limiter RateLimiter) *SendService {
	if limiter == nil {
		limiter = NoopRateLimiter{}
	}

	return &SendService{
		backend: backend,
		limiter: limiter,
		now:     time.Now,
		newID:   func() string { return uuid.NewString() },
	}
}

// SendNotification validates, rate-limits and publishes a notification.
func (s *SendService) SendNotification(ctx context.Context, req model.SendRequest) (*model.Envelope, error) {
	if s == nil || s.backend == nil {
		return nil, errors.New("notification backend is not configured")
	}
	if err := validateSendRequest(req); err != nil {
		return nil, err
	}
	if err := s.limiter.Allow(ctx, req.SubmittingUser); err != nil {
		return nil, err
	}

	env := model.Envelope{
		ID:             s.newID(),
		EventType:      req.EventType,
		SubmittingUser: req.SubmittingUser,
		Sender:         req.Sender,
		Recipients:     append([]string(nil), req.Recipients...),
		TemplateData:   cloneMap(req.TemplateData),
		SubmittedAt:    s.now(),
	}

	if err := s.backend.Publish(ctx, env); err != nil {
		return nil, err
	}
	return &env, nil
}

func validateSendRequest(req model.SendRequest) error {
	if req.EventType == "" {
		return errors.New("event type is required")
	}
	if req.SubmittingUser == "" {
		return errors.New("submitting user is required")
	}
	if len(req.Recipients) == 0 {
		return errors.New("at least one recipient is required")
	}
	return nil
}
