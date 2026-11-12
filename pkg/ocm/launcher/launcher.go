// Package launcher renders the page that hands a user over to a web
// application with a POST request. The request is made by the browser, so the
// access token never appears in a URL.
package launcher

import (
	"crypto/rand"
	"encoding/base64"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/v3/pkg/errtypes"
	"golang.org/x/net/idna"
)

var formPage = template.Must(template.New("launcher").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="referrer" content="no-referrer">
<title>Opening application</title>
</head>
<body>
<form id="launch" method="post" action="{{.Action}}">
{{range .Fields}}<input type="hidden" name="{{.Name}}" value="{{.Value}}">
{{end}}<noscript><button type="submit">Open application</button></noscript>
</form>
<script nonce="{{.Nonce}}">document.getElementById("launch").submit();</script>
</body>
</html>
`))

var errorPage = template.Must(template.New("launcher-error").Parse(`<!DOCTYPE html>
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

// Field is one hidden input of the launch form.
type Field struct {
	Name, Value string
}

// RandRead is a seam so tests can force nonce generation to fail.
var RandRead = rand.Read

// NewNonce returns a random value for the Content-Security-Policy script source.
func NewNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := RandRead(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// FormTarget returns the URL the launcher form posts to and the exact
// form-action origin for it. Internationalized hosts are posted to in their
// ASCII form so the policy and the target name the same host; a host that a
// CSP host source cannot name is refused rather than widening the policy.
func FormTarget(appURL string) (action, origin string, err error) {
	refused := errtypes.BadRequest("app host cannot be named in the launcher policy")
	u, err := url.Parse(appURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", "", refused
	}
	host := u.Hostname()
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || !cspHostSource(ascii) {
		return "", "", refused
	}
	hostPort := ascii
	if port := u.Port(); port != "" {
		hostPort += ":" + port
	}
	origin = "https://" + hostPort
	if ascii == strings.ToLower(host) {
		return appURL, origin, nil
	}
	u.Host = hostPort
	return u.String(), origin, nil
}

// cspHostSource reports whether host is a DNS name or IPv4 address made of
// lowercase letters, digits and hyphens in non-empty dot-separated labels.
func cspHostSource(host string) bool {
	if host == "" {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// SetHeaders applies the security headers shared by success and error
// pages. An empty origin means no form may be submitted.
func SetHeaders(w http.ResponseWriter, nonce, origin string) {
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

// WriteForm writes the self-submitting form. action and origin come from FormTarget.
func WriteForm(w http.ResponseWriter, nonce, action, origin string, fields []Field) error {
	SetHeaders(w, nonce, origin)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	return formPage.Execute(w, struct {
		Action string
		Nonce  string
		Fields []Field
	}{action, nonce, fields})
}

// WriteError writes a fixed message, so no remote or token-bearing text reaches the page.
func WriteError(w http.ResponseWriter, nonce string, status int, msg string) {
	SetHeaders(w, nonce, "")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = errorPage.Execute(w, struct{ Message string }{msg})
}
