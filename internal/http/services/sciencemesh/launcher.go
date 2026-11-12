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
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/launcher"
)

// WebappLaunch renders a page that posts the exchanged access token straight
// to the validated app URL. Nothing is stored; the token lives only in this
// response body.
func (h *appsHandler) WebappLaunch(w http.ResponseWriter, r *http.Request) {
	nonce, err := launcher.NewNonce()
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Msg("could not create launcher nonce")
		launcher.WriteError(w, "", http.StatusInternalServerError, "The launch failed.")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeLauncherError(w, r, nonce, http.StatusBadRequest, "The launch request could not be read.")
		return
	}
	filePath := r.Form.Get("file")
	if filePath == "" {
		writeLauncherError(w, r, nonce, http.StatusBadRequest, "The launch request is missing the file.")
		return
	}

	shareID, rel, err := h.shareInfo(filePath)
	if err != nil {
		writeLauncherLaunchError(w, r, nonce, err)
		return
	}
	var action, formAction string
	payload, err := h.buildLaunch(r.Context(), shareID, rel, func(appURL string) error {
		var err error
		action, formAction, err = launcher.FormTarget(appURL)
		return err
	})
	if err != nil {
		writeLauncherLaunchError(w, r, nonce, err)
		return
	}

	fields := []launcher.Field{{Name: "access_token", Value: payload.AccessToken}}
	if redirect := h.allowedExpiredSessionRedirect(r.Form.Get("expired_session_redirect_uri")); redirect != "" {
		fields = append(fields, launcher.Field{Name: "expired_session_redirect_uri", Value: redirect})
	}
	if err := launcher.WriteForm(w, nonce, action, formAction, fields); err != nil {
		appctx.GetLogger(r.Context()).Error().Msg("error writing launcher page")
	}
}

// allowedExpiredSessionRedirect keeps the redirect only when it points at the
// receiver's own web origin, https://<provider_domain>. Anything else is dropped.
func (h *appsHandler) allowedExpiredSessionRedirect(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" {
		return ""
	}
	if u.Host == "" || !strings.EqualFold(u.Hostname(), h.receiverDomain) {
		return ""
	}
	if p := u.Port(); p != "" && p != "443" {
		return ""
	}
	return raw
}

func writeLauncherLaunchError(w http.ResponseWriter, r *http.Request, nonce string, err error) {
	sanitized := redactLaunchError(err, "", "")
	status, msg := launcherErrorText(sanitized)
	appctx.GetLogger(r.Context()).Error().Err(sanitized).Msg("webapp launch failed")
	launcher.WriteError(w, nonce, status, msg)
}

func writeLauncherError(w http.ResponseWriter, r *http.Request, nonce string, status int, msg string) {
	appctx.GetLogger(r.Context()).Error().Int("status", status).Msg("webapp launch rejected")
	launcher.WriteError(w, nonce, status, msg)
}

// launcherErrorText maps a redacted launch error to a status and a fixed
// message, so no remote or token-bearing text reaches the page.
func launcherErrorText(err error) (int, string) {
	var (
		notFound           errtypes.NotFound
		badRequest         errtypes.BadRequest
		invalidCredentials errtypes.InvalidCredentials
		permissionDenied   errtypes.PermissionDenied
	)
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "The launch did not complete in time."
	case errors.As(err, &notFound):
		return http.StatusNotFound, "The shared application was not found."
	case errors.As(err, &badRequest):
		return http.StatusBadRequest, "The shared application cannot be launched."
	case errors.As(err, &invalidCredentials):
		return http.StatusUnauthorized, "You are not signed in."
	case errors.As(err, &permissionDenied):
		return http.StatusForbidden, "The sharing service is not trusted."
	default:
		return http.StatusInternalServerError, "The launch failed."
	}
}
