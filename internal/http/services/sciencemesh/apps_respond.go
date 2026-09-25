// Copyright 2018-2024 CERN
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

package sciencemesh

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func redactLaunchError(err error, secret, token string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}

	var notFound errtypes.NotFound
	var badRequest errtypes.BadRequest
	var invalidCredentials errtypes.InvalidCredentials
	var permissionDenied errtypes.PermissionDenied
	var internal errtypes.InternalError
	switch {
	case errors.As(err, &notFound):
		return errtypes.NotFound(safeDetail(string(notFound), secret, token, "received share not found"))
	case errors.As(err, &badRequest):
		return errtypes.BadRequest(safeDetail(string(badRequest), secret, token, "invalid parameter"))
	case errors.As(err, &invalidCredentials):
		return errtypes.InvalidCredentials(safeDetail(string(invalidCredentials), secret, token, "unauthenticated"))
	case errors.As(err, &permissionDenied):
		return errtypes.PermissionDenied(safeDetail(string(permissionDenied), secret, token, "untrusted service"))
	case errors.As(err, &internal):
		return errtypes.InternalError(safeInternalDetail(string(internal), secret, token))
	default:
		return errtypes.InternalError("launch failed")
	}
}

// safeInternalDetails are fixed messages this receiver authors. Anything else,
// including a remote body or URL, becomes the generic launch failure.
var safeInternalDetails = map[string]struct{}{
	"missing share response":                        {},
	"gateway client is not available":               {},
	"launch client is not available":                {},
	"received share lookup failed":                  {},
	"token exchange returned an empty access token": {},
	"launch failed":                                 {},
}

func safeInternalDetail(detail, secret, token string) string {
	if _, ok := safeInternalDetails[detail]; !ok {
		return "launch failed"
	}
	return safeDetail(detail, secret, token, "launch failed")
}

func safeDetail(detail, secret, token, fallback string) string {
	if containsSensitive(detail, secret) || containsSensitive(detail, token) {
		return fallback
	}
	return detail
}

func containsSensitive(msg, secret string) bool {
	return secret != "" && strings.Contains(msg, secret)
}

func writeLaunchError(w http.ResponseWriter, r *http.Request, err error) {
	sanitized := redactLaunchError(err, "", "")
	var (
		notFound           errtypes.NotFound
		badRequest         errtypes.BadRequest
		invalidCredentials errtypes.InvalidCredentials
		permissionDenied   errtypes.PermissionDenied
		internal           errtypes.InternalError
	)
	switch {
	case errors.As(sanitized, &notFound):
		reqres.WriteError(w, r, reqres.APIErrorNotFound, notFound.Error(), sanitized)
	case errors.As(sanitized, &badRequest):
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, badRequest.Error(), sanitized)
	case errors.As(sanitized, &invalidCredentials):
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, invalidCredentials.Error(), sanitized)
	case errors.As(sanitized, &permissionDenied):
		reqres.WriteError(w, r, reqres.APIErrorUntrustedService, permissionDenied.Error(), sanitized)
	case errors.As(sanitized, &internal):
		reqres.WriteError(w, r, reqres.APIErrorServerError, internal.Error(), sanitized)
	default:
		reqres.WriteError(w, r, reqres.APIErrorServerError, "launch failed", sanitized)
	}
}

// writeLaunchJSON marshals the payload before committing a successful status.
func writeLaunchJSON(w http.ResponseWriter, r *http.Request, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error marshalling JSON response", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(encoded); err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing launch response")
	}
}
