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

// Package spnego reads a Kerberos ticket from the Authorization header, so that
// a client with a TGT authenticates without a password and without an
// intermediate identity provider.
//
// The token is passed through as the client secret for the kerberos auth
// manager to validate; nothing is verified here. The frontend does not hold the
// service keytab — the auth provider does — which is also why the SPNEGO
// exchange is one-legged: there is no way to return a mutual-authentication
// response token from this side.
package spnego

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cs3org/reva/v3/internal/http/interceptors/auth/credential/registry"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func init() {
	registry.Register("spnego", New)
}

// negotiateScheme is the authentication scheme SPNEGO uses, as in RFC 4559.
const negotiateScheme = "Negotiate"

type config struct {
	// AuthType is the auth provider this strategy routes to. It exists so a
	// deployment can register the Kerberos provider under another name without
	// having to patch this.
	AuthType string `docs:"kerberos;The auth provider type the token is sent to." mapstructure:"auth_type"`
}

func (c *config) ApplyDefaults() {
	if c.AuthType == "" {
		c.AuthType = "kerberos"
	}
}

type strategy struct {
	authType string
}

// New returns a credential strategy that reads a SPNEGO token.
func New(m map[string]any) (auth.CredentialStrategy, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	return &strategy{authType: c.AuthType}, nil
}

// GetCredentials extracts the SPNEGO token from the Authorization header.
//
// The token is left base64-encoded: that is how it arrives, and the auth
// manager has to decode it anyway, so decoding here would only mean encoding it
// again to put it in the CS3 request.
func (s *strategy) GetCredentials(w http.ResponseWriter, r *http.Request) (*auth.Credentials, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return nil, fmt.Errorf("no Authorization header provided")
	}

	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, negotiateScheme) {
		// Another scheme, which another strategy in the chain will handle.
		return nil, fmt.Errorf("no SPNEGO credentials provided")
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("empty SPNEGO token")
	}

	return &auth.Credentials{
		Type: s.authType,
		// No client id: the ticket says who the client is, and it is the only
		// statement of identity here that has been verified.
		ClientSecret: token,
	}, nil
}

// AddWWWAuthenticate offers Kerberos to a client that has not authenticated.
//
// The realm is not included. RFC 4559 defines the challenge as the bare scheme,
// and a client that sees a parameter it does not expect may decline to
// negotiate at all.
func (s *strategy) AddWWWAuthenticate(w http.ResponseWriter, r *http.Request, realm string) {
	w.Header().Add("WWW-Authenticate", negotiateScheme)
}
