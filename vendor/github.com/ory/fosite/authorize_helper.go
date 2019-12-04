/*
 * Copyright © 2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * @author		Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @copyright 	2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @license 	Apache-2.0
 *
 */

package fosite

import (
	"net/url"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/pkg/errors"
)

// GetRedirectURIFromRequestValues extracts the redirect_uri from values but does not do any sort of validation.
//
// Considered specifications
// * https://tools.ietf.org/html/rfc6749#section-3.1
//   The endpoint URI MAY include an
//   "application/x-www-form-urlencoded" formatted (per Appendix B) query
//   component ([RFC3986] Section 3.4), which MUST be retained when adding
//   additional query parameters.
func GetRedirectURIFromRequestValues(values url.Values) (string, error) {
	// rfc6749 3.1.   Authorization Endpoint
	// The endpoint URI MAY include an "application/x-www-form-urlencoded" formatted (per Appendix B) query component
	redirectURI, err := url.QueryUnescape(values.Get("redirect_uri"))
	if err != nil {
		return "", errors.WithStack(ErrInvalidRequest.WithHint(`The "redirect_uri" parameter is malformed or missing.`).WithDebug(err.Error()))
	}
	return redirectURI, nil
}

// MatchRedirectURIWithClientRedirectURIs if the given uri is a registered redirect uri. Does not perform
// uri validation.
//
// Considered specifications
// * https://tools.ietf.org/html/rfc6749#section-3.1.2.3
//   If multiple redirection URIs have been registered, if only part of
//   the redirection URI has been registered, or if no redirection URI has
//   been registered, the client MUST include a redirection URI with the
//   authorization request using the "redirect_uri" request parameter.
//
//   When a redirection URI is included in an authorization request, the
//   authorization server MUST compare and match the value received
//   against at least one of the registered redirection URIs (or URI
//   components) as defined in [RFC3986] Section 6, if any redirection
//   URIs were registered.  If the client registration included the full
//   redirection URI, the authorization server MUST compare the two URIs
//   using simple string comparison as defined in [RFC3986] Section 6.2.1.
//
// * https://tools.ietf.org/html/rfc6819#section-4.4.1.7
//   * The authorization server may also enforce the usage and validation
//     of pre-registered redirect URIs (see Section 5.2.3.5).  This will
//     allow for early recognition of authorization "code" disclosure to
//     counterfeit clients.
//   * The attacker will need to use another redirect URI for its
//     authorization process rather than the target web site because it
//     needs to intercept the flow.  So, if the authorization server
//     associates the authorization "code" with the redirect URI of a
//     particular end-user authorization and validates this redirect URI
//     with the redirect URI passed to the token's endpoint, such an
//     attack is detected (see Section 5.2.4.5).
func MatchRedirectURIWithClientRedirectURIs(rawurl string, client Client) (*url.URL, error) {
	if rawurl == "" && len(client.GetRedirectURIs()) == 1 {
		if redirectURIFromClient, err := url.Parse(client.GetRedirectURIs()[0]); err == nil && IsValidRedirectURI(redirectURIFromClient) {
			// If no redirect_uri was given and the client has exactly one valid redirect_uri registered, use that instead
			return redirectURIFromClient, nil
		}
	} else if rawurl != "" && StringInSlice(rawurl, client.GetRedirectURIs()) {
		// If a redirect_uri was given and the clients knows it (simple string comparison!)
		// return it.
		if parsed, err := url.Parse(rawurl); err == nil && IsValidRedirectURI(parsed) {
			// If no redirect_uri was given and the client has exactly one valid redirect_uri registered, use that instead
			return parsed, nil
		}
	}

	return nil, errors.WithStack(ErrInvalidRequest.WithHint(`The "redirect_uri" parameter does not match any of the OAuth 2.0 Client's pre-registered redirect urls.`))
}

// IsValidRedirectURI validates a redirect_uri as specified in:
//
// * https://tools.ietf.org/html/rfc6749#section-3.1.2
//   * The redirection endpoint URI MUST be an absolute URI as defined by [RFC3986] Section 4.3.
//   * The endpoint URI MUST NOT include a fragment component.
// * https://tools.ietf.org/html/rfc3986#section-4.3
//   absolute-URI  = scheme ":" hier-part [ "?" query ]
// * https://tools.ietf.org/html/rfc6819#section-5.1.1
func IsValidRedirectURI(redirectURI *url.URL) bool {
	// We need to explicitly check for a scheme
	if !govalidator.IsRequestURL(redirectURI.String()) {
		return false
	}

	if redirectURI.Fragment != "" {
		// "The endpoint URI MUST NOT include a fragment component."
		return false
	}

	return true
}

func IsRedirectURISecure(redirectURI *url.URL) bool {
	return !(redirectURI.Scheme == "http" && !IsLocalhost(redirectURI))
}

func IsLocalhost(redirectURI *url.URL) bool {
	hn := redirectURI.Hostname()
	return strings.HasSuffix(hn, ".localhost") || hn == "127.0.0.1" || hn == "localhost"
}
