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
	"crypto/rand"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

var launcherPage = template.Must(template.New("launcher").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="referrer" content="no-referrer">
<title>Opening application</title>
</head>
<body>
<form id="launch" method="post" action="{{.Action}}">
<input type="hidden" name="access_token" value="{{.AccessToken}}">
{{if .ExpiredSessionRedirectURI}}<input type="hidden" name="expired_session_redirect_uri" value="{{.ExpiredSessionRedirectURI}}">
{{end}}<noscript><button type="submit">Open application</button></noscript>
</form>
<script nonce="{{.Nonce}}">document.getElementById("launch").submit();</script>
</body>
</html>
`))

var launcherErrorPage = template.Must(template.New("launcher-error").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Cannot open application</title>
</head>
<body>
<p>{{.Message}}</p>
</body>
</html>
`))

type launcherData struct {
	Action                    string
	AccessToken               string
	ExpiredSessionRedirectURI string
	Nonce                     string
}

// WebappLaunch renders a page that posts the exchanged access token straight
// to the validated app URL. Nothing is stored; the token lives only in this
// response body.
func (h *appsHandler) WebappLaunch(w http.ResponseWriter, r *http.Request) {
	nonce, err := newCSPNonce()
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Msg("could not create launcher nonce")
		renderLauncherError(w, "", http.StatusInternalServerError, "The launch failed.")
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
	payload, err := h.buildLaunch(r.Context(), shareID, rel)
	if err != nil {
		writeLauncherLaunchError(w, r, nonce, err)
		return
	}

	appOrigin := cspFormAction(payload.AppURL)

	data := launcherData{
		Action:                    payload.AppURL,
		AccessToken:               payload.AccessToken,
		ExpiredSessionRedirectURI: h.allowedExpiredSessionRedirect(r.Form.Get("expired_session_redirect_uri")),
		Nonce:                     nonce,
	}
	setLauncherHeaders(w, nonce, appOrigin)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := launcherPage.Execute(w, data); err != nil {
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

// randRead is a seam so tests can force nonce generation to fail.
var randRead = rand.Read

func newCSPNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := randRead(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// cspFormAction returns the form-action source for an app URL that already
// passed the shared launch validation. A plain ASCII origin is used as is.
// Hosts that cannot be written safely as a CSP host source (non-ASCII names,
// or punctuation such as ';' and ',') fall back to the https: scheme source, so
// a validated URL always gets a usable form and nothing can inject directives.
func cspFormAction(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	for _, r := range u.Host {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			strings.ContainsRune(".-_:[]", r)
		if !ok {
			return "https:"
		}
	}
	return "https://" + u.Host
}

// setLauncherHeaders applies the security headers shared by success and error
// pages. An empty origin means no form may be submitted.
func setLauncherHeaders(w http.ResponseWriter, nonce, origin string) {
	formAction := "'none'"
	if origin != "" {
		formAction = origin
	}
	csp := "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action " + formAction
	if nonce != "" {
		csp += "; script-src 'nonce-" + nonce + "'"
	} else {
		csp += "; script-src 'none'"
	}
	h := w.Header()
	h.Set("Content-Security-Policy", csp)
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
}

func writeLauncherLaunchError(w http.ResponseWriter, r *http.Request, nonce string, err error) {
	sanitized := redactLaunchError(err, "", "")
	status, msg := launcherErrorText(sanitized)
	appctx.GetLogger(r.Context()).Error().Err(sanitized).Msg("webapp launch failed")
	renderLauncherError(w, nonce, status, msg)
}

func writeLauncherError(w http.ResponseWriter, r *http.Request, nonce string, status int, msg string) {
	appctx.GetLogger(r.Context()).Error().Int("status", status).Msg("webapp launch rejected")
	renderLauncherError(w, nonce, status, msg)
}

func renderLauncherError(w http.ResponseWriter, nonce string, status int, msg string) {
	setLauncherHeaders(w, nonce, "")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = launcherErrorPage.Execute(w, struct{ Message string }{msg})
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
