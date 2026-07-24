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
	"errors"
	"fmt"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/services/gateway/ratelimiters"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/backends"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/google/uuid"
)

// PublishEvent accepts an event from a trusted daemon and publishes it to the
// notification backend. It is restricted to callers authenticated via machine
// auth (verified through the machine scope). The submitting user and the sender
// are taken from that authenticated context, never from the request.
func (s *svc) PublishEvent(ctx context.Context, req *gateway.PublishEventRequest) (*gateway.PublishEventResponse, error) {
	if s.notificationSender == nil {
		return &gateway.PublishEventResponse{
			Status: status.NewUnimplemented(ctx, errtypes.NotSupported("notifications"), "gateway: notifications are not configured"),
		}, nil
	}

	scopes, ok := appctx.ContextGetScopes(ctx)
	if _, isMachine := scopes[scope.MachineScope]; !ok || !isMachine {
		return &gateway.PublishEventResponse{
			Status: status.NewPermissionDenied(ctx, errtypes.PermissionDenied("machine auth required"), "gateway: PublishEvent requires machine authentication"),
		}, nil
	}

	u, ok := appctx.ContextGetUser(ctx)
	if !ok || u == nil {
		return &gateway.PublishEventResponse{
			Status: status.NewUnauthenticated(ctx, errtypes.UserRequired("gateway: no user in context"), "gateway: cannot publish event without a user"),
		}, nil
	}

	recipients, templateData, err := notifications.DecodeEvent(req.GetEvent())
	if err != nil {
		return &gateway.PublishEventResponse{
			Status: status.NewInvalid(ctx, err.Error()),
		}, nil
	}

	sendReq := notifications.SendRequest{
		EventType:      req.GetEvent().GetType(),
		SubmittingUser: notifications.UserIDString(u.GetId()),
		Sender:         u.GetMail(),
		Recipients:     recipients,
		TemplateData:   templateData,
	}

	env, err := s.notificationSender.publish(ctx, sendReq)
	if err != nil {
		return &gateway.PublishEventResponse{
			Status: publishEventStatus(ctx, err),
		}, nil
	}

	return &gateway.PublishEventResponse{
		Status:  status.NewOK(ctx),
		EventId: env.ID,
	}, nil
}

func publishEventStatus(ctx context.Context, err error) *rpc.Status {
	var rateLimitErr *ratelimiters.LimitError
	switch {
	case errors.Is(err, errInvalidRequest):
		return status.NewInvalid(ctx, err.Error())
	case errors.As(err, &rateLimitErr):
		return &rpc.Status{
			Code:    rpc.Code_CODE_RESOURCE_EXHAUSTED,
			Message: fmt.Sprintf("gateway: notification rate limit exceeded, retry after %s", rateLimitErr.RetryAfter),
		}
	default:
		return status.NewInternal(ctx, err, "gateway: error publishing event")
	}
}

// notificationPublisher validates, rate-limits and publishes accepted events to
// the notification backend.
type notificationPublisher struct {
	backend backends.Backend
	limiter ratelimiters.Limiter
	now     func() time.Time
	newID   func() string
}

func newNotificationPublisher(backend backends.Backend, limiter ratelimiters.Limiter) *notificationPublisher {
	if limiter == nil {
		limiter = ratelimiters.Noop{}
	}

	return &notificationPublisher{
		backend: backend,
		limiter: limiter,
		now:     time.Now,
		newID:   func() string { return uuid.NewString() },
	}
}

// errInvalidRequest marks a notification that was rejected because the request
// itself is malformed, as opposed to a backend failure.
var errInvalidRequest = errors.New("invalid notification request")

func (p *notificationPublisher) publish(ctx context.Context, req notifications.SendRequest) (*model.Envelope, error) {
	if p == nil || p.backend == nil {
		return nil, errors.New("notification backend is not configured")
	}
	if err := validateSendRequest(req); err != nil {
		return nil, err
	}
	if err := p.limiter.Allow(ctx, req.SubmittingUser); err != nil {
		return nil, err
	}

	env := model.Envelope{
		ID:             p.newID(),
		EventType:      req.EventType,
		SubmittingUser: req.SubmittingUser,
		Sender:         req.Sender,
		Recipients:     append([]string(nil), req.Recipients...),
		TemplateData:   cloneMap(req.TemplateData),
		SubmittedAt:    p.now(),
	}

	if err := p.backend.Publish(ctx, env); err != nil {
		return nil, err
	}
	return &env, nil
}

func validateSendRequest(req notifications.SendRequest) error {
	if req.EventType == "" {
		return fmt.Errorf("%w: event type is required", errInvalidRequest)
	}
	if req.SubmittingUser == "" {
		return fmt.Errorf("%w: submitting user is required", errInvalidRequest)
	}
	if len(req.Recipients) == 0 {
		return fmt.Errorf("%w: at least one recipient is required", errInvalidRequest)
	}
	return nil
}

// senderConfig configures the backend the gateway publishes accepted
// notification events to.
type senderConfig struct {
	NATS backends.NATSConfig `mapstructure:"nats"`

	// Keep the flat NATS keys accepted by existing configs while the backend
	// config is being moved under a nested notifications.nats section.
	NATSAddress string `mapstructure:"nats_address"`
	NATSToken   string `mapstructure:"nats_token"`
}

func (c *senderConfig) ApplyDefaults() {
	if c.NATS.Address == "" {
		c.NATS.Address = c.NATSAddress
	}
	if c.NATS.Token == "" {
		c.NATS.Token = c.NATSToken
	}
}

// newSender creates a notificationPublisher and close function from service
// config. A nil publisher means notifications are not configured, and
// PublishEvent is rejected.
func newSender(ctx context.Context, m map[string]any) (*notificationPublisher, func() error, error) {
	if len(m) == 0 {
		return nil, nil, nil
	}

	var c senderConfig
	if err := cfg.Decode(m, &c); err != nil {
		return nil, nil, err
	}
	c.ApplyDefaults()
	if c.NATS.Address == "" {
		return nil, nil, nil
	}

	backend, err := backends.NewNATSBackend(c.NATS, *appctx.GetLogger(ctx))
	if err != nil {
		return nil, nil, err
	}

	return newNotificationPublisher(backend, ratelimiters.Noop{}), backend.Close, nil
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}

	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
