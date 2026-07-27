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
	"github.com/google/uuid"
)

// PublishEvent accepts an event from a trusted daemon and publishes it to the
// event backend. It is restricted to callers carrying the machine scope (reva
// daemons); the submitting user and the sender are taken from the authenticated
// context, never from the request.
func (s *svc) PublishEvent(ctx context.Context, req *gateway.PublishEventRequest) (*gateway.PublishEventResponse, error) {
	if s.eventBackend == nil {
		return &gateway.PublishEventResponse{
			Status: status.NewUnimplemented(ctx, errtypes.NotSupported("events"), "gateway: event backend is not configured"),
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
	if req.GetEvent().GetType() == "" {
		return &gateway.PublishEventResponse{
			Status: status.NewInvalid(ctx, "gateway: event type is required"),
		}, nil
	}
	if len(recipients) == 0 {
		return &gateway.PublishEventResponse{
			Status: status.NewInvalid(ctx, "gateway: at least one recipient is required"),
		}, nil
	}

	env := model.Envelope{
		ID:             uuid.NewString(),
		EventType:      req.GetEvent().GetType(),
		SubmittingUser: notifications.UserIDString(u.GetId()),
		Sender:         u.GetMail(),
		Recipients:     recipients,
		TemplateData:   templateData,
		SubmittedAt:    time.Now().UTC(),
	}

	if err := s.eventLimiter.Allow(ctx, env.SubmittingUser); err != nil {
		var limitErr *ratelimiters.LimitError
		if errors.As(err, &limitErr) {
			return &gateway.PublishEventResponse{
				Status: &rpc.Status{
					Code:    rpc.Code_CODE_RESOURCE_EXHAUSTED,
					Message: fmt.Sprintf("gateway: event rate limit exceeded, retry after %s", limitErr.RetryAfter),
				},
			}, nil
		}
		return &gateway.PublishEventResponse{
			Status: status.NewInternal(ctx, err, "gateway: error rate-limiting event"),
		}, nil
	}

	if err := s.eventBackend.Publish(ctx, env); err != nil {
		return &gateway.PublishEventResponse{
			Status: status.NewInternal(ctx, err, "gateway: error publishing event"),
		}, nil
	}

	return &gateway.PublishEventResponse{
		Status:  status.NewOK(ctx),
		EventId: env.ID,
	}, nil
}

// eventBackendConfig configures the NATS backend the gateway publishes accepted
// events to.
type eventBackendConfig struct {
	NATS backends.NATSConfig `mapstructure:"nats"`
}

// newEventBackend builds the NATS backend the gateway publishes events to. An
// empty NATS address means the backend is not configured, so a nil backend is
// returned and PublishEvent is rejected.
func newEventBackend(ctx context.Context, c eventBackendConfig) (backends.Backend, func() error, error) {
	if c.NATS.Address == "" {
		return nil, nil, nil
	}

	backend, err := backends.NewNATSBackend(c.NATS, *appctx.GetLogger(ctx))
	if err != nil {
		return nil, nil, err
	}

	return backend, backend.Close, nil
}
