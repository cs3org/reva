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

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/cs3api"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
)

// PublishEvent accepts an event from a service and publishes it to the
// notification backend. The submitting user and the sender are taken from the
// authenticated context, never from the request.
func (s *svc) PublishEvent(ctx context.Context, req *gateway.PublishEventRequest) (*gateway.PublishEventResponse, error) {
	if s.notificationSender == nil {
		return &gateway.PublishEventResponse{
			Status: status.NewUnimplemented(ctx, errtypes.NotSupported("notifications"), "gateway: notifications are not configured"),
		}, nil
	}

	u, ok := appctx.ContextGetUser(ctx)
	if !ok || u == nil {
		return &gateway.PublishEventResponse{
			Status: status.NewUnauthenticated(ctx, errtypes.UserRequired("gateway: no user in context"), "gateway: cannot publish event without a user"),
		}, nil
	}

	sendReq, err := cs3api.SendRequestFromEvent(req.GetEvent(), cs3api.UserIDString(u.GetId()), u.GetMail())
	if err != nil {
		return &gateway.PublishEventResponse{
			Status: status.NewInvalid(ctx, err.Error()),
		}, nil
	}

	env, err := s.notificationSender.SendNotification(ctx, sendReq)
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
	var rateLimitErr *notifications.RateLimitError
	switch {
	case errors.Is(err, notifications.ErrInvalidRequest):
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
