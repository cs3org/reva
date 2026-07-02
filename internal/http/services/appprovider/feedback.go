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
	"fmt"
	"io"
	"net/http"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/service"
)

const (
	maxFeedbackBodyBytes = 1 << 20
	maxFeedbackTextLen   = 10000
)

// handleFeedback accepts a plain-text feedback message from the authenticated
// user and publishes a feedback notification event, which the notifications
// service delivers by email to the configured recipient. The email carries the
// submitter's name and the submitted text.
func (s *svc) handleFeedback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	if s.conf.FeedbackRecipient == "" {
		writeError(w, r, appErrorServerError, "feedback endpoint is not configured", nil)
		return
	}

	submitter, ok := appctx.ContextGetUser(ctx)
	if !ok || submitter == nil {
		writeError(w, r, appErrorUnauthenticated, "missing authenticated user", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxFeedbackBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, appErrorInvalidParameter, "could not read request body", nil)
		return
	}

	text := strings.TrimSpace(string(raw))
	if text == "" {
		writeError(w, r, appErrorInvalidParameter, "missing feedback text", nil)
		return
	}
	if len(text) > maxFeedbackTextLen {
		writeError(w, r, appErrorInvalidParameter, "feedback text is too long", nil)
		return
	}

	client, err := service.Gateway(ctx)
	if err != nil {
		writeError(w, r, appErrorServerError, "error getting grpc gateway client", err)
		return
	}

	submitterName := submitter.GetDisplayName()
	if submitterName == "" {
		submitterName = submitter.GetUsername()
	}
	templateData := map[string]any{
		"submitter_display_name": submitterName,
		"submitter_username":     submitter.GetUsername(),
		"submitter_mail":         submitter.GetMail(),
		"text":                   text,
	}

	// PublishEvent is restricted to reva daemons. Re-sign this request's token with
	// the machine scope, keeping the acting user (the submitter) unchanged.
	publishCtx, err := scope.ContextWithMachineScope(ctx)
	if err != nil {
		writeError(w, r, appErrorServerError, "failed to elevate context for feedback notification", err)
		return
	}

	// The feedback mailbox is a configured address, not an account, so it is
	// only known by its mail.
	recipients := []*userpb.User{{Mail: s.conf.FeedbackRecipient}}
	event := notifications.EncodeEvent(model.EventFeedback, recipients, templateData)
	res, err := client.PublishEvent(publishCtx, &gateway.PublishEventRequest{Event: event})
	if err != nil {
		writeError(w, r, appErrorServerError, "failed to send feedback notification", err)
		return
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		writeError(w, r, appErrorServerError, fmt.Sprintf("gateway rejected feedback notification: %s (%s)", code, res.GetStatus().GetMessage()), nil)
		return
	}

	log.Info().Str("submitter", submitter.GetUsername()).Msg("feedback submitted")
	w.WriteHeader(http.StatusAccepted)
}
